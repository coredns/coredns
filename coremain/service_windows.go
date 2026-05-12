//go:build windows

package coremain

import (
	"flag"
	"log"

	"github.com/coredns/caddy"

	"golang.org/x/sys/windows/svc"
)

var windowsService bool

func init() {
	flag.BoolVar(&windowsService, "windows-service", false, "Run as a Windows service")
}

type corednsService struct {
	instance *caddy.Instance
}

func (s *corednsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for req := range r {
		switch req.Cmd {
		case svc.Interrogate:
			changes <- req.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			if s.instance != nil {
				s.instance.Stop()
			}
			return false, 0
		default:
			log.Printf("unexpected control request #%d", req.Cmd)
		}
	}

	return false, 0
}

func runService(instance *caddy.Instance) {
	if windowsService {
		isService, err := svc.IsWindowsService()
		if err != nil {
			log.Fatalf("failed to determine if running as service: %v", err)
		}
		if isService {
			err = svc.Run("CoreDNS", &corednsService{instance: instance})
			if err != nil {
				log.Fatalf("failed to start service: %v", err)
			}
			return
		} else {
			log.Printf("Windows service flag provided, but not running as a Windows service.")
		}
	}
	instance.Wait()
}
