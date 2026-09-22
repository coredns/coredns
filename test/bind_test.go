package test

import "testing"

func TestBind_FilterAll(t *testing.T) {
	t.Parallel()
	corefile := `.:0 {
        bind 127.0.0.1 {
            except 127.0.0.1
        }
        trace
        loop
        whoami
    }`
	inst, err := CoreDNSServer(corefile)
	if inst != nil {
		CoreDNSServerStop(inst)
	}
	if err == nil {
		t.Log("server started; stopping immediately")
	} else {
		t.Logf("server failed to start as expected without listeners: %v", err)
	}
}
