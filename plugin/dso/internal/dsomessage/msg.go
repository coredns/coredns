package dsomessage

import (
	"fmt"
	"slices"
)

type
// Msg bundles DSO header and TLVs.
Msg struct {
	MsgHeader
	TLV    []TLV
	Origin Origin
}

// Equal returns true of messages have equal header and TLVs.
func (m *Msg) Equal(m1 *Msg) bool {
	return m.MsgHeader == m1.MsgHeader && slices.EqualFunc(m.TLV, m1.TLV, func(tlv, tlv1 TLV) bool { return tlv.Equal(tlv1) })
}

// Len returns upper bound of message's wire length.
func (m *Msg) Len() (n int) {
	n = MsgHeaderLen
	for _, tlv := range m.TLV {
		n += TLVHeaderLen + tlv.Len()
	}
	return n
}

// PackTo packs message with given builder.
func (m *Msg) PackTo(b *Builder) {
	b.Clear()
	b.SetHeader(m.MsgHeader)
	for _, tlv := range m.TLV {
		if b.Err() != nil {
			break
		}
		b.WriteTLV(tlv)
	}
}

// Pack packs message with default builder.
func (m *Msg) Pack() (msg []byte, err error) {
	b := NewBuilder(make([]byte, m.Len()))
	m.PackTo(b)
	return b.Message()
}

// Clone returns deep copy of message.
func (m *Msg) Clone() (m1 *Msg) {
	m1 = &Msg{
		MsgHeader: m.MsgHeader,
		TLV:       make([]TLV, len(m.TLV)),
		Origin:    m.Origin,
	}
	for i, tlv := range m.TLV {
		m1.TLV[i] = tlv.Clone()
	}
	return m1
}

// Verify checks that usage of TLVs satisfy requirements with respect to origin and request, if any.
func (m *Msg) Verify(request *Msg) (err error) {
	switch {
	case m.Origin&(OriginServer|OriginClient) == 0:
		return fmt.Errorf("bad origin")
	case m.IsUnidirectional() && len(m.TLV) == 0:
		fallthrough
	case m.IsRequest() && len(m.TLV) == 0:
		return fmt.Errorf("%w - empty TLVs", ErrTLV)
	case request != nil && !m.IsResponse():
		fallthrough
	case request == nil && m.IsResponse():
		fallthrough
	case request != nil && request.ID != m.ID:
		return fmt.Errorf("%w - bad response", ErrHeader)
	}

	var usage Usage
	switch {
	case m.IsUnidirectional():
		usage = UsageSU
	case m.IsRequest():
		usage = UsageSP
	case len(request.TLV) > 0 && len(m.TLV) > 0 && request.TLV[0].Type() == m.TLV[0].Type():
		usage = UsageCRP
	default:
		usage = UsageCRA
	}

	for _, tlv := range m.TLV {
		if usage == UsageCRP && request.TLV[0].Type() != tlv.Type() {
			usage = UsageCRA
		}

		err = tlv.Verify(usage.asOrigin(m.Origin))
		if err != nil {
			return err
		}

		if usage&(UsageSU|UsageSP) != 0 {
			usage = UsageSA
		}
	}
	return nil
}

// UnpackMsg parses message header and TLVs from wire representation.
func UnpackMsg(buf []byte, origin Origin) (m *Msg, err error) {
	var p Parser
	m = &Msg{Origin: origin}
	if m.MsgHeader, err = p.Start(buf, origin); err != nil {
		return nil, err
	}
	var tlv TLV
	for {
		_, err = p.TLVHeader()
		if err != nil {
			break
		}
		tlv, err = p.TLV()
		if err != nil {
			break
		}
		m.TLV = append(m.TLV, tlv)
	}
	if err != ErrDone {
		return nil, err
	}
	return m, nil
}

func NewRepMsg(id uint16, tlvs ...TLV) *Msg {
	return &Msg{
		MsgHeader: MsgHeader{
			ID:       id,
			Response: true,
			Rcode:    0,
		},
		TLV: tlvs,
	}
}

func NewReqMsg(id uint16, tlvs ...TLV) *Msg {
	_ = tlvs[0]
	return &Msg{
		MsgHeader: MsgHeader{
			ID:       id,
			Response: false,
			Rcode:    0,
		},
		TLV: tlvs,
	}
}

func NewUniMsg(tlvs ...TLV) *Msg {
	_ = tlvs[0]
	return &Msg{
		MsgHeader: MsgHeader{
			ID:       0,
			Response: false,
			Rcode:    0,
		},
		TLV: tlvs,
	}
}

func NewErrorMsg(id uint16, rcode uint8, tlvs ...TLV) (m *Msg) {
	m = NewRepMsg(id, tlvs...)
	m.Rcode = rcode
	return m
}
