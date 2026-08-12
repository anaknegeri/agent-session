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
	snap, err := r.snapshot(ctx, dir)
	if err != nil {
		return ports.WorkspaceStatus{}, err
	}

	var commit string
	if snap.hasHead {
		out, _ := r.run(ctx, dir, "rev-parse", "--short", "HEAD")
		commit = strings.TrimSpace(out)
	}

	var repository string
	if out, err := r.run(ctx, dir, "rev-parse", "--show-toplevel"); err == nil {
		if top := strings.TrimSpace(out); top != "" {
			repository = filepath.Base(top)
		}
	}

	return ports.WorkspaceStatus{
		Repository: repository,
		Branch:     snap.branch,
		Commit:     commit,
		Dirty:      len(snap.changes) > 0,
		Changes:    snap.changes,
	}, nil
}

func (r *runner) DiffStat(ctx context.Context, dir string) ([]ports.FileChange, error) {
	snap, err := r.snapshot(ctx, dir)
	if err != nil {
		return nil, err
	}
	return snap.changes, nil
}

// gitSnapshot is what a single `git status --porcelain=v2 --branch` yields.
type gitSnapshot struct {
	branch  string
	hasHead bool
	changes []ports.FileChange
}

// snapshot collects branch, HEAD presence and the whole change set in one git
// invocation. Status used to spawn six processes for this — branch, two
// rev-parse, plus DiffStat's own rev-parse, diff and status — and porcelain v2
// reports all of it at once, including untracked files.
func (r *runner) snapshot(ctx context.Context, dir string) (gitSnapshot, error) {
	out, err := r.run(ctx, dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return gitSnapshot{}, domainerr.ErrNotGitRepo
	}
	return parsePorcelainV2(out), nil
}

// parsePorcelainV2 reads git's v2 status format. Paths may contain spaces, so
// every entry is split with a field limit and the path is whatever remains.
// Paths containing newlines or quotes arrive C-quoted; that matches the previous
// implementation's behaviour and is left as-is.
func parsePorcelainV2(out string) gitSnapshot {
	var snap gitSnapshot
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			// "(detached)" is a placeholder, not a branch name
			if head := strings.TrimPrefix(line, "# branch.head "); head != "(detached)" {
				snap.branch = head
			}
		case strings.HasPrefix(line, "# branch.oid "):
			snap.hasHead = strings.TrimPrefix(line, "# branch.oid ") != "(initial)"
		case strings.HasPrefix(line, "1 "):
			// 1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>
			if f := strings.SplitN(line, " ", 9); len(f) == 9 {
				snap.changes = append(snap.changes, ports.FileChange{Path: f[8], Status: changeStatus(f[1])})
			}
		case strings.HasPrefix(line, "2 "):
			// 2 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <score> <path>\t<origPath>
			if f := strings.SplitN(line, " ", 10); len(f) == 10 {
				path, _, _ := strings.Cut(f[9], "\t")
				snap.changes = append(snap.changes, ports.FileChange{Path: path, Status: "R"})
			}
		case strings.HasPrefix(line, "u "):
			// u <XY> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path>
			if f := strings.SplitN(line, " ", 11); len(f) == 11 {
				snap.changes = append(snap.changes, ports.FileChange{Path: f[10], Status: "U"})
			}
		case strings.HasPrefix(line, "? "):
			snap.changes = append(snap.changes, ports.FileChange{Path: strings.TrimPrefix(line, "? "), Status: "??"})
		}
	}
	return snap
}

// changeStatus reduces porcelain v2's two-character <XY> code to the single
// letter ports.FileChange.Status documents. X is the change staged against HEAD
// and Y the change in the working tree; the staged one wins when both are set.
func changeStatus(xy string) string {
	if len(xy) != 2 {
		return xy
	}
	if xy[0] != '.' {
		return string(xy[0])
	}
	return string(xy[1])
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
