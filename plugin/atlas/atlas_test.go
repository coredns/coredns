package atlas

// func TestAtlas(t *testing.T) {
// 	// Create a new Atlas Plugin. Use the test.ErrorHandler as the next plugin.
// 	x := Atlas{Next: test.ErrorHandler()}
//
// 	// Setup a new output buffer that is *not* standard output, so we can check if
// 	// atlas is really being printed.
// 	b := &bytes.Buffer{}
// 	golog.SetOutput(b)
//
// 	ctx := context.TODO()
// 	r := new(dns.Msg)
// 	r.SetQuestion("example.org.", dns.TypeA)
// 	// Create a new Recorder that captures the result, this isn't actually used in this test
// 	// as it just serves as something that implements the dns.ResponseWriter interface.
// 	rec := dnstest.NewRecorder(&test.ResponseWriter{})
//
// 	// Call our plugin directly, and check the result.
// 	x.ServeDNS(ctx, rec, r)
// 	if a := b.String(); !strings.Contains(a, "[INFO] plugin/atlas: atlas") {
// 		t.Errorf("Failed to print '%s', got %s", "[INFO] plugin/atlas: atlas", a)
// 	}
// }
