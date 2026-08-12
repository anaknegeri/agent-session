package services_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/anaknegeri/agent-session/internal/domain/entities"
)

// TestSyncFileChangesAutoRecords verifies that unrecorded git changes are
// auto-appended as file.changed events by context.get (via SyncService).
func TestSyncFileChangesAutoRecords(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	// create a file so the git tree is dirty
	newFile := filepath.Join(fx.dir, "new_file.go")
	if err := os.WriteFile(newFile, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// context.get should auto-record the file change
	if _, err := fx.app.Context.Get(ctx, initRes.Session.ID, "summary"); err != nil {
		t.Fatalf("context.get: %v", err)
	}

	events, err := fx.app.Event.List(ctx, initRes.Session.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range events {
		if e.Type == entities.EventFileChanged {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected file.changed event to be auto-recorded by context.get")
	}
}

// TestSyncFileChangesDoesNotDuplicate verifies auto-record is idempotent:
// running context.get twice does not append duplicate file.changed events.
func TestSyncFileChangesDoesNotDuplicate(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	initRes, err := fx.app.Init.Init(ctx, fx.dir, "claude")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	newFile := filepath.Join(fx.dir, "dup.go")
	if err := os.WriteFile(newFile, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := fx.app.Context.Get(ctx, initRes.Session.ID, "summary"); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.Context.Get(ctx, initRes.Session.ID, "summary"); err != nil {
		t.Fatal(err)
	}

	events, _ := fx.app.Event.List(ctx, initRes.Session.ID, 100)
	count := 0
	for _, e := range events {
		if e.Type == entities.EventFileChanged {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 file.changed event, got %d", count)
	}
}
