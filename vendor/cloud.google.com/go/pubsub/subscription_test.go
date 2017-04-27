// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pubsub

import (
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"

	"google.golang.org/api/iterator"
)

type subListService struct {
	service
	subs []string
	err  error

	t *testing.T // for error logging.
}

func (s *subListService) newNextStringFunc() nextStringFunc {
	return func() (string, error) {
		if len(s.subs) == 0 {
			return "", iterator.Done
		}
		sn := s.subs[0]
		s.subs = s.subs[1:]
		return sn, s.err
	}
}

func (s *subListService) listProjectSubscriptions(ctx context.Context, projName string) nextStringFunc {
	if projName != "projects/projid" {
		s.t.Fatalf("unexpected call: projName: %q", projName)
		return nil
	}
	return s.newNextStringFunc()
}

func (s *subListService) listTopicSubscriptions(ctx context.Context, topicName string) nextStringFunc {
	if topicName != "projects/projid/topics/topic" {
		s.t.Fatalf("unexpected call: topicName: %q", topicName)
		return nil
	}
	return s.newNextStringFunc()
}

// All returns the remaining subscriptions from this iterator.
func slurpSubs(it *SubscriptionIterator) ([]*Subscription, error) {
	var subs []*Subscription
	for {
		switch sub, err := it.Next(); err {
		case nil:
			subs = append(subs, sub)
		case iterator.Done:
			return subs, nil
		default:
			return nil, err
		}
	}
}

func TestSubscriptionID(t *testing.T) {
	const id = "id"
	serv := &subListService{
		subs: []string{"projects/projid/subscriptions/s1", "projects/projid/subscriptions/s2"},
		t:    t,
	}
	c := &Client{projectID: "projid", s: serv}
	s := c.Subscription(id)
	if got, want := s.ID(), id; got != want {
		t.Errorf("Subscription.ID() = %q; want %q", got, want)
	}
	want := []string{"s1", "s2"}
	subs, err := slurpSubs(c.Subscriptions(context.Background()))
	if err != nil {
		t.Errorf("error listing subscriptions: %v", err)
	}
	for i, s := range subs {
		if got, want := s.ID(), want[i]; got != want {
			t.Errorf("Subscription.ID() = %q; want %q", got, want)
		}
	}
}

func TestListProjectSubscriptions(t *testing.T) {
	snames := []string{"projects/projid/subscriptions/s1", "projects/projid/subscriptions/s2",
		"projects/projid/subscriptions/s3"}
	s := &subListService{subs: snames, t: t}
	c := &Client{projectID: "projid", s: s}
	subs, err := slurpSubs(c.Subscriptions(context.Background()))
	if err != nil {
		t.Errorf("error listing subscriptions: %v", err)
	}
	got := subNames(subs)
	want := []string{
		"projects/projid/subscriptions/s1",
		"projects/projid/subscriptions/s2",
		"projects/projid/subscriptions/s3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sub list: got: %v, want: %v", got, want)
	}
	if len(s.subs) != 0 {
		t.Errorf("outstanding subs: %v", s.subs)
	}
}

func TestListTopicSubscriptions(t *testing.T) {
	snames := []string{"projects/projid/subscriptions/s1", "projects/projid/subscriptions/s2",
		"projects/projid/subscriptions/s3"}
	s := &subListService{subs: snames, t: t}
	c := &Client{projectID: "projid", s: s}
	subs, err := slurpSubs(c.Topic("topic").Subscriptions(context.Background()))
	if err != nil {
		t.Errorf("error listing subscriptions: %v", err)
	}
	got := subNames(subs)
	want := []string{
		"projects/projid/subscriptions/s1",
		"projects/projid/subscriptions/s2",
		"projects/projid/subscriptions/s3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sub list: got: %v, want: %v", got, want)
	}
	if len(s.subs) != 0 {
		t.Errorf("outstanding subs: %v", s.subs)
	}
}

func subNames(subs []*Subscription) []string {
	var names []string
	for _, sub := range subs {
		names = append(names, sub.name)
	}
	return names
}

func TestFlowControllerCancel(t *testing.T) {
	// Test canceling a flow controller's context.
	t.Parallel()
	fc := newFlowController(3, 10)
	if err := fc.acquire(context.Background(), 5); err != nil {
		t.Fatal(err)
	}
	// Experiment: a context that times out should always return an error.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := fc.acquire(ctx, 6); err != context.DeadlineExceeded {
		t.Fatalf("got %v, expected DeadlineExceeded", err)
	}
	// Control: a context that is not done should always return nil.
	go func() {
		time.Sleep(5 * time.Millisecond)
		fc.release(5)
	}()
	if err := fc.acquire(context.Background(), 6); err != nil {
		t.Errorf("got %v, expected nil", err)
	}
}

func TestFlowControllerLargeRequest(t *testing.T) {
	// Large requests succeed, consuming the entire allotment.
	t.Parallel()
	fc := newFlowController(3, 10)
	err := fc.acquire(context.Background(), 11)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlowControllerNoStarve(t *testing.T) {
	// A large request won't starve, because the flowController is
	// (best-effort) FIFO.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	fc := newFlowController(10, 10)
	first := make(chan int)
	for i := 0; i < 20; i++ {
		go func() {
			for {
				if err := fc.acquire(ctx, 1); err != nil {
					if err != context.Canceled {
						t.Error(err)
					}
					return
				}
				select {
				case first <- 1:
				default:
				}
				fc.release(1)
			}
		}()
	}
	<-first // Wait until the flowController's state is non-zero.
	if err := fc.acquire(ctx, 11); err != nil {
		t.Errorf("got %v, want nil", err)
	}
}

func TestFlowControllerSaturation(t *testing.T) {
	t.Parallel()
	const (
		maxCount = 6
		maxSize  = 10
	)
	for _, test := range []struct {
		acquireSize         int
		wantCount, wantSize int64
	}{
		{
			// Many small acquires cause the flow controller to reach its max count.
			acquireSize: 1,
			wantCount:   6,
			wantSize:    6,
		},
		{
			// Five acquires of size 2 will cause the flow controller to reach its max size,
			// but not its max count.
			acquireSize: 2,
			wantCount:   5,
			wantSize:    10,
		},
		{
			// If the requests are the right size (relatively prime to maxSize),
			// the flow controller will not saturate on size. (In this case, not on count either.)
			acquireSize: 3,
			wantCount:   3,
			wantSize:    9,
		},
	} {
		fc := newFlowController(maxCount, maxSize)
		// Atomically track flow controller state.
		var curCount, curSize int64
		success := errors.New("")
		// Time out if wantSize or wantCount is never reached.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		g, ctx := errgroup.WithContext(ctx)
		for i := 0; i < 10; i++ {
			g.Go(func() error {
				var hitCount, hitSize bool
				// Run at least until we hit the expected values, and at least
				// for enough iterations to exceed them if the flow controller
				// is broken.
				for i := 0; i < 100 || !hitCount || !hitSize; i++ {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
					}
					if err := fc.acquire(ctx, test.acquireSize); err != nil {
						return err
					}
					c := atomic.AddInt64(&curCount, 1)
					if c > test.wantCount {
						return fmt.Errorf("count %d exceeds want %d", c, test.wantCount)
					}
					if c == test.wantCount {
						hitCount = true
					}
					s := atomic.AddInt64(&curSize, int64(test.acquireSize))
					if s > test.wantSize {
						return fmt.Errorf("size %d exceeds want %d", s, test.wantSize)
					}
					if s == test.wantSize {
						hitSize = true
					}
					time.Sleep(5 * time.Millisecond) // Let other goroutines make progress.
					if atomic.AddInt64(&curCount, -1) < 0 {
						return errors.New("negative count")
					}
					if atomic.AddInt64(&curSize, -int64(test.acquireSize)) < 0 {
						return errors.New("negative size")
					}
					fc.release(test.acquireSize)
				}
				return success
			})
		}
		if err := g.Wait(); err != success {
			t.Errorf("%+v: %v", test, err)
			continue
		}
	}
}
