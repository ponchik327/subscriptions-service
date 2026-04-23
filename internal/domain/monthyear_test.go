package domain_test

import (
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ponchik327/subscriptions-service/internal/domain"
)

func TestMonthYear_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	type wrapper struct {
		D domain.MonthYear `json:"d"`
	}

	tests := []struct {
		name    string
		input   string
		wantM   int
		wantY   int
		wantErr bool
	}{
		{name: "july 2025", input: `{"d":"07-2025"}`, wantM: 7, wantY: 2025},
		{name: "january 2000", input: `{"d":"01-2000"}`, wantM: 1, wantY: 2000},
		{name: "december 2099", input: `{"d":"12-2099"}`, wantM: 12, wantY: 2099},
		{name: "empty string", input: `{"d":""}`, wantErr: true},
		{name: "literal null", input: `{"d":"null"}`, wantErr: false}, // null → zero value
		{name: "no leading zero", input: `{"d":"7-2025"}`, wantErr: true},
		{name: "month 13", input: `{"d":"13-2025"}`, wantErr: true},
		{name: "month 00", input: `{"d":"00-2025"}`, wantErr: true},
		{name: "short year", input: `{"d":"07-25"}`, wantErr: true},
		{name: "reversed format", input: `{"d":"2025-07"}`, wantErr: true},
		{name: "letters", input: `{"d":"abc"}`, wantErr: true},
		{name: "three parts", input: `{"d":"07-2025-01"}`, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var w wrapper
			err := json.Unmarshal([]byte(tc.input), &w)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.wantM == 0 {
				// null → zero value
				assert.True(t, w.D.IsZero())
				return
			}
			assert.Equal(t, tc.wantM, int(w.D.Month()))
			assert.Equal(t, tc.wantY, w.D.Year())
			assert.Equal(t, 1, w.D.Day())
			assert.Equal(t, time.UTC, w.D.Location())
		})
	}
}

func TestMonthYear_MarshalJSON(t *testing.T) {
	t.Parallel()

	valid := []struct {
		name  string
		input string
	}{
		{name: "july 2025", input: "07-2025"},
		{name: "january 2000", input: "01-2000"},
		{name: "december 2099", input: "12-2099"},
	}

	for _, tc := range valid {
		tc := tc
		t.Run("roundtrip/"+tc.name, func(t *testing.T) {
			t.Parallel()
			type wrapper struct {
				D domain.MonthYear `json:"d"`
			}
			raw := `{"d":"` + tc.input + `"}`
			var w wrapper
			require.NoError(t, json.Unmarshal([]byte(raw), &w))
			out, err := json.Marshal(w)
			require.NoError(t, err)
			assert.Equal(t, raw, string(out))
		})
	}

	t.Run("zero value marshals to null", func(t *testing.T) {
		t.Parallel()
		var m domain.MonthYear
		b, err := m.MarshalJSON()
		require.NoError(t, err)
		assert.Equal(t, "null", string(b))
	})
}

func TestMonthYear_Scan(t *testing.T) {
	t.Parallel()

	t.Run("scan from time.Time", func(t *testing.T) {
		t.Parallel()
		src := time.Date(2025, 7, 15, 12, 0, 0, 0, time.UTC) // mid-month time
		var m domain.MonthYear
		require.NoError(t, m.Scan(src))
		assert.Equal(t, 7, int(m.Month()))
		assert.Equal(t, 2025, m.Year())
		assert.Equal(t, 1, m.Day()) // normalized to first
	})

	t.Run("scan from nil", func(t *testing.T) {
		t.Parallel()
		var m domain.MonthYear
		require.NoError(t, m.Scan(nil))
		assert.True(t, m.IsZero())
	})

	t.Run("scan wrong type", func(t *testing.T) {
		t.Parallel()
		var m domain.MonthYear
		assert.Error(t, m.Scan("not a time"))
	})
}

func TestMonthYear_Value(t *testing.T) {
	t.Parallel()

	t.Run("valid value", func(t *testing.T) {
		t.Parallel()
		m := domain.NewMonthYear(7, 2025)
		v, err := m.Value()
		require.NoError(t, err)
		require.NotNil(t, v)
		tv, ok := v.(time.Time)
		require.True(t, ok)
		assert.Equal(t, driver.Value(time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)), driver.Value(tv))
	})

	t.Run("zero value returns nil", func(t *testing.T) {
		t.Parallel()
		var m domain.MonthYear
		v, err := m.Value()
		require.NoError(t, err)
		assert.Nil(t, v)
	})
}
