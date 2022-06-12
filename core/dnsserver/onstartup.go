package dnsserver

import (
	"fmt"
	"regexp"
	"sort"
)

// compileRegexp() returns regexp-object for RFC1035 preferred syntax.
func compileRegexp() *regexp.Regexp {
	matchDomain, _ := regexp.Compile("(^([A-Za-z]([A-Za-z0-9-]*)(\\.([A-Za-z0-9-]+))*\\.)$)|^(\\.)$")
	return matchDomain
}

// startUpZones creates the text that we show when starting up:
// grpc://example.com.:1055
// example.com.:1053 on 127.0.0.1
func startUpZones(protocol, addr string, zones map[string]*Config) string {
	s := ""
	regexpObj := compileRegexp() // regexpObj initialised and declared with a regexp-object for RFC1035 preferred syntax.

	keys := make([]string, len(zones))
	i := 0
	for k := range zones {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	for _, zone := range keys {
		if !regexpObj.MatchString(zone) {
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
