package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/rebeccapanel/rebecca/go/internal/platform/db"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

const (
	envAdminPassword = "REBECCA_ADMIN_PASSWORD"
	envDatabaseURL   = "SQLALCHEMY_DATABASE_URL"
)

type cli struct {
	db      *sql.DB
	dialect string
	stdin   *bufio.Reader
}

type adminRecord struct {
	ID         int64
	Username   string
	Role       string
	CreatedAt  sql.NullTime
	TelegramID sql.NullInt64
	Status     string
}

type optionalString struct {
	value string
	set   bool
}

func (o *optionalString) String() string {
	return o.value
}

func (o *optionalString) Set(value string) error {
	o.value = value
	o.set = true
	return nil
}

func main() {
	loadEnvFiles()

	app, err := newCLI()
	if err != nil {
		exitErr(err)
	}
	defer app.db.Close()

	if err := app.run(os.Args[1:]); err != nil {
		exitErr(err)
	}
}

func newCLI() (*cli, error) {
	databaseURL := strings.TrimSpace(os.Getenv(envDatabaseURL))
	if databaseURL == "" {
		databaseURL = "sqlite:///db.sqlite3"
	}
	pool, err := db.Open(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return &cli{db: pool.DB, dialect: pool.Dialect, stdin: bufio.NewReader(os.Stdin)}, nil
}

func (c *cli) run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "admin":
		return c.runAdmin(args[1:])
	case "user":
		return c.runUser(args[1:])
	case "subscription":
		return c.runSubscription(args[1:])
	case "completion":
		fmt.Println("Shell completion is not needed by the Go CLI yet.")
		return nil
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (c *cli) runAdmin(args []string) error {
	if len(args) == 0 {
		printAdminUsage()
		return nil
	}
	switch args[0] {
	case "list":
		return c.adminList(args[1:])
	case "create":
		return c.adminCreate(args[1:])
	case "update":
		return c.adminUpdate(args[1:])
	case "change-role":
		return c.adminChangeRole(args[1:])
	case "delete":
		return c.adminDelete(args[1:])
	case "import-from-env":
		return c.adminImportFromEnv(args[1:])
	case "-h", "--help", "help":
		printAdminUsage()
		return nil
	default:
		return fmt.Errorf("unknown admin command %q", args[0])
	}
}

func (c *cli) adminList(args []string) error {
	fs := newFlagSet("admin list")
	var username string
	var limit int
	var offset int
	fs.StringVar(&username, "username", "", "search by username")
	fs.StringVar(&username, "u", "", "search by username")
	fs.IntVar(&limit, "limit", 0, "limit")
	fs.IntVar(&limit, "l", 0, "limit")
	fs.IntVar(&offset, "offset", 0, "offset")
	fs.IntVar(&offset, "o", 0, "offset")
	if err := fs.Parse(args); err != nil {
		return err
	}

	query := `
SELECT a.id, a.username, a.role, a.created_at, a.telegram_id, a.status,
       COALESCE(a.users_usage, 0),
       COALESCE((SELECT SUM(u.used_traffic) FROM users u WHERE u.admin_id = a.id), 0),
       COALESCE((
           SELECT SUM(ul.used_traffic_at_reset)
           FROM user_usage_logs ul
           JOIN users u2 ON u2.id = ul.user_id
           WHERE u2.admin_id = a.id
       ), 0)
FROM admins a
WHERE a.status != 'deleted'`
	params := []any{}
	if strings.TrimSpace(username) != "" {
		query += " AND LOWER(a.username) LIKE ?"
		params = append(params, "%"+strings.ToLower(strings.TrimSpace(username))+"%")
	}
	query += " ORDER BY a.id"
	if limit > 0 {
		query += " LIMIT ?"
		params = append(params, limit)
		if offset > 0 {
			query += " OFFSET ?"
			params = append(params, offset)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := c.db.QueryContext(ctx, query, params...)
	if err != nil {
		return err
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Username\tUsage\tReseted usage\tUsers Usage\tRole\tCreated at\tTelegram ID\tStatus")
	for rows.Next() {
		var admin adminRecord
		var usersUsage, usage, resetedUsage int64
		if err := rows.Scan(
			&admin.ID,
			&admin.Username,
			&admin.Role,
			&admin.CreatedAt,
			&admin.TelegramID,
			&admin.Status,
			&usersUsage,
			&usage,
			&resetedUsage,
		); err != nil {
			return err
		}
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			admin.Username,
			readableSize(usage),
			readableSize(resetedUsage),
			readableSize(usersUsage),
			admin.Role,
			formatTime(admin.CreatedAt),
			formatNullInt(admin.TelegramID),
			admin.Status,
		)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return w.Flush()
}

func (c *cli) adminCreate(args []string) error {
	fs := newFlagSet("admin create")
	var username string
	var roleValue string
	var password string
	var telegramID optionalString
	fs.StringVar(&username, "username", "", "admin username")
	fs.StringVar(&username, "u", "", "admin username")
	fs.StringVar(&roleValue, "role", "", "admin role")
	fs.StringVar(&password, "password", "", "admin password")
	fs.Var(&telegramID, "telegram-id", "telegram id")
	fs.Var(&telegramID, "tg", "telegram id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if username == "" {
		username = c.mustPrompt("Username", "")
	}
	role, err := parseRoleOrPrompt(roleValue, c, "")
	if err != nil {
		return err
	}
	if password == "" {
		password = os.Getenv(envAdminPassword)
	}
	if password == "" {
		value, err := c.promptPassword("Password")
		if err != nil {
			return err
		}
		password = value
	}
	if password == "" {
		return errors.New("password cannot be empty")
	}

	telegramValue, err := normalizeTelegramValue(telegramID.value, telegramID.set)
	if err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	permissions, err := rolePermissionsJSON(role)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = c.db.ExecContext(ctx, `
INSERT INTO admins (
    username, hashed_password, created_at, role, permissions, telegram_id,
    subscription_settings, users_usage, lifetime_usage, created_traffic,
    deleted_users_usage, traffic_limit_mode, use_service_traffic_limits,
    show_user_traffic, delete_user_usage_limit_enabled, status
) VALUES (?, ?, ?, ?, ?, ?, '{}', 0, 0, 0, 0, 'used_traffic', ?, 1, 0, 'active')`,
		username,
		hash,
		time.Now().UTC(),
		role,
		permissions,
		telegramValue,
		0,
	)
	if err != nil {
		return err
	}
	fmt.Printf("Admin %q created successfully.\n", username)
	return nil
}

func (c *cli) adminUpdate(args []string) error {
	fs := newFlagSet("admin update")
	var username string
	var roleValue optionalString
	var password optionalString
	var telegramID optionalString
	fs.StringVar(&username, "username", "", "admin username")
	fs.StringVar(&username, "u", "", "admin username")
	fs.Var(&roleValue, "role", "admin role")
	fs.Var(&password, "password", "new password")
	fs.Var(&telegramID, "telegram-id", "telegram id")
	fs.Var(&telegramID, "tg", "telegram id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if username == "" {
		username = c.mustPrompt("Username", "")
	}
	admin, err := c.getAdminByUsername(username)
	if err != nil {
		return err
	}

	if !roleValue.set && !password.set && !telegramID.set {
		fmt.Printf("Editing %q. Press Enter to leave a field unchanged.\n", admin.Username)
		role, changed, err := c.promptRole(admin.Role)
		if err != nil {
			return err
		}
		if changed {
			roleValue = optionalString{value: role, set: true}
		}
		newPassword, err := c.promptPasswordAllowEmpty("New password")
		if err != nil {
			return err
		}
		if newPassword != "" {
			password = optionalString{value: newPassword, set: true}
		}
		currentTelegram := ""
		if admin.TelegramID.Valid {
			currentTelegram = strconv.FormatInt(admin.TelegramID.Int64, 10)
		}
		telegram := c.mustPrompt("Telegram ID (Enter 0 to clear current value)", currentTelegram)
		telegramID = optionalString{value: telegram, set: true}
	}

	updates := []string{}
	params := []any{}
	if roleValue.set {
		role, err := parseRole(roleValue.value)
		if err != nil {
			return err
		}
		if role != admin.Role {
			permissions, err := rolePermissionsJSON(role)
			if err != nil {
				return err
			}
			updates = append(updates, "role = ?", "permissions = ?")
			params = append(params, role, permissions)
			if role == "full_access" {
				updates = append(
					updates,
					"traffic_limit_mode = 'used_traffic'",
					"show_user_traffic = 1",
					"use_service_traffic_limits = 0",
					"delete_user_usage_limit_enabled = 0",
				)
			}
		}
	}
	if password.set && password.value != "" {
		hash, err := hashPassword(password.value)
		if err != nil {
			return err
		}
		updates = append(updates, "hashed_password = ?", "password_reset_at = ?")
		params = append(params, hash, time.Now().UTC())
	}
	if telegramID.set {
		telegramValue, err := normalizeTelegramValue(telegramID.value, true)
		if err != nil {
			return err
		}
		updates = append(updates, "telegram_id = ?")
		params = append(params, telegramValue)
	}
	if len(updates) == 0 {
		fmt.Printf("Admin %q is unchanged.\n", admin.Username)
		return nil
	}
	params = append(params, admin.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = c.db.ExecContext(ctx, "UPDATE admins SET "+strings.Join(updates, ", ")+" WHERE id = ?", params...)
	if err != nil {
		return err
	}
	fmt.Printf("Admin %q updated successfully.\n", admin.Username)
	return nil
}

func (c *cli) adminChangeRole(args []string) error {
	fs := newFlagSet("admin change-role")
	var username string
	var roleValue string
	var yes bool
	fs.StringVar(&username, "username", "", "admin username")
	fs.StringVar(&username, "u", "", "admin username")
	fs.StringVar(&roleValue, "role", "", "target role")
	fs.BoolVar(&yes, "yes", false, "skip confirmations")
	fs.BoolVar(&yes, "y", false, "skip confirmations")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if username == "" {
		username = c.mustPrompt("Username", "")
	}
	admin, err := c.getAdminByUsername(username)
	if err != nil {
		return err
	}
	role, err := parseRoleOrPrompt(roleValue, c, admin.Role)
	if err != nil {
		return err
	}
	if role == admin.Role {
		fmt.Printf("Admin %q is already %s.\n", admin.Username, role)
		return nil
	}
	if !yes && !c.confirm(fmt.Sprintf("Change %q role from %s to %s?", admin.Username, admin.Role, role), false) {
		return errors.New("operation aborted")
	}
	return c.adminUpdate([]string{"--username", username, "--role", role})
}

func (c *cli) adminDelete(args []string) error {
	fs := newFlagSet("admin delete")
	var username string
	var yes bool
	fs.StringVar(&username, "username", "", "admin username")
	fs.StringVar(&username, "u", "", "admin username")
	fs.BoolVar(&yes, "yes", false, "skip confirmations")
	fs.BoolVar(&yes, "y", false, "skip confirmations")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if username == "" {
		username = c.mustPrompt("Username", "")
	}
	admin, err := c.getAdminByUsername(username)
	if err != nil {
		return err
	}
	if !yes && !c.confirm(fmt.Sprintf("Delete %q?", admin.Username), false) {
		return errors.New("operation aborted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = c.db.ExecContext(ctx, "UPDATE admins SET status = 'deleted' WHERE id = ?", admin.ID)
	if err != nil {
		return err
	}
	fmt.Printf("Admin %q deleted successfully.\n", admin.Username)
	return nil
}

func (c *cli) adminImportFromEnv(args []string) error {
	fs := newFlagSet("admin import-from-env")
	var yes bool
	fs.BoolVar(&yes, "yes", false, "skip confirmations")
	fs.BoolVar(&yes, "y", false, "skip confirmations")
	if err := fs.Parse(args); err != nil {
		return err
	}
	username := strings.TrimSpace(os.Getenv("SUDO_USERNAME"))
	password := os.Getenv("SUDO_PASSWORD")
	if username == "" || password == "" {
		return errors.New("SUDO_USERNAME and SUDO_PASSWORD must be set")
	}

	admin, err := c.getAdminByUsername(username)
	if err == nil && admin.ID > 0 {
		if !yes && !c.confirm(fmt.Sprintf("Admin %q already exists. Sync it with env?", username), false) {
			return errors.New("operation aborted")
		}
		if err := c.adminUpdate([]string{"--username", username, "--role", "full_access", "--password", password}); err != nil {
			return err
		}
	} else {
		if err := c.adminCreate([]string{"--username", username, "--role", "full_access", "--password", password}); err != nil {
			return err
		}
		admin, err = c.getAdminByUsername(username)
		if err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := c.db.ExecContext(ctx, "UPDATE users SET admin_id = ? WHERE admin_id IS NULL", admin.ID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	fmt.Printf("Admin %q imported successfully. %d users linked.\n", username, count)
	return nil
}

func (c *cli) runUser(args []string) error {
	if len(args) == 0 {
		printUserUsage()
		return nil
	}
	switch args[0] {
	case "list":
		return c.userList(args[1:])
	case "set-owner":
		return c.userSetOwner(args[1:])
	case "-h", "--help", "help":
		printUserUsage()
		return nil
	default:
		return fmt.Errorf("unknown user command %q", args[0])
	}
}

func (c *cli) userList(args []string) error {
	fs := newFlagSet("user list")
	var username string
	var search string
	var status string
	var adminName string
	var limit int
	var offset int
	fs.StringVar(&username, "username", "", "search by username")
	fs.StringVar(&username, "u", "", "search by username")
	fs.StringVar(&search, "search", "", "search by username or note")
	fs.StringVar(&search, "s", "", "search by username or note")
	fs.StringVar(&status, "status", "", "status")
	fs.StringVar(&adminName, "admin", "", "owner admin")
	fs.StringVar(&adminName, "owner", "", "owner admin")
	fs.IntVar(&limit, "limit", 0, "limit")
	fs.IntVar(&limit, "l", 0, "limit")
	fs.IntVar(&offset, "offset", 0, "offset")
	fs.IntVar(&offset, "o", 0, "offset")
	if err := fs.Parse(args); err != nil {
		return err
	}

	query := `
SELECT u.id, u.username, u.status, COALESCE(u.used_traffic, 0), u.data_limit,
       u.data_limit_reset_strategy, u.expire, COALESCE(a.username, '')
FROM users u
LEFT JOIN admins a ON a.id = u.admin_id
WHERE 1 = 1`
	params := []any{}
	if username != "" {
		query += " AND LOWER(u.username) LIKE ?"
		params = append(params, "%"+strings.ToLower(username)+"%")
	}
	if search != "" {
		query += " AND (LOWER(u.username) LIKE ? OR LOWER(COALESCE(u.note, '')) LIKE ?)"
		needle := "%" + strings.ToLower(search) + "%"
		params = append(params, needle, needle)
	}
	if status != "" {
		query += " AND u.status = ?"
		params = append(params, status)
	}
	if adminName != "" {
		query += " AND LOWER(a.username) = ?"
		params = append(params, strings.ToLower(adminName))
	}
	query += " ORDER BY u.id"
	if limit > 0 {
		query += " LIMIT ?"
		params = append(params, limit)
		if offset > 0 {
			query += " OFFSET ?"
			params = append(params, offset)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := c.db.QueryContext(ctx, query, params...)
	if err != nil {
		return err
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUsername\tStatus\tUsed traffic\tData limit\tReset strategy\tExpires at\tOwner")
	for rows.Next() {
		var id int64
		var name, userStatus, resetStrategy, owner string
		var used int64
		var dataLimit sql.NullInt64
		var expire sql.NullInt64
		if err := rows.Scan(&id, &name, &userStatus, &used, &dataLimit, &resetStrategy, &expire, &owner); err != nil {
			return err
		}
		limitText := "Unlimited"
		if dataLimit.Valid && dataLimit.Int64 > 0 {
			limitText = readableSize(dataLimit.Int64)
		}
		expireText := "-"
		if expire.Valid && expire.Int64 > 0 {
			expireText = time.Unix(expire.Int64, 0).Format("02 January 2006")
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", id, name, userStatus, readableSize(used), limitText, resetStrategy, expireText, owner)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return w.Flush()
}

func (c *cli) userSetOwner(args []string) error {
	fs := newFlagSet("user set-owner")
	var username string
	var adminName string
	var yes bool
	fs.StringVar(&username, "username", "", "username")
	fs.StringVar(&username, "u", "", "username")
	fs.StringVar(&adminName, "admin", "", "admin username")
	fs.StringVar(&adminName, "owner", "", "admin username")
	fs.BoolVar(&yes, "yes", false, "skip confirmations")
	fs.BoolVar(&yes, "y", false, "skip confirmations")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if username == "" {
		username = c.mustPrompt("Username", "")
	}
	if adminName == "" {
		adminName = c.mustPrompt("Admin", "")
	}
	admin, err := c.getAdminByUsername(adminName)
	if err != nil {
		return err
	}

	var userID int64
	var oldOwner sql.NullString
	err = c.db.QueryRow(`
SELECT u.id, a.username
FROM users u
LEFT JOIN admins a ON a.id = u.admin_id
WHERE LOWER(u.username) = LOWER(?)`, username).Scan(&userID, &oldOwner)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("user %q not found", username)
	}
	if err != nil {
		return err
	}
	if oldOwner.Valid && oldOwner.String != "" && !yes && !c.confirm(fmt.Sprintf("%s's current owner is %q. Transfer to %q?", username, oldOwner.String, adminName), false) {
		return errors.New("operation aborted")
	}
	_, err = c.db.Exec("UPDATE users SET admin_id = ? WHERE id = ?", admin.ID, userID)
	if err != nil {
		return err
	}
	fmt.Printf("%s's owner successfully set to %q.\n", username, admin.Username)
	return nil
}

func (c *cli) runSubscription(args []string) error {
	if len(args) == 0 {
		printSubscriptionUsage()
		return nil
	}
	switch args[0] {
	case "get-link":
		return c.subscriptionGetLink(args[1:])
	case "get-config":
		return errors.New("subscription get-config is still served by the Python runtime; use the API for generated configs")
	case "-h", "--help", "help":
		printSubscriptionUsage()
		return nil
	default:
		return fmt.Errorf("unknown subscription command %q", args[0])
	}
}

func (c *cli) subscriptionGetLink(args []string) error {
	fs := newFlagSet("subscription get-link")
	var username string
	fs.StringVar(&username, "username", "", "username")
	fs.StringVar(&username, "u", "", "username")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if username == "" {
		username = c.mustPrompt("Username", "")
	}
	var credentialKey, subadress sql.NullString
	err := c.db.QueryRow("SELECT credential_key, subadress FROM users WHERE LOWER(username) = LOWER(?)", username).Scan(&credentialKey, &subadress)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("user %q not found", username)
	}
	if err != nil {
		return err
	}
	prefix := strings.TrimRight(os.Getenv("XRAY_SUBSCRIPTION_URL_PREFIX"), "/")
	if prefix == "" {
		prefix = "/sub"
	}
	if subadress.Valid && strings.TrimSpace(subadress.String) != "" {
		fmt.Println(prefix + "/" + strings.TrimSpace(subadress.String))
		return nil
	}
	if credentialKey.Valid && strings.TrimSpace(credentialKey.String) != "" {
		fmt.Println(prefix + "/" + strings.TrimSpace(credentialKey.String))
		return nil
	}
	return fmt.Errorf("user %q does not have a subscription key", username)
}

func (c *cli) getAdminByUsername(username string) (adminRecord, error) {
	var admin adminRecord
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := c.db.QueryRowContext(ctx, `
SELECT id, username, role, created_at, telegram_id, status
FROM admins
WHERE LOWER(username) = LOWER(?) AND status != 'deleted'
LIMIT 1`, username).Scan(
		&admin.ID,
		&admin.Username,
		&admin.Role,
		&admin.CreatedAt,
		&admin.TelegramID,
		&admin.Status,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return adminRecord{}, fmt.Errorf("admin %q not found", username)
	}
	return admin, err
}

func parseRoleOrPrompt(value string, c *cli, current string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return parseRole(value)
	}
	role, _, err := c.promptRole(current)
	return role, err
}

func parseRole(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "1":
		return "standard", nil
	case "2":
		return "reseller", nil
	case "3":
		return "sudo", nil
	case "4":
		return "full_access", nil
	case "standard", "reseller", "sudo", "full_access":
		return value, nil
	default:
		return "", fmt.Errorf("role must be one of: standard, reseller, sudo, full_access")
	}
}

func (c *cli) promptRole(current string) (string, bool, error) {
	roles := []string{"standard", "reseller", "sudo", "full_access"}
	fmt.Println("Available roles:")
	for i, role := range roles {
		marker := ""
		if role == current {
			marker = " (current)"
		}
		fmt.Printf("  %d) %s%s\n", i+1, role, marker)
	}
	defaultChoice := "1"
	for i, role := range roles {
		if role == current {
			defaultChoice = strconv.Itoa(i + 1)
			break
		}
	}
	choice := c.mustPrompt("Select role", defaultChoice)
	role, err := parseRole(choice)
	if err != nil {
		return "", false, err
	}
	return role, role != current, nil
}

func rolePermissionsJSON(role string) (string, error) {
	users := map[string]any{
		"create":                  true,
		"delete":                  false,
		"reset_usage":             false,
		"revoke":                  true,
		"create_on_hold":          true,
		"allow_unlimited_data":    true,
		"allow_unlimited_expire":  true,
		"allow_next_plan":         true,
		"advanced_actions":        true,
		"set_flow":                false,
		"allow_custom_key":        false,
		"max_data_limit_per_user": nil,
	}
	adminManagement := map[string]any{
		"can_view":        false,
		"can_edit":        false,
		"can_manage_sudo": false,
	}
	sections := map[string]any{
		"usage":        false,
		"admins":       false,
		"services":     false,
		"hosts":        false,
		"nodes":        false,
		"integrations": false,
		"xray":         false,
	}
	if role == "sudo" || role == "full_access" {
		users["set_flow"] = true
		users["allow_custom_key"] = true
		adminManagement["can_view"] = true
		adminManagement["can_edit"] = true
		for key := range sections {
			sections[key] = true
		}
	}
	if role == "full_access" {
		users["delete"] = true
		users["reset_usage"] = true
		adminManagement["can_manage_sudo"] = true
	}
	payload := map[string]any{
		"users":            users,
		"admin_management": adminManagement,
		"sections":         sections,
		"self_permissions": map[string]bool{
			"self_myaccount":       true,
			"self_change_password": true,
			"self_api_keys":        true,
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func normalizeTelegramValue(value string, wasSet bool) (any, error) {
	if !wasSet {
		return nil, nil
	}
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return nil, errors.New("telegram id must be a positive integer")
	}
	return parsed, nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(hash), err
}

func (c *cli) mustPrompt(label string, defaultValue string) string {
	value, err := c.prompt(label, defaultValue)
	if err != nil {
		exitErr(err)
	}
	return value
}

func (c *cli) prompt(label string, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", label, defaultValue)
	} else {
		fmt.Printf("%s: ", label)
	}
	value, err := c.stdin.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

func (c *cli) promptPassword(label string) (string, error) {
	value, err := readPassword(label + ": ")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func (c *cli) promptPasswordAllowEmpty(label string) (string, error) {
	value, err := readPassword(label + ": ")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		bytes, err := term.ReadPassword(fd)
		fmt.Println()
		return string(bytes), err
	}
	reader := bufio.NewReader(os.Stdin)
	value, err := reader.ReadString('\n')
	return value, err
}

func (c *cli) confirm(prompt string, defaultValue bool) bool {
	suffix := "y/N"
	if defaultValue {
		suffix = "Y/n"
	}
	answer := strings.ToLower(c.mustPrompt(prompt+" ["+suffix+"]", ""))
	if answer == "" {
		return defaultValue
	}
	return answer == "y" || answer == "yes"
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func readableSize(value int64) string {
	if value < 0 {
		value = 0
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	amount := float64(value)
	unit := 0
	for amount >= 1024 && unit < len(units)-1 {
		amount /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", value)
	}
	return fmt.Sprintf("%.2f %s", amount, units[unit])
}

func formatTime(value sql.NullTime) string {
	if !value.Valid {
		return "-"
	}
	return value.Time.Format("02 January 2006, 15:04:05")
}

func formatNullInt(value sql.NullInt64) string {
	if !value.Valid || value.Int64 == 0 {
		return "-"
	}
	return strconv.FormatInt(value.Int64, 10)
}

func boolToDB(value bool) int {
	if value {
		return 1
	}
	return 0
}

func printUsage() {
	fmt.Println("Usage: rebecca cli <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  admin          Manage admins")
	fmt.Println("  user           Manage users")
	fmt.Println("  subscription   Subscription helpers")
}

func printAdminUsage() {
	fmt.Println("Usage: rebecca cli admin <command> [options]")
	fmt.Println("Commands: list, create, update, change-role, delete, import-from-env")
}

func printUserUsage() {
	fmt.Println("Usage: rebecca cli user <command> [options]")
	fmt.Println("Commands: list, set-owner")
}

func printSubscriptionUsage() {
	fmt.Println("Usage: rebecca cli subscription <command> [options]")
	fmt.Println("Commands: get-link, get-config")
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func loadEnvFiles() {
	for _, candidate := range envCandidates() {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			_ = loadEnvFile(candidate)
			return
		}
	}
}

func envCandidates() []string {
	candidates := []string{}
	if explicit := strings.TrimSpace(os.Getenv("REBECCA_ENV_FILE")); explicit != "" {
		candidates = append(candidates, explicit)
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, ".env"), filepath.Join(filepath.Dir(exeDir), ".env"))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".env"))
	}
	return candidates
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key != "" {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}
