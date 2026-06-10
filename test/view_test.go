package test

import (
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/test"

	"github.com/miekg/dns"
)

func TestView(t *testing.T) {
	// Hack to get an available port - We spin up a temporary dummy coredns on :0 to get the port number, then we re-use
	// that one port consistently across all server blocks.
	corefile := `example.org:0 {
		erratic
	}`
	tmp, addr, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}

	port := addr[strings.LastIndex(addr, ":")+1:]

	// Corefile with test views
	corefile = `
      # split-type config: splits quries for A/AAAA into separate views
      split-type:` + port + ` {
		view test-view-a {
          expr type() == 'A'
	    }
        hosts {
          1.2.3.4 test.split-type
        }
      }
      split-type:` + port + ` {
		view test-view-aaaa {
          expr type() == 'AAAA'
	    }
        hosts {
          1:2:3::4 test.split-type
        }
      }

      # split-name config: splits queries into separate views based on first label in query name ("one", "two")
      split-name:` + port + ` {
		view test-view-1 {
          expr name() matches '^one\\..*\\.split-name\\.$'
	    }
        hosts {
          1.1.1.1 one.test.split-name one.test.test.test.split-name
        }
      }
      split-name:` + port + ` {
		view test-view-2 {
          expr name() matches '^two\\..*\\.split-name\\.$'
	    }
        hosts {
          2.2.2.2 two.test.split-name two.test.test.test.split-name
        }
      }
      split-name:` + port + ` {
        hosts {
          3.3.3.3 default.test.split-name
        }
      }

     # metadata config: verifies that metadata is properly collected by the server,
     # and that metadata function correctly looks up the value of the metadata.
     metadata:` + port + ` {
       metadata
       view test-view-meta1 {
         # This is never true
         expr metadata('view/name') == 'not-the-view-name'
	   }
       hosts {
         1.1.1.1 test.metadata
       }
     }
     metadata:` + port + ` {
       view test-view-meta2 {
         # This is never true. The metadata plugin is not enabled in this server block so the metadata function returns
         # an empty string
         expr metadata('view/name') == 'test-view-meta2'
	   }
       hosts {
         2.2.2.2 test.metadata
       }
     }
     metadata:` + port + ` {
       metadata
       view test-view-meta3 {
         # This is always true.  Queries in the zone 'metadata.' should always be served using this view.
         expr metadata('view/name') == 'test-view-meta3'
	   }
       hosts {
         2.2.2.2 test.metadata
       }
     }
     metadata:` + port + ` {
       # This block should never be reached since the prior view in the same zone is always true
       hosts {
         3.3.3.3 test.metadata
       }
     }
    `

	i, addr, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	// there are multiple sever blocks, but they are all on the same port, so it's a single server instance to stop
	defer i.Stop()
	// stop the temporary instance before starting tests.
	tmp.Stop()

	viewTest(t, "split-type A", addr, "test.split-type.", dns.TypeA, dns.RcodeSuccess,
		[]dns.RR{test.A("test.split-type.	303	IN	A	1.2.3.4")})

	viewTest(t, "split-type AAAA", addr, "test.split-type.", dns.TypeAAAA, dns.RcodeSuccess,
		[]dns.RR{test.AAAA("test.split-type.	303	IN	AAAA	1:2:3::4")})

	viewTest(t, "split-name one.test.test.test.split-name", addr, "one.test.test.test.split-name.", dns.TypeA, dns.RcodeSuccess,
		[]dns.RR{test.A("one.test.test.test.split-name.	303	IN	A	1.1.1.1")})

	viewTest(t, "split-name one.test.split-name", addr, "one.test.split-name.", dns.TypeA, dns.RcodeSuccess,
		[]dns.RR{test.A("one.test.split-name.	303	IN	A	1.1.1.1")})

	viewTest(t, "split-name two.test.test.test.split-name", addr, "two.test.test.test.split-name.", dns.TypeA, dns.RcodeSuccess,
		[]dns.RR{test.A("two.test.test.test.split-name.	303	IN	A	2.2.2.2")})

	viewTest(t, "split-name two.test.split-name", addr, "two.test.split-name.", dns.TypeA, dns.RcodeSuccess,
		[]dns.RR{test.A("two.test.split-name.	303	IN	A	2.2.2.2")})

	viewTest(t, "split-name default.test.split-name", addr, "default.test.split-name.", dns.TypeA, dns.RcodeSuccess,
		[]dns.RR{test.A("default.test.split-name.	303	IN	A	3.3.3.3")})

	viewTest(t, "metadata test.metadata", addr, "test.metadata.", dns.TypeA, dns.RcodeSuccess,
		[]dns.RR{test.A("test.metadata.	303	IN	A	2.2.2.2")})
}

func viewTest(t *testing.T, testName, addr, qname string, qtype uint16, expectRcode int, expectAnswers []dns.RR) {
	t.Helper()
	t.Run(testName, func(t *testing.T) {
		m := new(dns.Msg)

		m.SetQuestion(qname, qtype)
		resp, err := dns.Exchange(m, addr)
		if err != nil {
			t.Fatalf("Expected to receive reply, but didn't: %s", err)
		}

		tc := test.Case{
			Qname: qname, Qtype: qtype,
			Rcode:  expectRcode,
			Answer: expectAnswers,
		}

		err = test.SortAndCheck(resp, tc)
		if err != nil {
			t.Error(err)
		}
	})
}

func TestViewServerBlockOrdering(t *testing.T) {
	// Verify that filtered and unfiltered server blocks sharing the same
	// zone/port start without error regardless of declaration order, and
	// that queries are routed correctly.
	//
	// Declaration order matters: server blocks are evaluated top-to-bottom
	// and the first block whose filter matches (or has no filter) handles
	// the query. An unfiltered block declared before a filtered block will
	// catch all queries, shadowing the filtered block.
	//
	// All scenarios use distinct zone names and are combined into a single
	// Corefile/server instance to avoid stop/start races with SO_REUSEPORT.
	//
	// See https://github.com/coredns/coredns/issues/7733

	corefile := `example.org:0 {
		erratic
	}`
	tmp, addr, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	port := addr[strings.LastIndex(addr, ":")+1:]

	corefile = `
      # Scenario: filtered blocks before unfiltered (order-test)
      # Filtered blocks are listed first, unfiltered catch-all is last.
      # Each view handles its matching queries; the catch-all handles the rest.
      order-test:` + port + ` {
        view v-a {
          expr type() == 'A'
        }
        hosts {
          1.2.3.4 test.order-test
        }
      }
      order-test:` + port + ` {
        view v-aaaa {
          expr type() == 'AAAA'
        }
        hosts {
          1:2:3::4 test.order-test
        }
      }
      order-test:` + port + ` {
        hosts {
          5.6.7.8 test.order-test
        }
      }

      # Scenario: unfiltered block first (order-test2)
      # Unfiltered block is declared first. It matches all queries, so the
      # filtered block below it is effectively shadowed.
      order-test2:` + port + ` {
        hosts {
          5.6.7.8 test.order-test2
        }
      }
      order-test2:` + port + ` {
        view v-a {
          expr type() == 'A'
        }
        hosts {
          1.2.3.4 test.order-test2
        }
      }

      # Scenario: unfiltered block in the middle (order-test3)
      # The first view catches A queries. The unfiltered block catches
      # everything else, shadowing the second filtered block.
      order-test3:` + port + ` {
        view v-a {
          expr type() == 'A'
        }
        hosts {
          1.2.3.4 test.order-test3
        }
      }
      order-test3:` + port + ` {
        hosts {
          5.6.7.8 test.order-test3
        }
      }
      order-test3:` + port + ` {
        view v-aaaa {
          expr type() == 'AAAA'
        }
        hosts {
          1:2:3::4 test.order-test3
        }
      }

      # Scenario: no catch-all (order-test4)
      # Only filtered blocks. Queries not matching any view get REFUSED.
      order-test4:` + port + ` {
        view v-a {
          expr type() == 'A'
        }
        hosts {
          1.2.3.4 test.order-test4
        }
      }
      order-test4:` + port + ` {
        view v-aaaa {
          expr type() == 'AAAA'
        }
        hosts {
          1:2:3::4 test.order-test4
        }
      }
    `

	i, addr, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not start server: %s", err)
	}
	defer i.Stop()
	tmp.Stop()

	// filtered blocks before unfiltered — views handle matching types, catch-all handles the rest
	viewTest(t, "A routed to v-a", addr, "test.order-test.", dns.TypeA, dns.RcodeSuccess,
		[]dns.RR{test.A("test.order-test.	303	IN	A	1.2.3.4")})
	viewTest(t, "AAAA routed to v-aaaa", addr, "test.order-test.", dns.TypeAAAA, dns.RcodeSuccess,
		[]dns.RR{test.AAAA("test.order-test.	303	IN	AAAA	1:2:3::4")})
	viewTest(t, "MX falls through to catch-all", addr, "test.order-test.", dns.TypeMX, dns.RcodeSuccess, nil)

	// unfiltered block first — catches everything, view is shadowed
	viewTest(t, "A hits unfiltered", addr, "test.order-test2.", dns.TypeA, dns.RcodeSuccess,
		[]dns.RR{test.A("test.order-test2.	303	IN	A	5.6.7.8")})

	// unfiltered block in the middle — v-a catches A, unfiltered shadows v-aaaa
	viewTest(t, "A routed to v-a (middle)", addr, "test.order-test3.", dns.TypeA, dns.RcodeSuccess,
		[]dns.RR{test.A("test.order-test3.	303	IN	A	1.2.3.4")})
	viewTest(t, "AAAA hits unfiltered (shadowed view)", addr, "test.order-test3.", dns.TypeAAAA, dns.RcodeSuccess, nil)

	// no catch-all — matching view works, non-matching type is REFUSED
	viewTest(t, "A routed to v-a (no catch-all)", addr, "test.order-test4.", dns.TypeA, dns.RcodeSuccess,
		[]dns.RR{test.A("test.order-test4.	303	IN	A	1.2.3.4")})
	viewTest(t, "MX refused without catch-all", addr, "test.order-test4.", dns.TypeMX, dns.RcodeRefused, nil)
}
