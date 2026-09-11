package dsomessage

import (
	"encoding/binary"
	"io"
	"maps"

	"github.com/miekg/dns"
)

// Builder incrementally builds DSO message.
//
// Errors are sticky. It's expected that callers take care allocating and growing buffers
// such that under normal program flow TLVs fit without fail. Thus when [Builder.Message]
// returns an error it's because user miscalculated buffer size, tried to write malformed
// data or some other programmer's error fatal to DSO session.
//
// When padding is enabled, operations that change layout or length enforce requirement
// of having enough available space to pad message.
type Builder struct {
	buf []byte
	msg []byte // len(msg) is multiple of blockLen up to maxMsgLen with [HeaderLen] in bounds

	base     int // offset of msg in buf
	off      int // position for next write
	blockLen int // block length for padding, constrained to uint16

	compression map[string]int // RR compression map

	err error
}

// NewBuilder returns new [Builder] that takes ownership of buf.
//
// Will panic if buffer's length is less than [MsgHeaderLen].
func NewBuilder(buf []byte) *Builder {
	return (&Builder{buf: buf[:cap(buf)]}).Reset()
}

var zeroMsgHeader = []byte{0, 0, dns.OpcodeStateful << 3, 0, 0, 0, 0, 0, 0, 0, 0, 0}

// Reset resets builder to like new state.
func (b *Builder) Reset() *Builder {
	*b = Builder{
		buf:      b.buf,
		msg:      b.buf,
		off:      MsgHeaderLen,
		blockLen: 1,
	}
	copy(b.buf, zeroMsgHeader)
	return b
}

// Clear clears error and resets message but maintains header and settings.
func (b *Builder) Clear() *Builder {
	b.off = MsgHeaderLen
	b.err = nil
	clear(b.compression)
	return b
}

// Err returns error of most recent sticky error.
func (b *Builder) Err() error {
	return b.err
}

// checkTLVLen ensures that off padded to blockLen fits msgLen.
func checkTLVLen(off, blockLen, msgLen int) bool {
	return (off <= msgLen && off%blockLen == 0) || off+TLVHeaderLen <= msgLen
}

// EnableLengthPrefix uses first two bytes to encode message length.
//
// [ErrBufferFull] is set if there is not enough available space for adjustment.
func (b *Builder) EnableLengthPrefix() *Builder {
	if b.err == nil {
		msgLen := (len(b.buf) - LengthPrefixLen) / b.blockLen * b.blockLen
		if checkTLVLen(b.off, b.blockLen, msgLen) {
			b.msg = b.buf[LengthPrefixLen : LengthPrefixLen+msgLen]
			copy(b.msg, b.buf[b.base:b.base+b.off])
			b.base = LengthPrefixLen
		} else {
			b.err = ErrBufferFull
		}
	}
	return b
}

// EnableCompression enables compression for [dns.PackRR] and [dns.PackDomainName].
func (b *Builder) EnableCompression() *Builder {
	if b.err == nil {
		if b.compression == nil {
			b.compression = make(map[string]int)
		}
	}
	return b
}

// EnablePadding aligns message length to multiple of blockLen.
//
// If > 1 writes enforce that either message length is proper multiple
// or at least [TLVHeaderLen] bytes are available for [EncryptionPadding] TLV.
//
// [ErrBufferFull] is set if there is not enough available space for adjustment.
func (b *Builder) EnablePadding(blockLen uint16) *Builder {
	if b.err == nil {
		msgLen := (len(b.buf) - b.base) / int(blockLen) * int(blockLen)
		if checkTLVLen(b.off, int(blockLen), msgLen) {
			b.msg = b.buf[b.base : b.base+msgLen]
			b.blockLen = int(blockLen)
		} else {
			b.err = ErrBufferFull
		}
	}
	return b
}

// Grow resizes underlying buffer to guarantee space for another n bytes.
//
// If n is negative, Grow will panic.
func (b *Builder) Grow(n int) *Builder {
	if n < 0 {
		panic("dsomessage.Builder.Grow: negative count")
	}
	if b.err == nil {
		n += b.off
		if n%b.blockLen != 0 {
			n = (n + TLVHeaderLen + b.blockLen - 1) / b.blockLen * b.blockLen
		}
		if len(b.msg) < n {
			b.buf = append(b.buf, make([]byte, b.base+n-len(b.buf))...)
			b.msg = b.buf[b.base:]
		}
	}
	return b
}

// SetHeader sets DSO message header.
func (b *Builder) SetHeader(h MsgHeader) *Builder {
	if b.err == nil {
		h.pack(b.msg, 0)
	}
	return b
}

func writeDynamicLenTLV[T TLV](b *Builder, tlv T) (int, error) {
	if b.err != nil {
		return 0, b.err
	}

	// TLV header must fit before payload is attempted.
	tlvHeaderStart := b.off
	tlvHeaderEnd := tlvHeaderStart + TLVHeaderLen
	if len(b.msg) < tlvHeaderEnd {
		b.err = ErrShortWrite
		return 0, b.err
	}

	off, err := tlv.pack(b.msg, tlvHeaderEnd, b.compression)
	if err != nil {
		b.err = err
		return 0, b.err
	}
	if !checkTLVLen(off, b.blockLen, len(b.msg)) {
		b.err = ErrShortWrite
		return 0, b.err
	}

	TLVHeader{tlv.Type(), uint16(off - tlvHeaderEnd)}.pack(b.msg, tlvHeaderStart) // #nosec G115
	b.off = off
	return b.off - tlvHeaderStart, nil
}

// WriteKeepAlive writes [KeepAlive] TLV to underlying buffer.
func (b *Builder) WriteKeepAlive(tlv *KeepAlive) (int, error) {
	if b.err == nil {
		const tlvLen = TLVHeaderLen + KeepAliveLen
		if checkTLVLen(b.off+tlvLen, b.blockLen, len(b.msg)) {
			binary.BigEndian.PutUint32(b.msg[b.off:], uint32(TypeKeepAlive)<<16|KeepAliveLen)
			binary.BigEndian.PutUint64(b.msg[b.off+4:], uint64(tlv.InactivityTimeout)<<32|uint64(tlv.KeepAliveInterval))
			b.off += tlvLen
			return tlvLen, nil
		}
		b.err = ErrShortWrite
	}
	return 0, b.err
}

// WriteRetryDelay writes [RetryDelay] TLV to underlying buffer.
func (b *Builder) WriteRetryDelay(tlv *RetryDelay) (int, error) {
	if b.err == nil {
		const tlvLen = TLVHeaderLen + RetryDelayLen
		if checkTLVLen(b.off+tlvLen, b.blockLen, len(b.msg)) {
			binary.BigEndian.PutUint32(b.msg[b.off:], uint32(TypeRetryDelay)<<16|RetryDelayLen)
			binary.BigEndian.PutUint32(b.msg[b.off+4:], tlv.RetryDelay)
			b.off += tlvLen
			return tlvLen, nil
		}
		b.err = ErrShortWrite
	}
	return 0, b.err
}

// WriteEncryptionPadding writes [EncryptionPadding] TLV to underlying buffer.
func (b *Builder) WriteEncryptionPadding(tlv *EncryptionPadding) (int, error) {
	if b.err == nil {
		tlvLen := TLVHeaderLen + int(tlv.Padding)
		if checkTLVLen(b.off+tlvLen, b.blockLen, len(b.msg)) {
			binary.BigEndian.PutUint32(b.msg[b.off:], uint32(TypeEncryptionPadding)<<16|uint32(tlv.Padding))
			b.off += tlvLen
			return tlvLen, nil
		}
		b.err = ErrShortWrite
	}
	return 0, b.err
}

// WriteSubscribe writes [Subscribe] TLV to underlying buffer.
func (b *Builder) WriteSubscribe(tlv *Subscribe) (int, error) {
	return writeDynamicLenTLV(b, tlv)
}

// WritePush writes [Push] TLV to underlying buffer.
func (b *Builder) WritePush(tlv *Push) (int, error) {
	return writeDynamicLenTLV(b, tlv)
}

// WriteUnsubscribe writes [Unsubscribe] TLV to underlying buffer.
func (b *Builder) WriteUnsubscribe(tlv *Unsubscribe) (int, error) {
	if b.err == nil {
		const tlvLen = TLVHeaderLen + UnsubscribeLen
		if checkTLVLen(b.off+tlvLen, b.blockLen, len(b.msg)) {
			binary.BigEndian.PutUint32(b.msg[b.off:], uint32(TypeUnsubscribe)<<16|UnsubscribeLen)
			binary.BigEndian.PutUint16(b.msg[b.off+4:], tlv.SubscribeID)
			b.off += tlvLen
			return tlvLen, nil
		}
		b.err = ErrShortWrite
	}
	return 0, b.err
}

// WriteReconfirm writes [Reconfirm] TLV to underlying buffer.
func (b *Builder) WriteReconfirm(tlv *Reconfirm) (int, error) {
	return writeDynamicLenTLV(b, tlv)
}

// WriteTLV writes [TLV] to underlying buffer.
func (b *Builder) WriteTLV(tlv TLV) (int, error) {
	switch tlv := tlv.(type) {
	case *KeepAlive:
		return b.WriteKeepAlive(tlv)
	case *RetryDelay:
		return b.WriteRetryDelay(tlv)
	case *EncryptionPadding:
		return b.WriteEncryptionPadding(tlv)
	case *Subscribe:
		return b.WriteSubscribe(tlv)
	case *Push:
		return b.WritePush(tlv)
	case *Unsubscribe:
		return b.WriteUnsubscribe(tlv)
	case *Reconfirm:
		return b.WriteReconfirm(tlv)
	default:
		return writeDynamicLenTLV(b, tlv)
	}
}

// WritePushChange writes [Push] TLV to underlying buffer with as many RRs from change as can fit.
//
// Unlike [Builder.WritePush] it allows to write changes incrementally with non-sticky [PackingError]
// being returned on incomplete write with [PackingError.Index] pointing where to continue.
func (b *Builder) WritePushChange(change []dns.RR) (n int, err error) {
	if b.err != nil {
		return 0, b.err
	}
	if len(change) == 0 {
		return 0, nil
	}
	if len(b.msg) < b.off+TLVHeaderLen {
		return 0, ErrShortWrite
	}
	tlvHeaderStart := b.off
	tlvHeaderEnd := tlvHeaderStart + TLVHeaderLen
	b.off = tlvHeaderEnd
	for i, rr := range change {
		rrOff, rrErr := dns.PackRR(rr, b.msg, b.off, b.compression, b.compression != nil)
		if rrErr != nil || !checkTLVLen(rrOff, b.blockLen, len(b.msg)) {
			// Remove dangling pointers from compression map.
			maps.DeleteFunc(b.compression, func(_ string, off int) bool { return off >= b.off })
			err = &PackingError{i, b.off, rrErr}
			break
		}
		// Packing is successful and padding requirements are satisfied.
		b.off = rrOff
	}
	if err == nil || err.(*PackingError).Index > 0 {
		TLVHeader{TypePush, uint16(b.off - tlvHeaderEnd)}.pack(b.msg, tlvHeaderStart) // #nosec G115
	} else {
		b.off = tlvHeaderStart
	}
	return b.off - tlvHeaderStart, err
}

// Write implements [io.Writer.Write].
// If underlying buffer is too small for entire input, n is 0 and err is set.
func (b *Builder) Write(p []byte) (n int, err error) {
	if b.err == nil {
		if checkTLVLen(b.off+len(p), b.blockLen, len(b.msg)) {
			n = copy(b.msg[b.off:], p)
			b.off += n
		} else {
			b.err = ErrShortWrite
		}
	}
	return n, b.err
}

// WriteTo implements [io.WriterTo.WriteTo].
func (b *Builder) WriteTo(w io.Writer) (int64, error) {
	msg, err := b.Message()
	if err != nil {
		return 0, err
	}
	n, wErr := w.Write(msg)
	return int64(n), wErr
}

// Message returns subslice of underlying buffer with bytes written so far, padded if needed.
//
// [ErrTooLong] is set if length prefix is requested, but message is longer than [MaxMsgLen].
func (b *Builder) Message() ([]byte, error) {
	off := b.off
	if off%b.blockLen != 0 {
		padLen := (b.blockLen - (off+TLVHeaderLen)%b.blockLen) % b.blockLen
		binary.BigEndian.PutUint32(b.msg[off:], uint32(TypeEncryptionPadding)<<16|uint32(padLen)) // #nosec G115
		off += TLVHeaderLen + padLen
	}
	if b.base > 0 {
		if off > MaxMsgLen {
			b.err = ErrTooLong
			binary.BigEndian.PutUint16(b.buf, 0)
		} else {
			binary.BigEndian.PutUint16(b.buf, uint16(off)) // #nosec G115
		}
	}
	return b.buf[:b.base+off], b.err
}

// Available returns how many more bytes can be written.
func (b *Builder) Available() int {
	return len(b.msg) - b.off
}

// Len returns number of written bytes.
func (b *Builder) Len() int {
	return b.off
}

// Cap returns useful capacity of underlying buffer.
func (b *Builder) Cap() int {
	return (cap(b.buf) - b.base) / b.blockLen * b.blockLen
}

// PaddedMsgLen pads msgLen, if needed, up to multiple of blockLen.
func PaddedMsgLen(msgLen int, blockLen uint16) int {
	if msgLen%int(blockLen) == 0 {
		return msgLen
	}
	return (msgLen + TLVHeaderLen + int(blockLen) - 1) / int(blockLen) * int(blockLen)
}
