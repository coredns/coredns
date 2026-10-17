package transport

// These transports are supported by CoreDNS.
const (
	DNS    = "dns"
	TLS    = "tls"
	QUIC   = "quic"
	GRPC   = "grpc"
	HTTPS  = "https"
	HTTPS3 = "https3"
	UNIX   = "unix"
)

// Port numbers for the various transports.
const (
	// Port is the default port for DNS
	Port = "53"
	// TLSPort is the default port for DNS-over-TLS.
	TLSPort = "853"
	// QUICPort is the default port for DNS-over-QUIC.
	QUICPort = "853"
	// GRPCPort is the default port for DNS-over-gRPC.
	GRPCPort = "443"
	// HTTPSPort is the default port for DNS-over-HTTPS.
	HTTPSPort = "443"
)

var Ports = map[string]string{
	DNS:    Port,
	TLS:    TLSPort,
	QUIC:   QUICPort,
	GRPC:   GRPCPort,
	HTTPS:  HTTPSPort,
	HTTPS3: HTTPSPort,
}

func Register(name string, port string) {
	if name == "" {
		panic("transport must have a name")
	}
	if p, dup := Ports[name]; dup && p != port { // allows builtin to "re-register"
		panic("port for " + name + ":// already registered as " + p)
	}
	Ports[name] = port
}
