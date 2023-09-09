package reload_test

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/coredns/caddy"
	_ "github.com/coredns/coredns/core/plugin"
)

var (
	startInstance   func() error
	stopInstance    func()
	waitForInstance func(timeout time.Duration) error
	updateCorefile  func(corefile string)
)

func TestMain(m *testing.M) {
	var instance *caddy.Instance
	var wait = make(chan *caddy.Instance, 1)
	var hook caddy.EventHook
	var mtx sync.Mutex

	hook = func(eventType caddy.EventName, eventInfo interface{}) error {
		mtx.Lock()
		defer mtx.Unlock()
		switch eventType {
		case caddy.InstanceStartupEvent:
			instance = eventInfo.(*caddy.Instance)
			// (re)register hook when a restart happens (needed because the USR1 signal handler clears hooks before restarting)
			instance.OnRestart = append(instance.OnRestart, func() error {
				caddy.RegisterOrUpdateEventHook("reload_test", hook)
				return nil
			})
			select {
			case <-wait:
			default:
			}
			wait <- instance
		}
		return nil
	}

	caddy.RegisterEventHook("reload_test", hook)

	startInstance = func() error {
		if instance != nil {
			panic("instance already running")
		}
		coreInput, err := caddy.LoadCaddyfile("dns")
		if err != nil {
			return err
		}
		_, err = caddy.Start(coreInput)
		if err != nil {
			return err
		}
		return nil
	}

	stopInstance = func() {
		mtx.Lock()
		defer mtx.Unlock()
		if instance == nil {
			return
		}
		err := instance.Stop()
		if err != nil {
			panic(err)
		}
		errs := instance.ShutdownCallbacks()
		if len(errs) > 0 {
			panic(errs)
		}
		instance = nil
	}

	waitForInstance = func(timeout time.Duration) error {
		select {
		case <-wait:
			return nil
		case <-time.After(timeout):
			return fmt.Errorf("timeout")
		}
	}

	tmpdir, err := os.MkdirTemp("", "")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpdir)

	updateCorefile = func(corefile string) {
		err := os.WriteFile(tmpdir+"/Corefile", []byte(corefile), 0644)
		if err != nil {
			panic(err)
		}
	}

	caddy.RegisterCaddyfileLoader("test", caddy.LoaderFunc(func(serverType string) (caddy.Input, error) {
		corefile, err := os.ReadFile(tmpdir + "/Corefile")
		return caddy.CaddyfileInput{Filepath: tmpdir + "/Corefile", Contents: corefile, ServerTypeName: "dns"}, err
	}))

	os.Exit(m.Run())
}
