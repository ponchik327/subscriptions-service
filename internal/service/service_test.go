package service_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ponchik327/subscriptions-service/internal/domain"
	"github.com/ponchik327/subscriptions-service/internal/mocks"
	"github.com/ponchik327/subscriptions-service/internal/repository"
	"github.com/ponchik327/subscriptions-service/internal/service"
)

func newService(repo *mocks.SubscriptionRepository) *service.Service {
	return service.New(repo, slog.Default())
}

func TestService_Summary_PeriodValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		from    domain.MonthYear
		to      domain.MonthYear
		wantErr bool
	}{
		{
			name:    "from before to — valid",
			from:    domain.NewMonthYear(1, 2025),
			to:      domain.NewMonthYear(6, 2025),
			wantErr: false,
		},
		{
			name:    "from equals to — valid",
			from:    domain.NewMonthYear(3, 2025),
			to:      domain.NewMonthYear(3, 2025),
			wantErr: false,
		},
		{
			name:    "from after to — invalid",
			from:    domain.NewMonthYear(6, 2025),
			to:      domain.NewMonthYear(1, 2025),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := mocks.NewSubscriptionRepository(t)
			if !tc.wantErr {
				repo.EXPECT().Summary(context.Background(), domain.SummaryFilter{
					From: tc.from,
					To:   tc.to,
				}).Return(int64(0), nil)
			}
			svc := newService(repo)
			_, err := svc.Summary(context.Background(), service.SummaryParams{
				From: tc.from,
				To:   tc.to,
			})
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestService_Summary_MapsRepoError(t *testing.T) {
	t.Parallel()
	repo := mocks.NewSubscriptionRepository(t)
	from := domain.NewMonthYear(1, 2025)
	to := domain.NewMonthYear(3, 2025)

	repo.EXPECT().Summary(context.Background(), domain.SummaryFilter{From: from, To: to}).
		Return(int64(0), assert.AnError)

	svc := newService(repo)
	_, err := svc.Summary(context.Background(), service.SummaryParams{From: from, To: to})
	assert.Error(t, err)
}

func TestService_GetByID_MapsNotFound(t *testing.T) {
	t.Parallel()
	repo := mocks.NewSubscriptionRepository(t)
	id := uuid.New()

	repo.EXPECT().GetByID(context.Background(), id).Return(domain.Subscription{}, repository.ErrNotFound)

	svc := newService(repo)
	_, err := svc.GetByID(context.Background(), id)
	assert.ErrorIs(t, err, service.ErrNotFound)
}

func TestService_Update_MapsNotFound(t *testing.T) {
	t.Parallel()
	repo := mocks.NewSubscriptionRepository(t)
	sub := domain.Subscription{ID: uuid.New()}

	repo.EXPECT().Update(context.Background(), sub).Return(domain.Subscription{}, repository.ErrNotFound)

	svc := newService(repo)
	_, err := svc.Update(context.Background(), sub)
	assert.ErrorIs(t, err, service.ErrNotFound)
}

func TestService_Delete_MapsNotFound(t *testing.T) {
	t.Parallel()
	repo := mocks.NewSubscriptionRepository(t)
	id := uuid.New()

	repo.EXPECT().Delete(context.Background(), id).Return(repository.ErrNotFound)

	svc := newService(repo)
	err := svc.Delete(context.Background(), id)
	assert.ErrorIs(t, err, service.ErrNotFound)
}
