package user

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	adminapp "github.com/rebeccapanel/rebecca/go/internal/app/admin"
)

func rollbackQuiet(tx *sql.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}

func permissionHTTPError(err error) error {
	if err == nil {
		return nil
	}
	var perm PermissionError
	if errorsAs(err, &perm) {
		return clientError(403, err.Error())
	}
	return clientError(400, err.Error())
}

func errorsAs(err error, target any) bool {
	switch target.(type) {
	case *PermissionError:
		_, ok := err.(PermissionError)
		return ok
	default:
		return false
	}
}

func dbTime(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04:05")
}

func nullableInt64Value(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableInt64Ptr(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableStringValue(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableStringPtr(value *string) any {
	if value == nil {
		return nil
	}
	clean := strings.TrimSpace(*value)
	if clean == "" {
		return nil
	}
	return clean
}

func nilIfZero(value *int64) *int64 {
	if value == nil || *value == 0 {
		return nil
	}
	return value
}

func int64OrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func int64PtrValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func resetStrategyOrDefault(value UserDataLimitResetStrategy) string {
	if value == "" {
		return string(UserDataLimitResetNoReset)
	}
	return string(value)
}

func rawFieldPresent(fields map[string]json.RawMessage, key string) bool {
	_, ok := fields[key]
	return ok
}

func sameInt64Ptr(a *int64, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func isRuntimeStatus(status UserStatus) bool {
	return status == UserStatusActive || status == UserStatusOnHold
}

func statusBecomesActive(oldStatus UserStatus, newStatus UserStatus) bool {
	return !isRuntimeStatus(oldStatus) && isRuntimeStatus(newStatus)
}

func operationForStatusChange(oldStatus UserStatus, newStatus UserStatus) string {
	if isRuntimeStatus(oldStatus) && !isRuntimeStatus(newStatus) {
		return NodeOperationDisableUser
	}
	if !isRuntimeStatus(oldStatus) && isRuntimeStatus(newStatus) {
		return NodeOperationEnableUser
	}
	if isRuntimeStatus(newStatus) {
		return NodeOperationUpdateUser
	}
	return ""
}

func ensureCanAccessUser(admin adminapp.Admin, user existingUserRow) error {
	if admin.Role == adminapp.RoleSudo || admin.Role == adminapp.RoleFullAccess {
		return nil
	}
	if user.AdminID != nil && *user.AdminID == admin.ID {
		return nil
	}
	return clientError(403, "You're not allowed")
}

func excludedTagsForProtocol(protocol string, selected []string, catalog MutationContext) []string {
	protocol = normalizeProtocol(protocol)
	selectedSet := map[string]struct{}{}
	for _, tag := range selected {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			selectedSet[tag] = struct{}{}
		}
	}
	all := []string{}
	for tag, info := range catalog.Inbounds {
		if normalizeProtocol(info.Protocol) != protocol {
			continue
		}
		if !info.HasEnabledHosts {
			continue
		}
		all = append(all, tag)
	}
	if len(all) == 0 {
		return []string{}
	}
	if len(selectedSet) == 0 {
		return all
	}
	excluded := []string{}
	for _, tag := range all {
		if _, ok := selectedSet[tag]; !ok {
			excluded = append(excluded, tag)
		}
	}
	return excluded
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

type nextPlanRow struct {
	ID                  int64
	DataLimit           int64
	Expire              *int64
	AddRemainingTraffic bool
	FireOnEither        bool
	IncreaseDataLimit   bool
	StartOnFirstConnect bool
	TriggerOn           string
}

func (r Repository) ensureUsernameAvailableTx(ctx context.Context, tx *sql.Tx, username string) error {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE LOWER(username) = LOWER(?) AND status != ? LIMIT 1`, username, string(UserStatusDeleted)).Scan(&id)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return clientError(409, "User username already exists")
}

func (r Repository) existingUserTx(ctx context.Context, tx *sql.Tx, username string) (existingUserRow, error) {
	row := existingUserRow{}
	var dataLimit, expire, serviceID, adminID, holdDuration sql.NullInt64
	var credentialKey sql.NullString
	var onlineAt any
	err := tx.QueryRowContext(
		ctx,
		`SELECT id, username, status, COALESCE(used_traffic, 0), data_limit, expire, service_id, admin_id, credential_key, on_hold_expire_duration, online_at
FROM users WHERE LOWER(username) = LOWER(?) AND status != ? LIMIT 1`,
		username,
		string(UserStatusDeleted),
	).Scan(
		&row.ID,
		&row.Username,
		&row.Status,
		&row.UsedTraffic,
		&dataLimit,
		&expire,
		&serviceID,
		&adminID,
		&credentialKey,
		&holdDuration,
		&onlineAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return row, clientError(404, "User not found")
		}
		return row, err
	}
	row.DataLimit = int64Ptr(dataLimit)
	row.Expire = int64Ptr(expire)
	row.ServiceID = int64Ptr(serviceID)
	row.AdminID = int64Ptr(adminID)
	row.CredentialKey = nullStringValue(credentialKey)
	row.OnHoldExpireDuration = int64Ptr(holdDuration)
	if value := dbTimeString(onlineAt); value != "" {
		row.OnlineAt = &value
	}
	proxies, err := r.proxiesByUserTx(ctx, tx, row.ID)
	if err != nil {
		return row, err
	}
	row.Proxies = proxies
	return row, nil
}

func (r Repository) proxiesByUserTx(ctx context.Context, tx *sql.Tx, userID int64) (ProxyPayload, error) {
	rows, err := tx.QueryContext(ctx, `SELECT type, settings FROM proxies WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := ProxyPayload{}
	for rows.Next() {
		var protocol string
		var raw any
		if err := rows.Scan(&protocol, &raw); err != nil {
			return nil, err
		}
		result[normalizeProtocol(protocol)] = jsonMap(raw)
	}
	return result, rows.Err()
}

func (r Repository) mutationContextTx(ctx context.Context, tx *sql.Tx, admin adminapp.Admin, excludeUserID *int64) (MutationContext, error) {
	ctxData := MutationContext{
		ServiceActiveUsers: map[int64]int64{},
		Services:           map[int64]ServiceInfo{},
		Inbounds:           map[string]InboundInfo{},
	}
	activeArgs := []any{admin.ID, string(UserStatusActive), string(UserStatusOnHold)}
	activeSQL := `SELECT COUNT(*) FROM users WHERE admin_id = ? AND status IN (?, ?)`
	if excludeUserID != nil {
		activeSQL += ` AND id != ?`
		activeArgs = append(activeArgs, *excludeUserID)
	}
	if err := tx.QueryRowContext(ctx, activeSQL, activeArgs...).Scan(&ctxData.ActiveUsers); err != nil {
		return ctxData, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT service_id, COUNT(*) FROM users WHERE admin_id = ? AND service_id IS NOT NULL AND status IN (?, ?) GROUP BY service_id`, admin.ID, string(UserStatusActive), string(UserStatusOnHold))
	if err != nil {
		return ctxData, err
	}
	for rows.Next() {
		var serviceID int64
		var count int64
		if err := rows.Scan(&serviceID, &count); err != nil {
			rows.Close()
			return ctxData, err
		}
		ctxData.ServiceActiveUsers[serviceID] = count
	}
	rows.Close()

	resolved, _, _ := r.ResolvedInboundsByTag(ctx)
	hostRows, err := tx.QueryContext(ctx, `
SELECT h.inbound_tag, COALESCE(h.is_disabled, 0), COALESCE(sh.service_id, 0)
FROM hosts h
LEFT JOIN service_hosts sh ON sh.host_id = h.id`)
	if err == nil {
		for hostRows.Next() {
			var tag string
			var disabled bool
			var serviceID int64
			if err := hostRows.Scan(&tag, &disabled, &serviceID); err != nil {
				hostRows.Close()
				return ctxData, err
			}
			protocol := "vless"
			if inbound, ok := resolved[tag]; ok && strings.TrimSpace(stringValueAny(inbound["protocol"])) != "" {
				protocol = normalizeProtocol(stringValueAny(inbound["protocol"]))
			}
			info := ctxData.Inbounds[tag]
			info.Tag = tag
			info.Protocol = protocol
			if !disabled {
				info.HasEnabledHosts = true
			}
			if serviceID > 0 {
				info.ServiceIDs = append(info.ServiceIDs, serviceID)
			}
			ctxData.Inbounds[tag] = info
		}
		hostRows.Close()
	}
	for tag, inbound := range resolved {
		info := ctxData.Inbounds[tag]
		info.Tag = tag
		info.Protocol = normalizeProtocol(stringValueAny(inbound["protocol"]))
		if info.Protocol == "" {
			info.Protocol = "vless"
		}
		ctxData.Inbounds[tag] = info
	}

	serviceRows, err := tx.QueryContext(ctx, `SELECT id, name FROM services ORDER BY id`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "no such table") {
		return ctxData, err
	}
	if err == nil {
		for serviceRows.Next() {
			var service ServiceInfo
			if err := serviceRows.Scan(&service.ID, &service.Name); err != nil {
				serviceRows.Close()
				return ctxData, err
			}
			service.AllowedInbounds = map[string][]string{}
			ctxData.Services[service.ID] = service
		}
		serviceRows.Close()
	}
	adminRows, err := tx.QueryContext(ctx, `SELECT service_id, admin_id FROM admins_services`)
	if err == nil {
		for adminRows.Next() {
			var serviceID, adminID int64
			if err := adminRows.Scan(&serviceID, &adminID); err != nil {
				adminRows.Close()
				return ctxData, err
			}
			service := ctxData.Services[serviceID]
			service.AdminIDs = append(service.AdminIDs, adminID)
			ctxData.Services[serviceID] = service
		}
		adminRows.Close()
	}
	for _, inbound := range ctxData.Inbounds {
		for _, serviceID := range inbound.ServiceIDs {
			service := ctxData.Services[serviceID]
			if service.AllowedInbounds == nil {
				service.AllowedInbounds = map[string][]string{}
			}
			service.HasActiveHosts = service.HasActiveHosts || inbound.HasEnabledHosts
			service.AllowedInbounds[normalizeProtocol(inbound.Protocol)] = append(service.AllowedInbounds[normalizeProtocol(inbound.Protocol)], inbound.Tag)
			ctxData.Services[serviceID] = service
		}
	}
	for id, service := range ctxData.Services {
		for protocol, tags := range service.AllowedInbounds {
			sort.Strings(tags)
			service.AllowedInbounds[protocol] = uniqueStrings(tags)
		}
		ctxData.Services[id] = service
	}
	return ctxData, nil
}

func (r Repository) replaceProxiesTx(ctx context.Context, tx *sql.Tx, userID int64, proxies ProxyPayload, inbounds map[string][]string, serviceID *int64, catalog MutationContext) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM exclude_inbounds_association WHERE proxy_id IN (SELECT id FROM proxies WHERE user_id = ?)`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM proxies WHERE user_id = ?`, userID); err != nil {
		return err
	}
	protocols := make([]string, 0, len(proxies))
	for protocol := range proxies {
		protocols = append(protocols, normalizeProtocol(protocol))
	}
	sort.Strings(protocols)
	for _, protocol := range protocols {
		settingsJSON, err := json.Marshal(proxies[protocol])
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO proxies (user_id, type, settings) VALUES (?, ?, ?)`, userID, protocol, string(settingsJSON))
		if err != nil {
			return err
		}
		proxyID, _ := res.LastInsertId()
		if proxyID == 0 {
			if err := tx.QueryRowContext(ctx, `SELECT id FROM proxies WHERE user_id = ? AND type = ? ORDER BY id DESC LIMIT 1`, userID, protocol).Scan(&proxyID); err != nil {
				return err
			}
		}
		if serviceID == nil {
			excluded := excludedTagsForProtocol(protocol, inbounds[protocol], catalog)
			if err := r.replaceProxyExcludedTx(ctx, tx, proxyID, excluded); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r Repository) replaceProxySettingsOnlyTx(ctx context.Context, tx *sql.Tx, userID int64, proxies ProxyPayload) error {
	for protocol, settings := range proxies {
		settingsJSON, err := json.Marshal(settings)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE proxies SET settings = ? WHERE user_id = ? AND type = ?`, string(settingsJSON), userID, normalizeProtocol(protocol)); err != nil {
			return err
		}
	}
	return nil
}

func (r Repository) updateProxyInboundsTx(ctx context.Context, tx *sql.Tx, userID int64, inbounds map[string][]string, serviceID *int64, catalog MutationContext) error {
	if serviceID != nil {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, type FROM proxies WHERE user_id = ?`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var proxyID int64
		var protocol string
		if err := rows.Scan(&proxyID, &protocol); err != nil {
			return err
		}
		if err := r.replaceProxyExcludedTx(ctx, tx, proxyID, excludedTagsForProtocol(protocol, inbounds[normalizeProtocol(protocol)], catalog)); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r Repository) replaceProxyExcludedTx(ctx context.Context, tx *sql.Tx, proxyID int64, excluded []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM exclude_inbounds_association WHERE proxy_id = ?`, proxyID); err != nil {
		return err
	}
	for _, tag := range uniqueStrings(excluded) {
		if err := r.ensureInboundTx(ctx, tx, tag); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO exclude_inbounds_association (proxy_id, inbound_tag) VALUES (?, ?)`, proxyID, tag); err != nil {
			return err
		}
	}
	return nil
}

func (r Repository) ensureInboundTx(ctx context.Context, tx *sql.Tx, tag string) error {
	if strings.TrimSpace(tag) == "" {
		return nil
	}
	stmt := `INSERT IGNORE INTO inbounds (tag) VALUES (?)`
	if r.dialect == "sqlite" {
		stmt = `INSERT OR IGNORE INTO inbounds (tag) VALUES (?)`
	}
	_, err := tx.ExecContext(ctx, stmt, tag)
	return err
}

func (r Repository) replaceNextPlansTx(ctx context.Context, tx *sql.Tx, userID int64, nextPlan *NextPlanPayload, nextPlans []NextPlanPayload) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM next_plans WHERE user_id = ?`, userID); err != nil {
		return err
	}
	plans := nextPlans
	if len(plans) == 0 && nextPlan != nil {
		plans = []NextPlanPayload{*nextPlan}
	}
	for idx, plan := range plans {
		dataLimit := int64(0)
		if plan.DataLimit != nil {
			dataLimit = *plan.DataLimit
		}
		trigger := strings.TrimSpace(plan.TriggerOn)
		if trigger == "" {
			trigger = "either"
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO next_plans (user_id, position, data_limit, expire, add_remaining_traffic, fire_on_either, increase_data_limit, start_on_first_connect, trigger_on) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			userID,
			idx,
			dataLimit,
			nullableInt64Ptr(plan.Expire),
			plan.AddRemainingTraffic,
			plan.FireOnEither,
			plan.IncreaseDataLimit,
			plan.StartOnFirstConnect,
			trigger,
		); err != nil {
			return err
		}
	}
	return nil
}

func (r Repository) nextPlanTx(ctx context.Context, tx *sql.Tx, userID int64) (*nextPlanRow, error) {
	var plan nextPlanRow
	var expire sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT id, COALESCE(data_limit, 0), expire, COALESCE(add_remaining_traffic, 0), COALESCE(fire_on_either, 1), COALESCE(increase_data_limit, 0), COALESCE(start_on_first_connect, 0), COALESCE(trigger_on, 'either') FROM next_plans WHERE user_id = ? ORDER BY position, id LIMIT 1`, userID).Scan(&plan.ID, &plan.DataLimit, &expire, &plan.AddRemainingTraffic, &plan.FireOnEither, &plan.IncreaseDataLimit, &plan.StartOnFirstConnect, &plan.TriggerOn)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	plan.Expire = int64Ptr(expire)
	return &plan, nil
}

func (r Repository) compactNextPlansTx(ctx context.Context, tx *sql.Tx, userID int64) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM next_plans WHERE user_id = ? ORDER BY position, id`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	pos := int64(0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE next_plans SET position = ? WHERE id = ?`, pos, id); err != nil {
			return err
		}
		pos++
	}
	return rows.Err()
}

func (r Repository) enqueueUserOperationForNodesTx(ctx context.Context, tx *sql.Tx, operationType string, userID int64, queuedAt time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM nodes WHERE COALESCE(status, '') NOT IN ('disabled', 'limited') ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var nodeID int64
		if err := rows.Scan(&nodeID); err != nil {
			return err
		}
		payload := map[string]any{"queued_at": queuedAt.Format(time.RFC3339Nano)}
		if err := r.enqueueNodeOperationTx(ctx, tx, operationType, nodeID, userID, payload, queuedAt); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r Repository) enqueueNodeOperationTx(ctx context.Context, tx *sql.Tx, operationType string, nodeID int64, userID int64, payload any, now time.Time) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	keySource := fmt.Sprintf("%s:%d:%d:%s", operationType, nodeID, userID, string(payloadJSON))
	sum := sha256.Sum256([]byte(keySource))
	key := hex.EncodeToString(sum[:])
	var existing int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM node_operations WHERE idempotency_key = ? LIMIT 1`, key).Scan(&existing)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO node_operations (operation_type, node_id, user_id, payload, status, attempts, idempotency_key, created_at, updated_at) VALUES (?, ?, ?, ?, 'pending', 0, ?, ?, ?)`,
		operationType,
		nodeID,
		userID,
		string(payloadJSON),
		key,
		dbTime(now),
		dbTime(now),
	)
	return err
}

func (r Repository) recordCreatedTrafficTx(ctx context.Context, tx *sql.Tx, admin adminapp.Admin, serviceID *int64, delta int64, action string, now time.Time) error {
	if delta == 0 || admin.ID <= 0 || admin.Role == adminapp.RoleFullAccess {
		return nil
	}
	if admin.UseServiceTrafficLimits && serviceID != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE admins_services SET created_traffic = COALESCE(created_traffic, 0) + ?, updated_at = ? WHERE admin_id = ? AND service_id = ?`, delta, dbTime(now), admin.ID, *serviceID); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE admins SET created_traffic = COALESCE(created_traffic, 0) + ? WHERE id = ?`, delta, admin.ID); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO admin_created_traffic_logs (admin_id, service_id, amount, action, created_at) VALUES (?, ?, ?, ?, ?)`, admin.ID, nullableInt64Ptr(serviceID), delta, action, dbTime(now))
	return err
}
