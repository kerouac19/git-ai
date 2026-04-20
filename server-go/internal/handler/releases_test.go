package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"git-ai-server/internal/middleware"
	"git-ai-server/internal/service"

	"github.com/gin-gonic/gin"
)

func newTestRouter(t *testing.T) (*gin.Engine, *service.ReleaseStore, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir, err := os.MkdirTemp("", "release-handler-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	store := &service.ReleaseStore{Root: dir}
	const token = "test-token"
	auth := middleware.UploadTokenAuth(token)
	readH := &ReleaseHandler{Store: store}
	adminH := &ReleaseAdminHandler{Store: store}

	r := gin.New()
	r.GET("/worker/releases", readH.GetReleases)
	r.GET("/worker/releases/:channel/download/:name", readH.Download)

	api := r.Group("/api")
	api.PUT("/releases/:channel/artifacts/:tag/:name", auth, adminH.PutArtifact)
	api.PUT("/releases/:channel/current.json", auth, adminH.PutCurrent)
	api.GET("/releases/:channel/current.json", auth, adminH.GetCurrent)
	return r, store, token
}

func hexSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func doReq(t *testing.T, r http.Handler, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func seedRelease(t *testing.T, r http.Handler, token, channel, tag string) {
	t.Helper()
	sh := []byte("#!/bin/sh\n")
	ps1 := []byte("Write-Host\n")
	sums := fmt.Sprintf("%s  install.sh\n%s  install.ps1\n", hexSum(sh), hexSum(ps1))
	headers := map[string]string{"Authorization": "Bearer " + token}
	for _, a := range []struct {
		name string
		body []byte
	}{{"install.sh", sh}, {"install.ps1", ps1}, {"SHA256SUMS", []byte(sums)}} {
		w := doReq(t, r, "PUT",
			fmt.Sprintf("/api/releases/%s/artifacts/%s/%s", channel, tag, a.name),
			a.body, headers)
		if w.Code != http.StatusOK {
			t.Fatalf("seed PUT %s: status %d body %s", a.name, w.Code, w.Body.String())
		}
	}
	cur, _ := json.Marshal(service.CurrentPointer{Tag: tag, Checksum: hexSum([]byte(sums))})
	w := doReq(t, r, "PUT",
		fmt.Sprintf("/api/releases/%s/current.json", channel), cur,
		map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json"})
	if w.Code != http.StatusOK {
		t.Fatalf("seed current.json: status %d body %s", w.Code, w.Body.String())
	}
}

func TestGetReleasesEmpty(t *testing.T) {
	r, _, _ := newTestRouter(t)
	w := doReq(t, r, "GET", "/worker/releases", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got map[string]map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got["channels"]) != 0 {
		t.Fatalf("expected empty channels, got %v", got)
	}
}

func TestGetReleasesEnterpriseFallback(t *testing.T) {
	r, _, token := newTestRouter(t)
	seedRelease(t, r, token, "latest", "v1.0.0")
	w := doReq(t, r, "GET", "/worker/releases", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var resp struct {
		Channels map[string]struct {
			Version  string `json:"version"`
			Checksum string `json:"checksum"`
		} `json:"channels"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Channels["latest"].Version != "v1.0.0" {
		t.Fatalf("latest missing: %+v", resp.Channels)
	}
	if resp.Channels["enterprise-latest"].Version != "v1.0.0" {
		t.Fatalf("enterprise-latest fallback missing: %+v", resp.Channels)
	}
	if _, ok := resp.Channels["next"]; ok {
		t.Fatal("next should be absent")
	}
	if _, ok := resp.Channels["enterprise-next"]; ok {
		t.Fatal("enterprise-next should be absent (no next data)")
	}
}

func TestDownloadEnterpriseFallback(t *testing.T) {
	r, _, token := newTestRouter(t)
	seedRelease(t, r, token, "latest", "v1.0.0")
	w := doReq(t, r, "GET", "/worker/releases/enterprise-latest/download/SHA256SUMS", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "install.sh") {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestDownload503WhenNoReleases(t *testing.T) {
	r, _, _ := newTestRouter(t)
	w := doReq(t, r, "GET", "/worker/releases/latest/download/SHA256SUMS", nil, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", w.Code)
	}
}

func TestDownloadInvalidName(t *testing.T) {
	r, _, token := newTestRouter(t)
	seedRelease(t, r, token, "latest", "v1")
	w := doReq(t, r, "GET", "/worker/releases/latest/download/passwd", nil, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status %d", w.Code)
	}
}

func TestUploadUnauthorized(t *testing.T) {
	r, _, _ := newTestRouter(t)
	w := doReq(t, r, "PUT", "/api/releases/latest/artifacts/v1/install.sh", []byte("x"), nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	w = doReq(t, r, "PUT", "/api/releases/latest/artifacts/v1/install.sh", []byte("x"),
		map[string]string{"Authorization": "Bearer wrong"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong token, got %d", w.Code)
	}
}

func TestUploadDisabledWhenTokenUnset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir, _ := os.MkdirTemp("", "release-disabled-*")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	store := &service.ReleaseStore{Root: dir}
	adminH := &ReleaseAdminHandler{Store: store}
	r := gin.New()
	r.PUT("/api/releases/:channel/artifacts/:tag/:name",
		middleware.UploadTokenAuth(""), adminH.PutArtifact)
	w := doReq(t, r, "PUT", "/api/releases/latest/artifacts/v1/install.sh", []byte("x"),
		map[string]string{"Authorization": "Bearer anything"})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestUpload413(t *testing.T) {
	r, _, token := newTestRouter(t)
	big := make([]byte, (1<<20)+10)
	w := doReq(t, r, "PUT", "/api/releases/latest/artifacts/v1/install.sh",
		big, map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", w.Code)
	}
}

func TestPutCurrentMissingScriptEntry(t *testing.T) {
	r, _, token := newTestRouter(t)
	sh := []byte("#!/bin/sh\n")
	ps1 := []byte("Write-Host\n")
	// Missing install.sh entry.
	sums := fmt.Sprintf("%s  install.ps1\n", hexSum(ps1))
	headers := map[string]string{"Authorization": "Bearer " + token}
	doReq(t, r, "PUT", "/api/releases/latest/artifacts/v1/install.sh", sh, headers)
	doReq(t, r, "PUT", "/api/releases/latest/artifacts/v1/install.ps1", ps1, headers)
	doReq(t, r, "PUT", "/api/releases/latest/artifacts/v1/SHA256SUMS", []byte(sums), headers)

	cur, _ := json.Marshal(service.CurrentPointer{Tag: "v1", Checksum: hexSum([]byte(sums))})
	w := doReq(t, r, "PUT", "/api/releases/latest/current.json", cur,
		map[string]string{"Authorization": "Bearer " + token, "Content-Type": "application/json"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body %s", w.Code, w.Body.String())
	}
}
