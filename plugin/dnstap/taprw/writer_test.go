package taprw

import (
	"bytes"
	"errors"
	"testing"

	"github.com/coredns/coredns/plugin/dnstap/msg"
	"github.com/coredns/coredns/plugin/dnstap/test"
	mwtest "github.com/coredns/coredns/plugin/test"

	tap "github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
)

type TapFailer struct {
}

func (TapFailer) TapMessage(*tap.Message, []byte) error {
	return errors.New("failed")
}
func (TapFailer) TapBuilder() msg.Builder {
	return msg.Builder{Full: true}
}

func TestDnstapError(t *testing.T) {
	rw := ResponseWriter{
		Query:          new(dns.Msg),
		ResponseWriter: &mwtest.ResponseWriter{},
		Tapper:         TapFailer{},
	}
	if err := rw.WriteMsg(new(dns.Msg)); err != nil {
		t.Errorf("dnstap error during Write: %s", err)
	}
	if rw.DnstapError() == nil {
		t.Fatal("no dnstap error")
	}
}

func testingMsg() (m *dns.Msg) {
	m = new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)
	m.SetEdns0(4097, true)
	return
}

func TestClientQueryResponse(t *testing.T) {
	trapper := test.TrapTapper{Full: true}
	m := testingMsg()
	rw := ResponseWriter{
		Query:          m,
		Tapper:         &trapper,
		ResponseWriter: &mwtest.ResponseWriter{},
	}
	d := test.TestingData()

	// will the wire-format msg be reported?
	bin, err := m.Pack()
	if err != nil {
		t.Fatal(err)
		return
	}
	d.Packed = bin

	if err := rw.WriteMsg(m); err != nil {
		t.Fatal(err)
		return
	}
	if l := len(trapper.Trap); l != 2 {
		t.Fatalf("%d msg trapped", l)
		return
	}
	want := d.ToClientQuery()
	have := trapper.Trap[0]
	if !test.MsgEqual(want, have) {
		t.Fatalf("query: want: %v\nhave: %v", want, have)
	}
	want = d.ToClientResponse()
	have = trapper.Trap[1]
	if !test.MsgEqual(want, have) {
		t.Fatalf("response: want: %v\nhave: %v", want, have)
	}
}

func testingExtraData() map[string]DnstapExtra {
	m := make(map[string]DnstapExtra)
	policy_extra := DnstapExtra{extras: make(map[tap.Message_Type][]byte)}
	policy_extra.extras[tap.Message_CLIENT_QUERY] = []byte{0xaa, 0xaa, 0xaa, 0xaa}
	policy_extra.extras[tap.Message_CLIENT_RESPONSE] = []byte{0xbb, 0xbb, 0xbb, 0xbb}
	m["policy"] = policy_extra
	return m
}

func TestClientQueryResponseWithExtra(t *testing.T) {
	trapper := test.TrapTapper{Full: true}
	m := testingMsg()
	extra := testingExtraData()
	rw := ResponseWriter{
		Query:          m,
		Tapper:         &trapper,
		ResponseWriter: &mwtest.ResponseWriter{},
		DnsTapExtras:   extra,
	}
	d := test.TestingData()
	bin, err := m.Pack()
	if err != nil {
		t.Fatal(err)
		return
	}
	d.Packed = bin

	if err := rw.WriteMsg(m); err != nil {
		t.Fatal(err)
		return
	}
	if l := len(trapper.Trap); l != 2 {
		t.Fatalf("%d msg trapped", l)
		return
	}

	e_want := rw.extraData(tap.Message_CLIENT_QUERY)
	e_have := trapper.Extra[0]
	if bytes.Compare(e_want, e_have) > 0 {
		t.Fatalf("extra in query: want: %v\nhave: %v", e_want, e_have)
	}

	e_want = rw.extraData(tap.Message_CLIENT_RESPONSE)
	e_have = trapper.Extra[1]
	if bytes.Compare(e_want, e_have) > 0 {
		t.Fatalf("extra in query: want: %v\nhave: %v", e_want, e_have)
	}

}
