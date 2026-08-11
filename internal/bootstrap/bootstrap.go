package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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

	if err := os.MkdirAll(filepath.Join(abs, config.DirName), 0o755); err != nil {
		return nil, fmt.Errorf("create .agent dir: %w", err)
	}

	dbPath := filepath.Join(abs, config.DirName, config.DBFileName)
	store, err := database.OpenStore(dbPath)
	if err != nil {
		return nil, err
	}

	log := logger.New("info")
	wired, err := wire.InitializeApp(store, log)
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

	cfg := config.Default()
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

	log := logger.New("info")
	wired, err := wire.InitializeApp(store, log)
	if err != nil {
		return nil, fmt.Errorf("wire app: %w", err)
	}

	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}

	return &App{App: wired, Root: root, Cfg: cfg}, nil
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

// ResolveRoot returns the project root for dir, walking up to the nearest
// directory containing .agent/ (or dir itself when none exists). It does not
// require the project to be initialized yet.
func ResolveRoot(dir string) (string, error) {
	return findRoot(dir)
}

// findRoot walks up from dir until it finds a directory containing .agent/.
// If none is found the input dir is used.
func findRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve dir: %w", err)
	}
	for d := abs; ; d = filepath.Dir(d) {
		if st, err := os.Stat(filepath.Join(d, config.DirName)); err == nil && st.IsDir() {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
	}
	return abs, nil
}
