package rlc

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	tap "github.com/dnstap/golang-dnstap"
	"github.com/miekg/dns"
	"google.golang.org/protobuf/proto"
)

func (h *RlcHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == healthPath {
		w.Header().Add("content-type", "application/json")
		w.Write(([]byte)("{status: \"ok\" }"))
		w.WriteHeader(200)
		return
	}

	w.WriteHeader(404)
}

func (h *RlcHandler) Printf(format string, v ...interface{}) {
	logger.Debug(fmt.Sprintf(format, v...))
}

func (h *RlcHandler) initRemote() error {
	address := fmt.Sprintf("0.0.0.0:%d", h.RemotePort)
	var err error
	h.remoteListener, err = net.Listen("tcp", address)
	if err != nil {
		logger.Fatalf("Error %v", err)
	}

	tapChan := make(chan []byte)

	go func(tapChan chan []byte) {
		for data := range tapChan {
			var srcMsg tap.Dnstap
			proto.Unmarshal(data, &srcMsg)

			var targetMsg dns.Msg
			if srcMsg.Message == nil {
				logger.Debug(fmt.Sprintf("%s: empty message: %v\n",
					h.remoteListener.Addr(),
					err))
				continue
			}
			err = targetMsg.Unpack(srcMsg.Message.QueryMessage)
			if err != nil {
				logger.Debug(fmt.Sprintf("%s: Unpack failed: %v\n",
					h.remoteListener.Addr(),
					err))
				continue
			}

			if h.isQueryMessageOfInterest(&targetMsg) {
				h.handleQueryMessage(&targetMsg)
				logger.Debug(fmt.Sprintf("received:\n%v\n%v\n\n", srcMsg.Message, targetMsg))
			} else if h.isPtrMessageOfInterest(&targetMsg) {
				h.handleRemotePtrMessage(&targetMsg)
				logger.Debug(fmt.Sprintf("received:\n%v\n%v\n\n", srcMsg.Message, targetMsg))
			}
			//			targetMsg.MsgHdr.Type = srcMsg.Message.Type

		}
	}(tapChan)

	h.wgRemoteServerDone.Add(1)
	go func() {
		const timeout = 60 * time.Second
		var n uint64
		for {
			conn, err := h.remoteListener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					break
				}
				logger.Debug(fmt.Sprintf("%s: accept failed: %v\n",
					h.remoteListener.Addr(),
					err))
				continue
			}
			n++
			origin := ""
			switch conn.RemoteAddr().Network() {
			case "tcp", "tcp4", "tcp6":
				origin = fmt.Sprintf(" from %s", conn.RemoteAddr())
			}
			i, err := tap.NewFrameStreamInputTimeout(conn, true, timeout)
			if err != nil {
				logger.Debug(fmt.Sprintf("%s: connection %d: open input%s failed: %v",
					conn.LocalAddr(), n, origin, err))
				continue
			}
			logger.Debug(fmt.Sprintf("%s: accepted connection %d%s",
				conn.LocalAddr(), n, origin))
			i.SetLogger(h)
			go func(cn uint64) {
				i.ReadInto(tapChan)
				logger.Debug(fmt.Sprintf("%s: closed connection %d%s",
					conn.LocalAddr(), cn, origin))
			}(n)
		}

		h.wgRemoteServerDone.Done()
	}()

	return nil
}
