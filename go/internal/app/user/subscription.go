package user

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rebeccapanel/rebecca/go/internal/app/usage"
)

type SubscriptionClientConfig struct {
	Format  string
	Media   string
	Base64  bool
	Reverse bool
}

type SubscriptionRenderRequest struct {
	Identifier string
	Username   string
	Key        string
	ClientType string
	UserAgent  string
	Accept     string
	URL        string
	Start      string
	End        string
	ReadOnly   bool
	Usage      usage.Service
}

type SubscriptionHTTPResponse struct {
	Status    int
	MediaType string
	Headers   map[string]string
	Body      []byte
	JSON      any
}

type subscriptionTokenPayload struct {
	Username  string
	CreatedAt time.Time
}

var subscriptionClientConfigs = map[string]SubscriptionClientConfig{
	"clash-meta": {Format: "clash-meta", Media: "text/yaml"},
	"sing-box":   {Format: "sing-box", Media: "application/json"},
	"clash":      {Format: "clash", Media: "text/yaml"},
	"v2ray":      {Format: "v2ray", Media: "text/plain", Base64: true},
	"outline":    {Format: "outline", Media: "application/json"},
	"v2ray-json": {Format: "v2ray-json", Media: "application/json"},
}

func NormalizeSubscriptionClientType(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "json" {
		value = "v2ray-json"
	}
	_, ok := subscriptionClientConfigs[value]
	return value, ok
}

func (s Service) RenderSubscription(ctx context.Context, req SubscriptionRenderRequest) (SubscriptionHTTPResponse, error) {
	user, err := s.resolveSubscriptionUser(ctx, req)
	if err != nil {
		return SubscriptionHTTPResponse{}, err
	}
	if strings.Contains(req.Accept, "text/html") && req.ClientType == "" {
		return SubscriptionHTTPResponse{
			Status:    200,
			MediaType: "text/html; charset=utf-8",
			Body:      []byte(subscriptionHTML(user, req)),
		}, nil
	}
	if !req.ReadOnly {
		_ = s.repo.updateSubscriptionAccess(ctx, user.ID, req.UserAgent)
	}
	clientType := req.ClientType
	if clientType == "" {
		clientType = selectSubscriptionClientType(req.UserAgent, s.effectiveSettings(ctx, user.AdminID))
	}
	config, ok := subscriptionClientConfigs[clientType]
	if !ok {
		return SubscriptionHTTPResponse{}, clientError(404, "Unsupported client type")
	}
	body, err := s.generateSubscriptionConfig(ctx, user, config)
	if err != nil {
		return SubscriptionHTTPResponse{}, err
	}
	return SubscriptionHTTPResponse{
		Status:    200,
		MediaType: config.Media,
		Headers:   subscriptionHeaders(user, req, s.effectiveSettings(ctx, user.AdminID)),
		Body:      []byte(body),
	}, nil
}

func (s Service) SubscriptionInfo(ctx context.Context, req SubscriptionRenderRequest) (UserDetail, error) {
	return s.resolveSubscriptionUser(ctx, req)
}

func (s Service) SubscriptionUsage(ctx context.Context, req SubscriptionRenderRequest) (map[string]any, error) {
	user, err := s.resolveSubscriptionUser(ctx, req)
	if err != nil {
		return nil, err
	}
	start, end, err := subscriptionUsageRange(req.Start, req.End)
	if err != nil {
		return nil, clientError(400, "Invalid date range or format")
	}
	daily, err := req.Usage.UserUsageTimeseries(ctx, usage.UsageRequest{
		UserID:      user.ID,
		Start:       start.Format(time.RFC3339Nano),
		End:         end.Format(time.RFC3339Nano),
		Granularity: "day",
	})
	if err != nil {
		return nil, err
	}
	hourly := []map[string]any{}
	if sameUTCDate(start, end) {
		rows, err := req.Usage.UserUsageTimeseries(ctx, usage.UsageRequest{
			UserID:      user.ID,
			Start:       start.Format(time.RFC3339Nano),
			End:         end.Format(time.RFC3339Nano),
			Granularity: "hour",
		})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			hourly = append(hourly, map[string]any{"timestamp": row.Timestamp, "used_traffic": row.UsedTraffic})
		}
	}
	nodes, err := req.Usage.UserUsageByNodes(ctx, usage.UsageRequest{
		UserID: user.ID,
		Start:  start.Format(time.RFC3339Nano),
		End:    end.Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, err
	}
	usages := make([]map[string]any, 0, len(daily))
	for _, row := range daily {
		date := row.Timestamp
		if len(date) >= 10 {
			date = date[:10]
		}
		usages = append(usages, map[string]any{"date": date, "used_traffic": row.UsedTraffic})
	}
	return map[string]any{
		"username":      user.Username,
		"start":         start.Format(time.RFC3339Nano),
		"end":           end.Format(time.RFC3339Nano),
		"usages":        usages,
		"hourly_usages": hourly,
		"node_usages":   nodes,
	}, nil
}

func (s Service) ResolveSubscriptionAlias(ctx context.Context, path string, query url.Values) (SubscriptionRenderRequest, bool, error) {
	settings, err := s.repo.subscriptionSettings(ctx)
	if err != nil {
		return SubscriptionRenderRequest{}, false, err
	}
	if req, ok := resolvePrefixedSubscriptionPath(path, "/sub/"); ok {
		return req, true, nil
	}
	if configured := "/" + normalizePath(settings.SubscriptionPath) + "/"; configured != "/sub/" {
		if req, ok := resolvePrefixedSubscriptionPath(path, configured); ok {
			return req, true, nil
		}
	}
	if clean := strings.TrimRight(path, "/"); clean == "/api/v1/client/subscribe" {
		identifier := firstNonEmptyString(query.Get("token"), query.Get("key"), query.Get("identifier"))
		if identifier == "" {
			return SubscriptionRenderRequest{}, true, clientError(400, "Provide token, key, or identifier")
		}
		return SubscriptionRenderRequest{Identifier: identifier}, true, nil
	}
	if strings.HasPrefix(path, "/api/v1/client/subscribe/") {
		identifier := strings.Trim(strings.TrimPrefix(path, "/api/v1/client/subscribe/"), "/")
		if identifier != "" {
			return SubscriptionRenderRequest{Identifier: identifier}, true, nil
		}
	}
	for _, alias := range settings.SubscriptionAliases {
		if identifier := matchSubscriptionQueryAlias(alias, path, query); identifier != "" {
			return SubscriptionRenderRequest{Identifier: identifier}, true, nil
		}
		if identifier := matchSubscriptionPathAlias(alias, path); identifier != "" {
			return SubscriptionRenderRequest{Identifier: identifier}, true, nil
		}
	}
	return SubscriptionRenderRequest{}, false, nil
}

func (s Service) resolveSubscriptionUser(ctx context.Context, req SubscriptionRenderRequest) (UserDetail, error) {
	if req.Username != "" || req.Key != "" {
		return s.repo.subscriptionUserByUsernameKey(ctx, req.Username, req.Key)
	}
	for _, candidate := range candidateIdentifiers(req.Identifier) {
		if user, err := s.resolveSubscriptionToken(ctx, candidate); err == nil {
			return user, nil
		}
		if isCredentialKey(candidate) {
			if user, err := s.repo.subscriptionUserByKeyOnly(ctx, candidate); err == nil {
				return user, nil
			}
		}
		if user, err := s.repo.subscriptionUserBySubadress(ctx, candidate); err == nil {
			return user, nil
		}
	}
	return UserDetail{}, clientError(404, "Not Found")
}

func (s Service) resolveSubscriptionToken(ctx context.Context, token string) (UserDetail, error) {
	secret, err := s.repo.subscriptionSecretKey(ctx)
	if err != nil {
		return UserDetail{}, err
	}
	payload, ok := parseSubscriptionToken(token, secret)
	if !ok {
		return UserDetail{}, clientError(404, "Not Found")
	}
	user, err := s.repo.subscriptionUserByUsername(ctx, payload.Username)
	if err != nil {
		return UserDetail{}, err
	}
	created, ok := parseDBTime(user.CreatedAt)
	if !ok || created.After(payload.CreatedAt) {
		return UserDetail{}, clientError(404, "Not Found")
	}
	revoked, hasRevoked, err := s.repo.subscriptionRevokedAt(ctx, user.ID)
	if err != nil {
		return UserDetail{}, err
	}
	if hasRevoked && revoked.After(payload.CreatedAt) {
		return UserDetail{}, clientError(404, "Not Found")
	}
	return user, nil
}

func (s Service) effectiveSettings(ctx context.Context, adminID *int64) SubscriptionSettings {
	settings, err := s.repo.subscriptionSettings(ctx)
	if err != nil {
		return SubscriptionSettings{SubscriptionProfileTitle: "Subscription", SubscriptionSupportURL: "https://t.me/", SubscriptionUpdateInterval: "12", SubscriptionPath: "sub"}
	}
	admin := AdminLinkSettings{}
	if adminID != nil && *adminID > 0 {
		admins, err := s.repo.adminLinkSettings(ctx, []int64{*adminID})
		if err == nil {
			admin = admins[*adminID]
		}
	}
	return effectiveSubscriptionSettings(settings, admin)
}

func (s Service) generateSubscriptionConfig(ctx context.Context, user UserDetail, config SubscriptionClientConfig) (string, error) {
	links, err := s.ConfigLinks(ctx, ConfigLinksRequest{UserID: user.ID, Reverse: config.Reverse})
	if err != nil {
		return "", err
	}
	raw := links.Links
	switch config.Format {
	case "v2ray":
		content := strings.Join(raw, "\n")
		if config.Base64 {
			return base64.StdEncoding.EncodeToString([]byte(content)), nil
		}
		return content, nil
	case "outline":
		return marshalPretty(map[string]any{"servers": raw})
	case "v2ray-json":
		outbounds := make([]map[string]any, 0, len(raw))
		for i, link := range raw {
			outbounds = append(outbounds, map[string]any{"tag": fmt.Sprintf("proxy-%d", i+1), "share_link": link})
		}
		return marshalPretty(map[string]any{"remarks": []string{user.Username}, "outbounds": outbounds})
	case "sing-box":
		outbounds := make([]map[string]any, 0, len(raw)+1)
		for i, link := range raw {
			outbounds = append(outbounds, map[string]any{"type": "selector", "tag": fmt.Sprintf("proxy-%d", i+1), "outbounds": []string{link}})
		}
		return marshalPretty(map[string]any{"outbounds": outbounds})
	case "clash", "clash-meta":
		return renderClashLikeYAML(user.Username, raw, config.Format == "clash-meta"), nil
	default:
		return "", clientError(404, "Unsupported client type")
	}
}

func (r Repository) subscriptionUserByUsername(ctx context.Context, username string) (UserDetail, error) {
	return r.UserGet(ctx, UserGetRequest{
		Username: strings.TrimSpace(username),
		Admin:    AdminContext{Username: "__subscription__", Role: "sudo", CanViewTraffic: true, CanSortTraffic: true},
	})
}

func (r Repository) subscriptionUserByUsernameKey(ctx context.Context, username string, key string) (UserDetail, error) {
	user, err := r.subscriptionUserByUsername(ctx, username)
	if err != nil {
		return UserDetail{}, clientError(404, "Not Found")
	}
	normalizedKey, keyOK := normalizeSubscriptionKey(key)
	if keyOK && user.CredentialKey != "" {
		stored, storedOK := normalizeSubscriptionKey(user.CredentialKey)
		if storedOK && stored == normalizedKey {
			return user, nil
		}
		return UserDetail{}, clientError(404, "Not Found")
	}
	if strings.TrimSpace(key) != "" && strings.EqualFold(strings.TrimSpace(user.Subadress), strings.TrimSpace(key)) {
		return user, nil
	}
	return UserDetail{}, clientError(404, "Not Found")
}

func (r Repository) subscriptionUserByKeyOnly(ctx context.Context, key string) (UserDetail, error) {
	normalized, ok := normalizeSubscriptionKey(key)
	if !ok {
		return UserDetail{}, clientError(400, "Invalid credential key")
	}
	var username string
	err := r.db.QueryRowContext(
		ctx,
		`SELECT username FROM users WHERE credential_key IS NOT NULL AND REPLACE(LOWER(credential_key), '-', '') = ? AND status != 'deleted' ORDER BY created_at DESC, id DESC LIMIT 1`,
		normalized,
	).Scan(&username)
	if err != nil {
		return UserDetail{}, clientError(404, "Not Found")
	}
	return r.subscriptionUserByUsername(ctx, username)
}

func (r Repository) subscriptionUserBySubadress(ctx context.Context, subadress string) (UserDetail, error) {
	subadress = strings.TrimSpace(subadress)
	if subadress == "" {
		return UserDetail{}, clientError(404, "Not Found")
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT username FROM users WHERE subadress != '' AND LOWER(subadress) = LOWER(?) AND status != 'deleted' ORDER BY created_at DESC, id DESC LIMIT 2`,
		subadress,
	)
	if err != nil {
		return UserDetail{}, err
	}
	defer rows.Close()
	usernames := []string{}
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return UserDetail{}, err
		}
		usernames = append(usernames, username)
	}
	if err := rows.Err(); err != nil {
		return UserDetail{}, err
	}
	if len(usernames) != 1 {
		return UserDetail{}, clientError(404, "Not Found")
	}
	return r.subscriptionUserByUsername(ctx, usernames[0])
}

func (r Repository) subscriptionRevokedAt(ctx context.Context, userID int64) (time.Time, bool, error) {
	var value any
	err := r.db.QueryRowContext(ctx, `SELECT sub_revoked_at FROM users WHERE id = ? LIMIT 1`, userID).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	parsed, ok := parseDBTime(value)
	return parsed, ok, nil
}

func (r Repository) updateSubscriptionAccess(ctx context.Context, userID int64, userAgent string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET sub_updated_at = ?, sub_last_user_agent = ? WHERE id = ?`, dbTime(time.Now().UTC()), strings.TrimSpace(userAgent), userID)
	return err
}

func parseSubscriptionToken(token string, secret string) (subscriptionTokenPayload, bool) {
	token = strings.TrimSpace(token)
	if len(token) < 15 || strings.TrimSpace(secret) == "" {
		return subscriptionTokenPayload{}, false
	}
	if strings.HasPrefix(token, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.") {
		return parseSubscriptionJWT(token, secret)
	}
	body := token[:len(token)-10]
	signature := token[len(token)-10:]
	if createSubscriptionTokenSignature(body, secret) != signature {
		return subscriptionTokenPayload{}, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return subscriptionTokenPayload{}, false
	}
	parts := strings.Split(string(decoded), ",")
	if len(parts) < 2 {
		return subscriptionTokenPayload{}, false
	}
	createdUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return subscriptionTokenPayload{}, false
	}
	return subscriptionTokenPayload{Username: parts[0], CreatedAt: time.Unix(createdUnix, 0).UTC()}, true
}

func parseSubscriptionJWT(token string, secret string) (subscriptionTokenPayload, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return subscriptionTokenPayload{}, false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return subscriptionTokenPayload{}, false
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return subscriptionTokenPayload{}, false
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		return subscriptionTokenPayload{}, false
	}
	if stringValue(payload["access"]) != "subscription" {
		return subscriptionTokenPayload{}, false
	}
	username := stringValue(payload["sub"])
	iat := int64Value(payload["iat"])
	if username == "" || iat <= 0 {
		return subscriptionTokenPayload{}, false
	}
	if exp := int64Value(payload["exp"]); exp > 0 && time.Now().UTC().After(time.Unix(exp, 0).UTC()) {
		return subscriptionTokenPayload{}, false
	}
	return subscriptionTokenPayload{Username: username, CreatedAt: time.Unix(iat, 0).UTC()}, true
}

func candidateIdentifiers(identifier string) []string {
	raw := strings.TrimSpace(identifier)
	if raw == "" {
		return nil
	}
	result := []string{raw}
	for _, sep := range []string{"+", ":", "|", " "} {
		if strings.Contains(raw, sep) {
			tail := strings.TrimSpace(raw[strings.LastIndex(raw, sep)+len(sep):])
			if tail != "" && !containsString(result, tail) {
				result = append(result, tail)
			}
		}
	}
	return result
}

func resolvePrefixedSubscriptionPath(path string, prefix string) (SubscriptionRenderRequest, bool) {
	if !strings.HasPrefix(path, prefix) {
		return SubscriptionRenderRequest{}, false
	}
	tail := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if tail == "" {
		return SubscriptionRenderRequest{}, false
	}
	segments := strings.Split(tail, "/")
	if len(segments) == 1 {
		return SubscriptionRenderRequest{Identifier: segments[0]}, true
	}
	if len(segments) == 2 {
		if client, ok := NormalizeSubscriptionClientType(segments[1]); ok {
			return SubscriptionRenderRequest{Identifier: segments[0], ClientType: client}, true
		}
		if segments[1] == "info" || segments[1] == "usage" {
			return SubscriptionRenderRequest{Identifier: segments[0], ClientType: segments[1]}, true
		}
		return SubscriptionRenderRequest{Username: segments[0], Key: segments[1]}, true
	}
	if len(segments) == 3 {
		if segments[2] == "info" || segments[2] == "usage" {
			return SubscriptionRenderRequest{Username: segments[0], Key: segments[1], ClientType: segments[2]}, true
		}
		if client, ok := NormalizeSubscriptionClientType(segments[2]); ok {
			return SubscriptionRenderRequest{Username: segments[0], Key: segments[1], ClientType: client}, true
		}
	}
	return SubscriptionRenderRequest{}, false
}

func matchSubscriptionPathAlias(alias string, path string) string {
	parsed, err := url.Parse(alias)
	if err != nil {
		return ""
	}
	aliasPath := strings.TrimSpace(parsed.Path)
	if aliasPath == "" {
		return ""
	}
	if strings.Contains(aliasPath, "{") {
		pattern := regexp.QuoteMeta(aliasPath)
		for _, placeholder := range []string{"\\{identifier\\}", "\\{token\\}", "\\{key\\}"} {
			pattern = strings.ReplaceAll(pattern, placeholder, "([^/]+)")
		}
		re := regexp.MustCompile("^" + pattern + "/?$")
		match := re.FindStringSubmatch(path)
		if len(match) > 1 {
			return match[1]
		}
		return ""
	}
	prefix := aliasPath
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	tail := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if tail == "" {
		return ""
	}
	return strings.Split(tail, "/")[0]
}

func matchSubscriptionQueryAlias(alias string, path string, query url.Values) string {
	parsed, err := url.Parse(alias)
	if err != nil || parsed.RawQuery == "" || strings.TrimRight(path, "/") != strings.TrimRight(parsed.Path, "/") {
		return ""
	}
	template := parsed.Query()
	for key, values := range template {
		expected := ""
		if len(values) > 0 {
			expected = values[0]
		}
		actual := query.Get(key)
		if expected == "{identifier}" || expected == "{token}" || expected == "{key}" || expected == "" {
			if actual != "" {
				return actual
			}
			return ""
		}
		if actual != expected {
			return ""
		}
	}
	return firstNonEmptyString(query.Get("token"), query.Get("key"), query.Get("identifier"))
}

func selectSubscriptionClientType(userAgent string, settings SubscriptionSettings) string {
	ua := strings.TrimSpace(userAgent)
	if regexp.MustCompile(`^([Cc]lash-verge|[Cc]lash[-\.]?[Mm]eta|[Ff][Ll][Cc]lash|[Mm]ihomo)`).MatchString(ua) {
		return "clash-meta"
	}
	if regexp.MustCompile(`^([Cc]lash|[Ss]tash)`).MatchString(ua) {
		return "clash"
	}
	if regexp.MustCompile(`^(SFA|SFI|SFM|SFT|[Kk]aring|[Hh]iddify[Nn]ext)`).MatchString(ua) {
		return "sing-box"
	}
	if regexp.MustCompile(`^(SS|SSR|SSD|SSS|Outline|Shadowsocks|SSconf)`).MatchString(ua) {
		return "outline"
	}
	if (settings.UseCustomJSONDefault || settings.UseCustomJSONForV2rayN) && regexp.MustCompile(`^v2rayN/(\d+\.\d+)`).MatchString(ua) {
		if versionAtLeast(firstVersion(ua), "6.40") {
			return "v2ray-json"
		}
	}
	if (settings.UseCustomJSONDefault || settings.UseCustomJSONForV2rayNG) && regexp.MustCompile(`(?i)^v2rayng/(\d+\.\d+)`).MatchString(ua) {
		return "v2ray-json"
	}
	if (settings.UseCustomJSONDefault || settings.UseCustomJSONForHapp) && regexp.MustCompile(`^Happ/(\d+\.\d+\.\d+)`).MatchString(ua) {
		if versionAtLeast(firstVersion(ua), "1.63.1") {
			return "v2ray-json"
		}
	}
	if (settings.UseCustomJSONDefault || settings.UseCustomJSONForStreisand) && strings.HasPrefix(ua, "Streisand") {
		return "v2ray-json"
	}
	return "v2ray"
}

func subscriptionHeaders(user UserDetail, req SubscriptionRenderRequest, settings SubscriptionSettings) map[string]string {
	return map[string]string{
		"content-disposition":     `attachment; filename="` + user.Username + `"`,
		"profile-web-page-url":    req.URL,
		"support-url":             strings.TrimSpace(settings.SubscriptionSupportURL),
		"profile-title":           "base64:" + base64.StdEncoding.EncodeToString([]byte(firstNonEmptyString(settings.SubscriptionProfileTitle, "Subscription"))),
		"profile-update-interval": firstNonEmptyString(settings.SubscriptionUpdateInterval, "12"),
		"subscription-userinfo":   fmt.Sprintf("upload=0; download=%d; total=%d; expire=%d", user.UsedTraffic, int64OrZero(user.DataLimit), int64OrZero(user.Expire)),
	}
}

func subscriptionHTML(user UserDetail, req SubscriptionRenderRequest) string {
	path := req.URL
	if parsed, err := url.Parse(req.URL); err == nil {
		path = strings.TrimRight(parsed.Path, "/")
	}
	usage := path + "/usage"
	return "<!doctype html><html><head><meta charset=\"utf-8\"><title>" + htmlEscape(user.Username) + "</title></head><body><h1>" + htmlEscape(user.Username) + "</h1><p><a href=\"" + htmlEscape(usage) + "\">Usage</a></p></body></html>"
}

func renderClashLikeYAML(username string, links []string, meta bool) string {
	var b strings.Builder
	b.WriteString("proxies:\n")
	for i, link := range links {
		b.WriteString("  - name: ")
		b.WriteString(strconv.Quote(fmt.Sprintf("%s-%d", username, i+1)))
		b.WriteString("\n    type: url-test\n    url: ")
		b.WriteString(strconv.Quote(link))
		b.WriteString("\n")
	}
	b.WriteString("proxy-groups:\n  - name: ")
	b.WriteString(strconv.Quote(username))
	if meta {
		b.WriteString("\n    type: select\n")
	} else {
		b.WriteString("\n    type: url-test\n")
	}
	b.WriteString("    proxies:\n")
	for i := range links {
		b.WriteString("      - ")
		b.WriteString(strconv.Quote(fmt.Sprintf("%s-%d", username, i+1)))
		b.WriteString("\n")
	}
	return b.String()
}

func marshalPretty(value any) (string, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func subscriptionUsageRange(startRaw string, endRaw string) (time.Time, time.Time, error) {
	end := time.Now().UTC()
	start := end.Add(-30 * 24 * time.Hour)
	if strings.TrimSpace(startRaw) != "" {
		parsed, err := parseSubscriptionTime(startRaw)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		start = parsed
	}
	if strings.TrimSpace(endRaw) != "" {
		parsed, err := parseSubscriptionTime(endRaw)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		end = parsed
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid range")
	}
	return start, end, nil
}

func parseSubscriptionTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999999", "2006-01-02T15:04:05", "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time")
}

func parseDBTime(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case nil:
		return time.Time{}, false
	case time.Time:
		return typed.UTC(), true
	case string:
		parsed, err := parseSubscriptionTime(typed)
		return parsed, err == nil
	case []byte:
		parsed, err := parseSubscriptionTime(string(typed))
		return parsed, err == nil
	default:
		parsed, err := parseSubscriptionTime(fmt.Sprint(typed))
		return parsed, err == nil
	}
}

func normalizeSubscriptionKey(value string) (string, bool) {
	cleaned := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", ""))
	return cleaned, len(cleaned) == 32 && isHexString(cleaned)
}

func firstVersion(value string) string {
	re := regexp.MustCompile(`(\d+(?:\.\d+){1,2})`)
	match := re.FindStringSubmatch(value)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func versionAtLeast(value string, minimum string) bool {
	left := versionParts(value)
	right := versionParts(minimum)
	for len(left) < len(right) {
		left = append(left, 0)
	}
	for len(right) < len(left) {
		right = append(right, 0)
	}
	for i := range left {
		if left[i] > right[i] {
			return true
		}
		if left[i] < right[i] {
			return false
		}
	}
	return true
}

func versionParts(value string) []int {
	parts := strings.Split(value, ".")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		n, _ := strconv.Atoi(part)
		result = append(result, n)
	}
	return result
}

func sameUTCDate(left time.Time, right time.Time) bool {
	l := left.UTC()
	r := right.UTC()
	return l.Year() == r.Year() && l.YearDay() == r.YearDay()
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case json.Number:
		n, _ := typed.Int64()
		return n
	default:
		n, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
		return n
	}
}

func htmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;")
	return replacer.Replace(value)
}
