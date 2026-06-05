package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	cfg    Config
	server *http.Server
}

func NewServer(cfg Config) (*Server, error) {
	target, err := url.Parse("http://" + cfg.PythonAddr())
	if err != nil {
		return nil, err
	}

	pythonProxy := httputil.NewSingleHostReverseProxy(target)
	pythonProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, fmt.Sprintf("python runtime unavailable: %s", err), http.StatusBadGateway)
	}

	var masterProxy *httputil.ReverseProxy
	if strings.TrimSpace(cfg.MasterAPIURL) != "" {
		masterTarget, err := url.Parse(strings.TrimRight(cfg.MasterAPIURL, "/"))
		if err != nil {
			return nil, err
		}
		masterProxy = httputil.NewSingleHostReverseProxy(masterTarget)
		masterProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, fmt.Sprintf("native Go Master API unavailable: %s", err), http.StatusServiceUnavailable)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/__rebecca_go/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/__rebecca_go/master_api_healthz", func(w http.ResponseWriter, r *http.Request) {
		if masterProxy == nil || strings.TrimSpace(cfg.MasterAPIURL) == "" {
			http.Error(w, "native node routes are not enabled", http.StatusServiceUnavailable)
			return
		}
		req, err := http.NewRequestWithContext(
			r.Context(),
			http.MethodGet,
			strings.TrimRight(cfg.MasterAPIURL, "/")+"/__rebecca_master_api/healthz",
			nil,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		defer res.Body.Close()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(res.StatusCode)
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			_, _ = w.Write([]byte("ok\n"))
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if isNativeAdminRoute(r) {
			if masterProxy == nil {
				http.Error(w, "native Go Master API unavailable", http.StatusServiceUnavailable)
				return
			}
			masterProxy.ServeHTTP(w, r)
			return
		}
		if isNativeUserRoute(r) {
			if masterProxy == nil {
				http.Error(w, "native Go Master API unavailable", http.StatusServiceUnavailable)
				return
			}
			masterProxy.ServeHTTP(w, r)
			return
		}
		if cfg.NativeSubscriptionRoutes && isNativeSubscriptionRoute(r, cfg.SubscriptionPrefixes) {
			if masterProxy == nil {
				http.Error(w, "native Go Master API unavailable", http.StatusServiceUnavailable)
				return
			}
			masterProxy.ServeHTTP(w, r)
			return
		}
		if cfg.NativeNodeRoutes && isNativeNodeRoute(r) {
			if masterProxy == nil {
				http.Error(w, "native Go Master API unavailable", http.StatusServiceUnavailable)
				return
			}
			masterProxy.ServeHTTP(w, r)
			return
		}
		pythonProxy.ServeHTTP(w, r)
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

func isNativeSubscriptionRoute(r *http.Request, prefixes []string) bool {
	if r.Method != http.MethodGet || strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	path := strings.TrimRight(r.URL.Path, "/")
	if path == "/api/v1/client/subscribe" || strings.HasPrefix(path, "/api/v1/client/subscribe/") {
		return true
	}
	if path == "/sub" || strings.HasPrefix(path, "/sub/") {
		return true
	}
	for _, prefix := range prefixes {
		prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
		if prefix == "" {
			continue
		}
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func isNativeAdminRoute(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	path := strings.TrimRight(r.URL.Path, "/")
	switch path {
	case "/api/admin":
		return r.Method == http.MethodGet || r.Method == http.MethodPost
	case "/api/admins":
		return r.Method == http.MethodGet
	case "/api/admin/token", "/admin/token":
		return r.Method == http.MethodPost
	}
	if strings.HasPrefix(path, "/api/admin/") {
		return true
	}
	if path == "/api/myaccount" || strings.HasPrefix(path, "/api/myaccount/") {
		return true
	}
	return false
}

func isNativeUserRoute(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	path := strings.TrimRight(r.URL.Path, "/")
	if path == "/api/users/actions" {
		return r.Method == http.MethodPost
	}
	if path == "/api/users/usage" {
		return r.Method == http.MethodGet
	}
	if isNativeServiceUsersActionRoute(path, r.Method) {
		return true
	}
	if path == "/api/users" {
		return r.Method == http.MethodGet
	}
	if path == "/api/user" || path == "/api/v2/users" {
		return r.Method == http.MethodPost
	}
	if strings.HasPrefix(path, "/api/v2/users/") {
		rest := strings.TrimPrefix(path, "/api/v2/users/")
		return rest != "" && !strings.Contains(rest, "/") && r.Method == http.MethodPut
	}
	if !strings.HasPrefix(path, "/api/user/") {
		return false
	}
	rest := strings.TrimPrefix(path, "/api/user/")
	if rest == "" || strings.Contains(rest, "/") {
		parts := strings.Split(rest, "/")
		if len(parts) != 2 || parts[0] == "" {
			return false
		}
		switch parts[1] {
		case "reset", "revoke_sub", "active-next":
			return r.Method == http.MethodPost
		case "usage":
			return r.Method == http.MethodGet
		default:
			return false
		}
	}
	return r.Method == http.MethodGet || r.Method == http.MethodPut || r.Method == http.MethodDelete
}

func isNativeServiceUsersActionRoute(path string, method string) bool {
	if method != http.MethodPost || !strings.HasPrefix(path, "/api/v2/services/") {
		return false
	}
	rest := strings.TrimPrefix(path, "/api/v2/services/")
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "users" || parts[2] != "actions" {
		return false
	}
	_, err := strconv.ParseInt(parts[0], 10, 64)
	return err == nil
}

func isNativeNodeRoute(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	path := strings.TrimRight(r.URL.Path, "/")
	switch path {
	case "/api/nodes":
		return r.Method == http.MethodGet
	case "/api/nodes/usage":
		return r.Method == http.MethodGet
	}

	if !strings.HasPrefix(path, "/api/node/") {
		return false
	}
	rest := strings.TrimPrefix(path, "/api/node/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		return false
	}
	if _, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
		return false
	}
	suffix := strings.Join(parts[1:], "/")
	switch suffix {
	case "":
		return r.Method == http.MethodGet
	case "reconnect", "restart", "sync", "xray/update", "geo/update", "service/restart", "service/update":
		return r.Method == http.MethodPost
	case "logs", "usage/daily":
		return r.Method == http.MethodGet
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
