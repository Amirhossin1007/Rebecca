package gateway

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                     string
	TLSCertFile              string
	TLSKeyFile               string
	MasterAPIEnabled         bool
	MasterAPIAddr            string
	MasterAPIURL             string
	NativeNodeRoutes         bool
	NativeSubscriptionRoutes bool
	SubscriptionPrefixes     []string
	DashboardPath            string
	DashboardBuildDir        string
	MasterAPIStartWait       time.Duration
	PythonEnabled            bool
	PythonBin                string
	PythonApp                string
	PythonHost               string
	PythonPort               int
	PythonEnvFile            string
	PythonStartTimeout       time.Duration
}

func LoadConfig() Config {
	return Config{
		Addr:                     env("REBECCA_GATEWAY_ADDR", ":8000"),
		TLSCertFile:              firstEnv("REBECCA_GATEWAY_TLS_CERTFILE", "UVICORN_SSL_CERTFILE"),
		TLSKeyFile:               firstEnv("REBECCA_GATEWAY_TLS_KEYFILE", "UVICORN_SSL_KEYFILE"),
		MasterAPIEnabled:         envBool("REBECCA_MASTER_API_ENABLED", true),
		MasterAPIAddr:            env("REBECCA_MASTER_API_ADDR", "127.0.0.1:18081"),
		MasterAPIURL:             env("GO_MASTER_API_URL", ""),
		NativeNodeRoutes:         envBool("REBECCA_GATEWAY_NATIVE_NODE_ROUTES", true),
		NativeSubscriptionRoutes: envBool("REBECCA_GATEWAY_NATIVE_SUBSCRIPTION_ROUTES", true),
		SubscriptionPrefixes:     splitCSVEnv("REBECCA_GATEWAY_SUBSCRIPTION_PREFIXES"),
		DashboardPath:            env("DASHBOARD_PATH", "/dashboard/"),
		DashboardBuildDir:        env("REBECCA_DASHBOARD_BUILD_DIR", defaultDashboardBuildDir()),
		MasterAPIStartWait:       time.Duration(envInt("REBECCA_MASTER_API_START_TIMEOUT_SECONDS", 30)) * time.Second,
		PythonEnabled:            envBool("REBECCA_PYTHON_ENABLED", false),
		PythonBin:                env("REBECCA_PYTHON_BIN", defaultPythonBin()),
		PythonApp:                env("REBECCA_PYTHON_APP", "app:app"),
		PythonHost:               env("REBECCA_PYTHON_HOST", "127.0.0.1"),
		PythonPort:               envInt("REBECCA_PYTHON_PORT", 18000),
		PythonEnvFile:            env("REBECCA_PYTHON_ENV_FILE", ""),
		PythonStartTimeout:       time.Duration(envInt("REBECCA_PYTHON_START_TIMEOUT_SECONDS", 300)) * time.Second,
	}
}

func defaultDashboardBuildDir() string {
	candidates := []string{}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "dashboard", "build"))
		candidates = append(candidates, filepath.Join(cwd, "dashboard", "dist"))
		candidates = append(candidates, filepath.Join(filepath.Dir(cwd), "dashboard", "build"))
		candidates = append(candidates, filepath.Join(filepath.Dir(cwd), "dashboard", "dist"))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, "dashboard", "build"))
		candidates = append(candidates, filepath.Join(dir, "dashboard", "dist"))
		candidates = append(candidates, filepath.Join(filepath.Dir(dir), "dashboard", "build"))
		candidates = append(candidates, filepath.Join(filepath.Dir(dir), "dashboard", "dist"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func splitCSVEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		cleaned := strings.TrimSpace(part)
		if cleaned == "" {
			continue
		}
		if !strings.HasPrefix(cleaned, "/") {
			cleaned = "/" + cleaned
		}
		result = append(result, cleaned)
	}
	return result
}

func defaultPythonBin() string {
	exe, err := os.Executable()
	if err != nil {
		return "python"
	}
	candidate := filepath.Join(filepath.Dir(exe), packagedPythonServerName())
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return "python"
}

func packagedPythonServerName() string {
	if runtime.GOOS == "windows" {
		return "rebecca-python-server.exe"
	}
	return "rebecca-python-server"
}

func (c Config) PythonAddr() string {
	return net.JoinHostPort(c.PythonHost, strconv.Itoa(c.PythonPort))
}

func (c Config) ResolvedMasterAPIURL() string {
	if strings.TrimSpace(c.MasterAPIURL) != "" {
		return strings.TrimRight(strings.TrimSpace(c.MasterAPIURL), "/")
	}
	host, port, err := net.SplitHostPort(c.MasterAPIAddr)
	if err != nil {
		return "http://" + strings.TrimRight(c.MasterAPIAddr, "/")
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
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
