package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddNamedMigrationContext("000008_user_soft_delete_username_repair.go", up000008UserSoftDeleteUsernameRepair, emptyDown)
}

func up000008UserSoftDeleteUsernameRepair(ctx context.Context, tx *sql.Tx) error {
	dialect := activeDialect()
	hasUsers, err := HasTable(ctx, tx, dialect, "users")
	if err != nil || !hasUsers {
		return err
	}
	if err := normalizeUserLifecycleStatus(ctx, tx, dialect); err != nil {
		return err
	}
	if err := repairDuplicateUsernames(ctx, tx); err != nil {
		return err
	}
	if err := dropUserUsernameIndexIfPossible(ctx, tx, dialect, "ix_users_username"); err != nil {
		return err
	}
	if err := dropUserUsernameIndexIfPossible(ctx, tx, dialect, "username"); err != nil {
		return err
	}
	return createIndex(ctx, tx, dialect, "users", "ix_users_username", []string{"username"}, true)
}

func dropUserUsernameIndexIfPossible(ctx context.Context, tx *sql.Tx, dialect string, index string) error {
	if _, err := DropIndexIfExists(ctx, tx, dialect, "users", index); err != nil {
		if NormalizeDialect(dialect) == "mysql" && isMySQLForeignKeyIndexError(err) {
			return nil
		}
		return err
	}
	return nil
}

func isMySQLForeignKeyIndexError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "needed in a foreign key constraint")
}

func repairDuplicateUsernames(ctx context.Context, tx *sql.Tx) error {
	for attempt := 0; attempt < 128; attempt++ {
		groups, err := duplicateUsernameGroups(ctx, tx)
		if err != nil {
			return err
		}
		if len(groups) == 0 {
			return nil
		}
		for _, key := range groups {
			rows, err := usernameRowsForGroup(ctx, tx, key)
			if err != nil {
				return err
			}
			if len(rows) <= 1 {
				continue
			}
			representative := rows[0].username
			total := len(rows)
			exactIDs := make([]int64, 0, total)
			for _, row := range rows {
				if row.username == representative {
					exactIDs = append(exactIDs, row.id)
				}
			}
			if len(exactIDs) == total {
				for i, row := range rows {
					if i == len(rows)-1 {
						continue
					}
					newName := fmt.Sprintf("%s_%d_%d", row.username, total, row.id)
					if _, err := tx.ExecContext(ctx, `UPDATE users SET username = ? WHERE id = ?`, newName, row.id); err != nil {
						return err
					}
				}
				continue
			}
			newName := fmt.Sprintf("%s_%d", representative, total)
			if _, err := tx.ExecContext(ctx, `UPDATE users SET username = ? WHERE username = ?`, newName, representative); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("username duplicate repair did not converge")
}

func duplicateUsernameGroups(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT LOWER(username)
FROM users
WHERE username IS NOT NULL AND TRIM(username) != ''
GROUP BY LOWER(username)
HAVING COUNT(*) > 1
ORDER BY LOWER(username)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		groups = append(groups, key)
	}
	return groups, rows.Err()
}

type usernameRepairRow struct {
	id       int64
	username string
}

func usernameRowsForGroup(ctx context.Context, tx *sql.Tx, key string) ([]usernameRepairRow, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, username
FROM users
WHERE username IS NOT NULL AND LOWER(username) = ?
ORDER BY id`, strings.ToLower(key))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []usernameRepairRow
	for rows.Next() {
		var row usernameRepairRow
		if err := rows.Scan(&row.id, &row.username); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
