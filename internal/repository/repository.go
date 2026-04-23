package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ponchik327/subscriptions-service/internal/domain"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, sub domain.Subscription) (domain.Subscription, error) {
	var endDate *time.Time
	if sub.EndDate != nil && !sub.EndDate.IsZero() {
		t := sub.EndDate.Time
		endDate = &t
	}

	q, args, err := psql.Insert("subscriptions").
		Columns("id", "service_name", "price", "user_id", "start_date", "end_date").
		Values(sub.ID, sub.ServiceName, sub.Price, sub.UserID, sub.StartDate.Time, endDate).
		Suffix("RETURNING id, service_name, price, user_id, start_date, end_date, created_at, updated_at").
		ToSql()
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("repository.Create build query: %w", err)
	}

	return scanSubscription(r.pool.QueryRow(ctx, q, args...))
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (domain.Subscription, error) {
	q, args, err := psql.Select("id, service_name, price, user_id, start_date, end_date, created_at, updated_at").
		From("subscriptions").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("repository.GetByID build query: %w", err)
	}

	sub, err := scanSubscription(r.pool.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Subscription{}, ErrNotFound
	}
	return sub, err
}

func (r *Repository) Update(ctx context.Context, sub domain.Subscription) (domain.Subscription, error) {
	var endDate *time.Time
	if sub.EndDate != nil && !sub.EndDate.IsZero() {
		t := sub.EndDate.Time
		endDate = &t
	}

	q, args, err := psql.Update("subscriptions").
		Set("service_name", sub.ServiceName).
		Set("price", sub.Price).
		Set("user_id", sub.UserID).
		Set("start_date", sub.StartDate.Time).
		Set("end_date", endDate).
		Set("updated_at", sq.Expr("NOW()")).
		Where(sq.Eq{"id": sub.ID}).
		Suffix("RETURNING id, service_name, price, user_id, start_date, end_date, created_at, updated_at").
		ToSql()
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("repository.Update build query: %w", err)
	}

	updated, err := scanSubscription(r.pool.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Subscription{}, ErrNotFound
	}
	return updated, err
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	q, args, err := psql.Delete("subscriptions").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return fmt.Errorf("repository.Delete build query: %w", err)
	}

	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("repository.Delete exec: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, filter domain.ListFilter) ([]domain.Subscription, error) {
	qb := psql.Select("id, service_name, price, user_id, start_date, end_date, created_at, updated_at").
		From("subscriptions").
		OrderBy("created_at DESC").
		Limit(filter.Limit).
		Offset(filter.Offset)

	if filter.UserID != nil {
		qb = qb.Where(sq.Eq{"user_id": *filter.UserID})
	}
	if filter.ServiceName != nil {
		qb = qb.Where(sq.Eq{"service_name": *filter.ServiceName})
	}

	q, args, err := qb.ToSql()
	if err != nil {
		return nil, fmt.Errorf("repository.List build query: %w", err)
	}

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("repository.List query: %w", err)
	}
	defer rows.Close()

	var subs []domain.Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// Summary computes the total subscription cost over [from, to] using a single SQL query.
// $1 = from date, $2 = to date; optional filters use $3, $4, … appended manually to avoid
// squirrel placeholder collision with the hardcoded $1/$2 in the aggregation expression.
func (r *Repository) Summary(ctx context.Context, f domain.SummaryFilter) (int64, error) {
	args := []interface{}{f.From.Time, f.To.Time}

	extraWhere := ""
	if f.UserID != nil {
		args = append(args, *f.UserID)
		extraWhere += fmt.Sprintf(" AND user_id = $%d", len(args))
	}
	if f.ServiceName != nil {
		args = append(args, *f.ServiceName)
		extraWhere += fmt.Sprintf(" AND service_name = $%d", len(args))
	}

	q := `
		SELECT COALESCE(SUM(
			price * (
				(EXTRACT(YEAR  FROM LEAST(COALESCE(end_date, $2::date), $2::date)) * 12
			   + EXTRACT(MONTH FROM LEAST(COALESCE(end_date, $2::date), $2::date)))
			  - (EXTRACT(YEAR  FROM GREATEST(start_date, $1::date)) * 12
			   + EXTRACT(MONTH FROM GREATEST(start_date, $1::date)))
			  + 1
			)
		), 0)::bigint AS total
		FROM subscriptions
		WHERE start_date <= $2::date
		  AND COALESCE(end_date, $2::date) >= $1::date` + extraWhere

	var total int64
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("repository.Summary query: %w", err)
	}
	return total, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSubscription(row scanner) (domain.Subscription, error) {
	var sub domain.Subscription
	var endDate *time.Time
	var startDate time.Time

	err := row.Scan(
		&sub.ID,
		&sub.ServiceName,
		&sub.Price,
		&sub.UserID,
		&startDate,
		&endDate,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err != nil {
		return domain.Subscription{}, err
	}

	sub.StartDate = domain.MonthYear{Time: time.Date(startDate.Year(), startDate.Month(), 1, 0, 0, 0, 0, time.UTC)}
	if endDate != nil {
		my := domain.MonthYear{Time: time.Date(endDate.Year(), endDate.Month(), 1, 0, 0, 0, 0, time.UTC)}
		sub.EndDate = &my
	}
	return sub, nil
}
