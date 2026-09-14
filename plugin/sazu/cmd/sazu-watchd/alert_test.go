package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotifierSendWebhookPostsExpectedPayload(t *testing.T) {
	var got webhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %q", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decoding webhook body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &Notifier{HTTPClient: srv.Client()}
	alert := Alert{Zone: "example.org.", Addresses: []string{srv.URL}, Err: fmt.Errorf("no DS published")}
	if errs := n.Send(alert); len(errs) != 0 {
		t.Fatalf("expected no errors, got %+v", errs)
	}
	if got.Zone != "example.org." || got.Recovered || got.Error != "no DS published" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestNotifierSendWebhookReportsNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := &Notifier{HTTPClient: srv.Client()}
	alert := Alert{Zone: "example.org.", Addresses: []string{srv.URL}}
	errs := n.Send(alert)
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error for a 500 response, got %+v", errs)
	}
}

func TestNotifierSendRecoveryPayloadCarriesNoError(t *testing.T) {
	var got webhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &Notifier{HTTPClient: srv.Client()}
	alert := Alert{Zone: "example.org.", Addresses: []string{srv.URL}, Recovered: true}
	if errs := n.Send(alert); len(errs) != 0 {
		t.Fatalf("expected no errors, got %+v", errs)
	}
	if !got.Recovered || got.Error != "" {
		t.Fatalf("expected a recovery payload with no error field, got %+v", got)
	}
}

func TestNotifierSendEmailWithoutSMTPAddrConfiguredFails(t *testing.T) {
	n := &Notifier{} // no SMTPAddr
	alert := Alert{Zone: "example.org.", Addresses: []string{"mailto:ops@example.org"}, Err: fmt.Errorf("broken")}
	errs := n.Send(alert)
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error when SMTP isn't configured, got %+v", errs)
	}
}

func TestNotifierSendEmailRejectsInvalidAddress(t *testing.T) {
	n := &Notifier{SMTPAddr: "smtp.example.org:587", SMTPFrom: "alerts@example.org"}
	alert := Alert{Zone: "example.org.", Addresses: []string{"mailto:not-an-email"}, Err: fmt.Errorf("broken")}
	errs := n.Send(alert)
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error for an invalid email address, got %+v", errs)
	}
}

func TestNotifierSendUnknownSchemeReportsError(t *testing.T) {
	n := &Notifier{}
	alert := Alert{Zone: "example.org.", Addresses: []string{"ftp://example.org"}}
	errs := n.Send(alert)
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error for an unrecognized scheme, got %+v", errs)
	}
}

func TestNotifierSendDispatchesToMultipleAddressesIndependently(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := &Notifier{HTTPClient: srv.Client()} // email will fail (no SMTP configured), webhook should still succeed
	alert := Alert{Zone: "example.org.", Addresses: []string{"mailto:ops@example.org", srv.URL}, Err: fmt.Errorf("broken")}
	errs := n.Send(alert)
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error (the email address, since no SMTP is configured), got %+v", errs)
	}
}
