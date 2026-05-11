package service

import (
	"os"
	"testing"

	"git-ai-server/internal/database"
	"git-ai-server/internal/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidateBatchShapeAcceptsCurrentMetricsSchema(t *testing.T) {
	svc := &MetricsService{}

	batch, err := svc.ValidateBatchShape(map[string]any{
		"v": float64(1),
		"events": []any{
			map[string]any{
				"t": float64(1712000000),
				"e": float64(1),
				"v": map[string]any{
					"2":  float64(100),
					"4":  []any{float64(10)},
					"5":  []any{float64(42)},
					"7":  []any{float64(60)},
					"10": float64(1711999000),
					"11": "feat: smoke",
					"12": "body",
				},
				"a": map[string]any{
					"0":  "1.2.8",
					"1":  "https://github.com/test/repo",
					"2":  "dev@example.com",
					"3":  "abc123",
					"4":  "base456",
					"5":  "main",
					"20": "claude-code",
					"21": "gpt-5.4",
					"22": "prompt-123",
					"23": "external-session-123",
					"30": "{\"workspace\":\"smoke\"}",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ValidateBatchShape() error = %v", err)
	}

	if batch.V != 1 {
		t.Fatalf("ValidateBatchShape() version = %d, want 1", batch.V)
	}

	if len(batch.Events) != 1 {
		t.Fatalf("ValidateBatchShape() events = %d, want 1", len(batch.Events))
	}

	if errMsg := validateEvent(batch.Events[0]); errMsg != "" {
		t.Fatalf("validateEvent() error = %q, want empty", errMsg)
	}
}

func TestValidateBatchShapeRejectsNonIntegerSchemaVersion(t *testing.T) {
	svc := &MetricsService{}

	_, err := svc.ValidateBatchShape(map[string]any{
		"v":      1.5,
		"events": []any{},
	})
	if err == nil {
		t.Fatal("ValidateBatchShape() error = nil, want non-nil")
	}
}

func TestValidateEventRejectsNonObjectEvent(t *testing.T) {
	svc := &MetricsService{}

	batch, err := svc.ValidateBatchShape(map[string]any{
		"v":      float64(1),
		"events": []any{"not-an-object"},
	})
	if err != nil {
		t.Fatalf("ValidateBatchShape() error = %v", err)
	}

	if len(batch.Events) != 1 {
		t.Fatalf("ValidateBatchShape() events = %d, want 1", len(batch.Events))
	}

	if errMsg := validateEvent(batch.Events[0]); errMsg != "event must be an object" {
		t.Fatalf("validateEvent() error = %q, want %q", errMsg, "event must be an object")
	}
}

func TestUploadBatchReturnsEmptyErrorsSlice(t *testing.T) {
	svc := &MetricsService{}

	errors, err := svc.UploadBatch(t.Context(), "user-1", nil, &model.MetricsBatch{
		V:      1,
		Events: nil,
	})
	if err != nil {
		t.Fatalf("UploadBatch() error = %v", err)
	}
	if errors == nil {
		t.Fatal("UploadBatch() errors = nil, want empty slice")
	}
	if len(errors) != 0 {
		t.Fatalf("UploadBatch() errors length = %d, want 0", len(errors))
	}
}

func TestValidateEventAcceptsSessionEvent(t *testing.T) {
	svc := &MetricsService{}

	batch, err := svc.ValidateBatchShape(map[string]any{
		"v": float64(1),
		"events": []any{
			map[string]any{
				"t": float64(1712000000),
				"e": float64(5), // SessionEvent
				"v": map[string]any{
					"0": "session-started",
				},
				"a": map[string]any{
					"0":  "1.4.7",
					"1":  "https://github.com/test/repo",
					"23": "external-session-abc",
					"24": "session-123",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ValidateBatchShape() error = %v", err)
	}

	if len(batch.Events) != 1 {
		t.Fatalf("ValidateBatchShape() events = %d, want 1", len(batch.Events))
	}

	if errMsg := validateEvent(batch.Events[0]); errMsg != "" {
		t.Fatalf("validateEvent(SessionEvent) error = %q, want empty (event_id=5 should be supported)", errMsg)
	}
}

func TestUploadBatchPopulatesAllAttrColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: requires running PostgreSQL")
	}

	pool := openTestPool(t)

	svc := &MetricsService{Pool: pool}

	batch := &model.MetricsBatch{
		V: 1,
		Events: []model.MetricsEvent{
			{
				IsObject: true,
				T:        1715000000,
				E:        5,
				V:        map[string]any{"0": "session-started"},
				A: map[string]any{
					"0":  "1.4.7",
					"1":  "https://github.com/test/repo",
					"2":  "dev@example.com",
					"3":  "abc123",
					"4":  "base456",
					"5":  "feature-branch",
					"20": "claude-code",
					"21": "gpt-5.4",
					"23": "ext-session-xyz",
					"24": "session-xyz",
					"25": "trace-xyz",
					"26": "parent-session-xyz",
					"27": "ext-parent-session-xyz",
					"30": `{"workspace":"smoke"}`,
				},
			},
		},
	}

	errs, err := svc.UploadBatch(t.Context(), "00000000-0000-0000-0000-000000000001", nil, batch)
	if err != nil {
		t.Fatalf("UploadBatch() error = %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("UploadBatch() errors = %+v, want empty", errs)
	}

	row := pool.QueryRow(t.Context(), `
		SELECT event_id, git_ai_version, repo_url,
		       author, commit_sha, base_commit_sha, branch,
		       tool, model, external_session_id,
		       session_id, trace_id, parent_session_id,
		       external_parent_session_id, custom_attributes
		  FROM metrics_events
		 ORDER BY received_at DESC
		 LIMIT 1`)

	var eventID int
	var got [14]*string
	if err := row.Scan(&eventID,
		&got[0], &got[1], &got[2], &got[3], &got[4], &got[5],
		&got[6], &got[7], &got[8], &got[9], &got[10], &got[11],
		&got[12], &got[13],
	); err != nil {
		t.Fatalf("scanning inserted row: %v", err)
	}

	wantEventID := 5
	if eventID != wantEventID {
		t.Fatalf("event_id = %d, want %d", eventID, wantEventID)
	}

	want := []string{
		"1.4.7", "https://github.com/test/repo",
		"dev@example.com", "abc123", "base456", "feature-branch",
		"claude-code", "gpt-5.4", "ext-session-xyz",
		"session-xyz", "trace-xyz", "parent-session-xyz",
		"ext-parent-session-xyz", `{"workspace":"smoke"}`,
	}
	for i, w := range want {
		if got[i] == nil {
			t.Fatalf("column %d is NULL, want %q", i, w)
		}
		if *got[i] != w {
			t.Fatalf("column %d = %q, want %q", i, *got[i], w)
		}
	}
}

func TestUploadBatchLeavesMissingAttrsAsNull(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: requires running PostgreSQL")
	}

	pool := openTestPool(t)

	svc := &MetricsService{Pool: pool}

	// Only mandatory attr (git_ai_version) is set; all others omitted.
	batch := &model.MetricsBatch{
		V: 1,
		Events: []model.MetricsEvent{
			{
				IsObject: true,
				T:        1715000000,
				E:        5,
				V:        map[string]any{"0": "session-started"},
				A:        map[string]any{"0": "1.4.7"},
			},
		},
	}

	errs, err := svc.UploadBatch(t.Context(), "00000000-0000-0000-0000-000000000001", nil, batch)
	if err != nil {
		t.Fatalf("UploadBatch() error = %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("UploadBatch() errors = %+v, want empty", errs)
	}

	row := pool.QueryRow(t.Context(), `
		SELECT git_ai_version,
		       author, commit_sha, base_commit_sha, branch,
		       tool, model, external_session_id,
		       session_id, trace_id, parent_session_id,
		       external_parent_session_id, custom_attributes
		  FROM metrics_events
		 ORDER BY received_at DESC
		 LIMIT 1`)

	var version *string
	nullables := make([]*string, 12)
	scanArgs := []any{&version}
	for i := range nullables {
		scanArgs = append(scanArgs, &nullables[i])
	}
	if err := row.Scan(scanArgs...); err != nil {
		t.Fatalf("scanning inserted row: %v", err)
	}

	if version == nil || *version != "1.4.7" {
		t.Fatalf("git_ai_version = %v, want \"1.4.7\"", version)
	}

	names := []string{
		"author", "commit_sha", "base_commit_sha", "branch",
		"tool", "model", "external_session_id",
		"session_id", "trace_id", "parent_session_id",
		"external_parent_session_id", "custom_attributes",
	}
	for i, p := range nullables {
		if p != nil {
			t.Errorf("%s = %q, want NULL", names[i], *p)
		}
	}
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("METRICS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("METRICS_TEST_DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := database.RunMigrations(dsn); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	if _, err := pool.Exec(t.Context(), `TRUNCATE TABLE metrics_events`); err != nil {
		t.Fatalf("truncate metrics_events: %v", err)
	}
	return pool
}
