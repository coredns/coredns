package roundrobin

import (
	"fmt"
	"testing"

	"github.com/coredns/coredns/plugin/roundrobin/internal/strategy"
	"github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
)

func TestWriteMessage(t *testing.T) {
	var tests = []struct {
		name          string
		expectedError bool
		setQuestion   bool
		msg           *dns.Msg
		shuffler      strategy.Shuffler
	}{
		{"nil dns.Msg", true, true, nil, strategy.NewStateful()},
		{"nil dns.Msg.Question", true, false, &dns.Msg{}, strategy.NewStateful()},
		{"empty answers", false, true, &dns.Msg{Answer: []dns.RR{}}, strategy.NewStateful()},
		{"skip shuffling", false, true, &dns.Msg{Answer: []dns.RR{
			test.A("alpha.cloud.example.com.		300	IN	A			10.240.0.1"),
		}}, &shufflerStub{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// arrange

			w, err := NewMessageWriter(&wStub{}, test.msg, test.shuffler)
			if err != nil {
				t.Errorf("unexpepcted error %s", err)
			}

			// act
			if test.setQuestion && test.msg != nil {
				test.msg.SetQuestion("alpha.cloud.example.com.", dns.TypeA)
			}
			err = w.WriteMsg(test.msg)

			// assert
			if (err == nil) == test.expectedError {
				t.Errorf("unexpepcted error %s", err)
			}

			if !test.expectedError {
				if len(test.msg.Answer) != w.ResponseWriter.(*wStub).AnswersCount {
					t.Errorf("unexpected answer count")
				}
			}
		})
	}
}

type wStub struct {
	dns.ResponseWriter
	AnswersCount int
}

func (s *wStub) WriteMsg(msg *dns.Msg) error {
	s.AnswersCount = len(msg.Answer)
	return nil
}

type shufflerStub struct {
	strategy.Shuffler
}

func (s *shufflerStub) Shuffle(req request.Request, msg *dns.Msg) ([]dns.RR, error) {
	return []dns.RR{}, fmt.Errorf("skip shuffling")
}
