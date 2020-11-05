package dnstap

import (
	"io"
	"time"

	tap "github.com/dnstap/golang-dnstap"
	fs "github.com/farsightsec/golang-framestream"
	"github.com/golang/protobuf/proto"
)

// encoder wraps a golang-framestream.Encoder.
type encoder struct {
	fs *fs.Encoder
}

func newEncoder(w io.Writer, timeout time.Duration) (*encoder, error) {
	fs, err := fs.NewEncoder(w, &fs.EncoderOptions{
		ContentType:   []byte("protobuf:dnstap.Dnstap"),
		Bidirectional: true,
		Timeout:       timeout,
	})
	if err != nil {
		return nil, err
	}
	return &encoder{fs}, nil
}

func (e *encoder) writeMsg(msg *tap.Dnstap) error {
	buf, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = e.fs.Write(buf) // n < len(buf) should return an error?
	return err
}

func (e *encoder) flush() error { return e.fs.Flush() }
func (e *encoder) close() error { return e.fs.Close() }
