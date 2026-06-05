package masterapi

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:18080"

type Config struct {
	Address                     string
	Database                    string
	TLSCert                     string
	TLSKey                      string
	NodeOperationsPollInterval  string
	UserLifecycleInterval       string
	UserLifecycleBatchSize      int
	UserUsageResetInterval      string
	UserUsageResetBatchSize     int
	JWTAccessTokenExpireMinutes int
	UsersListTimeoutSeconds     float64
	SubscriptionReadOnly        bool
	SudoUsername                string
	SudoPassword                string
}

func LoadConfig() (Config, error) {
	env := loadEnvFiles()
	lookup := func(keys ...string) string {
		for _, key := range keys {
			if value := strings.TrimSpace(os.Getenv(key)); value != "" {
				return value
			}
			if value := strings.TrimSpace(env[key]); value != "" {
				return value
			}
		}
		return ""
	}

	addr := lookup("REBECCA_MASTER_API_ADDR", "REBECCA_GO_API_ADDR")
	if addr == "" {
		host := lookup("REBECCA_MASTER_API_HOST", "REBECCA_GO_API_HOST")
		port := lookup("REBECCA_MASTER_API_PORT", "REBECCA_GO_API_PORT")
		switch {
		case host != "" && port != "":
			addr = host + ":" + port
		case port != "":
			addr = "127.0.0.1:" + port
		default:
			addr = defaultAddress
		}
	}

	cfg := Config{
		Address:                     addr,
		Database:                    lookup("SQLALCHEMY_DATABASE_URL", "DATABASE_URL"),
		TLSCert:                     lookup("REBECCA_MASTER_API_TLS_CERTFILE", "UVICORN_SSL_CERTFILE", "SSL_CERTFILE"),
		TLSKey:                      lookup("REBECCA_MASTER_API_TLS_KEYFILE", "UVICORN_SSL_KEYFILE", "SSL_KEYFILE"),
		NodeOperationsPollInterval:  lookup("REBECCA_NODE_OPERATIONS_POLL_INTERVAL"),
		UserLifecycleInterval:       firstNonEmpty(lookup("REBECCA_USER_LIFECYCLE_INTERVAL"), secondsEnv(lookup("JOB_REVIEW_USERS_INTERVAL"))),
		UserLifecycleBatchSize:      parseIntDefault(lookup("REBECCA_USER_LIFECYCLE_BATCH_SIZE", "JOB_REVIEW_USERS_BATCH_SIZE"), 500),
		UserUsageResetInterval:      lookup("REBECCA_USER_USAGE_RESET_INTERVAL"),
		UserUsageResetBatchSize:     parseIntDefault(lookup("REBECCA_USER_USAGE_RESET_BATCH_SIZE"), 500),
		JWTAccessTokenExpireMinutes: parseIntDefault(lookup("JWT_ACCESS_TOKEN_EXPIRE_MINUTES"), 1440),
		UsersListTimeoutSeconds:     parseFloatDefault(lookup("USERS_LIST_TIMEOUT_SECONDS"), 0),
		SubscriptionReadOnly:        parseBoolDefault(lookup("SUBSCRIPTION_READ_ONLY"), false),
		SudoUsername:                lookup("SUDO_USERNAME"),
		SudoPassword:                lookup("SUDO_PASSWORD"),
	}
	if cfg.Database == "" {
		return Config{}, fmt.Errorf("SQLALCHEMY_DATABASE_URL is required")
	}
	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func secondsEnv(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return value + "s"
	}
	return value
}

func parseBoolDefault(value string, fallback bool) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseIntDefault(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	var result int
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return fallback
	}
	return result
}

func parseFloatDefault(value string, fallback float64) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	result, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return result
}

func loadEnvFiles() map[string]string {
	result := map[string]string{}
	for _, path := range candidateEnvFiles() {
		mergeEnvFile(result, path)
	}
	return result
}

func candidateEnvFiles() []string {
	seen := map[string]bool{}
	add := func(paths []string, path string) []string {
		path = strings.TrimSpace(path)
		if path == "" {
			return paths
		}
		abs, err := filepath.Abs(path)
		if err == nil {
			path = abs
		}
		if seen[path] {
			return paths
		}
		seen[path] = true
		return append(paths, path)
	}

	paths := []string{}
	paths = add(paths, os.Getenv("REBECCA_ENV_FILE"))
	paths = add(paths, os.Getenv("REBECCA_PYTHON_ENV_FILE"))
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		paths = add(paths, filepath.Join(dir, ".env"))
		paths = add(paths, filepath.Join(filepath.Dir(dir), ".env"))
	}
	if cwd, err := os.Getwd(); err == nil {
		paths = add(paths, filepath.Join(cwd, ".env"))
		paths = add(paths, filepath.Join(filepath.Dir(cwd), ".env"))
	}
	return paths
}

func mergeEnvFile(dst map[string]string, path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key != "" {
			if _, exists := dst[key]; exists {
				continue
			}
			dst[key] = value
		}
	}
}
