package log

import (
	"bytes"
	golog "log"
	"strings"
	"testing"
)

func TestRegisterAndDeregisterListener(t *testing.T) {
	for _, name := range []string{"listener1", "listener2", "listener1"} {
		err := RegisterListener(NewMockListener(name))
		if err != nil {
			t.Errorf("RegisterListener Error %s", err)
		}
	}
	if len(ls.listeners) != 2 {
		t.Errorf("Expected number of listeners to be %d, got %d", 2, len(ls.listeners))
	}
	for _, name := range []string{"listener1", "listener2"} {
		err := DeregisterListener(NewMockListener(name))
		if err != nil {
			t.Errorf("DeregsiterListener Error %s", err)
		}
	}
	if len(ls.listeners) != 0 {
		t.Errorf("Expected number of listeners to be %d, got %d", 0, len(ls.listeners))
	}
}

func TestSingleListenerMock(t *testing.T) {
	listener1Name := "listener1"
	listener1Output := info + listener1Name + " mocked info"
	testListenersCalled(t, []string{listener1Name}, []string{listener1Output})
}

func TestMultipleListenerMock(t *testing.T) {
	listener1Name := "listener1"
	listener1Output := info + listener1Name + " mocked info"
	listener2Name := "listener2"
	listener2Output := info + listener2Name + " mocked info"
	testListenersCalled(t, []string{listener1Name, listener2Name}, []string{listener1Output, listener2Output})
}

func testListenersCalled(t *testing.T, listenerNames []string, outputs []string) {
	for _, name := range listenerNames {
		err := RegisterListener(NewMockListener(name))
		if err != nil {
			t.Errorf("RegisterListener Error %s", err)
		}
	}
	var f bytes.Buffer
	const ts = "test"
	golog.SetOutput(&f)
	lg := NewWithPlugin("testplugin")
	lg.Info(ts)
	for _, str := range outputs {
		if x := f.String(); !strings.Contains(x, str) {
			t.Errorf("Expected log to contain %s, got %s", str, x)
		}
	}
	for _, name := range listenerNames {
		err := DeregisterListener(NewMockListener(name))
		if err != nil {
			t.Errorf("DeregsiterListener Error %s", err)
		}
	}
}

type mockListener struct {
	name string
}

func NewMockListener(name string) *mockListener {
	return &mockListener{name: name}
}

func (l *mockListener) Name() string {
	return l.name
}

func (l *mockListener) Debug(plugin string, v ...interface{}) {
	log(debug, l.name+" mocked debug")
}

func (l *mockListener) Debugf(plugin string, format string, v ...interface{}) {
	log(debug, l.name+" mocked debug")
}

func (l *mockListener) Info(plugin string, v ...interface{}) {
	log(info, l.name+" mocked info")
}

func (l *mockListener) Infof(plugin string, format string, v ...interface{}) {
	log(info, l.name+" mocked info")
}

func (l *mockListener) Warning(plugin string, v ...interface{}) {
	log(warning, l.name+" mocked warning")
}

func (l *mockListener) Warningf(plugin string, format string, v ...interface{}) {
	log(warning, l.name+" mocked warning")
}

func (l *mockListener) Error(plugin string, v ...interface{}) {
	log(err, l.name+" mocked error")
}

func (l *mockListener) Errorf(plugin string, format string, v ...interface{}) {
	log(err, l.name+" mocked error")
}

func (l *mockListener) Fatal(plugin string, v ...interface{}) {
	log(fatal, l.name+" mocked fatal")
}

func (l *mockListener) Fatalf(plugin string, format string, v ...interface{}) {
	log(fatal, l.name+" mocked fatal")
}
