package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/ponchik327/subscriptions-service/internal/domain"
	"github.com/ponchik327/subscriptions-service/internal/repository"
)

//go:generate mockery --name SubscriptionRepository --output ../mocks --outpkg mocks --case snake --with-expecter

// SubscriptionRepository is the interface the service requires from the persistence layer.
type SubscriptionRepository interface {
	Create(ctx context.Context, sub domain.Subscription) (domain.Subscription, error)
	GetByID(ctx context.Context, id uuid.UUID) (domain.Subscription, error)
	Update(ctx context.Context, sub domain.Subscription) (domain.Subscription, error)
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, filter domain.ListFilter) ([]domain.Subscription, error)
	Summary(ctx context.Context, f repository.SummaryFilter) (int64, error)
}

type SummaryParams struct {
	From        domain.MonthYear
	To          domain.MonthYear
	UserID      *uuid.UUID
	ServiceName *string
}

type Service struct {
	repo   SubscriptionRepository
	logger *slog.Logger
}

func New(repo SubscriptionRepository, logger *slog.Logger) *Service {
	return &Service{repo: repo, logger: logger}
}

func (s *Service) Create(ctx context.Context, sub domain.Subscription) (domain.Subscription, error) {
	sub.ID = uuid.New()
	created, err := s.repo.Create(ctx, sub)
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("service.Create: %w", err)
	}
	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (domain.Subscription, error) {
	sub, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return domain.Subscription{}, ErrNotFound
	}
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("service.GetByID: %w", err)
	}
	return sub, nil
}

func (s *Service) Update(ctx context.Context, sub domain.Subscription) (domain.Subscription, error) {
	updated, err := s.repo.Update(ctx, sub)
	if errors.Is(err, repository.ErrNotFound) {
		return domain.Subscription{}, ErrNotFound
	}
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("service.Update: %w", err)
	}
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("service.Delete: %w", err)
	}
	return nil
}

func (s *Service) List(ctx context.Context, filter domain.ListFilter) ([]domain.Subscription, error) {
	subs, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("service.List: %w", err)
	}
	return subs, nil
}

func (s *Service) Summary(ctx context.Context, p SummaryParams) (domain.SummaryResult, error) {
	if !p.From.IsZero() && !p.To.IsZero() && p.From.After(p.To.Time) {
		return domain.SummaryResult{}, fmt.Errorf("from must be <= to")
	}

	total, err := s.repo.Summary(ctx, repository.SummaryFilter{
		From:        p.From,
		To:          p.To,
		UserID:      p.UserID,
		ServiceName: p.ServiceName,
	})
	if err != nil {
		return domain.SummaryResult{}, fmt.Errorf("service.Summary: %w", err)
	}

	return domain.SummaryResult{
		Total:    total,
		Currency: "RUB",
		From:     p.From.String(),
		To:       p.To.String(),
	}, nil
}
