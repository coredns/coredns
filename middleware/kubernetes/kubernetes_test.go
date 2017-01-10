package kubernetes

import "testing"

// Test data for TestSymbolContainsWildcard cases.
var testdataSymbolContainsWildcard = []struct {
	Symbol         string
	ExpectedResult bool
}{
	{"mynamespace", false},
	{"*", true},
	{"any", true},
	{"my*space", true},
	{"*space", true},
	{"myname*", true},
}

func TestSymbolContainsWildcard(t *testing.T) {
	for _, example := range testdataSymbolContainsWildcard {
		actualResult := symbolContainsWildcard(example.Symbol)
		if actualResult != example.ExpectedResult {
			t.Errorf("Expected SymbolContainsWildcard result '%v' for example string='%v'. Instead got result '%v'.", example.ExpectedResult, example.Symbol, actualResult)
		}
	}
}

func TestParseRequest(t *testing.T) {
	k := Kubernetes{Zones: []string{"inter.webs.test"}}

	// Test a valid SRV request
	query := "_http._tcp.webs.mynamespace.svc.inter.webs.test."
	r, e := k.parseRequest(query, "SRV")
	if e != nil {
		t.Errorf("Expected no error from parseRequest(%v). Instead got '%v'.", query, e)
	}
	if expected := "http"; r.port != expected {
		t.Errorf("Expected parseRequest(%v) to get port == \"%v\". Instead got \"%v\".", query, expected, r.port)
	}
	if expected := "tcp"; r.protocol != expected {
		t.Errorf("Expected parseRequest(%v) to get protocol == \"%v\". Instead got \"%v\".", query, expected, r.protocol)
	}
	if expected := ""; r.endpoint != expected {
		t.Errorf("Expected parseRequest(%v) to get endpoint == \"%v\". Instead got \"%v\".", query, expected, r.endpoint)
	}
	if expected := "webs"; r.service != expected {
		t.Errorf("Expected parseRequest(%v) to get service == \"%v\". Instead got \"%v\".", query, expected, r.service)
	}
	if expected := "mynamespace"; r.namespace != expected {
		t.Errorf("Expected parseRequest(%v) to get namespace == \"%v\". Instead got \"%v\".", query, expected, r.namespace)
	}
	if expected := "svc"; r.typeName != expected {
		t.Errorf("Expected parseRequest(%v) to get typeName == \"%v\". Instead got \"%v\".", query, expected, r.typeName)
	}
	if expected := "inter.webs.test"; r.zone != expected {
		t.Errorf("Expected parseRequest(%v) to get zone == \"%v\". Instead got \"%v\".", query, expected, r.zone)
	}

	// Test wildcard acceptance
	query = "*.any.*.any.svc.inter.webs.test."
	r, e = k.parseRequest(query, "SRV")
	if e != nil {
		t.Errorf("Expected no error from parseRequest(%v). Instead got '%v'.", query, e)
	}
	if expected := "*"; r.port != expected {
		t.Errorf("Expected parseRequest(%v) to get port == \"%v\". Instead got '%v'.", query, expected, r.port)
	}
	if expected := "any"; r.protocol != expected {
		t.Errorf("Expected parseRequest(%v) to get protocol == \"%v\". Instead got '%v'.", query, expected, r.protocol)
	}
	if expected := ""; r.endpoint != expected {
		t.Errorf("Expected parseRequest(%v) to get endpoint == \"%v\". Instead got \"%v\".", query, expected, r.endpoint)
	}
	if expected := "*"; r.service != expected {
		t.Errorf("Expected parseRequest(%v) to get service == \"%v\". Instead got \"%v\".", query, expected, r.service)
	}
	if expected := "any"; r.namespace != expected {
		t.Errorf("Expected parseRequest(%v) to get namespace == \"%v\". Instead got '%v'.", query, expected, r.namespace)
	}
	if expected := "svc"; r.typeName != expected {
		t.Errorf("Expected parseRequest(%v) to get typeName == \"%v\". Instead got '%v'.", query, expected, r.typeName)
	}
	if expected := "inter.webs.test"; r.zone != expected {
		t.Errorf("Expected parseRequest(%v) to get zone == \"%v\". Instead got '%v'.", query, expected, r.zone)
	}

	// Test A request of endpoint
	query = "1-2-3-4.webs.mynamespace.svc.inter.webs.test."
	r, e = k.parseRequest(query, "A")
	if e != nil {
		t.Errorf("Expected no error from parseRequest(%v). Instead got '%v'.", query, e)
	}
	if expected := ""; r.port != expected {
		t.Errorf("Expected parseRequest(%v) to get port == \"%v\". Instead got \"%v\".", query, expected, r.port)
	}
	if expected := ""; r.protocol != expected {
		t.Errorf("Expected parseRequest(%v) to get protocol == \"%v\". Instead got \"%v\".", query, expected, r.protocol)
	}
	if expected := "1-2-3-4"; r.endpoint != expected {
		t.Errorf("Expected parseRequest(%v) to get endpoint == \"%v\". Instead got \"%v\".", query, expected, r.endpoint)
	}
	if expected := "webs"; r.service != expected {
		t.Errorf("Expected parseRequest(%v) to get service == \"%v\". Instead got \"%v\".", query, expected, r.service)
	}
	if expected := "mynamespace"; r.namespace != expected {
		t.Errorf("Expected parseRequest(%v) to get namespace == \"%v\". Instead got \"%v\".", query, expected, r.namespace)
	}
	if expected := "svc"; r.typeName != expected {
		t.Errorf("Expected parseRequest(%v) to get typeName == \"%v\". Instead got \"%v\".", query, expected, r.typeName)
	}
	if expected := "inter.webs.test"; r.zone != expected {
		t.Errorf("Expected parseRequest(%v) to get zone == \"%v\". Instead got \"%v\".", query, expected, r.zone)
	}

	// Test invalid A request (with port protocol)
	query = "_http._tcp.webs.mynamespace.svc.inter.webs.test."
	_, e = k.parseRequest(query, "A")
	if e == nil {
		t.Errorf("Expected error from parseRequest(%v).", query)
	}

	// Test invalid SRV request (without port protocol)
	query = "webs.mynamespace.svc.inter.webs.test."
	_, e = k.parseRequest(query, "SRV")
	if e == nil {
		t.Errorf("Expected error from parseRequest(%v).", query)
	}

	// Test invalid SRV request (with invalid protocol)
	query = "_http._pcp.webs.mynamespace.svc.inter.webs.test."
	_, e = k.parseRequest(query, "SRV")
	if e == nil {
		t.Errorf("Expected error from parseRequest(%v).", query)
	}

	// Test invalid SRV request (with invalid wildcards)
	query = "_*._*.webs.mynamespace.svc.inter.webs.test."
	_, e = k.parseRequest(query, "SRV")
	if e == nil {
		t.Errorf("Expected error from parseRequest(%v).", query)
	}
}
