package service

import (
	"testing"

	"git-ai-server/internal/model"
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
					"23": "external-prompt-123",
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
