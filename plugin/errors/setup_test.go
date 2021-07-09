package errors

import (
	"bytes"
	golog "log"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

func TestErrorsParse(t *testing.T) {
	tests := []struct {
		inputErrorsRules string
		shouldErr        bool
		optCount         int
	}{
		{`errors`, false, 0},
		{`errors stdout`, false, 0},
		{`errors errors.txt`, true, 0},
		{`errors visible`, true, 0},
		{`errors { log visible }`, true, 0},
		{`errors
		  errors `, true, 0},
		{`errors a b`, true, 0},

		{`errors {
		    consolidate
		  }`, true, 0},
		{`errors {
		    consolidate 1m
		  }`, true, 0},
		{`errors {
		    consolidate 1m .* extra
		  }`, true, 0},
		{`errors {
		    consolidate abc .*
		  }`, true, 0},
		{`errors {
		    consolidate 1 .*
		  }`, true, 0},
		{`errors {
		    consolidate 1m ())
		  }`, true, 0},
		{`errors {
		    consolidate 1m ^exact$
		  }`, false, 1},
		{`errors {
		    consolidate 1m error
		  }`, false, 1},
		{`errors {
		    consolidate 1m "format error"
		  }`, false, 1},
		{`errors {
		    consolidate 1m error1
		    consolidate 5s error2
		  }`, false, 2},
	}
	for i, test := range tests {
		c := caddy.NewTestController("dns", test.inputErrorsRules)
		h, err := errorsParse(c)

		if err == nil && test.shouldErr {
			t.Errorf("Test %d didn't error, but it should have", i)
		} else if err != nil && !test.shouldErr {
			t.Errorf("Test %d errored, but it shouldn't have; got '%v'", i, err)
		} else if h != nil && len(h.patterns) != test.optCount {
			t.Errorf("Test %d: pattern count mismatch, expected %d, got %d",
				i, test.optCount, len(h.patterns))
		}
	}
}

func TestProperLogCallbackIsSet(t *testing.T) {
	tests := []struct {
		name             string
		inputErrorsRules string
		wantLogLevel     string
	}{
		{
			name: "warning is parsed properly",
			inputErrorsRules: `errors {
		        consolidate 1m .* warning
		    }`,
			wantLogLevel: "[WARNING]",
		},
		{
			name: "error is parsed properly",
			inputErrorsRules: `errors {
		        consolidate 1m .* error
		    }`,
			wantLogLevel: "[ERROR]",
		},
		{
			name: "info is parsed properly",
			inputErrorsRules: `errors {
		        consolidate 1m .* info
		    }`,
			wantLogLevel: "[INFO]",
		},
		{
			name: "debug is parsed properly",
			inputErrorsRules: `errors {
		        consolidate 1m .* debug
		    }`,
			wantLogLevel: "[DEBUG]",
		},
		{
			name: "default is error",
			inputErrorsRules: `errors {
		        consolidate 1m .*
		    }`,
			wantLogLevel: "[ERROR]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.Buffer{}
			golog.SetOutput(&buf)
			clog.D.Set()

			c := caddy.NewTestController("dns", tt.inputErrorsRules)
			h, _ := errorsParse(c)

			l := h.patterns[0].logCallback
			l("some error happened")

			if log := buf.String(); !strings.Contains(log, tt.wantLogLevel) {
				t.Errorf("Expected log %q, but got %q", tt.wantLogLevel, log)
			}
		})
	}
}
