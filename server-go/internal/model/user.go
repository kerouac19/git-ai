package model

import "time"

const (
	UserStatusEnabled  = 1
	UserStatusDisabled = 2
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email,omitempty"`
	DisplayName  string    `json:"display_name,omitempty"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	Status       int       `json:"status"`
	OrgID        string    `json:"org_id"`
	OrgName      string    `json:"org_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
