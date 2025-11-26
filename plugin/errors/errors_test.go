package errors

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	golog "log"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestErrors(t *testing.T) {
	buf := bytes.Buffer{}
	golog.SetOutput(&buf)
	em := errorHandler{}

	testErr := errors.New("test error")
	tests := []struct {
		next         plugin.Handler
		expectedCode int
		expectedLog  string
		expectedErr  error
	}{
		{
			next:         genErrorHandler(dns.RcodeSuccess, nil),
			expectedCode: dns.RcodeSuccess,
			expectedLog:  "",
			expectedErr:  nil,
		},
		{
			next:         genErrorHandler(dns.RcodeNotAuth, testErr),
			expectedCode: dns.RcodeNotAuth,
			expectedLog:  fmt.Sprintf("%d %s: %v\n", dns.RcodeNotAuth, "example.org. A", testErr),
			expectedErr:  testErr,
		},
	}

	ctx := context.TODO()
	req := new(dns.Msg)
	req.SetQuestion("example.org.", dns.TypeA)

	for i, tc := range tests {
		em.Next = tc.next
		buf.Reset()
		rec := dnstest.NewRecorder(&test.ResponseWriter{})
		code, err := em.ServeDNS(ctx, rec, req)

		if err != tc.expectedErr {
			t.Errorf("Test %d: Expected error %v, but got %v",
				i, tc.expectedErr, err)
		}
		if code != tc.expectedCode {
			t.Errorf("Test %d: Expected status code %d, but got %d",
				i, tc.expectedCode, code)
		}
		if log := buf.String(); !strings.Contains(log, tc.expectedLog) {
			t.Errorf("Test %d: Expected log %q, but got %q",
				i, tc.expectedLog, log)
		}
	}
}

func TestLogPattern(t *testing.T) {
	type args struct {
		logCallback func(format string, v ...any)
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "error log",
			args: args{logCallback: log.Errorf},
			want: "[ERROR] plugin/errors: 4 errors like '^error.*!$' occurred in last 2s",
		},
		{
			name: "warn log",
			args: args{logCallback: log.Warningf},
			want: "[WARNING] plugin/errors: 4 errors like '^error.*!$' occurred in last 2s",
		},
		{
			name: "info log",
			args: args{logCallback: log.Infof},
			want: "[INFO] plugin/errors: 4 errors like '^error.*!$' occurred in last 2s",
		},
		{
			name: "debug log",
			args: args{logCallback: log.Debugf},
			want: "[DEBUG] plugin/errors: 4 errors like '^error.*!$' occurred in last 2s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.Buffer{}
			clog.D.Set()
			golog.SetOutput(&buf)

			h := &errorHandler{
				patterns: []*pattern{{
					count:       4,
					period:      2 * time.Second,
					pattern:     regexp.MustCompile("^error.*!$"),
					logCallback: tt.args.logCallback,
				}},
			}
			h.logPattern(0)

			if log := buf.String(); !strings.Contains(log, tt.want) {
				t.Errorf("Expected log %q, but got %q", tt.want, log)
			}
		})
	}
}

func TestInc(t *testing.T) {
	h := &errorHandler{
		stopFlag: 1,
		patterns: []*pattern{{
			period:  2 * time.Second,
			pattern: regexp.MustCompile("^error.*!$"),
		}},
	}

	ret := h.consolidateError(0)
	if ret {
		t.Error("Unexpected return value, expected false, actual true")
	}

	h.stopFlag = 0
	ret = h.consolidateError(0)
	if !ret {
		t.Error("Unexpected return value, expected true, actual false")
	}

	expCnt := uint32(1)
	actCnt := atomic.LoadUint32(&h.patterns[0].count)
	if actCnt != expCnt {
		t.Errorf("Unexpected 'count', expected %d, actual %d", expCnt, actCnt)
	}

	t1 := h.patterns[0].timer()
	if t1 == nil {
		t.Error("Unexpected 'timer', expected not nil")
	}

	ret = h.consolidateError(0)
	if !ret {
		t.Error("Unexpected return value, expected true, actual false")
	}

	expCnt = uint32(2)
	actCnt = atomic.LoadUint32(&h.patterns[0].count)
	if actCnt != expCnt {
		t.Errorf("Unexpected 'count', expected %d, actual %d", expCnt, actCnt)
	}

	t2 := h.patterns[0].timer()
	if t2 != t1 {
		t.Error("Unexpected 'timer', expected the same")
	}

	ret = t1.Stop()
	if !ret {
		t.Error("Timer was unexpectedly stopped before")
	}
	ret = t2.Stop()
	if ret {
		t.Error("Timer was unexpectedly not stopped before")
	}
}

func TestStop(t *testing.T) {
	buf := bytes.Buffer{}
	golog.SetOutput(&buf)

	h := &errorHandler{
		patterns: []*pattern{{
			period:      2 * time.Second,
			pattern:     regexp.MustCompile("^error.*!$"),
			logCallback: log.Errorf,
		}},
	}

	h.consolidateError(0)
	h.consolidateError(0)
	h.consolidateError(0)
	expCnt := uint32(3)
	actCnt := atomic.LoadUint32(&h.patterns[0].count)
	if actCnt != expCnt {
		t.Errorf("Unexpected initial 'count', expected %d, actual %d", expCnt, actCnt)
		return
	}

	h.stop()

	expCnt = uint32(0)
	actCnt = atomic.LoadUint32(&h.patterns[0].count)
	if actCnt != expCnt {
		t.Errorf("Unexpected 'count', expected %d, actual %d", expCnt, actCnt)
	}

	expStop := uint32(1)
	actStop := h.stopFlag
	if actStop != expStop {
		t.Errorf("Unexpected 'stop', expected %d, actual %d", expStop, actStop)
	}

	t1 := h.patterns[0].timer()
	if t1 == nil {
		t.Error("Unexpected 'timer', expected not nil")
	} else if t1.Stop() {
		t.Error("Timer was unexpectedly not stopped before")
	}

	expLog := "3 errors like '^error.*!$' occurred in last 2s"
	if log := buf.String(); !strings.Contains(log, expLog) {
		t.Errorf("Expected log %q, but got %q", expLog, log)
	}
}

func TestShowFirst(t *testing.T) {
	tests := []struct {
		name              string
		errorCount        int
		expectSummary     string
		shouldHaveSummary bool
	}{
		{
			name:              "multiple errors",
			errorCount:        3,
			expectSummary:     "3 errors like '^error.*!$' occurred in last 2s",
			shouldHaveSummary: true,
		},
		{
			name:              "single error",
			errorCount:        1,
			expectSummary:     "",
			shouldHaveSummary: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.Buffer{}
			clog.D.Set()
			golog.SetOutput(&buf)

			h := &errorHandler{
				patterns: []*pattern{{
					count:       0,
					period:      2 * time.Second,
					pattern:     regexp.MustCompile("^error.*!$"),
					logCallback: log.Errorf,
					showFirst:   true,
				}},
			}

			// Add errors and verify return values
			for i := range tt.errorCount {
				ret := h.consolidateError(0)
				if i == 0 {
					// First call should return false (showFirst enabled)
					if ret {
						t.Errorf("First consolidateError call: expected false, got true")
					}
					// Simulate ServeDNS logging with pattern's logCallback
					h.patterns[0].logCallback("2 example.org. A: error %d!", i+1)
				} else {
					// Subsequent calls should return true (consolidated)
					if !ret {
						t.Errorf("consolidateError call %d: expected true, got false", i+1)
					}
				}
			}

			// Check count
			expCnt := uint32(tt.errorCount)
			actCnt := atomic.LoadUint32(&h.patterns[0].count)
			if actCnt != expCnt {
				t.Errorf("Unexpected 'count', expected %d, actual %d", expCnt, actCnt)
				return
			}

			// Check that first error was logged
			output1 := buf.String()
			if !strings.Contains(output1, "2 example.org. A: error 1!") {
				t.Errorf("Expected first error to be logged, but got: %q", output1)
			}

			// Clear buffer and trigger log pattern
			buf.Reset()
			h.logPattern(0)

			// Verify summary in logPattern output
			output2 := buf.String()
			if tt.shouldHaveSummary {
				if !strings.Contains(output2, tt.expectSummary) {
					t.Errorf("Expected summary %q not found in logPattern output: %q", tt.expectSummary, output2)
				}
			} else {
				if strings.Contains(output2, "errors like") {
					t.Errorf("Did not expect summary for single error, but got: %q", output2)
				}
			}
		})
	}
}

func genErrorHandler(rcode int, err error) plugin.Handler {
	return plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		return rcode, err
	})
}
