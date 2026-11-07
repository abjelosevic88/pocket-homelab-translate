package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-translate/internal/store"
)

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Get("/pocket-ask.js", s.core.AskWidget("translate"))
	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	// the LibreTranslate API, for what was made for it
	r.Method(http.MethodPost, "/translate", s.compat(s.handleCompatTranslate))
	r.Method(http.MethodPost, "/detect", s.compat(s.handleCompatDetect))
	r.Method(http.MethodGet, "/languages", s.compat(s.handleCompatLanguages))

	r.Route("/api/v1", func(r chi.Router) {
		// documents, photos, speech, imports and the AI can take longer than a request may
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/import", s.handleImport)
			r.Post("/translate", s.handleTranslate)
			r.Post("/explain", s.handleExplain)
			r.Post("/ocr", s.handleOCR)
			r.Post("/speech", s.handleSpeech)
			r.Post("/documents", s.handleUploadDocument)
			r.Post("/documents/from", s.handleDocumentFrom)
			r.Post("/documents/{id}/save", s.handleSaveDocument)
			r.Post("/conversations/{id}/turns", s.handleAddTurn)
			r.Post("/turns/{id}/retry", s.handleRetryTurn)
			r.Post("/translations", s.handleAddTranslation)
			r.Post("/translations/{id}/learn", s.handleToLearn)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(120 * time.Second))
			r.Post("/auth/login", s.handleLogin)
			r.Post("/auth/setup", s.handleSetup)
			r.Get("/auth/status", s.handleAuthStatus)
			r.Get("/auth/oidc/start", s.handleOIDCStart)
			r.Get("/auth/oidc/callback", s.handleOIDCCallback)

			r.Group(func(r chi.Router) {
				r.Use(s.requireAuth)
				r.Post("/auth/logout", s.handleLogout)
				r.Get("/auth/me", s.handleMe)
				r.Put("/auth/me", s.handleUpdateMe)

				// translating
				r.Post("/detect", s.handleDetect)
				r.Get("/languages", s.handleLanguages)
				r.Get("/engines", s.handleEngines)

				// history and the phrasebook
				r.Get("/translations", s.handleListTranslations)
				r.Delete("/translations", s.handleClearHistory)
				r.Get("/translations/{id}", s.handleGetTranslation)
				r.Put("/translations/{id}", s.handleUpdateTranslation)
				r.Delete("/translations/{id}", s.handleDeleteTranslation)
				r.Post("/translations/{id}/note", s.handleToNotes)
				r.Get("/collections", s.handleCollections)
				r.Post("/collections/rename", s.handleRenameCollection)

				// the glossary
				r.Get("/glossary", s.handleListTerms)
				r.Post("/glossary", s.handleAddTerm)
				r.Post("/glossary/import", s.handleImportTerms)
				r.Get("/glossary.csv", s.handleExportTerms)
				r.Put("/glossary/{id}", s.handleUpdateTerm)
				r.Delete("/glossary/{id}", s.handleDeleteTerm)

				// documents
				r.Get("/documents", s.handleListDocuments)
				r.Get("/documents/{id}", s.handleGetDocument)
				r.Get("/documents/{id}/original", s.handleDocumentFile("original"))
				r.Get("/documents/{id}/result", s.handleDocumentFile("result"))
				r.Post("/documents/{id}/retry", s.handleRetryDocument)
				r.Delete("/documents/{id}", s.handleDeleteDocument)

				// conversations
				r.Get("/conversations", s.handleListConversations)
				r.Post("/conversations", s.handleAddConversation)
				r.Get("/conversations/{id}", s.handleGetConversation)
				r.Put("/conversations/{id}", s.handleUpdateConversation)
				r.Delete("/conversations/{id}", s.handleDeleteConversation)
				r.Get("/conversations/{id}/transcript.txt", s.handleTranscript)
				r.Post("/conversations/{id}/note", s.handleConversationToNotes)
				r.Delete("/turns/{id}", s.handleDeleteTurn)

				// the other Pocket apps
				r.Get("/apps", s.handleApps)
				r.Get("/apps/files", s.handleBrowseFiles)
				r.Get("/apps/documents", s.handleSearchDocuments)

				// for the rest of the box: Search, Pocket Day, AI
				r.Get("/summary", s.handleSummary)
				r.Get("/search", s.handleSearch)
				r.Get("/day/{date}", s.handleDay)

				// AI
				r.Get("/ai/status", s.handleAIStatus)
				r.Get("/ai/models", s.handleAIModels)

				r.Post("/integrations/test", s.handleIntegrationTest)
				r.Post("/notify/test", s.handleNotifyTest)

				r.Get("/users", s.handleListUsers)
				r.Post("/users", s.handleCreateUser)
				r.Put("/users/{id}", s.handleUpdateUser)
				r.Delete("/users/{id}", s.handleDeleteUser)

				r.Get("/settings", s.handleGetSettings)
				r.Put("/settings", s.handlePutSettings)
				r.Get("/settings/system", s.handleSystemInfo)

				r.Get("/tokens", s.handleListTokens)
				r.Post("/tokens", s.handleCreateToken)
				r.Delete("/tokens/{id}", s.handleDeleteToken)

				r.Get("/backup", s.handleBackup)
				r.Get("/export.json", s.handleExportJSON)
				r.Get("/export.csv", s.handleExportCSV)
			})
		})
	})

	r.NotFound(s.handleSPA)
	return r
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, store.ErrConflict), errors.Is(err, core.ErrOperationConflict):
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func (s *Server) readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(nil, r.Body, s.cfg.MaxBodyBytes))
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	return dec.Decode(v)
}

// decodeKeys decodes JSON into v and also reports which top-level keys were
// present, so PUT handlers can tell "not sent" from "sent empty".
func decodeKeys(r *http.Request, v any) (map[string]bool, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, json.Unmarshal(b, v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// ---------- health / metrics ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"core_outbox": s.coreOutboxHealth(r.Context()), "status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts := s.store.Counts(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_translate_info Build info.\n# TYPE pocket_translate_info gauge\npocket_translate_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_translate_uptime_seconds Process uptime.\n# TYPE pocket_translate_uptime_seconds gauge\npocket_translate_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_translate_http_requests_total HTTP requests served.\n# TYPE pocket_translate_http_requests_total counter\npocket_translate_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_translate_http_errors_total HTTP 5xx responses.\n# TYPE pocket_translate_http_errors_total counter\npocket_translate_http_errors_total %d\n", s.errors.Load())
	fmt.Fprintf(w, "# HELP pocket_translate_rows Rows per table.\n# TYPE pocket_translate_rows gauge\n")
	for k, v := range counts {
		fmt.Fprintf(w, "pocket_translate_rows{table=%q} %d\n", k, v)
	}
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}
