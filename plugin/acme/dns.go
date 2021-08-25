package acme

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)


func getAuthoritativeNameServers(zone string) ([]string, error) {
	resolvers := recursiveNameservers(nil)
	nameservers, err := lookupNameservers(zone, resolvers)
	if err != nil {
		return []string{}, err
	}
	return nameservers, nil
}

func getExternalIpAddress(zone string) (string, error) {
	resolvers := recursiveNameservers(nil)
	r, err := dnsQuery(zone, dns.TypeA, resolvers, true)
	if err != nil {
		return "", fmt.Errorf("dns query for zone %v with resolvers %+v error: %+v", zone, resolvers, err)
	}
	for _, rr := range r.Answer {
		if a, ok := rr.(*dns.A); ok {
			return a.A.String(), nil
		}
	}
	return "", fmt.Errorf("no A record found for zone %s", zone)
}

func findZoneByFQDN(fqdn string, nameservers []string) (string, error) {
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}
	soa, err := lookupSoaByFqdn(fqdn, nameservers)
	if err != nil {
		return "", err
	}
	return soa.zone, nil
}

func lookupSoaByFqdn(fqdn string, nameservers []string) (*soaEntry, error) {
	if !strings.HasSuffix(fqdn, ".") {
		fqdn += "."
	}

	ent, err := fetchSoaByFqdn(fqdn, nameservers)
	if err != nil {
		return nil, err
	}

	return ent, nil
}

func fetchSoaByFqdn(fqdn string, nameservers []string) (*soaEntry, error) {
	var err error
	var in *dns.Msg

	labelIndexes := dns.Split(fqdn)
	for _, index := range labelIndexes {
		domain := fqdn[index:]

		in, err = dnsQuery(domain, dns.TypeSOA, nameservers, true)
		if err != nil {
			continue
		}
		if in == nil {
			continue
		}

		switch in.Rcode {
		case dns.RcodeSuccess:
			// Check if we got a SOA RR in the answer section
			if len(in.Answer) == 0 {
				continue
			}

			// CNAME records cannot/should not exist at the root of a zone.
			// So we skip a domain when a CNAME is found.
			if dnsMsgContainsCNAME(in) {
				continue
			}

			for _, ans := range in.Answer {
				if soa, ok := ans.(*dns.SOA); ok {
					return newSoaEntry(soa), nil
				}
			}
		case dns.RcodeNameError:
			// NXDOMAIN
		default:
			// Any response code other than NOERROR and NXDOMAIN is treated as error
			return nil, fmt.Errorf("unexpected response code '%s' for %s", dns.RcodeToString[in.Rcode], domain)
		}
	}

	return nil, fmt.Errorf("could not find the start of authority for %s%s", fqdn, formatDNSError(in, err))
}

// dnsMsgContainsCNAME checks for a CNAME answer in msg
func dnsMsgContainsCNAME(msg *dns.Msg) bool {
	for _, ans := range msg.Answer {
		if _, ok := ans.(*dns.CNAME); ok {
			return true
		}
	}
	return false
}

func dnsQuery(fqdn string, rtype uint16, nameservers []string, recursive bool) (*dns.Msg, error) {
	m := createDNSMsg(fqdn, rtype, recursive)
	var in *dns.Msg
	var err error
	for _, ns := range nameservers {
		in, err = sendDNSQuery(m, ns)
		if err == nil && len(in.Answer) > 0 {
			break
		}
	}
	return in, err
}

func createDNSMsg(fqdn string, rtype uint16, recursive bool) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(fqdn, rtype)
	m.SetEdns0(4096, false)
	if !recursive {
		m.RecursionDesired = false
	}
	return m
}

func sendDNSQuery(m *dns.Msg, ns string) (*dns.Msg, error) {
	udp := &dns.Client{Net: "udp", Timeout: dnsTimeout}
	in, _, err := udp.Exchange(m, ns)
	truncated := in != nil && in.Truncated
	timeoutErr := err != nil && strings.Contains(err.Error(), "timeout")
	if truncated || timeoutErr {
		tcp := &dns.Client{Net: "tcp", Timeout: dnsTimeout}
		in, _, err = tcp.Exchange(m, ns)
	}
	return in, err
}

func formatDNSError(msg *dns.Msg, err error) string {
	var parts []string
	if msg != nil {
		parts = append(parts, dns.RcodeToString[msg.Rcode])
	}
	if err != nil {
		parts = append(parts, err.Error())
	}
	if len(parts) > 0 {
		return ": " + strings.Join(parts, " ")
	}
	return ""
}

type soaEntry struct {
	zone      string    // zone apex (a domain name)
	primaryNs string    // primary nameserver for the zone apex
	expires   time.Time // time when this cache entry should be evicted
}

func newSoaEntry(soa *dns.SOA) *soaEntry {
	return &soaEntry{
		zone:      soa.Hdr.Name,
		primaryNs: soa.Ns,
		expires:   time.Now().Add(time.Duration(soa.Refresh) * time.Second),
	}
}

// systemOrDefaultNameservers attempts to get system nameservers from the
// resolv.conf file given by path before falling back to hard-coded defaults.
func systemOrDefaultNameservers(path string, defaults []string) []string {
	config, err := dns.ClientConfigFromFile(path)
	if err != nil || len(config.Servers) == 0 {
		return defaults
	}
	return config.Servers
}

// populateNameserverPorts ensures that all nameservers have a port number.
func populateNameserverPorts(servers []string) {
	for i := range servers {
		_, port, _ := net.SplitHostPort(servers[i])
		if port == "" {
			servers[i] = net.JoinHostPort(servers[i], "53")
		}
	}
}

// lookupNameservers returns the authoritative nameservers for the given fqdn.
func lookupNameservers(fqdn string, resolvers []string) ([]string, error) {
	var authoritativeNss []string

	zone, err := findZoneByFQDN(fqdn, resolvers)
	if err != nil {
		return nil, fmt.Errorf("could not determine the zone: %w", err)
	}

	r, err := dnsQuery(zone, dns.TypeNS, resolvers, true)
	if err != nil {
		return nil, err
	}

	for _, rr := range r.Answer {
		if ns, ok := rr.(*dns.NS); ok {
			authoritativeNss = append(authoritativeNss, strings.ToLower(ns.Ns))
		}
	}

	if len(authoritativeNss) > 0 {
		return authoritativeNss, nil
	}
	return nil, errors.New("could not determine authoritative nameservers")
}

// recursiveNameservers are used to pre-check DNS propagation. It
// picks user-configured nameservers (custom) OR the defaults
// obtained from resolv.conf and defaultNameservers if none is
// configured and ensures that all server addresses have a port value.
func recursiveNameservers(custom []string) []string {
	var servers []string
	if len(custom) == 0 {
		//servers = systemOrDefaultNameservers(defaultResolvConf, defaultNameservers)
		servers = defaultNameservers
	} else {
		servers = make([]string, len(custom))
		copy(servers, custom)
	}
	populateNameserverPorts(servers)
	return servers
}

var defaultNameservers = []string{
	"8.8.8.8:53",
	"8.8.4.4:53",
	"1.1.1.1:53",
	"1.0.0.1:53",
}

var dnsTimeout = 10 * time.Second

const defaultResolvConf = "/etc/resolv.conf"
