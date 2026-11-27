package readfile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

func TestRead(t *testing.T) {
	content := []byte("example.org. 10\n")
	path := writeFile(t, "weights", content)

	got, err := Read(path, DefaultLimit)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("Read = %q, want %q", got, content)
	}
}

func TestReadAtLimit(t *testing.T) {
	content := bytes.Repeat([]byte("a"), 16)
	path := writeFile(t, "at-limit", content)

	got, err := Read(path, int64(len(content)))
	if err != nil {
		t.Fatalf("Read at the limit: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("Read = %q, want %q", got, content)
	}
}

func TestReadTooLarge(t *testing.T) {
	path := writeFile(t, "too-large", bytes.Repeat([]byte("a"), 17))

	_, err := Read(path, 16)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Read = %v, want ErrTooLarge", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the file %q", err, path)
	}
}

func TestReadMissing(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "missing"), DefaultLimit)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Read = %v, want a not-exist error", err)
	}
}
