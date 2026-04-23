package domain

import (
	"time"

	"github.com/google/uuid"
)

type Subscription struct {
	ID          uuid.UUID  `json:"id"`
	ServiceName string     `json:"service_name"`
	Price       int        `json:"price"`
	UserID      uuid.UUID  `json:"user_id"`
	StartDate   MonthYear  `json:"start_date"`
	EndDate     *MonthYear `json:"end_date,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type SummaryResult struct {
	Total    int64  `json:"total"`
	Currency string `json:"currency"`
	From     string `json:"from"`
	To       string `json:"to"`
}

type ListFilter struct {
	UserID      *uuid.UUID
	ServiceName *string
	Limit       uint64
	Offset      uint64
}
