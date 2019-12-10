// Package route53 implements a plugin that returns resource records
// from AWS route53.
package route53

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/fall"
	"github.com/coredns/coredns/plugin/pkg/upstream"
	"github.com/coredns/coredns/request"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/miekg/dns"
)

// Route53 is a plugin that returns RR from AWS route53.
type Route53 struct {
	Next plugin.Handler
	Fall fall.F

	zoneNames []string
	client    route53iface.Route53API
	upstream  *upstream.Upstream
	refresh   time.Duration

	aliasRefresh  time.Duration
	aliasTTL      int64
	aliasResolver AliasResolver

	zMu   sync.RWMutex
	zones zones
}

type zone struct {
	id      string
	z       *file.Zone
	dns     string
	aliases map[string]aliasRecord
}

type zones map[string][]*zone

type aliasRecord struct {
	dnsName     string
	dnsType     uint16
	dnsTypeName string
	dnsZoneId   string
}

// New reads from the keys map which uses domain names as its key and hosted
// zone id lists as its values, validates that each domain name/zone id pair
// does exist, and returns a new *Route53. In addition to this, upstream is use
// for doing recursive queries against CNAMEs. Returns error if it cannot
// verify any given domain name/zone id pair.
func New(ctx context.Context, c route53iface.Route53API, keys map[string][]string, refresh time.Duration, aliasResolver AliasResolver) (*Route53, error) {
	zones := make(map[string][]*zone, len(keys))
	zoneNames := make([]string, 0, len(keys))
	for dns, hostedZoneIDs := range keys {
		for _, hostedZoneID := range hostedZoneIDs {
			_, err := c.ListHostedZonesByNameWithContext(ctx, &route53.ListHostedZonesByNameInput{
				DNSName:      aws.String(dns),
				HostedZoneId: aws.String(hostedZoneID),
			})
			if err != nil {
				return nil, err
			}
			if _, ok := zones[dns]; !ok {
				zoneNames = append(zoneNames, dns)
			}
			zones[dns] = append(zones[dns], &zone{id: hostedZoneID, dns: dns, z: file.NewZone(dns, "")})
		}
	}
	return &Route53{
		client:        c,
		zoneNames:     zoneNames,
		zones:         zones,
		upstream:      upstream.New(),
		refresh:       refresh,
		aliasRefresh:  time.Minute,
		aliasTTL:      60,
		aliasResolver: aliasResolver,
	}, nil
}

// Run executes first update, spins up an update forever-loop.
// Returns error if first update fails.
func (h *Route53) Run(ctx context.Context) error {
	if err := h.updateZones(ctx); err != nil {
		return err
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Infof("Breaking out of Route53 update loop: %v", ctx.Err())
				return
			case <-time.After(h.aliasRefresh):
				if h.aliasRefresh >= h.refresh {
					// we don't need to do anything as the refresh period is lower than the alias refresh
					// period, so we can skip the extra call to update the aliases
					continue
				}
				// make this auto adjustable so we can adjust based on the TTL,
				// health-checks can be 15, 30 or 60 seconds per AWS so worst case this has to execute every 15 seconds
				if err := h.updateAliases(ctx); err != nil && ctx.Err() == nil /* Don't log error if ctx expired. */ {
					log.Errorf("Failed to update aliases: %v", err)
				}
			case <-time.After(h.refresh):
				if err := h.updateZones(ctx); err != nil && ctx.Err() == nil /* Don't log error if ctx expired. */ {
					log.Errorf("Failed to update zones: %v", err)
				}
			}
		}
	}()
	return nil
}

func updateAliasesForZone(ctx context.Context, zMu sync.RWMutex, zone *zone, resolver AliasResolver, aliasTTL int64) (err error) {
	for name, alias := range zone.aliases {
		// remove old records from the hosted zone file if they exist
		zMu.RLock()
		records, found := zone.z.Search(name)
		zMu.RUnlock()
		if found {
			// we need to remove all of them so the next time we add it will be the new ones
			zMu.Lock()
			for _, record := range records.All() {
				zone.z.Delete(record)
			}
			zMu.Unlock()
		}

		// check to make sure that we don't host the zone first, if we do
		// we need to return the alias from our zone, no need for an external lookup
		if zone.id == alias.dnsZoneId {
			records, found := zone.z.Search(alias.dnsName)
			if found {
				for _, record := range records.All() {
					rec, _ := remapDnsAliasRR(name, record, int64(record.Header().Ttl))
					zMu.Lock()
					_ = zone.z.Insert(rec)
					zMu.Unlock()
				}
			}
			continue // we are finished, onto the next alias
		}

		// go through all the ns servers, before giving up on the resolving
		for _, ns := range zone.z.NS {
			var rrs []dns.RR
			nameserver := fmt.Sprintf("%s:53", ns.(*dns.NS).Ns)
			rrs, err = resolver.Resolve(ctx, name, alias.dnsType, nameserver, aliasTTL)
			if err != nil {
				continue // we skip and continue onto the next one until we exhaust all of them
			}
			for _, rr := range rrs {
				zMu.Lock()
				_ = zone.z.Insert(rr)
				zMu.Unlock()
			}
			break // we finished successfully no need to go over the next nameserver
		}
	}
	return err
}

func (h *Route53) updateAliases(ctx context.Context) error {
	errc := make(chan error)
	defer close(errc)

	for zName, z := range h.zones {
		go func(zName string, z []*zone) {
			var err error
			defer func() {
				errc <- err
			}()
			for _, hostedZone := range z {
				err = updateAliasesForZone(ctx, h.zMu, hostedZone, h.aliasResolver, h.aliasTTL)
			}
		}(zName, z)
	}

	// Collect errors (if any). This will also sync on all zones updates
	// completion.
	var errs []string
	for i := 0; i < len(h.zones); i++ {
		err := <-errc
		if err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) != 0 {
		return fmt.Errorf("errors updating aliases: %v", errs)
	}

	return nil
}

// ServeDNS implements the plugin.Handler.ServeDNS.
func (h *Route53) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}
	qname := state.Name()

	zName := plugin.Zones(h.zoneNames).Matches(qname)
	if zName == "" {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}
	z, ok := h.zones[zName]
	if !ok || z == nil {
		return dns.RcodeServerFailure, nil
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	var result file.Result
	for _, hostedZone := range z {
		h.zMu.RLock()
		m.Answer, m.Ns, m.Extra, result = hostedZone.z.Lookup(ctx, state, qname)
		h.zMu.RUnlock()

		// Take the answer if it's non-empty OR if there is another
		// record type exists for this name (NODATA).
		if len(m.Answer) != 0 || result == file.NoData {
			break
		}
	}

	if len(m.Answer) == 0 && result != file.NoData && h.Fall.Through(qname) {
		return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
	}

	switch result {
	case file.Success:
	case file.NoData:
	case file.NameError:
		m.Rcode = dns.RcodeNameError
	case file.Delegation:
		m.Authoritative = false
	case file.ServerFailure:
		return dns.RcodeServerFailure, nil
	}

	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

const escapeSeq = `\\`

// maybeUnescape parses s and converts escaped ASCII codepoints (in octal) back
// to its ASCII representation.
//
// From AWS docs:
//
// "If the domain name includes any characters other than a to z, 0 to 9, -
// (hyphen), or _ (underscore), Route 53 API actions return the characters as
// escape codes."
//
// For our purposes (and with respect to RFC 1035), we'll fish for a-z, 0-9,
// '-', '.' and '*' as the leftmost character (for wildcards) and throw error
// for everything else.
//
// Example:
//   `\\052.example.com.` -> `*.example.com`
//   `\\137.example.com.` -> error ('_' is not valid)
func maybeUnescape(s string) (string, error) {
	var out string
	for {
		i := strings.Index(s, escapeSeq)
		if i < 0 {
			return out + s, nil
		}

		out += s[:i]

		li, ri := i+len(escapeSeq), i+len(escapeSeq)+3
		if ri > len(s) {
			return "", fmt.Errorf("invalid escape sequence: '%s%s'", escapeSeq, s[li:])
		}
		// Parse `\\xxx` in base 8 (2nd arg) and attempt to fit into
		// 8-bit result (3rd arg).
		n, err := strconv.ParseInt(s[li:ri], 8, 8)
		if err != nil {
			return "", fmt.Errorf("invalid escape sequence: '%s%s'", escapeSeq, s[li:ri])
		}

		r := rune(n)
		switch {
		case r >= rune('a') && r <= rune('z'): // Route53 converts everything to lowercase.
		case r >= rune('0') && r <= rune('9'):
		case r == rune('*'):
			if out != "" {
				return "", errors.New("`*' only supported as wildcard (leftmost label)")
			}
		case r == rune('-'):
		case r == rune('.'):
		default:
			return "", fmt.Errorf("invalid character: %s%#03o", escapeSeq, r)
		}

		out += string(r)

		s = s[i+len(escapeSeq)+3:]
	}
}

func rrFromRR(name string, dnsType string, ttl int64, record route53.ResourceRecord) (dns.RR, error) {

	v, err := maybeUnescape(aws.StringValue(record.Value))
	if err != nil {
		return nil, fmt.Errorf("failed to unescape `%s' value: %v", aws.StringValue(record.Value), err)
	}

	rfc1035 := fmt.Sprintf("%s %d IN %s %s", name, ttl, dnsType, v)
	r, err := dns.NewRR(rfc1035)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource record: %v", err)
	}
	return r, nil
}

func remapDnsAliasRR(name string, record dns.RR, ttl int64) (dns.RR, error) {
	var val string
	var dnsTypeName string
	switch r := record.(type) {
	case *dns.A:
		val = r.A.String()
		dnsTypeName = "A"
	case *dns.AAAA:
		val = r.AAAA.String()
		dnsTypeName = "AAAA"
	}

	return rrFromRR(name, dnsTypeName, ttl, route53.ResourceRecord{Value: &val})
}

func updateZoneFromRRS(rrs *route53.ResourceRecordSet, z *file.Zone, alias map[string]aliasRecord) error {
	n, err := maybeUnescape(aws.StringValue(rrs.Name))
	if err != nil {
		return fmt.Errorf("failed to unescape `%s' name: %v", aws.StringValue(rrs.Name), err)
	}

	if rrs.AliasTarget != nil {
		var dnsType uint16
		var dnsName = aws.StringValue(rrs.AliasTarget.DNSName)
		switch aws.StringValue(rrs.Type) {
		case "A":
			dnsType = dns.TypeA
		case "AAAA":
			dnsType = dns.TypeAAAA
		default:
			return fmt.Errorf("failed to process alias record for %v => %v from route53: %v", n, dnsName, err)
		}
		alias[n] = aliasRecord{dnsName: dnsName, dnsType: dnsType, dnsTypeName: aws.StringValue(rrs.Type), dnsZoneId: aws.StringValue(rrs.AliasTarget.HostedZoneId)}
	}

	for _, rr := range rrs.ResourceRecords {
		r, err := rrFromRR(n, aws.StringValue(rrs.Type), aws.Int64Value(rrs.TTL), *rr)
		if err != nil {
			return err
		}
		z.Insert(r)
	}
	return nil
}

// updateZones re-queries resource record sets for each zone and updates the
// zone object.
// Returns error if any zones error'ed out, but waits for other zones to
// complete first.
func (h *Route53) updateZones(ctx context.Context) error {
	errc := make(chan error)
	defer close(errc)
	for zName, z := range h.zones {
		go func(zName string, z []*zone) {
			var err error
			defer func() {
				errc <- err
			}()

			for i, hostedZone := range z {
				newZ := file.NewZone(zName, "")
				newZ.Upstream = h.upstream
				in := &route53.ListResourceRecordSetsInput{
					HostedZoneId: aws.String(hostedZone.id),
					MaxItems:     aws.String("1000"),
				}
				alias := map[string]aliasRecord{}
				err = h.client.ListResourceRecordSetsPagesWithContext(ctx, in,
					func(out *route53.ListResourceRecordSetsOutput, last bool) bool {
						for _, rrs := range out.ResourceRecordSets {
							if err := updateZoneFromRRS(rrs, newZ, alias); err != nil {
								// Maybe unsupported record type. Log and carry on.
								log.Warningf("Failed to process resource record set: %v", err)
							}
						}
						return true
					})
				if err != nil {
					err = fmt.Errorf("failed to list resource records for %v:%v from route53: %v", zName, hostedZone.id, err)
					return
				}

				h.zMu.Lock()
				(*z[i]).z = newZ
				(*z[i]).aliases = alias
				h.zMu.Unlock()
			}

		}(zName, z)
	}
	// Collect errors (if any). This will also sync on all zones updates
	// completion.
	var errs []string
	for i := 0; i < len(h.zones); i++ {
		err := <-errc
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) != 0 {
		return fmt.Errorf("errors updating zones: %v", errs)
	}

	_ = h.updateAliases(ctx)
	return nil
}

// Name implements plugin.Handler.Name.
func (h *Route53) Name() string { return "route53" }
