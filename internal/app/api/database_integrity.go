package api

import (
	"context"
	"database/sql"
	"fmt"
)

func checkDatabaseIntegrity(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}

	nodeCount, err := countRows(ctx, db, `SELECT COUNT(*) FROM nodes`)
	if err != nil {
		return err
	}
	operationCount, err := countRows(ctx, db, `SELECT COUNT(*) FROM node_operations`)
	if err != nil {
		return err
	}
	if nodeCount == 0 && operationCount > 0 {
		return fmt.Errorf("node operation queue has %d rows but the nodes table is empty; restore the correct database or recover missing node rows before startup", operationCount)
	}

	orphanNodeUsages, err := countRows(ctx, db, `SELECT COUNT(*) FROM node_usages nu LEFT JOIN nodes n ON n.id = nu.node_id WHERE nu.node_id IS NOT NULL AND n.id IS NULL`)
	if err != nil {
		return err
	}
	orphanUserUsages, err := countRows(ctx, db, `SELECT COUNT(*) FROM node_user_usages nu LEFT JOIN nodes n ON n.id = nu.node_id WHERE nu.node_id IS NOT NULL AND n.id IS NULL`)
	if err != nil {
		return err
	}
	orphanOperations, err := countRows(ctx, db, `SELECT COUNT(*) FROM node_operations no LEFT JOIN nodes n ON n.id = no.node_id WHERE no.node_id IS NOT NULL AND n.id IS NULL`)
	if err != nil {
		return err
	}
	if orphanNodeUsages > 0 || orphanUserUsages > 0 || orphanOperations > 0 {
		return fmt.Errorf("node references are inconsistent: node_usages=%d node_user_usages=%d node_operations=%d point to missing nodes", orphanNodeUsages, orphanUserUsages, orphanOperations)
	}

	checks := []struct {
		name string
		sql  string
	}{
		{"users.service_id", `SELECT COUNT(*) FROM users u LEFT JOIN services s ON s.id = u.service_id WHERE u.service_id IS NOT NULL AND s.id IS NULL`},
		{"service_hosts.host_id", `SELECT COUNT(*) FROM service_hosts sh LEFT JOIN hosts h ON h.id = sh.host_id WHERE h.id IS NULL`},
		{"service_hosts.service_id", `SELECT COUNT(*) FROM service_hosts sh LEFT JOIN services s ON s.id = sh.service_id WHERE s.id IS NULL`},
		{"hosts.inbound_tag", `SELECT COUNT(*) FROM hosts h LEFT JOIN inbounds i ON i.tag = h.inbound_tag WHERE i.id IS NULL`},
		{"admins_services.admin_id", `SELECT COUNT(*) FROM admins_services l LEFT JOIN admins a ON a.id = l.admin_id WHERE a.id IS NULL`},
		{"admins_services.service_id", `SELECT COUNT(*) FROM admins_services l LEFT JOIN services s ON s.id = l.service_id WHERE s.id IS NULL`},
	}
	for _, check := range checks {
		count, err := countRows(ctx, db, check.sql)
		if err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("database integrity check failed: %s has %d orphan rows", check.name, count)
		}
	}

	return nil
}

func countRows(ctx context.Context, db *sql.DB, query string, args ...any) (int64, error) {
	var count int64
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
