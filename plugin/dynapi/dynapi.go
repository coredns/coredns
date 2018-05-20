package dynapi

import (
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"sync"

	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var log = clog.NewWithPlugin("dynapi")

type dynapi struct {
	Addr string

	ln      net.Listener
	nlSetup bool
	mux     *http.ServeMux

	sync.RWMutex
	dynapiImplementable map[string]DynapiImplementable
}

func newApi(addr string) *dynapi {
	return &dynapi{Addr: addr, dynapiImplementable: map[string]DynapiImplementable{}}
}

func (d *dynapi) OnStartup() error {
	if d.Addr == "" {
		d.Addr = defAddr
	}

	ln, err := net.Listen("tcp", d.Addr)
	if err != nil {
		return err
	}

	d.ln = ln
	d.mux = http.NewServeMux()
	d.nlSetup = true

	d.mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "POST":
			body, err := ioutil.ReadAll(r.Body)
			defer r.Body.Close()

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			var dnsRecordRequest *DynapiRequest
			err = json.Unmarshal(body, &dnsRecordRequest)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			dynapiImplementer, mapOk := d.dynapiImplementable[dnsRecordRequest.Zone]

			if !mapOk {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(zoneNotFound))
				return
			}

			err = dynapiImplementer.Create(dnsRecordRequest)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("content-type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(ok))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	go func() { http.Serve(d.ln, d.mux) }()

	return nil
}

func (d *dynapi) OnRestart() error { return d.OnFinalShutdown() }

func (d *dynapi) OnFinalShutdown() error {
	if !d.nlSetup {
		return nil
	}

	d.ln.Close()
	d.nlSetup = false
	return nil
}

const (
	ok           = "OK"
	zoneNotFound = "Zone not found"
	defAddr      = ":8090"
	path         = "/dynapi"
)
