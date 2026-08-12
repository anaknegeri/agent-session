package services_test

import (
	"context"
	"testing"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

// TestMemorySearchHandlesPunctuation runs each query against real FTS5. A query
// builder can produce a well-shaped expression that still matches nothing, so
// these assert hits rather than strings.
//
// The failures this pins: a separator like the "/" in "OAuth2 / PKCE" used to
// become its own quoted term, and a quoted term with no indexable character
// matches nothing — so ANDing it returned zero results even though both real
// terms were indexed.
func TestMemorySearchHandlesPunctuation(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	facts := []string{
		"Use OAuth2 with PKCE for the mobile client",
		"PostgreSQL and TimescaleDB store the metrics",
		"the foo.bar helper handles retries",
		"the parser is written in C++ for speed",
		"rotate the tokens on every refresh",
	}
	for _, f := range facts {
		if _, err := fx.app.Memory.Put(ctx, "", entities.KnowledgeKindArchitecture, f, "claude"); err != nil {
			t.Fatalf("put %q: %v", f, err)
		}
	}

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"slash separator", "OAuth2 / PKCE", "OAuth2"},
		{"plus separator", "PostgreSQL + TimescaleDB", "TimescaleDB"},
		{"dotted identifier", "foo.bar", "foo.bar"},
		{"trailing question mark", "TimescaleDB?", "TimescaleDB"},
		{"explicit phrase", `"rotate the tokens"`, "rotate the tokens"},
		{"prefix search", "Postgre*", "PostgreSQL"},
		{"plain term", "PKCE", "PKCE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits, err := fx.app.Memory.Search(ctx, tt.query, 5)
			if err != nil {
				t.Fatalf("search %q: %v", tt.query, err)
			}
			if len(hits) == 0 {
				t.Fatalf("search %q returned no hits; expected the fact containing %q", tt.query, tt.want)
			}
			found := false
			for _, h := range hits {
				if contains(h.Content, tt.want) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("search %q did not surface the fact containing %q; hits=%v", tt.query, tt.want, contentsOf(hits))
			}
		})
	}
}

// TestMemorySearchRejectsUnsearchableQuery verifies a query with nothing
// indexable is reported instead of silently searching for nothing.
func TestMemorySearchRejectsUnsearchableQuery(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	for _, q := range []string{"", "   ", "/ + -"} {
		if _, err := fx.app.Memory.Search(ctx, q, 5); err == nil {
			t.Errorf("search %q returned no error", q)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOfSub(haystack, needle) >= 0
}

func indexOfSub(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func contentsOf(hits []*entities.KnowledgeHit) []string {
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.Content)
	}
	return out
}
