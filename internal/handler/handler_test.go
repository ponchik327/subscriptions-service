package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ponchik327/subscriptions-service/internal/domain"
	"github.com/ponchik327/subscriptions-service/internal/handler"
	"github.com/ponchik327/subscriptions-service/internal/mocks"
	"github.com/ponchik327/subscriptions-service/internal/service"
)

// helpers

type okPinger struct{}

func (okPinger) Ping(_ context.Context) error { return nil }

func newRouter(svc handler.SubscriptionService) http.Handler {
	return handler.NewRouter(svc, okPinger{}, slog.Default())
}

func doRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, dest any) {
	t.Helper()
	require.NoError(t, json.NewDecoder(rr.Body).Decode(dest))
}

type errResp struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func makeTestSub() domain.Subscription {
	return domain.Subscription{
		ID:          uuid.New(),
		ServiceName: "Netflix",
		Price:       800,
		UserID:      uuid.New(),
		StartDate:   domain.NewMonthYear(1, 2025),
	}
}

// ── POST /subscriptions ───────────────────────────────────────────────────────

func TestHandler_Create_Success(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	sub := makeTestSub()
	svc.EXPECT().Create(mock.Anything, mock.MatchedBy(func(s domain.Subscription) bool {
		return s.ServiceName == "Netflix" && s.Price == 800
	})).Return(sub, nil)

	rr := doRequest(t, newRouter(svc), http.MethodPost, "/subscriptions", map[string]any{
		"service_name": "Netflix",
		"price":        800,
		"user_id":      sub.UserID.String(),
		"start_date":   "01-2025",
	})

	assert.Equal(t, http.StatusCreated, rr.Code)
	var got domain.Subscription
	decodeJSON(t, rr, &got)
	assert.Equal(t, sub.ID, got.ID)
	assert.Equal(t, "Netflix", got.ServiceName)
}

func TestHandler_Create_InvalidJSON(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	req := httptest.NewRequest(http.MethodPost, "/subscriptions", bytes.NewBufferString("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var e errResp
	decodeJSON(t, rr, &e)
	assert.Equal(t, "INVALID_JSON", e.Code)
}

func TestHandler_Create_InvalidStartDate(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	rr := doRequest(t, newRouter(svc), http.MethodPost, "/subscriptions", map[string]any{
		"service_name": "Netflix",
		"price":        800,
		"user_id":      uuid.New().String(),
		"start_date":   "bad-date",
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandler_Create_MissingServiceName(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	rr := doRequest(t, newRouter(svc), http.MethodPost, "/subscriptions", map[string]any{
		"price":      800,
		"user_id":    uuid.New().String(),
		"start_date": "01-2025",
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var e errResp
	decodeJSON(t, rr, &e)
	assert.Equal(t, "VALIDATION_ERROR", e.Code)
}

func TestHandler_Create_EndDateBeforeStartDate(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	rr := doRequest(t, newRouter(svc), http.MethodPost, "/subscriptions", map[string]any{
		"service_name": "Netflix",
		"price":        800,
		"user_id":      uuid.New().String(),
		"start_date":   "06-2025",
		"end_date":     "01-2025",
	})
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var e errResp
	decodeJSON(t, rr, &e)
	assert.Equal(t, "INVALID_DATE_RANGE", e.Code)
}

func TestHandler_Create_ServiceError(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	svc.EXPECT().Create(mock.Anything, mock.Anything).Return(domain.Subscription{}, assert.AnError)

	rr := doRequest(t, newRouter(svc), http.MethodPost, "/subscriptions", map[string]any{
		"service_name": "Netflix",
		"price":        800,
		"user_id":      uuid.New().String(),
		"start_date":   "01-2025",
	})
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// ── GET /subscriptions/{id} ───────────────────────────────────────────────────

func TestHandler_GetByID_Success(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	sub := makeTestSub()
	svc.EXPECT().GetByID(mock.Anything, sub.ID).Return(sub, nil)

	rr := doRequest(t, newRouter(svc), http.MethodGet, "/subscriptions/"+sub.ID.String(), nil)
	assert.Equal(t, http.StatusOK, rr.Code)
	var got domain.Subscription
	decodeJSON(t, rr, &got)
	assert.Equal(t, sub.ID, got.ID)
}

func TestHandler_GetByID_InvalidUUID(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	rr := doRequest(t, newRouter(svc), http.MethodGet, "/subscriptions/not-a-uuid", nil)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var e errResp
	decodeJSON(t, rr, &e)
	assert.Equal(t, "INVALID_UUID", e.Code)
}

func TestHandler_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	id := uuid.New()
	svc.EXPECT().GetByID(mock.Anything, id).Return(domain.Subscription{}, service.ErrNotFound)

	rr := doRequest(t, newRouter(svc), http.MethodGet, "/subscriptions/"+id.String(), nil)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// ── PUT /subscriptions/{id} ───────────────────────────────────────────────────

func TestHandler_Update_Success(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	sub := makeTestSub()
	svc.EXPECT().Update(mock.Anything, mock.MatchedBy(func(s domain.Subscription) bool {
		return s.ID == sub.ID && s.ServiceName == "Updated"
	})).Return(sub, nil)

	rr := doRequest(t, newRouter(svc), http.MethodPut, "/subscriptions/"+sub.ID.String(), map[string]any{
		"service_name": "Updated",
		"price":        900,
		"user_id":      sub.UserID.String(),
		"start_date":   "01-2025",
	})
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandler_Update_NotFound(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	id := uuid.New()
	svc.EXPECT().Update(mock.Anything, mock.Anything).Return(domain.Subscription{}, service.ErrNotFound)

	rr := doRequest(t, newRouter(svc), http.MethodPut, "/subscriptions/"+id.String(), map[string]any{
		"service_name": "X",
		"price":        0,
		"user_id":      uuid.New().String(),
		"start_date":   "01-2025",
	})
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestHandler_Update_InvalidBody(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	id := uuid.New()
	req := httptest.NewRequest(http.MethodPut, "/subscriptions/"+id.String(), bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	newRouter(svc).ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ── DELETE /subscriptions/{id} ────────────────────────────────────────────────

func TestHandler_Delete_Success(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	id := uuid.New()
	svc.EXPECT().Delete(mock.Anything, id).Return(nil)

	rr := doRequest(t, newRouter(svc), http.MethodDelete, "/subscriptions/"+id.String(), nil)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestHandler_Delete_NotFound(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	id := uuid.New()
	svc.EXPECT().Delete(mock.Anything, id).Return(service.ErrNotFound)

	rr := doRequest(t, newRouter(svc), http.MethodDelete, "/subscriptions/"+id.String(), nil)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// ── GET /subscriptions ────────────────────────────────────────────────────────

func TestHandler_List_DefaultPagination(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	svc.EXPECT().List(mock.Anything, mock.MatchedBy(func(f domain.ListFilter) bool {
		return f.Limit == 50 && f.Offset == 0
	})).Return([]domain.Subscription{makeTestSub()}, nil)

	rr := doRequest(t, newRouter(svc), http.MethodGet, "/subscriptions", nil)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandler_List_CustomPagination(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	svc.EXPECT().List(mock.Anything, mock.MatchedBy(func(f domain.ListFilter) bool {
		return f.Limit == 10 && f.Offset == 20
	})).Return(nil, nil)

	rr := doRequest(t, newRouter(svc), http.MethodGet, "/subscriptions?limit=10&offset=20", nil)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandler_List_FilterByUserID(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	uid := uuid.New()
	svc.EXPECT().List(mock.Anything, mock.MatchedBy(func(f domain.ListFilter) bool {
		return f.UserID != nil && *f.UserID == uid
	})).Return(nil, nil)

	rr := doRequest(t, newRouter(svc), http.MethodGet, "/subscriptions?user_id="+uid.String(), nil)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandler_List_NegativeLimitReturns400(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	rr := doRequest(t, newRouter(svc), http.MethodGet, "/subscriptions?limit=-1", nil)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ── GET /subscriptions/summary ────────────────────────────────────────────────

func TestHandler_Summary_Success(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	from := domain.NewMonthYear(1, 2025)
	to := domain.NewMonthYear(6, 2025)

	svc.EXPECT().Summary(mock.Anything, mock.MatchedBy(func(p service.SummaryParams) bool {
		return p.From == from && p.To == to && p.UserID == nil && p.ServiceName == nil
	})).Return(domain.SummaryResult{Total: 1200, Currency: "RUB", From: "01-2025", To: "06-2025"}, nil)

	rr := doRequest(t, newRouter(svc), http.MethodGet, "/subscriptions/summary?from=01-2025&to=06-2025", nil)
	assert.Equal(t, http.StatusOK, rr.Code)

	var result domain.SummaryResult
	decodeJSON(t, rr, &result)
	assert.Equal(t, int64(1200), result.Total)
	assert.Equal(t, "RUB", result.Currency)
}

func TestHandler_Summary_MissingFrom(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	rr := doRequest(t, newRouter(svc), http.MethodGet, "/subscriptions/summary?to=06-2025", nil)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var e errResp
	decodeJSON(t, rr, &e)
	assert.Equal(t, "MISSING_PARAMS", e.Code)
}

func TestHandler_Summary_MissingTo(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	rr := doRequest(t, newRouter(svc), http.MethodGet, "/subscriptions/summary?from=01-2025", nil)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestHandler_Summary_FromAfterTo(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	rr := doRequest(t, newRouter(svc), http.MethodGet, "/subscriptions/summary?from=12-2025&to=01-2025", nil)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var e errResp
	decodeJSON(t, rr, &e)
	assert.Equal(t, "INVALID_DATE_RANGE", e.Code)
}

func TestHandler_Summary_InvalidFromDate(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	rr := doRequest(t, newRouter(svc), http.MethodGet, "/subscriptions/summary?from=baddate&to=06-2025", nil)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var e errResp
	decodeJSON(t, rr, &e)
	assert.Equal(t, "INVALID_DATE", e.Code)
}

func TestHandler_Summary_FiltersPassedToService(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	uid := uuid.New()
	sn := "Netflix"

	from := domain.NewMonthYear(1, 2025)
	to := domain.NewMonthYear(6, 2025)

	svc.EXPECT().Summary(mock.Anything, mock.MatchedBy(func(p service.SummaryParams) bool {
		return p.From == from && p.To == to &&
			p.UserID != nil && *p.UserID == uid &&
			p.ServiceName != nil && *p.ServiceName == sn
	})).Return(domain.SummaryResult{Total: 0, Currency: "RUB", From: "01-2025", To: "06-2025"}, nil)

	url := fmt.Sprintf("/subscriptions/summary?from=01-2025&to=06-2025&user_id=%s&service_name=%s", uid, sn)
	rr := doRequest(t, newRouter(svc), http.MethodGet, url, nil)
	assert.Equal(t, http.StatusOK, rr.Code)
}

// ── GET /healthz ──────────────────────────────────────────────────────────────

func TestHandler_Healthz(t *testing.T) {
	t.Parallel()
	svc := mocks.NewSubscriptionService(t)
	rr := doRequest(t, newRouter(svc), http.MethodGet, "/healthz", nil)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Consume context so mock doesn't warn on unused expectations
	_ = context.Background()
}
