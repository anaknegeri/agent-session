package contract_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/anaknegeri/agent-session/internal/config"
	"github.com/anaknegeri/agent-session/internal/domain/entities"
	"github.com/anaknegeri/agent-session/pkg/contract"
	"github.com/anaknegeri/agent-session/pkg/ids"
)

// formatEntriesV1 is the .agent/ layout an Agent Session Format v1 project has
// after init (docs/spec/format-v1.md).
//
// This is the outermost contract: a human looks for these paths, .gitignore rules
// name them, and another build of this binary opens the project by finding
// .agent/ and reading exactly these files. Renaming or moving one does not just
// break a reader, it makes an existing project unopenable.
var formatEntriesV1 = []string{
	"config.toml",
	"context/current.md",
	"session.db",
}

// idPrefixesV1 is the frozen prefix per record kind. Prefixes end up in event
// payloads, in handoff documents and in every CLI argument a human types, so they
// are part of the format rather than an implementation detail — and two prefixes
// for one kind is a defect in the contract, not a cosmetic inconsistency.
var idPrefixesV1 = map[string]string{
	"agent session": "asess",
	"artifact":      "art",
	"blocker":       "blocker",
	"checkpoint":    "chk",
	"decision":      "decision",
	"event":         "evt",
	"handoff":       "handoff",
	"memory":        "mem",
	"project":       "proj",
	"session":       "sess",
	"task":          "task",
}

// TestFormatLayoutV1 holds the directory a fresh project gets.
func TestFormatLayoutV1(t *testing.T) {
	app := newProject(t)

	dir := filepath.Join(app.Root, config.DirName)
	for _, entry := range formatEntriesV1 {
		if _, err := os.Stat(filepath.Join(dir, entry)); err != nil {
			t.Errorf("a v1 project has %s/%s, and this one does not: %v\n see docs/spec/format-v1.md",
				config.DirName, entry, err)
		}
	}

	// The names themselves, so a rename fails here rather than only in whichever
	// test happened to hardcode the old string.
	for name, want := range map[string]string{
		"DirName":        ".agent",
		"ConfigFileName": "config.toml",
		"DBFileName":     "session.db",
		"ContextDir":     "context",
		"CheckpointsDir": "checkpoints",
		"SyncModeLocal":  "local-only",
	} {
		got := map[string]string{
			"DirName":        config.DirName,
			"ConfigFileName": config.ConfigFileName,
			"DBFileName":     config.DBFileName,
			"ContextDir":     config.ContextDir,
			"CheckpointsDir": config.CheckpointsDir,
			"SyncModeLocal":  config.SyncModeLocal,
		}[name]
		if got != want {
			t.Errorf("config.%s is %q, v1 says %q", name, got, want)
		}
	}
}

// TestFormatVersionIsRecordedV1 is what makes the format version usable: the
// directory has to say which layout it follows, in the one file a reader opens
// before it knows anything else.
func TestFormatVersionIsRecordedV1(t *testing.T) {
	app := newProject(t)

	path := filepath.Join(app.Root, config.DirName, config.ConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "[format]") {
		t.Errorf("the written config has no [format] section:\n%s", text)
	}
	if !strings.Contains(text, fmt.Sprintf("version = %d", contract.Format)) {
		t.Errorf("the written config does not declare version = %d:\n%s", contract.Format, text)
	}

	cfg, err := config.Load(app.Root)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Format.Version != contract.Format {
		t.Errorf("loaded format version is %d, this build writes v%d", cfg.Format.Version, contract.Format)
	}
}

// TestFormatAcceptsPreVersionedConfig covers the .agent/ directories already on
// disk. They predate [format], and v1 is defined to be their layout, so opening
// one must work without a migration.
func TestFormatAcceptsPreVersionedConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, config.DirName), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(root, config.DirName, config.ConfigFileName)
	if err := os.WriteFile(path, []byte("[project]\nname = \"old\"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("a config written before [format] must still load: %v", err)
	}
	if cfg.Format.Version != contract.Format {
		t.Errorf("a pre-[format] config reports version %d, want %d", cfg.Format.Version, contract.Format)
	}
	if cfg.Project.Name != "old" {
		t.Errorf("project name lost while loading: %q", cfg.Project.Name)
	}
}

// TestFormatRejectsFutureVersion is the other half. Opening a directory laid out
// by a build that knows more than this one risks writing into it on assumptions
// that no longer hold, so it has to stop and say what to do instead.
func TestFormatRejectsFutureVersion(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, config.DirName), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(root, config.DirName, config.ConfigFileName)
	body := fmt.Sprintf("[format]\nversion = %d\n", contract.Format+1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := config.Load(root); err == nil {
		t.Fatal("a .agent/ directory from a newer format version was accepted")
	} else if !strings.Contains(err.Error(), "upgrade agent-session") {
		t.Errorf("the error should tell the user what to do, got: %v", err)
	}
}

// TestFormatConfigDefaultsV1 holds the configuration table of
// docs/spec/format-v1.md:57-58 against the config a real project is opened with.
// The numbers are literals here on purpose: the loader agrees with any default it
// returns, while a project on disk has no config.toml of its own and inherits
// whatever this build says. Changing one number re-renders every existing
// project's context, or re-bounds its checkpoint pruning, with nothing on disk
// having changed.
func TestFormatConfigDefaultsV1(t *testing.T) {
	app := newProject(t)

	cfg, err := config.Load(app.Root)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	// format-v1.md:57 | [context] | context budget: max_decisions 5, max_blockers 3,
	// max_files 8, max_events 10, max_progress 10, max_item_chars 200,
	// max_total_chars 4000, inject_memory true, max_memory 3
	budget := map[string]int{
		"max_decisions":   cfg.Context.MaxDecisions,
		"max_blockers":    cfg.Context.MaxBlockers,
		"max_files":       cfg.Context.MaxFiles,
		"max_events":      cfg.Context.MaxEvents,
		"max_progress":    cfg.Context.MaxProgress,
		"max_item_chars":  cfg.Context.MaxItemChars,
		"max_total_chars": cfg.Context.MaxTotalChars,
		"max_memory":      cfg.Context.MaxMemory,
	}
	for key, want := range map[string]int{
		"max_decisions":   5,
		"max_blockers":    3,
		"max_files":       8,
		"max_events":      10,
		"max_progress":    10,
		"max_item_chars":  200,
		"max_total_chars": 4000,
		"max_memory":      3,
	} {
		if budget[key] != want {
			t.Errorf("context %s is %d, v1 says %d\n see docs/spec/format-v1.md", key, budget[key], want)
		}
	}
	if !cfg.Context.InjectMemory {
		t.Error("inject_memory defaults to false, v1 says true\n see docs/spec/format-v1.md")
	}

	// format-v1.md:58 | [retention] | checkpoints kept per kind: manual 50,
	// auto 20, precompact 10, handoff 20. Read through CheckpointLimit, because
	// that is how pruning reaches the number: a kind it stops matching is
	// unbounded, whatever the field says.
	for kind, want := range map[string]int{
		"manual":     50,
		"auto":       20,
		"precompact": 10,
		"handoff":    20,
	} {
		if got := cfg.Retention.CheckpointLimit(kind); got != want {
			t.Errorf("retention keeps %d %s checkpoints, v1 says %d\n see docs/spec/format-v1.md", got, kind, want)
		}
	}

	// The key names as init wrote them. A renamed toml tag still loads and still
	// reports these defaults — it just silently ignores the value an existing
	// project already wrote under the old name.
	data, err := os.ReadFile(filepath.Join(app.Root, config.DirName, config.ConfigFileName))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	for _, key := range []string{
		"max_decisions", "max_blockers", "max_files", "max_events", "max_progress",
		"max_item_chars", "max_total_chars", "inject_memory", "max_memory",
		"max_manual", "max_auto", "max_precompact", "max_handoff",
	} {
		if !strings.Contains(text, key) {
			t.Errorf("the written config has no %s key, so a config that sets it is ignored:\n%s", key, text)
		}
	}
}

// TestIDPrefixesV1 drives the real write paths and checks the IDs they mint. The
// prefixes are asserted from records rather than from the ids.New call sites,
// because a call site can be right in one service and wrong in another — which is
// exactly what happened on the import path.
func TestIDPrefixesV1(t *testing.T) {
	app := newProject(t)
	ctx := context.Background()
	sessionID := activeSession(t, app)

	projectID, err := app.ResolveProjectID(ctx, app.Root)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}

	task, err := app.Task.Create(ctx, sessionID, "prefix contract", "claude")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	decision, err := app.Decision.Create(ctx, sessionID, "freeze the prefixes", "they are typed by humans", "claude")
	if err != nil {
		t.Fatalf("create decision: %v", err)
	}
	blocker, err := app.Decision.CreateBlocker(ctx, sessionID, "waiting on the spec", "claude")
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	checkpoint, err := app.Checkpoint.Create(ctx, sessionID, "prefix", "", "claude")
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	memory, err := app.Memory.Put(ctx, sessionID, "note", "prefixes are part of the format", "claude")
	if err != nil {
		t.Fatalf("put memory: %v", err)
	}
	artifactID, err := app.Artifact.Store(ctx, sessionID, "log", "", "artifact body")
	if err != nil {
		t.Fatalf("store artifact: %v", err)
	}
	agentSession, err := app.Store.AgentSessions().GetLatest(ctx, sessionID)
	if err != nil {
		t.Fatalf("get agent session: %v", err)
	}
	if _, err := app.Handoff.Handoff(ctx, sessionID, "codex"); err != nil {
		t.Fatalf("handoff: %v", err)
	}

	events, err := app.Event.List(ctx, sessionID, 200)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("no events to check an event ID against")
	}
	handoffID := handoffIDFrom(t, events)

	observed := map[string]string{
		"agent session": agentSession.ID,
		"artifact":      artifactID,
		"blocker":       blocker.ID,
		"checkpoint":    checkpoint.ID,
		"decision":      decision.ID,
		"event":         events[0].ID,
		"handoff":       handoffID,
		"memory":        memory.ID,
		"project":       projectID,
		"session":       sessionID,
		"task":          task.ID,
	}

	// Every kind in the spec has to be covered, or the table would freeze
	// prefixes nothing checks.
	for kind := range idPrefixesV1 {
		if _, ok := observed[kind]; !ok {
			t.Errorf("no record was produced for the %q prefix; the test cannot hold what it does not observe", kind)
		}
	}

	shape := regexp.MustCompile(`^[a-z]+_[0-9a-f]{16}$`)
	kinds := make([]string, 0, len(observed))
	for kind := range observed {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		id := observed[kind]
		want, specified := idPrefixesV1[kind]
		if !specified {
			t.Errorf("%s IDs (%s) are minted but not in the v1 prefix table", kind, id)
			continue
		}
		if !strings.HasPrefix(id, want+"_") {
			t.Errorf("%s ID %q does not use the v1 prefix %q\n see docs/spec/format-v1.md", kind, id, want)
		}
		if !shape.MatchString(id) {
			t.Errorf("%s ID %q is not <prefix>_<16 hex>", kind, id)
		}
	}
}

// TestIDShapeV1 pins the generator itself, so the shape asserted above is a rule
// rather than a coincidence of the records this test happened to create.
func TestIDShapeV1(t *testing.T) {
	shape := regexp.MustCompile(`^task_[0-9a-f]{16}$`)
	seen := make(map[string]bool, 64)
	for i := 0; i < 64; i++ {
		id := ids.New("task")
		if !shape.MatchString(id) {
			t.Fatalf("ids.New produced %q, v1 says <prefix>_<16 lowercase hex>", id)
		}
		if seen[id] {
			t.Fatalf("ids.New repeated %q within 64 calls", id)
		}
		seen[id] = true
	}
}

// TestTimestampEncodingV1 freezes the two encodings a timestamp has, and the
// reason the on-disk one is not RFC3339Nano: timestamps live in TEXT columns, so
// every ORDER BY compares them as strings. A variable-width fraction makes string
// order disagree with chronological order, and every "latest" query — the
// checkpoint behind next_action, restore, retention pruning — is one of those.
func TestTimestampEncodingV1(t *testing.T) {
	if entities.TimestampLayout != "2006-01-02T15:04:05.000000000Z07:00" {
		t.Errorf("the on-disk timestamp layout changed to %q; every stored timestamp is written in the old one\n see docs/spec/format-v1.md",
			entities.TimestampLayout)
	}

	base := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	ordered := []time.Time{
		base,
		base.Add(500 * time.Millisecond),
		base.Add(1 * time.Second),
		base.Add(1500 * time.Millisecond),
	}

	encoded := make([]string, 0, len(ordered))
	for _, at := range ordered {
		value, err := entities.Timestamp{Time: at}.Value()
		if err != nil {
			t.Fatalf("encode %s: %v", at, err)
		}
		encoded = append(encoded, value.(string))
	}
	for i := 1; i < len(encoded); i++ {
		if encoded[i-1] >= encoded[i] {
			t.Errorf("string order disagrees with time order: %q >= %q — ORDER BY on a TEXT column would return the wrong row",
				encoded[i-1], encoded[i])
		}
	}
	width := len(encoded[0])
	for _, value := range encoded {
		if len(value) != width {
			t.Errorf("encoded timestamps have different widths (%d vs %d): %q", width, len(value), value)
		}
	}

	// JSON is the other encoding: plain RFC3339, because it is read by agents and
	// by anything consuming a checkpoint, not sorted as text.
	raw, err := json.Marshal(entities.Timestamp{Time: base})
	if err != nil {
		t.Fatalf("marshal timestamp: %v", err)
	}
	if got, want := string(raw), `"2026-08-13T10:00:00Z"`; got != want {
		t.Errorf("timestamp JSON is %s, v1 says %s", got, want)
	}
}

// handoffIDFrom pulls the handoff ID out of the event that carries it, so the
// prefix is read from the payload another program would read it from.
func handoffIDFrom(t *testing.T, events []*entities.SessionEvent) string {
	t.Helper()
	for _, ev := range events {
		if ev.Type != entities.EventHandoffCreated {
			continue
		}
		var payload struct {
			HandoffID string `json:"handoff_id"`
		}
		if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
			t.Fatalf("handoff.created payload is not JSON: %v", err)
		}
		return payload.HandoffID
	}
	t.Fatal("no handoff.created event to read a handoff ID from")
	return ""
}
