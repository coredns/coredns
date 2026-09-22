package errors

import (
	"bytes"
	"fmt"
	golog "log"
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

func TestErrorsParse(t *testing.T) {
	tests := []struct {
		inputErrorsRules string
		shouldErr        bool
		optCount         int
		stacktrace       bool
	}{
		{`errors`, false, 0, false},
		{`errors stdout`, false, 0, false},
		{`errors errors.txt`, true, 0, false},
		{`errors visible`, true, 0, false},
		{`errors { log visible }`, true, 0, false},
		{`errors
		  errors `, true, 0, false},
		{`errors a b`, true, 0, false},

		{`errors {
		    consolidate
		  }`, true, 0, false},
		{`errors {
		    consolidate 1m
		  }`, true, 0, false},
		{`errors {
		    consolidate 1m .* extra
		  }`, true, 0, false},
		{`errors {
		    consolidate abc .*
		  }`, true, 0, false},
		{`errors {
		    consolidate 1 .*
		  }`, true, 0, false},
		{`errors {
		    consolidate 1m ())
		  }`, true, 0, false},
		{`errors {
            stacktrace
		  }`, false, 0, true},
		{`errors {
            stacktrace
		    consolidate 1m ^exact$
		  }`, false, 1, true},
		{`errors {
		    consolidate 1m ^exact$
		  }`, false, 1, false},
		{`errors {
		    consolidate 1m error
		  }`, false, 1, false},
		{`errors {
		    consolidate 1m "format error"
		  }`, false, 1, false},
		{`errors {
		    consolidate 1m error1
		    consolidate 5s error2
		  }`, false, 2, false},
		{`errors {
		    consolidate 1m error show_first
		  }`, false, 1, false},
		{`errors {
		    consolidate 1m error warning show_first
		  }`, false, 1, false},
		{`errors {
		    consolidate 1m error invalid_option
		  }`, true, 0, false},
		{`errors {
		    consolidate 1m error warning error
		  }`, true, 0, false},
		{`errors {
		    consolidate 1m error info debug
		  }`, true, 0, false},
		{`errors {
		    consolidate 1m error show_first warning
		  }`, true, 0, false},
		{`errors {
		    consolidate 1m error show_first info
		  }`, true, 0, false},
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
		if dnsserver.GetConfig(c).Stacktrace != test.stacktrace {
			t.Errorf("Test %d: stacktrace, expected %t, got %t",
				i, test.stacktrace, dnsserver.GetConfig(c).Stacktrace)
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

func TestShowFirstOption(t *testing.T) {
	tests := []struct {
		name             string
		inputErrorsRules string
		wantShowFirst    bool
		wantLogLevel     string
	}{
		{
			name: "show_first without log level",
			inputErrorsRules: `errors {
		        consolidate 1m .* show_first
		    }`,
			wantShowFirst: true,
			wantLogLevel:  "[ERROR]", // default
		},
		{
			name: "show_first with warning log level",
			inputErrorsRules: `errors {
		        consolidate 1m .* warning show_first
		    }`,
			wantShowFirst: true,
			wantLogLevel:  "[WARNING]",
		},
		{
			name: "show_first with error log level",
			inputErrorsRules: `errors {
		        consolidate 1m .* error show_first
		    }`,
			wantShowFirst: true,
			wantLogLevel:  "[ERROR]",
		},
		{
			name: "no show_first",
			inputErrorsRules: `errors {
		        consolidate 1m .*
		    }`,
			wantShowFirst: false,
			wantLogLevel:  "[ERROR]",
		},
		{
			name: "no show_first with log level",
			inputErrorsRules: `errors {
		        consolidate 1m .* info
		    }`,
			wantShowFirst: false,
			wantLogLevel:  "[INFO]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.Buffer{}
			golog.SetOutput(&buf)
			clog.D.Set()

			c := caddy.NewTestController("dns", tt.inputErrorsRules)
			h, err := errorsParse(c)

			if err != nil {
				t.Errorf("Failed to parse: %v", err)
				return
			}

			if len(h.patterns) != 1 {
				t.Errorf("Expected 1 pattern, got %d", len(h.patterns))
				return
			}

			if h.patterns[0].showFirst != tt.wantShowFirst {
				t.Errorf("Expected showFirst=%v, got %v", tt.wantShowFirst, h.patterns[0].showFirst)
			}

			// Test log level
			l := h.patterns[0].logCallback
			l("test log")

			if log := buf.String(); !strings.Contains(log, tt.wantLogLevel) {
				t.Errorf("Expected log level %q, but got %q", tt.wantLogLevel, log)
			}
		})
	}
}

func TestErrorsParseLargeRegex(t *testing.T) {
	largeRegex := strings.Repeat("a", maxRegexpLen+1)
	config := fmt.Sprintf(`errors {
		consolidate 1m %s
	}`, largeRegex)

	c := caddy.NewTestController("dns", config)
	_, err := errorsParse(c)
	if err == nil {
		t.Fatal("Expected error for large regex, got nil")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("Expected 'too long' error, got: %v", err)
	}
}
