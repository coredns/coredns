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
	buf := bytes.Buffer{}
	golog.SetOutput(&buf)

	h := &errorHandler{
		patterns: []*pattern{{
			count:   4,
			period:  2 * time.Second,
			pattern: regexp.MustCompile("^error.*!$"),
		}},
	}
	h.logPattern(0)

	expLog := "4 errors like '^error.*!$' occurred in last 2s"
	if log := buf.String(); !strings.Contains(log, expLog) {
		t.Errorf("Expected log %q, but got %q", expLog, log)
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

	ret := h.inc(0)
	if ret {
		t.Error("Unexpected return value, expected false, actual true")
	}

	h.stopFlag = 0
	ret = h.inc(0)
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

	ret = h.inc(0)
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
			period:  2 * time.Second,
			pattern: regexp.MustCompile("^error.*!$"),
		}},
	}

	h.inc(0)
	h.inc(0)
	h.inc(0)
	expCnt := uint32(3)
	actCnt := atomic.LoadUint32(&h.patterns[0].count)
	if actCnt != expCnt {
		t.Fatalf("Unexpected initial 'count', expected %d, actual %d", expCnt, actCnt)
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

func genErrorHandler(rcode int, err error) plugin.Handler {
	return plugin.HandlerFunc(func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
		return rcode, err
	})
}
