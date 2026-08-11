package version

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.1.0", "0.1.1", -1},
		{"0.1.1", "0.1.0", 1},
		{"0.2.0", "0.1.9", 1},
		{"1.0.0", "0.9.9", 1},
		{"0.1.0", "0.1.0-dev", 1},
		{"0.1.0-dev", "0.1.0", -1},
		{"0.1.0-dev", "0.0.9", 1},
		{"v0.1.0", "0.1.0", 0},
	}
	for _, tt := range tests {
		got := Compare(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
