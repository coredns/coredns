package test

/*
The functions contained in the file are meant to test CoreDNS *process* reloads. This is a bit
tricky to do with normal go test, because you want to start a new process and keep track of that.
What this code allows you to do is using the coredns binary and test reloads.
*/

import (
	"io"
	"io/ioutil"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Reload is called with two Corefile. The first one is used to start CoreDNS, the second one replaces
// the Corefile on disk and then we will send the reload signal and see if we error.
func Reload(coredns, core1, core2 string) error {
	buf := make([]byte, 4096)

	err := writeCorefile(core1)
	if err != nil {
		return err
	}

	// First Corefile with core1
	server, out, err := coreStart(coredns)
	n, _ := out.Read(buf)
	buf = buf[:n]

	if err != nil {
		// Include buf
		return err
	}

	if err = writeCorefile(core2); err != nil {
		return err
	}

	// Wait for fully started.
	time.Sleep(500 * time.Millisecond)

	if err := server.Process.Signal(syscall.SIGUSR1); err != nil {
		return err
	}

	err = nil
	go func() {
		err := server.Wait()
		if err != nil {
			// yech, but so be it, check actual error text returned.
			if strings.Contains(err.Error(), "signal: killed") {
				err = nil
				return
			}
			if strings.Contains(string(buf), "KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be defined") {
				err = nil
				return
			}
		}
	}()
	time.Sleep(500 * time.Millisecond)
	server.Process.Kill()

	return err
}

const conffile = "/tmp/corefile-reload"

func writeCorefile(conf string) error {
	return ioutil.WriteFile(conffile, []byte(conf), 0640)
}

// coreStart starts a CoreDNS serving instance, where the coredns binary is found in path and conf
// is the Corefile used to start it.
func coreStart(path string) (*exec.Cmd, io.ReadCloser, error) {
	cmd := exec.Command(path, "-conf", conffile, "-dns.port", "0")
	out, _ := cmd.StderrPipe()

	return cmd, out, cmd.Start()
}
