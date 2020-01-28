package test

// These tests check for meta level items, like trailing whitespace, correct file naming etc.

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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

func TestFileNameHyphen(t *testing.T) {
	err := filepath.Walk("..", hasHyphen)
	if err != nil {
		t.Fatal(err)
	}
}

func hasHyphen(path string, info os.FileInfo, _ error) error {
	// only for regular files, not starting with a . and those that are go files.
	if !info.Mode().IsRegular() {
		return nil
	}
	if strings.HasPrefix(path, "../.") {
		return nil
	}
	if filepath.Ext(path) != ".go" {
		return nil
	}

	if strings.Index(path, "-") > 0 {
		return fmt.Errorf("file %q has a hyphen, please use underscores in file names", path)
	}

	return nil
}

// Test if error messages start with an upper case.
func TestLowercaseLog(t *testing.T) {
	err := filepath.Walk("..", hasLowercase)
	if err != nil {
		t.Fatal(err)
	}
}

func hasLowercase(path string, info os.FileInfo, _ error) error {
	// only for regular files, not starting with a . and those that are go files.
	if !info.Mode().IsRegular() {
		return nil
	}
	if strings.HasPrefix(path, "../.") {
		return nil
	}
	if !strings.HasSuffix(path, "_test.go") {
		return nil
	}

	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, path, nil, parser.AllErrors)
	if err != nil {
		return err
	}
	l := &logfmt{}
	ast.Walk(l, f)
	if l.err != nil {
		return l.err
	}
	return nil
}

type logfmt struct {
	err error
}

func (l logfmt) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return nil
	}
	ce, ok := n.(*ast.CallExpr)
	if !ok {
		return l
	}
	se, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok {
		return l
	}
	id, ok := se.X.(*ast.Ident)
	if !ok {
		return l
	}
	if id.Name != "t" { //t *testing.T
		return l
	}

	switch se.Sel.Name {
	case "Errorf":
	case "Logf":
	case "Log":
	case "Fatalf":
	case "Fatal":
	default:
		return l
	}
	// Check first arg, that should have basic lit with capital
	if len(ce.Args) < 1 {
		return l
	}
	bl, ok := ce.Args[0].(*ast.BasicLit)
	if !ok {
		return l
	}
	if bl.Kind != token.STRING {
		return l
	}
	if strings.HasPrefix(bl.Value, "\"%s") || strings.HasPrefix(bl.Value, "\"%d") {
		return l
	}
	if strings.HasPrefix(bl.Value, "\"%v") || strings.HasPrefix(bl.Value, "\"%+v") {
		return l
	}
	for i, u := range bl.Value {
		// disregard "
		if i == 1 && !unicode.IsUpper(u) {
			l.err = fmt.Errorf("test error message %s doesn't start with an uppercase", bl.Value)
			return nil
		}
		if i == 1 {
			break
		}
	}
	return l
}

func TestImportTesting(t *testing.T) {
	err := filepath.Walk("..", hasImportTesting)
	if err != nil {
		t.Fatal(err)
	}
}

func hasImportTesting(path string, info os.FileInfo, _ error) error {
	// only for regular files, not starting with a . and those that are go files.
	if !info.Mode().IsRegular() {
		return nil
	}
	if strings.HasPrefix(path, "../.") {
		return nil
	}
	if strings.HasSuffix(path, "_test.go") {
		return nil
	}

	if strings.HasSuffix(path, ".go") {
		fs := token.NewFileSet()
		f, err := parser.ParseFile(fs, path, nil, parser.AllErrors)
		if err != nil {
			return err
		}
		for _, im := range f.Imports {
			if im.Path.Value == `"testing"` {
				return fmt.Errorf("file %q is importing %q", path, "testing")
			}
		}
	}
	return nil
}
