package loadbalance

import (
	"bufio"
	"bytes"
	"crypto/md5" // #nosec G501 -- used only as a checksum for file change detection (not for security).
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coredns/coredns/plugin"

	"github.com/miekg/dns"
)

type (
	// "weighted-round-robin" policy specific data
	weightedRR struct {
		fileName string
		reload   time.Duration
		md5sum   [md5.Size]byte
		domains  map[string]weights
		randomGen
		mutex sync.Mutex
	}
	// Per domain weights
	weights []*weightItem
	// Weight assigned to an address
	weightItem struct {
		address net.IP
		value   uint8
	}
	// Random uint generator
	randomGen interface {
		randInit()
		randUint(limit uint) uint
	}
)

// Random uint generator
type randomUint struct {
	rn *rand.Rand
}

func (r *randomUint) randInit() {
	r.rn = rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec G404 -- non-cryptographic randomness for load balancing.
}

func (r *randomUint) randUint(limit uint) uint {
	return uint(r.rn.Intn(int(limit)))
}

func weightedShuffle(res *dns.Msg, w *weightedRR) *dns.Msg {
	switch res.Question[0].Qtype {
	case dns.TypeA, dns.TypeAAAA, dns.TypeSRV:
		res.Answer = w.weightedRoundRobin(res.Answer)
		res.Extra = w.weightedRoundRobin(res.Extra)
	}
	return res
}

func weightedOnStartUp(w *weightedRR, stopReloadChan chan bool) error {
	err := w.updateWeights()
	if errors.Is(err, errOpen) && w.reload != 0 {
		log.Warningf("Failed to open weight file:%v. Will try again in %v",
			err, w.reload)
	} else if err != nil {
		return plugin.Error("loadbalance", err)
	}
	// start periodic weight file reload go routine
	w.periodicWeightUpdate(stopReloadChan)
	return nil
}

func createWeightedFuncs(weightFileName string,
	reload time.Duration) *lbFuncs {
	lb := &lbFuncs{
		weighted: &weightedRR{
			fileName:  weightFileName,
			reload:    reload,
			randomGen: &randomUint{},
		},
	}
	lb.weighted.randInit()

	lb.shuffleFunc = func(res *dns.Msg) *dns.Msg {
		return weightedShuffle(res, lb.weighted)
	}

	stopReloadChan := make(chan bool)

	lb.onStartUpFunc = func() error {
		return weightedOnStartUp(lb.weighted, stopReloadChan)
	}

	lb.onShutdownFunc = func() error {
		// stop periodic weigh reload go routine
		close(stopReloadChan)
		return nil
	}
	return lb
}

// Apply weighted round robin policy to the answer
func (w *weightedRR) weightedRoundRobin(in []dns.RR) []dns.RR {
	cname := []dns.RR{}
	address := []dns.RR{}
	mx := []dns.RR{}
	rest := []dns.RR{}
	for _, r := range in {
		switch r.Header().Rrtype {
		case dns.TypeCNAME:
			cname = append(cname, r)
		case dns.TypeA, dns.TypeAAAA:
			address = append(address, r)
		case dns.TypeMX:
			mx = append(mx, r)
		default:
			rest = append(rest, r)
		}
	}

	if len(address) == 0 {
		// no change
		return in
	}

	w.setTopRecord(address)

	out := append(cname, rest...)
	out = append(out, address...)
	out = append(out, mx...)
	return out
}

// Move the next expected address to the first position in the result list
func (w *weightedRR) setTopRecord(address []dns.RR) {
	itop := w.topAddressIndex(address)

	if itop < 0 {
		// internal error
		return
	}

	if itop != 0 {
		// swap the selected top entry with the actual one
		address[0], address[itop] = address[itop], address[0]
	}
}

// Compute the top (first) address index
func (w *weightedRR) topAddressIndex(address []dns.RR) int {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	// Determine the weight value for each address in the answer
	var wsum uint
	type waddress struct {
		index  int
		weight uint8
	}
	weightedAddr := make([]waddress, len(address))
	for i, ar := range address {
		wa := &weightedAddr[i]
		wa.index = i
		wa.weight = 1 // default weight
		var ip net.IP
		switch ar.Header().Rrtype {
		case dns.TypeA:
			ip = ar.(*dns.A).A
		case dns.TypeAAAA:
			ip = ar.(*dns.AAAA).AAAA
		}
		ws := w.domains[ar.Header().Name]
		for _, w := range ws {
			if w.address.Equal(ip) {
				wa.weight = w.value
				break
			}
		}
		wsum += uint(wa.weight)
	}

	// Select the first (top) IP
	sort.Slice(weightedAddr, func(i, j int) bool {
		return weightedAddr[i].weight > weightedAddr[j].weight
	})
	v := w.randUint(wsum)
	var psum uint
	for _, wa := range weightedAddr {
		psum += uint(wa.weight)
		if v < psum {
			return wa.index
		}
	}

	// we should never reach this
	log.Errorf("Internal error: cannot find top address (randv:%v wsum:%v)", v, wsum)
	return -1
}

// Start go routine to update weights from the weight file periodically
func (w *weightedRR) periodicWeightUpdate(stopReload <-chan bool) {
	if w.reload == 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(w.reload)
		for {
			select {
			case <-stopReload:
				return
			case <-ticker.C:
				err := w.updateWeights()
				if err != nil {
					log.Error(err)
				}
			}
		}
	}()
}

// Update weights from weight file
func (w *weightedRR) updateWeights() error {
	reader, err := os.Open(filepath.Clean(w.fileName))
	if err != nil {
		return errOpen
	}
	defer reader.Close()

	// check if the contents has changed
	var buf bytes.Buffer
	tee := io.TeeReader(reader, &buf)
	bytes, err := io.ReadAll(tee)
	if err != nil {
		return err
	}
	md5sum := md5.Sum(bytes) // #nosec G401 -- used only as a checksum for file change detection (not for security).
	if md5sum == w.md5sum {
		// file contents has not changed
		return nil
	}
	w.md5sum = md5sum
	scanner := bufio.NewScanner(&buf)

	// Parse the weight file contents
	domains, err := w.parseWeights(scanner)
	if err != nil {
		return err
	}

	// access to weights must be protected
	w.mutex.Lock()
	w.domains = domains
	w.mutex.Unlock()

	log.Infof("Successfully reloaded weight file %s", w.fileName)
	return nil
}

// Parse the weight file contents
func (w *weightedRR) parseWeights(scanner *bufio.Scanner) (map[string]weights, error) {
	var dname string
	var ws weights
	domains := make(map[string]weights)

	for scanner.Scan() {
		nextLine := strings.TrimSpace(scanner.Text())
		if len(nextLine) == 0 || nextLine[0:1] == "#" {
			// Empty and comment lines are ignored
			continue
		}
		fields := strings.Fields(nextLine)
		switch len(fields) {
		case 1:
			// (domain) name sanity check
			if net.ParseIP(fields[0]) != nil {
				return nil, fmt.Errorf("wrong domain name:\"%s\" in weight file %s. (Maybe a missing weight value?)",
					fields[0], w.fileName)
			}
			dname = fields[0]

			// add the root domain if it is missing
			if dname[len(dname)-1] != '.' {
				dname += "."
			}
			var ok bool
			ws, ok = domains[dname]
			if !ok {
				ws = make(weights, 0)
				domains[dname] = ws
			}
		case 2:
			// IP address and weight value
			ip := net.ParseIP(fields[0])
			if ip == nil {
				return nil, fmt.Errorf("wrong IP address:\"%s\" in weight file %s", fields[0], w.fileName)
			}
			weight, err := strconv.ParseUint(fields[1], 10, 8)
			if err != nil || weight == 0 {
				return nil, fmt.Errorf("wrong weight value:\"%s\" in weight file %s", fields[1], w.fileName)
			}
			witem := &weightItem{address: ip, value: uint8(weight)}
			if dname == "" {
				return nil, fmt.Errorf("missing domain name in weight file %s", w.fileName)
			}
			ws = append(ws, witem)
			domains[dname] = ws
		default:
			return nil, fmt.Errorf("could not parse weight line:\"%s\" in weight file %s", nextLine, w.fileName)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("weight file %s parsing error:%s", w.fileName, err)
	}

	return domains, nil
}
