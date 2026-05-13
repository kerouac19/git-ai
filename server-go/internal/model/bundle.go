package model

import (
	"encoding/json"
	"time"
)

type CreateBundleRequest struct {
	Title string          `json:"title" binding:"required"`
	Data  json.RawMessage `json:"data" binding:"required"`
}

type BundleRecord struct {
	ID        string          `json:"id"`
	UserID    string          `json:"user_id"`
	Title     string          `json:"title"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type CreateBundleResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
	URL     string `json:"url"`
}
