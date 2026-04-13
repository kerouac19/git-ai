package service

import (
	"context"
	"encoding/json"
	"fmt"

	gitcrypto "git-ai-server/internal/crypto"
	"git-ai-server/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SysConfigService struct {
	Pool      *pgxpool.Pool
	MasterKey []byte
}

func (s *SysConfigService) GetConfig(ctx context.Context, key string) (*model.ConfigRecord, error) {
	var rec model.ConfigRecord
	err := s.Pool.QueryRow(ctx, `
		SELECT id, key, value, description, category, is_sensitive, created_at, updated_at
		FROM config
		WHERE key = $1
	`, key).Scan(&rec.ID, &rec.Key, &rec.Value, &rec.Description, &rec.Category, &rec.IsSensitive, &rec.CreatedAt, &rec.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying config: %w", err)
	}

	if rec.IsSensitive {
		masked := "[REDACTED]"
		rec.Value = &masked
	}

	return &rec, nil
}

func (s *SysConfigService) GetAllConfigs(ctx context.Context, category, key string) ([]model.ConfigRecord, error) {
	query := `SELECT id, key, value, description, category, is_sensitive, created_at, updated_at FROM config WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if category != "" {
		query += fmt.Sprintf(` AND category = $%d`, argIdx)
		args = append(args, category)
		argIdx++
	}
	if key != "" {
		query += fmt.Sprintf(` AND key = $%d`, argIdx)
		args = append(args, key)
		argIdx++
	}

	query += ` ORDER BY category ASC, key ASC`

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying configs: %w", err)
	}
	defer rows.Close()

	var records []model.ConfigRecord
	for rows.Next() {
		var rec model.ConfigRecord
		if err := rows.Scan(&rec.ID, &rec.Key, &rec.Value, &rec.Description, &rec.Category, &rec.IsSensitive, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning config record: %w", err)
		}
		if rec.IsSensitive {
			masked := "[REDACTED]"
			rec.Value = &masked
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating config rows: %w", err)
	}
	if records == nil {
		records = []model.ConfigRecord{}
	}

	return records, nil
}

func (s *SysConfigService) CreateConfig(ctx context.Context, dto model.CreateConfigDTO) (*model.ConfigRecord, error) {
	var exists bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM config WHERE key = $1)`, dto.Key).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("checking existing config: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("configuration key '%s' already exists", dto.Key)
	}

	processedValue := dto.Value
	if dto.IsSensitive {
		encrypted, err := s.encryptConfigValue(dto.Key, dto.Value)
		if err != nil {
			return nil, fmt.Errorf("encrypting sensitive config value: %w", err)
		}
		processedValue = encrypted
	}

	category := dto.Category
	if category == "" {
		category = "general"
	}

	var rec model.ConfigRecord
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO config (id, key, value, description, category, is_sensitive, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, now(), now())
		RETURNING id, key, value, description, category, is_sensitive, created_at, updated_at
	`, dto.Key, processedValue, dto.Description, category, dto.IsSensitive,
	).Scan(&rec.ID, &rec.Key, &rec.Value, &rec.Description, &rec.Category, &rec.IsSensitive, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("inserting config: %w", err)
	}

	if rec.IsSensitive {
		masked := "[REDACTED]"
		rec.Value = &masked
	}

	return &rec, nil
}

func (s *SysConfigService) UpdateConfig(ctx context.Context, key string, dto model.UpdateConfigDTO) (*model.ConfigRecord, error) {
	var existing model.ConfigRecord
	err := s.Pool.QueryRow(ctx, `
		SELECT id, key, value, description, category, is_sensitive, created_at, updated_at
		FROM config
		WHERE key = $1
	`, key).Scan(&existing.ID, &existing.Key, &existing.Value, &existing.Description, &existing.Category, &existing.IsSensitive, &existing.CreatedAt, &existing.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("configuration with key '%s' not found", key)
	}
	if err != nil {
		return nil, fmt.Errorf("querying existing config: %w", err)
	}

	updateQuery := `UPDATE config SET updated_at = now()`
	args := []interface{}{}
	argIdx := 1

	if dto.Description != nil {
		updateQuery += fmt.Sprintf(`, description = $%d`, argIdx)
		args = append(args, *dto.Description)
		argIdx++
	}

	if dto.Value != nil {
		value := *dto.Value
		if existing.IsSensitive {
			encrypted, err := s.encryptConfigValue(key, value)
			if err != nil {
				return nil, fmt.Errorf("encrypting sensitive config value: %w", err)
			}
			value = encrypted
		}
		updateQuery += fmt.Sprintf(`, value = $%d`, argIdx)
		args = append(args, value)
		argIdx++
	}

	updateQuery += fmt.Sprintf(` WHERE key = $%d`, argIdx)
	args = append(args, key)
	argIdx++

	updateQuery += ` RETURNING id, key, value, description, category, is_sensitive, created_at, updated_at`

	var rec model.ConfigRecord
	err = s.Pool.QueryRow(ctx, updateQuery, args...).Scan(&rec.ID, &rec.Key, &rec.Value, &rec.Description, &rec.Category, &rec.IsSensitive, &rec.CreatedAt, &rec.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("updating config: %w", err)
	}

	if rec.IsSensitive {
		masked := "[REDACTED]"
		rec.Value = &masked
	}

	return &rec, nil
}

func (s *SysConfigService) DeleteConfig(ctx context.Context, key string) error {
	result, err := s.Pool.Exec(ctx, `DELETE FROM config WHERE key = $1`, key)
	if err != nil {
		return fmt.Errorf("deleting config: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("configuration with key '%s' not found", key)
	}
	return nil
}

func (s *SysConfigService) encryptConfigValue(key, value string) (string, error) {
	payload, err := gitcrypto.EncryptGeneric(value, s.MasterKey)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshaling encrypted payload: %w", err)
	}
	return string(encoded), nil
}
