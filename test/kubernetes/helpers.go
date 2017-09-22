// +build k8s k8s1 k8s2 k8sauto k8sexclust

package kubernetes

import (
	"log"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/test"
	intTest "github.com/coredns/coredns/test"

	"errors"
	"github.com/mholt/caddy"
	"github.com/miekg/dns"
	"io/ioutil"
	"time"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

// doIntegrationTests executes test cases
func doIntegrationTests(t *testing.T, testCases []test.Case, namespace string) {

	clientName := "coredns-test-client"
	err := startClientPod(namespace, clientName)
	if err != nil {
		t.Fatalf("Failed to start client pod: %s", err)
	}
	for _, tc := range testCases {
		digCmd := "dig -t " + dns.TypeToString[tc.Qtype] + " " + tc.Qname + " +search +showsearch +time=10 +tries=6"

		// attach to client and execute query.
		var cmdout string
		tries := 3
		for {
			cmdout, err = kubectl("-n " + namespace + " exec " + clientName + " -- " + digCmd)
			if err == nil {
				break
			}
			tries = tries - 1
			if tries == 0 {
				t.Errorf("Failed to execute query: %s\n Got Error: %s\n Logs: %s", digCmd, err, corednsLogs())
			}
			time.Sleep(500 * time.Millisecond)
		}
		results, err := test.ParseDigResponse(cmdout)
		if err != nil {
			t.Errorf("Failed to parse result: (%s) '%s'", err, cmdout)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 query attempt, observed %v.", len(results))
		}
		res := results[0]

		// Before sort and check, make sure that CNAMES do not appear after their target records.
		test.CNAMEOrder(t, res)

		sort.Sort(test.RRSet(tc.Answer))
		sort.Sort(test.RRSet(tc.Ns))
		sort.Sort(test.RRSet(tc.Extra))
		test.SortAndCheck(t, res, tc)

		println("BEGIN COREDNS LOGS : " + dns.TypeToString[tc.Qtype] + " " + tc.Qname)
		println(corednsLogs())
		println("END COREDNS LOGS")
	}
}

func startClientPod(namespace, clientName string) error {
	_, err := kubectl("-n " + namespace + " run " + clientName + " --image=infoblox/dnstools --restart=Never -- -c 'while [ 1 ]; do sleep 100; done'")
	if err != nil {
		// ignore error (pod already running)
		return nil
	}
	maxWait := 60 // 60 seconds
	for {
		o, _ := kubectl("-n " + namespace + "  get pod " + clientName)
		if strings.Contains(o, "Running") {
			return nil
		}
		time.Sleep(time.Second)
		maxWait = maxWait - 1
		if maxWait == 0 {
			break
		}
	}
	return errors.New("Timeout waiting for " + clientName + " to be ready.")

}

// upstreamServer starts a local instance of coredns with the given zone file
func upstreamServer(t *testing.T, zone, zoneFile string) (func(), *caddy.Instance, string) {
	upfile, rmFunc, err := intTest.TempFile(os.TempDir(), zoneFile)
	if err != nil {
		t.Fatalf("Could not create file for CNAME upstream lookups: %s", err)
	}
	upstreamCorefile := `.:0 {
    file ` + upfile + ` ` + zone + `
    bind ` + locaIP().String() + `
}`
	server, udp, _, err := intTest.CoreDNSServerAndPorts(upstreamCorefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	return rmFunc, server, udp
}

// localIP returns the local system's first ipv4 non-loopback address
func locaIP() net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, addr := range addrs {
		ip, _, _ := net.ParseCIDR(addr.String())
		ip = ip.To4()
		if ip == nil || ip.IsLoopback() {
			continue
		}
		return ip
	}
	return nil
}

// headlessAResponse returns the answer to an A request for the specific name and namespace.
func headlessAResponse(qname, name, namespace string) []dns.RR {
	rr := []dns.RR{}

	str, err := endpointIPs(name, namespace)
	if err != nil {
		log.Fatal("Error running kubectl command: ", err.Error())
	}
	result := strings.Split(string(str), " ")
	lr := len(result)

	for i := 0; i < lr; i++ {
		rr = append(rr, test.A(qname+"    303    IN      A   "+result[i]))
	}
	return rr
}

// srvResponse returns the answer to a SRV request for the specific name and namespace
// qtype is the type of answer to generate, eg: TypeSRV (for answer section) or TypeA (for extra section).
func srvResponse(qname string, qtype uint16, name, namespace string) []dns.RR {
	rr := []dns.RR{}

	str, err := endpointIPs(name, namespace)

	if err != nil {
		log.Fatal("Error running kubectl command: ", err.Error())
	}
	result := strings.Split(string(str), " ")
	lr := len(result)

	for i := 0; i < lr; i++ {
		ip := strings.Replace(result[i], ".", "-", -1)
		t := strconv.Itoa(100 / (lr + 1))

		switch qtype {
		case dns.TypeA:
			rr = append(rr, test.A(ip+"."+name+"."+namespace+".svc.cluster.local.	303	IN	A	"+result[i]))
		case dns.TypeSRV:
			if name == "headless-svc" {
				rr = append(rr, test.SRV(qname+"   303    IN    SRV 0 "+t+" 1234  "+ip+"."+name+"."+namespace+".svc.cluster.local."))
			} else {
				rr = append(rr, test.SRV(qname+"   303    IN    SRV 0 "+t+" 443  "+ip+"."+name+"."+namespace+".svc.cluster.local."))
				rr = append(rr, test.SRV(qname+"   303    IN    SRV 0 "+t+" 80  "+ip+"."+name+"."+namespace+".svc.cluster.local."))
			}
		}
	}
	return rr
}

//endpointIPs retrieves the IP address for a given name and namespace by parsing json using kubectl command
func endpointIPs(name, namespace string) (cmdOut []byte, err error) {
	cmdout, err := kubectl(string(" -n " + namespace + " get endpoints " + name + " -o jsonpath={.subsets[*].addresses[*].ip}"))
	return []byte(cmdout), err
}

// loadCorefile calls loadCorefileAndZonefile
func loadCorefile(corefile string) error {
	return loadCorefileAndZonefile(corefile, "")
}

// loadCorefileAndZonefile constructs and configmap defining files for the corefile and zone,
// forces the coredns pod to load the new configmap, and waits for the coredns pod to be ready.
func loadCorefileAndZonefile(corefile, zonefile string) error {

	// apply configmap yaml
	yamlString := configmap + "\n"
	yamlString += "  Corefile: |\n" + prepForConfigMap(corefile)
	yamlString += "  Zonefile: |\n" + prepForConfigMap(zonefile)

	file, rmFunc, err := intTest.TempFile(os.TempDir(), yamlString)
	if err != nil {
		return err
	}
	defer rmFunc()
	_, err = kubectl("apply -f " + file)
	if err != nil {
		return err
	}

	// force coredns pod reload the config
	kubectl("-n kube-system delete pods -l k8s-app=coredns")

	// wait for coredns to be ready before continuing
	maxWait := 30 // 15 seconds (each failed check sleeps 0.5 seconds)
	running := 0
	for {
		o, _ := kubectl("-n kube-system get pods -l k8s-app=coredns")
		if strings.Contains(o, "Running") {
			running += 1
		}
		if running >= 2 {
			// give coredns a chance to read its config before declaring victory
			break
		}
		time.Sleep(500 * time.Millisecond)
		maxWait = maxWait - 1
		if maxWait == 0 {
			//println(o)
			logs := corednsLogs()
			return errors.New("Timeout waiting for coredns to be ready. coredns log: " + logs)
		}
	}
	return nil
}

func corednsLogs() string {
	name, _ := kubectl("-n kube-system get pods -l k8s-app=coredns -o jsonpath='{.items[0].metadata.name}'")
	logs, _ := kubectl("-n kube-system logs " + name)
	return (logs)
}

// prepForConfigMap returns a config prepared for inclusion in a configmap definition
func prepForConfigMap(config string) string {
	var configOut string
	lines := strings.Split(config, "\n")
	for _, line := range lines {
		// replace all tabs with spaces
		line = strings.Replace(line, "\t", "  ", -1)
		// indent line with 4 addtl spaces
		configOut += "    " + line + "\n"
	}
	return configOut
}

// kubectl executes the kubectl command with the given arguments
func kubectl(args string) (result string, err error) {
	kctl := os.Getenv("KUBECTL")

	if kctl == "" {
		kctl = "kubectl"
	}
	cmdOut, err := exec.Command("sh", "-c", kctl+" "+args).CombinedOutput()
	if err != nil {
		return "", errors.New("Error: " + string(cmdOut) + " for command: " + kctl + " " + args)
	}
	return string(cmdOut), nil
}

// configmap is the header used for defining the coredns configmap
const configmap = `apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
data:`

// exampleNet is an example upstream zone file
const exampleNet = `; example.net. test file for cname tests
example.net.          IN      SOA     ns.example.net. admin.example.net. 2015082541 7200 3600 1209600 3600
example.net. IN A 13.14.15.16
`
