package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anaknegeri/agent-session/pkg/version"
)

// ManagedMarker appears in every whole-file resource agent-session generates
// into an agent's configuration directory. A file carrying it is ours; a file
// without it belongs to the user and is left exactly as found.
//
// The marker alone is not enough to decide "ours". It says who wrote the file
// first, not whether the user has since tuned the extension, skill or slash
// command it holds — and rewriting a file the user edited destroys work setup
// has no way to give back. So the marker we write also records a digest of the
// generated body: identical digest means the file is still exactly what we
// produced and may be updated, a different digest means the user changed it and
// only they can decide what happens next.
//
// This is change detection, not authentication. Anyone editing the file can
// recompute the digest; the point is to notice ordinary edits, not to defend
// against someone forging one.
const ManagedMarker = "agent-session:managed"

// markerHeadLines bounds how far into a file the marker is honoured. Matching it
// anywhere means a document that merely quotes the marker — this package's own
// documentation, a user's notes on how agent-session works — becomes a file we
// believe we may overwrite. Every generated resource carries it in the first few
// lines (line 1 for the TypeScript extensions, line 4-5 after YAML frontmatter
// for skills, rules and slash commands).
const markerHeadLines = 10

// digestField is the key under which the marker records the body digest.
const digestField = "sha256="

// versionField records which agent-session generated the file. It is for the
// person reading the file, and deliberately outside the digest: a version bump
// must not turn every installed file into a conflict.
const versionField = "version="

// ErrUserModified is the sentinel behind every conflict the managed helpers
// report, so a caller that installs several files can tell "the user edited this
// one" from "the disk is broken" without matching on strings.
var ErrUserModified = errors.New("modified since agent-session generated it")

// Conflict names a generated file the user has edited, which the managed helpers
// therefore neither rewrote nor removed.
//
// It is a returned value rather than a log line on purpose: the only person who
// can decide what to do with a hand-tuned extension is the one who tuned it, and
// they cannot decide if setup silently skips the file and reports success.
type Conflict struct{ Path string }

func (c *Conflict) Error() string {
	return fmt.Sprintf("%s: %v — left as you have it", c.Path, ErrUserModified)
}

func (c *Conflict) Unwrap() error { return ErrUserModified }

// SplitConflicts separates the user-modified conflicts inside err — including
// the leaves of an errors.Join tree — from the failures that should actually
// stop the caller. A conflict is news for the user, not a failed install.
func SplitConflicts(err error) ([]*Conflict, error) {
	var conflicts []*Conflict
	var rest []error
	var walk func(error)
	walk = func(e error) {
		if e == nil {
			return
		}
		// The join tree has to be taken apart first: errors.As would happily match
		// a single conflict buried in a join and we would drop everything beside it.
		if joined, ok := e.(interface{ Unwrap() []error }); ok {
			for _, sub := range joined.Unwrap() {
				walk(sub)
			}
			return
		}
		var c *Conflict
		if errors.As(e, &c) {
			conflicts = append(conflicts, c)
			return
		}
		rest = append(rest, e)
	}
	walk(err)
	return conflicts, errors.Join(rest...)
}

// WriteManaged writes generated content at path, stamping its marker with this
// version and a digest of the body. It reports whether it wrote anything.
//
// Absent           → created.
// Ours, unchanged  → updated (or left alone when already identical).
// Ours, edited     → left alone, reported as *Conflict.
// No marker        → left alone, no error: it is the user's own file.
func WriteManaged(path, content string) (bool, error) {
	stamped, err := stampMarker(content)
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}

	existing, readErr := os.ReadFile(path)
	switch {
	case readErr == nil:
		switch classify(string(existing)) {
		case foreign:
			return false, nil
		case edited:
			return false, &Conflict{Path: path}
		}
		if string(existing) == stamped {
			return false, nil
		}
	case !os.IsNotExist(readErr):
		// A file we cannot read is not a file we may replace: overwriting on a
		// permission or I/O error is how a managed helper destroys the one thing
		// it is supposed to protect.
		return false, fmt.Errorf("read %s: %w", path, readErr)
	}

	// Directories are created only once the answer is "yes, write". Doing it
	// first leaves .omp/extensions/ and .pi/skills/agent-session/ behind in a
	// project we then decline to touch, advertising wiring that is not there.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := WriteFileAtomic(path, []byte(stamped), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveManaged deletes path only while it is still ours by the same rule
// WriteManaged applies, and reports whether it removed anything. A missing file
// is not an error: uninstall has to be re-runnable. A file the user edited is
// kept and reported as *Conflict — uninstall is not a licence to delete their
// work either.
func RemoveManaged(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		// Swallowing this reported a permission-denied file as a successful
		// uninstall, leaving an extension that keeps resuming and checkpointing
		// after the user asked for it to be gone.
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	switch classify(string(data)) {
	case foreign:
		return false, nil
	case edited:
		return false, &Conflict{Path: path}
	}
	if err := os.Remove(path); err != nil {
		return false, fmt.Errorf("remove %s: %w", path, err)
	}
	return true, nil
}

// ownership is what an existing file's head says about who owns it.
type ownership int

const (
	foreign ownership = iota // no marker: the user's own file
	ours                     // our marker, body still what we generated
	edited                   // our marker, body changed since
)

func classify(existing string) ownership {
	lines := strings.Split(existing, "\n")
	idx := markerLine(lines)
	if idx < 0 {
		return foreign
	}
	recorded := recordedDigest(lines[idx])
	if recorded == "" {
		// 0.1.7 and 0.1.8 wrote a bare marker with no digest. Nothing can tell
		// whether those files were edited, and calling them modified would freeze
		// every existing install out of updates forever, so they are adopted.
		return ours
	}
	if recorded == bodyDigest(lines, idx) {
		return ours
	}
	return edited
}

// stampMarker rewrites the marker line of generated content so it records the
// version that produced the file and a digest of everything else in it.
func stampMarker(content string) (string, error) {
	lines := strings.Split(content, "\n")
	idx := markerLine(lines)
	if idx < 0 {
		return "", fmt.Errorf("generated content carries no %q marker in its first %d lines", ManagedMarker, markerHeadLines)
	}
	stamp := ManagedMarker + " " + versionField + version.Version + " " + digestField + bodyDigest(lines, idx)
	lines[idx] = strings.Replace(lines[idx], ManagedMarker, stamp, 1)
	return strings.Join(lines, "\n"), nil
}

// bodyDigest hashes every line except the marker's own. Excluding that line is
// what makes the digest computable at all — it cannot contain itself — and it is
// also what keeps the digest stable across a version bump, so upgrading updates
// files instead of declaring them all modified.
func bodyDigest(lines []string, markerIdx int) string {
	h := sha256.New()
	for i, line := range lines {
		if i == markerIdx {
			continue
		}
		h.Write([]byte(line))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// recordedDigest pulls the digest out of a marker line, whatever comment syntax
// surrounds it: `<!-- agent-session:managed version=x sha256=y -->` and
// `// agent-session:managed version=x sha256=y — written by …` both work.
func recordedDigest(line string) string {
	for _, field := range strings.Fields(line) {
		if value, ok := strings.CutPrefix(field, digestField); ok {
			// A marker closing an HTML comment can leave the delimiter attached
			// when there is no space before it; hex never contains either byte.
			return strings.TrimRight(value, "->")
		}
	}
	return ""
}

func markerLine(lines []string) int {
	for i, line := range lines {
		if i >= markerHeadLines {
			return -1
		}
		if strings.Contains(line, ManagedMarker) {
			return i
		}
	}
	return -1
}
