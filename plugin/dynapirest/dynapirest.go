package dynapirest

import (
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"sync"

	"github.com/coredns/coredns/dynapi"
	clog "github.com/coredns/coredns/plugin/pkg/log"
)

var log = clog.NewWithPlugin("dynapirest")

const (
	ok                  = "OK"
	zoneNotFound        = "Zone not found"
	recordAlreadyExists = "Record already exists"
	recordDoesNotExist  = "Record does not exist"
	defAddr             = ":8090"
	path                = "/dynapi"
)

type dynapirest struct {
	Addr string

	ln      net.Listener
	nlSetup bool
	mux     *http.ServeMux

	sync.RWMutex
	dynapiWriters map[string]dynapi.Writable
}

func newDynapiRest(addr string) *dynapirest {
	return &dynapirest{Addr: addr, dynapiWriters: map[string]dynapi.Writable{}}
}

func (d *dynapirest) parseRequest(r *http.Request) (*dynapi.Request, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	var dynapiRequest *dynapi.Request
	err = json.Unmarshal(body, &dynapiRequest)
	if err != nil {
		return nil, err
	}

	return dynapiRequest, nil
}

func (d *dynapirest) handleDelete(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	dynapiRequest, err := d.parseRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dynapiWriter, mapOk := d.dynapiWriters[dynapiRequest.Zone]
	if !mapOk {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(zoneNotFound))
		return
	}

	if !dynapiWriter.Exists(dynapiRequest) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(recordDoesNotExist))
		return
	}

	err = dynapiWriter.Delete(dynapiRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(ok))
	return
}

func (d *dynapirest) handlePATCH(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	dynapiRequest, err := d.parseRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dynapiWriter, mapOk := d.dynapiWriters[dynapiRequest.Zone]
	if !mapOk {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(zoneNotFound))
		return
	}

	if !dynapiWriter.ExistsByName(dynapiRequest) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(recordDoesNotExist))
		return
	}

	err = dynapiWriter.Update(dynapiRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(ok))
}

func (d *dynapirest) handlePUT(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	dynapiRequest, err := d.parseRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dynapiWriter, mapOk := d.dynapiWriters[dynapiRequest.Zone]
	if !mapOk {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(zoneNotFound))
		return
	}

	err = dynapiWriter.Upsert(dynapiRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(ok))
}

func (d *dynapirest) handlePOST(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	dynapiRequest, err := d.parseRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dynapiWriter, mapOk := d.dynapiWriters[dynapiRequest.Zone]
	if !mapOk {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(zoneNotFound))
		return
	}

	if dynapiWriter.ExistsByName(dynapiRequest) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(recordAlreadyExists))
		return
	}

	err = dynapiWriter.Create(dynapiRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(ok))
}

func (d *dynapirest) OnStartup() error {
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
		case http.MethodDelete:
			d.handleDelete(w, r)
		case http.MethodPut:
			d.handlePUT(w, r)
		case http.MethodPost:
			d.handlePOST(w, r)
		case http.MethodPatch:
			d.handlePATCH(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	go func() { http.Serve(d.ln, d.mux) }()

	return nil
}

func (d *dynapirest) OnRestart() error { return d.OnFinalShutdown() }

func (d *dynapirest) OnFinalShutdown() error {
	if !d.nlSetup {
		return nil
	}

	d.ln.Close()
	d.nlSetup = false
	return nil
}
