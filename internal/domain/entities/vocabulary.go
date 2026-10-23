package entities

import "sort"

// sortedKeys returns the members of a frozen vocabulary in sorted order.
//
// Every closed value set in this package needs the same two things: a predicate
// that says whether a value belongs to it, and the list itself, because a
// rejection that does not name the accepted values leaves the caller guessing
// what to send instead. Sorted, so the list an error message quotes is stable
// between runs rather than map order.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
