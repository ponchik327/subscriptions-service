package domain

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MonthYear represents a month+year value stored as the first day of that month (UTC).
// JSON format: "MM-YYYY".
type MonthYear struct{ time.Time }

const monthYearLayout = "01-2006"

func NewMonthYear(month, year int) MonthYear {
	return MonthYear{time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)}
}

func ParseMonthYear(s string) (MonthYear, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return MonthYear{}, fmt.Errorf("invalid MM-YYYY format: %q", s)
	}
	if len(parts[0]) != 2 || len(parts[1]) != 4 {
		return MonthYear{}, fmt.Errorf("invalid MM-YYYY format: %q", s)
	}
	month, err := strconv.Atoi(parts[0])
	if err != nil || month < 1 || month > 12 {
		return MonthYear{}, fmt.Errorf("invalid month in %q", s)
	}
	year, err := strconv.Atoi(parts[1])
	if err != nil {
		return MonthYear{}, fmt.Errorf("invalid year in %q", s)
	}
	return MonthYear{time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)}, nil
}

func (m *MonthYear) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" {
		return nil
	}
	parsed, err := ParseMonthYear(s)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

func (m MonthYear) MarshalJSON() ([]byte, error) {
	if m.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + m.Format(monthYearLayout) + `"`), nil
}

// Scan implements sql.Scanner — pgx delivers DATE columns as time.Time.
func (m *MonthYear) Scan(src any) error {
	if src == nil {
		return nil
	}
	t, ok := src.(time.Time)
	if !ok {
		return fmt.Errorf("MonthYear.Scan: expected time.Time, got %T", src)
	}
	m.Time = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return nil
}

// Value implements driver.Valuer for pgx.
func (m MonthYear) Value() (driver.Value, error) {
	if m.IsZero() {
		return nil, nil
	}
	return m.Time, nil
}

func (m MonthYear) String() string {
	if m.IsZero() {
		return ""
	}
	return m.Format(monthYearLayout)
}
