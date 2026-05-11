package service

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUploadObjectsPersistsMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: requires running PostgreSQL")
	}

	pool := openCasTestPool(t)

	svc := &CasService{Pool: pool, CASKey: "test-key-32-bytes-test-key-32byt"}

	objects := []CasUploadRequest{{
		Hash:    "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		Content: map[string]string{"hello": "world"},
		Metadata: map[string]string{
			"source": "transcript",
			"tag":    "smoke",
		},
	}}

	result, err := svc.UploadObjects(t.Context(), objects)
	if err != nil {
		t.Fatalf("UploadObjects: %v", err)
	}
	if result.FailureCount != 0 {
		t.Fatalf("FailureCount = %d, want 0; results=%+v", result.FailureCount, result.Results)
	}

	var metaJSON []byte
	if err := pool.QueryRow(t.Context(),
		`SELECT metadata FROM cas_entries WHERE hash = $1`, objects[0].Hash,
	).Scan(&metaJSON); err != nil {
		if err == pgx.ErrNoRows {
			t.Fatalf("entry not inserted")
		}
		t.Fatalf("query metadata: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(metaJSON, &got); err != nil {
		t.Fatalf("decode metadata: %v (raw=%s)", err, string(metaJSON))
	}
	if got["source"] != "transcript" || got["tag"] != "smoke" {
		t.Fatalf("metadata = %+v, want source=transcript tag=smoke", got)
	}
}

func openCasTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := openTestPool(t)
	if _, err := pool.Exec(t.Context(), `TRUNCATE TABLE cas_entries`); err != nil {
		t.Fatalf("truncate cas_entries: %v", err)
	}
	return pool
}
