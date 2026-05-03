package urlrouting_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNoEtcHostsModification asserts the product-level NEVER rule:
// portree must not modify /etc/hosts. The rule is a deliberate architectural
// choice — portree relies on RFC 6761 *.localhost resolution (per ADR-003)
// rather than touching /etc/hosts, which would require elevated privileges
// and persistent system state.
//
// The test scans every Go source file under cmd/ and internal/ for any
// occurrence of the string "/etc/hosts". A new caller path that referenced
// /etc/hosts would be flagged here long before a release ever shipped.
func TestNoEtcHostsModification(t *testing.T) {
	root := projectRoot(t)
	scanDirs := []string{"cmd", "internal", "main.go"}

	var offenders []string
	for _, sub := range scanDirs {
		path := filepath.Join(root, sub)
		_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			if strings.Contains(string(data), "/etc/hosts") {
				rel, _ := filepath.Rel(root, p)
				offenders = append(offenders, rel)
			}
			return nil
		})
	}

	if len(offenders) > 0 {
		t.Errorf("portree must never modify /etc/hosts; found references in: %v", offenders)
	}
}

// projectRoot walks up from this test file's directory until it finds go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from test file")
		}
		dir = parent
	}
}
