// Package ftsquery turns a human or agent search query into a SQLite FTS5 MATCH
// expression.
package ftsquery

import (
	"fmt"
	"strings"
	"unicode"
)

// Build joins the query's terms with operator ("AND" or "OR"), quoting each one
// so ordinary punctuation cannot be read as FTS5 syntax.
//
// Splitting on whitespace alone was not enough. A separator such as the "/" in
// "OAuth2 / PKCE" became its own term, and a quoted term holding no indexable
// character matches nothing — so ANDing it zeroed a query whose other terms were
// present in the index. Embedded double quotes were passed through unescaped,
// which corrupted the surrounding term.
//
// Supported forms:
//
//	refresh token      two terms, joined by operator
//	"refresh token"    one phrase, matched in order
//	oauth*             prefix match
//	"refresh tok"*     phrase prefix match
//
// Returns an error when nothing indexable remains, so the caller reports an
// unusable query instead of silently searching for nothing.
func Build(query, operator string) (string, error) {
	terms := split(query)

	parts := make([]string, 0, len(terms))
	for _, t := range terms {
		if !hasIndexable(t.text) {
			continue
		}
		// split() consumes every quote as a phrase delimiter, so a term should never
		// carry one; doubling any that appear keeps the expression well-formed even
		// if that ever stops holding.
		part := `"` + strings.ReplaceAll(t.text, `"`, `""`) + `"`
		if t.prefix {
			part += "*"
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("search query has no searchable terms: %q", query)
	}
	return strings.Join(parts, " "+operator+" "), nil
}

type term struct {
	text   string
	prefix bool
}

// split breaks the query into terms, keeping double-quoted runs together. An
// unterminated quote is treated as running to the end of the query rather than
// as an error, since a truncated phrase is still a usable search.
func split(query string) []term {
	var terms []term
	var current strings.Builder
	inPhrase := false

	flush := func(prefix bool) {
		if text := strings.TrimSpace(current.String()); text != "" {
			terms = append(terms, term{text: text, prefix: prefix})
		}
		current.Reset()
	}

	runes := []rune(query)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '"':
			if inPhrase {
				// a phrase may be followed by * to match a prefix
				if i+1 < len(runes) && runes[i+1] == '*' {
					flush(true)
					i++
				} else {
					flush(false)
				}
				inPhrase = false
				continue
			}
			flush(false)
			inPhrase = true
		case unicode.IsSpace(r) && !inPhrase:
			flush(false)
		case r == '*' && !inPhrase && current.Len() > 0:
			flush(true)
		default:
			current.WriteRune(r)
		}
	}
	flush(false)
	return terms
}

// hasIndexable reports whether s holds anything FTS5's tokenizer will index.
// Punctuation on its own produces no tokens, so such a term can never match.
func hasIndexable(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
