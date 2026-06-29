package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/config"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

type Server struct {
	cfg        config.Config
	store      *store.Store
	logger     *slog.Logger
	httpClient *http.Client
}

func NewServer(cfg config.Config, store *store.Store, logger *slog.Logger) *Server {
	return &Server{
		cfg:        cfg,
		store:      store,
		logger:     logger,
		httpClient: &http.Client{Timeout: cfg.SMLPaperlessTimeout},
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", s.live)
	mux.HandleFunc("GET /health/ready", s.ready)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.Handle("GET /api/auth/me", s.requireAuth(http.HandlerFunc(s.me)))
	mux.Handle("POST /api/auth/logout", s.requireAuth(http.HandlerFunc(s.logout)))
	mux.Handle("GET /api/users", s.requireAdmin(http.HandlerFunc(s.listUsers)))
	mux.Handle("POST /api/users", s.requireAdmin(http.HandlerFunc(s.createUser)))
	mux.Handle("PUT /api/users/{id}", s.requireAdmin(http.HandlerFunc(s.updateUser)))
	mux.Handle("DELETE /api/users/{id}", s.requireAdmin(http.HandlerFunc(s.deactivateUser)))
	mux.Handle("GET /api/sml/doc-formats", s.requireAdmin(http.HandlerFunc(s.listSMLDocFormats)))
	mux.Handle("GET /api/document-configs", s.requireAdmin(http.HandlerFunc(s.listDocumentConfigSteps)))
	mux.Handle("POST /api/document-configs", s.requireAdmin(http.HandlerFunc(s.createDocumentConfigStep)))
	mux.Handle("PUT /api/document-configs/{id}", s.requireAdmin(http.HandlerFunc(s.updateDocumentConfigStep)))
	mux.Handle("DELETE /api/document-configs/{id}", s.requireAdmin(http.HandlerFunc(s.deleteDocumentConfigStep)))

	return s.recover(s.cors(mux))
}

func (s *Server) cors(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, origin := range s.cfg.CORSOrigins {
		allowed[origin] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowed[origin] || allowed["*"]) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	return r.RemoteAddr
}
