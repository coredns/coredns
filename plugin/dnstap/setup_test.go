package dnstap

import (
	"testing"

	"github.com/coredns/caddy"
)

func TestConfig(t *testing.T) {
	tests := []struct {
		file  string
		path  string
		full  bool
		proto string
		fail  bool
	}{
		{"dnstap dnstap.sock full", "dnstap.sock", true, "unix", false},
		{"dnstap unix://dnstap.sock", "dnstap.sock", false, "unix", false},
		{"dnstap tcp://127.0.0.1:6000", "127.0.0.1:6000", false, "tcp", false},
		{"dnstap", "fail", false, "tcp", true},
	}
	for _, c := range tests {
		cad := caddy.NewTestController("dns", c.file)
		conf, err := parseConfig(cad)
		if c.fail {
			if err == nil {
				t.Errorf("%s: %s", c.file, err)
			}
		} else if err != nil || conf.target != c.path || conf.full != c.full || conf.proto != c.proto {
			t.Errorf("Expected: %+v\nhave: %+v\nerror: %s", c, conf, err)
		}
	}
}
