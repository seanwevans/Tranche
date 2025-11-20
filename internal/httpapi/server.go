package httpapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"

	"tranche/internal/db"
	"tranche/internal/logging"
)

type Server struct {
	log        *logging.Logger
	db         *db.Queries
	sqlDB      *sql.DB
	r          chi.Router
	adminToken string
}

type authContextKey struct{}

type authContext struct {
	customerID int64
	superuser  bool
}

const maxRequestBodyBytes int64 = 1 << 20 // 1 MiB

func NewServer(log *logging.Logger, conn *sql.DB, dbx *db.Queries, adminToken string) *Server {
	s := &Server{log: log, db: dbx, sqlDB: conn, r: chi.NewRouter(), adminToken: strings.TrimSpace(adminToken)}
	s.routes()
	return s
}

func (s *Server) Router() http.Handler { return s.r }

func (s *Server) routes() {
	s.r.Use(middleware.RequestID)
	s.r.Use(s.loggingMiddleware)
	s.r.Get("/healthz", s.handleHealth)
	s.r.Get("/readyz", s.handleReady)
	s.r.Route("/v1", func(r chi.Router) {
		r.Use(s.authMiddleware)
		r.Route("/services", func(r chi.Router) {
			r.Get("/", s.handleListServices)
			r.Post("/", s.handleCreateService)

			r.Route("/{serviceID}", func(r chi.Router) {
				r.Get("/", s.handleGetService)
				r.Patch("/", s.handleUpdateService)
				r.Delete("/", s.handleDeleteService)

				r.Route("/domains", func(r chi.Router) {
					r.Get("/", s.handleListDomains)
					r.Post("/", s.handleCreateDomain)
					r.Delete("/{domainID}", s.handleDeleteDomain)
				})

				r.Route("/storm-policies", func(r chi.Router) {
					r.Get("/", s.handleListStormPolicies)
					r.Post("/", s.handleCreateStormPolicy)
					r.Patch("/{policyID}", s.handleUpdateStormPolicy)
					r.Delete("/{policyID}", s.handleDeleteStormPolicy)
				})
			})
		})
	})
}

var (
	errUnauthenticated      = errors.New("authentication required")
	errCustomerScopeMissing = errors.New("customer scope required")
)

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		baseLogger := logging.FromContext(r.Context(), s.log)
		token := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = strings.TrimSpace(token[7:])
		}
		if token == "" {
			token = strings.TrimSpace(r.Header.Get("X-API-Key"))
		}
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing API token", nil)
			return
		}

		if s.adminToken != "" && token == s.adminToken {
			customerID, err := s.extractCustomerID(r)
			if err != nil {
				if errors.Is(err, errCustomerScopeMissing) {
					writeError(w, http.StatusBadRequest, "customer_id is required for admin requests", nil)
					return
				}
				writeError(w, http.StatusBadRequest, err.Error(), nil)
				return
			}
			ctx := context.WithValue(r.Context(), authContextKey{}, authContext{customerID: customerID, superuser: true})
			ctx = logging.ContextWithLogger(ctx, baseLogger.WithCustomerID(customerID))
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		hash := hashToken(token)
		customerID, err := s.db.GetCustomerIDForToken(r.Context(), hash)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, "invalid API token", nil)
				return
			}
			baseLogger.Error("GetCustomerIDForToken failed", "error", err)
			writeError(w, http.StatusInternalServerError, "authentication failed", nil)
			return
		}

		ctx := context.WithValue(r.Context(), authContextKey{}, authContext{customerID: customerID})
		ctx = logging.ContextWithLogger(ctx, baseLogger.WithCustomerID(customerID))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := middleware.GetReqID(r.Context())
		logger := s.log.WithRequestID(reqID)
		ctx := logging.ContextWithLogger(r.Context(), logger)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Server) extractCustomerID(r *http.Request) (int64, error) {
	header := strings.TrimSpace(r.Header.Get("X-Customer-ID"))
	if header == "" {
		header = strings.TrimSpace(r.URL.Query().Get("customer_id"))
	}
	if header == "" {
		return 0, errCustomerScopeMissing
	}
	id, err := strconv.ParseInt(header, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid customer_id")
	}
	return id, nil
}

func (s *Server) customerIDFromContext(ctx context.Context) (int64, error) {
	val := ctx.Value(authContextKey{})
	if val == nil {
		return 0, errUnauthenticated
	}
	info, ok := val.(authContext)
	if !ok {
		return 0, errUnauthenticated
	}
	if info.customerID == 0 {
		return 0, errCustomerScopeMissing
	}
	return info.customerID, nil
}

func (s *Server) requireCustomerID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	customerID, err := s.customerIDFromContext(r.Context())
	if err != nil {
		switch {
		case errors.Is(err, errUnauthenticated):
			writeError(w, http.StatusUnauthorized, err.Error(), nil)
		case errors.Is(err, errCustomerScopeMissing):
			writeError(w, http.StatusBadRequest, err.Error(), nil)
		default:
			writeError(w, http.StatusInternalServerError, "failed to read auth context", nil)
		}
		return 0, false
	}
	return customerID, true
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := db.Ready(ctx, s.sqlDB); err != nil {
		s.log.WithRequestID(middleware.GetReqID(r.Context())).Error("readyz failed", "error", err.Error())
		writeError(w, http.StatusServiceUnavailable, "not ready", map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	customerID, ok := s.requireCustomerID(w, r)
	if !ok {
		return
	}
	services, err := s.db.GetActiveServicesForCustomer(ctx, customerID)
	if err != nil {
		s.log.Printf("GetActiveServicesForCustomer: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list services", nil)
		return
	}
	writeJSON(w, http.StatusOK, services)
}

func (s *Server) handleCreateService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	customerID, ok := s.requireCustomerID(w, r)
	if !ok {
		return
	}
	var req createServiceRequest
	if err := decodeJSON(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes), &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload", err)
		return
	}
	svc, err := s.db.InsertService(ctx, db.InsertServiceParams{
		CustomerID: customerID,
		Name:       req.Name,
		PrimaryCdn: req.PrimaryCDN,
		BackupCdn:  req.BackupCDN,
	})
	if err != nil {
		s.log.Printf("InsertService: %v", err)
		writeDBError(w, err, "failed to create service")
		return
	}
	writeJSON(w, http.StatusCreated, svc)
}

func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serviceID, err := parseIDParam(chi.URLParam(r, "serviceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	customerID, ok := s.requireCustomerID(w, r)
	if !ok {
		return
	}
	svc, err := s.db.GetServiceForCustomer(ctx, db.GetServiceForCustomerParams{ID: serviceID, CustomerID: customerID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "service not found", nil)
			return
		}
		s.log.Printf("GetServiceForCustomer: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load service", nil)
		return
	}
	domains, err := s.db.GetServiceDomains(ctx, svc.ID)
	if err != nil {
		s.log.Printf("GetServiceDomains: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load domains", nil)
		return
	}
	policies, err := s.db.GetStormPoliciesForService(ctx, svc.ID)
	if err != nil {
		s.log.Printf("GetStormPoliciesForService: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load storm policies", nil)
		return
	}
	writeJSON(w, http.StatusOK, serviceDetailResponse{
		Service:       svc,
		Domains:       domains,
		StormPolicies: policies,
	})
}

func (s *Server) handleUpdateService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serviceID, err := parseIDParam(chi.URLParam(r, "serviceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	customerID, ok := s.requireCustomerID(w, r)
	if !ok {
		return
	}
	svc, err := s.db.GetServiceForCustomer(ctx, db.GetServiceForCustomerParams{ID: serviceID, CustomerID: customerID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "service not found", nil)
			return
		}
		s.log.Printf("GetServiceForCustomer: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load service", nil)
		return
	}
	var req updateServiceRequest
	if err := decodeJSON(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes), &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload", err)
		return
	}
	updated := req.Apply(svc)
	svc, err = s.db.UpdateService(ctx, db.UpdateServiceParams{
		ID:         svc.ID,
		CustomerID: svc.CustomerID,
		Name:       updated.Name,
		PrimaryCdn: updated.PrimaryCdn,
		BackupCdn:  updated.BackupCdn,
	})
	if err != nil {
		s.log.Printf("UpdateService: %v", err)
		writeDBError(w, err, "failed to update service")
		return
	}
	writeJSON(w, http.StatusOK, svc)
}

func (s *Server) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serviceID, err := parseIDParam(chi.URLParam(r, "serviceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	customerID, ok := s.requireCustomerID(w, r)
	if !ok {
		return
	}
	_, err = s.db.SoftDeleteService(ctx, db.SoftDeleteServiceParams{ID: serviceID, CustomerID: customerID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "service not found", nil)
			return
		}
		s.log.Printf("SoftDeleteService: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete service", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListDomains(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceContext(w, r)
	if !ok {
		return
	}
	domains, err := s.db.GetServiceDomains(r.Context(), svc.ID)
	if err != nil {
		s.log.Printf("GetServiceDomains: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list domains", nil)
		return
	}
	writeJSON(w, http.StatusOK, domains)
}

func (s *Server) handleCreateDomain(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceContext(w, r)
	if !ok {
		return
	}
	var req domainRequest
	if err := decodeJSON(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes), &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload", err)
		return
	}
	domain, err := s.db.InsertServiceDomain(r.Context(), db.InsertServiceDomainParams{
		ServiceID: svc.ID,
		Name:      req.Name,
	})
	if err != nil {
		s.log.Printf("InsertServiceDomain: %v", err)
		writeDBError(w, err, "failed to add domain")
		return
	}
	writeJSON(w, http.StatusCreated, domain)
}

func (s *Server) handleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceContext(w, r)
	if !ok {
		return
	}
	domainID, err := parseIDParam(chi.URLParam(r, "domainID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	_, err = s.db.DeleteServiceDomain(r.Context(), db.DeleteServiceDomainParams{ID: domainID, ServiceID: svc.ID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "domain not found", nil)
			return
		}
		s.log.Printf("DeleteServiceDomain: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete domain", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListStormPolicies(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceContext(w, r)
	if !ok {
		return
	}
	policies, err := s.db.GetStormPoliciesForService(r.Context(), svc.ID)
	if err != nil {
		s.log.Printf("GetStormPoliciesForService: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list storm policies", nil)
		return
	}
	writeJSON(w, http.StatusOK, policies)
}

func (s *Server) handleCreateStormPolicy(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceContext(w, r)
	if !ok {
		return
	}
	var req stormPolicyRequest
	if err := decodeJSON(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes), &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload", err)
		return
	}
	policy, err := s.db.InsertStormPolicy(r.Context(), req.ToInsertParams(svc.ID))
	if err != nil {
		s.log.Printf("InsertStormPolicy: %v", err)
		writeDBError(w, err, "failed to create storm policy")
		return
	}
	writeJSON(w, http.StatusCreated, policy)
}

func (s *Server) handleUpdateStormPolicy(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceContext(w, r)
	if !ok {
		return
	}
	policyID, err := parseIDParam(chi.URLParam(r, "policyID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	existing, err := s.db.GetStormPolicyForService(r.Context(), db.GetStormPolicyForServiceParams{ID: policyID, ServiceID: svc.ID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "storm policy not found", nil)
			return
		}
		s.log.Printf("GetStormPolicyForService: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load storm policy", nil)
		return
	}
	var req stormPolicyPatchRequest
	if err := decodeJSON(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes), &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload", err)
		return
	}
	params := req.Apply(existing)
	params.ID = existing.ID
	params.ServiceID = existing.ServiceID
	policy, err := s.db.UpdateStormPolicy(r.Context(), params)
	if err != nil {
		s.log.Printf("UpdateStormPolicy: %v", err)
		writeDBError(w, err, "failed to update storm policy")
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) handleDeleteStormPolicy(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.requireServiceContext(w, r)
	if !ok {
		return
	}
	policyID, err := parseIDParam(chi.URLParam(r, "policyID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}
	_, err = s.db.DeleteStormPolicy(r.Context(), db.DeleteStormPolicyParams{ID: policyID, ServiceID: svc.ID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "storm policy not found", nil)
			return
		}
		s.log.Printf("DeleteStormPolicy: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete storm policy", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) requireServiceContext(w http.ResponseWriter, r *http.Request) (db.Service, bool) {
	ctx := r.Context()
	serviceID, err := parseIDParam(chi.URLParam(r, "serviceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), nil)
		return db.Service{}, false
	}
	customerID, ok := s.requireCustomerID(w, r)
	if !ok {
		return db.Service{}, false
	}
	svc, err := s.db.GetServiceForCustomer(ctx, db.GetServiceForCustomerParams{ID: serviceID, CustomerID: customerID})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "service not found", nil)
			return db.Service{}, false
		}
		s.log.Printf("GetServiceForCustomer: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load service", nil)
		return db.Service{}, false
	}
	return svc, true
}

func parseIDParam(raw string) (int64, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, errors.New("missing id parameter")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id parameter")
	}
	return id, nil
}

func decodeJSON(body io.ReadCloser, dst any) error {
	defer body.Close()
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is required")
		}
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("body must contain a single JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string, details map[string]string) {
	writeJSON(w, status, errorResponse{Error: message, Details: details})
}

func writeDBError(w http.ResponseWriter, err error, fallback string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			writeError(w, http.StatusConflict, pgErr.Message, nil)
			return
		case "23503":
			writeError(w, http.StatusBadRequest, "related record missing", nil)
			return
		}
	}
	writeError(w, http.StatusInternalServerError, fallback, nil)
}

type errorResponse struct {
	Error   string            `json:"error"`
	Details map[string]string `json:"details,omitempty"`
}

type serviceDetailResponse struct {
	Service       db.Service         `json:"service"`
	Domains       []db.ServiceDomain `json:"domains"`
	StormPolicies []db.StormPolicy   `json:"storm_policies"`
}

type createServiceRequest struct {
	Name       string `json:"name"`
	PrimaryCDN string `json:"primary_cdn"`
	BackupCDN  string `json:"backup_cdn"`
}

func (r createServiceRequest) Validate() map[string]string {
	errs := map[string]string{}
	if strings.TrimSpace(r.Name) == "" {
		errs["name"] = "cannot be blank"
	}
	if strings.TrimSpace(r.PrimaryCDN) == "" {
		errs["primary_cdn"] = "cannot be blank"
	}
	if strings.TrimSpace(r.BackupCDN) == "" {
		errs["backup_cdn"] = "cannot be blank"
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

type updateServiceRequest struct {
	Name       *string `json:"name"`
	PrimaryCDN *string `json:"primary_cdn"`
	BackupCDN  *string `json:"backup_cdn"`
}

func (r updateServiceRequest) Validate() map[string]string {
	if r.Name == nil && r.PrimaryCDN == nil && r.BackupCDN == nil {
		return map[string]string{"body": "at least one field is required"}
	}
	errs := map[string]string{}
	if r.Name != nil && strings.TrimSpace(*r.Name) == "" {
		errs["name"] = "cannot be blank"
	}
	if r.PrimaryCDN != nil && strings.TrimSpace(*r.PrimaryCDN) == "" {
		errs["primary_cdn"] = "cannot be blank"
	}
	if r.BackupCDN != nil && strings.TrimSpace(*r.BackupCDN) == "" {
		errs["backup_cdn"] = "cannot be blank"
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func (r updateServiceRequest) Apply(svc db.Service) db.Service {
	if r.Name != nil {
		svc.Name = strings.TrimSpace(*r.Name)
	}
	if r.PrimaryCDN != nil {
		svc.PrimaryCdn = strings.TrimSpace(*r.PrimaryCDN)
	}
	if r.BackupCDN != nil {
		svc.BackupCdn = strings.TrimSpace(*r.BackupCDN)
	}
	return svc
}

type domainRequest struct {
	Name string `json:"name"`
}

func (r domainRequest) Validate() map[string]string {
	if strings.TrimSpace(r.Name) == "" {
		return map[string]string{"name": "cannot be blank"}
	}
	return nil
}

type stormPolicyRequest struct {
	Kind              string  `json:"kind"`
	ThresholdAvail    float64 `json:"threshold_avail"`
	WindowSeconds     int32   `json:"window_seconds"`
	CooldownSeconds   int32   `json:"cooldown_seconds"`
	MaxCoverageFactor float64 `json:"max_coverage_factor"`
}

func (r stormPolicyRequest) Validate() map[string]string {
	errs := map[string]string{}
	if strings.TrimSpace(r.Kind) == "" {
		errs["kind"] = "cannot be blank"
	}
	if r.ThresholdAvail <= 0 || r.ThresholdAvail > 1 {
		errs["threshold_avail"] = "must be between 0 and 1"
	}
	if r.WindowSeconds <= 0 {
		errs["window_seconds"] = "must be positive"
	}
	if r.CooldownSeconds < 0 {
		errs["cooldown_seconds"] = "cannot be negative"
	}
	if r.MaxCoverageFactor <= 0 {
		errs["max_coverage_factor"] = "must be positive"
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func (r stormPolicyRequest) ToInsertParams(serviceID int64) db.InsertStormPolicyParams {
	return db.InsertStormPolicyParams{
		ServiceID:         serviceID,
		Kind:              strings.TrimSpace(r.Kind),
		ThresholdAvail:    r.ThresholdAvail,
		WindowSeconds:     r.WindowSeconds,
		CooldownSeconds:   r.CooldownSeconds,
		MaxCoverageFactor: r.MaxCoverageFactor,
	}
}

type stormPolicyPatchRequest struct {
	Kind              *string  `json:"kind"`
	ThresholdAvail    *float64 `json:"threshold_avail"`
	WindowSeconds     *int32   `json:"window_seconds"`
	CooldownSeconds   *int32   `json:"cooldown_seconds"`
	MaxCoverageFactor *float64 `json:"max_coverage_factor"`
}

func (r stormPolicyPatchRequest) Validate() map[string]string {
	if r.Kind == nil && r.ThresholdAvail == nil && r.WindowSeconds == nil && r.CooldownSeconds == nil && r.MaxCoverageFactor == nil {
		return map[string]string{"body": "at least one field is required"}
	}
	errs := map[string]string{}
	if r.Kind != nil && strings.TrimSpace(*r.Kind) == "" {
		errs["kind"] = "cannot be blank"
	}
	if r.ThresholdAvail != nil {
		if *r.ThresholdAvail <= 0 || *r.ThresholdAvail > 1 {
			errs["threshold_avail"] = "must be between 0 and 1"
		}
	}
	if r.WindowSeconds != nil && *r.WindowSeconds <= 0 {
		errs["window_seconds"] = "must be positive"
	}
	if r.CooldownSeconds != nil && *r.CooldownSeconds < 0 {
		errs["cooldown_seconds"] = "cannot be negative"
	}
	if r.MaxCoverageFactor != nil && *r.MaxCoverageFactor <= 0 {
		errs["max_coverage_factor"] = "must be positive"
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func (r stormPolicyPatchRequest) Apply(existing db.StormPolicy) db.UpdateStormPolicyParams {
	if r.Kind != nil {
		existing.Kind = strings.TrimSpace(*r.Kind)
	}
	if r.ThresholdAvail != nil {
		existing.ThresholdAvail = *r.ThresholdAvail
	}
	if r.WindowSeconds != nil {
		existing.WindowSeconds = *r.WindowSeconds
	}
	if r.CooldownSeconds != nil {
		existing.CooldownSeconds = *r.CooldownSeconds
	}
	if r.MaxCoverageFactor != nil {
		existing.MaxCoverageFactor = *r.MaxCoverageFactor
	}
	return db.UpdateStormPolicyParams{
		ID:                existing.ID,
		ServiceID:         existing.ServiceID,
		Kind:              existing.Kind,
		ThresholdAvail:    existing.ThresholdAvail,
		WindowSeconds:     existing.WindowSeconds,
		CooldownSeconds:   existing.CooldownSeconds,
		MaxCoverageFactor: existing.MaxCoverageFactor,
	}
}
