package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	httpswagger "github.com/swaggo/http-swagger"

	"github.com/ponchik327/subscriptions-service/internal/domain"
	appmiddleware "github.com/ponchik327/subscriptions-service/internal/middleware"
	"github.com/ponchik327/subscriptions-service/internal/service"
)

//go:generate mockery --name SubscriptionService --output ../mocks --outpkg mocks --case snake --with-expecter

// SubscriptionService is the interface the handler layer depends on.
type SubscriptionService interface {
	Create(ctx context.Context, sub domain.Subscription) (domain.Subscription, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.Subscription, error)
	Update(ctx context.Context, sub domain.Subscription) (domain.Subscription, error)
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, filter domain.ListFilter) ([]domain.Subscription, error)
	Summary(ctx context.Context, p service.SummaryParams) (domain.SummaryResult, error)
}

// Pinger is satisfied by *pgxpool.Pool.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	svc      SubscriptionService
	db       Pinger
	logger   *slog.Logger
	validate *validator.Validate
}

// NewRouter creates and returns the chi router wired to the given service.
// Both main.go and e2e tests call this function so the router is identical.
func NewRouter(svc SubscriptionService, db Pinger, logger *slog.Logger) http.Handler {
	h := &Handler{
		svc:      svc,
		db:       db,
		logger:   logger,
		validate: validator.New(),
	}

	r := chi.NewRouter()
	r.Use(appmiddleware.RequestID)
	r.Use(appmiddleware.Logger(logger))
	r.Use(appmiddleware.Recoverer(logger))
	r.Use(chimiddleware.StripSlashes)

	r.Get("/healthz", h.Healthz)
	r.Get("/swagger/*", httpswagger.Handler())

	r.Route("/subscriptions", func(r chi.Router) {
		r.Get("/summary", h.Summary)
		r.Post("/", h.Create)
		r.Get("/", h.List)
		r.Get("/{id}", h.GetByID)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
	})

	return r
}

// errResponse is the canonical error format.
type errResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg, code string) {
	writeJSON(w, status, errResponse{Error: msg, Code: code})
}

func parseUUID(w http.ResponseWriter, s string) (uuid.UUID, bool) {
	id, err := uuid.Parse(s)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid UUID", "INVALID_UUID")
		return uuid.UUID{}, false
	}
	return id, true
}

func notFoundOrInternal(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found", "NOT_FOUND")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
}

// Healthz godoc
// @Summary      Health check
// @Description  Returns 200 if the service is up and the DB is reachable, 503 otherwise
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /healthz [get]
func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	if err := h.db.Ping(r.Context()); err != nil {
		h.logger.ErrorContext(r.Context(), "healthz db ping failed", slog.String("err", err.Error()))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── DTOs ─────────────────────────────────────────────────────────────────────

type createRequest struct {
	ServiceName string            `json:"service_name" validate:"required"`
	Price       int               `json:"price"        validate:"min=0"`
	UserID      uuid.UUID         `json:"user_id"      validate:"required"`
	StartDate   domain.MonthYear  `json:"start_date"   validate:"required"`
	EndDate     *domain.MonthYear `json:"end_date"`
}

type updateRequest struct {
	ServiceName string            `json:"service_name" validate:"required"`
	Price       int               `json:"price"        validate:"min=0"`
	UserID      uuid.UUID         `json:"user_id"      validate:"required"`
	StartDate   domain.MonthYear  `json:"start_date"   validate:"required"`
	EndDate     *domain.MonthYear `json:"end_date"`
}

// Create godoc
// @Summary      Create subscription
// @Description  Creates a new subscription and returns it with the generated id
// @Tags         subscriptions
// @Accept       json
// @Produce      json
// @Param        body  body      createRequest         true  "Subscription payload"
// @Success      201   {object}  domain.Subscription
// @Failure      400   {object}  errResponse
// @Failure      500   {object}  errResponse
// @Router       /subscriptions [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "INVALID_JSON")
		return
	}

	if req.StartDate.IsZero() {
		writeError(w, http.StatusBadRequest, "start_date is required", "MISSING_START_DATE")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "VALIDATION_ERROR")
		return
	}

	if req.EndDate != nil && !req.EndDate.IsZero() && req.EndDate.Before(req.StartDate.Time) {
		writeError(w, http.StatusBadRequest, "end_date must be >= start_date", "INVALID_DATE_RANGE")
		return
	}

	sub := domain.Subscription{
		ServiceName: req.ServiceName,
		Price:       req.Price,
		UserID:      req.UserID,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
	}

	created, err := h.svc.Create(r.Context(), sub)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "Create subscription", slog.String("err", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// GetByID godoc
// @Summary      Get subscription by ID
// @Tags         subscriptions
// @Produce      json
// @Param        id   path      string              true  "Subscription UUID"
// @Success      200  {object}  domain.Subscription
// @Failure      400  {object}  errResponse
// @Failure      404  {object}  errResponse
// @Failure      500  {object}  errResponse
// @Router       /subscriptions/{id} [get]
func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	sub, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		notFoundOrInternal(w, err)
		return
	}

	writeJSON(w, http.StatusOK, sub)
}

// Update godoc
// @Summary      Update subscription
// @Tags         subscriptions
// @Accept       json
// @Produce      json
// @Param        id    path      string              true  "Subscription UUID"
// @Param        body  body      updateRequest       true  "Updated subscription"
// @Success      200   {object}  domain.Subscription
// @Failure      400   {object}  errResponse
// @Failure      404   {object}  errResponse
// @Failure      500   {object}  errResponse
// @Router       /subscriptions/{id} [put]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error(), "INVALID_JSON")
		return
	}

	if req.StartDate.IsZero() {
		writeError(w, http.StatusBadRequest, "start_date is required", "MISSING_START_DATE")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "VALIDATION_ERROR")
		return
	}

	if req.EndDate != nil && !req.EndDate.IsZero() && req.EndDate.Before(req.StartDate.Time) {
		writeError(w, http.StatusBadRequest, "end_date must be >= start_date", "INVALID_DATE_RANGE")
		return
	}

	sub := domain.Subscription{
		ID:          id,
		ServiceName: req.ServiceName,
		Price:       req.Price,
		UserID:      req.UserID,
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
	}

	updated, err := h.svc.Update(r.Context(), sub)
	if err != nil {
		notFoundOrInternal(w, err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// Delete godoc
// @Summary      Delete subscription
// @Tags         subscriptions
// @Param        id  path  string  true  "Subscription UUID"
// @Success      204
// @Failure      400  {object}  errResponse
// @Failure      404  {object}  errResponse
// @Failure      500  {object}  errResponse
// @Router       /subscriptions/{id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUID(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		notFoundOrInternal(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// List godoc
// @Summary      List subscriptions
// @Tags         subscriptions
// @Produce      json
// @Param        limit         query     int     false  "Page size"       default(50)
// @Param        offset        query     int     false  "Page offset"     default(0)
// @Param        user_id       query     string  false  "Filter by user UUID"
// @Param        service_name  query     string  false  "Filter by service name"
// @Success      200  {array}   domain.Subscription
// @Failure      400  {object}  errResponse
// @Failure      500  {object}  errResponse
// @Router       /subscriptions [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit := uint64(50)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if len(raw) > 0 && raw[0] == '-' {
			writeError(w, http.StatusBadRequest, "limit must be >= 0", "INVALID_LIMIT")
			return
		}
		if parsed, err := strconv.ParseUint(raw, 10, 64); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := uint64(0)
	if raw := r.URL.Query().Get("offset"); raw != "" && raw[0] != '-' {
		if parsed, err := strconv.ParseUint(raw, 10, 64); err == nil {
			offset = parsed
		}
	}

	filter := domain.ListFilter{Limit: limit, Offset: offset}

	if raw := r.URL.Query().Get("user_id"); raw != "" {
		uid, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user_id UUID", "INVALID_UUID")
			return
		}
		filter.UserID = &uid
	}

	if sn := r.URL.Query().Get("service_name"); sn != "" {
		filter.ServiceName = &sn
	}

	subs, err := h.svc.List(r.Context(), filter)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "List subscriptions", slog.String("err", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	if subs == nil {
		subs = []domain.Subscription{}
	}
	writeJSON(w, http.StatusOK, subs)
}

// Summary godoc
// @Summary      Aggregate subscription costs
// @Description  Returns total cost for subscriptions overlapping the given period
// @Tags         subscriptions
// @Produce      json
// @Param        from          query     string  true   "Period start (MM-YYYY)"
// @Param        to            query     string  true   "Period end (MM-YYYY)"
// @Param        user_id       query     string  false  "Filter by user UUID"
// @Param        service_name  query     string  false  "Filter by service name"
// @Success      200  {object}  domain.SummaryResult
// @Failure      400  {object}  errResponse
// @Failure      500  {object}  errResponse
// @Router       /subscriptions/summary [get]
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	rawFrom := r.URL.Query().Get("from")
	rawTo := r.URL.Query().Get("to")

	if rawFrom == "" || rawTo == "" {
		writeError(w, http.StatusBadRequest, "from and to are required", "MISSING_PARAMS")
		return
	}

	from, err := domain.ParseMonthYear(rawFrom)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from: "+err.Error(), "INVALID_DATE")
		return
	}

	to, err := domain.ParseMonthYear(rawTo)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid to: "+err.Error(), "INVALID_DATE")
		return
	}

	if from.After(to.Time) {
		writeError(w, http.StatusBadRequest, "from must be <= to", "INVALID_DATE_RANGE")
		return
	}

	params := service.SummaryParams{From: from, To: to}

	if raw := r.URL.Query().Get("user_id"); raw != "" {
		uid, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user_id UUID", "INVALID_UUID")
			return
		}
		params.UserID = &uid
	}

	if sn := r.URL.Query().Get("service_name"); sn != "" {
		params.ServiceName = &sn
	}

	result, err := h.svc.Summary(r.Context(), params)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "Summary", slog.String("err", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
