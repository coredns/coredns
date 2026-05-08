package dnstap

import (
	"net"
	"time"

	tap "github.com/dnstap/golang-dnstap"
	fs "github.com/farsightsec/golang-framestream"
	"google.golang.org/protobuf/proto"
)

// encoder wraps a golang-framestream.Encoder.
type encoder struct {
	fs   *fs.Encoder
	conn net.Conn
}

func newEncoder(conn net.Conn, timeout time.Duration) (*encoder, error) {
	fs, err := fs.NewEncoder(conn, &fs.EncoderOptions{
		ContentType:   []byte("protobuf:dnstap.Dnstap"),
		Bidirectional: true,
		Timeout:       timeout,
	})
	if err != nil {
		return nil, err
	}
	return &encoder{fs: fs, conn: conn}, nil
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
func (e *encoder) close() error {
	err := e.fs.Close()
	if closeErr := e.conn.Close(); err == nil {
		err = closeErr
	}
	return err
}
