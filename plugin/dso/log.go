package dso

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/coredns/coredns/plugin/dso/internal/dsomessage"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

func formatLog(conn net.Conn, origin dsomessage.Origin, buf []byte) string {
	var (
		parser dsomessage.Parser
		b      strings.Builder
	)

	b.WriteString(conn.RemoteAddr().String())
	b.WriteString(" - ")

	switch origin {
	case dsomessage.OriginClient:
		b.WriteString("IN:")
	case dsomessage.OriginServer:
		b.WriteString("OUT:")
	default:
		b.WriteString("BAD:")
	}

	h, err := parser.Start(buf, origin)
	switch {
	case err != nil:
		b.WriteString("BAD")
		b.WriteString(strconv.Itoa(len(buf)))
		return b.String()
	case h.IsRequest():
		b.WriteString("REQ:")
		b.WriteString(strconv.Itoa(int(h.ID)))
	case h.IsResponse():
		b.WriteString("REP:")
		b.WriteString(strconv.Itoa(int(h.ID)))
	case h.IsUnidirectional():
		b.WriteString("UNI")
	}
	b.WriteByte('\n')

	var (
		tlv  dsomessage.TLV
		tlvH dsomessage.TLVHeader
	)
	tlvH, err = parser.TLVHeader()
	for i := 0; err == nil; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		u := parser.TLVUsage()
		switch {
		case u&dsomessage.UsagePrimary != 0:
			b.WriteString("   Primary: ")
		case u&dsomessage.UsageAdditional != 0:
			b.WriteString("Additional: ")
		default:
			b.WriteString("       Bad: ")
		}
		b.WriteString(tlvH.Type.String())
		b.WriteByte('\n')

		tlv, err = parser.TLV()
		if err != nil {
			b.WriteString(err.Error())
			err = parser.SkipTLV()
			if err != nil {
				break
			}
		} else {
			b.WriteString(tlv.String())
		}

		tlvH, err = parser.TLVHeader()
	}

	if err != dsomessage.ErrDone {
		b.Reset()
		b.WriteString("malformed")
		if clog.D.Value() {
			b.WriteString(":")
			hexdump(&b, buf)
		}
	}

	return b.String()
}

func hexdump(b *strings.Builder, data []byte) {
	for i := range data {
		if i%16 == 0 {
			fmt.Fprintf(b, "\ndebug: %06x", i)
		}
		fmt.Fprintf(b, " %02x", data[i])
	}
	fmt.Fprintf(b, "\ndebug: %06x", len(data))
}
