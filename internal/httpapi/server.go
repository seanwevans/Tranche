package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"tranche/internal/db"
)

type Server struct {
	log Logger
	db  *db.Queries
	r   chi.Router
}

type Logger interface {
	Printf(string, ...any)
	Println(...any)
	Fatalf(string, ...any)
}

func NewServer(log Logger, dbx *db.Queries) *Server {
	s := &Server{log: log, db: dbx, r: chi.NewRouter()}
	s.routes()
	return s
}

func (s *Server) Router() http.Handler { return s.r }

func (s *Server) routes() {
	s.r.Get("/healthz", s.handleHealth)
	s.r.Get("/v1/services", s.handleListServices)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	services, err := s.db.GetActiveServices(ctx)
	if err != nil {
		s.log.Printf("GetActiveServices: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(services)
}
