// +build go1.9

package sarama

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFuncConsumerGroupPartitioning(t *testing.T) {
	checkKafkaVersion(t, "0.10.2")
	setupFunctionalTest(t)
	defer teardownFunctionalTest(t)

	groupID := testFuncConsumerGroupID(t)

	// start M1
	m1 := runTestFuncConsumerGroupMember(t, groupID, "M1", 0, nil)
	defer m1.Stop()
	m1.WaitForState(2)
	m1.WaitForClaims(map[string]int{"test.4": 4})
	m1.WaitForHandlers(4)

	// start M2
	m2 := runTestFuncConsumerGroupMember(t, groupID, "M2", 0, nil, "test.1", "test.4")
	defer m2.Stop()
	m2.WaitForState(2)

	// assert that claims are shared among both members
	m1.WaitForClaims(map[string]int{"test.4": 2})
	m1.WaitForHandlers(2)
	m2.WaitForClaims(map[string]int{"test.1": 1, "test.4": 2})
	m2.WaitForHandlers(3)

	// shutdown M1, wait for M2 to take over
	m1.AssertCleanShutdown()
	m2.WaitForClaims(map[string]int{"test.1": 1, "test.4": 4})
	m2.WaitForHandlers(5)

	// shutdown M2
	m2.AssertCleanShutdown()
}

func TestFuncConsumerGroupExcessConsumers(t *testing.T) {
	checkKafkaVersion(t, "0.10.2")
	setupFunctionalTest(t)
	defer teardownFunctionalTest(t)

	groupID := testFuncConsumerGroupID(t)

	// start members
	m1 := runTestFuncConsumerGroupMember(t, groupID, "M1", 0, nil)
	defer m1.Stop()
	m2 := runTestFuncConsumerGroupMember(t, groupID, "M2", 0, nil)
	defer m2.Stop()
	m3 := runTestFuncConsumerGroupMember(t, groupID, "M3", 0, nil)
	defer m3.Stop()
	m4 := runTestFuncConsumerGroupMember(t, groupID, "M4", 0, nil)
	defer m4.Stop()

	m1.WaitForClaims(map[string]int{"test.4": 1})
	m2.WaitForClaims(map[string]int{"test.4": 1})
	m3.WaitForClaims(map[string]int{"test.4": 1})
	m4.WaitForClaims(map[string]int{"test.4": 1})

	// start M5
	m5 := runTestFuncConsumerGroupMember(t, groupID, "M5", 0, nil)
	defer m5.Stop()
	m5.WaitForState(1)
	m5.AssertNoErrs()

	// assert that claims are shared among both members
	m4.AssertCleanShutdown()
	m5.WaitForState(2)
	m5.WaitForClaims(map[string]int{"test.4": 1})

	// shutdown everything
	m1.AssertCleanShutdown()
	m2.AssertCleanShutdown()
	m3.AssertCleanShutdown()
	m5.AssertCleanShutdown()
}

func TestFuncConsumerGroupFuzzy(t *testing.T) {
	checkKafkaVersion(t, "0.10.2")
	setupFunctionalTest(t)
	defer teardownFunctionalTest(t)

	if err := testFuncConsumerGroupFuzzySeed("test.4"); err != nil {
		t.Fatal(err)
	}

	groupID := testFuncConsumerGroupID(t)
	sink := &testFuncConsumerGroupSink{msgs: make(chan testFuncConsumerGroupMessage, 20000)}
	waitForMessages := func(t *testing.T, n int) {
		t.Helper()

		for i := 0; i < 600; i++ {
			if sink.Len() >= n {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if sz := sink.Len(); sz < n {
			log.Fatalf("expected to consume %d messages, but consumed %d", n, sz)
		}
	}

	defer runTestFuncConsumerGroupMember(t, groupID, "M1", 1500, sink).Stop()
	defer runTestFuncConsumerGroupMember(t, groupID, "M2", 3000, sink).Stop()
	defer runTestFuncConsumerGroupMember(t, groupID, "M3", 1500, sink).Stop()
	defer runTestFuncConsumerGroupMember(t, groupID, "M4", 200, sink).Stop()
	defer runTestFuncConsumerGroupMember(t, groupID, "M5", 100, sink).Stop()
	waitForMessages(t, 3000)

	defer runTestFuncConsumerGroupMember(t, groupID, "M6", 300, sink).Stop()
	defer runTestFuncConsumerGroupMember(t, groupID, "M7", 400, sink).Stop()
	defer runTestFuncConsumerGroupMember(t, groupID, "M8", 500, sink).Stop()
	defer runTestFuncConsumerGroupMember(t, groupID, "M9", 2000, sink).Stop()
	waitForMessages(t, 8000)

	defer runTestFuncConsumerGroupMember(t, groupID, "M10", 1000, sink).Stop()
	waitForMessages(t, 10000)

	defer runTestFuncConsumerGroupMember(t, groupID, "M11", 1000, sink).Stop()
	defer runTestFuncConsumerGroupMember(t, groupID, "M12", 2500, sink).Stop()
	waitForMessages(t, 12000)

	defer runTestFuncConsumerGroupMember(t, groupID, "M13", 1000, sink).Stop()
	waitForMessages(t, 15000)

	if umap := sink.Close(); len(umap) != 15000 {
		dupes := make(map[string][]string)
		for k, v := range umap {
			if len(v) > 1 {
				dupes[k] = v
			}
		}
		t.Fatalf("expected %d unique messages to be consumed but got %d, including %d duplicates:\n%v", 15000, len(umap), len(dupes), dupes)
	}
}

// --------------------------------------------------------------------

func testFuncConsumerGroupID(t *testing.T) string {
	return fmt.Sprintf("sarama.%s%d", t.Name(), time.Now().UnixNano())
}

func testFuncConsumerGroupFuzzySeed(topic string) error {
	client, err := NewClient(kafkaBrokers, nil)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	total := int64(0)
	for pn := int32(0); pn < 4; pn++ {
		newest, err := client.GetOffset(topic, pn, OffsetNewest)
		if err != nil {
			return err
		}
		oldest, err := client.GetOffset(topic, pn, OffsetOldest)
		if err != nil {
			return err
		}
		total = total + newest - oldest
	}
	if total >= 21000 {
		return nil
	}

	producer, err := NewAsyncProducerFromClient(client)
	if err != nil {
		return err
	}
	for i := total; i < 21000; i++ {
		producer.Input() <- &ProducerMessage{Topic: topic, Value: ByteEncoder([]byte("testdata"))}
	}
	return producer.Close()
}

type testFuncConsumerGroupMessage struct {
	ClientID string
	*ConsumerMessage
}

type testFuncConsumerGroupSink struct {
	msgs  chan testFuncConsumerGroupMessage
	count int32
}

func (s *testFuncConsumerGroupSink) Len() int {
	if s == nil {
		return -1
	}
	return int(atomic.LoadInt32(&s.count))
}

func (s *testFuncConsumerGroupSink) Push(clientID string, m *ConsumerMessage) {
	if s != nil {
		s.msgs <- testFuncConsumerGroupMessage{ClientID: clientID, ConsumerMessage: m}
		atomic.AddInt32(&s.count, 1)
	}
}

func (s *testFuncConsumerGroupSink) Close() map[string][]string {
	close(s.msgs)

	res := make(map[string][]string)
	for msg := range s.msgs {
		key := fmt.Sprintf("%s-%d:%d", msg.Topic, msg.Partition, msg.Offset)
		res[key] = append(res[key], msg.ClientID)
	}
	return res
}

type testFuncConsumerGroupMember struct {
	ConsumerGroup
	clientID    string
	claims      map[string]int
	state       int32
	handlers    int32
	errs        []error
	maxMessages int32
	isCapped    bool
	sink        *testFuncConsumerGroupSink

	t  *testing.T
	mu sync.RWMutex
}

func runTestFuncConsumerGroupMember(t *testing.T, groupID, clientID string, maxMessages int32, sink *testFuncConsumerGroupSink, topics ...string) *testFuncConsumerGroupMember {
	t.Helper()

	config := NewConfig()
	config.ClientID = clientID
	config.Version = V0_10_2_0
	config.Consumer.Return.Errors = true
	config.Consumer.Offsets.Initial = OffsetOldest
	config.Consumer.Group.Rebalance.Timeout = 10 * time.Second

	group, err := NewConsumerGroup(kafkaBrokers, groupID, config)
	if err != nil {
		t.Fatal(err)
		return nil
	}

	if len(topics) == 0 {
		topics = []string{"test.4"}
	}

	member := &testFuncConsumerGroupMember{
		ConsumerGroup: group,
		clientID:      clientID,
		claims:        make(map[string]int),
		maxMessages:   maxMessages,
		isCapped:      maxMessages != 0,
		sink:          sink,
		t:             t,
	}
	go member.loop(topics)
	return member
}

func (m *testFuncConsumerGroupMember) AssertCleanShutdown() {
	m.t.Helper()

	if err := m.Close(); err != nil {
		m.t.Fatalf("unexpected error on Close(): %v", err)
	}
	m.WaitForState(4)
	m.WaitForHandlers(0)
	m.AssertNoErrs()
}

func (m *testFuncConsumerGroupMember) AssertNoErrs() {
	m.t.Helper()

	var errs []error
	m.mu.RLock()
	errs = append(errs, m.errs...)
	m.mu.RUnlock()

	if len(errs) != 0 {
		m.t.Fatalf("unexpected consumer errors: %v", errs)
	}
}

func (m *testFuncConsumerGroupMember) WaitForState(expected int32) {
	m.t.Helper()

	m.waitFor("state", expected, func() (interface{}, error) {
		return atomic.LoadInt32(&m.state), nil
	})
}

func (m *testFuncConsumerGroupMember) WaitForHandlers(expected int) {
	m.t.Helper()

	m.waitFor("handlers", expected, func() (interface{}, error) {
		return int(atomic.LoadInt32(&m.handlers)), nil
	})
}

func (m *testFuncConsumerGroupMember) WaitForClaims(expected map[string]int) {
	m.t.Helper()

	m.waitFor("claims", expected, func() (interface{}, error) {
		m.mu.RLock()
		claims := m.claims
		m.mu.RUnlock()
		return claims, nil
	})
}

func (m *testFuncConsumerGroupMember) Stop() { _ = m.Close() }

func (m *testFuncConsumerGroupMember) Setup(s ConsumerGroupSession) error {
	// store claims
	claims := make(map[string]int)
	for topic, partitions := range s.Claims() {
		claims[topic] = len(partitions)
	}
	m.mu.Lock()
	m.claims = claims
	m.mu.Unlock()

	// enter post-setup state
	atomic.StoreInt32(&m.state, 2)
	return nil
}
func (m *testFuncConsumerGroupMember) Cleanup(s ConsumerGroupSession) error {
	// enter post-cleanup state
	atomic.StoreInt32(&m.state, 3)
	return nil
}
func (m *testFuncConsumerGroupMember) ConsumeClaim(s ConsumerGroupSession, c ConsumerGroupClaim) error {
	atomic.AddInt32(&m.handlers, 1)
	defer atomic.AddInt32(&m.handlers, -1)

	for msg := range c.Messages() {
		if n := atomic.AddInt32(&m.maxMessages, -1); m.isCapped && n < 0 {
			break
		}
		s.MarkMessage(msg, "")
		m.sink.Push(m.clientID, msg)
	}
	return nil
}

func (m *testFuncConsumerGroupMember) waitFor(kind string, expected interface{}, factory func() (interface{}, error)) {
	m.t.Helper()

	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var actual interface{}
	for {
		var err error
		if actual, err = factory(); err != nil {
			m.t.Errorf("failed retrieve value, expected %s %#v but received error %v", kind, expected, err)
		}

		if reflect.DeepEqual(expected, actual) {
			return
		}

		select {
		case <-deadline.C:
			m.t.Fatalf("ttl exceeded, expected %s %#v but got %#v", kind, expected, actual)
			return
		case <-ticker.C:
		}
	}
}

func (m *testFuncConsumerGroupMember) loop(topics []string) {
	defer atomic.StoreInt32(&m.state, 4)

	go func() {
		for err := range m.Errors() {
			_ = m.Close()

			m.mu.Lock()
			m.errs = append(m.errs, err)
			m.mu.Unlock()
		}
	}()

	ctx := context.Background()
	for {
		// set state to pre-consume
		atomic.StoreInt32(&m.state, 1)

		if err := m.Consume(ctx, topics, m); err == ErrClosedConsumerGroup {
			return
		} else if err != nil {
			m.mu.Lock()
			m.errs = append(m.errs, err)
			m.mu.Unlock()
			return
		}

		// return if capped
		if n := atomic.LoadInt32(&m.maxMessages); m.isCapped && n < 0 {
			return
		}
	}
}
