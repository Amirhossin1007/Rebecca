//go:build cgo

package masterapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	adminapp "github.com/rebeccapanel/rebecca/go/internal/app/admin"
	userapp "github.com/rebeccapanel/rebecca/go/internal/app/user"
)

func testUserMutationServer(t *testing.T) (*Server, *sql.DB, string) {
	t.Helper()
	server, db := testUserReadServer(t)
	server.userService = userapp.NewService(userapp.NewRepository(db, "sqlite"))
	extra := []string{
		`ALTER TABLE users ADD COLUMN sub_revoked_at DATETIME NULL`,
		`ALTER TABLE users ADD COLUMN edit_at DATETIME NULL`,
		`ALTER TABLE user_usage_logs ADD COLUMN reset_at DATETIME NULL`,
		`ALTER TABLE nodes ADD COLUMN status TEXT DEFAULT 'connected'`,
		`ALTER TABLE admins_services ADD COLUMN updated_at DATETIME NULL`,
		`CREATE TABLE IF NOT EXISTS inbounds (id INTEGER PRIMARY KEY, tag TEXT UNIQUE)`,
		`INSERT INTO xray_config (id, data) VALUES (1, '{"inbounds":[{"tag":"vless-in","protocol":"vless","port":443,"settings":{"decryption":"none"},"streamSettings":{"network":"tcp","security":"none","tcpSettings":{"header":{"type":"none"}}}}]}')`,
		`INSERT INTO hosts (id, inbound_tag, remark, address, is_disabled) VALUES (1, 'vless-in', 'main', 'example.com', 0)`,
		`INSERT INTO inbounds (id, tag) VALUES (1, 'vless-in')`,
		`INSERT INTO nodes (id, name, status) VALUES (1, 'node-1', 'connected')`,
	}
	for _, statement := range extra {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
	insertMasterAPIAdmin(t, db, 1, "owner", "pass123", adminapp.RoleFullAccess, adminapp.StatusActive)
	return server, db, adminBearerToken(t, server, "owner", "pass123")
}

func TestUserMutationCreateUpdateDeleteQueuesOperations(t *testing.T) {
	server, db, token := testUserMutationServer(t)

	rec := userReadRequest(t, server, http.MethodPost, "/api/user", token)
	rec = adminJSONRequest(t, server, http.MethodPost, "/api/user", token, `{"username":"go_user","proxies":{"vless":{}},"inbounds":{"vless":["vless-in"]},"data_limit":1000}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
	}
	assertUserOperationCount(t, db, "add_user", "go_user", 1)

	rec = adminJSONRequest(t, server, http.MethodPost, "/api/user", token, `{"username":"go_user","proxies":{"vless":{}},"inbounds":{"vless":["vless-in"]}}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = adminJSONRequest(t, server, http.MethodPut, "/api/user/go_user", token, `{"status":"disabled","data_limit":2000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d body=%s", rec.Code, rec.Body.String())
	}
	assertUserOperationCount(t, db, "disable_user", "go_user", 1)
	assertDBInt64(t, db, `SELECT data_limit FROM users WHERE username = 'go_user'`, 2000)

	rec = adminJSONRequest(t, server, http.MethodPut, "/api/v2/users/go_user", token, `{"status":"active"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("v2 update status = %d body=%s", rec.Code, rec.Body.String())
	}
	assertUserOperationCount(t, db, "enable_user", "go_user", 1)

	rec = adminJSONRequest(t, server, http.MethodDelete, "/api/user/go_user", token, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", rec.Code, rec.Body.String())
	}
	assertUserOperationCount(t, db, "remove_user", "go_user", 1)
	assertDBString(t, db, `SELECT status FROM users WHERE username = 'go_user'`, "deleted")
}

func TestUserMutationResetRevokeAndActiveNext(t *testing.T) {
	server, db, token := testUserMutationServer(t)
	rec := adminJSONRequest(t, server, http.MethodPost, "/api/user", token, `{"username":"plan_user","proxies":{"vless":{}},"inbounds":{"vless":["vless-in"]},"data_limit":1000}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := db.Exec(`UPDATE users SET used_traffic = 500 WHERE username = 'plan_user'`); err != nil {
		t.Fatal(err)
	}
	rec = adminJSONRequest(t, server, http.MethodPost, "/api/user/plan_user/reset", token, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d body=%s", rec.Code, rec.Body.String())
	}
	assertDBInt64(t, db, `SELECT used_traffic FROM users WHERE username = 'plan_user'`, 0)
	assertUserOperationAtLeast(t, db, "update_user", "plan_user", 1)

	var oldKey string
	if err := db.QueryRow(`SELECT credential_key FROM users WHERE username = 'plan_user'`).Scan(&oldKey); err != nil {
		t.Fatal(err)
	}
	rec = adminJSONRequest(t, server, http.MethodPost, "/api/user/plan_user/revoke_sub", token, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke status = %d body=%s", rec.Code, rec.Body.String())
	}
	var newKey string
	if err := db.QueryRow(`SELECT credential_key FROM users WHERE username = 'plan_user'`).Scan(&newKey); err != nil {
		t.Fatal(err)
	}
	if oldKey == newKey {
		t.Fatalf("expected credential key to rotate")
	}

	userID := assertUserID(t, db, "plan_user")
	if _, err := db.Exec(`INSERT INTO next_plans (user_id, position, data_limit, expire, add_remaining_traffic, fire_on_either, trigger_on) VALUES (?, 0, 4096, NULL, 0, 1, 'either')`, userID); err != nil {
		t.Fatal(err)
	}
	rec = adminJSONRequest(t, server, http.MethodPost, "/api/user/plan_user/active-next", token, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("active-next status = %d body=%s", rec.Code, rec.Body.String())
	}
	assertDBInt64(t, db, `SELECT data_limit FROM users WHERE username = 'plan_user'`, 4096)
	assertDBInt64(t, db, `SELECT COUNT(*) FROM next_plans WHERE user_id = ?`, 0, userID)
}

func TestUserMutationRollsBackWhenNodeOperationFails(t *testing.T) {
	server, db, token := testUserMutationServer(t)
	if _, err := db.Exec(`DROP TABLE node_operations`); err != nil {
		t.Fatal(err)
	}
	rec := adminJSONRequest(t, server, http.MethodPost, "/api/user", token, `{"username":"rollback_user","proxies":{"vless":{}},"inbounds":{"vless":["vless-in"]},"data_limit":1000}`)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected create to fail when operation enqueue fails")
	}
	assertDBInt64(t, db, `SELECT COUNT(*) FROM users WHERE username = 'rollback_user'`, 0)
}

func TestUsersBulkActionsGoNative(t *testing.T) {
	server, db, token := testUserMutationServer(t)
	if _, err := db.Exec(`INSERT INTO services (id, name) VALUES (1, 'basic'), (2, 'premium')`); err != nil {
		t.Fatal(err)
	}
	now := "2026-06-05 00:00:00"
	old := "2020-01-01 00:00:00"
	if _, err := db.Exec(`
INSERT INTO users (id, username, admin_id, status, credential_key, used_traffic, data_limit, expire, created_at, last_status_change, service_id)
VALUES
	(20, 'bulk_a', 1, 'active', 'ka', 100, 1024, 2000000000, ?, ?, 1),
	(21, 'bulk_b', 1, 'active', 'kb', 100, 2048, 2000000000, ?, ?, 1),
	(22, 'bulk_c', 1, 'expired', 'kc', 100, 4096, 1000000000, ?, ?, NULL)`,
		now, now, now, now, now, old,
	); err != nil {
		t.Fatal(err)
	}

	rec := adminJSONRequest(t, server, http.MethodPost, "/api/users/actions", token, `{"action":"increase_traffic","gigabytes":1,"service_id":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("increase bulk status = %d body=%s", rec.Code, rec.Body.String())
	}
	assertDBInt64(t, db, `SELECT data_limit FROM users WHERE username = 'bulk_a'`, 1024+1073741824)
	assertDBInt64(t, db, `SELECT COUNT(*) FROM node_operations WHERE operation_type = 'sync_config'`, 1)

	rec = adminJSONRequest(t, server, http.MethodPost, "/api/v2/services/1/users/actions", token, `{"action":"disable_users"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("service disable bulk status = %d body=%s", rec.Code, rec.Body.String())
	}
	assertDBString(t, db, `SELECT status FROM users WHERE username = 'bulk_a'`, "disabled")
	assertDBString(t, db, `SELECT status FROM users WHERE username = 'bulk_c'`, "expired")

	rec = adminJSONRequest(t, server, http.MethodPost, "/api/users/actions", token, `{"action":"change_service","admin_username":"owner","service_id":1,"target_service_id":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("change service bulk status = %d body=%s", rec.Code, rec.Body.String())
	}
	assertDBInt64(t, db, `SELECT service_id FROM users WHERE username = 'bulk_b'`, 2)

	rec = adminJSONRequest(t, server, http.MethodPost, "/api/users/actions", token, `{"action":"cleanup_status","days":1,"statuses":["expired"],"service_id_is_null":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("cleanup bulk status = %d body=%s", rec.Code, rec.Body.String())
	}
	assertDBString(t, db, `SELECT status FROM users WHERE username = 'bulk_c'`, "deleted")
}

func assertUserOperationCount(t *testing.T, db *sql.DB, op string, username string, want int64) {
	t.Helper()
	assertUserOperationAtLeast(t, db, op, username, want)
	var got int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM node_operations no JOIN users u ON u.id = no.user_id WHERE no.operation_type = ? AND u.username = ?`, op, username).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("operation %s for %s count=%d want=%d", op, username, got, want)
	}
}

func assertUserOperationAtLeast(t *testing.T, db *sql.DB, op string, username string, want int64) {
	t.Helper()
	var got int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM node_operations no JOIN users u ON u.id = no.user_id WHERE no.operation_type = ? AND u.username = ?`, op, username).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got < want {
		t.Fatalf("operation %s for %s count=%d want at least %d", op, username, got, want)
	}
}

func assertUserID(t *testing.T, db *sql.DB, username string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func assertDBInt64(t *testing.T, db *sql.DB, query string, want int64, args ...any) {
	t.Helper()
	var got int64
	if err := db.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("query %q got=%d want=%d", query, got, want)
	}
}

func assertDBString(t *testing.T, db *sql.DB, query string, want string, args ...any) {
	t.Helper()
	var got string
	if err := db.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("query %q got=%q want=%q", query, got, want)
	}
}

func decodeBody(t *testing.T, recBody []byte) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recBody, &body); err != nil {
		t.Fatal(err)
	}
	return body
}
