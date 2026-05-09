package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"git-ai-server/internal/model"

	"github.com/gin-gonic/gin"
)

type fakeAdminDashSvc struct {
	data   *model.AdminDashboardData
	err    error
	gotCtx context.Context
	gotKey string
	calls  int
}

func (f *fakeAdminDashSvc) GetGlobalStats(ctx context.Context, rangeKey string) (*model.AdminDashboardData, error) {
	f.calls++
	f.gotCtx = ctx
	f.gotKey = rangeKey
	return f.data, f.err
}

func newAdminDashTestRouter(svc AdminDashboardSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &AdminDashboardHandler{Svc: svc}
	r.GET("/api/dashboard/global", h.GetGlobalStats)
	return r
}

func TestAdminDashboard_DefaultRange(t *testing.T) {
	fake := &fakeAdminDashSvc{
		data: &model.AdminDashboardData{Range: "7d"},
	}
	r := newAdminDashTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/global", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if fake.gotKey != "7d" {
		t.Errorf("service got range %q, want 7d", fake.gotKey)
	}
}

func TestAdminDashboard_ExplicitRange30d(t *testing.T) {
	fake := &fakeAdminDashSvc{data: &model.AdminDashboardData{Range: "30d"}}
	r := newAdminDashTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/global?range=30d", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if fake.gotKey != "30d" {
		t.Errorf("service got range %q, want 30d", fake.gotKey)
	}
}

func TestAdminDashboard_InvalidRange(t *testing.T) {
	fake := &fakeAdminDashSvc{}
	r := newAdminDashTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/global?range=42d", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if fake.calls != 0 {
		t.Errorf("service called %d times on bad range; want 0", fake.calls)
	}
}

func TestAdminDashboard_ServiceError(t *testing.T) {
	fake := &fakeAdminDashSvc{err: errors.New("db blew up")}
	r := newAdminDashTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/global", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestAdminDashboard_ResponseShape(t *testing.T) {
	fake := &fakeAdminDashSvc{
		data: &model.AdminDashboardData{
			Range: "7d",
			Summary: model.AdminDashboardSummary{
				TotalPrompts:     5,
				AICodePercentage: 12.5,
			},
			Trend:             []model.AdminTrendPoint{{Date: "2026-05-01", ActiveUsers: 1}},
			TopUsers:          []model.AdminTopUser{},
			TopOrgs:           []model.AdminTopOrg{},
			AgentDistribution: []model.AdminDistributionRow{},
			ModelDistribution: []model.AdminDistributionRow{},
		},
	}
	r := newAdminDashTestRouter(fake)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/global", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var payload struct {
		Success bool                     `json:"success"`
		Data    model.AdminDashboardData `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, w.Body.String())
	}
	if !payload.Success {
		t.Error("success should be true")
	}
	if payload.Data.Range != "7d" {
		t.Errorf("range = %q, want 7d", payload.Data.Range)
	}
	if payload.Data.Summary.TotalPrompts != 5 {
		t.Errorf("totalPrompts = %d, want 5", payload.Data.Summary.TotalPrompts)
	}
}
