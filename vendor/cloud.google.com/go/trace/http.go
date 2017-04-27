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

import "net/http"

type tracerTransport struct {
	base http.RoundTripper
}

func (tt *tracerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	span := FromContext(req.Context()).NewRemoteChild(req)
	resp, err := tt.base.RoundTrip(req)

	// TODO(jbd): Is it possible to defer the span.Finish?
	// In cases where RoundTrip panics, we still can finish the span.
	span.Finish(WithResponse(resp))
	return resp, err
}

// HTTPClient is an HTTP client that enhances http.Client
// with automatic tracing support.
type HTTPClient struct {
	http.Client
	traceClient *Client
}

// Do behaves like (*http.Client).Do but automatically traces
// outgoing requests if tracing is enabled for the current request.
//
// If req.Context() contains a traced *Span, the outgoing request
// is traced with the existing span. If not, the request is not traced.
func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.Client.Do(req)
}

// NewHTTPClient creates a new HTTPClient that will trace the outgoing
// requests using tc. The attributes of this client are inherited from the
// given http.Client. If orig is nil, http.DefaultClient is used.
func (c *Client) NewHTTPClient(orig *http.Client) *HTTPClient {
	if orig == nil {
		orig = http.DefaultClient
	}
	rt := orig.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	client := http.Client{
		Transport:     &tracerTransport{base: rt},
		CheckRedirect: orig.CheckRedirect,
		Jar:           orig.Jar,
		Timeout:       orig.Timeout,
	}
	return &HTTPClient{
		Client:      client,
		traceClient: c,
	}
}

// HTTPHandler returns a http.Handler from the given handler
// that is aware of the incoming request's span.
// The span can be extracted from the incoming request in handler
// functions from incoming request's context:
//
//    span := trace.FromContext(r.Context())
//
// The span will be auto finished by the handler.
func (c *Client) HTTPHandler(h http.Handler) http.Handler {
	return &handler{traceClient: c, handler: h}
}

type handler struct {
	traceClient *Client
	handler     http.Handler
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	span := h.traceClient.SpanFromRequest(r)
	defer span.Finish()

	r = r.WithContext(NewContext(r.Context(), span))
	h.handler.ServeHTTP(w, r)
}
