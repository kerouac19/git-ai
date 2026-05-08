package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git-ai-server/internal/auth"

	"github.com/gin-gonic/gin"
)

func newDeviceFlowTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

type fakeDeviceCodeEntry struct {
	UserCode  string
	Status    string
	ExpiresAt int64
	Subject   *auth.TokenSubject
}

type fakeDeviceFlowSvc struct {
	entry         *fakeDeviceCodeEntry
	denyCalled    bool
	approveCalled bool
}

func (f *fakeDeviceFlowSvc) GetDeviceCodeByUserCode(ctx context.Context, code string) (*auth.DeviceCodeInfo, error) {
	if f.entry == nil || f.entry.UserCode != code {
		return nil, nil
	}
	return &auth.DeviceCodeInfo{
		UserCode:  f.entry.UserCode,
		Status:    f.entry.Status,
		ExpiresAt: f.entry.ExpiresAt,
		Subject:   f.entry.Subject,
	}, nil
}

func (f *fakeDeviceFlowSvc) ApproveDeviceCode(ctx context.Context, code string) (*auth.DeviceCodeInfo, error) {
	f.approveCalled = true
	if f.entry == nil || f.entry.UserCode != code {
		return nil, nil
	}
	return &auth.DeviceCodeInfo{UserCode: f.entry.UserCode, Status: "approved"}, nil
}

func (f *fakeDeviceFlowSvc) DenyDeviceCode(ctx context.Context, code string) (*auth.DeviceCodeInfo, error) {
	f.denyCalled = true
	if f.entry == nil || f.entry.UserCode != code {
		return nil, nil
	}
	return &auth.DeviceCodeInfo{UserCode: f.entry.UserCode, Status: "denied"}, nil
}

func (f *fakeDeviceFlowSvc) UpdateDeviceCodeSubject(ctx context.Context, code string, s auth.TokenSubject) error {
	return nil
}

func (f *fakeDeviceFlowSvc) DecodeAccessToken(token string) (*auth.Claims, error) {
	// Return nil claims always — these tests do not exercise the authenticated path.
	return nil, nil
}

func TestDeviceFlowHandler_InfoMissingUserCode(t *testing.T) {
	h := &DeviceFlowHandler{Svc: &fakeDeviceFlowSvc{}}
	r := newDeviceFlowTestRouter()
	r.GET("/info", h.Info)

	req := httptest.NewRequest(http.MethodGet, "/info", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestDeviceFlowHandler_InfoNotFound(t *testing.T) {
	h := &DeviceFlowHandler{Svc: &fakeDeviceFlowSvc{}}
	r := newDeviceFlowTestRouter()
	r.GET("/info", h.Info)

	req := httptest.NewRequest(http.MethodGet, "/info?user_code=NOPE", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

func TestDeviceFlowHandler_InfoFound(t *testing.T) {
	svc := &fakeDeviceFlowSvc{
		entry: &fakeDeviceCodeEntry{UserCode: "ABCD", Status: "pending", ExpiresAt: 1234567890},
	}
	h := &DeviceFlowHandler{Svc: svc}
	r := newDeviceFlowTestRouter()
	r.GET("/info", h.Info)

	req := httptest.NewRequest(http.MethodGet, "/info?user_code=ABCD", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if body["user_code"] != "ABCD" || body["status"] != "pending" {
		t.Fatalf("body=%v", body)
	}
	if body["authenticated"] != false {
		t.Fatalf("expected authenticated=false, got %v", body["authenticated"])
	}
}

func TestDeviceFlowHandler_ApproveRequiresLogin(t *testing.T) {
	h := &DeviceFlowHandler{Svc: &fakeDeviceFlowSvc{}}
	r := newDeviceFlowTestRouter()
	r.POST("/approve", h.Approve)

	req := httptest.NewRequest(http.MethodPost, "/approve", strings.NewReader(`{"user_code":"ABCD"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", w.Code)
	}
}

func TestDeviceFlowHandler_DenySuccess(t *testing.T) {
	svc := &fakeDeviceFlowSvc{
		entry: &fakeDeviceCodeEntry{UserCode: "ABCD", Status: "denied"},
	}
	h := &DeviceFlowHandler{Svc: svc}
	r := newDeviceFlowTestRouter()
	r.POST("/deny", h.Deny)

	req := httptest.NewRequest(http.MethodPost, "/deny", strings.NewReader(`{"user_code":"ABCD"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if !svc.denyCalled {
		t.Fatalf("Deny not called")
	}
}
