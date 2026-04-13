package service

import (
	"context"
	"encoding/json"
	"fmt"

	"git-ai-server/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthorshipService struct {
	Pool *pgxpool.Pool
}

func (s *AuthorshipService) SaveRecord(ctx context.Context, dto model.AuthorshipRecordDTO) (*model.AuthorshipRecord, error) {
	fileAttrsJSON, err := json.Marshal(dto.FileAttributions)
	if err != nil {
		return nil, fmt.Errorf("serializing file attributions: %w", err)
	}

	var rec model.AuthorshipRecord
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO authorship_records
			(id, user_id, git_commit_hash, file_attributions, created_at, updated_at, ai_attribution_percentage)
		VALUES (gen_random_uuid(), $1, $2, $3, now(), now(), $4)
		RETURNING id, user_id, git_commit_hash, file_attributions, created_at, updated_at, ai_attribution_percentage
	`, dto.UserID, dto.GitCommitHash, string(fileAttrsJSON), dto.AIAttributionPercentage,
	).Scan(&rec.ID, &rec.UserID, &rec.GitCommitHash, &rec.FileAttributions, &rec.CreatedAt, &rec.UpdatedAt, &rec.AIAttributionPercentage)
	if err != nil {
		return nil, fmt.Errorf("inserting authorship record: %w", err)
	}

	rec.FileAttributions = parseJSONField(rec.FileAttributions, []any{})
	return &rec, nil
}

func (s *AuthorshipService) SaveCommitAttribution(ctx context.Context, dto model.CommitAttributionDTO) (*model.CommitAttribution, error) {
	fileChangesJSON, err := json.Marshal(dto.FileChanges)
	if err != nil {
		return nil, fmt.Errorf("serializing file changes: %w", err)
	}

	metricsJSON, err := json.Marshal(dto.AIContributionMetrics)
	if err != nil {
		return nil, fmt.Errorf("serializing ai contribution metrics: %w", err)
	}

	var rec model.CommitAttribution
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO commit_attributions
			(id, commit_hash, author, file_changes, ai_contribution_metrics, timestamp, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, now(), now())
		RETURNING id, commit_hash, author, file_changes, ai_contribution_metrics, timestamp, updated_at
	`, dto.CommitHash, dto.Author, string(fileChangesJSON), string(metricsJSON),
	).Scan(&rec.ID, &rec.CommitHash, &rec.Author, &rec.FileChanges, &rec.AIContributionMetrics, &rec.Timestamp, &rec.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("inserting commit attribution: %w", err)
	}

	rec.FileChanges = parseJSONField(rec.FileChanges, map[string]any{})
	rec.AIContributionMetrics = parseJSONField(rec.AIContributionMetrics, map[string]any{
		"aiLineCount":    0,
		"totalLineCount": 0,
		"aiPercentage":   0,
		"tokensUsed":     0,
	})
	return &rec, nil
}

func (s *AuthorshipService) FindByUserID(ctx context.Context, userID string) ([]model.AuthorshipRecord, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, user_id, git_commit_hash, file_attributions, created_at, updated_at, ai_attribution_percentage
		FROM authorship_records
		WHERE user_id LIKE '%' || $1 || '%'
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying authorship records: %w", err)
	}
	defer rows.Close()

	return scanAuthorshipRecords(rows)
}

func (s *AuthorshipService) FindByCommitHash(ctx context.Context, commitHash string) ([]model.AuthorshipRecord, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, user_id, git_commit_hash, file_attributions, created_at, updated_at, ai_attribution_percentage
		FROM authorship_records
		WHERE git_commit_hash LIKE '%' || $1 || '%'
		ORDER BY created_at DESC
	`, commitHash)
	if err != nil {
		return nil, fmt.Errorf("querying authorship records: %w", err)
	}
	defer rows.Close()

	return scanAuthorshipRecords(rows)
}

func (s *AuthorshipService) FindCommitAttributionByHash(ctx context.Context, commitHash string) (*model.CommitAttribution, error) {
	var rec model.CommitAttribution
	err := s.Pool.QueryRow(ctx, `
		SELECT id, commit_hash, author, file_changes, ai_contribution_metrics, timestamp, updated_at
		FROM commit_attributions
		WHERE commit_hash = $1
		LIMIT 1
	`, commitHash).Scan(&rec.ID, &rec.CommitHash, &rec.Author, &rec.FileChanges, &rec.AIContributionMetrics, &rec.Timestamp, &rec.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying commit attribution: %w", err)
	}

	rec.FileChanges = parseJSONField(rec.FileChanges, map[string]any{})
	rec.AIContributionMetrics = parseJSONField(rec.AIContributionMetrics, map[string]any{
		"aiLineCount":    0,
		"totalLineCount": 0,
		"aiPercentage":   0,
		"tokensUsed":     0,
	})
	return &rec, nil
}

func (s *AuthorshipService) FindAll(ctx context.Context, userID string, limit, offset int) ([]model.AuthorshipRecord, int, error) {
	var total int
	err := s.Pool.QueryRow(ctx, `
		SELECT count(*) FROM authorship_records WHERE user_id = $1
	`, userID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting authorship records: %w", err)
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT id, user_id, git_commit_hash, file_attributions, created_at, updated_at, ai_attribution_percentage
		FROM authorship_records
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying authorship records: %w", err)
	}
	defer rows.Close()

	records, err := scanAuthorshipRecords(rows)
	if err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

func (s *AuthorshipService) MergeAuthorshipData(ctx context.Context, userID string, dto model.AuthorshipRecordDTO) (*model.AuthorshipRecord, error) {
	fileAttrsJSON, err := json.Marshal(dto.FileAttributions)
	if err != nil {
		return nil, fmt.Errorf("serializing file attributions: %w", err)
	}

	var existingID string
	err = s.Pool.QueryRow(ctx, `
		SELECT id FROM authorship_records
		WHERE user_id = $1 AND git_commit_hash = $2
		LIMIT 1
	`, userID, dto.GitCommitHash).Scan(&existingID)

	if err == pgx.ErrNoRows {
		dto.UserID = userID
		return s.SaveRecord(ctx, dto)
	}
	if err != nil {
		return nil, fmt.Errorf("querying existing record: %w", err)
	}

	var rec model.AuthorshipRecord
	err = s.Pool.QueryRow(ctx, `
		UPDATE authorship_records
		SET file_attributions = $1, ai_attribution_percentage = $2, updated_at = now()
		WHERE id = $3
		RETURNING id, user_id, git_commit_hash, file_attributions, created_at, updated_at, ai_attribution_percentage
	`, string(fileAttrsJSON), dto.AIAttributionPercentage, existingID,
	).Scan(&rec.ID, &rec.UserID, &rec.GitCommitHash, &rec.FileAttributions, &rec.CreatedAt, &rec.UpdatedAt, &rec.AIAttributionPercentage)
	if err != nil {
		return nil, fmt.Errorf("updating authorship record: %w", err)
	}

	rec.FileAttributions = parseJSONField(rec.FileAttributions, []any{})
	return &rec, nil
}

func scanAuthorshipRecords(rows pgx.Rows) ([]model.AuthorshipRecord, error) {
	var records []model.AuthorshipRecord
	for rows.Next() {
		var rec model.AuthorshipRecord
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.GitCommitHash, &rec.FileAttributions, &rec.CreatedAt, &rec.UpdatedAt, &rec.AIAttributionPercentage); err != nil {
			return nil, fmt.Errorf("scanning authorship record: %w", err)
		}
		rec.FileAttributions = parseJSONField(rec.FileAttributions, []any{})
		records = append(records, rec)
	}
	if records == nil {
		records = []model.AuthorshipRecord{}
	}
	return records, rows.Err()
}

func parseJSONField(value any, fallback any) any {
	switch v := value.(type) {
	case string:
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			return fallback
		}
		return parsed
	default:
		if v == nil {
			return fallback
		}
		return v
	}
}
