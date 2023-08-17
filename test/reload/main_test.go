package reload_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/coredns/caddy"
	_ "github.com/coredns/coredns/core/plugin"
	"github.com/google/uuid"
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
	var corefilePath string

	hook = func(eventType caddy.EventName, eventInfo interface{}) error {
		switch eventType {
		case caddy.InstanceStartupEvent:
			instance = eventInfo.(*caddy.Instance)
			// (re)register hook when a restart happens (needed because the USR1 signal handler clears hooks before restarting)
			instance.OnRestart = append(instance.OnRestart, func() error {
				registerEventHook("reload_test", hook)
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

	registerEventHook("reload_test", hook)

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
		if corefilePath != "" {
			err := os.Remove(corefilePath)
			if err != nil {
				panic(err)
			}
		}
		corefilePath = tmpdir + "/Corefile-" + uuid.New().String()
		err := os.WriteFile(corefilePath, []byte(corefile), 0644)
		if err != nil {
			panic(err)
		}
	}

	caddy.RegisterCaddyfileLoader("test", caddy.LoaderFunc(func(serverType string) (caddy.Input, error) {
		corefile, err := os.ReadFile(corefilePath)
		return caddy.CaddyfileInput{Filepath: corefilePath, Contents: corefile, ServerTypeName: "dns"}, err
	}))

	os.Exit(m.Run())
}

// TODO: it would be nicer if github.com/coredns/caddy would expose some method RegisterEventHookIfNotRegistered()
func registerEventHook(name string, hook caddy.EventHook) (changed bool) {
	defer func() {
		if r := recover(); r != nil {
			if s, ok := r.(string); !ok || s != "hook named "+name+" already registered" {
				panic(r)
			}
		}
	}()
	caddy.RegisterEventHook(name, hook)
	changed = true
	return
}
