package agent

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anaknegeri/agent-session/pkg/version"
)

// generated builds what a renderer produces: the bare marker on line 1, body
// after it. WriteManaged is what turns the bare marker into a stamped one.
func generated(body string) string {
	return "<!-- " + ManagedMarker + " -->\n\n" + body
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWriteManagedCreatesAndStamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "skill.md")

	wrote, err := WriteManaged(path, generated("first body\n"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !wrote {
		t.Fatal("WriteManaged reported no write for a file that did not exist")
	}

	got := readFile(t, path)
	if !strings.Contains(got, ManagedMarker+" version="+version.Version+" sha256=") {
		t.Fatalf("marker not stamped with version and digest:\n%s", got)
	}
	if !strings.Contains(got, "first body") {
		t.Fatalf("body missing:\n%s", got)
	}
	// The marker still closes its HTML comment, or the skill stops parsing.
	if !strings.HasSuffix(strings.SplitN(got, "\n", 2)[0], "-->") {
		t.Fatalf("stamping broke the comment syntax: %q", strings.SplitN(got, "\n", 2)[0])
	}
}

func TestWriteManagedUpdatesAnUntouchedFileAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skill.md")
	if _, err := WriteManaged(path, generated("v1\n")); err != nil {
		t.Fatalf("first write: %v", err)
	}

	// Re-running setup with the same content changes nothing.
	wrote, err := WriteManaged(path, generated("v1\n"))
	if err != nil {
		t.Fatalf("re-install: %v", err)
	}
	if wrote {
		t.Error("re-installing identical content rewrote the file")
	}

	// A new release ships corrected content: the file is still ours, so it updates.
	wrote, err = WriteManaged(path, generated("v2\n"))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !wrote {
		t.Fatal("a clean generated file was not updated")
	}
	if got := readFile(t, path); !strings.Contains(got, "v2") {
		t.Fatalf("update did not land:\n%s", got)
	}
}

// A user who tunes a generated skill, extension or slash command must not lose it
// on the next `init` — and must be told, or they only find out when the edit
// silently stops being carried forward.
func TestWriteManagedKeepsAndReportsAUserEditedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skill.md")
	if _, err := WriteManaged(path, generated("shipped\n")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	edited := readFile(t, path) + "\n# my own note\n"
	writeFile(t, path, edited)

	wrote, err := WriteManaged(path, generated("shipped v2\n"))
	if wrote {
		t.Error("a file the user edited was rewritten")
	}
	if got := readFile(t, path); got != edited {
		t.Errorf("the user's edit was lost:\n%s", got)
	}
	if !errors.Is(err, ErrUserModified) {
		t.Fatalf("conflict not reported, got %v", err)
	}
	var conflict *Conflict
	if !errors.As(err, &conflict) || conflict.Path != path {
		t.Fatalf("conflict does not name the file: %v", err)
	}
}

func TestWriteManagedLeavesAFileWithNoMarkerAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skill.md")
	const mine = "# my own skill\nnothing to do with agent-session\n"
	writeFile(t, path, mine)

	wrote, err := WriteManaged(path, generated("ours\n"))
	if err != nil {
		t.Fatalf("write over a user file: %v", err)
	}
	if wrote {
		t.Error("a file the user wrote was overwritten")
	}
	if got := readFile(t, path); got != mine {
		t.Errorf("user file modified:\n%s", got)
	}
}

// The upgrade path. 0.1.7 and 0.1.8 wrote a bare marker with no digest; treating
// those as user-modified would freeze every existing install out of updates.
func TestWriteManagedAdoptsTheHashlessMarkerFromOlderReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "extensions", "agent-session.ts")
	const oldInstall = "// agent-session:managed — written by `agent-session init`.\n" +
		"export default { name: 'agent-session' }\n"
	writeFile(t, path, oldInstall)

	wrote, err := WriteManaged(path, "// "+ManagedMarker+" — written by `agent-session init`.\nexport default { name: 'agent-session', version: 2 }\n")
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if !wrote {
		t.Fatal("a 0.1.8 install was not adopted, so it would never receive updates again")
	}
	got := readFile(t, path)
	if !strings.Contains(got, "version: 2") {
		t.Fatalf("update did not land:\n%s", got)
	}
	if !strings.Contains(got, "sha256=") {
		t.Fatalf("adopted file was not stamped, so the next run cannot tell edits:\n%s", got)
	}
}

// A document that merely quotes the marker — release notes, a user's own notes on
// how agent-session works — is not a file we may overwrite.
func TestWriteManagedIgnoresAMarkerQuotedInAnUnrelatedDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.md")
	doc := strings.Repeat("# notes\n", 12) +
		"agent-session writes `" + ManagedMarker + "` into the files it generates.\n"
	writeFile(t, path, doc)

	wrote, err := WriteManaged(path, generated("ours\n"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if wrote {
		t.Error("a document quoting the marker was treated as ours and overwritten")
	}
	if got := readFile(t, path); got != doc {
		t.Errorf("document modified:\n%s", got)
	}

	removed, err := RemoveManaged(path)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed {
		t.Error("a document quoting the marker was deleted by uninstall")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("document gone: %v", err)
	}
}

func TestRemoveManaged(t *testing.T) {
	dir := t.TempDir()

	ours := filepath.Join(dir, "ours.md")
	if _, err := WriteManaged(ours, generated("ours\n")); err != nil {
		t.Fatalf("write ours: %v", err)
	}
	removed, err := RemoveManaged(ours)
	if err != nil || !removed {
		t.Fatalf("our own file not removed: removed=%v err=%v", removed, err)
	}

	// Re-runnable: a missing file is not an error and reports no work.
	removed, err = RemoveManaged(ours)
	if err != nil || removed {
		t.Fatalf("second remove: removed=%v err=%v", removed, err)
	}

	// A file the user edited survives uninstall and is reported.
	edited := filepath.Join(dir, "edited.md")
	if _, err := WriteManaged(edited, generated("ours\n")); err != nil {
		t.Fatalf("write edited: %v", err)
	}
	writeFile(t, edited, readFile(t, edited)+"# my own note\n")
	removed, err = RemoveManaged(edited)
	if removed {
		t.Error("uninstall deleted a file the user had edited")
	}
	if !errors.Is(err, ErrUserModified) {
		t.Fatalf("kept file not reported: %v", err)
	}
	if _, statErr := os.Stat(edited); statErr != nil {
		t.Errorf("edited file gone: %v", statErr)
	}
}

// Swallowing a read error reported a permission-denied file as a successful
// uninstall, leaving an extension that keeps resuming and checkpointing after the
// user asked for it to be gone.
func TestRemoveManagedReportsAnUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads regardless of mode")
	}
	path := filepath.Join(t.TempDir(), "locked.md")
	writeFile(t, path, generated("ours\n"))
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	removed, err := RemoveManaged(path)
	if removed {
		t.Error("reported a removal it could not have made")
	}
	if err == nil {
		t.Fatal("an unreadable file was reported as a clean uninstall")
	}
	if errors.Is(err, ErrUserModified) {
		t.Fatalf("a read failure was reported as a user edit: %v", err)
	}
}

// Declining to write must not leave directories behind: creating
// .omp/extensions/ in a project we then leave alone advertises wiring that is
// not there.
func TestWriteManagedCreatesNoDirectoriesWhenItCannotOwnTheContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "extensions", "agent-session.ts")

	if _, err := WriteManaged(path, "export default {}\n"); err == nil {
		t.Fatal("content with no marker was written as if we owned it")
	}
	if _, err := os.Stat(filepath.Join(root, "extensions")); !os.IsNotExist(err) {
		t.Errorf("directory created for a file that was never written: %v", err)
	}
}

func TestSplitConflictsSeparatesEditsFromFailures(t *testing.T) {
	boom := errors.New("disk on fire")
	joined := errors.Join(&Conflict{Path: "a.md"}, boom, errors.Join(&Conflict{Path: "b.md"}))

	conflicts, rest := SplitConflicts(joined)
	if len(conflicts) != 2 || conflicts[0].Path != "a.md" || conflicts[1].Path != "b.md" {
		t.Fatalf("conflicts not collected: %v", conflicts)
	}
	if !errors.Is(rest, boom) {
		t.Fatalf("the real failure was dropped: %v", rest)
	}
	if errors.Is(rest, ErrUserModified) {
		t.Fatalf("a conflict leaked into the failures: %v", rest)
	}
	if conflicts, rest := SplitConflicts(nil); conflicts != nil || rest != nil {
		t.Fatalf("nil error produced %v / %v", conflicts, rest)
	}
}

func TestWriteFileAtomicReplacesWithoutLeavingTemporaries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	writeFile(t, path, "a much longer previous version of this file\n")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := WriteFileAtomic(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("atomic write: %v", err)
	}

	// A different file now stands at the path. That is the whole guarantee: the
	// old one was never truncated, so a crash mid-write cannot leave a config
	// holding half a document — which ReadJSONConfig then refuses to overwrite,
	// stranding the user's own MCP servers.
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Error("the target was rewritten in place instead of being replaced")
	}
	if got := readFile(t, path); got != "{}\n" {
		t.Errorf("content not replaced: %q", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Errorf("temp file left in the user's config directory: %v", entries)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// CreateTemp makes 0600; an agent that cannot read the file it was given is
	// the same as no wiring at all.
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode not applied: %v", info.Mode().Perm())
	}
}

// The concurrency guarantee, stated without a race: a reader that already has the
// file open keeps reading the old, complete document. Truncate-then-write instead
// empties that same file underneath them, which is how a crash or a second `init`
// leaves ~/.claude.json holding half a document — after which ReadJSONConfig
// (correctly) refuses to overwrite it and the user's own MCP servers are stuck
// behind a hand edit.
func TestWriteFileAtomicNeverEmptiesTheFileAReaderHolds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claude.json")
	const before = `{"mcpServers":{"mine":{"command":"my-mcp"}}}`
	writeFile(t, path, before)

	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	if err := WriteFileAtomic(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("atomic write: %v", err)
	}

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != before {
		t.Errorf("the config was rewritten under an open reader: %q", got)
	}
}
