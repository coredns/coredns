// Package trace implements OpenTracing-based tracing
package rlc

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func newNameEntry(name string) *NameEntry {
	return &NameEntry{
		Name:       name,
		LastAccess: timestamppb.Now(),
		Hits:       0,
	}
}

type NameMap map[string]*NameEntry
type NameList []*NameEntry

func (l NameList) String() string {
	r := strings.Builder{}
	r.WriteString("[")
	for i := 0; i < len(l); i++ {
		r.WriteString(l[i].Name)
		if i < len(l)-1 {
			r.WriteString(", ")
		}
	}
	r.WriteString("]")
	return r.String()
}

func (p NameList) Len() int { return len(p) }
func (p NameList) Less(i, j int) bool {
	if p[i].LastAccess == p[j].LastAccess {
		if p[i].Hits == p[j].Hits {
			return p[i].Name > p[j].Name
		}
		return p[i].Hits < p[j].Hits
	}
	return p[i].LastAccess.AsTime().After(p[j].LastAccess.AsTime())
}
func (p NameList) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

// return the list of names sorted by last access, hits and then name
func (m NameMap) getNames() NameList {
	result := make(NameList, len(m))
	i := 0
	for _, v := range m {
		result[i] = v
		i++
	}
	sort.Sort(result)
	return result
}

func newPTREntry(ip net.IP, lastAccess time.Time) *PTREntry {
	return &PTREntry{
		Ip:         ip,
		LastAccess: timestamppb.New(lastAccess),
		Names:      make(NameMap),
	}
}

func (p *PTREntry) AddName(name string, timestamp time.Time) bool {
	if existing, ok := p.Names[name]; ok {
		existing.LastAccess = timestamppb.New(timestamp)
		return false
	} else {
		newEntry := newNameEntry(name)
		p.Names[name] = newEntry
		return true
	}
}

func (p *PTREntry) TouchName(name string, timestamp time.Time) bool {
	if existing, ok := p.Names[name]; ok {
		existing.Hits++
		existing.LastAccess = timestamppb.New(timestamp)
		return false
	} else {
		newEntry := newNameEntry(name)
		newEntry.Hits++
		p.Names[name] = newEntry
		return true
	}
}

func (p *PTREntry) Dump() string {
	return fmt.Sprintf("%s %s", net.IP(p.Ip).String(), NameMap(p.Names).getNames().String())
}
