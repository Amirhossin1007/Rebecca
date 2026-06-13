package gateway

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	cfg    Config
	server *http.Server
}

func NewServer(cfg Config) (*Server, error) {
	dashboard := newDashboardFiles(cfg)
	apiHandler := cfg.APIHandler

	mux := http.NewServeMux()
	mux.HandleFunc("/__rebecca_go/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/__rebecca_go/api_healthz", func(w http.ResponseWriter, r *http.Request) {
		if apiHandler == nil {
			http.Error(w, "Go API handler is unavailable", http.StatusServiceUnavailable)
			return
		}
		apiHandler.ServeHTTP(w, apiHealthRequest(r))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if dashboard.matches(r) {
			dashboard.serve(w, r)
			return
		}
		if isDeprecatedRuntimeRoute(r) {
			http.Error(w, deprecatedRuntimeRouteDetail(r), http.StatusGone)
			return
		}
		if isDeprecatedTelegramSettingsRoute(r) {
			http.Error(w, "Telegram integration is temporarily disabled and tracked in TODO_GO_TELEGRAM.md.", http.StatusGone)
			return
		}
		if isDeprecatedMasterNodeRoute(r) {
			http.Error(w, "master node usage/runtime routes have been removed", http.StatusGone)
			return
		}
		if apiHandler == nil {
			http.Error(w, "Go API handler is unavailable", http.StatusServiceUnavailable)
			return
		}
		apiHandler.ServeHTTP(w, r)
	})

	return &Server{
		cfg: cfg,
		server: &http.Server{
			Addr:              cfg.Addr,
			Handler:           mux,
			ReadHeaderTimeout: 15 * time.Second,
		},
	}, nil
}

func apiHealthRequest(r *http.Request) *http.Request {
	req := r.Clone(r.Context())
	req.Method = http.MethodGet
	req.URL.Path = "/__rebecca_api/healthz"
	req.URL.RawPath = ""
	req.URL.RawQuery = ""
	return req
}

func isDeprecatedRuntimeRoute(r *http.Request) bool {
	path := strings.TrimRight(r.URL.Path, "/")
	return path == "/api/core/xray/update" || path == "/api/core/access" || strings.HasPrefix(path, "/api/core/access/")
}

func deprecatedRuntimeRouteDetail(r *http.Request) string {
	path := strings.TrimRight(r.URL.Path, "/")
	if path == "/api/core/xray/update" {
		return "Master runtime is node-only; update nodes instead."
	}
	return "Access Insights is temporarily disabled while it is rebuilt as a Go-native feature."
}

func isDeprecatedTelegramSettingsRoute(r *http.Request) bool {
	path := strings.TrimRight(r.URL.Path, "/")
	return path == "/api/settings/telegram"
}

func isDeprecatedMasterNodeRoute(r *http.Request) bool {
	path := strings.TrimRight(r.URL.Path, "/")
	switch path {
	case "/api/node/master":
		return r.Method == http.MethodGet || r.Method == http.MethodPut
	case "/api/node/master/usage/reset":
		return r.Method == http.MethodPost
	default:
		return false
	}
}

func (s *Server) Run() error {
	var err error
	if s.cfg.TLSCertFile != "" && s.cfg.TLSKeyFile != "" {
		err = s.server.ListenAndServeTLS(s.cfg.TLSCertFile, s.cfg.TLSKeyFile)
	} else {
		err = s.server.ListenAndServe()
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
