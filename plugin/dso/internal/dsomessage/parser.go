package dsomessage

// Parser incrementally parses a DSO message.
//
//  1. [Parser.Start] starts parsing and returns message [MsgHeader]
//  2. Use [Parser.TLVHeader] to parse [TLVHeader] and, optionally, [Parser.TLVUsage] to get [Usage] context
//  3. Use [Parser.XxxTLV] to fully parse the TLV, or [Parser.SkipTLV] to skip
//  4. Continue 2-3 until [ErrDone], or other error, is returned.
//
// Parser is safe to copy to preserve the parsing state.
//
// Note that there is no requirement to fully skip or parse the message.
type Parser struct {
	msg []byte
	off int

	header    MsgHeader
	tlvHeader TLVHeader
	tlvUsage  Usage

	tlvSameResponseType bool
	isPadded            bool
}

// Start parses [MsgHeader] and enables the parsing of TLVs.
func (p *Parser) Start(msg []byte, origin Origin) (MsgHeader, error) {
	if len(msg) < MsgHeaderLen {
		return MsgHeader{}, ErrHeader
	}
	p.header, p.off = unpackMsgHeader(msg, 0)
	p.msg = msg
	p.tlvUsage = Usage(origin)
	return p.header, nil
}

// SetResponseType sets primary response type of message to distinguish
// response primary from response additional usage contexts.
func (p *Parser) SetResponseType(t Type) {
	if p.off <= MsgHeaderLen {
		p.tlvHeader.Type = t
		p.tlvSameResponseType = true
	}
}

// IsPadded returns true if message contains [EncryptionPadding].
// The result it cached.
func (p *Parser) IsPadded() bool {
	var (
		h   TLVHeader
		off = MsgHeaderLen
	)
	for !p.isPadded && len(p.msg)-off >= TLVHeaderLen {
		h, off = unpackTLVHeader(p.msg, off)
		p.isPadded = h.Type == TypeEncryptionPadding
		off += int(h.Length)
	}
	return p.isPadded
}

// TLVHeader parses a single [TLVHeader].
func (p *Parser) TLVHeader() (h TLVHeader, err error) {
	if p.off == len(p.msg) {
		return TLVHeader{}, ErrDone
	}
	if len(p.msg)-p.off < TLVHeaderLen {
		return TLVHeader{}, errMalformed
	}
	h, p.off = unpackTLVHeader(p.msg, p.off)
	p.tlvSameResponseType = p.tlvSameResponseType && h.Type == p.tlvHeader.Type
	p.isPadded = p.isPadded || h.Type == TypeEncryptionPadding
	p.tlvHeader = h
	return h, nil
}

// TLVUsage returns [Usage] context of current TLV.
func (p *Parser) TLVUsage() (u Usage) {
	switch {
	case p.off <= MsgHeaderLen:
		u = UsageInvalid
	case p.off == MsgHeaderLen+TLVHeaderLen && p.header.ID == 0:
		u = UsageSU
	case p.off == MsgHeaderLen+TLVHeaderLen && !p.header.Response:
		u = UsageSP
	case !p.header.Response:
		u = UsageSA
	case p.tlvSameResponseType:
		u = UsageCRP
	default:
		u = UsageCRA
	}
	if p.tlvUsage&UsageFromClient != 0 {
		u = u << usageClientOff
	}
	return u
}

// SkipTLV skips a single TLV.
func (p *Parser) SkipTLV() (err error) {
	if len(p.msg)-p.off < int(p.tlvHeader.Length) {
		return errMalformed
	}
	p.off += int(p.tlvHeader.Length)
	return nil
}

// KeepAlive parses a single [KeepAlive] TLV.
func (p *Parser) KeepAlive() (tlv KeepAlive, err error) {
	p.off, err = tlv.unpack(p.msg, p.off, p.tlvHeader.Length)
	return tlv, err
}

// RetryDelay parses a single [RetryDelay] TLV.
func (p *Parser) RetryDelay() (tlv RetryDelay, err error) {
	p.off, err = tlv.unpack(p.msg, p.off, p.tlvHeader.Length)
	return tlv, err
}

// EncryptionPadding parses a single [EncryptionPadding] TLV.
func (p *Parser) EncryptionPadding() (tlv EncryptionPadding, err error) {
	p.off, err = tlv.unpack(p.msg, p.off, p.tlvHeader.Length)
	return tlv, err
}

// Subscribe parses a single [Subscribe] TLV.
func (p *Parser) Subscribe() (tlv Subscribe, err error) {
	p.off, err = tlv.unpack(p.msg, p.off, p.tlvHeader.Length)
	return tlv, err
}

// Push parses a single [Push] TLV.
//
// If parser was able to partially
func (p *Parser) Push() (tlv Push, err error) {
	p.off, err = tlv.unpack(p.msg, p.off, p.tlvHeader.Length)
	return tlv, err
}

// Unsubscribe parses a single [Unsubscribe] TLV.
func (p *Parser) Unsubscribe() (tlv Unsubscribe, err error) {
	p.off, err = tlv.unpack(p.msg, p.off, p.tlvHeader.Length)
	return tlv, err
}

// Reconfirm parses a single [Reconfirm] TLV.
func (p *Parser) Reconfirm() (tlv Reconfirm, err error) {
	p.off, err = tlv.unpack(p.msg, p.off, p.tlvHeader.Length)
	return tlv, err
}

func (p *Parser) TLV() (TLV, error) {
	switch p.tlvHeader.Type {
	case TypeKeepAlive:
		tlv, err := p.KeepAlive()
		return &tlv, err
	case TypeRetryDelay:
		tlv, err := p.RetryDelay()
		return &tlv, err
	case TypeEncryptionPadding:
		tlv, err := p.EncryptionPadding()
		return &tlv, err
	case TypeSubscribe:
		tlv, err := p.Subscribe()
		return &tlv, err
	case TypePush:
		tlv, err := p.Push()
		return &tlv, err
	case TypeUnsubscribe:
		tlv, err := p.Unsubscribe()
		return &tlv, err
	case TypeReconfirm:
		tlv, err := p.Reconfirm()
		return &tlv, err
	default:
		return nil, ErrTLV
	}
}
