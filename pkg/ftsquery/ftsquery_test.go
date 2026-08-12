package ftsquery

import (
	"strings"
	"testing"
)

func TestBuild(t *testing.T) {
	tests := []struct {
		name string
		in   string
		op   string
		want string
	}{
		{"single term", "oauth", "AND", `"oauth"`},
		{"two terms and", "refresh token", "AND", `"refresh" AND "token"`},
		{"two terms or", "refresh token", "OR", `"refresh" OR "token"`},
		{
			// a separator carries no indexable character, so ANDing it as a phrase
			// matched nothing and zeroed the whole query
			"punctuation between terms is dropped",
			"OAuth2 / PKCE", "AND", `"OAuth2" AND "PKCE"`,
		},
		{"plus between terms is dropped", "PostgreSQL + TimescaleDB", "AND", `"PostgreSQL" AND "TimescaleDB"`},
		{"dotted identifier kept as one term", "foo.bar", "AND", `"foo.bar"`},
		{"trailing punctuation kept inside the term", "TimescaleDB?", "AND", `"TimescaleDB?"`},
		{"cpp keeps its plusses", "C++", "AND", `"C++"`},
		{"quoted phrase stays one term", `"rotate the tokens"`, "AND", `"rotate the tokens"`},
		{"phrase plus term", `"refresh token" rotation`, "AND", `"refresh token" AND "rotation"`},
		// a double quote always delimits a phrase, so it never survives into a term
		{"stray quote opens a phrase", `say "hi`, "AND", `"say" AND "hi"`},
		{"quote inside a word splits it", `foo"bar`, "AND", `"foo" AND "bar"`},
		{"prefix search", "oauth*", "AND", `"oauth"*`},
		{"prefix inside phrase", `"refresh tok"*`, "AND", `"refresh tok"*`},
		{"unterminated phrase still usable", `"refresh token`, "AND", `"refresh token"`},
		{"mixed case preserved", "OAuth2", "OR", `"OAuth2"`},
		{"extra whitespace collapsed", "  refresh   token  ", "AND", `"refresh" AND "token"`},
		{"unicode term", "café", "AND", `"café"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Build(tt.in, tt.op)
			if err != nil {
				t.Fatalf("Build(%q, %q) error: %v", tt.in, tt.op, err)
			}
			if got != tt.want {
				t.Errorf("Build(%q, %q) = %q, want %q", tt.in, tt.op, got, tt.want)
			}
		})
	}
}

func TestBuildRejectsQueriesWithNothingToMatch(t *testing.T) {
	for _, in := range []string{"", "   ", "-", "/ + -", `"`, "()", "***"} {
		if got, err := Build(in, "AND"); err == nil {
			t.Errorf("Build(%q) = %q, want an error: nothing indexable to search for", in, got)
		}
	}
}

// TestBuildNeverLeaksBareQuote pins the invariant the escaping relies on: no
// term reaches the output holding an unpaired quote.
func TestBuildNeverLeaksBareQuote(t *testing.T) {
	for _, in := range []string{`say "hi`, `foo"bar`, `""`, `a"b"c`, `"unterminated`} {
		got, err := Build(in, "AND")
		if err != nil {
			continue
		}
		if strings.Count(got, `"`)%2 != 0 {
			t.Errorf("Build(%q) = %q has an unpaired quote", in, got)
		}
	}
}
