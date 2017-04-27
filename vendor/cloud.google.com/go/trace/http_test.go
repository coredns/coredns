// Copyright 2017 Google Inc. All Rights Reserved.
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

// +build go1.7

package trace

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type noopTransport struct{}

func (rt *noopTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       ioutil.NopCloser(strings.NewReader("{}")),
	}
	return resp, nil
}

type recorderTransport struct {
	ch chan *http.Request
}

func (rt *recorderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.ch <- req
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       ioutil.NopCloser(strings.NewReader("{}")),
	}
	return resp, nil
}

func TestNewHTTPClient(t *testing.T) {
	rt := &recorderTransport{
		ch: make(chan *http.Request, 1),
	}

	tc := newTestClient(&noopTransport{})
	client := tc.NewHTTPClient(&http.Client{
		Transport: rt,
	})
	req, _ := http.NewRequest("GET", "http://example.com", nil)

	t.Run("NoTrace", func(t *testing.T) {
		_, err := client.Do(req)
		if err != nil {
			t.Error(err)
		}
		outgoing := <-rt.ch
		if got, want := outgoing.Header.Get(httpHeader), ""; want != got {
			t.Errorf("got trace header = %q; want none", got)
		}
	})

	t.Run("Trace", func(t *testing.T) {
		span := tc.NewSpan("/foo")

		req = req.WithContext(NewContext(req.Context(), span))
		_, err := client.Do(req)
		if err != nil {
			t.Error(err)
		}
		outgoing := <-rt.ch

		s := tc.SpanFromHeader("/foo", outgoing.Header.Get(httpHeader))
		if got, want := s.TraceID(), span.TraceID(); got != want {
			t.Errorf("trace ID = %q; want %q", got, want)
		}
	})
}

func TestHTTPHandlerNoTrace(t *testing.T) {
	tc := newTestClient(&noopTransport{})
	client := tc.NewHTTPClient(&http.Client{})
	handler := tc.HTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := FromContext(r.Context())
		if span == nil {
			t.Errorf("span is nil; want non-nil span")
		}
	}))

	ts := httptest.NewServer(handler)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL, nil)
	_, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
}
