package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostsInlineLookup(t *testing.T) {
	corefile := `example.org:0 {
		hosts highly_unlikely_to_exist_hosts_file example.org {
			10.0.0.1 example.org
			fallthrough
		}
	}`

	i, udp, _, err := CoreDNSServerAndPorts(corefile)
	if err != nil {
		t.Fatalf("Could not get CoreDNS serving instance: %s", err)
	}
	defer i.Stop()

	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeA)
	resp, err := dns.Exchange(m, udp)
	if err != nil {
		t.Fatal("Expected to receive reply, but didn't")
	}
	// expect answer section with A record in it
	if len(resp.Answer) == 0 {
		t.Fatal("Expected to at least one RR in the answer section, got none")
	}
	if resp.Answer[0].Header().Rrtype != dns.TypeA {
		t.Errorf("Expected RR to A, got: %d", resp.Answer[0].Header().Rrtype)
	}
	if resp.Answer[0].(*dns.A).A.String() != "10.0.0.1" {
		t.Errorf("Expected 10.0.0.1, got: %s", resp.Answer[0].(*dns.A).A.String())
	}
}

func TestHostsReloadAfterFileRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	const name = "app.example.org."
	writeHosts := func(address string) {
		t.Helper()
		require.NoError(t, os.WriteFile(path, []byte(address+" "+name+"\n"), 0600))
	}
	writeHosts("192.0.2.1")
	info, err := os.Stat(path)
	require.NoError(t, err)

	corefile := fmt.Sprintf(`.:0 {
		bind 127.0.0.1
		hosts %q {
			192.0.2.2 inline.example.org
			reload 20ms
			fallthrough
		}
		whoami
	}`, filepath.ToSlash(path))
	instance, udp, _, err := CoreDNSServerAndPorts(corefile)
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, err := range instance.ShutdownCallbacks() {
			assert.NoError(t, err)
		}
		assert.NoError(t, instance.Stop())
	})

	client := dns.Client{Timeout: time.Second}
	checkAnswer := func(t require.TestingT, name, want string) {
		query := new(dns.Msg).SetQuestion(name, dns.TypeA)
		response, _, err := client.Exchange(query, udp)
		require.NoError(t, err)
		require.Equal(t, dns.RcodeSuccess, response.Rcode)
		if want == "" {
			require.Empty(t, response.Answer)
			return
		}
		require.Len(t, response.Answer, 1)
		answer, ok := response.Answer[0].(*dns.A)
		require.True(t, ok, "expected an A record, got %s", response.Answer[0])
		require.Equal(t, want, answer.A.String())
	}
	checkAnswer(t, name, "192.0.2.1")
	require.NoError(t, os.Remove(path))
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		checkAnswer(t, name, "")
	}, 2*time.Second, 20*time.Millisecond)
	checkAnswer(t, "inline.example.org.", "192.0.2.2")

	// Restore a file with the same size and modification time as the deleted one.
	require.NoError(t, os.WriteFile(path+".new", []byte("192.0.2.3 "+name+"\n"), 0600))
	require.NoError(t, os.Chtimes(path+".new", info.ModTime(), info.ModTime()))
	require.NoError(t, os.Rename(path+".new", path))
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		checkAnswer(t, name, "192.0.2.3")
	}, 2*time.Second, 20*time.Millisecond)
}
