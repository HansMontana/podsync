package quality

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGoFilesStayBelowFiveHundredLines(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(testFile), "../.."))

	var violations []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := bytes.Count(data, []byte{'\n'})
		if len(data) > 0 && !bytes.HasSuffix(data, []byte{'\n'}) {
			lines++
		}
		if lines >= 500 {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			violations = append(violations, fmt.Sprintf("%s (%d lines)", relative, lines))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repository: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("Go files must stay below 500 lines:\n%s", strings.Join(violations, "\n"))
	}
}
