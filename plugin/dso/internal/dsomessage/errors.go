package dsomessage

import (
	"fmt"
	"strconv"
)

var (
	// ErrFormat indicates that DSO message content is invalid.
	ErrFormat = fmt.Errorf("dsomessage: bad message format")

	// ErrHeader indicates that DSO message header is invalid.
	ErrHeader = fmt.Errorf("%w - bad header", ErrFormat)

	// ErrTLV indicates that TLV's content or usage is invalid.
	ErrTLV = fmt.Errorf("%w - bad TLV", ErrFormat)

	// ErrBufferFull indicates that there is not enough space in buffer
	// to accomodate change in message layout.
	ErrBufferFull = fmt.Errorf("dsomessage.Builder: short buffer")

	// ErrShortWrite indicates that write required longer buffer than was supplied.
	ErrShortWrite = fmt.Errorf("dsomessage.Builder: short write")

	// ErrTooLong indicates that message is too long to set length prefix.
	ErrTooLong = fmt.Errorf("dsomessage.Builder: msg too long")

	// ErrDone indicates that all TLVs have been parsed.
	ErrDone = fmt.Errorf("dsomessage.Parser: done")
)

type (
	// PackingError wraps [dns.PackRR] error (available via [PackingError.Unwrap]) with offset and index of RR that failed to pack.
	PackingError struct {
		Index  int // index of RR that failed to pack; -1 if method packs exactly one RR
		Offset int // offset where packing halted
		cause  error
	}

	// UnpackingError wraps [dns.UnpackRR] error (available via [UnpackingError.Unwrap]) with offset where unpacking halted.
	UnpackingError struct {
		Offset int // offset where unpacking halted
		cause  error
	}
)

func (e *PackingError) Error() string {
	if e.Index >= 0 {
		return "failed to pack RR at index " + strconv.Itoa(e.Index) + " at byte " + strconv.Itoa(e.Offset) + " - " + e.cause.Error()
	}
	return "failed to pack RR at byte " + strconv.Itoa(e.Offset) + " - " + e.cause.Error()
}

func (e *PackingError) Unwrap() error { return e.cause }

func (e *UnpackingError) Error() string {
	return "failed to unpack RR at byte " + strconv.Itoa(e.Offset) + " - " + e.cause.Error()
}

func (e *UnpackingError) Unwrap() error { return e.cause }

var (
	errUsage             = fmt.Errorf("%w - invalid usage context", ErrTLV)
	errMalformed         = fmt.Errorf("%w - malformed", ErrTLV)
	errBadKeepAlive      = fmt.Errorf("%w - bad keepalive interval", ErrTLV)
	errEmptyPushChange   = fmt.Errorf("%w - empty push tlv", ErrTLV)
	errBadPushCollective = fmt.Errorf("%w - non-empty collective removal in push tlv", ErrTLV)
)
