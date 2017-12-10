package template

import (
	"bytes"
	"context"
	"regexp"
	"strconv"

	gotmpl "text/template"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

// Handler is a plugin handler that takes a query and templates a response.
type Handler struct {
	Next      plugin.Handler
	Templates []template
}

type template struct {
	rcode      int
	class      uint16
	qtype      uint16
	regex      []*regexp.Regexp
	answer     []*gotmpl.Template
	additional []*gotmpl.Template
}

type templateData struct {
	Name     string
	Match    []string
	Group    map[string]string
	Class    string
	Type     string
	Message  *dns.Msg
	Question *dns.Question
}

// ServeDNS implements the plugin.Handler interface.
func (h Handler) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	for _, template := range h.Templates {
		data, match := template.match(r)
		if !match {
			continue
		}

		if template.rcode == dns.RcodeServerFailure {
			return template.rcode, nil
		}

		msg := new(dns.Msg)
		state := request.Request{W: w, Req: r}
		msg.SetReply(r)
		msg.Authoritative, msg.RecursionAvailable, msg.Compress = true, true, true
		msg.Rcode = template.rcode

		for _, answer := range template.answer {
			rr, err := executeRRTemplate(answer, data)
			if err != nil {
				return dns.RcodeServerFailure, err
			}
			msg.Answer = append(msg.Answer, *rr)
		}
		for _, additional := range template.additional {
			rr, err := executeRRTemplate(additional, data)
			if err != nil {
				return dns.RcodeServerFailure, err
			}
			msg.Extra = append(msg.Extra, *rr)
		}

		state.SizeAndDo(msg)
		w.WriteMsg(msg)
		return template.rcode, nil
	}
	return plugin.NextOrFailure(h.Name(), h.Next, ctx, w, r)
}

// Name implements the plugin.Handler interface.
func (h Handler) Name() string {
	return "template"
}

func executeRRTemplate(template *gotmpl.Template, data templateData) (*dns.RR, error) {
	buffer := &bytes.Buffer{}
	err := template.Execute(buffer, data)
	if err != nil {
		return nil, err
	}
	rrDefinition := buffer.String()
	rr, err := dns.NewRR(rrDefinition)
	if err != nil {
		return nil, err
	}
	return &rr, nil
}

func (t template) match(r *dns.Msg) (templateData, bool) {
	for _, q := range r.Question {
		if t.class != dns.ClassANY && q.Qclass != dns.ClassANY && q.Qclass != t.class {
			continue
		}
		if t.qtype != dns.TypeANY && q.Qtype != dns.TypeANY && q.Qtype != t.qtype {
			continue
		}
		for _, regex := range t.regex {
			if !regex.MatchString(q.Name) {
				continue
			}

			data := templateData{}
			data.Name = q.Name
			data.Question = &q
			data.Message = r
			data.Class = dns.ClassToString[q.Qclass]
			data.Type = dns.TypeToString[q.Qtype]

			matches := regex.FindStringSubmatch(q.Name)
			data.Match = make([]string, len(matches))
			data.Group = make(map[string]string)
			groupNames := regex.SubexpNames()
			for i, m := range matches {
				data.Match[i] = m
				data.Group[strconv.Itoa(i)] = m
			}
			for i, m := range matches {
				if len(groupNames[i]) > 0 {
					data.Group[groupNames[i]] = m
				}
			}

			return data, true
		}
	}
	return templateData{}, false
}
