package test

// These tests check for meta level items, like trailing whitespace, correct file naming etc.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

func TestFileNameHyphen(t *testing.T) {
	walker := hasHyphenWalker{}
	err := filepath.Walk("..", walker.walk)

	if err != nil {
		t.Fatal(err)
	}

	if len(walker.Errors) > 0 {
		for _, err = range walker.Errors {
			t.Error(err)
		}
	}
}

type hasHyphenWalker struct {
	Errors []error
}

func (w *hasHyphenWalker) walk(path string, info os.FileInfo, _ error) error {
	// only for regular files, not starting with a . and those that are go files.
	if !info.Mode().IsRegular() {
		return nil
	}
	if strings.HasPrefix(path, "../.") {
		return nil
	}
	if strings.Contains(path, "/vendor") {
		return nil
	}
	if filepath.Ext(path) != ".go" {
		return nil
	}

	if strings.Index(path, "-") > 0 {
		absPath, _ := filepath.Abs(path)
		w.Errors = append(w.Errors, fmt.Errorf("file %q has a hyphen, please use underscores in file names", absPath))
	}

	return nil
}

// Test if error messages start with an upper case.
func TestLowercaseLog(t *testing.T) {
	walker := hasLowercaseWalker{}
	err := filepath.Walk("..", walker.walk)

	if err != nil {
		t.Fatal(err)
	}

	if len(walker.Errors) > 0 {
		for _, err = range walker.Errors {
			t.Error(err)
		}
	}
}

type hasLowercaseWalker struct {
	Errors []error
}

func (w *hasLowercaseWalker) walk(path string, info os.FileInfo, _ error) error {
	// only for regular files, not starting with a . and those that are go files.
	if !info.Mode().IsRegular() {
		return nil
	}
	if strings.HasPrefix(path, "../.") {
		return nil
	}
	if strings.Contains(path, "/vendor") {
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
		w.Errors = append(w.Errors, l.err)
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
	walker := hasLowercaseWalker{}
	err := filepath.Walk("..", walker.walk)

	if err != nil {
		t.Fatal(err)
	}

	if len(walker.Errors) > 0 {
		for _, err = range walker.Errors {
			t.Error(err)
		}
	}
}

type hasImportTestingWalker struct {
	Errors []error
}

func (w *hasImportTestingWalker) walk(path string, info os.FileInfo, _ error) error {
	// only for regular files, not starting with a . and those that are go files.
	if !info.Mode().IsRegular() {
		return nil
	}
	if strings.HasPrefix(path, "../.") {
		return nil
	}
	if strings.Contains(path, "/vendor") {
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
				absPath, _ := filepath.Abs(path)
				w.Errors = append(w.Errors, fmt.Errorf("file %q is importing %q", absPath, "testing"))
			}
		}
	}
	return nil
}

func TestImportOrdering(t *testing.T) {
	walker := testImportOrderingWalker{}
	err := filepath.Walk("..", walker.walk)

	if err != nil {
		t.Fatal(err)
	}

	if len(walker.Errors) > 0 {
		for _, err = range walker.Errors {
			t.Error(err)
		}
	}
}

type testImportOrderingWalker struct {
	Errors []error
}

func (w *testImportOrderingWalker) walk(path string, info os.FileInfo, _ error) error {
	if !info.Mode().IsRegular() {
		return nil
	}
	if strings.HasPrefix(path, "../.") {
		return nil
	}
	if strings.Contains(path, "/vendor") {
		return nil
	}
	if filepath.Ext(path) != ".go" {
		return nil
	}

	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, path, nil, parser.AllErrors)
	if err != nil {
		return err
	}
	if len(f.Imports) == 0 {
		return nil
	}

	// 3 blocks total, if
	// 3 blocks: std + coredns + 3rd party
	// 2 blocks: std + coredns, std + 3rd party, coredns + 3rd party
	// 1 block: std, coredns, 3rd party
	// first entry in a block specifies the type (std, coredns, 3rd party)
	// we want: std, coredns, 3rd party
	// more than 3 blocks as an error
	blocks := [3][]*ast.ImportSpec{}
	prevpos := 0
	bl := 0
	for _, im := range f.Imports {
		line := fs.Position(im.Path.Pos()).Line
		if line-prevpos > 1 && prevpos > 0 {
			bl++
		}
		if bl > 2 {
			absPath, _ := filepath.Abs(path)
			w.Errors = append(w.Errors, fmt.Errorf("more than %d import blocks in %q", bl, absPath))
		}
		blocks[bl] = append(blocks[bl], im)
		prevpos = line
	}
	// if it:
	// contains strings github.com/coredns -> coredns
	// contains dots -> 3rd
	// no dots -> std
	ip := [3]string{} // type per block, just string, either std, coredns, 3rd
	for i := 0; i <= bl; i++ {
		ip[i] = importtype(blocks[i][0].Path.Value)
	}

	// Ok, now that we have the type, let's see if all members adhere to it.
	// After that we check if the are in the right order.
	for i := 0; i <= bl; i++ {
		for _, p := range blocks[i] {
			t := importtype(p.Path.Value)
			if t != ip[i] {
				absPath, _ := filepath.Abs(path)
				w.Errors = append(w.Errors, fmt.Errorf("import path for %s is not of the same type %q in %q", p.Path.Value, ip[i], absPath))
			}
		}
	}

	// check order
	switch bl {
	case 0:
		// we don't care
	case 1:
		if ip[0] == "std" && ip[1] == "coredns" {
			break // OK
		}
		if ip[0] == "std" && ip[1] == "3rd" {
			break // OK
		}
		if ip[0] == "coredns" && ip[1] == "3rd" {
			break // OK
		}
		absPath, _ := filepath.Abs(path)
		w.Errors = append(w.Errors, fmt.Errorf("import path in %q are not in the right order (std -> coredns -> 3rd)", absPath))
	case 2:
		if ip[0] == "std" && ip[1] == "coredns" && ip[2] == "3rd" {
			break // OK
		}
		absPath, _ := filepath.Abs(path)
		w.Errors = append(w.Errors, fmt.Errorf("import path in %q are not in the right order (std -> coredns -> 3rd)", absPath))
	}

	return nil
}

func importtype(s string) string {
	if strings.Contains(s, "github.com/coredns") {
		return "coredns"
	}
	if strings.Contains(s, ".") {
		return "3rd"
	}
	return "std"
}

// TestMetricNaming tests the imports path used for metrics. It depends on faillint to be installed: go install github.com/fatih/faillint
func TestPrometheusImports(t *testing.T) {
	if _, err := exec.LookPath("faillint"); err != nil {
		fmt.Fprintf(os.Stderr, "Not executing TestPrometheusImports: faillint not found\n")
		return
	}

	// make this multiline?
	p := `github.com/prometheus/client_golang/prometheus.{NewCounter,NewCounterVec,NewCounterVec,NewGauge,NewGaugeVec,NewGaugeFunc,NewHistorgram,NewHistogramVec,NewSummary,NewSummaryVec}=github.com/prometheus/client_golang/prometheus/promauto.{NewCounter,NewCounterVec,NewCounterVec,NewGauge,NewGaugeVec,NewGaugeFunc,NewHistorgram,NewHistogramVec,NewSummary,NewSummaryVec}`

	cmd := exec.Command("faillint", "-paths", p, "./...")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed: %s\n%s", err, out)
	}
}
