package service

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Known release channels.
var releaseChannels = []string{"latest", "next", "enterprise-latest", "enterprise-next"}

// Known artifact filenames the store will accept.
var releaseArtifactNames = map[string]bool{
	"SHA256SUMS":  true,
	"install.sh":  true,
	"install.ps1": true,
}

var tagRegexp = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// Errors returned by ReleaseStore to allow handlers to map them to HTTP status
// codes without coupling to specific string contents.
var (
	ErrReleaseInvalidChannel   = errors.New("invalid release channel")
	ErrReleaseInvalidTag       = errors.New("invalid release tag")
	ErrReleaseInvalidName      = errors.New("invalid release artifact name")
	ErrReleaseConflict         = errors.New("release artifact already exists with different content")
	ErrReleaseMissingArtifact  = errors.New("release artifact missing")
	ErrReleaseChecksumMismatch = errors.New("release checksum mismatch")
	ErrReleaseNotFound         = errors.New("release not found")
	ErrReleaseBadCurrentJSON   = errors.New("invalid current.json payload")
)

// CurrentPointer is the JSON shape of current.json.
type CurrentPointer struct {
	Tag       string `json:"tag"`
	Checksum  string `json:"checksum"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// ReleaseStore is a filesystem-backed store for release metadata artifacts.
type ReleaseStore struct {
	Root string
}

// KnownReleaseChannel reports whether the given channel name is on the
// allowlist. Exposed as a package function to allow handler-level callers to
// reject unknown channels before touching the filesystem.
func KnownReleaseChannel(channel string) bool {
	for _, c := range releaseChannels {
		if c == channel {
			return true
		}
	}
	return false
}

// KnownReleaseArtifact reports whether the given filename is on the allowlist.
func KnownReleaseArtifact(name string) bool {
	return releaseArtifactNames[name]
}

// ValidateTag performs the strict tag-name validation rules from the plan.
func ValidateTag(tag string) error {
	if tag == "" || len(tag) > 64 {
		return ErrReleaseInvalidTag
	}
	if strings.HasPrefix(tag, ".") || strings.HasPrefix(tag, "-") {
		return ErrReleaseInvalidTag
	}
	if strings.Contains(tag, "..") || strings.Contains(tag, "/") || strings.Contains(tag, "\\") {
		return ErrReleaseInvalidTag
	}
	if !tagRegexp.MatchString(tag) {
		return ErrReleaseInvalidTag
	}
	return nil
}

func (s *ReleaseStore) channelDir(channel string) string {
	return filepath.Join(s.Root, channel)
}

func (s *ReleaseStore) tagDir(channel, tag string) string {
	return filepath.Join(s.Root, channel, tag)
}

func (s *ReleaseStore) currentPath(channel string) string {
	return filepath.Join(s.Root, channel, "current.json")
}

// hasCurrent returns true if channel/current.json exists. Assumes caller has
// already validated the channel via KnownReleaseChannel.
func (s *ReleaseStore) hasCurrent(channel string) bool {
	info, err := os.Stat(s.currentPath(channel))
	return err == nil && !info.IsDir()
}

// ResolveEffectiveChannel returns the effective channel directory to read
// from given the requested channel. Enterprise channels fall back to their
// public counterparts if no enterprise-specific data is present.
func (s *ReleaseStore) ResolveEffectiveChannel(requested string) (effective string, ok bool) {
	if !KnownReleaseChannel(requested) {
		return "", false
	}
	if s.hasCurrent(requested) {
		return requested, true
	}
	switch requested {
	case "enterprise-latest":
		if s.hasCurrent("latest") {
			return "latest", true
		}
	case "enterprise-next":
		if s.hasCurrent("next") {
			return "next", true
		}
	}
	return "", false
}

// GetCurrentRaw returns the raw bytes of the given channel's current.json.
// Returns ErrReleaseNotFound if it does not exist.
func (s *ReleaseStore) GetCurrentRaw(channel string) ([]byte, error) {
	if !KnownReleaseChannel(channel) {
		return nil, ErrReleaseInvalidChannel
	}
	b, err := os.ReadFile(s.currentPath(channel))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrReleaseNotFound
		}
		return nil, err
	}
	return b, nil
}

// OpenArtifact opens a specific tag's artifact file for reading.
func (s *ReleaseStore) OpenArtifact(channel, tag, name string) (io.ReadCloser, int64, error) {
	if !KnownReleaseChannel(channel) {
		return nil, 0, ErrReleaseInvalidChannel
	}
	if err := ValidateTag(tag); err != nil {
		return nil, 0, err
	}
	if !KnownReleaseArtifact(name) {
		return nil, 0, ErrReleaseInvalidName
	}
	p := filepath.Join(s.tagDir(channel, tag), name)
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, ErrReleaseMissingArtifact
		}
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// OpenCurrentArtifact resolves the channel's current tag, then opens the named
// artifact. Returns ErrReleaseNotFound when current.json is missing.
func (s *ReleaseStore) OpenCurrentArtifact(channel, name string) (io.ReadCloser, int64, error) {
	if !KnownReleaseArtifact(name) {
		return nil, 0, ErrReleaseInvalidName
	}
	effective, ok := s.ResolveEffectiveChannel(channel)
	if !ok {
		return nil, 0, ErrReleaseNotFound
	}
	body, err := s.GetCurrentRaw(effective)
	if err != nil {
		return nil, 0, err
	}
	var cur CurrentPointer
	if err := json.Unmarshal(body, &cur); err != nil {
		return nil, 0, ErrReleaseBadCurrentJSON
	}
	if err := ValidateTag(cur.Tag); err != nil {
		return nil, 0, err
	}
	return s.OpenArtifact(effective, cur.Tag, name)
}

// PutArtifact stores an artifact for the given channel/tag with immutable
// create semantics. Returns nil on success (including idempotent retries with
// identical content), ErrReleaseConflict on content divergence.
func (s *ReleaseStore) PutArtifact(channel, tag, name string, r io.Reader) error {
	if !KnownReleaseChannel(channel) {
		return ErrReleaseInvalidChannel
	}
	if err := ValidateTag(tag); err != nil {
		return err
	}
	if !KnownReleaseArtifact(name) {
		return ErrReleaseInvalidName
	}

	dir := s.tagDir(channel, tag)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	nonce, err := randomNonce()
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".tmp-"+name+"-"+nonce)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	var tmpCleaned bool
	defer func() {
		if !tmpCleaned {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	target := filepath.Join(dir, name)

	// Attempt hard link for immutable-create semantics.
	for attempt := 0; attempt < 2; attempt++ {
		// Fast path: target already exists -> compare contents.
		if _, err := os.Stat(target); err == nil {
			same, cmpErr := filesEqual(tmp, target)
			if cmpErr != nil {
				return cmpErr
			}
			_ = os.Remove(tmp)
			tmpCleaned = true
			if same {
				return nil
			}
			return ErrReleaseConflict
		}

		if err := os.Link(tmp, target); err != nil {
			if errors.Is(err, os.ErrExist) {
				// Race: another writer won. Loop back to compare contents.
				continue
			}
			return err
		}
		_ = os.Remove(tmp)
		tmpCleaned = true
		return nil
	}
	return ErrReleaseConflict
}

// PutCurrent writes current.json after verifying the referenced release is
// internally consistent with what clients will fetch.
func (s *ReleaseStore) PutCurrent(channel string, body []byte) error {
	if !KnownReleaseChannel(channel) {
		return ErrReleaseInvalidChannel
	}

	var cur CurrentPointer
	if err := json.Unmarshal(body, &cur); err != nil {
		return ErrReleaseBadCurrentJSON
	}
	if err := ValidateTag(cur.Tag); err != nil {
		return err
	}
	cur.Checksum = strings.ToLower(strings.TrimSpace(cur.Checksum))
	if len(cur.Checksum) != 64 {
		return ErrReleaseBadCurrentJSON
	}

	// All 3 files must exist.
	tagDir := s.tagDir(channel, cur.Tag)
	for name := range releaseArtifactNames {
		p := filepath.Join(tagDir, name)
		if _, err := os.Stat(p); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return ErrReleaseMissingArtifact
			}
			return err
		}
	}

	// checksum must match SHA256SUMS.
	sumsBytes, err := os.ReadFile(filepath.Join(tagDir, "SHA256SUMS"))
	if err != nil {
		return err
	}
	actualSumsHash := sha256Hex(sumsBytes)
	if actualSumsHash != cur.Checksum {
		return ErrReleaseChecksumMismatch
	}

	// Parse SHA256SUMS using strict double-space semantics matching client.
	scriptHashes, err := parseSHA256SUMS(sumsBytes)
	if err != nil {
		return err
	}
	for _, script := range []string{"install.sh", "install.ps1"} {
		expected, ok := scriptHashes[script]
		if !ok {
			return fmt.Errorf("%w: SHA256SUMS missing %s entry", ErrReleaseBadCurrentJSON, script)
		}
		actual, err := hashFile(filepath.Join(tagDir, script))
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("%w: %s hash mismatch", ErrReleaseChecksumMismatch, script)
		}
	}

	// Atomically replace channel/current.json.
	if err := os.MkdirAll(s.channelDir(channel), 0o755); err != nil {
		return err
	}
	nonce, err := randomNonce()
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.channelDir(channel), ".current.tmp-"+nonce)
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.currentPath(channel)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// parseSHA256SUMS parses the file using the exact same rule as the client:
// trim each line, split on the literal two-space separator.
func parseSHA256SUMS(body []byte) (map[string]string, error) {
	out := map[string]string{}
	for _, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		hash, name, found := strings.Cut(line, "  ")
		if !found {
			// Not an error: permit extra unrecognized lines, but we simply
			// do not register them. The caller checks for required entries.
			continue
		}
		hash = strings.TrimSpace(hash)
		name = strings.TrimSpace(name)
		if hash == "" || name == "" {
			continue
		}
		out[name] = strings.ToLower(hash)
	}
	return out, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func filesEqual(a, b string) (bool, error) {
	af, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer af.Close()
	bf, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer bf.Close()

	aInfo, err := af.Stat()
	if err != nil {
		return false, err
	}
	bInfo, err := bf.Stat()
	if err != nil {
		return false, err
	}
	if aInfo.Size() != bInfo.Size() {
		return false, nil
	}
	bufA := make([]byte, 64*1024)
	bufB := make([]byte, 64*1024)
	for {
		na, errA := io.ReadFull(af, bufA)
		nb, errB := io.ReadFull(bf, bufB)
		if !bytes.Equal(bufA[:na], bufB[:nb]) {
			return false, nil
		}
		if errA == io.EOF || errA == io.ErrUnexpectedEOF {
			return errB == io.EOF || errB == io.ErrUnexpectedEOF, nil
		}
		if errA != nil {
			return false, errA
		}
		if errB != nil {
			return false, errB
		}
	}
}

func randomNonce() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
