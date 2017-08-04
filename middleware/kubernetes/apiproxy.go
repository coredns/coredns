package kubernetes

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type proxyUpstream struct {
	network   string
	address   string
	stop      chan struct{}
	wait      sync.WaitGroup
	unhealthy int32
}

type proxyHandler struct {
	upstreams []proxyUpstream
	pickIndex int32
}

type apiProxy struct {
	http.Server
	listener net.Listener
	handler  proxyHandler
}

func (p proxyUpstream) Network() string {
	return p.network
}

func (p proxyUpstream) String() string {
	return p.address
}

func (p *proxyUpstream) Healthcheck() {
	p.wait.Add(1)
	defer p.wait.Done()

	status := func(unhealthy int32) string {
		if unhealthy == 0 {
			return "healthy"
		}
		return "unhealthy"
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial(p.Network(), p.String())
			},
		},
		Timeout: 5 * time.Second,
	}
	tick := time.NewTicker(5 * time.Second)
	// Start healthcheck immediately
	unhealthy := int32(1)
	if resp, err := client.Get("http://kubernetes/"); err == nil {
		defer resp.Body.Close()
		unhealthy = 0
	}
	log.Printf("[INFO] Healthcheck upstream %s://%s: %s", p.Network(), p.String(), status(unhealthy))
	atomic.StoreInt32(&(p.unhealthy), unhealthy)

	// Continue healthcheck in the next tick
	for {
		select {
		case <-tick.C:
			unhealthy := int32(1)
			if resp, err := client.Get("http://docker/"); err == nil {
				defer resp.Body.Close()
				unhealthy = 0
			}
			log.Printf("[INFO] Healthcheck upstream %s://%s: %s", p.Network(), p.String(), status(unhealthy))
			atomic.StoreInt32(&(p.unhealthy), unhealthy)
		case <-p.stop:
			return
		}
	}

}

func (p *proxyHandler) StartHealthcheck() {
	for i := range p.upstreams {
		go p.upstreams[i].Healthcheck()
	}
}

func (p *proxyHandler) StopHealthcheck() {
	for i := range p.upstreams {
		close(p.upstreams[i].stop)
	}
	for i := range p.upstreams {
		p.upstreams[i].wait.Wait()
	}
}

func (p *proxyHandler) SelectUpstream() net.Addr {
	pickIndex := atomic.LoadInt32(&(p.pickIndex))
	if atomic.LoadInt32(&(p.upstreams[pickIndex].unhealthy)) != 0 {
		log.Printf("[WARNING] Upstream status: %v %s://%s unhealthy", pickIndex, p.upstreams[pickIndex].Network(), p.upstreams[pickIndex].String())
		length := int32(len(p.upstreams))
		for index := (pickIndex + int32(1)) % length; index != pickIndex; index = (index + int32(1)) % length {
			if atomic.LoadInt32(&(p.upstreams[index].unhealthy)) == 0 {
				atomic.StoreInt32(&(p.pickIndex), index)
				pickIndex = index
				log.Printf("[INFO] Upstream update: %v %s://%s healthy", pickIndex, p.upstreams[pickIndex].Network(), p.upstreams[pickIndex].String())
				break
			}
		}
	}
	return p.upstreams[pickIndex]
}

func (p *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upstream := p.SelectUpstream()
	d, err := net.Dial(upstream.Network(), upstream.String())
	if err != nil {
		log.Printf("[ERROR] Unable to establish connection to upstream %s://%s: %s", upstream.Network(), upstream.String(), err)
		http.Error(w, fmt.Sprintf("Unable to establish connection to upstream %s://%s: %s", upstream.Network(), upstream.String(), err), 500)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		log.Printf("[ERROR] Unable to establish connection: no hijacker")
		http.Error(w, "Unable to establish connection: no hijacker", 500)
		return
	}
	nc, _, err := hj.Hijack()
	if err != nil {
		log.Printf("[ERROR] Unable to hijack connection: %s", err)
		http.Error(w, fmt.Sprintf("Unable to hijack connection: %s", err), 500)
		return
	}
	defer nc.Close()
	defer d.Close()

	err = r.Write(d)
	if err != nil {
		log.Printf("[ERROR] Unable to copy connection to upstream %s://%s: %s", upstream.Network(), upstream.String(), err)
		http.Error(w, fmt.Sprintf("Unable to copy connection to upstream %s://%s: %s", upstream.Network(), upstream.String(), err), 500)
		return
	}

	errChan := make(chan error, 2)
	cp := func(dst io.Writer, src io.Reader) {
		_, err := io.Copy(dst, src)
		errChan <- err
	}
	go cp(d, nc)
	go cp(nc, d)
	<-errChan
}

func (p *apiProxy) Run() {
	p.handler.StartHealthcheck()
	p.Serve(p.listener)
}

func (p *apiProxy) Stop() {
	p.handler.StopHealthcheck()
	p.Close()
}
