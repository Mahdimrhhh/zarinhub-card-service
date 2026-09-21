package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPClientCardToIBAN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/card-to-iban" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %q", r.Header.Get("Content-Type"))
		}
		var body struct {
			Card string `json:"card"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Card != "4532015112830366" {
			t.Fatalf("request body = %+v, error = %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"iban":"IR820540102680028017900902","bankName":"Example Bank","card":"4532015112830366"},"meta":{"isSuccess":true,"status":"success"}}`))
	}))
	defer server.Close()

	httpClient, err := NewZarinHubClient(server.URL+"/card-to-iban", "test-token", time.Second)
	if err != nil {
		t.Fatalf("NewZarinHubClient() error = %v", err)
	}
	result, err := httpClient.CardToIBAN(context.Background(), "4532015112830366")
	if err != nil {
		t.Fatalf("CardToIBAN() error = %v", err)
	}
	if result.IBAN == "" || result.BankName != "Example Bank" || result.ExternalHTTPStatus != http.StatusOK {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestHTTPClientApplicationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{},"meta":{"isSuccess":false,"status":"error","code":-1}}`))
	}))
	defer server.Close()

	httpClient, err := NewZarinHubClient(server.URL, "test-token", time.Second)
	if err != nil {
		t.Fatalf("NewZarinHubClient() error = %v", err)
	}
	_, err = httpClient.CardToIBAN(context.Background(), "4532015112830366")
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Kind != ErrorRejected {
		t.Fatalf("error = %v, want application rejection", err)
	}
}

func TestHTTPClientAuthenticationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	httpClient, err := NewZarinHubClient(server.URL, "test-token", time.Second)
	if err != nil {
		t.Fatalf("NewZarinHubClient() error = %v", err)
	}
	_, err = httpClient.CardToIBAN(context.Background(), "4532015112830366")
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Kind != ErrorAuth || clientErr.ExternalHTTPStatus != http.StatusUnauthorized {
		t.Fatalf("error = %v, want authentication error", err)
	}
}

func TestHTTPClientMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":`))
	}))
	defer server.Close()

	httpClient, err := NewZarinHubClient(server.URL, "test-token", time.Second)
	if err != nil {
		t.Fatalf("NewZarinHubClient() error = %v", err)
	}
	_, err = httpClient.CardToIBAN(context.Background(), "4532015112830366")
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Kind != ErrorInvalidResponse {
		t.Fatalf("error = %v, want invalid response", err)
	}
}

func TestHTTPClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpClient, err := NewZarinHubClient(server.URL, "test-token", 10*time.Millisecond)
	if err != nil {
		t.Fatalf("NewZarinHubClient() error = %v", err)
	}
	_, err = httpClient.CardToIBAN(context.Background(), "4532015112830366")
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Kind != ErrorTimeout {
		t.Fatalf("error = %v, want timeout", err)
	}
	if strings.Contains(err.Error(), "test-token") {
		t.Fatal("client error exposed the access token")
	}
}
