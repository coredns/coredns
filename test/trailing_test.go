package test

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

func TestTrailingWhitespace(t *testing.T) {
	err := filepath.Walk("..", hasTrailingWhitespace)
	if err != nil {
		t.Fatal(err)
	}
}

func hasTrailingWhitespace(path string, info os.FileInfo, _ error) error {
	// Only handle regular files, skip files that are executable and skip file in the
	// root that start with a .
	if !info.Mode().IsRegular() {
		return nil
	}
	if info.Mode().Perm()&0111 != 0 {
		return nil
	}
	if strings.HasPrefix(path, "../.") {
		return nil
	}

	println("looking at", path)
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		text := scanner.Text()
		trimmed := strings.TrimRightFunc(text, unicode.IsSpace)
		if len(text) != len(trimmed) {
			return fmt.Errorf("file %q has trailing whitespace, text: %q", path, text)
		}
	}

	return scanner.Err()
}
