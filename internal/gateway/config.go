package gateway

import (
	"net"
	"net/http"
	"os"
	"strings"
)

type Config struct {
	Addr          string
	TLSCertFile   string
	TLSKeyFile    string
	DashboardPath string
	APIHandler    http.Handler
}

func LoadConfig() Config {
	return Config{
		Addr:          gatewayListenAddr(),
		TLSCertFile:   strings.TrimSpace(os.Getenv("UVICORN_SSL_CERTFILE")),
		TLSKeyFile:    strings.TrimSpace(os.Getenv("UVICORN_SSL_KEYFILE")),
		DashboardPath: env("DASHBOARD_PATH", "/dashboard/"),
	}
}

func gatewayListenAddr() string {
	if addr := strings.TrimSpace(os.Getenv("REBECCA_GATEWAY_ADDR")); addr != "" {
		return addr
	}
	host := env("UVICORN_HOST", "0.0.0.0")
	port := env("UVICORN_PORT", "8000")
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return net.JoinHostPort(host, port)
	}
	if host == "" || host == "0.0.0.0" {
		return ":" + port
	}
	return net.JoinHostPort(host, port)
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
