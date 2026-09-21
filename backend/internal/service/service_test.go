package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"card-service/backend/internal/client"
	"card-service/backend/internal/model"
	"card-service/backend/internal/validation"
)

type fakeZarinHubClient struct {
	result client.Result
	err    error
	calls  int
	card   string
}

func (f *fakeZarinHubClient) CardToIBAN(_ context.Context, card string) (client.Result, error) {
	f.calls++
	f.card = card
	return f.result, f.err
}

type fakeAuditRepository struct {
	records []model.AuditRecord
	err     error
}

func (f *fakeAuditRepository) SaveAudit(_ context.Context, record model.AuditRecord) error {
	f.records = append(f.records, record)
	return f.err
}

func TestServiceCardToIBANSuccess(t *testing.T) {
	zarinHub := &fakeZarinHubClient{
		result: client.Result{
			IBAN:               "IR820540102680028017900902",
			BankName:           "Example Bank",
			ExternalHTTPStatus: 200,
			RawResponse:        []byte(`{"data":{"iban":"IR820540102680028017900902","card":"4532015112830366"},"meta":{"isSuccess":true}}`),
		},
	}
	repo := &fakeAuditRepository{}
	svc := NewService(zarinHub, repo, nil)

	response, err := svc.CardToIBAN(context.Background(), "4532015112830366", "request-1", "127.0.0.1")
	if err != nil {
		t.Fatalf("CardToIBAN() error = %v", err)
	}
	if response.Data.IBAN != zarinHub.result.IBAN {
		t.Fatalf("IBAN = %q, want %q", response.Data.IBAN, zarinHub.result.IBAN)
	}
	if zarinHub.calls != 1 {
		t.Fatalf("client calls = %d, want 1", zarinHub.calls)
	}
	if len(repo.records) != 1 {
		t.Fatalf("audit records = %d, want 1", len(repo.records))
	}
	if repo.records[0].Status != "SUCCESS" || repo.records[0].CardMasked != "4532********0366" {
		t.Fatalf("unexpected audit record: %+v", repo.records[0])
	}
	if strings.Contains(repo.records[0].ResponseJSON, "4532015112830366") {
		t.Fatal("audit response contains the full card number")
	}
}

func TestServiceInvalidCardDoesNotCallZarinHub(t *testing.T) {
	zarinHub := &fakeZarinHubClient{}
	repo := &fakeAuditRepository{}
	svc := NewService(zarinHub, repo, nil)

	_, err := svc.CardToIBAN(context.Background(), "123", "request-2", "127.0.0.1")
	if err == nil || !errors.Is(err, validation.ErrInvalidChecksum) && !strings.Contains(err.Error(), "شماره کارت نامعتبر است") {
		t.Fatalf("CardToIBAN() error = %v, want invalid-card error", err)
	}
	if zarinHub.calls != 0 {
		t.Fatalf("client calls = %d, want 0", zarinHub.calls)
	}
	if len(repo.records) != 1 || repo.records[0].Status != "VALIDATION_FAILED" {
		t.Fatalf("unexpected audit records: %+v", repo.records)
	}
}

func TestServiceZarinHubErrorIsAudited(t *testing.T) {
	raw := []byte(`{"data":{"card":"4532015112830366"},"meta":{"isSuccess":false,"message":"rejected"}}`)
	zarinHub := &fakeZarinHubClient{err: &client.Error{
		Kind:               client.ErrorRejected,
		ExternalHTTPStatus: 422,
		RawResponse:        raw,
		Err:                errors.New("rejected"),
	}}
	repo := &fakeAuditRepository{}
	svc := NewService(zarinHub, repo, nil)

	_, err := svc.CardToIBAN(context.Background(), "4532015112830366", "request-3", "127.0.0.1")
	var serviceErr *Error
	if !errors.As(err, &serviceErr) || serviceErr.Code != CodeExternalRejected {
		t.Fatalf("CardToIBAN() error = %v, want external rejection", err)
	}
	if len(repo.records) != 1 || repo.records[0].Status != "EXTERNAL_FAILED" {
		t.Fatalf("unexpected audit records: %+v", repo.records)
	}
	if strings.Contains(repo.records[0].ResponseJSON, "4532015112830366") {
		t.Fatal("audit response contains the full card number")
	}
}

func TestServiceZarinHubTimeout(t *testing.T) {
	zarinHub := &fakeZarinHubClient{err: &client.Error{Kind: client.ErrorTimeout, Err: context.DeadlineExceeded}}
	svc := NewService(zarinHub, &fakeAuditRepository{}, nil)

	_, err := svc.CardToIBAN(context.Background(), "4532015112830366", "request-4", "127.0.0.1")
	var serviceErr *Error
	if !errors.As(err, &serviceErr) || serviceErr.Code != CodeExternalTimeout {
		t.Fatalf("CardToIBAN() error = %v, want timeout", err)
	}
}

func TestServiceRepositoryError(t *testing.T) {
	repoErr := errors.New("database unavailable")
	zarinHub := &fakeZarinHubClient{result: client.Result{IBAN: "IR820540102680028017900902", ExternalHTTPStatus: 200}}
	repo := &fakeAuditRepository{err: repoErr}
	svc := NewService(zarinHub, repo, nil)

	_, err := svc.CardToIBAN(context.Background(), "4532015112830366", "request-5", "127.0.0.1")
	var serviceErr *Error
	if !errors.As(err, &serviceErr) || serviceErr.Code != CodePersistence {
		t.Fatalf("CardToIBAN() error = %v, want persistence error", err)
	}
}
