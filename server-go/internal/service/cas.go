package service

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	gitcrypto "git-ai-server/internal/crypto"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CasUploadRequest struct {
	Hash     string            `json:"hash"`
	Content  any               `json:"content"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type CasUploadResult struct {
	Results      []CasUploadStatus `json:"results"`
	SuccessCount int               `json:"success_count"`
	FailureCount int               `json:"failure_count"`
}

type CasUploadStatus struct {
	Hash   string `json:"hash"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type CasReadResult struct {
	Content     string
	ContentType string
}

type CasReadObjectsResult struct {
	Results      []CasReadObjectStatus `json:"results"`
	SuccessCount int                   `json:"success_count"`
	FailureCount int                   `json:"failure_count"`
}

type CasReadObjectStatus struct {
	Hash    string `json:"hash"`
	Status  string `json:"status"`
	Content any    `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

type CasService struct {
	Pool   *pgxpool.Pool
	CASKey string
}

func (s *CasService) UploadObject(ctx context.Context, hash string, content any) (string, error) {
	h := strings.TrimSpace(strings.ToLower(hash))

	var exists bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM cas_entries WHERE hash = $1)`, h,
	).Scan(&exists)
	if err != nil {
		return "", fmt.Errorf("checking existing entry: %w", err)
	}
	if exists {
		return h, nil
	}

	serialized, err := json.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("serializing content: %w", err)
	}

	compressed, err := zlibCompress(serialized)
	if err != nil {
		return "", fmt.Errorf("compressing content: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(compressed)

	encrypted, err := gitcrypto.EncryptCAS(b64, s.CASKey)
	if err != nil {
		return "", fmt.Errorf("encrypting content: %w", err)
	}

	_, err = s.Pool.Exec(ctx,
		`INSERT INTO cas_entries (id, hash, encrypted_content, content_type, created_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, now())`,
		h, encrypted, "application/json",
	)
	if err != nil {
		return "", fmt.Errorf("inserting cas entry: %w", err)
	}

	return h, nil
}

func (s *CasService) UploadObjects(ctx context.Context, objects []CasUploadRequest) (*CasUploadResult, error) {
	results := make([]CasUploadStatus, 0, len(objects))

	for _, obj := range objects {
		h := strings.TrimSpace(strings.ToLower(obj.Hash))
		if h == "" {
			results = append(results, CasUploadStatus{
				Hash:   obj.Hash,
				Status: "error",
				Error:  "hash is required",
			})
			continue
		}

		if obj.Content == nil {
			results = append(results, CasUploadStatus{
				Hash:   h,
				Status: "error",
				Error:  "content is required",
			})
			continue
		}

		if _, err := s.UploadObject(ctx, h, obj.Content); err != nil {
			results = append(results, CasUploadStatus{
				Hash:   h,
				Status: "error",
				Error:  err.Error(),
			})
		} else {
			results = append(results, CasUploadStatus{
				Hash:   h,
				Status: "ok",
			})
		}
	}

	successCount := 0
	for _, r := range results {
		if r.Status == "ok" {
			successCount++
		}
	}

	return &CasUploadResult{
		Results:      results,
		SuccessCount: successCount,
		FailureCount: len(results) - successCount,
	}, nil
}

func (s *CasService) UploadContent(ctx context.Context, content string, contentType string) (string, error) {
	if contentType == "" {
		contentType = "text/plain"
	}

	compressed, err := zlibCompress([]byte(content))
	if err != nil {
		return "", fmt.Errorf("compressing content: %w", err)
	}

	h := sha256.Sum256(compressed)
	contentHash := hex.EncodeToString(h[:])

	var exists bool
	err = s.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM cas_entries WHERE hash = $1)`, contentHash,
	).Scan(&exists)
	if err != nil {
		return "", fmt.Errorf("checking existing entry: %w", err)
	}
	if exists {
		return contentHash, nil
	}

	b64 := base64.StdEncoding.EncodeToString(compressed)
	encrypted, err := gitcrypto.EncryptCAS(b64, s.CASKey)
	if err != nil {
		return "", fmt.Errorf("encrypting content: %w", err)
	}

	_, err = s.Pool.Exec(ctx,
		`INSERT INTO cas_entries (id, hash, encrypted_content, content_type, created_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, now())`,
		contentHash, encrypted, contentType,
	)
	if err != nil {
		return "", fmt.Errorf("inserting cas entry: %w", err)
	}

	return contentHash, nil
}

func (s *CasService) ReadContent(ctx context.Context, hash string) (*CasReadResult, error) {
	var encrypted, ct string
	err := s.Pool.QueryRow(ctx,
		`SELECT encrypted_content, content_type FROM cas_entries WHERE hash = $1`, hash,
	).Scan(&encrypted, &ct)
	if err != nil {
		return nil, nil // not found
	}

	decrypted, err := gitcrypto.DecryptCAS(encrypted, s.CASKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting content: %w", err)
	}

	compressedBytes, err := base64.StdEncoding.DecodeString(decrypted)
	if err != nil {
		return nil, fmt.Errorf("decoding base64: %w", err)
	}

	decompressed, err := zlibDecompress(compressedBytes)
	if err != nil {
		return nil, fmt.Errorf("decompressing content: %w", err)
	}

	return &CasReadResult{
		Content:     string(decompressed),
		ContentType: ct,
	}, nil
}

func (s *CasService) ReadObject(ctx context.Context, hash string) (any, error) {
	result, err := s.ReadContent(ctx, strings.TrimSpace(strings.ToLower(hash)))
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	var parsed any
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		return nil, nil
	}

	return parsed, nil
}

func (s *CasService) ReadObjects(ctx context.Context, hashes []string) (*CasReadObjectsResult, error) {
	results := make([]CasReadObjectStatus, 0, len(hashes))

	for _, originalHash := range hashes {
		h := strings.TrimSpace(strings.ToLower(originalHash))
		if h == "" {
			continue
		}

		content, err := s.ReadObject(ctx, h)
		if err != nil {
			results = append(results, CasReadObjectStatus{
				Hash:   h,
				Status: "error",
				Error:  err.Error(),
			})
			continue
		}

		if content == nil {
			results = append(results, CasReadObjectStatus{
				Hash:   h,
				Status: "error",
				Error:  "Content not found",
			})
			continue
		}

		results = append(results, CasReadObjectStatus{
			Hash:    h,
			Status:  "ok",
			Content: content,
		})
	}

	successCount := 0
	for _, r := range results {
		if r.Status == "ok" {
			successCount++
		}
	}

	return &CasReadObjectsResult{
		Results:      results,
		SuccessCount: successCount,
		FailureCount: len(results) - successCount,
	}, nil
}

func zlibCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func zlibDecompress(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
