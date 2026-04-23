//go:build integration

package repository_test

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ponchik327/subscriptions-service/internal/domain"
	"github.com/ponchik327/subscriptions-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/testcontainers/testcontainers-go"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	pg, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("get connection string: %v", err)
	}

	mig, err := migrate.New("file://../../migrations", dsn)
	if err != nil {
		log.Fatalf("create migrator: %v", err)
	}
	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("run migrations: %v", err)
	}

	testPool, err = pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("create pool: %v", err)
	}

	code := m.Run()

	testPool.Close()
	_ = pg.Terminate(ctx)
	os.Exit(code)
}

func truncate(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), "TRUNCATE subscriptions RESTART IDENTITY")
	require.NoError(t, err)
}

func insertSub(t *testing.T, sub domain.Subscription) domain.Subscription {
	t.Helper()
	repo := repository.New(testPool)
	created, err := repo.Create(context.Background(), sub)
	require.NoError(t, err)
	return created
}

func makeSub(serviceName string, price int, userID uuid.UUID, startM, startY int, endM, endY *int) domain.Subscription {
	sub := domain.Subscription{
		ID:          uuid.New(),
		ServiceName: serviceName,
		Price:       price,
		UserID:      userID,
		StartDate:   domain.NewMonthYear(startM, startY),
	}
	if endM != nil && endY != nil {
		my := domain.NewMonthYear(*endM, *endY)
		sub.EndDate = &my
	}
	return sub
}

func ptr[T any](v T) *T { return &v }

// ── CRUD Tests ────────────────────────────────────────────────────────────────

func TestRepository_Create(t *testing.T) {
	truncate(t)
	repo := repository.New(testPool)

	sub := makeSub("Netflix", 800, uuid.New(), 1, 2025, nil, nil)
	created, err := repo.Create(context.Background(), sub)
	require.NoError(t, err)

	assert.Equal(t, sub.ID, created.ID)
	assert.Equal(t, "Netflix", created.ServiceName)
	assert.Equal(t, 800, created.Price)
	assert.Equal(t, 1, int(created.StartDate.Month()))
	assert.Equal(t, 2025, created.StartDate.Year())
	assert.Nil(t, created.EndDate)
	assert.False(t, created.CreatedAt.IsZero())
}

func TestRepository_GetByID(t *testing.T) {
	truncate(t)
	repo := repository.New(testPool)

	sub := insertSub(t, makeSub("Spotify", 200, uuid.New(), 3, 2024, ptr(6), ptr(2024)))

	t.Run("found", func(t *testing.T) {
		got, err := repo.GetByID(context.Background(), sub.ID)
		require.NoError(t, err)
		assert.Equal(t, sub.ID, got.ID)
		assert.Equal(t, "Spotify", got.ServiceName)
		require.NotNil(t, got.EndDate)
		assert.Equal(t, 6, int(got.EndDate.Month()))
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, repository.ErrNotFound)
	})
}

func TestRepository_Update(t *testing.T) {
	truncate(t)
	repo := repository.New(testPool)

	original := insertSub(t, makeSub("Yandex Plus", 400, uuid.New(), 1, 2025, nil, nil))
	time.Sleep(time.Millisecond) // ensure updated_at changes

	updated := original
	updated.ServiceName = "Yandex Plus Pro"
	updated.Price = 500
	endDate := domain.NewMonthYear(12, 2025)
	updated.EndDate = &endDate

	got, err := repo.Update(context.Background(), updated)
	require.NoError(t, err)

	assert.Equal(t, "Yandex Plus Pro", got.ServiceName)
	assert.Equal(t, 500, got.Price)
	require.NotNil(t, got.EndDate)
	assert.Equal(t, 12, int(got.EndDate.Month()))
	assert.True(t, got.UpdatedAt.After(original.UpdatedAt))
}

func TestRepository_Delete(t *testing.T) {
	truncate(t)
	repo := repository.New(testPool)

	sub := insertSub(t, makeSub("Apple Music", 300, uuid.New(), 5, 2024, nil, nil))

	require.NoError(t, repo.Delete(context.Background(), sub.ID))

	_, err := repo.GetByID(context.Background(), sub.ID)
	assert.ErrorIs(t, err, repository.ErrNotFound)

	err = repo.Delete(context.Background(), sub.ID)
	assert.ErrorIs(t, err, repository.ErrNotFound)
}

func TestRepository_List(t *testing.T) {
	truncate(t)
	repo := repository.New(testPool)

	userA := uuid.New()
	userB := uuid.New()

	insertSub(t, makeSub("Netflix", 800, userA, 1, 2024, nil, nil))
	insertSub(t, makeSub("Spotify", 200, userA, 2, 2024, nil, nil))
	insertSub(t, makeSub("Netflix", 800, userB, 3, 2024, nil, nil))
	insertSub(t, makeSub("Yandex Plus", 400, userB, 4, 2024, nil, nil))
	insertSub(t, makeSub("Apple Music", 300, userA, 5, 2024, nil, nil))

	t.Run("no filters all 5", func(t *testing.T) {
		subs, err := repo.List(context.Background(), domain.ListFilter{Limit: 10})
		require.NoError(t, err)
		assert.Len(t, subs, 5)
	})

	t.Run("pagination limit 2", func(t *testing.T) {
		subs, err := repo.List(context.Background(), domain.ListFilter{Limit: 2, Offset: 0})
		require.NoError(t, err)
		assert.Len(t, subs, 2)
	})

	t.Run("filter by user_id", func(t *testing.T) {
		subs, err := repo.List(context.Background(), domain.ListFilter{Limit: 10, UserID: &userA})
		require.NoError(t, err)
		assert.Len(t, subs, 3)
		for _, s := range subs {
			assert.Equal(t, userA, s.UserID)
		}
	})

	t.Run("filter by service_name", func(t *testing.T) {
		sn := "Netflix"
		subs, err := repo.List(context.Background(), domain.ListFilter{Limit: 10, ServiceName: &sn})
		require.NoError(t, err)
		assert.Len(t, subs, 2)
	})

	t.Run("both filters", func(t *testing.T) {
		sn := "Netflix"
		subs, err := repo.List(context.Background(), domain.ListFilter{Limit: 10, UserID: &userA, ServiceName: &sn})
		require.NoError(t, err)
		assert.Len(t, subs, 1)
		assert.Equal(t, userA, subs[0].UserID)
	})
}

// ── Summary Tests (14-case matrix) ───────────────────────────────────────────

func TestRepository_Summary(t *testing.T) {
	repo := repository.New(testPool)
	userA := uuid.New()
	userB := uuid.New()

	from := domain.NewMonthYear(3, 2025) // March 2025
	to := domain.NewMonthYear(8, 2025)   // August 2025  (6 months)

	tests := []struct {
		name        string
		subs        []domain.Subscription
		filter      repository.SummaryFilter
		wantTotal   int64
	}{
		{
			name: "1: sub fully inside [from,to]",
			// May-2025 to Jun-2025 → 2 months
			subs:      []domain.Subscription{makeSub("A", 100, userA, 5, 2025, ptr(6), ptr(2025))},
			filter:    repository.SummaryFilter{From: from, To: to},
			wantTotal: 200, // 100 * 2
		},
		{
			name: "2: starts before from, ends inside",
			// Jan-2025 to May-2025, period Mar-Aug → overlap Mar-May = 3 months
			subs:      []domain.Subscription{makeSub("A", 100, userA, 1, 2025, ptr(5), ptr(2025))},
			filter:    repository.SummaryFilter{From: from, To: to},
			wantTotal: 300, // 100 * 3
		},
		{
			name: "3: starts inside, ends after to",
			// Jun-2025 to Nov-2025, period Mar-Aug → overlap Jun-Aug = 3 months
			subs:      []domain.Subscription{makeSub("A", 100, userA, 6, 2025, ptr(11), ptr(2025))},
			filter:    repository.SummaryFilter{From: from, To: to},
			wantTotal: 300, // 100 * 3
		},
		{
			name: "4: covers entire period (before from and after to)",
			// Jan-2025 to Dec-2025, period Mar-Aug = 6 months
			subs:      []domain.Subscription{makeSub("A", 100, userA, 1, 2025, ptr(12), ptr(2025))},
			filter:    repository.SummaryFilter{From: from, To: to},
			wantTotal: 600, // 100 * 6
		},
		{
			name: "5: end_date=NULL, starts inside",
			// May-2025, no end → overlap May-Aug = 4 months
			subs:      []domain.Subscription{makeSub("A", 100, userA, 5, 2025, nil, nil)},
			filter:    repository.SummaryFilter{From: from, To: to},
			wantTotal: 400, // 100 * 4
		},
		{
			name: "6: end_date=NULL, starts before from",
			// Jan-2025, no end → overlap Mar-Aug = 6 months
			subs:      []domain.Subscription{makeSub("A", 100, userA, 1, 2025, nil, nil)},
			filter:    repository.SummaryFilter{From: from, To: to},
			wantTotal: 600, // 100 * 6
		},
		{
			name: "7: ended before from (filtered out)",
			// Jan-2025 to Feb-2025
			subs:      []domain.Subscription{makeSub("A", 100, userA, 1, 2025, ptr(2), ptr(2025))},
			filter:    repository.SummaryFilter{From: from, To: to},
			wantTotal: 0,
		},
		{
			name: "8: starts after to (filtered out)",
			// Sep-2025 onwards
			subs:      []domain.Subscription{makeSub("A", 100, userA, 9, 2025, nil, nil)},
			filter:    repository.SummaryFilter{From: from, To: to},
			wantTotal: 0,
		},
		{
			name: "9: exactly 1 month, coincides with from",
			// Mar-2025 to Mar-2025 → 1 month
			subs:      []domain.Subscription{makeSub("A", 100, userA, 3, 2025, ptr(3), ptr(2025))},
			filter:    repository.SummaryFilter{From: from, To: to},
			wantTotal: 100, // 100 * 1
		},
		{
			name: "10: exactly 1 month, coincides with to",
			// Aug-2025 to Aug-2025 → 1 month
			subs:      []domain.Subscription{makeSub("A", 100, userA, 8, 2025, ptr(8), ptr(2025))},
			filter:    repository.SummaryFilter{From: from, To: to},
			wantTotal: 100, // 100 * 1
		},
		{
			name: "11: filter by user_id - only count for that user",
			// userA: Jan-2025 to Dec-2025 → 6 months; userB: same → also 6 months
			// filter by userA → 100 * 6 = 600
			subs: []domain.Subscription{
				makeSub("Netflix", 100, userA, 1, 2025, ptr(12), ptr(2025)),
				makeSub("Netflix", 100, userB, 1, 2025, ptr(12), ptr(2025)),
			},
			filter:    repository.SummaryFilter{From: from, To: to, UserID: &userA},
			wantTotal: 600,
		},
		{
			name: "12: filter by service_name",
			// Netflix: 100/mo 6 months; Spotify: 200/mo 6 months
			// filter Netflix → 600
			subs: []domain.Subscription{
				makeSub("Netflix", 100, userA, 1, 2025, ptr(12), ptr(2025)),
				makeSub("Spotify", 200, userA, 1, 2025, ptr(12), ptr(2025)),
			},
			filter:    repository.SummaryFilter{From: from, To: to, ServiceName: ptr("Netflix")},
			wantTotal: 600,
		},
		{
			name: "13: both filters simultaneously",
			// userA Netflix: 100 * 6 = 600; userA Spotify: 200 * 6; userB Netflix: 100 * 6
			// filter userA + Netflix → 600
			subs: []domain.Subscription{
				makeSub("Netflix", 100, userA, 1, 2025, ptr(12), ptr(2025)),
				makeSub("Spotify", 200, userA, 1, 2025, ptr(12), ptr(2025)),
				makeSub("Netflix", 100, userB, 1, 2025, ptr(12), ptr(2025)),
			},
			filter:    repository.SummaryFilter{From: from, To: to, UserID: &userA, ServiceName: ptr("Netflix")},
			wantTotal: 600,
		},
		{
			name:      "14: empty selection returns 0 not error",
			subs:      nil,
			filter:    repository.SummaryFilter{From: from, To: to},
			wantTotal: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			for _, s := range tc.subs {
				insertSub(t, s)
			}

			total, err := repo.Summary(context.Background(), tc.filter)
			require.NoError(t, err)
			assert.Equal(t, tc.wantTotal, total)
		})
	}
}
