package dynapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/coredns/coredns/plugin/file"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

func TestApiCreateRecord(t *testing.T) {
	api := newApi(":0")
	zone, err := file.Parse(strings.NewReader(fileMasterZone), "example.mydomain.", "stdin", 0)
	api.dynapiImplementable = append(api.dynapiImplementable)

	if err := api.OnStartup(); err != nil {
		t.Fatalf("Unable to startup the dynapi server: %v", err)
	}
	defer api.OnFinalShutdown()

	// Reconstruct the http address based on the port allocated by operating system.
	address := fmt.Sprintf("http://%s%s", api.ln.Addr().String(), path)

	apiRequest := DynapiRequest{Zone: "example.mydomain", Name: "myserver.example.mydomain", Type: "A", Address: "192.168.0.5"}

	body, err := json.Marshal(apiRequest)
	if err != nil {
		t.Fatal(err)
	}

	response, err := http.Post(address, "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Unable to query %s: %v", address, err)
	}
	if response.StatusCode != 200 {
		t.Errorf("Invalid status code: expecting '200', got '%d'", response.StatusCode)
	}
	content, err := ioutil.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != ok {
		t.Errorf("Invalid response body: expecting '%s', got '%s'", ok, string(content))
	}

	response.Body.Close()
}

const fileMasterZone = `
$ORIGIN .;
$TTL 1h;
. IN         SOA        dns.example.mydomain. exampleadmin@example.mydomain. (
                                2018051901 ; serial
                                10800      ; refresh (3 hours)
                                3600       ; retry (1 hour)
                                604800     ; expire (1 week)
                                300        ; minimum (5 minutes)
                                )
                        NS      dns.example.mydomain.


$ORIGIN example.mydomain.
$TTL 300; 5 minutes

dns	        IN      A       192.168.0.100
example1        IN      A       192.168.0.101
example2        IN      A       192.168.0.102
example3        IN      A       192.168.0.103
`
