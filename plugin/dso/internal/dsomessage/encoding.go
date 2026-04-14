package dsomessage

// AppendBinary implements [encoding.BinaryAppender].
func (h MsgHeader) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, make([]byte, MsgHeaderLen)...)
	h.pack(b, len(b)-MsgHeaderLen)
	return b, nil
}

// MarshalBinary implements [encoding.BinaryMarshaler].
func (h MsgHeader) MarshalBinary() (data []byte, err error) {
	return h.AppendBinary(nil)
}

// UnmarshalBinary implements [encoding.BinaryUnmarshaler].
func (h *MsgHeader) UnmarshalBinary(data []byte) (err error) {
	if len(data) != MsgHeaderLen {
		return errMalformed
	}
	*h, _ = unpackMsgHeader(data, 0)
	return nil
}

// AppendBinary implements [encoding.BinaryAppender].
func (h TLVHeader) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, make([]byte, TLVHeaderLen)...)
	h.pack(b, len(b)-TLVHeaderLen)
	return b, nil
}

// MarshalBinary implements [encoding.BinaryMarshaler].
func (h TLVHeader) MarshalBinary() (data []byte, err error) {
	return h.AppendBinary(nil)
}

// UnmarshalBinary implements [encoding.BinaryUnmarshaler].
func (h *TLVHeader) UnmarshalBinary(data []byte) error {
	if len(data) != TLVHeaderLen {
		return errMalformed
	}
	*h, _ = unpackTLVHeader(data, 0)
	return nil
}

// AppendBinary implements [encoding.BinaryAppender].
func (tlv *KeepAlive) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, make([]byte, KeepAliveLen)...)
	tlv.pack(b, len(b)-KeepAliveLen, nil)
	return b, nil
}

// MarshalBinary implements [encoding.BinaryMarshaler].
func (tlv *KeepAlive) MarshalBinary() ([]byte, error) {
	return tlv.AppendBinary(nil)
}

// UnmarshalBinary implements [encoding.BinaryUnmarshaler].
func (tlv *KeepAlive) UnmarshalBinary(data []byte) error {
	if len(data) > MaxMsgLen {
		return errMalformed
	}
	_, err := tlv.unpack(data, 0, uint16(len(data)))
	if err != nil {
		return err
	}
	return nil
}

// AppendBinary implements [encoding.BinaryAppender].
func (tlv *RetryDelay) AppendBinary(b []byte) (b1 []byte, err error) {
	b = append(b, make([]byte, RetryDelayLen)...)
	tlv.pack(b, len(b)-RetryDelayLen, nil)
	return b, nil
}

// MarshalBinary implements [encoding.BinaryMarshaler].
func (tlv *RetryDelay) MarshalBinary() ([]byte, error) {
	return tlv.AppendBinary(nil)
}

// UnmarshalBinary implements [encoding.BinaryUnmarshaler].
func (tlv *RetryDelay) UnmarshalBinary(data []byte) error {
	if len(data) > MaxMsgLen {
		return errMalformed
	}
	_, err := tlv.unpack(data, 0, uint16(len(data)))
	if err != nil {
		return err
	}
	return nil
}

// AppendBinary implements [encoding.BinaryAppender].
func (tlv *EncryptionPadding) AppendBinary(b []byte) (b1 []byte, err error) {
	b = append(b, make([]byte, tlv.Padding)...)
	tlv.pack(b, len(b)-int(tlv.Padding), nil)
	return b, nil
}

// MarshalBinary implements [encoding.BinaryMarshaler].
func (tlv *EncryptionPadding) MarshalBinary() ([]byte, error) {
	return tlv.AppendBinary(nil)
}

// UnmarshalBinary implements [encoding.BinaryUnmarshaler].
func (tlv *EncryptionPadding) UnmarshalBinary(data []byte) error {
	if len(data) > MaxMsgLen {
		return errMalformed
	}
	_, err := tlv.unpack(data, 0, uint16(len(data)))
	if err != nil {
		return err
	}
	return nil
}

// AppendBinary implements [encoding.BinaryAppender] by writing uncompressed wire representation.
func (tlv *Subscribe) AppendBinary(b []byte) (b1 []byte, err error) {
	b1 = b[:cap(b)]
	off, err := tlv.pack(b1, len(b), nil)
	if _, ok := err.(*PackingError); ok {
		l := len(b) - len(b1) + tlv.Len()
		if l <= 0 {
			return nil, err
		}
		b1 = append(b1, make([]byte, l)...)

		off, err = tlv.pack(b1, len(b), nil)
	}
	if err != nil {
		return nil, err
	}
	return b1[:off], nil
}

// MarshalBinary implements [encoding.BinaryMarshaler] by writing uncompressed wire representation.
func (tlv *Subscribe) MarshalBinary() ([]byte, error) {
	return tlv.AppendBinary(nil)
}

// UnmarshalBinary implements [encoding.BinaryUnmarshaler] for uncompressed wire representation.
func (tlv *Subscribe) UnmarshalBinary(data []byte) error {
	if len(data) > MaxMsgLen {
		return errMalformed
	}
	_, err := tlv.unpack(data, 0, uint16(len(data)))
	if err != nil {
		return err
	}
	return nil
}

// AppendBinary implements [encoding.BinaryAppender] by writing uncompressed wire representation.
func (tlv *Push) AppendBinary(b []byte) (b1 []byte, err error) {
	b1 = b[:cap(b)]
	off, err := tlv.pack(b1, len(b), nil)
	if packErr, ok := err.(*PackingError); ok {
		// Continue where packing halted.
		tlv.Change = tlv.Change[packErr.Index:]

		l := packErr.Offset - len(b1)
		for _, rr := range tlv.Change {
			l += RRLen(rr)
		}
		if l <= 0 {
			return nil, packErr
		}
		b1 = append(b1, make([]byte, l)...)

		off, err = tlv.pack(b1, packErr.Offset, nil)
		if packErr1, ok := err.(*PackingError); ok {
			packErr1.Index += packErr.Index // adjust error's index for caller
		}
	}
	if err != nil {
		return nil, err
	}
	return b1[:off], nil
}

// MarshalBinary implements [encoding.BinaryMarshaler] by writing uncompressed wire representation.
func (tlv *Push) MarshalBinary() ([]byte, error) {
	return tlv.AppendBinary(nil)
}

// UnmarshalBinary implements [encoding.BinaryUnmarshaler] for uncompressed wire representation.
func (tlv *Push) UnmarshalBinary(data []byte) error {
	if len(data) > MaxMsgLen {
		return errMalformed
	}
	_, err := tlv.unpack(data, 0, uint16(len(data)))
	if err != nil {
		return err
	}
	return nil
}

// AppendBinary implements [encoding.BinaryAppender].
func (tlv *Unsubscribe) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, make([]byte, UnsubscribeLen)...)
	tlv.pack(b, len(b)-UnsubscribeLen, nil)
	return b, nil
}

// MarshalBinary implements [encoding.BinaryMarshaler].
func (tlv *Unsubscribe) MarshalBinary() ([]byte, error) {
	return tlv.AppendBinary(nil)
}

// UnmarshalBinary implements [encoding.BinaryUnmarshaler].
func (tlv *Unsubscribe) UnmarshalBinary(data []byte) error {
	if len(data) > MaxMsgLen {
		return errMalformed
	}
	_, err := tlv.unpack(data, 0, uint16(len(data)))
	if err != nil {
		return err
	}
	return nil
}

// AppendBinary implements [encoding.BinaryAppender] by writing uncompressed wire representation.
func (tlv *Reconfirm) AppendBinary(b []byte) (b1 []byte, err error) {
	b1 = b[:cap(b)]
	off, err := tlv.pack(b1, len(b), nil)
	if _, ok := err.(*PackingError); ok {
		l := len(b) - len(b1) + tlv.Len()
		if l <= 0 {
			return b, err
		}
		b1 = append(b1, make([]byte, l)...)

		off, err = tlv.pack(b1, len(b), nil)
	}
	if err != nil {
		return nil, err
	}
	return b1[:off], nil
}

// MarshalBinary implements [encoding.BinaryMarshaler] by writing uncompressed wire representation.
func (tlv *Reconfirm) MarshalBinary() ([]byte, error) {
	return tlv.AppendBinary(nil)
}

// UnmarshalBinary implements [encoding.BinaryUnmarshaler] for uncompressed wire representation.
func (tlv *Reconfirm) UnmarshalBinary(data []byte) error {
	if len(data) > MaxMsgLen {
		return errMalformed
	}
	_, err := tlv.unpack(data, 0, uint16(len(data)))
	if err != nil {
		return err
	}
	return nil
}

func (m *Msg) AppendBinary(b []byte) ([]byte, error) {
	data, err := m.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return append(b, data...), nil
}

func (m *Msg) MarshalBinary() ([]byte, error) {
	builder := NewBuilder(make([]byte, m.Len()))
	m.PackTo(builder)
	data, err := builder.Message()
	if err != nil {
		return nil, err
	}
	return data, err
}

func (m *Msg) UnmarshalBinary(data []byte) error {
	m1, err := UnpackMsg(data, OriginInvalid)
	if err != nil {
		return err
	}
	*m = *m1
	return nil
}
