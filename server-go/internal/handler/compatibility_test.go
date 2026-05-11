package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUploadWorkerCasRejectsTooManyObjects(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Build a body with 101 objects; do not need a real service — we expect
	// the limit check to short-circuit before service dispatch.
	objs := make([]map[string]any, 101)
	for i := range objs {
		objs[i] = map[string]any{"hash": fmt.Sprintf("%064x", i), "content": "x"}
	}
	body, err := json.Marshal(map[string]any{"objects": objs})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	r := gin.New()
	h := &CompatibilityHandler{} // service-less; limit check must precede service dispatch
	r.POST("/worker/cas/upload", h.UploadWorkerCas)

	req := httptest.NewRequest(http.MethodPost, "/worker/cas/upload", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "maximum of 100") {
		t.Fatalf("body missing 'maximum of 100': %s", rec.Body.String())
	}
}
