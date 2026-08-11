package services_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	app "github.com/anaknegeri/agent-session/internal/application/services"
	"github.com/anaknegeri/agent-session/internal/infrastructure/database"
	"github.com/anaknegeri/agent-session/internal/wire"
	"github.com/anaknegeri/agent-session/pkg/logger"
)

type fixture struct {
	dir string
	app *app.App
}

// newFixture creates a git repo and a wired app backed by :memory: SQLite.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-q", "-b", "main")
	gitCmd(t, dir, "config", "user.email", "t@t.co")
	gitCmd(t, dir, "config", "user.name", "t")
	writeFile(t, dir, "server.go", "package oauth\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-qm", "init")

	store, err := database.OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	app, err := wire.InitializeApp(store, logger.New("error"), ports.DefaultContextBudget())
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{dir: dir, app: app}
}

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
