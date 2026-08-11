package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type registryEntry struct {
	Path         string `json:"path"`
	Name         string `json:"name"`
	RegisteredAt string `json:"registered_at"`
}

func registryDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".agent-session")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create registry dir: %w", err)
	}
	return dir, nil
}

func registryPath() (string, error) {
	dir, err := registryDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "projects.json"), nil
}

func loadRegistry() ([]registryEntry, error) {
	p, err := registryPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []registryEntry{}, nil
		}
		return nil, fmt.Errorf("read registry: %w", err)
	}
	var entries []registryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	return entries, nil
}

func saveRegistry(entries []registryEntry) error {
	p, err := registryPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	return os.WriteFile(p, data, 0o644)
}

func registerProject(path, name string) error {
	entries, err := loadRegistry()
	if err != nil {
		return err
	}
	for i, e := range entries {
		if e.Path == path {
			entries[i].Name = name
			entries[i].RegisteredAt = time.Now().Format(time.RFC3339)
			return saveRegistry(entries)
		}
	}
	entries = append(entries, registryEntry{
		Path:         path,
		Name:         name,
		RegisteredAt: time.Now().Format(time.RFC3339),
	})
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return saveRegistry(entries)
}

func unregisterProject(path string) error {
	entries, err := loadRegistry()
	if err != nil {
		return err
	}
	var filtered []registryEntry
	for _, e := range entries {
		if e.Path != path {
			filtered = append(filtered, e)
		}
	}
	return saveRegistry(filtered)
}
