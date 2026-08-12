package entities

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// Timestamp wraps time.Time with SQLite (RFC3339 TEXT) and JSON serialization.
// glebarez/sqlite does not scan string into time.Time, so a custom
// driver.Valuer / sql.Scanner is required.
type Timestamp struct {
	time.Time
}

func Now() Timestamp {
	return Timestamp{Time: time.Now().UTC()}
}

// TimestampLayout is the on-disk encoding: RFC 3339 with a fixed nine-digit
// fraction.
//
// Timestamps live in TEXT columns, so every ORDER BY compares them as strings and
// string order has to match chronological order. time.RFC3339Nano trims trailing
// zeros, which broke that: a whole second rendered as "10:00:00Z" and sorted
// after the later "10:00:00.5Z", because 'Z' is greater than '.'. Any "latest"
// query could return the wrong row, including the checkpoint lookup behind
// next_action, restore and retention pruning. A fixed width removes the
// possibility.
const TimestampLayout = "2006-01-02T15:04:05.000000000Z07:00"

func (t Timestamp) Value() (driver.Value, error) {
	return t.UTC().Format(TimestampLayout), nil
}

func (t *Timestamp) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		t.Time = time.Time{}
	case time.Time:
		t.Time = v
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return fmt.Errorf("parse time %q: %w", v, err)
		}
		t.Time = parsed
	case []byte:
		return t.Scan(string(v))
	default:
		return fmt.Errorf("unsupported time value %T", value)
	}
	return nil
}

func (t Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.UTC().Format(time.RFC3339))
}

func (t *Timestamp) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return fmt.Errorf("parse time %q: %w", s, err)
	}
	t.Time = parsed
	return nil
}

func (t *Timestamp) Format(layout string) string {
	return t.Time.Format(layout)
}
