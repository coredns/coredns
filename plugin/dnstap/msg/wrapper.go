package msg

import (
	"fmt"

	lib "github.com/dnstap/golang-dnstap"
	"github.com/golang/protobuf/proto"
)

func wrap(m *lib.Message, e []byte) *lib.Dnstap {
	t := lib.Dnstap_MESSAGE
	return &lib.Dnstap{
		Type:    &t,
		Message: m,
		Extra:   e,
	}
}

// Marshal encodes the message to a binary dnstap payload.
func Marshal(m *lib.Message, e []byte) (data []byte, err error) {
	data, err = proto.Marshal(wrap(m, e))
	if err != nil {
		err = fmt.Errorf("proto: %s", err)
		return
	}
	return
}
