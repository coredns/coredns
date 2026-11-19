package dnstap

import (
	"net"
	"time"

	tap "github.com/dnstap/golang-dnstap"
	fs "github.com/farsightsec/golang-framestream"
	"google.golang.org/protobuf/proto"
)

// encoder wraps a golang-framestream.Encoder and owns the connection it writes
// to, so that closing the encoder also releases the socket.
type encoder struct {
	fs   *fs.Encoder
	conn net.Conn
}

func newEncoder(c net.Conn, timeout time.Duration) (*encoder, error) {
	fs, err := fs.NewEncoder(c, &fs.EncoderOptions{
		ContentType:   []byte("protobuf:dnstap.Dnstap"),
		Bidirectional: true,
		Timeout:       timeout,
	})
	if err != nil {
		return nil, err
	}
	return &encoder{fs: fs, conn: c}, nil
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

// close shuts the framestream down and then closes the underlying connection.
// framestream's Close only writes CONTROL_STOP and waits for the peer's
// CONTROL_FINISH; it never touches the socket. Closing it here is what keeps a
// reconnect or a shutdown from leaking the fd until the runtime finalizes it.
func (e *encoder) close() error {
	err := e.fs.Close()
	if cerr := e.conn.Close(); err == nil {
		err = cerr
	}
	return err
}
