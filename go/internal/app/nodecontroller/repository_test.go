//go:build cgo

package nodecontroller

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestRepositoryProcessesOperationState(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "queue.db")+"?_busy_timeout=30000")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, `
CREATE TABLE node_operations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	operation_type TEXT NOT NULL,
	node_id INTEGER NULL,
	user_id INTEGER NULL,
	payload TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	attempts INTEGER NOT NULL DEFAULT 0,
	last_error TEXT NULL,
	idempotency_key TEXT NOT NULL UNIQUE,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO node_operations (operation_type, node_id, user_id, payload, status, idempotency_key, created_at, updated_at)
VALUES ('sync_config', 7, 42, '{"config_json":"{}"}', 'pending', 'op-1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatal(err)
	}

	repo := NewRepository(db, "sqlite")
	rows, err := repo.PendingOperations(ctx, 7, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one pending operation, got %d", len(rows))
	}

	claimed, err := repo.MarkOperationRunning(ctx, rows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("expected operation to be claimed")
	}

	if err := repo.MarkOperationRetrying(ctx, rows[0].ID, "node down"); err != nil {
		t.Fatal(err)
	}
	rows, err = repo.PendingOperations(ctx, 7, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Attempts != 1 {
		t.Fatalf("expected retrying operation with one attempt, got %#v", rows)
	}

	claimed, err = repo.MarkOperationRunning(ctx, rows[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("expected retrying operation to be claimed")
	}
	if err := repo.MarkOperationDone(ctx, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	rows, err = repo.PendingOperations(ctx, 7, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no pending operations after done, got %d", len(rows))
	}
}
