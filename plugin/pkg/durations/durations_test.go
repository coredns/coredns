package durations

import (
	"testing"
	"time"
)

func TestNewDurationFromArg(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		wantErr bool
		want    time.Duration
	}{
		{
			name: "valid GO duration - seconds",
			arg:  "30s",
			want: 30 * time.Second,
		},
		{
			name: "valid GO duration - minutes",
			arg:  "2m",
			want: 2 * time.Minute,
		},
		{
			name: "number - fallback to seconds",
			arg:  "30",
			want: 30 * time.Second,
		},
		{
			name:    "invalid duration",
			arg:     "twenty seconds",
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := NewDurationFromArg(test.arg)
			if test.wantErr && err == nil {
				t.Error("error was expected")
			}
			if !test.wantErr && err != nil {
				t.Error("error was not expected")
			}

			if test.want != actual {
				t.Errorf("expected '%v' got '%v'", test.want, actual)
			}
		})
	}
}
