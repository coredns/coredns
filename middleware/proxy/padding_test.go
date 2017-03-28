package proxy

import (
	"bytes"
	"testing"
)

func TestPaddingGenerateEmpty(t *testing.T) {

	p := NewPadding(4)
	p.Random = bytes.NewBuffer(nil)

	out, err := p.Generate("google.com")

	if len(out) > 0 {
		t.Errorf("result is not empty")
	}

	if err != nil {
		t.Errorf("unexpected error %s", err)
	}

}

func TestPaddingGenerateError(t *testing.T) {

	p := NewPadding(256)
	p.Random = bytes.NewBuffer(nil)

	_, err := p.Generate("google.com")

	if err == nil {
		t.Errorf("error is nil")
	} else if err.Error() != "failed to generate random bytes: EOF" {
		t.Errorf("unexpected error: %q", err)
	}

}

func TestPaddingGenerateRandom(t *testing.T) {

	it := 65536

	p := NewPadding(256)

	m := make(map[string]interface{}, it)

	for i := 0; i < it; i++ {

		out, err := p.Generate("google.com")

		if err != nil {
			t.Errorf("failed to generate padding: %s", err)
		}

		if _, ok := m[out]; ok {
			t.Errorf("duplicate padding %q generated", out)
		}

		m[out] = struct{}{}

	}

}
