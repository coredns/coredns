package test

import (
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
)

//ListenerAllocator is able to generate a generic tcp Listener on any address
type ListenerAllocator struct {
	Network string
	Address string
}

//AllocateListener provide a tcp Listener on protocol and address specified
func (lsn ListenerAllocator) AllocateListener() (net.Listener, error) {
	return net.Listen(lsn.Network, lsn.Address)
}

//NewListenerAllocator creates a new allocator at defined protocol and address
func NewListenerAllocator(network string, address string) ListenerAllocator {
	return ListenerAllocator{Network: network, Address: address}
}

//NewDefaultAllocator provides a convenient ListenerAllocator for default next available port
func NewDefaultAllocator() ListenerAllocator {
	return NewListenerAllocator("tcp", "localhost:0")
}

// TCPServer starts a DNS server with a TCP listener on laddr.
func TCPServer(laddr string) (*dns.Server, string, error) {
	l, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, "", err
	}

	server := &dns.Server{Listener: l, ReadTimeout: time.Hour, WriteTimeout: time.Hour}

	waitLock := sync.Mutex{}
	waitLock.Lock()
	server.NotifyStartedFunc = func() { waitLock.Unlock() }

	go func() {
		server.ActivateAndServe()
		l.Close()
	}()

	waitLock.Lock()
	return server, l.Addr().String(), nil
}

// UDPServer starts a DNS server with an UDP listener on laddr.
func UDPServer(laddr string) (*dns.Server, string, error) {
	pc, err := net.ListenPacket("udp", laddr)
	if err != nil {
		return nil, "", err
	}
	server := &dns.Server{PacketConn: pc, ReadTimeout: time.Hour, WriteTimeout: time.Hour}

	waitLock := sync.Mutex{}
	waitLock.Lock()
	server.NotifyStartedFunc = func() { waitLock.Unlock() }

	go func() {
		server.ActivateAndServe()
		pc.Close()
	}()

	waitLock.Lock()
	return server, pc.LocalAddr().String(), nil
}
