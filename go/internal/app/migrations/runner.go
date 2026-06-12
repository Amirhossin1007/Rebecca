package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/pressly/goose/v3"
)

//go:embed sql/*
var migrationFS embed.FS

var gooseMu sync.Mutex
var migrationDialect string

type Runner struct {
	DB      *sql.DB
	Dialect string
}

type VersionInfo struct {
	Dialect                string `json:"dialect"`
	HasAlembic             bool   `json:"has_alembic"`
	AlembicRevision        string `json:"alembic_revision,omitempty"`
	LegacyRevisionKnownBad bool   `json:"legacy_revision_known_bad"`
	LegacyRevisionHandling string `json:"legacy_revision_handling,omitempty"`
	HasGoose               bool   `json:"has_goose"`
	GooseVersion           int64  `json:"goose_version"`
}

type StatusInfo struct {
	Version VersionInfo `json:"version"`
	Dirty   bool        `json:"dirty"`
	Message string      `json:"message"`
}

func New(db *sql.DB, dialect string) Runner {
	return Runner{DB: db, Dialect: NormalizeDialect(dialect)}
}

func RunMigrations(ctx context.Context, db *sql.DB, dialect string) error {
	return New(db, dialect).Run(ctx)
}

func RunMigrationsTo(ctx context.Context, db *sql.DB, dialect string, version int64) error {
	return New(db, dialect).RunTo(ctx, version)
}

func Status(ctx context.Context, db *sql.DB, dialect string) (StatusInfo, error) {
	return New(db, dialect).Status(ctx)
}

func Version(ctx context.Context, db *sql.DB, dialect string) (VersionInfo, error) {
	return New(db, dialect).Version(ctx)
}

func (r Runner) Run(ctx context.Context) error {
	return r.run(ctx, 0)
}

func (r Runner) RunTo(ctx context.Context, version int64) error {
	if version <= 0 {
		return r.Run(ctx)
	}
	return r.run(ctx, version)
}

func (r Runner) run(ctx context.Context, targetVersion int64) error {
	if r.DB == nil {
		return fmt.Errorf("database is nil")
	}
	dialect := NormalizeDialect(r.Dialect)
	if err := setGooseDialect(dialect); err != nil {
		return err
	}
	gooseMu.Lock()
	defer gooseMu.Unlock()
	migrationDialect = dialect
	defer func() { migrationDialect = "" }()
	goose.SetLogger(log.New(io.Discard, "", 0))
	goose.SetBaseFS(migrationFS)
	defer goose.SetBaseFS(nil)
	var err error
	if targetVersion > 0 {
		err = goose.UpToContext(ctx, r.DB, "sql", targetVersion, goose.WithNoColor(true))
	} else {
		err = goose.UpContext(ctx, r.DB, "sql", goose.WithNoColor(true))
	}
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no migration files found") {
		_, ensureErr := goose.EnsureDBVersion(r.DB)
		return ensureErr
	}
	return err
}

func activeDialect() string {
	if migrationDialect == "" {
		return "sqlite"
	}
	return migrationDialect
}

func (r Runner) Status(ctx context.Context) (StatusInfo, error) {
	version, err := r.Version(ctx)
	if err != nil {
		return StatusInfo{}, err
	}
	message := "goose migrations are initialized"
	if version.LegacyRevisionKnownBad && !version.HasGoose {
		message = "known-bad legacy Alembic revision detected; Go migrations bypass the broken revision tag and run idempotent repairs"
	} else if version.HasAlembic && !version.HasGoose {
		message = "legacy alembic database detected; Go migrations have not been initialized"
	}
	return StatusInfo{Version: version, Message: message}, nil
}

func (r Runner) Version(ctx context.Context) (VersionInfo, error) {
	if r.DB == nil {
		return VersionInfo{}, fmt.Errorf("database is nil")
	}
	dialect := NormalizeDialect(r.Dialect)
	info := VersionInfo{Dialect: dialect, GooseVersion: -1}

	hasAlembic, err := HasTable(ctx, r.DB, dialect, "alembic_version")
	if err != nil {
		return VersionInfo{}, err
	}
	info.HasAlembic = hasAlembic
	if hasAlembic {
		revision, err := readAlembicRevision(ctx, r.DB)
		if err != nil {
			return VersionInfo{}, err
		}
		info.AlembicRevision = revision
		if handling := legacyAlembicRevisionHandling(revision); handling != "" {
			info.LegacyRevisionKnownBad = true
			info.LegacyRevisionHandling = handling
		}
	}

	hasGoose, err := HasTable(ctx, r.DB, dialect, "goose_db_version")
	if err != nil {
		return VersionInfo{}, err
	}
	info.HasGoose = hasGoose
	if hasGoose {
		if err := setGooseDialect(dialect); err != nil {
			return VersionInfo{}, err
		}
		gooseMu.Lock()
		version, err := goose.GetDBVersion(r.DB)
		gooseMu.Unlock()
		if err != nil {
			return VersionInfo{}, err
		}
		info.GooseVersion = version
	}
	return info, nil
}

func UnsupportedDowngrade() error {
	return fmt.Errorf("downgrade migrations are not supported")
}

func NormalizeDialect(dialect string) string {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "sqlite", "sqlite3":
		return "sqlite"
	case "mysql", "mariadb":
		return "mysql"
	default:
		return strings.ToLower(strings.TrimSpace(dialect))
	}
}

func setGooseDialect(dialect string) error {
	switch NormalizeDialect(dialect) {
	case "sqlite":
		return goose.SetDialect("sqlite3")
	case "mysql":
		return goose.SetDialect("mysql")
	default:
		return fmt.Errorf("unsupported migration dialect: %s", dialect)
	}
}

func readAlembicRevision(ctx context.Context, db *sql.DB) (string, error) {
	var revision sql.NullString
	err := db.QueryRowContext(ctx, `SELECT version_num FROM alembic_version LIMIT 1`).Scan(&revision)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return revision.String, nil
}

func legacyAlembicRevisionHandling(revision string) string {
	switch strings.TrimSpace(revision) {
	case "5g6h7i8j9k0l":
		return "skip broken Alembic merge/no-op revision; run all Go migrations normally"
	case "ff05a3b7cdef":
		return "skip broken Alembic direct seed; admin role repair is covered by Go migration 000004"
	default:
		return ""
	}
}
