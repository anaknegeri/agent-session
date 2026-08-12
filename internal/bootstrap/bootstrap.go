package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	app "github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/config"
	"github.com/anaknegeri/agent-session/internal/infrastructure/database"
	"github.com/anaknegeri/agent-session/internal/wire"
	"github.com/anaknegeri/agent-session/pkg/logger"
)

// App binds the wired services with the project root that owns the store.
type App struct {
	*app.App
	Root string         // project root (where .agent lives)
	Cfg  *config.Config // loaded project config
}

// Init bootstraps a fresh project: creates .agent/, saves config, opens and
// migrates the store, then records session.started (UC-01).
func Init(dir, agent string) (*App, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve dir: %w", err)
	}
	// Store the canonical path so later lookups (which resolve symlinks, e.g.
	// /var -> /private/var on macOS) find the project.
	if resolved, rerr := filepath.EvalSymlinks(abs); rerr == nil {
		abs = resolved
	}

	if err := os.MkdirAll(filepath.Join(abs, config.DirName), 0o755); err != nil {
		return nil, fmt.Errorf("create .agent dir: %w", err)
	}

	dbPath := filepath.Join(abs, config.DirName, config.DBFileName)
	store, err := database.OpenStore(dbPath)
	if err != nil {
		return nil, err
	}

	log := logger.New("info")
	cfg := config.Default()
	wired, err := wire.InitializeApp(store, log, contextBudget(cfg), cfg.Retention)
	if err != nil {
		return nil, fmt.Errorf("wire app: %w", err)
	}

	result, err := wired.Init.Init(context.Background(), abs, agent)
	if err != nil {
		return nil, err
	}

	if !result.IsGit {
		fmt.Fprintln(os.Stderr, gitInitReminder())
	}

	cfg.Project.Name = result.Project.Name
	if err := config.Save(abs, cfg); err != nil {
		return nil, err
	}

	if _, err := wired.Context.WriteContextMD(context.Background(), abs, result.Session.ID); err != nil {
		return nil, err
	}

	return &App{App: wired, Root: abs, Cfg: cfg}, nil
}

// Open resolves the .agent dir (walking up from dir), opens the SQLite store
// and wires the application services.
func Open(dir string) (*App, error) {
	root, err := findRoot(dir)
	if err != nil {
		return nil, err
	}

	dbPath := filepath.Join(root, config.DirName, config.DBFileName)
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w (%s)", errNotInitialized, dbPath)
		}
		return nil, fmt.Errorf("stat db: %w", err)
	}

	store, err := database.OpenStore(dbPath)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}

	log := logger.New("info")
	wired, err := wire.InitializeApp(store, log, contextBudget(cfg), cfg.Retention)
	if err != nil {
		return nil, fmt.Errorf("wire app: %w", err)
	}

	return &App{App: wired, Root: root, Cfg: cfg}, nil
}

// contextBudget maps config to the context render budget.
func contextBudget(cfg *config.Config) ports.ContextBudget {
	b := ports.DefaultContextBudget()
	if cfg == nil {
		return b
	}
	c := cfg.Context
	if c.MaxDecisions > 0 {
		b.MaxDecisions = c.MaxDecisions
	}
	if c.MaxBlockers > 0 {
		b.MaxBlockers = c.MaxBlockers
	}
	if c.MaxFiles > 0 {
		b.MaxFiles = c.MaxFiles
	}
	if c.MaxEvents > 0 {
		b.MaxEvents = c.MaxEvents
	}
	if c.MaxProgress > 0 {
		b.MaxProgress = c.MaxProgress
	}
	if c.MaxItemChars > 0 {
		b.MaxItemChars = c.MaxItemChars
	}
	if c.MaxTotalChars > 0 {
		b.MaxTotalChars = c.MaxTotalChars
	}
	if c.MaxMemory > 0 {
		b.MaxMemory = c.MaxMemory
	}
	b.InjectMemory = c.InjectMemory
	return b
}

var errNotInitialized = fmt.Errorf("project not initialized, run agent-session init")

// Check verifies the store connection is alive.
func (a *App) Check() error {
	if a.Store == nil {
		return fmt.Errorf("store not wired")
	}
	return nil
}

// WorkspaceCheck verifies the root is a git repository.
func (a *App) WorkspaceCheck() error {
	isGit, err := a.Workspace.DetectGit(context.Background(), a.Root)
	if err != nil {
		return err
	}
	if !isGit {
		return fmt.Errorf("not a git repository")
	}
	return nil
}

// gitInitReminder is shown when the project is not yet a git repository.
// Agent Session is Git-aware: git is the source of truth for source code (PRD §22).
func gitInitReminder() string {
	return "Warning: this project is not a git repository.\n" +
		"Agent Session is Git-aware and tracks branch/commit/diff. Run:\n\n" +
		"    git init\n" +
		"    git add . && git commit -m \"init\"\n\n" +
		"then re-run `agent-session init`. Session works in local-only mode meanwhile."
}

// ResolveRoot returns the project root for dir. See findRoot for the resolution
// order. It does not require the project to be initialized yet.
func ResolveRoot(dir string) (string, error) {
	return findRoot(dir)
}

// ProjectEnvVar names the environment variable that pins the project root
// explicitly, bypassing directory discovery.
const ProjectEnvVar = "AGENT_SESSION_PROJECT"

// findRoot resolves the project root, in this order:
//
//  1. $AGENT_SESSION_PROJECT, when it names a directory that exists. The MCP
//     server is registered once at user scope, so it can be started in a
//     directory unrelated to the project being worked on; this is the explicit
//     way to say which project that is.
//  2. The nearest ancestor of dir containing .agent/, searching no further up
//     than the git repository that contains dir.
//  3. The git repository root, when dir is inside one but holds no .agent/ yet.
//  4. dir itself.
//
// The search stops at the repository boundary because it used to run to the
// filesystem root: a developer who had once run `agent-session init` in $HOME
// had a .agent/ above every repository, so any uninitialized repo underneath
// resolved to that unrelated project and recorded its session there. Outside a
// git repository there is no such boundary and the search still runs to the
// root, which is what ProjectEnvVar exists to override.
//
// The result is symlink-resolved so every entrypoint agrees on one canonical
// path.
func findRoot(dir string) (string, error) {
	if override := strings.TrimSpace(os.Getenv(ProjectEnvVar)); override != "" {
		if resolved, err := canonicalDir(override); err == nil {
			return resolved, nil
		}
	}

	abs, err := canonicalDir(dir)
	if err != nil {
		return "", err
	}

	for d := abs; ; d = filepath.Dir(d) {
		if st, err := os.Stat(filepath.Join(d, config.DirName)); err == nil && st.IsDir() {
			return d, nil
		}
		// The repository root is the outermost directory that can belong to this
		// project; anything above it is someone else's. With no .agent/ anywhere
		// inside, the repository root is the answer — it is where one belongs.
		// Stat covers .git as a file too, which is how worktrees and submodules
		// record it.
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
	}
	return abs, nil
}

// canonicalDir turns path into an absolute, symlink-resolved directory, failing
// when it does not exist or is not a directory.
func canonicalDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve dir: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(abs); rerr == nil {
		abs = resolved
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat dir %s: %w", abs, err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("not a directory: %s", abs)
	}
	return abs, nil
}
