package resolve

import (
	"reflect"
	"testing"

	"github.com/mholt/caddy"
)

func TestSetup(t *testing.T) {
	c := caddy.NewTestController("dns", `resolve . {
		blah
	}`)
	err := setup(c)
	if err == nil {
		t.Errorf("Expected setup to fail on broken confiuration, got no error")
	}
	c = caddy.NewTestController("dns", `resolve .`)
	err = setup(c)
	if err != nil {
		t.Errorf("Expected no errors, got: %v", err)
	}
}

func TestResolveParse(t *testing.T) {
	type args struct {
		c *caddy.Controller
	}
	tests := []struct {
		name    string
		args    args
		want    Resolve
		wantErr bool
	}{
		{
			name:    "implicit default",
			args:    args{c: caddy.NewTestController("dns", `resolve .`)},
			want:    Resolve{Next: nil, Zones: []string{"."}, DoCNAME: true, DoSRV: true},
			wantErr: false,
		},
		{
			name: "explicit default",
			args: args{c: caddy.NewTestController("dns", `resolve . {
	cname
	srv
}`)},
			want:    Resolve{Next: nil, Zones: []string{"."}, DoCNAME: true, DoSRV: true},
			wantErr: false,
		},
		{
			name: "no cname",
			args: args{c: caddy.NewTestController("dns", `resolve . {
	no cname
}`)},
			want:    Resolve{Next: nil, Zones: []string{"."}, DoCNAME: false, DoSRV: true},
			wantErr: false,
		},
		{
			name: "no srv",
			args: args{c: caddy.NewTestController("dns", `resolve . {
	no srv
}`)},
			want:    Resolve{Next: nil, Zones: []string{"."}, DoCNAME: true, DoSRV: false},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveParse(tt.args.c)
			if (err != nil) != tt.wantErr {
				t.Errorf("Test resolveParse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Test resolveParse() = %v, want %v", got, tt.want)
			}
		})
	}
}
