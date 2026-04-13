package model

import "time"

type AuditLog struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Action     string    `json:"action"`
	Resource   string    `json:"resource"`
	ParamsJSON any       `json:"params_json"`
	IP         string    `json:"ip"`
	UserAgent  *string   `json:"user_agent"`
	OccurredAt time.Time `json:"occurred_at"`
	Success    bool      `json:"success"`
	Details    *string   `json:"details"`
	CreatedAt  time.Time `json:"created_at"`
}
