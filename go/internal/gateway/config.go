package gateway

import (
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr               string
	TLSCertFile        string
	TLSKeyFile         string
	PythonBin          string
	PythonApp          string
	PythonHost         string
	PythonPort         int
	PythonEnvFile      string
	PythonStartTimeout time.Duration
}

func LoadConfig() Config {
	return Config{
		Addr:               env("REBECCA_GATEWAY_ADDR", ":8000"),
		TLSCertFile:        firstEnv("REBECCA_GATEWAY_TLS_CERTFILE", "UVICORN_SSL_CERTFILE"),
		TLSKeyFile:         firstEnv("REBECCA_GATEWAY_TLS_KEYFILE", "UVICORN_SSL_KEYFILE"),
		PythonBin:          env("REBECCA_PYTHON_BIN", "python"),
		PythonApp:          env("REBECCA_PYTHON_APP", "app:app"),
		PythonHost:         env("REBECCA_PYTHON_HOST", "127.0.0.1"),
		PythonPort:         envInt("REBECCA_PYTHON_PORT", 18000),
		PythonEnvFile:      env("REBECCA_PYTHON_ENV_FILE", ""),
		PythonStartTimeout: time.Duration(envInt("REBECCA_PYTHON_START_TIMEOUT_SECONDS", 300)) * time.Second,
	}
}

func (c Config) PythonAddr() string {
	return net.JoinHostPort(c.PythonHost, strconv.Itoa(c.PythonPort))
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
