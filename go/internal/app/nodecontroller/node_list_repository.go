package nodecontroller

import (
	"context"
	"database/sql"
	"strings"
)

func (r Repository) ListNodeItems(ctx context.Context, nodeID int64) ([]NodeListItem, string, error) {
	defaultCert := ""
	var defaultCertNull sql.NullString
	if err := r.db.QueryRowContext(ctx, `SELECT certificate FROM tls ORDER BY id LIMIT 1`).Scan(&defaultCertNull); err == nil && defaultCertNull.Valid {
		defaultCert = defaultCertNull.String
	}

	query := `SELECT
	id,
	COALESCE(name, ''),
	address,
	port,
	api_port,
	usage_coefficient,
	data_limit,
	use_nobetci,
	nobetci_port,
	proxy_enabled,
	proxy_type,
	proxy_host,
	proxy_port,
	proxy_username,
	proxy_password,
	status,
	message,
	xray_version,
	COALESCE(geo_mode, 'default'),
	COALESCE(xray_config_mode, 'default'),
	COALESCE(uplink, 0),
	COALESCE(downlink, 0),
	certificate
FROM nodes`
	args := []any{}
	if nodeID > 0 {
		query += ` WHERE id = ?`
		args = append(args, nodeID)
	}
	query += ` ORDER BY id`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, defaultCert, err
	}
	defer rows.Close()

	result := []NodeListItem{}
	for rows.Next() {
		var item NodeListItem
		var dataLimit, nobetciPort, proxyPort sql.NullInt64
		var proxyType, proxyHost, proxyUsername, proxyPassword, message, xrayVersion, certificate sql.NullString
		var useNobetci, proxyEnabled bool
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Address,
			&item.Port,
			&item.APIPort,
			&item.UsageCoefficient,
			&dataLimit,
			&useNobetci,
			&nobetciPort,
			&proxyEnabled,
			&proxyType,
			&proxyHost,
			&proxyPort,
			&proxyUsername,
			&proxyPassword,
			&item.Status,
			&message,
			&xrayVersion,
			&item.GeoMode,
			&item.XrayConfigMode,
			&item.Uplink,
			&item.Downlink,
			&certificate,
		); err != nil {
			return nil, defaultCert, err
		}
		item.DataLimit = int64PtrFromNull(dataLimit)
		item.UseNobetci = useNobetci
		item.NobetciPort = int64PtrFromNull(nobetciPort)
		item.ProxyEnabled = proxyEnabled
		item.ProxyType = stringPtrFromNull(proxyType)
		item.ProxyHost = stringPtrFromNull(proxyHost)
		item.ProxyPort = int64PtrFromNull(proxyPort)
		item.ProxyUsername = stringPtrFromNull(proxyUsername)
		item.ProxyPassword = stringPtrFromNull(proxyPassword)
		item.Message = stringPtrFromNull(message)
		item.XrayVersion = stringPtrFromNull(xrayVersion)
		if certificate.Valid && strings.TrimSpace(certificate.String) != "" {
			item.NodeCertificate = &certificate.String
		}
		if item.UsageCoefficient <= 0 {
			item.UsageCoefficient = 1
		}
		result = append(result, item)
	}
	return result, defaultCert, rows.Err()
}

func stringPtrFromNull(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	trimmed := strings.TrimSpace(value.String)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func int64PtrFromNull(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
