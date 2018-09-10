// Package route53 implements a plugin that returns resource records
// from AWS route53.
package route53

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/pkg/log"
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

	zoneNames []string
	client    route53iface.Route53API
	upstream  *upstream.Upstream

	zMu   sync.RWMutex
	zones map[string]*zone
}

type zone struct {
	id string
	z  *file.Zone
}

// New returns new *Route53.
func New(
	ctx context.Context,
	c route53iface.Route53API,
	keys map[string]string,
	up *upstream.Upstream) (*Route53, error) {

	zones := make(map[string]*zone, len(keys))
	zoneNames := make([]string, 0, len(keys))
	for dns, id := range keys {
		_, err := c.ListHostedZonesByNameWithContext(ctx, &route53.ListHostedZonesByNameInput{
			DNSName:      aws.String(dns),
			HostedZoneId: aws.String(id),
		})
		if err != nil {
			return nil, err
		}
		zones[dns] = &zone{id: id, z: file.NewZone(dns, "")}
		zoneNames = append(zoneNames, dns)
	}
	return &Route53{
		client:    c,
		zoneNames: zoneNames,
		zones:     zones,
		upstream:  up,
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
				log.Infof("breaking out of Route53 update loop: %v", ctx.Err())
				return
			case <-time.After(1 * time.Minute):
				if err := h.updateZones(ctx); err != nil && ctx.Err() == nil /* Don't log error if ctx expired. */ {
					log.Errorf("failed to update zones: %v", err)
				}
			}
		}
	}()
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
	m.Authoritative, m.RecursionAvailable = true, true
	var result file.Result
	h.zMu.RLock()
	m.Answer, m.Ns, m.Extra, result = z.z.Lookup(state, qname)
	h.zMu.RUnlock()

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

	state.SizeAndDo(m)
	m, _ = state.Scrub(m)
	w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

func parseSOA(soa string) (ns, mbox string, serial, refresh, retry, expire, minttl int, err error) {
	parts := strings.SplitN(soa, " ", 7)
	if len(parts) != 7 {
		err = fmt.Errorf("failed to parse SOA record: %q", soa)
		return
	}
	ns, mbox = parts[0], parts[1]
	if serial, err = strconv.Atoi(parts[2]); err != nil {
		return
	}
	if refresh, err = strconv.Atoi(parts[3]); err != nil {
		return
	}
	if retry, err = strconv.Atoi(parts[4]); err != nil {
		return
	}
	if expire, err = strconv.Atoi(parts[5]); err != nil {
		return
	}
	if minttl, err = strconv.Atoi(parts[6]); err != nil {
		return
	}
	return
}

func setRRValue(rr dns.RR, hdr *dns.RR_Header, value string) error {
	switch rr.(type) {
	case *dns.A:
		rr.(*dns.A).Hdr = *hdr
		rr.(*dns.A).A = net.ParseIP(value).To4()
	case *dns.AAAA:
		rr.(*dns.AAAA).Hdr = *hdr
		rr.(*dns.AAAA).AAAA = net.ParseIP(value).To16()
	case *dns.CNAME:
		rr.(*dns.CNAME).Hdr = *hdr
		rr.(*dns.CNAME).Target = dns.Fqdn(value)
	case *dns.PTR:
		rr.(*dns.PTR).Hdr = *hdr
		rr.(*dns.PTR).Ptr = value
	case *dns.SOA:
		rr.(*dns.SOA).Hdr = *hdr
		parts := strings.SplitN(value, " ", 7)
		if len(parts) != 7 {
			return fmt.Errorf("failed to parse SOA record: %q", value)
		}
		ns, mbox, serial, refresh, retry, expire, minttl, err := parseSOA(value)
		if err != nil {
			return err
		}
		rr.(*dns.SOA).Ns = dns.Fqdn(ns)
		rr.(*dns.SOA).Mbox = dns.Fqdn(mbox)
		rr.(*dns.SOA).Serial = uint32(serial)
		rr.(*dns.SOA).Refresh = uint32(refresh)
		rr.(*dns.SOA).Retry = uint32(retry)
		rr.(*dns.SOA).Expire = uint32(expire)
		rr.(*dns.SOA).Minttl = uint32(minttl)
	case *dns.NS:
		rr.(*dns.NS).Hdr = *hdr
		rr.(*dns.NS).Ns = value
	default:
		return fmt.Errorf("type not supported: %v", reflect.TypeOf(rr))
	}
	return nil
}

func updateZoneFromRRS(rrs *route53.ResourceRecordSet, z *file.Zone) error {
	t, ok := dns.StringToType[aws.StringValue(rrs.Type)]
	if !ok {
		return fmt.Errorf("unsupported record type: %s", aws.StringValue(rrs.Type))
	}
	hdr := dns.RR_Header{
		Name:   aws.StringValue(rrs.Name),
		Rrtype: t,
		Class:  dns.ClassINET,
		Ttl:    uint32(aws.Int64Value(rrs.TTL)),
	}
	for _, rr := range rrs.ResourceRecords {
		var r dns.RR
		r = dns.TypeToRR[t]()
		if err := setRRValue(r, &hdr, aws.StringValue(rr.Value)); err != nil {
			return fmt.Errorf("failed to set answer for %v: %v", r, err)
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
		go func(zName string) {
			var err error
			defer func() {
				errc <- err
			}()

			newZ := file.NewZone(zName, "")
			newZ.Upstream = *h.upstream

			in := &route53.ListResourceRecordSetsInput{
				HostedZoneId: aws.String(z.id),
			}
			err = h.client.ListResourceRecordSetsPagesWithContext(ctx, in,
				func(out *route53.ListResourceRecordSetsOutput, last bool) bool {
					for _, rrs := range out.ResourceRecordSets {
						if err := updateZoneFromRRS(rrs, newZ); err != nil {
							// Maybe unsupported record type. Log and carry on.
							log.Warningf("failed to process resource record set: %v", err)
						}
					}
					return true
				})
			if err != nil {
				err = fmt.Errorf("failed to list resource records for %v:%v from route53: %v", zName, z.id, err)
				return
			}

			h.zMu.Lock()
			z.z = newZ
			h.zMu.Unlock()
		}(zName)
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
	return nil
}

// Name implements plugin.Handler.Name.
func (h *Route53) Name() string { return "route53" }
