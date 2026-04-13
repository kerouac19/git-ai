package model

type MetricsEvent struct {
	T        int            `json:"t"`
	E        int            `json:"e"`
	V        map[string]any `json:"v"`
	A        map[string]any `json:"a"`
	IsObject bool           `json:"-"`
}

type MetricsBatch struct {
	V      int            `json:"v"`
	Events []MetricsEvent `json:"events"`
}

type MetricsSummary struct {
	EventCount7d int     `json:"eventCount7d"`
	RepoCount7d  int     `json:"repoCount7d"`
	LastSyncAt   *string `json:"lastSyncAt"`
}
