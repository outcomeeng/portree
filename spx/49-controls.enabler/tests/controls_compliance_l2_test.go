package controls_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSuccessCommandExitsZero verifies that commands completing without error
// exit with status 0 — required for script and agent compatibility.
func TestSuccessCommandExitsZero(t *testing.T) {
	dir := setupTestRepo(t)
	_, _, exitCode := runPortree(t, dir, "doctor")
	if exitCode != 0 {
		t.Errorf("portree doctor on valid config exited %d, want 0", exitCode)
	}
}

// TestErrorCommandExitsNonZero verifies that commands encountering an error
// exit with a non-zero status — required for script and agent compatibility.
func TestErrorCommandExitsNonZero(t *testing.T) {
	cases := []struct {
		name string
		args []string
		prep func(dir string)
	}{
		{
			name: "ls without config",
			args: []string{"ls"},
			prep: func(dir string) {
				_ = os.Remove(filepath.Join(dir, ".portree.toml"))
			},
		},
		{
			name: "doctor with invalid config",
			args: []string{"doctor"},
			prep: func(dir string) {
				bad := "[services.web]\ncommand = \"\"\nport_range = { min = 19900, max = 19999 }\nproxy_port = 19000\n"
				_ = os.WriteFile(filepath.Join(dir, ".portree.toml"), []byte(bad), 0600)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupTestRepo(t)
			tc.prep(dir)
			_, _, exitCode := runPortree(t, dir, tc.args...)
			if exitCode == 0 {
				t.Errorf("portree %v expected non-zero exit on error, got 0", tc.args)
			}
		})
	}
}
