package dnstap

import (
	"fmt"
	"io"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/dnstap/msg"
	"github.com/coredns/coredns/plugin/dnstap/taprw"

	tap "github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

// Dnstap is the dnstap handler.
type Dnstap struct {
	Next plugin.Handler
	Out  io.Writer
	Pack bool
}

type (
	// Tapper is implemented by the Context passed by the dnstap handler.
	Tapper interface {
		TapMessage(*tap.Message, []byte) error
		TapBuilder() msg.Builder
	}
	tapContext struct {
		context.Context
		Dnstap
	}
)

// Name of data map in the context to be sent in the extra field of tap.Dnstap
const (
	DnstapExtraMap = "dnstap_extra"
)

// TapperFromContext will return a Tapper if the dnstap plugin is enabled.
func TapperFromContext(ctx context.Context) (t Tapper) {
	t, _ = ctx.(Tapper)
	return
}

func tapMessageTo(w io.Writer, m *tap.Message, e []byte) error {
	frame, err := msg.Marshal(m, e)
	if err != nil {
		return fmt.Errorf("marshal: %s", err)
	}
	_, err = w.Write(frame)
	return err
}

// TapMessage implements Tapper.
func (h Dnstap) TapMessage(m *tap.Message, e []byte) error {
	return tapMessageTo(h.Out, m, e)
}

// TapBuilder implements Tapper.
func (h Dnstap) TapBuilder() msg.Builder {
	return msg.Builder{Full: h.Pack}
}

// ServeDNS logs the client query and response to dnstap and passes the dnstap Context.
func (h Dnstap) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	// Add a map where other middlewares can add data to be included into the
	// extra field in tap.Dnstap into the context
	extras := make(map[string]taprw.DnstapExtra)
	newCtx := context.WithValue(ctx, DnstapExtraMap, extras)

	rw := &taprw.ResponseWriter{ResponseWriter: w, Tapper: &h, Query: r, DnsTapExtras: extras}
	rw.QueryEpoch()

	code, err := plugin.NextOrFailure(h.Name(), h.Next, tapContext{newCtx, h}, rw, r)
	if err != nil {
		// ignore dnstap errors
		return code, err
	}

	if err := rw.DnstapError(); err != nil {
		return code, plugin.Error("dnstap", err)
	}

	return code, nil
}

// Name returns dnstap.
func (h Dnstap) Name() string { return "dnstap" }
