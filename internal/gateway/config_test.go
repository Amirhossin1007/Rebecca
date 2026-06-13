package gateway

import "testing"

func TestLoadConfigUsesLegacyUvicornEnv(t *testing.T) {
	t.Setenv("REBECCA_GATEWAY_ADDR", "")
	t.Setenv("UVICORN_HOST", "127.0.0.1")
	t.Setenv("UVICORN_PORT", "9443")
	t.Setenv("UVICORN_SSL_CERTFILE", "/tmp/rebecca/fullchain.pem")
	t.Setenv("UVICORN_SSL_KEYFILE", "/tmp/rebecca/key.pem")

	cfg := LoadConfig()

	if cfg.Addr != "127.0.0.1:9443" {
		t.Fatalf("Addr=%q want %q", cfg.Addr, "127.0.0.1:9443")
	}
	if cfg.TLSCertFile != "/tmp/rebecca/fullchain.pem" {
		t.Fatalf("TLSCertFile=%q", cfg.TLSCertFile)
	}
	if cfg.TLSKeyFile != "/tmp/rebecca/key.pem" {
		t.Fatalf("TLSKeyFile=%q", cfg.TLSKeyFile)
	}
}

func TestLoadConfigKeepsGatewayAddrOverride(t *testing.T) {
	t.Setenv("REBECCA_GATEWAY_ADDR", ":18080")
	t.Setenv("UVICORN_HOST", "127.0.0.1")
	t.Setenv("UVICORN_PORT", "9443")

	cfg := LoadConfig()

	if cfg.Addr != ":18080" {
		t.Fatalf("Addr=%q want %q", cfg.Addr, ":18080")
	}
}
