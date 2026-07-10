package bleephub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCryptoRandomReadsAreChecked(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch path {
			case ".git", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if path == "random_test.go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, "rand.Read(") {
				continue
			}
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "if _, err := rand.Read(") {
				continue
			}
			t.Errorf("%s:%d checks crypto/rand.Read incorrectly: %s", path, i+1, trimmed)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEntropyHelpersReturnErrors(t *testing.T) {
	if _, err := newHostedComputeIDFromReader(strings.NewReader("")); err == nil || !strings.Contains(err.Error(), "hosted compute resource id") {
		t.Fatalf("newHostedComputeIDFromReader error = %v", err)
	}
	if _, err := randomRunnerTokenFromReader(strings.NewReader("")); err == nil || !strings.Contains(err.Error(), "runner token") {
		t.Fatalf("randomRunnerTokenFromReader error = %v", err)
	}
	if _, err := newCacheDownloadTokenFromReader(strings.NewReader("")); err == nil || !strings.Contains(err.Error(), "cache download token") {
		t.Fatalf("newCacheDownloadTokenFromReader error = %v", err)
	}
	if _, err := newFineGrainedPATTokenFromReader(strings.NewReader("")); err == nil || !strings.Contains(err.Error(), "fine-grained personal access token") {
		t.Fatalf("newFineGrainedPATTokenFromReader error = %v", err)
	}
}
