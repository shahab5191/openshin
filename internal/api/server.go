package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/shahab5191/openshin/internal/store"
)

// Server exposes the agent-agnostic ingest and read API over HTTP. It is the
// contract every agent adapter (hooks, transcript tailer, MCP) ultimately
// reduces to.
type Server struct {
	store *store.Conversations
}

func NewServer(s *store.Conversations) *Server {
	return &Server{store: s}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/ingest", s.handleIngest())
	mux.HandleFunc("GET /v1/threads/{id}/messages", s.handleReadRange())
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	return logRequests(mux)
}

// ListenAndServe runs the HTTP server with sane timeouts. It only returns on
// error.
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return srv.ListenAndServe()
}

type ingestRequest struct {
	SessionID      string `json:"session_id"`
	ThreadID       string `json:"thread_id"`
	ParentThreadID string `json:"parent_thread_id"`
	TaskID         string `json:"task_id"`
	Kind           string `json:"kind"`
	Brief          string `json:"brief"`
	Actor          string `json:"actor"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	SourceRef      string `json:"source_ref"`
	CreatedAt      string `json:"created_at"`
}

func (s *Server) handleIngest() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ingestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}

		msg, err := s.store.AppendMessage(r.Context(), store.IngestMessage{
			SessionID:      req.SessionID,
			ThreadID:       req.ThreadID,
			ParentThreadID: req.ParentThreadID,
			TaskID:         req.TaskID,
			Kind:           req.Kind,
			Brief:          req.Brief,
			Actor:          req.Actor,
			Role:           req.Role,
			Content:        req.Content,
			SourceRef:      req.SourceRef,
			CreatedAt:      req.CreatedAt,
		})
		if err != nil {
			slog.Error("ingest failed", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusCreated, msg)
	}
}

func (s *Server) handleReadRange() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		threadID := r.PathValue("id")
		if threadID == "" {
			http.Error(w, "thread id is required", http.StatusBadRequest)
			return
		}

		start, end, err := parseRange(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		msgs, err := s.store.ReadRange(r.Context(), threadID, start, end)
		if err != nil {
			slog.Error("read range failed", "thread", threadID, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"thread_id": threadID,
			"start_seq": start,
			"end_seq":   end,
			"messages":  msgs,
		})
	}
}

func parseRange(r *http.Request) (int64, int64, error) {
	start, err := parseInt(r.URL.Query().Get("start_seq"), 1)
	if err != nil {
		return 0, 0, errors.New("start_seq must be an integer")
	}
	end, err := parseInt(r.URL.Query().Get("end_seq"), 1<<62)
	if err != nil {
		return 0, 0, errors.New("end_seq must be an integer")
	}
	if start > end {
		return 0, 0, fmt.Errorf("start_seq %d is after end_seq %d", start, end)
	}
	return start, end, nil
}

func parseInt(raw string, def int64) (int64, error) {
	if raw == "" {
		return def, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start))
	})
}
