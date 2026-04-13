package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditEntry struct {
	UserID    string
	Action    string
	Resource  string
	Params    interface{}
	IP        string
	UserAgent *string
	Success   bool
	Details   *string
}

type AuditService struct {
	Pool *pgxpool.Pool
}

func (s *AuditService) LogRequest(ctx context.Context, entry AuditEntry) error {
	paramsJSON, err := json.Marshal(entry.Params)
	if err != nil {
		return fmt.Errorf("serializing params: %w", err)
	}

	_, err = s.Pool.Exec(ctx, `
		INSERT INTO public.audit_logs (
			user_id,
			action,
			resource,
			params_json,
			ip,
			user_agent,
			occurred_at,
			success,
			details
		) VALUES ($1, $2, $3, $4::jsonb, $5, $6, now(), $7, $8)
	`, entry.UserID, entry.Action, entry.Resource, string(paramsJSON), entry.IP, entry.UserAgent, entry.Success, entry.Details)
	if err != nil {
		return fmt.Errorf("inserting audit log: %w", err)
	}

	return nil
}
