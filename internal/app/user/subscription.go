package user

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rebeccapanel/rebecca/internal/app/usage"
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
		settings := s.effectiveSettings(ctx, user.AdminID)
		html, err := s.renderSubscriptionHTML(ctx, user, req, settings)
		if err != nil {
			return SubscriptionHTTPResponse{}, err
		}
		return SubscriptionHTTPResponse{
			Status:    200,
			MediaType: "text/html; charset=utf-8",
			Body:      []byte(html),
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
		`SELECT username FROM users WHERE credential_key = ? AND status != 'deleted' ORDER BY created_at DESC, id DESC LIMIT 1`,
		normalized,
	).Scan(&username)
	if err == nil {
		return r.subscriptionUserByUsername(ctx, username)
	}
	if err != sql.ErrNoRows {
		return UserDetail{}, err
	}
	err = r.db.QueryRowContext(
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
		`SELECT username FROM users WHERE subadress = ? AND status != 'deleted' ORDER BY created_at DESC, id DESC LIMIT 2`,
		subadress,
	)
	if err != nil {
		return UserDetail{}, err
	}
	usernames, err := scanSubscriptionUsernames(rows)
	if err != nil {
		return UserDetail{}, err
	}
	if len(usernames) != 1 {
		rows, err = r.db.QueryContext(
			ctx,
			`SELECT username FROM users WHERE subadress != '' AND LOWER(subadress) = LOWER(?) AND status != 'deleted' ORDER BY created_at DESC, id DESC LIMIT 2`,
			subadress,
		)
		if err != nil {
			return UserDetail{}, err
		}
		usernames, err = scanSubscriptionUsernames(rows)
		if err != nil {
			return UserDetail{}, err
		}
		if len(usernames) != 1 {
			return UserDetail{}, clientError(404, "Not Found")
		}
	}
	return r.subscriptionUserByUsername(ctx, usernames[0])
}

func scanSubscriptionUsernames(rows *sql.Rows) ([]string, error) {
	defer rows.Close()
	usernames := []string{}
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			return nil, err
		}
		usernames = append(usernames, username)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return usernames, nil
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

func (s Service) renderSubscriptionHTML(ctx context.Context, user UserDetail, req SubscriptionRenderRequest, settings SubscriptionSettings) (string, error) {
	links, err := s.ConfigLinks(ctx, ConfigLinksRequest{UserID: user.ID})
	if err != nil {
		return "", err
	}
	path := req.URL
	if parsed, err := url.Parse(req.URL); err == nil {
		path = strings.TrimRight(parsed.Path, "/")
	}
	data := subscriptionHTMLData{
		Username:       user.Username,
		Status:         user.Status,
		StatusClass:    subscriptionStatusClass(user.Status),
		DataLimit:      subscriptionBytes(user.DataLimit),
		DataUsed:       formatBytes(user.UsedTraffic),
		ResetStrategy:  user.DataLimitResetStrategy,
		Expire:         subscriptionExpire(user.Expire),
		RemainingDays:  subscriptionRemainingDays(user.Expire),
		Links:          links.Links,
		UsageURL:       path + "/usage",
		SupportURL:     strings.TrimSpace(settings.SubscriptionSupportURL),
		HasActiveLinks: user.Status == "active" && len(links.Links) > 0,
	}
	var out bytes.Buffer
	if err := subscriptionPageTemplate.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func renderClashLikeYAML(username string, links []string, meta bool) string {
	var b strings.Builder
	proxyNames := make([]string, 0, len(links))
	b.WriteString("proxies:\n")
	for i, link := range links {
		name := fmt.Sprintf("%s-%d", username, i+1)
		proxy, ok := clashProxyFromShareLink(name, link)
		if !ok {
			continue
		}
		proxyNames = append(proxyNames, name)
		writeClashProxy(&b, proxy)
	}
	b.WriteString("proxy-groups:\n  - name: ")
	b.WriteString(yamlQuote("♻️ Automatic"))
	b.WriteString("\n    type: url-test\n    url: http://www.gstatic.com/generate_204\n    interval: 300\n")
	b.WriteString("    proxies:\n")
	for _, name := range proxyNames {
		b.WriteString("      - ")
		b.WriteString(yamlQuote(name))
		b.WriteString("\n")
	}
	b.WriteString("  - name: ")
	b.WriteString(yamlQuote(username))
	if meta {
		b.WriteString("\n    type: select\n")
	} else {
		b.WriteString("\n    type: url-test\n    url: http://www.gstatic.com/generate_204\n    interval: 300\n")
	}
	b.WriteString("    proxies:\n      - ")
	b.WriteString(yamlQuote("♻️ Automatic"))
	b.WriteString("\n")
	for _, name := range proxyNames {
		b.WriteString("      - ")
		b.WriteString(yamlQuote(name))
		b.WriteString("\n")
	}
	b.WriteString("rules:\n  - MATCH,")
	b.WriteString(yamlQuote(username))
	b.WriteString("\n")
	return b.String()
}

type subscriptionHTMLData struct {
	Username       string
	Status         string
	StatusClass    string
	DataLimit      string
	DataUsed       string
	ResetStrategy  string
	Expire         string
	RemainingDays  string
	Links          []string
	UsageURL       string
	SupportURL     string
	HasActiveLinks bool
}

var subscriptionPageTemplate = template.Must(template.New("subscription_page").Parse(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Subscription Information</title>
    <style>
        body { font-family: Arial, sans-serif; padding: 20px; }
        h1 { margin-top: 0; }
        .link-input { margin-bottom: 10px; }
        .copy-button { margin-left: 10px; }
        .status { display: inline-block; padding: 3px 8px; border-radius: 3px; font-weight: bold; font-size: 16px; line-height: 1; }
        .active { background-color: #4CAF50; color: white; }
        .limited { background-color: #F44336; color: white; }
        .expired { background-color: #FF9800; color: white; }
        .disabled { background-color: #9E9E9E; color: white; }
        .qr-popup { position: fixed; top: 50%; left: 50%; transform: translate(-50%, -50%); background-color: white; padding: 10px 25px 25px 25px; border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,.1); display: none; z-index: 9999; }
        .qr-close-button { text-align: right; margin-bottom: 5px; margin-right: -15px; }
        input[type=text] { width: min(900px, 80vw); }
    </style>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/qrcodejs/1.0.0/qrcode.min.js"></script>
</head>
<body>
    <h1>User Information</h1>
    <p>Username: {{ .Username }}</p>
    <p>Status: <span class="status {{ .StatusClass }}">{{ .Status }}</span></p>
    <p>Data Limit: {{ .DataLimit }}</p>
    <p>Data Used: {{ .DataUsed }}{{ if and .ResetStrategy (ne .ResetStrategy "no_reset") }} (resets every {{ .ResetStrategy }}){{ end }}</p>
    <p>Expiration Date: {{ .Expire }}{{ if .RemainingDays }} ({{ .RemainingDays }} days remaining){{ end }}</p>
    <p><a href="{{ .UsageURL }}">Usage</a>{{ if .SupportURL }} · <a href="{{ .SupportURL }}">Support</a>{{ end }}</p>
    {{ if .HasActiveLinks }}
    <h2>Links:</h2>
    <ul>
        {{ range .Links }}
        <li class="link-input">
            <input type="text" value="{{ . }}" readonly>
            <button class="copy-button" onclick="copyLink(this.previousElementSibling.value, this)">Copy</button>
            <button class="qr-button" data-link="{{ . }}">QR Code</button>
        </li>
        {{ end }}
    </ul>
    <div class="qr-popup" id="qrPopup">
        <div class="qr-close-button"><button onclick="closeQrPopup()">X</button></div>
        <div id="qrCodeContainer"></div>
    </div>
    {{ end }}
    <script>
        function copyLink(link, button) {
            const tempInput = document.createElement('input');
            tempInput.setAttribute('value', link);
            document.body.appendChild(tempInput);
            tempInput.select();
            document.execCommand('copy');
            document.body.removeChild(tempInput);
            button.textContent = 'Copied!';
            setTimeout(function () { button.textContent = 'Copy'; }, 1500);
        }
        const qrButtons = document.querySelectorAll('.qr-button');
        const qrPopup = document.getElementById('qrPopup');
        const qrCodeContainer = document.getElementById('qrCodeContainer');
        qrButtons.forEach((qrButton) => {
            qrButton.addEventListener('click', () => {
                const link = qrButton.dataset.link;
                while (qrCodeContainer.firstChild) qrCodeContainer.removeChild(qrCodeContainer.firstChild);
                new QRCode(qrCodeContainer, { text: link, width: 256, height: 256, correctLevel: QRCode.CorrectLevel.L });
                qrPopup.style.display = 'block';
            });
        });
        function closeQrPopup() { document.getElementById('qrPopup').style.display = 'none'; }
    </script>
</body>
</html>`))

func subscriptionStatusClass(status string) string {
	switch status {
	case "active", "limited", "expired", "disabled":
		return status
	default:
		return "disabled"
	}
}

func subscriptionBytes(value *int64) string {
	if value == nil || *value <= 0 {
		return "∞"
	}
	return formatBytes(*value)
}

func subscriptionExpire(value *int64) string {
	if value == nil || *value <= 0 {
		return "∞"
	}
	return time.Unix(*value, 0).UTC().Format("2006-01-02 15:04:05")
}

func subscriptionRemainingDays(value *int64) string {
	if value == nil || *value <= 0 {
		return ""
	}
	days := int64(time.Until(time.Unix(*value, 0).UTC()).Hours() / 24)
	return strconv.FormatInt(days, 10)
}

func formatBytes(value int64) string {
	if value < 0 {
		value = 0
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	size := float64(value)
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	if unit == 0 {
		return strconv.FormatInt(value, 10) + " " + units[unit]
	}
	return strconv.FormatFloat(size, 'f', 2, 64) + " " + units[unit]
}

func clashProxyFromShareLink(name string, link string) (map[string]any, bool) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, false
	}
	switch parsed.Scheme {
	case "ss":
		return clashShadowsocksProxy(name, parsed)
	case "vless":
		return clashVLESSProxy(name, parsed)
	case "trojan":
		return clashTrojanProxy(name, parsed)
	case "vmess":
		return clashVMessProxy(name, parsed)
	default:
		return nil, false
	}
}

func clashShadowsocksProxy(name string, parsed *url.URL) (map[string]any, bool) {
	user := parsed.User.Username()
	if decoded, err := decodeFlexibleBase64(user); err == nil {
		user = string(decoded)
	}
	method, password, ok := strings.Cut(user, ":")
	if !ok || strings.TrimSpace(method) == "" || strings.TrimSpace(password) == "" {
		return nil, false
	}
	port, ok := parseURLPort(parsed)
	if !ok {
		return nil, false
	}
	return map[string]any{
		"name":     name,
		"type":     "ss",
		"server":   parsed.Hostname(),
		"port":     port,
		"cipher":   method,
		"password": password,
		"udp":      true,
	}, true
}

func clashVLESSProxy(name string, parsed *url.URL) (map[string]any, bool) {
	port, ok := parseURLPort(parsed)
	if !ok || strings.TrimSpace(parsed.User.Username()) == "" {
		return nil, false
	}
	query := parsed.Query()
	network := firstNonEmptyString(query.Get("type"), "tcp")
	security := query.Get("security")
	proxy := map[string]any{
		"name":    name,
		"type":    "vless",
		"server":  parsed.Hostname(),
		"port":    port,
		"uuid":    parsed.User.Username(),
		"network": network,
		"udp":     true,
	}
	if security == "tls" || security == "reality" {
		proxy["tls"] = true
	}
	if sni := query.Get("sni"); sni != "" {
		proxy["servername"] = sni
	}
	if fp := query.Get("fp"); fp != "" {
		proxy["client-fingerprint"] = fp
	}
	if flow := query.Get("flow"); flow != "" {
		proxy["flow"] = flow
	}
	if query.Get("allowInsecure") == "1" || strings.EqualFold(query.Get("allowInsecure"), "true") {
		proxy["skip-cert-verify"] = true
	}
	if security == "reality" {
		reality := map[string]any{}
		if value := query.Get("pbk"); value != "" {
			reality["public-key"] = value
		}
		if value := query.Get("sid"); value != "" {
			reality["short-id"] = value
		}
		if value := query.Get("spx"); value != "" {
			reality["spider-x"] = value
		}
		if len(reality) > 0 {
			proxy["reality-opts"] = reality
		}
	}
	appendClashNetworkOptions(proxy, network, query)
	return proxy, true
}

func clashTrojanProxy(name string, parsed *url.URL) (map[string]any, bool) {
	port, ok := parseURLPort(parsed)
	if !ok || strings.TrimSpace(parsed.User.Username()) == "" {
		return nil, false
	}
	query := parsed.Query()
	network := firstNonEmptyString(query.Get("type"), "tcp")
	proxy := map[string]any{
		"name":     name,
		"type":     "trojan",
		"server":   parsed.Hostname(),
		"port":     port,
		"password": parsed.User.Username(),
		"network":  network,
		"udp":      true,
	}
	if query.Get("security") == "tls" || query.Get("security") == "reality" {
		proxy["tls"] = true
	}
	if sni := query.Get("sni"); sni != "" {
		proxy["sni"] = sni
	}
	if query.Get("allowInsecure") == "1" || strings.EqualFold(query.Get("allowInsecure"), "true") {
		proxy["skip-cert-verify"] = true
	}
	appendClashNetworkOptions(proxy, network, query)
	return proxy, true
}

func clashVMessProxy(name string, parsed *url.URL) (map[string]any, bool) {
	raw := strings.TrimPrefix(parsed.String(), "vmess://")
	decoded, err := decodeFlexibleBase64(raw)
	if err != nil {
		return nil, false
	}
	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, false
	}
	port, err := strconv.Atoi(stringValue(payload["port"]))
	if err != nil || port <= 0 {
		return nil, false
	}
	network := firstNonEmptyString(payload["net"], "tcp")
	proxy := map[string]any{
		"name":    name,
		"type":    "vmess",
		"server":  stringValue(payload["add"]),
		"port":    port,
		"uuid":    stringValue(payload["id"]),
		"alterId": intValue(payload["aid"]),
		"cipher":  firstNonEmptyString(payload["scy"], "auto"),
		"network": network,
		"udp":     true,
	}
	if stringValue(payload["tls"]) == "tls" {
		proxy["tls"] = true
	}
	if sni := firstNonEmptyString(payload["sni"], payload["host"]); sni != "" {
		proxy["servername"] = sni
	}
	if fp := stringValue(payload["fp"]); fp != "" {
		proxy["client-fingerprint"] = fp
	}
	query := url.Values{}
	query.Set("path", stringValue(payload["path"]))
	query.Set("host", stringValue(payload["host"]))
	appendClashNetworkOptions(proxy, network, query)
	return proxy, true
}

func appendClashNetworkOptions(proxy map[string]any, network string, query url.Values) {
	switch network {
	case "ws":
		opts := map[string]any{}
		if path := query.Get("path"); path != "" {
			opts["path"] = path
		}
		if host := query.Get("host"); host != "" {
			opts["headers"] = map[string]any{"Host": host}
		}
		if len(opts) > 0 {
			proxy["ws-opts"] = opts
		}
	case "grpc":
		opts := map[string]any{}
		if service := query.Get("serviceName"); service != "" {
			opts["grpc-service-name"] = service
		}
		if len(opts) > 0 {
			proxy["grpc-opts"] = opts
		}
	case "http":
		opts := map[string]any{}
		if host := query.Get("host"); host != "" {
			opts["headers"] = map[string]any{"Host": []string{host}}
		}
		if path := query.Get("path"); path != "" {
			opts["path"] = []string{path}
		}
		if len(opts) > 0 {
			proxy["http-opts"] = opts
		}
	}
}

func writeClashProxy(b *strings.Builder, proxy map[string]any) {
	order := []string{
		"name", "type", "server", "port", "cipher", "password", "uuid", "alterId",
		"tls", "servername", "sni", "skip-cert-verify", "client-fingerprint",
		"flow", "network", "udp", "ws-opts", "grpc-opts", "http-opts", "reality-opts",
	}
	b.WriteString("  - ")
	first := true
	for _, key := range order {
		value, ok := proxy[key]
		if !ok || isEmptyYAMLValue(value) {
			continue
		}
		if first {
			b.WriteString(key)
			b.WriteString(": ")
			writeYAMLInlineValue(b, value)
			b.WriteString("\n")
			first = false
			continue
		}
		b.WriteString("    ")
		b.WriteString(key)
		b.WriteString(":")
		writeYAMLValue(b, value, 4)
	}
}

func writeYAMLValue(b *strings.Builder, value any, indent int) {
	switch typed := value.(type) {
	case map[string]any:
		b.WriteString("\n")
		writeYAMLMap(b, typed, indent+2)
	case map[string]string:
		b.WriteString("\n")
		mapped := make(map[string]any, len(typed))
		for key, value := range typed {
			mapped[key] = value
		}
		writeYAMLMap(b, mapped, indent+2)
	default:
		b.WriteString(" ")
		writeYAMLInlineValue(b, value)
		b.WriteString("\n")
	}
}

func writeYAMLMap(b *strings.Builder, values map[string]any, indent int) {
	keys := []string{"path", "headers", "Host", "grpc-service-name", "public-key", "short-id", "spider-x"}
	seen := map[string]bool{}
	for _, key := range keys {
		if value, ok := values[key]; ok && !isEmptyYAMLValue(value) {
			writeYAMLMapItem(b, key, value, indent)
			seen[key] = true
		}
	}
	for key, value := range values {
		if seen[key] || isEmptyYAMLValue(value) {
			continue
		}
		writeYAMLMapItem(b, key, value, indent)
	}
}

func writeYAMLMapItem(b *strings.Builder, key string, value any, indent int) {
	b.WriteString(strings.Repeat(" ", indent))
	b.WriteString(key)
	b.WriteString(":")
	writeYAMLValue(b, value, indent)
}

func writeYAMLInlineValue(b *strings.Builder, value any) {
	switch typed := value.(type) {
	case bool:
		if typed {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case int:
		b.WriteString(strconv.Itoa(typed))
	case int64:
		b.WriteString(strconv.FormatInt(typed, 10))
	case []string:
		b.WriteString("[")
		for index, item := range typed {
			if index > 0 {
				b.WriteString(", ")
			}
			b.WriteString(yamlQuote(item))
		}
		b.WriteString("]")
	default:
		b.WriteString(yamlQuote(stringValue(typed)))
	}
}

func yamlQuote(value string) string {
	return strconv.Quote(value)
}

func isEmptyYAMLValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []string:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func parseURLPort(parsed *url.URL) (int, bool) {
	port, err := strconv.Atoi(parsed.Port())
	return port, err == nil && port > 0
}

func decodeFlexibleBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	return base64.RawURLEncoding.DecodeString(value)
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
