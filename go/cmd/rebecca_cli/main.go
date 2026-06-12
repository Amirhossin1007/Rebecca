package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/rebeccapanel/rebecca/go/internal/app/migrations"
	"github.com/rebeccapanel/rebecca/go/internal/platform/db"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/curve25519"
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

type subscriptionUser struct {
	ID            int64
	Username      string
	CredentialKey sql.NullString
	Subadress     sql.NullString
	Flow          sql.NullString
	Status        string
	UsedTraffic   int64
	DataLimit     sql.NullInt64
	Expire        sql.NullInt64
	ServiceID     sql.NullInt64
}

type proxyRecord struct {
	Type     string
	Settings map[string]any
}

type inboundInfo struct {
	Tag             string
	Protocol        string
	Port            any
	Network         string
	TLS             string
	SNI             []string
	Host            []string
	Path            string
	HeaderType      string
	Fingerprint     string
	ALPN            string
	PublicKey       string
	ShortIDs        []string
	SpiderX         string
	AllowInsecure   bool
	Encryption      string
	Heartbeat       int64
	MultiMode       bool
	Fragment        string
	RandomUserAgent bool
}

type hostInfo struct {
	ID              int64
	Remark          string
	Address         []string
	Port            sql.NullInt64
	Path            sql.NullString
	SNI             []string
	Host            []string
	TLS             sql.NullString
	ALPN            string
	Fingerprint     string
	InboundTag      string
	AllowInsecure   bool
	IsDisabled      bool
	MuxEnable       bool
	FragmentSetting sql.NullString
	RandomUserAgent bool
	UseSNIAsHost    bool
	Sort            int64
	ServiceSort     int64
}

type generatedNode struct {
	Remark   string
	Protocol string
	Address  string
	Port     string
	Network  string
	TLS      string
	SNI      string
	Host     string
	Path     string
	Header   string
	Settings map[string]any
	Inbound  inboundInfo
	Mux      bool
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
	case "migrate":
		return c.runMigrate(args[1:])
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

func (c *cli) runMigrate(args []string) error {
	if len(args) == 0 {
		printMigrateUsage()
		return nil
	}
	ctx := context.Background()
	switch args[0] {
	case "up":
		fs := flag.NewFlagSet("migrate up", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		targetVersion := fs.Int64("to", 0, "target goose migration version")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *targetVersion > 0 {
			if err := migrations.RunMigrationsTo(ctx, c.db, c.dialect, *targetVersion); err != nil {
				return err
			}
		} else if err := migrations.RunMigrations(ctx, c.db, c.dialect); err != nil {
			return err
		}
		version, err := migrations.Version(ctx, c.db, c.dialect)
		if err != nil {
			return err
		}
		fmt.Printf("Migrations applied. goose version: %d\n", version.GooseVersion)
		if version.HasAlembic {
			fmt.Printf("Legacy Alembic revision detected: %s\n", version.AlembicRevision)
		}
		return nil
	case "status":
		status, err := migrations.Status(ctx, c.db, c.dialect)
		if err != nil {
			return err
		}
		fmt.Printf("Dialect: %s\n", status.Version.Dialect)
		if status.Version.HasGoose {
			fmt.Printf("Goose version: %d\n", status.Version.GooseVersion)
		} else {
			fmt.Println("Goose version: not initialized")
		}
		if status.Version.HasAlembic {
			fmt.Printf("Alembic revision: %s\n", status.Version.AlembicRevision)
		}
		if status.Message != "" {
			fmt.Println(status.Message)
		}
		return nil
	case "down", "downgrade":
		return migrations.UnsupportedDowngrade()
	case "-h", "--help", "help":
		printMigrateUsage()
		return nil
	default:
		return fmt.Errorf("unknown migrate command %q", args[0])
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
		return c.subscriptionGetConfig(args[1:])
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

func (c *cli) subscriptionGetConfig(args []string) error {
	fs := newFlagSet("subscription get-config")
	var username string
	var format string
	var output string
	var asBase64 bool
	fs.StringVar(&username, "username", "", "username")
	fs.StringVar(&username, "u", "", "username")
	fs.StringVar(&format, "format", "", "config format: v2ray or clash")
	fs.StringVar(&format, "f", "", "config format: v2ray or clash")
	fs.StringVar(&output, "output", "", "write config to file")
	fs.StringVar(&output, "o", "", "write config to file")
	fs.BoolVar(&asBase64, "base64", false, "base64 encode output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if username == "" {
		username = c.mustPrompt("Username", "")
	}
	if format == "" {
		format = c.mustPrompt("Format", "v2ray")
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "clash-meta" {
		format = "clash"
	}
	if format != "v2ray" && format != "clash" {
		return fmt.Errorf("unsupported format %q; expected v2ray or clash", format)
	}

	user, err := c.getSubscriptionUser(username)
	if err != nil {
		return err
	}
	proxies, err := c.getUserProxies(user.ID)
	if err != nil {
		return err
	}
	if len(proxies) == 0 {
		return fmt.Errorf("user %q has no proxies", username)
	}
	inbounds, err := c.loadInbounds()
	if err != nil {
		return err
	}
	hosts, err := c.loadSubscriptionHosts(user.ServiceID)
	if err != nil {
		return err
	}
	nodes, err := c.buildSubscriptionNodes(user, proxies, inbounds, hosts)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return fmt.Errorf("no usable subscription hosts found for user %q", username)
	}

	var config string
	switch format {
	case "v2ray":
		config = strings.Join(buildV2RayLinks(nodes), "\n")
	case "clash":
		config = buildClashConfig(nodes)
	}
	if asBase64 {
		config = base64.StdEncoding.EncodeToString([]byte(config))
	}
	if output != "" {
		return os.WriteFile(output, []byte(config), 0o644)
	}
	fmt.Println(config)
	return nil
}

func (c *cli) getSubscriptionUser(username string) (subscriptionUser, error) {
	var user subscriptionUser
	err := c.db.QueryRow(`
SELECT id, username, credential_key, subadress, flow, status, COALESCE(used_traffic, 0), data_limit, expire, service_id
FROM users
WHERE LOWER(username) = LOWER(?)
LIMIT 1`, username).Scan(
		&user.ID,
		&user.Username,
		&user.CredentialKey,
		&user.Subadress,
		&user.Flow,
		&user.Status,
		&user.UsedTraffic,
		&user.DataLimit,
		&user.Expire,
		&user.ServiceID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return subscriptionUser{}, fmt.Errorf("user %q not found", username)
	}
	return user, err
}

func (c *cli) getUserProxies(userID int64) ([]proxyRecord, error) {
	rows, err := c.db.Query("SELECT type, settings FROM proxies WHERE user_id = ? ORDER BY id", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var proxies []proxyRecord
	for rows.Next() {
		var proxyType string
		var raw any
		if err := rows.Scan(&proxyType, &raw); err != nil {
			return nil, err
		}
		settings := map[string]any{}
		if err := decodeJSON(raw, &settings); err != nil {
			return nil, fmt.Errorf("decode proxy settings for %s: %w", proxyType, err)
		}
		proxies = append(proxies, proxyRecord{Type: proxyType, Settings: settings})
	}
	return proxies, rows.Err()
}

func (c *cli) loadInbounds() (map[string]inboundInfo, error) {
	var raw any
	err := c.db.QueryRow("SELECT data FROM xray_config ORDER BY id LIMIT 1").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]inboundInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	config := map[string]any{}
	if err := decodeJSON(raw, &config); err != nil {
		return nil, err
	}
	return parseInbounds(config), nil
}

func (c *cli) loadSubscriptionHosts(serviceID sql.NullInt64) ([]hostInfo, error) {
	if serviceID.Valid && serviceID.Int64 > 0 {
		return c.loadServiceHosts(serviceID.Int64)
	}
	return c.loadAllHosts()
}

func (c *cli) loadServiceHosts(serviceID int64) ([]hostInfo, error) {
	query := `
SELECT h.id, h.remark, h.address, h.port, h.path, h.sni, h.host, h.security, h.alpn,
       h.fingerprint, h.inbound_tag, h.allowinsecure, h.is_disabled, h.mux_enable,
       h.fragment_setting, h.random_user_agent, h.use_sni_as_host, h.sort, sh.sort
FROM service_hosts sh
JOIN hosts h ON h.id = sh.host_id
WHERE sh.service_id = ? AND (h.is_disabled IS NULL OR h.is_disabled = 0)
ORDER BY sh.sort, h.sort, h.id`
	rows, err := c.db.Query(query, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHosts(rows)
}

func (c *cli) loadAllHosts() ([]hostInfo, error) {
	query := `
SELECT id, remark, address, port, path, sni, host, security, alpn,
       fingerprint, inbound_tag, allowinsecure, is_disabled, mux_enable,
       fragment_setting, random_user_agent, use_sni_as_host, sort, sort
FROM hosts
WHERE is_disabled IS NULL OR is_disabled = 0
ORDER BY sort, id`
	rows, err := c.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHosts(rows)
}

func scanHosts(rows *sql.Rows) ([]hostInfo, error) {
	var hosts []hostInfo
	for rows.Next() {
		var h hostInfo
		var address, sni, host sql.NullString
		var tls, alpn, fingerprint sql.NullString
		var allowInsecure, disabled, mux, randomUA, useSNI sql.NullBool
		if err := rows.Scan(
			&h.ID,
			&h.Remark,
			&address,
			&h.Port,
			&h.Path,
			&sni,
			&host,
			&tls,
			&alpn,
			&fingerprint,
			&h.InboundTag,
			&allowInsecure,
			&disabled,
			&mux,
			&h.FragmentSetting,
			&randomUA,
			&useSNI,
			&h.Sort,
			&h.ServiceSort,
		); err != nil {
			return nil, err
		}
		h.Address = splitCSV(nullStringValue(address))
		h.SNI = splitCSV(nullStringValue(sni))
		h.Host = splitCSV(nullStringValue(host))
		h.TLS = tls
		h.ALPN = normalizeNone(nullStringValue(alpn))
		h.Fingerprint = normalizeNone(nullStringValue(fingerprint))
		h.AllowInsecure = allowInsecure.Valid && allowInsecure.Bool
		h.IsDisabled = disabled.Valid && disabled.Bool
		h.MuxEnable = mux.Valid && mux.Bool
		h.RandomUserAgent = randomUA.Valid && randomUA.Bool
		h.UseSNIAsHost = useSNI.Valid && useSNI.Bool
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

func (c *cli) buildSubscriptionNodes(user subscriptionUser, proxies []proxyRecord, inbounds map[string]inboundInfo, hosts []hostInfo) ([]generatedNode, error) {
	masks, _ := c.getUUIDMasks()
	proxyByProtocol := map[string]proxyRecord{}
	for _, proxy := range proxies {
		proxyByProtocol[proxy.Type] = proxy
	}
	sort.SliceStable(hosts, func(i, j int) bool {
		if hosts[i].ServiceSort != hosts[j].ServiceSort {
			return hosts[i].ServiceSort < hosts[j].ServiceSort
		}
		if hosts[i].Sort != hosts[j].Sort {
			return hosts[i].Sort < hosts[j].Sort
		}
		return hosts[i].ID < hosts[j].ID
	})

	nodes := []generatedNode{}
	for _, host := range hosts {
		if host.IsDisabled {
			continue
		}
		inbound, ok := inbounds[host.InboundTag]
		if !ok {
			continue
		}
		proxy, ok := proxyByProtocol[inbound.Protocol]
		if !ok {
			continue
		}
		settings, err := runtimeSettings(proxy, user, masks)
		if err != nil {
			return nil, err
		}
		address := firstOrEmpty(host.Address)
		if address == "" {
			continue
		}
		sni := firstOrEmpty(firstNonEmptyList(host.SNI, inbound.SNI))
		reqHost := firstOrEmpty(firstNonEmptyList(host.Host, inbound.Host))
		if host.UseSNIAsHost && sni != "" {
			reqHost = sni
		}
		path := inbound.Path
		if host.Path.Valid {
			path = host.Path.String
		}
		tlsValue := inbound.TLS
		if host.TLS.Valid && strings.TrimSpace(host.TLS.String) != "" && strings.TrimSpace(host.TLS.String) != "inbound_default" {
			tlsValue = strings.TrimSpace(host.TLS.String)
		}
		remark := formatSubscriptionText(host.Remark, user, inbound, host)
		node := generatedNode{
			Remark:   remark,
			Protocol: inbound.Protocol,
			Address:  formatSubscriptionText(address, user, inbound, host),
			Port:     anyToString(firstNonNil(host.Port, inbound.Port)),
			Network:  inbound.Network,
			TLS:      tlsValue,
			SNI:      formatSubscriptionText(sni, user, inbound, host),
			Host:     formatSubscriptionText(reqHost, user, inbound, host),
			Path:     formatSubscriptionText(path, user, inbound, host),
			Header:   inbound.HeaderType,
			Settings: settings,
			Inbound:  inbound,
			Mux:      host.MuxEnable,
		}
		if host.ALPN != "" {
			node.Inbound.ALPN = host.ALPN
		}
		if host.Fingerprint != "" {
			node.Inbound.Fingerprint = host.Fingerprint
		}
		if host.AllowInsecure {
			node.Inbound.AllowInsecure = true
		}
		if host.FragmentSetting.Valid {
			node.Inbound.Fragment = host.FragmentSetting.String
		}
		if host.RandomUserAgent {
			node.Inbound.RandomUserAgent = true
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func parseInbounds(config map[string]any) map[string]inboundInfo {
	result := map[string]inboundInfo{}
	rawInbounds, _ := config["inbounds"].([]any)
	for _, raw := range rawInbounds {
		inboundMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		protocol := stringValue(inboundMap["protocol"])
		if !isSupportedProxyProtocol(protocol) {
			continue
		}
		tag := stringValue(inboundMap["tag"])
		if tag == "" {
			continue
		}
		info := inboundInfo{
			Tag:        tag,
			Protocol:   protocol,
			Port:       inboundMap["port"],
			Network:    "tcp",
			TLS:        "none",
			SNI:        []string{},
			Host:       []string{},
			Path:       "",
			HeaderType: "",
		}
		if protocol == "vless" {
			if settings, ok := inboundMap["settings"].(map[string]any); ok {
				info.Encryption = stringValue(settings["encryption"])
			}
		}
		stream, _ := inboundMap["streamSettings"].(map[string]any)
		if len(stream) > 0 {
			network := stringValueDefault(stream["network"], "tcp")
			info.Network = network
			netSettings, _ := stream[network+"Settings"].(map[string]any)
			security := stringValue(stream["security"])
			if security == "tls" || security == "reality" {
				info.TLS = security
				tlsSettings, _ := stream[security+"Settings"].(map[string]any)
				meta, _ := tlsSettings["settings"].(map[string]any)
				info.Fingerprint = firstString(stringValue(meta["fingerprint"]), stringValue(tlsSettings["fingerprint"]))
				if security == "tls" {
					info.ALPN = joinStringList(tlsSettings["alpn"])
					info.AllowInsecure = boolValue(meta["allowInsecure"]) || boolValue(tlsSettings["allowInsecure"])
					serverName := firstString(stringValue(tlsSettings["serverName"]), stringValue(tlsSettings["sni"]))
					if serverName != "" {
						info.SNI = []string{serverName}
					}
				} else {
					info.SNI = toStringList(tlsSettings["serverNames"])
					info.PublicKey = firstString(stringValue(meta["publicKey"]), stringValue(tlsSettings["publicKey"]))
					if info.PublicKey == "" {
						if derived, err := deriveRealityPublicKey(stringValue(tlsSettings["privateKey"])); err == nil {
							info.PublicKey = derived
						}
					}
					info.ShortIDs = toStringList(tlsSettings["shortIds"])
					info.SpiderX = firstString(stringValue(meta["spiderX"]), stringValue(tlsSettings["SpiderX"]), stringValue(tlsSettings["spiderX"]))
				}
			}
			applyNetworkSettings(&info, network, netSettings)
		}
		result[tag] = info
	}
	return result
}

func applyNetworkSettings(info *inboundInfo, network string, settings map[string]any) {
	switch network {
	case "tcp", "raw":
		header, _ := settings["header"].(map[string]any)
		info.HeaderType = stringValue(header["type"])
		request, _ := header["request"].(map[string]any)
		info.Path = firstOrEmpty(toStringList(request["path"]))
		headers, _ := request["headers"].(map[string]any)
		info.Host = toStringList(headers["Host"])
	case "ws":
		info.Path = stringValue(settings["path"])
		info.Host = splitCSV(firstString(stringValue(settings["host"]), nestedString(settings, "headers", "Host")))
		info.Heartbeat = int64Value(settings["heartbeatPeriod"])
	case "grpc", "gun":
		info.Path = stringValue(settings["serviceName"])
		info.Host = splitCSV(stringValue(settings["authority"]))
		info.MultiMode = boolValue(settings["multiMode"])
	case "quic":
		header, _ := settings["header"].(map[string]any)
		info.HeaderType = stringValue(header["type"])
		info.Path = stringValue(settings["key"])
		info.Host = splitCSV(stringValue(settings["security"]))
	case "httpupgrade":
		info.Path = stringValue(settings["path"])
		info.Host = splitCSV(stringValue(settings["host"]))
	case "splithttp", "xhttp":
		info.Path = stringValue(settings["path"])
		info.Host = splitCSV(stringValue(settings["host"]))
	case "kcp":
		header, _ := settings["header"].(map[string]any)
		info.HeaderType = stringValue(header["type"])
		info.Path = stringValue(settings["seed"])
		info.Host = splitCSV(stringValue(header["domain"]))
	case "http", "h2", "h3":
		info.Path = stringValue(settings["path"])
		info.Host = toStringList(firstNonNilAny(settings["host"], settings["Host"]))
	default:
		info.Path = stringValue(settings["path"])
		info.Host = splitCSV(firstString(stringValue(settings["host"]), stringValue(settings["Host"])))
	}
}

func (c *cli) getUUIDMasks() (map[string][]byte, error) {
	masks := map[string][]byte{}
	var vmessMask, vlessMask sql.NullString
	err := c.db.QueryRow("SELECT vmess_mask, vless_mask FROM jwt ORDER BY id LIMIT 1").Scan(&vmessMask, &vlessMask)
	if errors.Is(err, sql.ErrNoRows) {
		return masks, nil
	}
	if err != nil {
		return masks, err
	}
	if vmessMask.Valid {
		if decoded, err := hexToBytes(vmessMask.String); err == nil {
			masks["vmess"] = decoded
		}
	}
	if vlessMask.Valid {
		if decoded, err := hexToBytes(vlessMask.String); err == nil {
			masks["vless"] = decoded
		}
	}
	return masks, nil
}

func runtimeSettings(proxy proxyRecord, user subscriptionUser, masks map[string][]byte) (map[string]any, error) {
	data := copyMap(proxy.Settings)
	key := ""
	if user.CredentialKey.Valid {
		key = normalizeCredentialKey(user.CredentialKey.String)
	}
	switch proxy.Type {
	case "vmess", "vless":
		id := stringValue(firstNonNilAny(data["id"], data["uuid"]))
		if id == "" && key != "" {
			id = keyToUUID(key, masks[proxy.Type])
		}
		if id == "" {
			return nil, fmt.Errorf("UUID is required for %s proxy", proxy.Type)
		}
		data["id"] = id
	case "trojan":
		if stringValue(data["password"]) == "" && key != "" {
			data["password"] = keyToPassword(key, "trojan")
		}
	case "shadowsocks":
		if stringValue(data["password"]) == "" && key != "" {
			data["password"] = keyToPassword(key, "shadowsocks")
		}
		if stringValue(data["method"]) == "" {
			data["method"] = "chacha20-poly1305"
		}
	}
	delete(data, "flow")
	if user.Flow.Valid && strings.TrimSpace(user.Flow.String) != "" {
		data["flow"] = normalizeFlow(user.Flow.String)
	}
	return data, nil
}

func buildV2RayLinks(nodes []generatedNode) []string {
	links := []string{}
	for _, node := range nodes {
		switch node.Protocol {
		case "vmess":
			links = append(links, buildVMessLink(node))
		case "vless":
			links = append(links, buildVLESSLink(node))
		case "trojan":
			links = append(links, buildTrojanLink(node))
		case "shadowsocks":
			links = append(links, buildShadowsocksLink(node))
		}
	}
	return links
}

func buildVMessLink(node generatedNode) string {
	payload := map[string]any{
		"add":  node.Address,
		"aid":  "0",
		"host": node.Host,
		"id":   stringValue(node.Settings["id"]),
		"net":  node.Network,
		"path": normalizeGRPCPath(node.Network, node.Path, node.Inbound.MultiMode),
		"port": node.Port,
		"ps":   node.Remark,
		"scy":  "auto",
		"tls":  node.TLS,
		"type": node.Header,
		"v":    "2",
	}
	addTLSParams(payload, node)
	encoded, _ := json.Marshal(payload)
	return "vmess://" + base64.StdEncoding.EncodeToString(encoded)
}

func buildVLESSLink(node generatedNode) string {
	values := commonShareQuery(node)
	values.Set("encryption", firstString(node.Inbound.Encryption, "none"))
	flow := stringValue(node.Settings["flow"])
	if flow != "" && (node.TLS == "tls" || node.TLS == "reality") && (node.Network == "tcp" || node.Network == "raw" || node.Network == "kcp") && node.Header != "http" {
		values.Set("flow", flow)
	}
	return "vless://" + stringValue(node.Settings["id"]) + "@" + formatAddressForURL(node.Address) + ":" + node.Port + "?" + values.Encode() + "#" + url.QueryEscape(node.Remark)
}

func buildTrojanLink(node generatedNode) string {
	values := commonShareQuery(node)
	flow := stringValue(node.Settings["flow"])
	if flow != "" && (node.TLS == "tls" || node.TLS == "reality") && (node.Network == "tcp" || node.Network == "raw" || node.Network == "kcp") && node.Header != "http" {
		values.Set("flow", flow)
	}
	password := url.PathEscape(stringValue(node.Settings["password"]))
	return "trojan://" + password + "@" + formatAddressForURL(node.Address) + ":" + node.Port + "?" + values.Encode() + "#" + url.QueryEscape(node.Remark)
}

func buildShadowsocksLink(node generatedNode) string {
	method := stringValue(node.Settings["method"])
	password := stringValue(node.Settings["password"])
	userInfo := base64.StdEncoding.EncodeToString([]byte(method + ":" + password))
	return "ss://" + userInfo + "@" + formatAddressForURL(node.Address) + ":" + node.Port + "#" + url.QueryEscape(node.Remark)
}

func commonShareQuery(node generatedNode) url.Values {
	values := url.Values{}
	values.Set("security", node.TLS)
	values.Set("type", node.Network)
	values.Set("headerType", node.Header)
	switch node.Network {
	case "grpc", "gun":
		values.Set("serviceName", normalizeGRPCPath(node.Network, node.Path, node.Inbound.MultiMode))
		values.Set("authority", node.Host)
		if node.Inbound.MultiMode {
			values.Set("mode", "multi")
		} else {
			values.Set("mode", "gun")
		}
	case "quic":
		values.Set("key", node.Path)
		values.Set("quicSecurity", node.Host)
	case "kcp":
		values.Set("seed", node.Path)
		values.Set("host", node.Host)
	default:
		values.Set("path", node.Path)
		values.Set("host", node.Host)
	}
	if node.TLS == "tls" {
		values.Set("sni", node.SNI)
		values.Set("fp", node.Inbound.Fingerprint)
		if node.Inbound.ALPN != "" {
			values.Set("alpn", node.Inbound.ALPN)
		}
		if node.Inbound.AllowInsecure {
			values.Set("allowInsecure", "1")
		}
		if node.Inbound.Fragment != "" {
			values.Set("fragment", node.Inbound.Fragment)
		}
	}
	if node.TLS == "reality" {
		values.Set("sni", node.SNI)
		values.Set("fp", firstString(node.Inbound.Fingerprint, "chrome"))
		values.Set("pbk", node.Inbound.PublicKey)
		values.Set("sid", firstOrEmpty(node.Inbound.ShortIDs))
		if node.Inbound.SpiderX != "" {
			values.Set("spx", node.Inbound.SpiderX)
		}
	}
	if node.Network == "ws" && node.Inbound.Heartbeat > 0 {
		values.Set("heartbeatPeriod", strconv.FormatInt(node.Inbound.Heartbeat, 10))
	}
	return values
}

func addTLSParams(payload map[string]any, node generatedNode) {
	if node.TLS == "tls" {
		payload["sni"] = node.SNI
		payload["fp"] = node.Inbound.Fingerprint
		if node.Inbound.ALPN != "" {
			payload["alpn"] = node.Inbound.ALPN
		}
		if node.Inbound.AllowInsecure {
			payload["allowInsecure"] = 1
		}
	}
	if node.TLS == "reality" {
		payload["sni"] = node.SNI
		payload["fp"] = firstString(node.Inbound.Fingerprint, "chrome")
		payload["pbk"] = node.Inbound.PublicKey
		payload["sid"] = firstOrEmpty(node.Inbound.ShortIDs)
		if node.Inbound.SpiderX != "" {
			payload["spx"] = node.Inbound.SpiderX
		}
	}
	if node.Network == "grpc" || node.Network == "gun" {
		if node.Inbound.MultiMode {
			payload["mode"] = "multi"
		} else {
			payload["mode"] = "gun"
		}
	}
}

func buildClashConfig(nodes []generatedNode) string {
	var b strings.Builder
	b.WriteString("mode: Global\n")
	b.WriteString("port: 7890\n")
	b.WriteString("proxies:\n")
	names := []string{}
	for _, node := range nodes {
		if node.Network == "kcp" || node.Network == "splithttp" || node.Network == "xhttp" {
			continue
		}
		names = append(names, node.Remark)
		writeClashNode(&b, node)
	}
	b.WriteString("proxy-groups:\n")
	b.WriteString("- name: 'Automatic'\n")
	b.WriteString("  type: 'url-test'\n")
	b.WriteString("  url: 'http://www.gstatic.com/generate_204'\n")
	b.WriteString("  interval: 300\n")
	b.WriteString("  proxies:\n")
	for _, name := range names {
		b.WriteString("  - ")
		b.WriteString(yamlQuote(name))
		b.WriteString("\n")
	}
	b.WriteString("rules: []\n")
	return b.String()
}

func writeClashNode(b *strings.Builder, node generatedNode) {
	protocol := node.Protocol
	if protocol == "shadowsocks" {
		protocol = "ss"
	}
	b.WriteString("- name: ")
	b.WriteString(yamlQuote(node.Remark))
	b.WriteString("\n")
	b.WriteString("  type: ")
	b.WriteString(yamlQuote(protocol))
	b.WriteString("\n")
	b.WriteString("  server: ")
	b.WriteString(yamlQuote(node.Address))
	b.WriteString("\n")
	b.WriteString("  port: ")
	b.WriteString(node.Port)
	b.WriteString("\n")
	b.WriteString("  network: ")
	b.WriteString(yamlQuote(normalizeClashNetwork(node.Network, node.Header)))
	b.WriteString("\n")
	b.WriteString("  udp: true\n")
	switch node.Protocol {
	case "vmess", "vless":
		b.WriteString("  uuid: ")
		b.WriteString(yamlQuote(stringValue(node.Settings["id"])))
		b.WriteString("\n")
		if node.Protocol == "vmess" {
			b.WriteString("  alterId: 0\n")
			b.WriteString("  cipher: auto\n")
		} else {
			b.WriteString("  encryption: ")
			b.WriteString(yamlQuote(firstString(node.Inbound.Encryption, "none")))
			b.WriteString("\n")
			if flow := stringValue(node.Settings["flow"]); flow != "" {
				b.WriteString("  flow: ")
				b.WriteString(yamlQuote(flow))
				b.WriteString("\n")
			}
		}
	case "trojan":
		b.WriteString("  password: ")
		b.WriteString(yamlQuote(stringValue(node.Settings["password"])))
		b.WriteString("\n")
	case "shadowsocks":
		b.WriteString("  password: ")
		b.WriteString(yamlQuote(stringValue(node.Settings["password"])))
		b.WriteString("\n")
		b.WriteString("  cipher: ")
		b.WriteString(yamlQuote(stringValue(node.Settings["method"])))
		b.WriteString("\n")
	}
	if node.TLS == "tls" || node.TLS == "reality" {
		b.WriteString("  tls: true\n")
		if node.SNI != "" {
			if node.Protocol == "trojan" {
				b.WriteString("  sni: ")
			} else {
				b.WriteString("  servername: ")
			}
			b.WriteString(yamlQuote(node.SNI))
			b.WriteString("\n")
		}
		if node.Inbound.ALPN != "" {
			b.WriteString("  alpn:\n")
			for _, alpn := range splitCSV(node.Inbound.ALPN) {
				b.WriteString("  - ")
				b.WriteString(yamlQuote(alpn))
				b.WriteString("\n")
			}
		}
		if node.Inbound.AllowInsecure {
			b.WriteString("  skip-cert-verify: true\n")
		}
	}
	writeClashTransportOptions(b, node)
}

func writeClashTransportOptions(b *strings.Builder, node generatedNode) {
	network := normalizeClashNetwork(node.Network, node.Header)
	switch network {
	case "ws":
		b.WriteString("  ws-opts:\n")
		if node.Path != "" {
			b.WriteString("    path: ")
			b.WriteString(yamlQuote(node.Path))
			b.WriteString("\n")
		}
		if node.Host != "" {
			b.WriteString("    headers:\n")
			b.WriteString("      Host: ")
			b.WriteString(yamlQuote(node.Host))
			b.WriteString("\n")
		}
	case "grpc":
		if node.Path != "" {
			b.WriteString("  grpc-opts:\n")
			b.WriteString("    grpc-service-name: ")
			b.WriteString(yamlQuote(normalizeGRPCPath(node.Network, node.Path, node.Inbound.MultiMode)))
			b.WriteString("\n")
		}
	case "http", "h2":
		b.WriteString("  h2-opts:\n")
		if node.Path != "" {
			b.WriteString("    path: ")
			b.WriteString(yamlQuote(node.Path))
			b.WriteString("\n")
		}
		if node.Host != "" {
			b.WriteString("    host:\n")
			b.WriteString("    - ")
			b.WriteString(yamlQuote(node.Host))
			b.WriteString("\n")
		}
	}
}

func decodeJSON(raw any, target any) error {
	switch value := raw.(type) {
	case nil:
		return nil
	case []byte:
		if len(value) == 0 {
			return nil
		}
		return json.Unmarshal(value, target)
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return json.Unmarshal([]byte(value), target)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return json.Unmarshal(encoded, target)
	}
}

func copyMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range in {
		out[key] = value
	}
	return out
}

func isSupportedProxyProtocol(protocol string) bool {
	switch protocol {
	case "vmess", "vless", "trojan", "shadowsocks":
		return true
	default:
		return false
	}
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func toStringList(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case []any:
		out := []string{}
		for _, item := range typed {
			text := strings.TrimSpace(stringValue(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		out := []string{}
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	default:
		return splitCSV(stringValue(value))
	}
}

func joinStringList(value any) string {
	return strings.Join(toStringList(value), ",")
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func stringValueDefault(value any, fallback string) string {
	if text := stringValue(value); text != "" {
		return text
	}
	return fallback
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		return 0
	}
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}

func normalizeNone(value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "none") {
		return ""
	}
	return value
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstOrEmpty(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstNonEmptyList(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstNonNil(port sql.NullInt64, fallback any) any {
	if port.Valid && port.Int64 > 0 {
		return port.Int64
	}
	return fallback
}

func firstNonNilAny(values ...any) any {
	for _, value := range values {
		if stringValue(value) != "" {
			return value
		}
	}
	return nil
}

func anyToString(value any) string {
	text := stringValue(value)
	if strings.Contains(text, ".") {
		if parsed, err := strconv.ParseFloat(text, 64); err == nil {
			return strconv.FormatInt(int64(parsed), 10)
		}
	}
	return text
}

func nestedString(value map[string]any, keys ...string) string {
	var current any = value
	for _, key := range keys {
		next, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = next[key]
	}
	return stringValue(current)
}

func formatSubscriptionText(template string, user subscriptionUser, inbound inboundInfo, host hostInfo) string {
	if template == "" {
		return ""
	}
	replacements := map[string]string{
		"{USERNAME}":  user.Username,
		"{PROTOCOL}":  inbound.Protocol,
		"{TRANSPORT}": inbound.Network,
	}
	result := template
	for key, value := range replacements {
		result = strings.ReplaceAll(result, key, value)
	}
	if strings.Contains(result, "*") {
		salt := user.Username
		if len(salt) > 8 {
			salt = salt[:8]
		}
		for len(salt) < 8 {
			salt += "0"
		}
		result = strings.ReplaceAll(result, "*", salt)
	}
	_ = host
	return result
}

func normalizeCredentialKey(key string) string {
	key = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", ""))
	if len(key) != 32 {
		return ""
	}
	for _, ch := range key {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			return ""
		}
	}
	return key
}

func hexToBytes(value string) ([]byte, error) {
	value = normalizeCredentialKey(value)
	if value == "" {
		return nil, errors.New("invalid hex")
	}
	out := make([]byte, 16)
	for i := 0; i < 16; i++ {
		parsed, err := strconv.ParseUint(value[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil, err
		}
		out[i] = byte(parsed)
	}
	return out, nil
}

func keyToUUID(key string, mask []byte) string {
	bytes, err := hexToBytes(key)
	if err != nil {
		return ""
	}
	if len(mask) == len(bytes) {
		for i := range bytes {
			bytes[i] ^= mask[i]
		}
	}
	hex := fmt.Sprintf("%x", bytes)
	return hex[:8] + "-" + hex[8:12] + "-" + hex[12:16] + "-" + hex[16:20] + "-" + hex[20:]
}

func keyToPassword(key string, label string) string {
	sum := sha256.Sum256([]byte(label + ":" + key))
	return fmt.Sprintf("%x", sum[:])[:32]
}

func normalizeFlow(flow string) string {
	flow = strings.ToLower(strings.TrimSpace(flow))
	switch flow {
	case "xtls-rprx-vision":
		return flow
	case "xtls-rprx-vision-udp443":
		return "xtls-rprx-vision"
	default:
		return ""
	}
}

func deriveRealityPublicKey(privateKey string) (string, error) {
	privateKey = strings.TrimSpace(privateKey)
	if privateKey == "" {
		return "", errors.New("empty private key")
	}
	raw, err := base64.RawURLEncoding.DecodeString(privateKey)
	if err != nil {
		raw, err = base64.StdEncoding.DecodeString(privateKey)
		if err != nil {
			return "", err
		}
	}
	if len(raw) != 32 {
		return "", errors.New("private key must be 32 bytes")
	}
	public, err := curve25519.X25519(raw, curve25519.Basepoint)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(public), nil
}

func formatAddressForURL(address string) string {
	if strings.Contains(address, ":") && net.ParseIP(address) != nil {
		return "[" + address + "]"
	}
	return address
}

func normalizeGRPCPath(network string, path string, multiMode bool) string {
	if network != "grpc" && network != "gun" {
		return path
	}
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "/") {
		path = strings.TrimLeft(path, "/")
	}
	return path
}

func normalizeClashNetwork(network string, header string) string {
	if network == "httpupgrade" {
		return "ws"
	}
	if (network == "tcp" || network == "raw") && header == "http" {
		return "http"
	}
	if network == "http" || network == "h2" || network == "h3" {
		return "h2"
	}
	if network == "gun" {
		return "grpc"
	}
	return network
}

func yamlQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
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
	fmt.Println("  migrate        Database migrations")
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

func printMigrateUsage() {
	fmt.Println("Usage: rebecca migrate <command>")
	fmt.Println("Commands: up [--to version], status")
	fmt.Println("Downgrades are intentionally unsupported.")
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
