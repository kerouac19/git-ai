package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"git-ai-server/internal/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

type BundleService struct {
	Pool *pgxpool.Pool
}

func (s *BundleService) CreateBundle(ctx context.Context, userID string, req model.CreateBundleRequest) (*model.BundleRecord, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	if len(req.Data) == 0 || !json.Valid(req.Data) {
		return nil, fmt.Errorf("data must be valid JSON")
	}

	var payload struct {
		Prompts map[string]json.RawMessage `json:"prompts"`
	}
	if err := json.Unmarshal(req.Data, &payload); err != nil {
		return nil, fmt.Errorf("data must be a JSON object")
	}
	if len(payload.Prompts) == 0 {
		return nil, fmt.Errorf("data.prompts must contain at least one prompt")
	}

	var rec model.BundleRecord
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO bundles (id, user_id, title, data, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3::jsonb, now(), now())
		RETURNING id, user_id, title, data::text, created_at, updated_at
	`, userID, req.Title, string(req.Data)).Scan(
		&rec.ID, &rec.UserID, &rec.Title, &rec.Data, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting bundle: %w", err)
	}

	return &rec, nil
}
