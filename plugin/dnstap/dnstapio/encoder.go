// Package dnstapio is a small wrapper around golang-framestream
package dnstapio

import (
	"io"
	"time"

	tap "github.com/dnstap/golang-dnstap"
	fs "github.com/farsightsec/golang-framestream"
	"github.com/golang/protobuf/proto"
)

// Encoder wraps a fs.Encoder.
type Encoder struct {
	fs *fs.Encoder
}

func newEncoder(w io.Writer, timeout time.Duration) (*Encoder, error) {
	fs, err := fs.NewEncoder(w, &fs.EncoderOptions{
		ContentType:   []byte("protobuf:dnstap.Dnstap"),
		Bidirectional: true,
		Timeout:       timeout,
	})
	if err != nil {
		return nil, err
	}
	return &Encoder{fs}, nil
}

func (e *Encoder) writeMsg(msg *tap.Dnstap) error {
	buf, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = e.fs.Write(buf) // n < len(buf) should return an error
	return err
}

func (e *Encoder) flush() error { return e.fs.Flush() }
func (e *Encoder) close() error { return e.fs.Close() }
