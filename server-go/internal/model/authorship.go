package model

import "time"

type AuthorshipRecord struct {
	ID                      string    `json:"id"`
	UserID                  string    `json:"userId"`
	GitCommitHash           string    `json:"gitCommitHash"`
	FileAttributions        any       `json:"fileAttributions"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
	AIAttributionPercentage float64   `json:"aiAttributionPercentage"`
}

type CommitAttribution struct {
	ID                    string    `json:"id"`
	CommitHash            string    `json:"commitHash"`
	Author                string    `json:"author"`
	FileChanges           any       `json:"fileChanges"`
	AIContributionMetrics any       `json:"aiContributionMetrics"`
	Timestamp             time.Time `json:"timestamp"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

type AuthorshipRecordDTO struct {
	UserID                  string  `json:"userId" binding:"required"`
	GitCommitHash           string  `json:"gitCommitHash" binding:"required"`
	FileAttributions        any     `json:"fileAttributions" binding:"required"`
	AIAttributionPercentage float64 `json:"aiAttributionPercentage" binding:"required"`
}

type CommitAttributionDTO struct {
	CommitHash            string `json:"commitHash" binding:"required"`
	Author                string `json:"author" binding:"required"`
	FileChanges           any    `json:"fileChanges" binding:"required"`
	AIContributionMetrics any    `json:"aiContributionMetrics" binding:"required"`
}
