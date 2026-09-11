package dsomessage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/miekg/dns"
)

func assertWriteResult(tb testing.TB, b *Builder, n int, err error) {
	tb.Helper()

	stickyErr := b.Err()
	if stickyErr != nil && n != 0 {
		tb.Errorf("Got Write() = %v when Err() = %v, want 0", n, stickyErr)
	}
	if stickyErr != nil && err != stickyErr {
		tb.Errorf("Got Write() = %v when Err() = %v, want same", err, stickyErr)
	}
	if n == 0 && stickyErr == nil && err != ErrShortWrite {
		tb.Errorf("Got Write() = %v when n = 0, want ErrShortWrite", err)
	}
	if n != 0 && err != nil {
		tb.Errorf("Got Write() = %v when n > 0, want <nil>", err)
	}
}

func writeBytes(tb testing.TB, b *Builder, l uint16) int {
	tb.Helper()

	if l == 0 {
		return 0
	}

	n, err := b.Write(slices.Repeat([]byte{'x'}, int(l)))
	assertWriteResult(tb, b, n, err)
	return n
}

func writeTLV(tb testing.TB, b *Builder, tlv TLV) int {
	tb.Helper()

	n, err := b.WriteTLV(tlv)
	assertWriteResult(tb, b, n, err)
	return n
}

func TestBuilderAllocs(t *testing.T) {
	var (
		buf     = make([]byte, 128)
		testMsg = testMsg.Clone()
	)
	n := testing.AllocsPerRun(100, func() {
		b := NewBuilder(buf)

		testMsg.PackTo(b)
		if err := b.Err(); err != nil {
			t.Errorf("Got %v, want to pack", err)
		}

		msgBytes, _ := b.Message()
		if got, want := len(msgBytes), len(testMsgBytes); got != want {
			t.Errorf("Got message of size %v, want %v", got, want)
		}
	})
	if n != 0 {
		t.Errorf("Got %v allocations, want none", n)
	}
}

func TestNewBuilderClearsHeader(t *testing.T) {
	t.Parallel()

	b := NewBuilder(slices.Repeat([]byte{42}, MsgHeaderLen))

	msgBytes, _ := b.Message()
	if slices.Contains(msgBytes, 42) {
		t.Error("Want NewBuilder() to clear HeaderLen bytes")
	}
	if msgBytes[2]>>3 != dns.OpcodeStateful {
		t.Error("Expected OpcodeStateful")
	}
}

func TestNewBuilderPanics(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Want NewBuilder() to panic when buffer is too short")
		}
	}()

	NewBuilder(make([]byte, MsgHeaderLen-1))
}

func TestBuilderStickyError(t *testing.T) {
	t.Parallel()

	b := NewBuilder(make([]byte, 128))
	b.Write(make([]byte, 256))

	if b.Err() == nil {
		t.Fatal("Want Err() to be set")
	}

	b.EnableLengthPrefix()
	if b.Err() == nil {
		t.Fatal("Expected error to stick")
	}
	if b.base != 0 {
		t.Error("Expected EnableLengthPrefix() to have no effect")
	}

	b.EnablePadding(32)
	if b.Err() == nil {
		t.Fatal("Expected error to stick")
	}
	if b.blockLen != 1 {
		t.Error("Expected EnablePadding() to have no effect")
	}

	b.EnableCompression()
	if b.Err() == nil {
		t.Fatal("Expected error to stick")
	}
	if b.compression != nil {
		t.Error("Expected EnableCompression() to have no effect")
	}

	n, err := b.Write(make([]byte, 42))
	if b.Err() == nil {
		t.Fatal("Expected error to stick")
	}
	if n != 0 || err == nil || b.off != MsgHeaderLen {
		t.Error("Expected Write() to have no effect")
	}

	for _, tlv := range testMsg.Clone().TLV {
		n, err := b.WriteTLV(tlv)
		if b.Err() == nil {
			t.Fatal("Expected error to stick")
		}
		if n != 0 || err == nil || b.off != MsgHeaderLen {
			t.Error("Expected Write() to have no effect")
		}
	}

	n, err = b.WritePushChange([]dns.RR{aRRf("test. IN A 192.0.2.1")})
	if b.Err() == nil {
		t.Fatal("Expected error to stick")
	}
	if n != 0 || err == nil || b.off != MsgHeaderLen {
		t.Error("Expected WritePushChanges() to have no effect")
	}
}

func TestBuilderReset(t *testing.T) {
	t.Parallel()

	b := NewBuilder(make([]byte, 128)).
		EnableLengthPrefix().
		EnablePadding(32).
		EnableCompression().
		SetHeader(MsgHeader{42, true, dns.RcodeServerFailure})
	b.Write(make([]byte, 42))
	b.Write(make([]byte, 256))

	b.Reset()

	if b.Len() != MsgHeaderLen {
		t.Errorf("Got Len() = %v, want HeaderLen", b.Len())
	}
	msgBytes, _ := b.Message()
	if msgLen := len(msgBytes); msgLen != MsgHeaderLen {
		t.Errorf("Got len(Message()) = %v, want HeaderLen", msgLen)
	}
	if !slices.Equal(msgBytes[:MsgHeaderLen], zeroMsgHeader) {
		t.Error(cmp.Diff(msgBytes, nil))
	}
	if b.compression != nil {
		t.Error("Want Reset() to disable compression")
	}
	if b.Err() != nil {
		t.Error("Want Reset() to clear error")
	}
}

func TestBuilderClear(t *testing.T) {
	t.Parallel()

	header := MsgHeader{42, true, dns.RcodeServerFailure}
	headerBytes := make([]byte, MsgHeaderLen)
	header.pack(headerBytes, 0)

	blockLen := 32
	b := NewBuilder(make([]byte, 128)).
		EnableLengthPrefix().
		EnablePadding(uint16(blockLen)).
		EnableCompression().
		SetHeader(header)
	b.Write(make([]byte, 42))
	b.Write(make([]byte, 256))

	b.Clear()

	if b.Len() != MsgHeaderLen {
		t.Errorf("Got Len() = %v, want HeaderLen", b.Len())
	}
	msgBytes, _ := b.Message()
	if msgLen := len(msgBytes); msgLen != LengthPrefixLen+blockLen {
		t.Errorf("Got len(Message()) = %v, want %v", msgLen, LengthPrefixLen+blockLen)
	}
	if !slices.Equal(msgBytes[LengthPrefixLen:LengthPrefixLen+MsgHeaderLen], headerBytes) {
		t.Error(cmp.Diff(msgBytes, nil))
	}
	if b.compression == nil {
		t.Error("Want Clear() to keep compression")
	}
	if len(b.compression) != 0 {
		t.Error("Want Clear() to clear compression")
	}
	if b.Err() != nil {
		t.Error("Want Clear() to clear error")
	}
}

func TestBuilderEnableLengthPrefix(t *testing.T) {
	t.Parallel()

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()

		b := NewBuilder(make([]byte, 128))
		n, _ := b.WriteKeepAlive(&KeepAlive{})

		wireLen := MsgHeaderLen + n
		if b.Len() != wireLen {
			t.Errorf("Got Len() = %v, want %v", b.Len(), wireLen)
		}

		msgBytes, _ := b.Message()
		if len(msgBytes) != wireLen {
			t.Errorf("Got len(Message()) %v, want %v", len(msgBytes), wireLen)
		}
	})

	t.Run("enabled", func(t *testing.T) {
		t.Parallel()

		b := NewBuilder(make([]byte, 128)).EnableLengthPrefix()
		n, _ := b.WriteKeepAlive(&KeepAlive{})

		msgLen := MsgHeaderLen + n
		if b.Len() != msgLen {
			t.Errorf("Got Len() = %v, want %v", b.Len(), msgLen)
		}

		wireLen := LengthPrefixLen + msgLen
		msgBytes, _ := b.Message()
		if len(msgBytes) != wireLen {
			t.Errorf("Got len(Message()) = %v, want %v", len(msgBytes), wireLen)
		}
		if got := int(binary.BigEndian.Uint16(msgBytes)); got != msgLen {
			t.Errorf("Got length prefix %v, want %v", got, msgLen)
		}
	})
}

func TestBuilderEnableLengthPrefixError(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name     string
		bufLen   int
		blockLen uint16
		err      error
	}{
		{
			"unpadded",
			LengthPrefixLen + MsgHeaderLen - 1,
			1,
			ErrBufferFull,
		},
		{
			"padded",
			LengthPrefixLen + MsgHeaderLen + TLVHeaderLen - 1,
			MsgHeaderLen + TLVHeaderLen,
			ErrBufferFull,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := NewBuilder(make([]byte, tc.bufLen)).EnablePadding(tc.blockLen)

			if err := b.Err(); err != nil {
				t.Fatalf("Got Err() = %v, want to set up builder", err)
			}

			b.EnableLengthPrefix()
			if err := b.Err(); err != tc.err {
				t.Errorf("Got Err() = %v, want %v", err, tc.err)
			}
		})
	}
}

func TestBuilderLengthPrefixOverflow(t *testing.T) {
	t.Parallel()

	b := NewBuilder(make([]byte, 2*MaxMsgLen)).EnableLengthPrefix()

	b.Write(make([]byte, MaxMsgLen))
	if b.Err() != nil {
		t.Fatalf("Got Err() = %v, want <nil>", b.Err())
	}

	msgBytes, err := b.Message()
	if want := LengthPrefixLen + MsgHeaderLen + MaxMsgLen; len(msgBytes) != want {
		t.Errorf("Got len(Message()) = %v, want %v", len(msgBytes), want)
	}
	if err != ErrTooLong {
		t.Errorf("Got Message() = %v, want ErrOverflow", err)
	}
	if got := binary.BigEndian.Uint16(msgBytes); got != 0 {
		t.Errorf("Got packed length %v, want 0", got)
	}
}

func TestBuilderEnablePadding(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name       string
		bufLen     int
		blockLen   uint16
		writeLen   int
		writtenLen int
		wireLen    int
	}{
		{
			"disabled",
			128,
			1,
			24,
			MsgHeaderLen + 24,
			MsgHeaderLen + 24,
		},
		{
			"enabled",
			128,
			128,
			24,
			MsgHeaderLen + 24,
			128,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := NewBuilder(make([]byte, tc.bufLen)).EnablePadding(tc.blockLen)
			b.Write(make([]byte, tc.writeLen))

			if b.Len() != tc.writtenLen {
				t.Errorf("Got Len() = %v, want %v", b.Len(), tc.writtenLen)
			}

			msgBytes, _ := b.Message()
			if len(msgBytes) != tc.wireLen {
				t.Errorf("Got len(Message()) = %v, want %v", len(msgBytes), tc.wireLen)
			}
		})
	}
}

func TestBuilderEnablePaddingError(t *testing.T) {
	t.Parallel()

	t.Run("zero", func(t *testing.T) {
		t.Parallel()

		defer func() {
			if r := recover(); r == nil {
				t.Error("Want EnablePadding() to panic when blockLen is 0")
			}
		}()

		NewBuilder(make([]byte, MsgHeaderLen)).EnablePadding(0)
	})

	tcs := []struct {
		name      string
		bufLen    int
		lenPrefix bool
		blockLen  uint16
		err       error
	}{
		{
			"unprefixed",
			MsgHeaderLen,
			false,
			MsgHeaderLen + 1,
			ErrBufferFull,
		},
		{
			"prefixed",
			LengthPrefixLen + MsgHeaderLen,
			true,
			MsgHeaderLen + 1,
			ErrBufferFull,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := NewBuilder(make([]byte, tc.bufLen))
			if tc.lenPrefix {
				b.EnableLengthPrefix()
			}
			b.EnablePadding(tc.blockLen)

			if err := b.Err(); err != tc.err {
				t.Errorf("Got Err() = %v, want %v", err, tc.err)
			}
		})
	}
}

func TestBuilderSizing(t *testing.T) {
	tcs := []struct {
		name          string
		bufLen        int
		lenPrefix     bool
		blockLen      uint16
		wantAvailable int
		wantLen       int
		wantCap       int
	}{
		{
			"unprefixed - unpadded",
			100,
			false,
			1,
			100 - MsgHeaderLen,
			MsgHeaderLen,
			100,
		},
		{
			"prefixed - unpadded",
			100,
			true,
			1,
			100 - LengthPrefixLen - MsgHeaderLen,
			MsgHeaderLen,
			100 - LengthPrefixLen,
		},
		{
			"unprefixed - padded",
			100,
			false,
			11,
			(100 / 11 * 11) - MsgHeaderLen, //(MsgHeaderLen+TLVHeaderLen)/(TLVHeaderLen-1)*(TLVHeaderLen-1) - MsgHeaderLen,
			MsgHeaderLen,
			100 / 11 * 11,
		},
		{
			"prefixed - padded",
			100,
			true,
			11,
			(100-LengthPrefixLen)/11*11 - MsgHeaderLen,
			MsgHeaderLen,
			(100 - LengthPrefixLen) / 11 * 11,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := NewBuilder(make([]byte, tc.bufLen)).
				EnablePadding(tc.blockLen)
			if tc.lenPrefix {
				b.EnableLengthPrefix()
			}

			if b.Available() != tc.wantAvailable {
				t.Errorf("Got Availale() = %v, want %v", b.Available(), tc.wantAvailable)
			}
			if b.Len() != tc.wantLen {
				t.Errorf("Got Len() = %v, want %v", b.Len(), tc.wantLen)
			}
			if b.Cap() != tc.wantCap {
				t.Errorf("Got Cap() = %v, want %v", b.Cap(), tc.wantCap)
			}
		})
	}
}

func TestBuilderEnableCompression(t *testing.T) {
	t.Parallel()

	b := NewBuilder(make([]byte, 128)).EnableCompression()

	n1, _ := b.WriteSubscribe(&Subscribe{"a.test.", dns.TypeA, dns.ClassINET})
	n2, _ := b.WriteSubscribe(&Subscribe{"a.test.", dns.TypeA, dns.ClassINET})
	if n1 <= n2 {
		t.Fatal("Want second write compressed")
	}
}

func TestBuilderWriteOverflow(t *testing.T) {
	t.Parallel()

	b := NewBuilder(make([]byte, 128))

	_, err := b.Write(make([]byte, b.Available()))
	if err != nil {
		t.Fatalf("Got Write(Available()) = %v, want <nil>", err)
	}

	_, err = b.Write([]byte{42})
	if err != ErrShortWrite {
		t.Errorf("Got Write() = %v, want ErrShortWrite", err)
	}
}

func TestBuilderWriteTLVOverflow(t *testing.T) {
	t.Parallel()

	for _, tlv := range testMsg.Clone().TLV {
		tcs := []struct {
			name     string
			bufLen   int
			writeLen int
		}{
			{
				tlv.Type().String(),
				128,
				128 - MsgHeaderLen - TLVHeaderLen,
			},
			{
				tlv.Type().String() + " header",
				128,
				128 - MsgHeaderLen,
			},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				b := NewBuilder(make([]byte, tc.bufLen))

				_, err := b.Write(make([]byte, tc.writeLen))
				if err != nil {
					t.Fatalf("Got Write(Available()) = %v, want <nil>", err)
				}

				b.WriteTLV(tlv)
				ok := errors.Is(b.Err(), ErrShortWrite)
				if !ok {
					_, ok = errors.AsType[*PackingError](err)
				}
				if !ok {
					t.Errorf("Got WriteTLV() = %v, want ErrShortWrite or PackingError", err)
				}
			})
		}
	}
}

func TestBuilderWriteAtPaddingBoundary(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name     string
		bufLen   int
		blockLen uint16
		writeLen int
		err      error
	}{
		{
			"before",
			128,
			128,
			128 - MsgHeaderLen - TLVHeaderLen - 1,
			nil,
		},
		{
			"at start",
			128,
			128,
			128 - MsgHeaderLen - TLVHeaderLen,
			nil,
		},
		{
			"within",
			128,
			128,
			128 - MsgHeaderLen - TLVHeaderLen + 1,
			ErrShortWrite,
		},
		{
			"at end",
			128,
			128,
			128 - MsgHeaderLen,
			nil,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBuilder(make([]byte, tc.bufLen)).EnablePadding(tc.blockLen)

			b.Write(make([]byte, tc.writeLen))
			if err := b.Err(); err != tc.err {
				t.Fatalf("Got Write() = %v, want %v", err, tc.err)
			}
		})
	}
}

func TestBuilderWriteTLVAtPaddingBoundary(t *testing.T) {
	t.Parallel()

	for _, tlv := range testMsg.Clone().TLV {
		dsoLen := func() int {
			b := NewBuilder(make([]byte, 128))
			n, _ := b.WriteTLV(tlv)
			return n
		}()
		tcs := []struct {
			name     string
			bufLen   int
			writeLen int
			blockLen uint16
			err      error
		}{
			{
				tlv.Type().String() + " before",
				128,
				0,
				128,
				nil,
			},
			{
				tlv.Type().String() + " at start",
				128,
				128 - MsgHeaderLen - dsoLen - TLVHeaderLen,
				128,
				nil,
			},
			{
				tlv.Type().String() + " within",
				128,
				128 - MsgHeaderLen - dsoLen - TLVHeaderLen + 1,
				128,
				ErrShortWrite,
			},
			{
				tlv.Type().String() + " at end",
				128,
				128 - MsgHeaderLen - dsoLen,
				128,
				nil,
			},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				b := NewBuilder(make([]byte, tc.bufLen)).EnablePadding(tc.blockLen)

				b.Write(make([]byte, tc.writeLen))
				if err := b.Err(); err != nil {
					t.Fatalf("Got Write() = %v, want <nil>", err)
				}

				b.WriteTLV(tlv)
				err := b.Err()
				if _, ok := errors.AsType[*PackingError](err); ok {
					t.Skipf("%v is dynamic length TLV that needs more space to pack", tlv.Type())
				}
				if err != tc.err {
					t.Errorf("Got WriteTLV() = %v, want %v", err, tc.err)
				}
			})
		}
	}
}

func TestBuilderWritePushChange(t *testing.T) {
	t.Parallel()

	var (
		rr     = aRRf("test. IN A 192.0.2.1")
		rrLen  = RRLen(rr)
		change = slices.Repeat([]dns.RR{rr}, 10)
	)
	tcs := []struct {
		name      string
		bufLen    int
		lenPrefix bool
		blockLen  int
	}{
		{
			"unprefixed unpadded",
			MsgHeaderLen + TLVHeaderLen + 5*rrLen,
			false,
			1,
		},
		{
			"prefixed",
			LengthPrefixLen + MsgHeaderLen + TLVHeaderLen + 5*rrLen,
			true,
			1,
		},
		{
			"padded",
			MsgHeaderLen + TLVHeaderLen + 6*rrLen + 1,
			false,
			MsgHeaderLen + TLVHeaderLen + 6*rrLen + 1, // cannot fit 6th RR and maintain padding
		},
		{
			"prefixed padded",
			LengthPrefixLen + MsgHeaderLen + TLVHeaderLen + 6*rrLen + 1,
			true,
			MsgHeaderLen + TLVHeaderLen + 6*rrLen + 1,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBuilder(make([]byte, tc.bufLen))
			if tc.lenPrefix {
				b.EnableLengthPrefix()
			}
			b.EnablePadding(uint16(tc.blockLen))

			_, err := b.WritePushChange(change)
			if got := err.(*PackingError).Index; got != 5 {
				t.Errorf("Got PackingError.Index = %v, want 5", got)
			}
			if b.Err() != nil {
				t.Errorf("Got Err() = %v, want <nil>", b.Err())
			}

			b.Clear()

			_, err = b.WritePushChange(change[5:])
			if err != nil {
				t.Errorf("Got WritePushChange() = %v, want <nil>", err)
			}
			if b.Err() != nil {
				t.Errorf("Got Err() = %v, want <nil>", b.Err())
			}
		})
	}

	t.Run("empty", func(t *testing.T) {
		b := NewBuilder(make([]byte, 128))
		n, err := b.WritePushChange(nil)
		if n != 0 {
			t.Errorf("Got WritePushChange() = %v, want 0", n)
		}
		if err != nil {
			t.Errorf("Got WritePushChange() = %v, want <nil>", err)
		}
	})

	t.Run("too small", func(t *testing.T) {
		b := NewBuilder(make([]byte, MsgHeaderLen+TLVHeaderLen))
		n, err := b.WritePushChange([]dns.RR{rr})
		if n != 0 {
			t.Errorf("Got WritePushChange() = %v, want 0", n)
		}
		if err.(*PackingError).Index != 0 {
			t.Errorf("Got WritePushChange() = %v, want PackingError{Index:0}", err)
		}
		if b.Err() != nil {
			t.Errorf("Got Err() = %v, want <nil>", b.Err())
		}
	})
}

func TestBuilderWriteTo(t *testing.T) {
	b := NewBuilder(make([]byte, 128))
	testMsg.Clone().PackTo(b)
	if b.Err() != nil {
		t.Fatal("Want to pack testMsg")
	}

	var buf bytes.Buffer
	_, err := b.WriteTo(&buf)
	if err != nil {
		t.Errorf("Got WriteTo() = %v, want <nil>", err)
	}
	if !slices.Equal(buf.Bytes(), testMsgBytes) {
		t.Error(cmp.Diff(testMsgBytes, buf.Bytes()))
	}
}

func TestBuilderGrow(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name      string
		bufLen    int
		blockLen  int
		growLen   int
		err       error
		available int
	}{
		{
			"len",
			128,
			1,
			1,
			nil,
			128 - MsgHeaderLen,
		},
		{
			"padded len",
			128,
			128,
			1,
			nil,
			128 - MsgHeaderLen,
		},
		{
			"realloc",
			128,
			1,
			128,
			nil,
			128,
		},
		{
			"padded realloc",
			128,
			128,
			128,
			nil,
			128*2 - MsgHeaderLen,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBuilder(make([]byte, tc.bufLen)).EnablePadding(uint16(tc.blockLen))

			if err := b.Err(); err != nil {
				t.Fatalf("Got Err() = %v, want <nil>", err)
			}

			b.Grow(tc.growLen)
			if b.Err() != tc.err {
				t.Errorf("Got Err() = %v, want %v", b.Err(), tc.err)
			}
			if b.Available() != tc.available {
				t.Errorf("Got Available() = %v, want %v", b.Available(), tc.available)
			}
		})
	}
}

func TestBuilderGrowAllocs(t *testing.T) {
	tcs := []struct {
		name    string
		buf     []byte
		growLen int
		alloc   bool
	}{
		{
			"len",
			make([]byte, 128),
			100,
			false,
		},
		{
			"cap",
			make([]byte, 128, 512),
			256,
			false,
		},
		{
			"realloc",
			make([]byte, 128, 512),
			1024,
			true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			n := testing.AllocsPerRun(100, func() {
				b := NewBuilder(tc.buf)

				b.Grow(tc.growLen)
				if err := b.Err(); err != nil {
					t.Fatalf("Got Grow().Err() = %v, want <nil>", err)
				}
			})
			if n == 0 && tc.alloc {
				t.Error("Got no allocations, want some")
			} else if n != 0 && !tc.alloc {
				t.Errorf("Got %v allocations, want none", n)
			}
		})
	}
}

func TestBuilderGrowNegativePanics(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Want to panic")
		}
	}()

	NewBuilder(make([]byte, 128)).Grow(-1)
}

func FuzzBuilder(f *testing.F) {
	TLVs := testMsg.Clone().TLV

	f.Fuzz(func(t *testing.T,
		bufLen uint16,

		writeInitialLen uint16,

		lenPrefix bool,
		compress bool,
		blockLen uint16,

		writeTLVSeq []byte,

		growLen uint16,
		writeAfterGrowLen uint16,
	) {
		if blockLen == 0 {
			t.SkipNow()
		}
		if bufLen < MsgHeaderLen {
			t.SkipNow()
		}
		if len(writeTLVSeq) > 4096 {
			t.SkipNow()
		}

		var (
			builder = NewBuilder(make([]byte, bufLen))
			written = builder.Len()
		)

		written += writeBytes(t, builder, writeInitialLen)

		if lenPrefix {
			builder.EnableLengthPrefix()
		}
		if compress {
			builder.EnableCompression()
		}
		builder.EnablePadding(blockLen)

		for _, n := range writeTLVSeq {
			tlv := TLVs[int(n)%len(TLVs)]
			written += writeTLV(t, builder, tlv)
		}

		builder.Grow(int(growLen))

		written += writeBytes(t, builder, writeAfterGrowLen)

		if builder.Len() != written {
			t.Errorf("Got Len() = %v, want %v", builder.Len(), written)
		}

		_, _ = builder.Message()
	})
}

func FuzzBuilderWritePushChange(f *testing.F) {
	WordList := []string{"apple", "banana", "cherry", "date", "elderberry", "fig", "grape"}

	f.Fuzz(func(t *testing.T,
		bufLen uint16,

		compress bool,
		blockLen uint16,

		writeRRSeq []byte,
	) {
		if blockLen == 0 {
			t.SkipNow()
		}
		if bufLen < MsgHeaderLen {
			t.SkipNow()
		}
		if bufLen < uint16(PaddedMsgLen(MsgHeaderLen, blockLen)) {
			t.SkipNow()
		}
		if len(writeRRSeq) == 0 || len(writeRRSeq) > 4096 {
			t.SkipNow()
		}

		builder := NewBuilder(make([]byte, bufLen))

		if compress {
			builder.EnableCompression()
		}
		builder.EnablePadding(blockLen)

		var (
			pcg = rand.NewPCG(uint64(writeRRSeq[0]), 0)
			r   = rand.New(pcg)

			change = make([]dns.RR, len(writeRRSeq))
		)
		for i, n := range writeRRSeq {
			var b strings.Builder
			for range 4 {
				b.WriteString(WordList[r.Int()%len(WordList)])
				b.WriteByte('.')
			}
			b.WriteString("test")
			domain := b.String()

			rdata := string(slices.Repeat([]byte{'x'}, int(n)+1)) // ensure non-empty rdata
			change[i] = txtRRf("%s IN TXT %s", domain, rdata)
		}

		var (
			msgs [][]byte

			i     int
			retry bool
		)

		for i < len(change) {
			n, err := builder.WritePushChange(change[i:])

			switch {
			case builder.Err() != nil:
				t.Fatalf("Got Err() = %v, want <nil>", builder.Err())
			case n == 0 && retry:
				t.Fatalf("Want to write RR after Grow()")
			case n == 0:
				builder.Grow(TLVHeaderLen + RRLen(change[i]))
				retry = true
				continue
			case err != nil:
				i += err.(*PackingError).Index
			default:
				i = len(change)
			}

			msgBytes, _ := builder.Message()
			msgs = append(msgs, slices.Clone(msgBytes))
			builder.Clear()
			retry = false
		}

		var (
			parser    Parser
			gotChange = make([]dns.RR, 0, len(writeRRSeq))
		)
		for _, m := range msgs {
			parser.Start(m, OriginServer)
			parser.TLVHeader()
			tlv, err := parser.Push()
			if err != nil {
				t.Fatalf("Got Push() = %v, want <nil>", err)
			}
			gotChange = append(gotChange, tlv.Change...)
		}

		if !slices.EqualFunc(gotChange, change, dns.IsDuplicate) {
			t.Error(cmp.Diff(change, gotChange, cmp.Comparer(dns.IsDuplicate)))
		}
	})
}

func BenchmarkBuilder(b *testing.B) {
	for _, tlv := range testMsg.Clone().TLV {
		b.Run(tlv.Type().String(), func(b *testing.B) {
			buf := make([]byte, 128)
			for b.Loop() {
				builder := NewBuilder(buf)
				builder.WriteTLV(tlv)
				_, _ = builder.Message()
			}
		})
	}
}
