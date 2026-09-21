package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"card-service/backend/internal/model"
	"card-service/backend/internal/service"
)

type fakeCardService struct {
	response model.CardToIBANResponse
	err      error
	calls    int
}

func (f *fakeCardService) CardToIBAN(_ context.Context, card, requestID, clientIP string) (model.CardToIBANResponse, error) {
	f.calls++
	if f.err != nil {
		return model.CardToIBANResponse{}, f.err
	}
	return model.CardToIBANResponse{Data: model.CardToIBANData{IBAN: "IR820540102680028017900902", BankName: "Example Bank"}}, nil
}

func (f *fakeCardService) RecordInvalidRequest(context.Context, string, string, int64) error {
	return nil
}

func TestCardToIBANHandler(t *testing.T) {
	fake := &fakeCardService{}
	handler := NewHandler(fake, nil, "http://localhost:3000")
	body := strings.NewReader(`{"card":"4532015112830366"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/card-to-iban", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", "http-test-request")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Request-ID") != "http-test-request" {
		t.Fatalf("request ID header = %q", response.Header().Get("X-Request-ID"))
	}
	if fake.calls != 1 {
		t.Fatalf("service calls = %d, want 1", fake.calls)
	}
	var payload model.CardToIBANResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data.IBAN == "" {
		t.Fatal("response did not contain an IBAN")
	}
}

func TestCardToIBANHandlerRejectsMalformedJSON(t *testing.T) {
	fake := &fakeCardService{}
	handler := NewHandler(fake, nil, "http://localhost:3000")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/card-to-iban", bytes.NewBufferString(`{"card":`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	var payload model.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error.Code != service.CodeInvalidRequest {
		t.Fatalf("error code = %q, want %q", payload.Error.Code, service.CodeInvalidRequest)
	}
}
