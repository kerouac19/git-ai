package model

import "time"

type ConfigRecord struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Value       *string   `json:"value"`
	Description *string   `json:"description"`
	Category    string    `json:"category"`
	IsSensitive bool      `json:"is_sensitive"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateConfigDTO struct {
	Key         string `json:"key" binding:"required"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Category    string `json:"category"`
	IsSensitive bool   `json:"is_sensitive"`
}

type UpdateConfigDTO struct {
	Value       *string `json:"value"`
	Description *string `json:"description"`
}
