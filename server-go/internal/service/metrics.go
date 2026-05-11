package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"git-ai-server/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxBatchSize           = 250
	supportedSchemaVersion = 1
)

var supportedEventIDs = map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true}

type MetricsUploadError struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

type MetricsService struct {
	Pool *pgxpool.Pool
}

func (s *MetricsService) ValidateBatchShape(body map[string]any) (*model.MetricsBatch, error) {
	vRaw, ok := body["v"]
	if !ok {
		return nil, fmt.Errorf("unsupported metrics schema version: <nil>")
	}
	v, okV := toInt(vRaw)
	if !okV || v != supportedSchemaVersion {
		return nil, fmt.Errorf("Unsupported metrics schema version: %v", vRaw)
	}

	eventsRaw, ok := body["events"]
	if !ok {
		return nil, fmt.Errorf("events must be an array")
	}
	eventsSlice, ok := eventsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("events must be an array")
	}

	if len(eventsSlice) > maxBatchSize {
		return nil, fmt.Errorf("events must contain at most %d items", maxBatchSize)
	}

	events := make([]model.MetricsEvent, 0, len(eventsSlice))
	for _, raw := range eventsSlice {
		m, ok := raw.(map[string]any)
		if !ok {
			events = append(events, model.MetricsEvent{})
			continue
		}
		var evt model.MetricsEvent
		evt.IsObject = true
		if t, ok := toInt(m["t"]); ok {
			evt.T = t
		}
		if e, ok := toInt(m["e"]); ok {
			evt.E = e
		}
		if vv, ok := m["v"].(map[string]any); ok {
			evt.V = vv
		}
		if aa, ok := m["a"].(map[string]any); ok {
			evt.A = aa
		}
		events = append(events, evt)
	}

	return &model.MetricsBatch{
		V:      v,
		Events: events,
	}, nil
}

func (s *MetricsService) UploadBatch(ctx context.Context, userID string, distinctID *string, batch *model.MetricsBatch) ([]MetricsUploadError, error) {
	errors := make([]MetricsUploadError, 0)
	var validRows [][]any

	for i, event := range batch.Events {
		if errMsg := validateEvent(event); errMsg != "" {
			errors = append(errors, MetricsUploadError{Index: i, Error: errMsg})
			continue
		}

		valuesJSON, _ := json.Marshal(event.V)
		attrsJSON, _ := json.Marshal(event.A)

		var did *string
		if distinctID != nil && *distinctID != "" {
			did = distinctID
		}

		eventTimestamp := time.Unix(int64(event.T), 0).UTC()

		validRows = append(validRows, []any{
			userID,
			did,
			batch.V,
			eventTimestamp,
			event.E,
			valuesJSON,
			attrsJSON,
			asNullableString(event.A["0"]),
			asNullableString(event.A["1"]),
			asNullableString(event.A["20"]),
			asNullableString(event.A["21"]),
			asNullableString(event.A["22"]),
			asNullableString(event.A["23"]),
		})
	}

	if len(validRows) > 0 {
		_, err := s.Pool.CopyFrom(
			ctx,
			pgx.Identifier{"public", "metrics_events"},
			[]string{
				"user_id", "distinct_id", "schema_version",
				"event_timestamp", "event_id",
				"values_json", "attrs_json",
				"git_ai_version", "repo_url",
				"tool", "model", "prompt_id", "external_prompt_id",
			},
			pgx.CopyFromRows(validRows),
		)
		if err != nil {
			return errors, fmt.Errorf("batch insert: %w", err)
		}
	}

	return errors, nil
}

func (s *MetricsService) GetUserMetricsSummary(ctx context.Context, userID string) (*model.MetricsSummary, error) {
	var eventCount7d, repoCount7d int
	var lastSyncAt *time.Time

	err := s.Pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (
				WHERE event_timestamp >= now() - interval '7 days'
			),
			count(DISTINCT repo_url) FILTER (
				WHERE event_timestamp >= now() - interval '7 days'
					AND repo_url IS NOT NULL
					AND repo_url <> ''
			),
			max(received_at)
		FROM public.metrics_events
		WHERE user_id = $1
	`, userID).Scan(&eventCount7d, &repoCount7d, &lastSyncAt)
	if err != nil {
		return nil, fmt.Errorf("querying metrics summary: %w", err)
	}

	var lastSync *string
	if lastSyncAt != nil {
		s := lastSyncAt.UTC().Format(time.RFC3339)
		lastSync = &s
	}

	return &model.MetricsSummary{
		EventCount7d: eventCount7d,
		RepoCount7d:  repoCount7d,
		LastSyncAt:   lastSync,
	}, nil
}

func validateEvent(event model.MetricsEvent) string {
	if !event.IsObject {
		return "event must be an object"
	}
	if event.T <= 0 {
		return "t must be a positive unix timestamp"
	}
	if !supportedEventIDs[event.E] {
		return "e must be a supported event id"
	}
	if event.V == nil {
		return "v must be an object"
	}
	if event.A == nil {
		return "a must be an object"
	}
	version, ok := event.A["0"]
	if !ok {
		return "a.0 (git_ai_version) is required"
	}
	s, ok := version.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "a.0 (git_ai_version) is required"
	}
	return ""
}

func asNullableString(value any) *string {
	s, ok := value.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		if math.Trunc(n) != n {
			return 0, false
		}
		return int(n), true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}
