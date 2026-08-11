package git

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anaknegeri/agent-session/internal/application/ports"
	domainerr "github.com/anaknegeri/agent-session/internal/domain/errors"
)

type runner struct{}

func NewRunner() ports.GitService {
	return &runner{}
}

func (r *runner) Detect(ctx context.Context, dir string) (bool, error) {
	out, err := r.run(ctx, dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(out) == "true", nil
}

func (r *runner) Status(ctx context.Context, dir string) (ports.WorkspaceStatus, error) {
	branch, err := r.run(ctx, dir, "branch", "--show-current")
	if err != nil {
		return ports.WorkspaceStatus{}, domainerr.ErrNotGitRepo
	}

	commit, _ := r.run(ctx, dir, "rev-parse", "--short", "HEAD")
	topLevel, _ := r.run(ctx, dir, "rev-parse", "--show-toplevel")

	changes, _ := r.DiffStat(ctx, dir)

	return ports.WorkspaceStatus{
		Repository: filepath.Base(strings.TrimSpace(topLevel)),
		Branch:     strings.TrimSpace(branch),
		Commit:     strings.TrimSpace(commit),
		Dirty:      len(changes) > 0,
		Changes:    changes,
	}, nil
}

func (r *runner) DiffStat(ctx context.Context, dir string) ([]ports.FileChange, error) {
	if !r.hasHead(ctx, dir) {
		return nil, nil
	}
	out, err := r.run(ctx, dir, "diff", "HEAD", "--name-status")
	if err != nil {
		return nil, fmt.Errorf("git diff HEAD --name-status: %w", err)
	}

	var changes []ports.FileChange
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}
		changes = append(changes, ports.FileChange{Path: parts[1], Status: parts[0]})
	}
	return changes, nil
}

func (r *runner) Diff(ctx context.Context, dir string) (string, error) {
	if !r.hasHead(ctx, dir) {
		return "", nil
	}
	out, err := r.run(ctx, dir, "diff", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git diff HEAD: %w", err)
	}
	return out, nil
}

// hasHead reports whether the repository has a commit to diff against.
func (r *runner) hasHead(ctx context.Context, dir string) bool {
	_, err := r.run(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD")
	return err == nil
}

func (r *runner) run(ctx context.Context, dir string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", dir}, args...)
	out, err := exec.CommandContext(ctx, "git", cmdArgs...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
