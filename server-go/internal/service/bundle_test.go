package service

import (
	"encoding/json"
	"strings"
	"testing"

	"git-ai-server/internal/model"
)

func TestCreateBundleRequiresUserID(t *testing.T) {
	svc := &BundleService{}

	_, err := svc.CreateBundle(t.Context(), "  ", model.CreateBundleRequest{
		Title: "Prompt bundle",
		Data:  json.RawMessage(`{"prompts":{"prompt-1":{}}}`),
	})
	if err == nil {
		t.Fatal("CreateBundle() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "user_id is required") {
		t.Fatalf("CreateBundle() error = %q, want user_id validation", err)
	}
}

func TestCreateBundlePersistsUserID(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: requires running PostgreSQL")
	}

	pool := openTestPool(t)
	if _, err := pool.Exec(t.Context(), `TRUNCATE TABLE bundles`); err != nil {
		t.Fatalf("truncate bundles: %v", err)
	}

	svc := &BundleService{Pool: pool}

	rec, err := svc.CreateBundle(t.Context(), "user-123", model.CreateBundleRequest{
		Title: "Prompt bundle",
		Data:  json.RawMessage(`{"prompts":{"prompt-1":{"messages":[]}}}`),
	})
	if err != nil {
		t.Fatalf("CreateBundle() error = %v", err)
	}
	if rec.UserID != "user-123" {
		t.Fatalf("record user_id = %q, want %q", rec.UserID, "user-123")
	}
	if rec.UpdatedAt.IsZero() {
		t.Fatal("record updated_at is zero")
	}

	var got string
	if err := pool.QueryRow(t.Context(),
		`SELECT user_id FROM bundles WHERE id = $1`, rec.ID,
	).Scan(&got); err != nil {
		t.Fatalf("query inserted bundle: %v", err)
	}
	if got != "user-123" {
		t.Fatalf("stored user_id = %q, want %q", got, "user-123")
	}
}
