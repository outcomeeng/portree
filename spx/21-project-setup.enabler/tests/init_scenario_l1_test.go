package projectsetup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fairy-pitta/portree/internal/config"
)

// TestInitCreatesDefaultConfig verifies that running Init in an empty repo
// creates a valid .portree.toml that config.Load accepts.
func TestInitCreatesDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	path, err := config.Init(dir)
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Init() created path %q does not exist: %v", path, err)
	}
	if _, err := config.Load(dir); err != nil {
		t.Errorf("Init() created an invalid config: %v", err)
	}
}

// TestInitPreservesExistingConfig verifies that Init returns an error and
// leaves the existing .portree.toml unchanged when one already exists.
func TestInitPreservesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	original := []byte("# original content")
	if err := os.WriteFile(filepath.Join(dir, ".portree.toml"), original, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := config.Init(dir)
	if err == nil {
		t.Fatal("Init() expected error when .portree.toml already exists, got nil")
	}
	content, readErr := os.ReadFile(filepath.Join(dir, ".portree.toml"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != string(original) {
		t.Error("Init() modified existing .portree.toml; it should be preserved unchanged")
	}
}
