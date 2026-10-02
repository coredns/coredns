package dsomessage

import (
	"errors"
	"slices"
	"testing"

	"github.com/miekg/dns"
)

func newResponseMsg(id uint16) (m *Msg) {
	m = &Msg{
		MsgHeader: MsgHeader{
			ID:       id,
			Response: true,
			Rcode:    dns.RcodeSuccess,
		},
		TLV: []TLV{
			&KeepAlive{},
		},
	}
	return m
}

func newRequestMsg(id uint16) (m *Msg) {
	m = &Msg{
		MsgHeader: MsgHeader{
			ID:       id,
			Response: false,
			Rcode:    dns.RcodeSuccess,
		},
		TLV: []TLV{
			&KeepAlive{},
		},
	}
	return m
}

func newUnidirectionalMsg() (m *Msg) {
	m = &Msg{
		MsgHeader: MsgHeader{
			ID:       0,
			Response: false,
			Rcode:    dns.RcodeSuccess,
		},
		TLV: []TLV{
			&KeepAlive{},
		},
	}
	return m
}

func TestParserAllocs(t *testing.T) {
	n := testing.AllocsPerRun(100, func() {
		p := Parser{}
		p.Start(testMsgBytes, OriginClient)
		_, err := p.TLVHeader()
		if err != nil {
			t.Fail()
		}
		_, err = p.KeepAlive()
		if err != nil {
			t.Fail()
		}

		_, err = p.TLVHeader()
		if err != nil {
			t.Fail()
		}
		_, err = p.RetryDelay()
		if err != nil {
			t.Fail()
		}

		_, err = p.TLVHeader()
		if err != nil {
			t.Fail()
		}
		_, err = p.EncryptionPadding()
		if err != nil {
			t.Fail()
		}

		h, err := p.TLVHeader()
		if h.Type != TypeSubscribe || err != nil {
			t.Fail()
		}
		err = p.SkipTLV()
		if err != nil {
			t.Fail()
		}

		h, err = p.TLVHeader()
		if h.Type != TypePush || err != nil {
			t.Fail()
		}
		err = p.SkipTLV()
		if err != nil {
			t.Fail()
		}

		_, err = p.TLVHeader()
		if err != nil {
			t.Fail()
		}
		_, err = p.Unsubscribe()
		if err != nil {
			t.Fail()
		}

		h, err = p.TLVHeader()
		if h.Type != TypeReconfirm || err != nil {
			t.Fail()
		}
		err = p.SkipTLV()
		if err != nil {
			t.Fail()
		}

		if _, err = p.TLVHeader(); err != ErrDone {
			t.Errorf("Expected parser to be done")
		}
	})
	if n != 0 {
		t.Errorf("Expected no allocations, got %v", n)
	}
}

func TestParseMalformed(t *testing.T) {
	t.Parallel()

	msgBytes := slices.Clone(testMsgBytes)

	t.Run("header", func(t *testing.T) {
		t.Parallel()

		var p Parser
		_, err := p.Start(msgBytes[:MsgHeaderLen-1], OriginClient)
		if err == nil {
			t.Error("Expected parser to fail")
		}
	})

	t.Run("TLV header", func(t *testing.T) {
		t.Parallel()

		var (
			p       Parser
			testMsg = testMsg.Clone()
		)
		_, err := p.Start(msgBytes[:MsgHeaderLen+TLVHeaderLen+testMsg.TLV[0].Len()+1], OriginClient)
		if err != nil {
			t.Fatal("Expected parser to start")
		}

		p.TLVHeader()
		p.SkipTLV()
		_, err = p.TLVHeader()
		if err == nil {
			t.Error("Expected parser to fail")
		}
	})

	t.Run("skip TLV", func(t *testing.T) {
		t.Parallel()

		var (
			p       Parser
			testMsg = testMsg.Clone()
		)
		msg := msgBytes[:MsgHeaderLen+TLVHeaderLen+testMsg.TLV[0].Len()+TLVHeaderLen+testMsg.TLV[1].Len()-1]
		_, err := p.Start(msg, OriginClient)
		if err != nil {
			t.Fatal("Expected parser to start")
		}

		p.TLVHeader()
		err = p.SkipTLV()
		if err != nil {
			t.Errorf("Expected to parse KeepAlive, got %v", err)
		}

		p.TLVHeader()
		err = p.SkipTLV()
		if err == nil {
			t.Error("Expected parser to fail")
		}
	})
}

func TestParserIsPadded(t *testing.T) {
	t.Parallel()

	t.Run("direct lookup", func(t *testing.T) {
		t.Parallel()

		var p Parser
		_, err := p.Start(slices.Clone(testMsgBytes), OriginServer)
		if err != nil {
			t.Fatalf("Expected parser to start, got %v", err)
		}
		if !p.IsPadded() {
			t.Error("Expected message to be padded")
		}
	})

	t.Run("parsing", func(t *testing.T) {
		t.Parallel()

		var p Parser
		_, err := p.Start(slices.Clone(testMsgBytes), OriginServer)
		if err != nil {
			t.Fatalf("Expected parser to start, got %v", err)
		}

		_, err = p.TLVHeader()
		for !errors.Is(err, ErrDone) {
			p.SkipTLV()
			_, err = p.TLVHeader()
		}
		if !p.IsPadded() {
			t.Error("Expected message to be padded")
		}
	})
}

func TestTLVUsage(t *testing.T) {
	t.Parallel()

	tlvs := []TLV{
		&KeepAlive{},
		&KeepAlive{},
		&EncryptionPadding{},
	}

	tcs := []struct {
		name    string
		m       *Msg
		repType Type
		origin  Origin
		usage   []Usage
	}{
		{
			"server REQ",
			newRequestMsg(1),
			0,
			OriginServer,
			[]Usage{
				UsageSP,
				UsageSA,
				UsageSA,
			},
		},
		{
			"server UNI",
			newUnidirectionalMsg(),
			0,
			OriginServer,
			[]Usage{
				UsageSU,
				UsageSA,
				UsageSA,
			},
		},
		{
			"server REP",
			newResponseMsg(1),
			TypeKeepAlive,
			OriginServer,
			[]Usage{
				UsageCRP,
				UsageCRP,
				UsageCRA,
			},
		},
		{
			"client REQ",
			newRequestMsg(1),
			0,
			OriginClient,
			[]Usage{
				UsageCP,
				UsageCA,
				UsageCA,
			},
		},
		{
			"client UNI",
			newUnidirectionalMsg(),
			0,
			OriginClient,
			[]Usage{
				UsageCU,
				UsageCA,
				UsageCA,
			},
		},
		{
			"client REP",
			newResponseMsg(1),
			TypeKeepAlive,
			OriginClient,
			[]Usage{
				UsageSRP,
				UsageSRP,
				UsageSRA,
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.m.TLV = tlvs
			b := NewBuilder(make([]byte, 128))
			tc.m.PackTo(b)
			msgBytes, _ := b.Message()

			var p Parser
			p.SetResponseType(tc.repType)
			_, err := p.Start(msgBytes, tc.origin)
			if err != nil {
				t.Fatalf("Expected parser to start, got %v", err)
			}

			var gotUsage []Usage
			_, err = p.TLVHeader()
			for !errors.Is(err, ErrDone) {
				gotUsage = append(gotUsage, p.TLVUsage())
				p.SkipTLV()
				_, err = p.TLVHeader()
			}

			if !slices.Equal(tc.usage, gotUsage) {
				t.Error("Expected usage to be equal")
			}
		})
	}
}

func FuzzParser(f *testing.F) {
	f.Add(testMsgBytes)
	f.Add(testMsgCompressedBytes)
	f.Fuzz(func(t *testing.T, in []byte) {
		var (
			parser Parser
			header MsgHeader
			tlv    TLV
			err    error
		)
		header, err = parser.Start(in, OriginServer)
		if err == nil {
			err = header.Verify()
		}
		if err != nil && !errors.Is(err, ErrHeader) {
			t.Fatalf("Unexpected error %v", err)
		}
		for {
			_, err = parser.TLVHeader()
			if err == nil {
				tlv, err = parser.TLV()
				if err == nil {
					err = tlv.Verify(parser.TLVUsage())
				} else if errors.Is(err, ErrTLV) {
					err = parser.SkipTLV()
				}
			}
			if err == ErrDone || errors.Is(err, ErrTLV) {
				break
			} else if err != nil {
				t.Fatalf("Unexpected error %v", err)
			}
		}
	})
}

func BenchmarkParser(b *testing.B) {
	for _, tlv := range testMsg.TLV {
		b.Run(tlv.Type().String(), func(b *testing.B) {
			builder := NewBuilder(make([]byte, 128))
			builder.WriteTLV(tlv)
			msgBytes, _ := builder.Message()
			for b.Loop() {
				var p Parser
				_, _ = p.Start(msgBytes, OriginServer)
				_, _ = p.TLVHeader()
				_ = p.TLVUsage()
				_, _ = p.TLV()
			}
		})
	}
}
