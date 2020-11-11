package test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil/promlint"
	dto "github.com/prometheus/client_model/go"
)

func TestMetricNaming(t *testing.T) {

	walker := validMetricWalker{}
	err := filepath.Walk("..", walker.walk)

	if err != nil {
		t.Fatal(err)
	}

	if len(walker.Metrics) > 0 {
		l := promlint.NewWithMetricFamilies(walker.Metrics)
		problems, err := l.Lint()
		if err != nil {
			t.Fatalf("Link found error: %s", err)
		}

		if len(problems) > 0 {
			t.Fatalf("A slice of Problems indicating any issues found in the metrics stream: %s", problems)
		}
	}
}

type validMetricWalker struct {
	Metrics []*dto.MetricFamily
}

func (w *validMetricWalker) walk(path string, info os.FileInfo, _ error) error {
	// only for regular files, not starting with a . and those that are go files.
	if !info.Mode().IsRegular() {
		return nil
	}
	// Is it appropriate to compare the file name equals metrics.go directly？
	if strings.HasPrefix(path, "../.") {
		return nil
	}
	if strings.HasSuffix(path, "_test.go") {
		return nil
	}
	if !strings.HasSuffix(path, ".go") {
		return nil
	}

	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, path, nil, parser.AllErrors)
	if err != nil {
		return err
	}
	l := &metric{}
	ast.Walk(l, f)
	if l.Metric != nil {
		w.Metrics = append(w.Metrics, l.Metric)
	}
	return nil
}

type metric struct {
	Metric *dto.MetricFamily
}

func (l *metric) Visit(n ast.Node) ast.Visitor {
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
	if id.Name != "prometheus" { //prometheus
		return l
	}
	var metricsType dto.MetricType
	switch se.Sel.Name {
	case "NewCounterVec", "NewCounter":
		metricsType = dto.MetricType_COUNTER
	case "NewGaugeVec", "NewGauge":
		metricsType = dto.MetricType_GAUGE
	case "NewHistogramVec", "NewHistogram":
		metricsType = dto.MetricType_HISTOGRAM
	case "NewSummaryVec", "NewSummary":
		metricsType = dto.MetricType_SUMMARY
	default:
		return l
	}
	// Check first arg, that should have basic lit with capital
	if len(ce.Args) < 1 {
		return l
	}
	bl, ok := ce.Args[0].(*ast.CompositeLit)
	if !ok {
		return l
	}

	// parse Namespace Subsystem Name Help
	var subsystem, name, help string
	for _, elt := range bl.Elts {
		expr, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		object, ok := expr.Key.(*ast.Ident)
		if !ok {
			continue
		}
		value, ok := expr.Value.(*ast.BasicLit)
		if !ok {
			continue
		}

		// remove quotes
		stringLiteral, err := strconv.Unquote(value.Value)
		if err != nil {
			return l
		}

		switch object.Name {
		case "Subsystem":
			subsystem = stringLiteral
		case "Name":
			name = stringLiteral
		case "Help":
			help = stringLiteral
		}
	}

	// validate metrics field
	if len(name) == 0 || len(help) == 0 {
		return l
	}

	metricName := prometheus.BuildFQName(plugin.Namespace, subsystem, name)
	l.Metric = &dto.MetricFamily{
		Name: &metricName,
		Help: &help,
		Type: &metricsType,
	}
	return l
}
