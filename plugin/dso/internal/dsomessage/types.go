package dsomessage

import (
	"encoding"
	"encoding/binary"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

type (
	// MsgHeader represents DSO DNS message header fields.
	MsgHeader struct {
		ID       uint16
		Response bool
		Rcode    uint8
	}

	// Type is DSO TLV type.
	Type uint16

	// TLVHeader is header of DSO TLV.
	TLVHeader struct {
		Type   Type
		Length uint16
	}

	// TLV is generic DSO TLV.
	TLV interface {
		// Type returns numerical TLV type.
		Type() Type
		// Len returns upper boundary of uncompressed length. Actual wire length can be lower.
		Len() int
		// Verify checks that TLV satisfies specification in given usage context.
		Verify(usage Usage) error
		// Equal returns true if TLVs are equal.
		Equal(tlv TLV) bool
		// Clone creates deep-copy of TLV.
		Clone() TLV
		// String converts TLV to readable string.
		String() string

		encoding.BinaryAppender
		encoding.BinaryMarshaler
		encoding.BinaryUnmarshaler

		pack(buf []byte, off int, compression map[string]int) (int, error)
		unpack(msg []byte, off int, tlvLen uint16) (int, error)
	}

	// KeepAlive is RFC 8490, Section 7.1 Keepalive TLV.
	KeepAlive struct {
		// This is the timeout at which the client MUST begin closing an inactive DSO Session.
		InactivityTimeout uint32
		// This is the interval at which a client MUST generate DSO keepalive traffic to maintain
		// connection state.
		KeepAliveInterval uint32
	}

	// RetryDelay is RFC 8490, Section 7.2 Retry Delay TLV.
	RetryDelay struct {
		// A time value within which the initiator MUST NOT retry this operation or retry connecting
		// to this server.
		RetryDelay uint32
	}

	// EncryptionPadding is RFC 8490, Section 7.3 Encryption Padding TLV.
	//
	// Even the empty TLV adds 4 bytes due to header.
	// See also RFC 8467.
	EncryptionPadding struct {
		Padding uint16
	}

	// Subscribe is RFC 8765, Section 6.2 Subscribe TLV.
	Subscribe struct {
		// Domain name of RR that subscriber wants.
		//
		// DNS wildcarding is not supported, case insensitivity applies, CNAME matches
		// only a CNAME record.
		Name string
		// Type of RR that subscriber wants.
		//
		// TypeANY (255) is interepreted to mean "ALL".
		RRType uint16
		// Class of RR that subscriber wants.
		//
		// ClassANY (255) is interpreted to mean "ALL".
		Class uint16
	}

	// Push is RFC 8765, Section 6.3 Push TLV.
	Push struct {
		// Changes (at least one) in RRs the receiver is subscribed to.
		//
		// RR's TTL value may carry special meaning, see RFC 8765, Section 6.3.1 for details.
		Change []dns.RR
	}

	// Unsubscribe is RFC 8765, Section 6.4 Unsubscribe TLV.
	Unsubscribe struct {
		// ID of the previously sent Subscribe request message.
		SubscribeID uint16
	}

	// Reconfirm is RFC 8765, Section 6.5 Reconfirm TLV.
	Reconfirm struct {
		// RR that the sender belives to be stale.
		//
		// RR's type must not be TypeANY (255), class must not be ClassANY (255), wildcarding
		// is not supported, case insensitivity applies, CNAME matches only a CNAME record.
		// RR's TTL is ignored and Rdlength is re-calculated.
		RR dns.RR
	}

	// Origin indicates side message originated from for verification of RFC compliance.
	Origin uint16

	// Usage is RFC 8490, Section 8.2 TLV usage matrix.
	Usage uint16
)

const (
	LengthPrefixLen = 2
	MsgHeaderLen    = 12
	TLVHeaderLen    = 4

	KeepAliveLen   = 8
	RetryDelayLen  = 4
	UnsubscribeLen = 2

	// TLSBlockLen is recommended multiple for padding.
	//
	// RFC 8467, Section 4.1: If a server receives a query that includes the EDNS(0) "Padding"
	// option, ... SHOULD pad the corresponding response to a multiple of 468 octets
	TLSBlockLen = 468

	MaxMsgLen = dns.MaxMsgSize
)

const (
	TypeKeepAlive         = Type(dns.StatefulTypeKeepAlive)
	TypeRetryDelay        = Type(dns.StatefulTypeRetryDelay)
	TypeEncryptionPadding = Type(dns.StatefulTypeEncryptionPadding)

	TypeSubscribe   Type = 0x0040
	TypePush        Type = 0x0041
	TypeUnsubscribe Type = 0x0042
	TypeReconfirm   Type = 0x0043
)

const (
	// RFC 8490, Section 6.2: On a new DSO Session, if no explicit DSO Keepalive message exchange
	// has taken place, the default value ... is 15 seconds.
	InactivityTimeoutDefault = 15 * 1000
	// RFC 8490, Section 6.4.2: An inactivity timeout of 0xFFFFFFFF represents "infinity"
	// and informs the client that it may keep an idle connection open as long as it wishes.
	InactivityTimeoutNever = 0xFFFFFFFF

	// RFC 8490, Section 6.2: On a new DSO Session, if no explicit DSO Keepalive message exchange
	// has taken place, the default value ... is 15 seconds.
	KeepAliveIntervalDefault = 15 * 1000
	// RFC 8490, Section 6.5.2: By default, it is RECOMMENDED that clients request, and servers
	// grant, a keepalive interval of 60 minutes.
	KeepAliveIntervalRecommended = 60 * 60 * 1000
	// RFC 8490, Section 7.1: The keepalive interval MUST NOT be less than ten seconds.
	KeepAliveIntervalMin = 10 * 1000
	// RFC 8490, Section 6.5.2: A keepalive interval value of 0xFFFFFFFF represents "infinity"
	// and informs the client that it should generate no DSO keepalive traffic.
	KeepAliveIntervalNever = 0xFFFFFFFF
)

const (
	// RFC 8765, Section 6.3.1: If the TTL has the value 0xFFFFFFFF, then the DNS Resource Record
	// with the given name, type, class, and RDATA is removed.
	PushTTLRemove = 0xFFFFFFFF
	// RFC 8765, Section 6.3.1: If the TTL has the value 0xFFFFFFFE, then this is a 'collective'
	// remove notification.
	PushTTLCollectiveRemove = 0xFFFFFFFE
	// RFC 8765, Section 6.3.1: If the TTL is in the range 0 to 2,147,483,647 seconds
	// (0 to 231 - 1, or 0x7FFFFFFF), then a new DNS Resource Record with the given name,
	// type, class, and RDATA is added.
	PushTTLAddMin = 0
	// RFC 8765, Section 6.3.1: If the TTL is in the range 0 to 2,147,483,647 seconds
	// (0 to 2^(31) - 1, or 0x7FFFFFFF), then a new DNS Resource Record with the given name,
	// type, class, and RDATA is added.
	PushTTLAddMax = 0x7FFFFFFF
	// RFC 8765, Section 6.3.1: Servers may generate PUSH messages up to a maximum DNS message
	// length of 16,382 bytes, counting from the start of the DSO 12-byte header. Including
	// the two-byte length prefix that is used to frame DNS over a byte stream like TLS,
	// this makes a total of 16,384 bytes. Servers MUST NOT generate PUSH messages larger than this.
	MaxPushMsgLen = 16382

	// PushDomain is domain name of DSO Push server in given zone.
	PushDomain = "_dns-push-tls._tcp."
)

const (
	OriginInvalid Origin = 0

	// OriginServer is origin of messages created on server.
	OriginServer Origin = Origin(UsageFromServer)
	// OriginClient is origin of messages created on client.
	OriginClient Origin = Origin(UsageFromClient)
)

const (
	UsageInvalid Usage = 0

	// UsageSP is primary TLV, sent in DSO request message, from server to client.
	UsageSP Usage = usageP
	// UsageSU is primary TLV, sent in DSO unidirectional message, from server to client.
	UsageSU Usage = usageU
	// UsageSA is additional TLV, optionally added to a DSO request message or DSO unidirectional message
	// from server to client.
	UsageSA Usage = usageA
	// UsageCRP is response primary TLV, included in response message sent back to the client
	// where the DSO-TYPE of the Response TLV matches the DSO-TYPE of the Primary TLV in the request.
	UsageCRP Usage = usageRP
	// UsageCRA is response additional TLV, included in response message sent back to the client where
	// the DSO-TYPE of the Response TLV does not match the DSO-TYPE of the Primary TLV in the request.
	UsageCRA Usage = usageRA

	// UsageCP is primary TLV, sent in DSO request message, from client to server.
	UsageCP Usage = usageP << usageClientOff
	// UsageCU is primary TLV, sent in DSO unidirectional message, from client to server.
	UsageCU Usage = usageU << usageClientOff
	// UsageCA is additional TLV, optionally added to a DSO request message or DSO unidirectional message
	// from client to server.
	UsageCA Usage = usageA << usageClientOff
	// UsageSRP is response primary TLV, included in response message sent back to the server where the DSO-TYPE
	// of the Response TLV matches the DSO-TYPE of the Primary TLV in the request.
	UsageSRP Usage = usageRP << usageClientOff
	// UsageSRA is response additional TLV, included in response message sent back to the server where the DSO-TYPE
	// of the Response TLV does not match the DSO-TYPE of the Primary TLV in the request.
	UsageSRA Usage = usageRA << usageClientOff

	// UsagePrimary is mask of primary usage contexts.
	UsagePrimary Usage = UsageCP | UsageCU | UsageCRP | UsageSP | UsageSU | UsageSRP
	// UsageAdditional is mask of additional usage contexts.
	UsageAdditional Usage = UsageCA | UsageCRA | UsageSA | UsageSRA

	// UsageFromServer is mask of usage contexts originated from server.
	UsageFromServer Usage = UsageSP | UsageSU | UsageSA | UsageCRP | UsageCRA
	// UsageFromClient is mask of usage contexts originated from client.
	UsageFromClient Usage = UsageCP | UsageCU | UsageCA | UsageSRP | UsageSRA

	// UsageKeepAlive is mask of allowed usage contexts of [KeepAlive].
	UsageKeepAlive Usage = UsageCP | UsageCRP | UsageSU
	// UsageRetryDelay is mask of allowed usage contexts of [RetryDelay].
	UsageRetryDelay Usage = UsageCRA | UsageSU | UsageSRA
	// UsageEncryptionPadding is mask of allowed usage contexts of [EncryptionPadding].
	UsageEncryptionPadding Usage = UsageCA | UsageCRA | UsageSA | UsageSRA
	// UsageSubscribe is mask of allowed usage contexts of [Subscribe].
	UsageSubscribe Usage = UsageCP
	// UsagePush is mask of allowed usage contexts of [Push].
	UsagePush Usage = UsageSU
	// UsageUnsubscribe is mask of allowed usage contexts of [Unsubscribe].
	UsageUnsubscribe Usage = UsageCU
	// UsageReconfirm is mask of allowed usage contexts of [Reconfirm].
	UsageReconfirm Usage = UsageCU
)

// IsRequest returns true if the message is a DSO request.
func (h MsgHeader) IsRequest() bool {
	return h.ID != 0 && !h.Response
}

// IsUnidirectional returns true if the message is a DSO unidirectional.
func (h MsgHeader) IsUnidirectional() bool {
	return h.ID == 0 && !h.Response
}

// IsResponse returns true if the message is a DSO response.
func (h MsgHeader) IsResponse() bool {
	return h.ID != 0 && h.Response
}

func (h MsgHeader) Verify() error {
	if h.ID == 0 && h.Response {
		return ErrHeader
	}
	return nil
}

func (h MsgHeader) pack(buf []byte, off int) int {
	v := uint32(h.ID)<<16 | dns.OpcodeStateful<<11 | uint32(h.Rcode)
	if h.Response {
		v |= 1 << 15
	}
	binary.BigEndian.PutUint32(buf[off:], v)
	binary.BigEndian.PutUint64(buf[off+4:], 0)
	return off + MsgHeaderLen
}

func unpackMsgHeader(msg []byte, off int) (MsgHeader, int) {
	return MsgHeader{
		binary.BigEndian.Uint16(msg[off:]),
		msg[off+2]&(1<<7) != 0,
		msg[off+3] & 0xF,
	}, off + MsgHeaderLen
}

func (t Type) String() string {
	switch t {
	case TypeKeepAlive:
		return "KeepAlive"
	case TypeRetryDelay:
		return "RetryDelay"
	case TypeEncryptionPadding:
		return "EncryptionPadding"
	case TypeSubscribe:
		return "Subscribe"
	case TypePush:
		return "Push"
	case TypeUnsubscribe:
		return "Unsubscribe"
	case TypeReconfirm:
		return "Reconfirm"
	default:
		var b strings.Builder
		b.WriteString("Type{0x")
		b.WriteString(strconv.FormatInt(int64(t), 16))
		b.WriteByte('}')
		return b.String()
	}
}

func (h TLVHeader) pack(buf []byte, off int) int {
	binary.BigEndian.PutUint32(buf[off:], uint32(h.Type)<<16|uint32(h.Length))
	return off + TLVHeaderLen
}

func unpackTLVHeader(msg []byte, off int) (TLVHeader, int) {
	return TLVHeader{
		Type(binary.BigEndian.Uint16(msg[off:])),
		binary.BigEndian.Uint16(msg[off+2:]),
	}, off + TLVHeaderLen
}

// Type implements [TLV.Type].
func (tlv KeepAlive) Type() Type {
	return TypeKeepAlive
}

// Len implements [TLV.Len].
func (tlv KeepAlive) Len() int {
	return KeepAliveLen
}

// Verify implements [TLV.Verify].
func (tlv *KeepAlive) Verify(usage Usage) error {
	switch {
	case usage&UsageKeepAlive == 0:
		return errUsage
	case usage&UsageFromServer != 0 && tlv.KeepAliveInterval < KeepAliveIntervalMin:
		return errBadKeepAlive
	default:
		return nil
	}
}

// Equal implements [TLV.Equal].
func (tlv *KeepAlive) Equal(tlv1 TLV) bool {
	ka, ok := tlv1.(*KeepAlive)
	return ok && *tlv == *ka
}

// Clone implements [TLV.Clone].
func (tlv *KeepAlive) Clone() TLV {
	return &KeepAlive{tlv.InactivityTimeout, tlv.KeepAliveInterval}
}

// String implements [TLV.String].
func (tlv KeepAlive) String() string {
	var b strings.Builder
	b.WriteString("timeout ")
	b.WriteString((time.Duration(tlv.InactivityTimeout) * time.Millisecond).String())
	b.WriteString(", interval ")
	b.WriteString((time.Duration(tlv.KeepAliveInterval) * time.Millisecond).String())
	return b.String()
}

func (tlv *KeepAlive) pack(buf []byte, off int, _ map[string]int) (int, error) {
	binary.BigEndian.PutUint64(buf[off:], uint64(tlv.InactivityTimeout)<<32|uint64(tlv.KeepAliveInterval))
	return off + KeepAliveLen, nil
}

func (tlv *KeepAlive) unpack(msg []byte, off int, tlvLen uint16) (int, error) {
	if tlvLen != KeepAliveLen || len(msg)-off < KeepAliveLen {
		return off, errMalformed
	}
	tlv.InactivityTimeout = binary.BigEndian.Uint32(msg[off:])
	tlv.KeepAliveInterval = binary.BigEndian.Uint32(msg[off+4:])
	return off + KeepAliveLen, nil
}

// Type implements [TLV.Type].
func (tlv RetryDelay) Type() Type {
	return TypeRetryDelay
}

// Len implements [TLV.Len].
func (tlv RetryDelay) Len() int {
	return RetryDelayLen
}

// Verify implements [TLV.Verify].
func (tlv *RetryDelay) Verify(usage Usage) error {
	if usage&UsageRetryDelay == 0 {
		return errUsage
	}
	return nil
}

// Equal implements [TLV.Equal].
func (tlv *RetryDelay) Equal(tlv1 TLV) bool {
	rd, ok := tlv1.(*RetryDelay)
	return ok && *tlv == *rd
}

// Clone implements [TLV.Clone].
func (tlv *RetryDelay) Clone() TLV {
	return &RetryDelay{tlv.RetryDelay}
}

// String implements [TLV.String].
func (tlv RetryDelay) String() string {
	return (time.Duration(tlv.RetryDelay) * time.Millisecond).String()
}

func (tlv *RetryDelay) pack(buf []byte, off int, _ map[string]int) (int, error) {
	binary.BigEndian.PutUint32(buf[off:], tlv.RetryDelay)
	return off + RetryDelayLen, nil
}

func (tlv *RetryDelay) unpack(msg []byte, off int, tlvLen uint16) (int, error) {
	if tlvLen != RetryDelayLen || len(msg)-off < RetryDelayLen {
		return off, errMalformed
	}
	tlv.RetryDelay = binary.BigEndian.Uint32(msg[off:])
	return off + RetryDelayLen, nil
}

// Type implements the [TLV.Type].
func (tlv EncryptionPadding) Type() Type {
	return TypeEncryptionPadding
}

// Len implements [TLV.Len].
func (tlv EncryptionPadding) Len() int {
	return int(tlv.Padding)
}

// Verify implements [TLV.Verify].
func (tlv *EncryptionPadding) Verify(usage Usage) error {
	if usage&UsageEncryptionPadding == 0 {
		return errUsage
	}
	return nil
}

// Equal implements [TLV.Equal].
func (tlv *EncryptionPadding) Equal(tlv1 TLV) bool {
	ep, ok := tlv1.(*EncryptionPadding)
	return ok && *tlv == *ep
}

// Clone implements [TLV.Clone]
func (tlv *EncryptionPadding) Clone() TLV {
	return &EncryptionPadding{tlv.Padding}
}

// String implements [TLV.String].
func (tlv EncryptionPadding) String() string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(int(tlv.Padding)))
	b.WriteString(" bytes")
	return b.String()
}

func (tlv *EncryptionPadding) pack(buf []byte, off int, _ map[string]int) (int, error) {
	if tlv.Padding > 0 {
		off += int(tlv.Padding)
		_ = buf[off-1]
	}
	return off, nil
}

func (tlv *EncryptionPadding) unpack(msg []byte, off int, tlvLen uint16) (int, error) {
	if len(msg)-off < int(tlvLen) {
		return off, errMalformed
	}
	tlv.Padding = tlvLen
	return off + int(tlvLen), nil
}

// Type implements [TLV.Type].
func (tlv Subscribe) Type() Type {
	return TypeSubscribe
}

// Len implements [TLV.Len].
func (tlv *Subscribe) Len() int {
	return len(tlv.Name) + 1 + 2 + 2 // Name + \0 + RRType(2) + Class(2)
}

// Verify implements [TLV.Verify].
func (tlv *Subscribe) Verify(usage Usage) error {
	if usage&UsageSubscribe == 0 {
		return errUsage
	}
	return nil
}

// Equal implements [TLV.Equal].
//
// Assumes Name is decoded by [dns.UnpackDomainName]. In particular,
// it expects escape sequences (\\DDD) in place of unicode characters
// that have lowercase per [strings.ToLower]) but follow case-insensitive
// comparison per RFC 1035, Section 3.1
func (tlv *Subscribe) Equal(tlv1 TLV) bool {
	sub, ok := tlv1.(*Subscribe)
	return ok && tlv.Class == sub.Class && tlv.RRType == sub.RRType && strings.EqualFold(tlv.Name, sub.Name)
}

// Clone implements [TLV.Clone].
func (tlv *Subscribe) Clone() TLV {
	return &Subscribe{tlv.Name, tlv.RRType, tlv.Class}
}

// String implements [TLV.String].
func (tlv Subscribe) String() string {
	var b strings.Builder
	b.WriteByte(';')
	b.WriteString(dns.Name(tlv.Name).String())
	b.WriteByte('\t')
	b.WriteString(dns.Class(tlv.Class).String())
	b.WriteString("\t ")
	b.WriteString(dns.Type(tlv.RRType).String())
	return b.String()
}

func (tlv *Subscribe) pack(buf []byte, off int, compression map[string]int) (int, error) {
	nameEnd, err := dns.PackDomainName(tlv.Name, buf, off, compression, compression != nil)
	if err != nil {
		return off, &PackingError{-1, off, err}
	}
	if len(buf) < nameEnd+4 {
		// Don't panic as nameEnd is compression dependent and
		// caller couldn't anticipate needed buffer length.
		return off, &PackingError{-1, off, dns.ErrBuf}
	}
	binary.BigEndian.PutUint16(buf[nameEnd:], tlv.RRType)
	binary.BigEndian.PutUint16(buf[nameEnd+2:], tlv.Class)
	return nameEnd + 4, nil
}

func (tlv *Subscribe) unpack(msg []byte, off int, tlvLen uint16) (off1 int, err error) {
	if len(msg)-off < int(tlvLen) {
		return off, errMalformed
	}
	msg = msg[:off+int(tlvLen)]
	if tlv.Name, off1, err = dns.UnpackDomainName(msg, off); err != nil {
		return off, fmt.Errorf("%w - %w", errMalformed, &UnpackingError{off, err})
	}
	if int(tlvLen)-(off1-off) != 4 {
		return off, errMalformed
	}
	tlv.RRType = binary.BigEndian.Uint16(msg[off1:])
	tlv.Class = binary.BigEndian.Uint16(msg[off1+2:])
	return off + int(tlvLen), nil
}

// Type implements [TLV.Type].
func (tlv Push) Type() Type {
	return TypePush
}

// Len implements [TLV.Len].
func (tlv *Push) Len() (l int) {
	for _, rr := range tlv.Change {
		l += RRLen(rr)
	}
	return l
}

// Verify implements [TLV.Verify].
func (tlv *Push) Verify(usage Usage) error {
	if usage&UsagePush == 0 {
		return errUsage
	}

	var h *dns.RR_Header
	for _, rr := range tlv.Change {
		h = rr.Header()
		switch {
		// RFC 8765, Section 6.3.1: If the TTL is in the range ... 0x7FFFFFFF then a new DNS
		// Resource Record with the given name, type, class, and RDATA is added. Type and class
		// MUST NOT be 255 (ANY).
		// RFC 8765, Section 6.3.1: If the TTL has the value 0xFFFFFFFF, then the DNS Resource
		// Record with the given name, type, class, and RDATA is removed. Type and class
		// MUST NOT be 255 (ANY)
		case (h.Ttl == 0xFFFFFFFF || h.Ttl <= 0x7FFFFFFF) && (h.Class == dns.ClassANY || h.Rrtype == dns.TypeANY):
			return fmt.Errorf("%w - bad class (%d) / type (%d) in push tlv", ErrTLV, h.Class, h.Rrtype)

		// RFC 8765, Section 6.3.1: If the TTL has the value 0xFFFFFFFE, then this is a
		// 'collective' remove notification. For collective remove notifications,
		// RDLEN MUST be zero
		case h.Ttl == 0xFFFFFFFE && h.Rdlength != 0:
			return errBadPushCollective

		// RFC 8765, Section 6.3.1: If the TTL is any value other than 0xFFFFFFFF, 0xFFFFFFFE,
		// or a value in the range 0 to 0x7FFFFFFF, then the receiver SHOULD silently ignore
		// this particular change notification record.
		default:
		}
	}

	// RFC 8765, Section 6.3.1: A PUSH Message MUST contain at least one change notification.
	if h == nil {
		return errEmptyPushChange
	}

	return nil
}

// Equal implements [TLV.Equal].
func (tlv *Push) Equal(tlv1 TLV) bool {
	push, ok := tlv1.(*Push)
	return ok && slices.EqualFunc(tlv.Change, push.Change, func(rr, rr1 dns.RR) bool {
		return dns.IsDuplicate(rr, rr1) && rr.Header().Ttl == rr1.Header().Ttl
	})
}

// Clone implements [TLV.Clone].
func (tlv *Push) Clone() TLV {
	tlv1 := new(Push)
	tlv1.Change = make([]dns.RR, len(tlv.Change))
	for i, rr := range tlv.Change {
		tlv1.Change[i] = dns.Copy(rr)
	}
	return tlv1
}

// String implements [TLV.String].
func (tlv Push) String() string {
	switch {
	case len(tlv.Change) == 0:
		return "<nil>"
	case len(tlv.Change) == 1:
		return tlv.Change[0].String()
	default:
		var s strings.Builder
		s.WriteString(tlv.Change[0].String())
		for _, r := range tlv.Change[1:] {
			s.WriteByte('\n')
			s.WriteString(r.String())
		}
		return s.String()
	}
}

func (tlv *Push) pack(buf []byte, off int, compression map[string]int) (off1 int, err error) {
	off1 = off
	for i, rr := range tlv.Change {
		if off2, err := dns.PackRR(rr, buf, off1, compression, compression != nil); err == nil {
			off1 = off2
		} else {
			return off, &PackingError{i, off1, err}
		}
	}
	return off1, nil
}

func (tlv *Push) unpack(msg []byte, off int, tlvLen uint16) (off1 int, err error) {
	if len(msg)-off < int(tlvLen) {
		return off, errMalformed
	}
	msg = msg[:off+int(tlvLen)]
	off1 = off
	for off1 < len(msg) {
		if rr, off2, err := dns.UnpackRR(msg, off1); err == nil {
			tlv.Change = append(tlv.Change, rr)
			off1 = off2
		} else {
			return off, fmt.Errorf("%w - %w", errMalformed, &UnpackingError{off1, err})
		}
	}
	return off1, nil
}

// Type implements [TLV.Type].
func (tlv Unsubscribe) Type() Type {
	return TypeUnsubscribe
}

// Len implements [TLV.Len].
func (tlv Unsubscribe) Len() int {
	return UnsubscribeLen
}

// Verify implements [TLV.Verify].
func (tlv *Unsubscribe) Verify(usage Usage) error {
	switch {
	case usage&UsageUnsubscribe == 0:
		return errUsage
	case tlv.SubscribeID == 0:
		// RFC 8765, Section 6.4.1: The DSO-DATA contains the value previously given in the MESSAGE ID
		// field of an active SUBSCRIBE request.
		return fmt.Errorf("%w - bad subscribe ID", ErrTLV)
	default:
		return nil
	}
}

// Equal implements [TLV.Equal].
func (tlv *Unsubscribe) Equal(tlv1 TLV) bool {
	unsub, ok := tlv1.(*Unsubscribe)
	return ok && *tlv == *unsub
}

// Clone implements [TLV.Clone].
func (tlv *Unsubscribe) Clone() TLV {
	return &Unsubscribe{tlv.SubscribeID}
}

// String implements [TLV.String].
func (tlv Unsubscribe) String() (s string) {
	return strconv.Itoa(int(tlv.SubscribeID))
}

func (tlv *Unsubscribe) pack(buf []byte, off int, _ map[string]int) (int, error) {
	binary.BigEndian.PutUint16(buf[off:], tlv.SubscribeID)
	return off + UnsubscribeLen, nil
}

func (tlv *Unsubscribe) unpack(msg []byte, off int, tlvLen uint16) (int, error) {
	if tlvLen != UnsubscribeLen || len(msg)-off < UnsubscribeLen {
		return off, errMalformed
	}
	tlv.SubscribeID = binary.BigEndian.Uint16(msg[off:])
	return off + UnsubscribeLen, nil
}

// Type implements [TLV.Type].
func (tlv Reconfirm) Type() Type {
	return TypeReconfirm
}

// Len implements [TLV.Len].
func (tlv *Reconfirm) Len() int {
	return RRLen(tlv.RR)
}

// Verify implements [TLV.Verify].
func (tlv *Reconfirm) Verify(usage Usage) error {
	if usage&UsageReconfirm == 0 {
		return errUsage
	}
	if h := tlv.RR.Header(); h.Class == dns.ClassANY || h.Rrtype == dns.TypeANY {
		return fmt.Errorf("%w - bad class (%d) / type (%d) in reconfirm tlv", ErrTLV, h.Class, h.Rrtype)
	}
	return nil
}

// Equal implements [TLV.Equal].
func (tlv *Reconfirm) Equal(tlv1 TLV) bool {
	rec, ok := tlv1.(*Reconfirm)
	return ok && dns.IsDuplicate(tlv.RR, rec.RR)
}

// Clone implements [TLV.Clone].
func (tlv *Reconfirm) Clone() TLV {
	return &Reconfirm{dns.Copy(tlv.RR)}
}

// String implements [TLV.String].
func (tlv Reconfirm) String() string {
	return tlv.RR.String()
}

func (tlv *Reconfirm) pack(buf []byte, off int, _ map[string]int) (off1 int, err error) {
	var (
		rrEnd       int
		rrHeader    = tlv.RR.Header()
		oldRdlength = rrHeader.Rdlength
	)
	if rrEnd, err = dns.PackRR(tlv.RR, buf, off, nil, false); err != nil {
		return off, &PackingError{-1, off, err}
	}
	copy(buf[rrEnd-int(rrHeader.Rdlength)-2-4:], buf[rrEnd-int(rrHeader.Rdlength):rrEnd]) // discard Rdlength(2) and TTL(4)
	rrHeader.Rdlength = oldRdlength                                                       // pack should not mutate but [dns.PackRR] overwrites Rdlength
	return rrEnd - 2 - 4, nil
}

func (tlv *Reconfirm) unpack(msg []byte, off int, tlvLen uint16) (off1 int, err error) {
	if len(msg)-off < int(tlvLen) {
		return off, errMalformed
	}
	msg = msg[:off+int(tlvLen)]

	var h dns.RR_Header
	if h.Name, off1, err = dns.UnpackDomainName(msg, off); err != nil {
		return off, fmt.Errorf("%w - %w", errMalformed, &UnpackingError{off, err})
	}
	if int(tlvLen)-(off1-off) < 4 {
		return off, errMalformed
	}
	h.Rrtype = binary.BigEndian.Uint16(msg[off1:])
	h.Class = binary.BigEndian.Uint16(msg[off1+2:])
	off1 += 4

	h.Rdlength = tlvLen - uint16(off1-off)
	if rr, off2, err := dns.UnpackRRWithHeader(h, msg, off1); err == nil {
		tlv.RR = rr
		off1 = off2
	} else {
		return off, fmt.Errorf("%w - %w", errMalformed, &UnpackingError{off1, err})
	}

	if off1 != len(msg) {
		return off, errMalformed
	}

	return off1, nil
}

// Remote returns counterparty to current origin.
func (o Origin) Remote() Origin {
	switch o {
	case OriginServer:
		return OriginClient
	case OriginClient:
		return OriginServer
	default:
		return OriginInvalid
	}
}

// Origin extracts origin from usage context.
func (u Usage) Origin() Origin {
	switch {
	case u&UsageFromServer != 0 && u&UsageFromClient == 0:
		return OriginServer
	case u&UsageFromServer == 0 && u&UsageFromClient != 0:
		return OriginClient
	default:
		return OriginInvalid
	}
}

func (u Usage) asOrigin(o Origin) Usage {
	switch u.Origin() {
	case o:
		return u
	case OriginServer:
		return u << usageClientOff
	case OriginClient:
		return u >> usageClientOff
	default:
		return UsageInvalid
	}
}

const (
	usageP         = 1 << 0
	usageU         = 1 << 1
	usageA         = 1 << 2
	usageRP        = 1 << 3
	usageRA        = 1 << 4
	usageClientOff = 5
)

func RRLen(rr dns.RR) (l int) {
	// [dns.Len] doesn't take into account that [dns.packTxt] writes '\0' for empty slice.
	l = dns.Len(rr)
	switch rr := rr.(type) {
	case *dns.AVC:
		if len(rr.Txt) == 0 {
			l += 1
		}
	case *dns.NINFO:
		if len(rr.ZSData) == 0 {
			l += 1
		}
	case *dns.RESINFO:
		if len(rr.Txt) == 0 {
			l += 1
		}
	case *dns.SPF:
		if len(rr.Txt) == 0 {
			l += 1
		}
	case *dns.TXT:
		if len(rr.Txt) == 0 {
			l += 1
		}
	}
	return l
}
