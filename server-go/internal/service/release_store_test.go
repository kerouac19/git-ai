package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func makeStore(t *testing.T) *ReleaseStore {
	t.Helper()
	dir, err := os.MkdirTemp("", "release-store-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return &ReleaseStore{Root: dir}
}

func sha256Hex2(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func writeArtifact(t *testing.T, s *ReleaseStore, channel, tag, name string, body []byte) {
	t.Helper()
	if err := s.PutArtifact(channel, tag, name, bytes.NewReader(body)); err != nil {
		t.Fatalf("PutArtifact(%s/%s/%s) failed: %v", channel, tag, name, err)
	}
}

// buildValidRelease writes 3 artifacts and returns the checksum of SHA256SUMS.
func buildValidRelease(t *testing.T, s *ReleaseStore, channel, tag string) (string, []byte) {
	t.Helper()
	sh := []byte("#!/bin/sh\necho install\n")
	ps1 := []byte("Write-Host install\n")
	sums := fmt.Sprintf("%s  install.sh\n%s  install.ps1\n", sha256Hex2(sh), sha256Hex2(ps1))
	writeArtifact(t, s, channel, tag, "install.sh", sh)
	writeArtifact(t, s, channel, tag, "install.ps1", ps1)
	writeArtifact(t, s, channel, tag, "SHA256SUMS", []byte(sums))
	return sha256Hex2([]byte(sums)), []byte(sums)
}

func TestPutArtifactIdempotentAndConflict(t *testing.T) {
	s := makeStore(t)
	body := []byte("hello\n")
	if err := s.PutArtifact("latest", "v1.0.0", "install.sh", bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	// Same content -> ok.
	if err := s.PutArtifact("latest", "v1.0.0", "install.sh", bytes.NewReader(body)); err != nil {
		t.Fatalf("retry same content: %v", err)
	}
	// Different content -> conflict.
	err := s.PutArtifact("latest", "v1.0.0", "install.sh", bytes.NewReader([]byte("different")))
	if err != ErrReleaseConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
	// File not overwritten.
	got, _ := os.ReadFile(filepath.Join(s.Root, "latest", "v1.0.0", "install.sh"))
	if !bytes.Equal(got, body) {
		t.Fatalf("artifact was overwritten: %q", got)
	}
}

func TestPutArtifactConcurrent(t *testing.T) {
	s := makeStore(t)
	var wg sync.WaitGroup
	var okCount, conflictCount int32
	a := []byte("aaaaaaaaaa")
	b := []byte("bbbbbbbbbb")
	wg.Add(20)
	for i := 0; i < 20; i++ {
		body := a
		if i%2 == 1 {
			body = b
		}
		go func(body []byte) {
			defer wg.Done()
			err := s.PutArtifact("latest", "vc", "install.sh", bytes.NewReader(body))
			if err == nil {
				atomic.AddInt32(&okCount, 1)
			} else if err == ErrReleaseConflict {
				atomic.AddInt32(&conflictCount, 1)
			} else {
				t.Errorf("unexpected: %v", err)
			}
		}(body)
	}
	wg.Wait()
	if okCount == 0 || conflictCount == 0 {
		t.Fatalf("expected both ok and conflict; got ok=%d conflict=%d", okCount, conflictCount)
	}
	// Final stored content must be exactly one of the two.
	got, _ := os.ReadFile(filepath.Join(s.Root, "latest", "vc", "install.sh"))
	if !bytes.Equal(got, a) && !bytes.Equal(got, b) {
		t.Fatalf("unexpected final content: %q", got)
	}
}

func TestInvalidChannelTagName(t *testing.T) {
	s := makeStore(t)
	cases := []struct {
		ch, tag, name string
		want          error
	}{
		{"foo", "v1", "install.sh", ErrReleaseInvalidChannel},
		{"latest", "..", "install.sh", ErrReleaseInvalidTag},
		{"latest", ".hidden", "install.sh", ErrReleaseInvalidTag},
		{"latest", "-bad", "install.sh", ErrReleaseInvalidTag},
		{"latest", "", "install.sh", ErrReleaseInvalidTag},
		{"latest", strings.Repeat("a", 65), "install.sh", ErrReleaseInvalidTag},
		{"latest", "v1/../x", "install.sh", ErrReleaseInvalidTag},
		{"latest", "v1", "evil.sh", ErrReleaseInvalidName},
	}
	for _, tc := range cases {
		err := s.PutArtifact(tc.ch, tc.tag, tc.name, bytes.NewReader([]byte("x")))
		if err != tc.want {
			t.Fatalf("PutArtifact(%q,%q,%q): got %v want %v", tc.ch, tc.tag, tc.name, err, tc.want)
		}
	}
}

func TestPutCurrentValid(t *testing.T) {
	s := makeStore(t)
	checksum, _ := buildValidRelease(t, s, "latest", "v1.2.3")
	body, _ := json.Marshal(CurrentPointer{Tag: "v1.2.3", Checksum: checksum, UpdatedAt: "2026-04-20T00:00:00Z"})
	if err := s.PutCurrent("latest", body); err != nil {
		t.Fatal(err)
	}
	raw, err := s.GetCurrentRaw("latest")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, body) {
		t.Fatalf("current.json mismatch")
	}
}

func TestPutCurrentMissingArtifacts(t *testing.T) {
	s := makeStore(t)
	body, _ := json.Marshal(CurrentPointer{Tag: "v1", Checksum: strings.Repeat("0", 64)})
	if err := s.PutCurrent("latest", body); err != ErrReleaseMissingArtifact {
		t.Fatalf("want ErrReleaseMissingArtifact, got %v", err)
	}
}

func TestPutCurrentChecksumMismatch(t *testing.T) {
	s := makeStore(t)
	_, _ = buildValidRelease(t, s, "latest", "v1")
	body, _ := json.Marshal(CurrentPointer{Tag: "v1", Checksum: strings.Repeat("0", 64)})
	if err := s.PutCurrent("latest", body); err != ErrReleaseChecksumMismatch {
		t.Fatalf("want checksum mismatch, got %v", err)
	}
}

func TestPutCurrentSHA256SUMSSingleSpaceRejected(t *testing.T) {
	s := makeStore(t)
	sh := []byte("#!/bin/sh\n")
	ps1 := []byte("Write-Host\n")
	// Single-space separator should not be parsed as an entry by strict parser.
	sums := fmt.Sprintf("%s install.sh\n%s install.ps1\n", sha256Hex2(sh), sha256Hex2(ps1))
	writeArtifact(t, s, "latest", "v1", "install.sh", sh)
	writeArtifact(t, s, "latest", "v1", "install.ps1", ps1)
	writeArtifact(t, s, "latest", "v1", "SHA256SUMS", []byte(sums))
	body, _ := json.Marshal(CurrentPointer{Tag: "v1", Checksum: sha256Hex2([]byte(sums))})
	err := s.PutCurrent("latest", body)
	if err == nil {
		t.Fatal("expected error for single-space SHA256SUMS")
	}
}

func TestPutCurrentSHA256SUMSWrongHash(t *testing.T) {
	s := makeStore(t)
	sh := []byte("#!/bin/sh\n")
	ps1 := []byte("Write-Host\n")
	bogus := strings.Repeat("a", 64)
	sums := fmt.Sprintf("%s  install.sh\n%s  install.ps1\n", bogus, sha256Hex2(ps1))
	writeArtifact(t, s, "latest", "v1", "install.sh", sh)
	writeArtifact(t, s, "latest", "v1", "install.ps1", ps1)
	writeArtifact(t, s, "latest", "v1", "SHA256SUMS", []byte(sums))
	body, _ := json.Marshal(CurrentPointer{Tag: "v1", Checksum: sha256Hex2([]byte(sums))})
	err := s.PutCurrent("latest", body)
	if err == nil {
		t.Fatal("expected error for mismatched install.sh hash in SHA256SUMS")
	}
}

func TestResolveEffectiveChannel(t *testing.T) {
	s := makeStore(t)
	if _, ok := s.ResolveEffectiveChannel("foo"); ok {
		t.Fatal("foo should be rejected")
	}
	if _, ok := s.ResolveEffectiveChannel("../etc"); ok {
		t.Fatal("path traversal channel should be rejected")
	}
	if _, ok := s.ResolveEffectiveChannel(""); ok {
		t.Fatal("empty channel should be rejected")
	}
	if _, ok := s.ResolveEffectiveChannel("enterprise-latest"); ok {
		t.Fatal("no data at all -> ok=false expected")
	}
	// Populate latest.
	checksum, _ := buildValidRelease(t, s, "latest", "v1")
	body, _ := json.Marshal(CurrentPointer{Tag: "v1", Checksum: checksum})
	if err := s.PutCurrent("latest", body); err != nil {
		t.Fatal(err)
	}
	eff, ok := s.ResolveEffectiveChannel("enterprise-latest")
	if !ok || eff != "latest" {
		t.Fatalf("fallback failed: eff=%q ok=%v", eff, ok)
	}
	if _, ok := s.ResolveEffectiveChannel("enterprise-next"); ok {
		t.Fatal("enterprise-next should not fall back when next is absent")
	}
	// Populate next.
	checksum2, _ := buildValidRelease(t, s, "next", "v2-rc")
	body2, _ := json.Marshal(CurrentPointer{Tag: "v2-rc", Checksum: checksum2})
	if err := s.PutCurrent("next", body2); err != nil {
		t.Fatal(err)
	}
	eff, ok = s.ResolveEffectiveChannel("enterprise-next")
	if !ok || eff != "next" {
		t.Fatalf("enterprise-next fallback failed: eff=%q ok=%v", eff, ok)
	}
}

func TestOpenCurrentArtifactFallback(t *testing.T) {
	s := makeStore(t)
	checksum, sums := buildValidRelease(t, s, "latest", "v9")
	body, _ := json.Marshal(CurrentPointer{Tag: "v9", Checksum: checksum})
	if err := s.PutCurrent("latest", body); err != nil {
		t.Fatal(err)
	}
	rc, _, err := s.OpenCurrentArtifact("enterprise-latest", "SHA256SUMS")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(rc)
	if !bytes.Equal(buf.Bytes(), sums) {
		t.Fatalf("enterprise-latest download did not return latest's SHA256SUMS")
	}
}
