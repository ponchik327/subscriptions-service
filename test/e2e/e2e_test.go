//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ponchik327/subscriptions-service/internal/domain"
	"github.com/ponchik327/subscriptions-service/internal/handler"
	"github.com/ponchik327/subscriptions-service/internal/repository"
	"github.com/ponchik327/subscriptions-service/internal/service"
)

type testApp struct {
	server  *httptest.Server
	pool    *pgxpool.Pool
	cleanup func()
}

var globalApp *testApp

func TestMain(m *testing.M) {
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("e2e"),
		tcpostgres.WithUsername("e2e"),
		tcpostgres.WithPassword("e2e"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		log.Fatalf("start postgres: %v", err)
	}

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("connection string: %v", err)
	}

	mig, err := migrate.New("file://../../migrations", dsn)
	if err != nil {
		log.Fatalf("create migrator: %v", err)
	}
	if err := mig.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("run migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("create pool: %v", err)
	}

	logger := slog.Default()
	repo := repository.New(pool)
	svc := service.New(repo, logger)
	router := handler.NewRouter(svc, pool, logger)
	srv := httptest.NewServer(router)

	globalApp = &testApp{
		server: srv,
		pool:   pool,
		cleanup: func() {
			srv.Close()
			pool.Close()
			_ = pg.Terminate(ctx)
		},
	}

	code := m.Run()

	globalApp.cleanup()
	os.Exit(code)
}

func truncateAll(t *testing.T) {
	t.Helper()
	_, err := globalApp.pool.Exec(context.Background(), "TRUNCATE subscriptions RESTART IDENTITY")
	require.NoError(t, err)
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func doPost(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(body))
	resp, err := http.Post(globalApp.server.URL+path, "application/json", &buf)
	require.NoError(t, err)
	return resp
}

func doGet(t *testing.T, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(globalApp.server.URL + path)
	require.NoError(t, err)
	return resp
}

func doPut(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(body))
	req, err := http.NewRequest(http.MethodPut, globalApp.server.URL+path, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func doDelete(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, globalApp.server.URL+path, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, dest any) {
	t.Helper()
	defer resp.Body.Close()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(dest))
}

// ── Test scenarios ────────────────────────────────────────────────────────────

// 1. Happy-path CRUD: POST → GET → PUT → GET → DELETE → GET 404
func TestE2E_CRUDHappyPath(t *testing.T) {
	truncateAll(t)
	userID := uuid.New()

	// POST
	postResp := doPost(t, "/subscriptions", map[string]any{
		"service_name": "Yandex Plus",
		"price":        400,
		"user_id":      userID.String(),
		"start_date":   "07-2025",
	})
	require.Equal(t, http.StatusCreated, postResp.StatusCode)
	var created domain.Subscription
	decodeBody(t, postResp, &created)
	assert.NotEqual(t, uuid.Nil, created.ID)
	assert.Equal(t, "Yandex Plus", created.ServiceName)
	assert.Equal(t, 400, created.Price)

	// GET
	getResp := doGet(t, "/subscriptions/"+created.ID.String())
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	var got domain.Subscription
	decodeBody(t, getResp, &got)
	assert.Equal(t, created.ID, got.ID)

	// PUT
	putResp := doPut(t, "/subscriptions/"+created.ID.String(), map[string]any{
		"service_name": "Yandex Plus Pro",
		"price":        500,
		"user_id":      userID.String(),
		"start_date":   "07-2025",
		"end_date":     "12-2025",
	})
	require.Equal(t, http.StatusOK, putResp.StatusCode)
	var updated domain.Subscription
	decodeBody(t, putResp, &updated)
	assert.Equal(t, "Yandex Plus Pro", updated.ServiceName)
	assert.Equal(t, 500, updated.Price)
	require.NotNil(t, updated.EndDate)
	assert.Equal(t, 12, int(updated.EndDate.Month()))

	// GET again → verify update persisted
	getResp2 := doGet(t, "/subscriptions/"+created.ID.String())
	require.Equal(t, http.StatusOK, getResp2.StatusCode)
	var got2 domain.Subscription
	decodeBody(t, getResp2, &got2)
	assert.Equal(t, "Yandex Plus Pro", got2.ServiceName)

	// DELETE
	delResp := doDelete(t, "/subscriptions/"+created.ID.String())
	require.Equal(t, http.StatusNoContent, delResp.StatusCode)
	delResp.Body.Close()

	// GET after delete → 404
	notFoundResp := doGet(t, "/subscriptions/"+created.ID.String())
	require.Equal(t, http.StatusNotFound, notFoundResp.StatusCode)
	notFoundResp.Body.Close()
}

// 2. List with filter and pagination
func TestE2E_ListWithFilterAndPagination(t *testing.T) {
	truncateAll(t)

	userA := uuid.New()
	userB := uuid.New()

	for i := 0; i < 3; i++ {
		resp := doPost(t, "/subscriptions", map[string]any{
			"service_name": fmt.Sprintf("Service%d", i),
			"price":        100,
			"user_id":      userA.String(),
			"start_date":   "01-2025",
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}
	for i := 0; i < 2; i++ {
		resp := doPost(t, "/subscriptions", map[string]any{
			"service_name": fmt.Sprintf("Other%d", i),
			"price":        200,
			"user_id":      userB.String(),
			"start_date":   "01-2025",
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// Filter by userA, limit 2
	resp := doGet(t, fmt.Sprintf("/subscriptions?user_id=%s&limit=2", userA))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var subs []domain.Subscription
	decodeBody(t, resp, &subs)
	require.Len(t, subs, 2)
	for _, s := range subs {
		assert.Equal(t, userA, s.UserID)
	}
}

// 3. /summary on live stack
func TestE2E_Summary(t *testing.T) {
	truncateAll(t)
	userID := uuid.New()

	// Sub fully inside period: Apr-Jun 2025 (3 months), price=100 → 300
	resp := doPost(t, "/subscriptions", map[string]any{
		"service_name": "Netflix",
		"price":        100,
		"user_id":      userID.String(),
		"start_date":   "04-2025",
		"end_date":     "06-2025",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Sub with end_date=NULL starting inside the period: May-2025, open-ended
	// Period is Jan-Dec 2025 (12 months), sub from May → Dec = 8 months, price=50 → 400
	resp2 := doPost(t, "/subscriptions", map[string]any{
		"service_name": "Spotify",
		"price":        50,
		"user_id":      userID.String(),
		"start_date":   "05-2025",
	})
	require.Equal(t, http.StatusCreated, resp2.StatusCode)
	resp2.Body.Close()

	// Query: from=01-2025, to=12-2025
	sumResp := doGet(t, "/subscriptions/summary?from=01-2025&to=12-2025")
	require.Equal(t, http.StatusOK, sumResp.StatusCode)
	var result domain.SummaryResult
	decodeBody(t, sumResp, &result)

	// Netflix: 100 * 3 = 300; Spotify: 50 * 8 = 400; total = 700
	assert.Equal(t, int64(700), result.Total)
	assert.Equal(t, "RUB", result.Currency)
	assert.Equal(t, "01-2025", result.From)
	assert.Equal(t, "12-2025", result.To)
}

// 4. Validation: end_date < start_date → 400
func TestE2E_Validation_EndDateBeforeStartDate(t *testing.T) {
	truncateAll(t)
	resp := doPost(t, "/subscriptions", map[string]any{
		"service_name": "X",
		"price":        100,
		"user_id":      uuid.New().String(),
		"start_date":   "06-2025",
		"end_date":     "01-2025",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var e struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&e))
	assert.Equal(t, "INVALID_DATE_RANGE", e.Code)
}

// 5. 404 on non-existent UUID
func TestE2E_NotFound(t *testing.T) {
	truncateAll(t)
	resp := doGet(t, "/subscriptions/"+uuid.New().String())
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// 6. GET /healthz
func TestE2E_Healthz(t *testing.T) {
	resp := doGet(t, "/healthz")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
