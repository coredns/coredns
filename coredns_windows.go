//go:build windows

package main

//go:generate go run directives_generate.go
//go:generate go run owners_generate.go

import (
	"flag"
	"log"

	_ "github.com/coredns/coredns/core/plugin" // Plug in CoreDNS.
	"github.com/coredns/coredns/coremain"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

type coreDnsService struct{}

func (m *coreDnsService) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {

	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	status <- svc.Status{State: svc.StartPending}

	instance := coremain.Run()

	status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				log.Print("Shutting down DNS service...")
				instance.ShutdownCallbacks()
				instance.Stop()
				break loop
			default:
				log.Printf("Unexpected service control request #%d", c)
			}
		}
	}

	status <- svc.Status{State: svc.Stopped}
	return false, 0
}

func runService(name string, isDebug bool) {
	if isDebug {
		err := debug.Run(name, &coreDnsService{})
		if err != nil {
			log.Fatalf("Error running service in debug mode: %s\n", err.Error())
		}
	} else {
		err := svc.Run(name, &coreDnsService{})
		if err != nil {
			log.Fatalf("Error running service in Service Control mode: %s\n", err.Error())
		}
	}
}

var svcMode = flag.Bool("service", false, "Run as a Windows service")

func main() {
	flag.Parse()
	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("Could not determine service status: %s", err.Error())
		return
	}

	if isService || *svcMode {
		log.Printf("Running CoreDNS in service mode")
		runService("CoreDNS", !isService)
	} else {
		coremain.RunForever()
	}
}
