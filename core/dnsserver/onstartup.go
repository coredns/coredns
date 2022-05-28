package dnsserver

import (
	"fmt"
	"regexp"
	"sort"
)

// checkDomainName() returns true if the given domain name follows RFC1035 preferred syntax.
func checkDomainName(domainName string) bool {
	matchDomain, _ := regexp.Compile("[A-Za-z0-9-]{1,}\\.[A-Za-z0-9-]{1,}\\.$)|^\\.$")
	return matchDomain.MatchString(domainName)
}

// startUpZones creates the text that we show when starting up:
// grpc://example.com.:1055
// example.com.:1053 on 127.0.0.1
func startUpZones(protocol, addr string, zones map[string]*Config) string {
	s := ""

	keys := make([]string, len(zones))
	i := 0
	for k := range zones {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	for _, zone := range keys {

		if !checkDomainName(zone) {
			s += fmt.Sprintln("Warning: Domain " + zone + " does not follow RFC1035 preferred syntax")
		}

		// split addr into protocol, IP and Port
		_, ip, port, err := SplitProtocolHostPort(addr)

		if err != nil {
			// this should not happen, but we need to take care of it anyway
			s += fmt.Sprintln(protocol + zone + ":" + addr)
			continue
		}
		if ip == "" {
			s += fmt.Sprintln(protocol + zone + ":" + port)
			continue
		}
		// if the server is listening on a specific address let's make it visible in the log,
		// so one can differentiate between all active listeners
		s += fmt.Sprintln(protocol + zone + ":" + port + " on " + ip)
	}
	return s
}
