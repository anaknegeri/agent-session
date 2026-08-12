package entities

import (
	"testing"
	"time"
)

// TestTimestampSortsLexicographically pins the property every ORDER BY in this
// project depends on: timestamps live in TEXT columns, so SQLite compares them as
// strings and string order has to match chronological order.
//
// RFC3339Nano trims trailing zeros from the fraction, which broke it. A whole
// second rendered as "10:00:00Z" and sorted after the later "10:00:00.5Z", since
// 'Z' is greater than '.'. Every "latest" query could therefore return the wrong
// row — including the checkpoint lookup that drives next_action, restore and
// retention pruning.
func TestTimestampSortsLexicographically(t *testing.T) {
	base := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	offsets := []time.Duration{
		0,
		1 * time.Nanosecond,
		500 * time.Microsecond,
		500*time.Microsecond + 1,
		1 * time.Millisecond,
		15 * time.Millisecond,
		100 * time.Millisecond,
		150 * time.Millisecond,
		200 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		time.Second + 50*time.Millisecond,
	}

	for i := 0; i < len(offsets); i++ {
		for j := i + 1; j < len(offsets); j++ {
			a := Timestamp{Time: base.Add(offsets[i])}
			b := Timestamp{Time: base.Add(offsets[j])}
			va, err := a.Value()
			if err != nil {
				t.Fatal(err)
			}
			vb, err := b.Value()
			if err != nil {
				t.Fatal(err)
			}
			sa, sb := va.(string), vb.(string)
			if !(sa < sb) {
				t.Errorf("%s (%v) should sort before %s (%v) but does not",
					sa, offsets[i], sb, offsets[j])
			}
		}
	}
}

// TestTimestampRoundTrip verifies the fixed-width encoding still reads back
// exactly, including values written by the earlier variable-width format.
func TestTimestampRoundTrip(t *testing.T) {
	for _, in := range []string{
		"2026-08-12T10:00:00.000000000Z",
		"2026-08-12T10:00:00Z",   // written by the old variable-width format
		"2026-08-12T10:00:00.5Z", // ditto
		"2026-08-12T10:00:00.123456789Z",
	} {
		var ts Timestamp
		if err := ts.Scan(in); err != nil {
			t.Fatalf("scan %q: %v", in, err)
		}
		want, err := time.Parse(time.RFC3339Nano, in)
		if err != nil {
			t.Fatal(err)
		}
		if !ts.Time.Equal(want) {
			t.Errorf("scan %q gave %v, want %v", in, ts.Time, want)
		}
	}
}
