package acl

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/caddyserver/caddy"
)

var (
	setupTestFiles = map[string]string{
		"acl-setup-test-1.txt": `10.218.128.0/24
35.39.53.223/32
43.105.127.35/18`,
	}
)

func envSetup(files map[string]string) {
	for filename, content := range files {
		err := ioutil.WriteFile(filename, []byte(content), 0600)
		if err != nil {
			panic(err)
		}
	}
}

func envCleanup(files map[string]string) {
	for filename := range files {
		err := os.Remove(filename)
		if err != nil {
			panic(err)
		}
	}
}

func TestSetup(t *testing.T) {
	envSetup(setupTestFiles)
	defer envCleanup(setupTestFiles)

	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			"Blacklist 1",
			`acl {
				block type A net 192.168.0.0/16
			}`,
			false,
		},
		{
			"Blacklist 2",
			`acl {
				block type * net 192.168.0.0/16
			}`,
			false,
		},
		{
			"Blacklist 3",
			`acl {
				block type A net *
			}`,
			false,
		},
		{
			"Blacklist 4",
			`acl {
				allow type * net 192.168.1.0/24
				block type * net 192.168.0.0/16
			}`,
			false,
		},
		{
			"Whitelist 1",
			`acl {
				allow type * net 192.168.0.0/16
				block type * net *
			}`,
			false,
		},
		{
			"fine-grained 1",
			`acl a.example.org {
				block type * net 192.168.1.0/24
			}`,
			false,
		},
		{
			"fine-grained 2",
			`acl a.example.org {
				block type * net 192.168.1.0/24
			}
			acl b.example.org {
				block type * net 192.168.2.0/24
			}`,
			false,
		},
		{
			"multiple-networks 1",
			`acl example.org {
				block type * net 192.168.1.0/24 192.168.3.0/24
			}`,
			false,
		},
		{
			"multiple-networks 2",
			`acl example.org {
				block type * net 192.168.3.0/24
			}`,
			false,
		},
		{
			"Local file 1",
			`acl {
				block type A file acl-setup-test-1.txt
			}`,
			false,
		},
		{
			"Missing argument 1",
			`acl {
				block A net 192.168.0.0/16
			}`,
			true,
		},
		{
			"Missing argument 2",
			`acl {
				block type net 192.168.0.0/16
			}`,
			true,
		},
		{
			"Illegal argument 1",
			`acl {
				block type ABC net 192.168.0.0/16
			}`,
			true,
		},
		{
			"Illegal argument 2",
			`acl {
				blck type A net 192.168.0.0/16
			}`,
			true,
		},
		{
			"Illegal argument 3",
			`acl {
				block type A net 192.168.0/16
			}`,
			true,
		},
		{
			"Illegal argument 4",
			`acl {
				block type A net 192.168.0.0/33
			}`,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctr := caddy.NewTestController("dns", tt.config)
			if err := setup(ctr); (err != nil) != tt.wantErr {
				t.Errorf("Error: setup() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStripComment(t *testing.T) {
	type args struct {
		line string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			"No change 1",
			args{`hello, world`},
			`hello, world`,
		},
		{
			"No comment 1",
			args{`  hello, world   `},
			`hello, world`,
		},
		{
			"Remove tailing comment 1",
			args{`hello, world# comments`},
			`hello, world`,
		},
		{
			"Remove tailing comment 2",
			args{`  hello, world   # comments`},
			`hello, world`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripComment(tt.args.line); got != tt.want {
				t.Errorf("Error: stripComment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	type args struct {
		rawNet string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			"Network range 1",
			args{"10.218.10.8/24"},
			"10.218.10.8/24",
		},
		{
			"IP address 1",
			args{"10.218.10.8"},
			"10.218.10.8/32",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalize(tt.args.rawNet); got != tt.want {
				t.Errorf("Error: normalize() = %v, want %v", got, tt.want)
			}
		})
	}
}
