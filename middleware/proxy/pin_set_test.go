package proxy

import (
	"fmt"
	"net/http"
	"testing"
)

func TestPinsetMalformedPin(t *testing.T) {

	p, err := NewPinSet([]string{
		"foo",
	})

	if msg := err.Error(); msg != `failed to decode pin "foo": illegal base64 data at input byte 0` {
		t.Errorf("unexpected error message %q", msg)
	}

	if p != nil {
		t.Error("unexpected pinset")
	}

}

func TestPinsetValid(t *testing.T) {

	cb := func(publicKey interface{}, pin string) {
		fmt.Printf("encountered pin %q\n", pin)
	}

	p, err := NewPinSet([]string{
		"6hbKqnGS0tU3sWk+eQZbIa3OZ2FkgUQWR9ck0mnMBSk=", // EC 256 bits (SHA256withRSA)
		"tNbb3RUi+t5M6pyOr5M4mx1Q26p30zTinmAViYd9ZQ4=", // RSA 2048 bits (SHA256withRSA)
	})

	if err != nil {
		t.Errorf("unexpected error message %q", err.Error())
	}

	p.Callback = cb

	res, err := http.Get("https://dns.google.com")

	if err != nil {
		t.Errorf("unexpected error message %q", err.Error())
	}

	if err := p.Check(res.TLS); err != nil {
		t.Errorf("unexpected error message %q", err.Error())
	}

}
