package nodecontroller

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type UserUsageDelta struct {
	UserID int64
	Value  int64
}

type OutboundUsageDelta struct {
	Tag  string
	Up   int64
	Down int64
}

type usageUserMapping struct {
	UserID    int64
	AdminID   sql.NullInt64
	ServiceID sql.NullInt64
}

func (r Repository) UsageNodes(ctx context.Context, nodeID int64, limit int) ([]NodeRow, error) {
	query := `SELECT
	id,
	COALESCE(name, ''),
	address,
	port,
	api_port,
	status,
	xray_version,
	message,
	certificate,
	certificate_key,
	xray_config_mode,
	xray_config,
	usage_coefficient
FROM nodes
WHERE status NOT IN ('disabled', 'limited')`
	args := []any{}
	if nodeID > 0 {
		query += ` AND id = ?`
		args = append(args, nodeID)
	}
	query += ` ORDER BY id`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []NodeRow
	for rows.Next() {
		row, err := scanNodeRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

type nodeRowScanner interface {
	Scan(dest ...any) error
}

func scanNodeRow(scanner nodeRowScanner) (NodeRow, error) {
	var row NodeRow
	var xrayVersion, message, cert, key, mode sql.NullString
	var rawConfig sql.NullString
	err := scanner.Scan(
		&row.ID,
		&row.Name,
		&row.Address,
		&row.Port,
		&row.APIPort,
		&row.Status,
		&xrayVersion,
		&message,
		&cert,
		&key,
		&mode,
		&rawConfig,
		&row.UsageCoefficient,
	)
	if err != nil {
		return NodeRow{}, err
	}
	row.XrayVersion = xrayVersion.String
	row.Message = message.String
	row.Certificate = cert.String
	row.CertificateKey = key.String
	row.XrayConfigMode = mode.String
	if rawConfig.Valid && strings.TrimSpace(rawConfig.String) != "" {
		row.XrayConfig = json.RawMessage(rawConfig.String)
	}
	if row.UsageCoefficient <= 0 {
		row.UsageCoefficient = 1
	}
	return row, nil
}

func (r Repository) PersistCollectedUsage(ctx context.Context, node NodeRow, userDeltas []UserUsageDelta, outboundDeltas []OutboundUsageDelta) error {
	if len(userDeltas) == 0 && len(outboundDeltas) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	bucket := now.Truncate(time.Hour)

	filteredUsers, changedUserIDs, err := r.persistUserUsage(ctx, tx, node, userDeltas, bucket, now)
	if err != nil {
		return err
	}
	if err := r.persistOutboundUsage(ctx, tx, node, outboundDeltas, bucket, now); err != nil {
		return err
	}
	if len(changedUserIDs) > 0 {
		if err := r.enqueueDisableOperations(ctx, tx, changedUserIDs, now); err != nil {
			return err
		}
	}
	_ = filteredUsers

	return tx.Commit()
}

func (r Repository) persistUserUsage(ctx context.Context, tx *sql.Tx, node NodeRow, deltas []UserUsageDelta, bucket time.Time, now time.Time) (map[int64]int64, []int64, error) {
	aggregated := map[int64]int64{}
	for _, delta := range deltas {
		if delta.UserID <= 0 || delta.Value <= 0 {
			continue
		}
		value := int64(math.Round(float64(delta.Value) * node.UsageCoefficient))
		if value <= 0 {
			continue
		}
		aggregated[delta.UserID] += value
	}
	if len(aggregated) == 0 {
		return aggregated, nil, nil
	}

	mapping, err := r.loadUsageUserMapping(ctx, tx, keysInt64(aggregated))
	if err != nil {
		return nil, nil, err
	}
	if len(mapping) == 0 {
		return map[int64]int64{}, nil, nil
	}

	adminUsage := map[int64]int64{}
	serviceUsage := map[int64]int64{}
	adminServiceUsage := map[[2]int64]int64{}

	for userID, value := range aggregated {
		row, ok := mapping[userID]
		if !ok {
			delete(aggregated, userID)
			continue
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE users
SET used_traffic = COALESCE(used_traffic, 0) + ?,
    online_at = CASE WHEN status IN ('active', 'on_hold') THEN ? ELSE online_at END
WHERE id = ?`,
			value,
			r.timeArg(now),
			userID,
		); err != nil {
			return nil, nil, err
		}
		if row.AdminID.Valid {
			adminUsage[row.AdminID.Int64] += value
		}
		if row.ServiceID.Valid {
			serviceUsage[row.ServiceID.Int64] += value
			if row.AdminID.Valid {
				adminServiceUsage[[2]int64{row.AdminID.Int64, row.ServiceID.Int64}] += value
			}
		}
		if err := r.upsertNodeUserUsage(ctx, tx, bucket, userID, node.ID, value); err != nil {
			return nil, nil, err
		}
	}

	for adminID, value := range adminUsage {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE admins
SET users_usage = COALESCE(users_usage, 0) + ?,
    lifetime_usage = COALESCE(lifetime_usage, 0) + ?
WHERE id = ?`,
			value,
			value,
			adminID,
		); err != nil {
			return nil, nil, err
		}
	}

	for serviceID, value := range serviceUsage {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE services
SET used_traffic = COALESCE(used_traffic, 0) + ?,
    lifetime_used_traffic = COALESCE(lifetime_used_traffic, 0) + ?,
    users_usage = COALESCE(users_usage, 0) + ?,
    updated_at = ?
WHERE id = ?`,
			value,
			value,
			value,
			r.timeArg(now),
			serviceID,
		); err != nil {
			return nil, nil, err
		}
	}

	for key, value := range adminServiceUsage {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE admins_services
SET used_traffic = COALESCE(used_traffic, 0) + ?,
    lifetime_used_traffic = COALESCE(lifetime_used_traffic, 0) + ?,
    updated_at = ?
WHERE admin_id = ? AND service_id = ?`,
			value,
			value,
			r.timeArg(now),
			key[0],
			key[1],
		); err != nil {
			return nil, nil, err
		}
	}

	changed, err := r.markUsersLimitedOrExpired(ctx, tx, keysInt64(aggregated), now)
	if err != nil {
		return nil, nil, err
	}
	return aggregated, changed, nil
}

func (r Repository) loadUsageUserMapping(ctx context.Context, tx *sql.Tx, userIDs []int64) (map[int64]usageUserMapping, error) {
	if len(userIDs) == 0 {
		return map[int64]usageUserMapping{}, nil
	}
	query := `SELECT id, admin_id, service_id FROM users WHERE status IN ('active', 'on_hold') AND id IN (` + placeholders(len(userIDs)) + `)`
	rows, err := tx.QueryContext(ctx, query, int64Args(userIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int64]usageUserMapping{}
	for rows.Next() {
		var row usageUserMapping
		if err := rows.Scan(&row.UserID, &row.AdminID, &row.ServiceID); err != nil {
			return nil, err
		}
		result[row.UserID] = row
	}
	return result, rows.Err()
}

func (r Repository) markUsersLimitedOrExpired(ctx context.Context, tx *sql.Tx, userIDs []int64, now time.Time) ([]int64, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	nowUnix := now.Unix()
	query := `SELECT id,
       CASE
         WHEN data_limit IS NOT NULL AND data_limit > 0 AND COALESCE(used_traffic, 0) >= data_limit THEN 'limited'
         ELSE 'expired'
       END AS target_status
FROM users
WHERE id IN (` + placeholders(len(userIDs)) + `)
  AND status IN ('active', 'on_hold')
  AND (
    (data_limit IS NOT NULL AND data_limit > 0 AND COALESCE(used_traffic, 0) >= data_limit)
    OR (expire IS NOT NULL AND expire > 0 AND expire <= ?)
  )`
	args := int64Args(userIDs)
	args = append(args, nowUnix)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	var changed []int64
	byStatus := map[string][]int64{"limited": {}, "expired": {}}
	for rows.Next() {
		var id int64
		var targetStatus string
		if err := rows.Scan(&id, &targetStatus); err != nil {
			rows.Close()
			return nil, err
		}
		changed = append(changed, id)
		if targetStatus != "expired" {
			targetStatus = "limited"
		}
		byStatus[targetStatus] = append(byStatus[targetStatus], id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(changed) == 0 {
		return nil, nil
	}
	for status, ids := range byStatus {
		if len(ids) == 0 {
			continue
		}
		updateArgs := []any{status, r.timeArg(now)}
		updateArgs = append(updateArgs, int64Args(ids)...)
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE users SET status = ?, last_status_change = ? WHERE id IN (`+placeholders(len(ids))+`)`,
			updateArgs...,
		); err != nil {
			return nil, err
		}
	}
	return changed, nil
}

func (r Repository) persistOutboundUsage(ctx context.Context, tx *sql.Tx, node NodeRow, deltas []OutboundUsageDelta, bucket time.Time, now time.Time) error {
	byTag := map[string]OutboundUsageDelta{}
	for _, delta := range deltas {
		tag := strings.TrimSpace(delta.Tag)
		if tag == "" {
			continue
		}
		item := byTag[tag]
		item.Tag = tag
		item.Up += maxInt64Usage(delta.Up, 0)
		item.Down += maxInt64Usage(delta.Down, 0)
		byTag[tag] = item
	}
	if len(byTag) == 0 {
		return nil
	}

	var totalUp, totalDown int64
	for _, delta := range byTag {
		totalUp += delta.Up
		totalDown += delta.Down
		if err := r.upsertOutboundTraffic(ctx, tx, node.ID, delta, now); err != nil {
			return err
		}
	}
	if totalUp != 0 || totalDown != 0 {
		if err := r.upsertNodeUsage(ctx, tx, bucket, node.ID, totalUp, totalDown); err != nil {
			return err
		}
		if err := r.incrementSystemUsage(ctx, tx, totalUp, totalDown); err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE nodes
SET uplink = COALESCE(uplink, 0) + ?,
    downlink = COALESCE(downlink, 0) + ?
WHERE id = ?`,
			totalUp,
			totalDown,
			node.ID,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE nodes
SET status = 'limited',
    message = 'Data limit reached',
    last_status_change = ?
WHERE id = ?
  AND data_limit IS NOT NULL
  AND data_limit > 0
  AND (COALESCE(uplink, 0) + COALESCE(downlink, 0)) >= data_limit`,
			r.timeArg(now),
			node.ID,
		); err != nil {
			return err
		}
	}
	return nil
}

func (r Repository) upsertNodeUserUsage(ctx context.Context, tx *sql.Tx, bucket time.Time, userID int64, nodeID int64, value int64) error {
	if r.dialect == "sqlite" {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO node_user_usages (created_at, user_id, node_id, used_traffic)
VALUES (?, ?, ?, ?)
ON CONFLICT(created_at, user_id, node_id) DO UPDATE
SET used_traffic = COALESCE(node_user_usages.used_traffic, 0) + excluded.used_traffic`,
			r.timeArg(bucket),
			userID,
			nodeID,
			value,
		)
		return err
	}
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO node_user_usages (created_at, user_id, node_id, used_traffic)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE used_traffic = COALESCE(used_traffic, 0) + VALUES(used_traffic)`,
		r.timeArg(bucket),
		userID,
		nodeID,
		value,
	)
	return err
}

func (r Repository) upsertNodeUsage(ctx context.Context, tx *sql.Tx, bucket time.Time, nodeID int64, up int64, down int64) error {
	if r.dialect == "sqlite" {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO node_usages (created_at, node_id, uplink, downlink)
VALUES (?, ?, ?, ?)
ON CONFLICT(created_at, node_id) DO UPDATE
SET uplink = COALESCE(node_usages.uplink, 0) + excluded.uplink,
    downlink = COALESCE(node_usages.downlink, 0) + excluded.downlink`,
			r.timeArg(bucket),
			nodeID,
			up,
			down,
		)
		return err
	}
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO node_usages (created_at, node_id, uplink, downlink)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    uplink = COALESCE(uplink, 0) + VALUES(uplink),
    downlink = COALESCE(downlink, 0) + VALUES(downlink)`,
		r.timeArg(bucket),
		nodeID,
		up,
		down,
	)
	return err
}

func (r Repository) upsertOutboundTraffic(ctx context.Context, tx *sql.Tx, nodeID int64, delta OutboundUsageDelta, now time.Time) error {
	targetID := fmt.Sprintf("node:%d", nodeID)
	outboundID := "tag_" + delta.Tag
	if r.dialect == "sqlite" {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO outbound_traffic (target_id, node_id, outbound_id, tag, uplink, downlink, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(target_id, outbound_id) DO UPDATE
SET node_id = excluded.node_id,
    tag = excluded.tag,
    uplink = COALESCE(outbound_traffic.uplink, 0) + excluded.uplink,
    downlink = COALESCE(outbound_traffic.downlink, 0) + excluded.downlink,
    updated_at = excluded.updated_at`,
			targetID,
			nodeID,
			outboundID,
			delta.Tag,
			delta.Up,
			delta.Down,
			r.timeArg(now),
			r.timeArg(now),
		)
		return err
	}
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO outbound_traffic (target_id, node_id, outbound_id, tag, uplink, downlink, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    node_id = VALUES(node_id),
    tag = VALUES(tag),
    uplink = COALESCE(uplink, 0) + VALUES(uplink),
    downlink = COALESCE(downlink, 0) + VALUES(downlink),
    updated_at = VALUES(updated_at)`,
		targetID,
		nodeID,
		outboundID,
		delta.Tag,
		delta.Up,
		delta.Down,
		r.timeArg(now),
		r.timeArg(now),
	)
	return err
}

func (r Repository) incrementSystemUsage(ctx context.Context, tx *sql.Tx, up int64, down int64) error {
	if r.dialect == "sqlite" {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO system (id, uplink, downlink)
VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE
SET uplink = COALESCE(system.uplink, 0) + excluded.uplink,
    downlink = COALESCE(system.downlink, 0) + excluded.downlink`,
			up,
			down,
		)
		return err
	}
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO system (id, uplink, downlink)
VALUES (1, ?, ?)
ON DUPLICATE KEY UPDATE
    uplink = COALESCE(uplink, 0) + VALUES(uplink),
    downlink = COALESCE(downlink, 0) + VALUES(downlink)`,
		up,
		down,
	)
	return err
}

func (r Repository) enqueueDisableOperations(ctx context.Context, tx *sql.Tx, userIDs []int64, now time.Time) error {
	if len(userIDs) == 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM nodes WHERE status NOT IN ('disabled', 'limited') ORDER BY id`)
	if err != nil {
		return err
	}
	var nodeIDs []int64
	for rows.Next() {
		var nodeID int64
		if err := rows.Scan(&nodeID); err != nil {
			rows.Close()
			return err
		}
		nodeIDs = append(nodeIDs, nodeID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(nodeIDs) == 0 {
		return nil
	}
	payload, err := json.Marshal(map[string]string{"queued_at": now.Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	for _, nodeID := range nodeIDs {
		for _, userID := range userIDs {
			key := operationKey("disable_user", nodeID, userID, now)
			_, err := tx.ExecContext(
				ctx,
				`INSERT INTO node_operations (operation_type, node_id, user_id, payload, status, attempts, idempotency_key, created_at, updated_at)
VALUES (?, ?, ?, ?, 'pending', 0, ?, ?, ?)`,
				"disable_user",
				nodeID,
				userID,
				string(payload),
				key,
				r.timeArg(now),
				r.timeArg(now),
			)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func operationKey(operationType string, nodeID int64, userID int64, now time.Time) string {
	sum := sha256.Sum256([]byte(operationType + ":" + strconv.FormatInt(nodeID, 10) + ":" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(now.UnixNano(), 10)))
	return hex.EncodeToString(sum[:])
}

func keysInt64(values map[int64]int64) []int64 {
	result := make([]int64, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}

func int64Args(values []int64) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func maxInt64Usage(value int64, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}
