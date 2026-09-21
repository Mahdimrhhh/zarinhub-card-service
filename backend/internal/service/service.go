package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"card-service/backend/internal/client"
	"card-service/backend/internal/model"
	"card-service/backend/internal/validation"
)

type ZarinHubClient interface {
	CardToIBAN(ctx context.Context, card string) (client.Result, error)
}

type AuditRepository interface {
	SaveAudit(ctx context.Context, record model.AuditRecord) error
}

type Service struct {
	zarinHub ZarinHubClient
	repo     AuditRepository
	logger   *slog.Logger
}

type Error struct {
	Code           string
	Message        string
	HTTPStatus     int
	Category       string
	ExternalStatus int
	Err            error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

const (
	CodeInvalidCard         = "INVALID_CARD"
	CodeInvalidRequest      = "INVALID_REQUEST"
	CodeExternalAuth        = "ZARINHUB_AUTHENTICATION"
	CodeExternalRejected    = "ZARINHUB_REJECTED"
	CodeExternalTimeout     = "ZARINHUB_TIMEOUT"
	CodeExternalUnavailable = "ZARINHUB_UNAVAILABLE"
	CodePersistence         = "PERSISTENCE_FAILED"
	CodeInternal            = "INTERNAL_SERVER_ERROR"
)

func NewService(zarinHub ZarinHubClient, repo AuditRepository, logger *slog.Logger) *Service {
	return &Service{
		zarinHub: zarinHub,
		repo:     repo,
		logger:   logger,
	}
}

func (s *Service) CardToIBAN(ctx context.Context, card, requestID, clientIP string) (model.CardToIBANResponse, error) {
	started := time.Now()
	record := model.AuditRecord{
		RequestID:  requestID,
		CreatedAt:  time.Now().UTC(),
		ClientIP:   clientIP,
		CardMasked: validation.Mask(card),
	}

	if err := validation.Validate(card); err != nil {
		record.Status = "VALIDATION_FAILED"
		record.DurationMS = durationMS(started)
		if saveErr := s.saveAudit(ctx, record); saveErr != nil {
			return model.CardToIBANResponse{}, s.persistenceError(saveErr)
		}
		s.logFailure(requestID, "validation")
		return model.CardToIBANResponse{}, &Error{
			Code:       CodeInvalidCard,
			Message:    "شماره کارت نامعتبر است",
			HTTPStatus: http.StatusBadRequest,
			Category:   "validation",
			Err:        err,
		}
	}

	if s.zarinHub == nil {
		record.Status = "EXTERNAL_FAILED"
		record.DurationMS = durationMS(started)
		if saveErr := s.saveAudit(ctx, record); saveErr != nil {
			return model.CardToIBANResponse{}, s.persistenceError(saveErr)
		}
		return model.CardToIBANResponse{}, s.internalError(errors.New("zarin-hub client is unavailable"))
	}

	result, err := s.zarinHub.CardToIBAN(ctx, card)
	if err != nil {
		record.Status = "EXTERNAL_FAILED"
		record.DurationMS = durationMS(started)
		record.ExternalHTTPStatus = externalStatusFromError(err)
		record.ResponseJSON = rawResponseFromError(err, card)
		if saveErr := s.saveAudit(ctx, record); saveErr != nil {
			return model.CardToIBANResponse{}, s.persistenceError(saveErr)
		}
		serviceErr := s.fromClientError(err)
		s.logFailure(requestID, serviceErr.Category)
		return model.CardToIBANResponse{}, serviceErr
	}

	if result.IBAN == "" {
		record.Status = "EXTERNAL_FAILED"
		record.DurationMS = durationMS(started)
		record.ResponseJSON = sanitizeExternalResponse(result.RawResponse, card)
		if saveErr := s.saveAudit(ctx, record); saveErr != nil {
			return model.CardToIBANResponse{}, s.persistenceError(saveErr)
		}
		s.logFailure(requestID, "external_invalid_response")
		return model.CardToIBANResponse{}, &Error{
			Code: CodeExternalUnavailable, Message: "پاسخ سرویس خارجی معتبر نبود", HTTPStatus: http.StatusBadGateway,
			Category: "external_invalid_response", Err: errors.New("zarin-hub response did not contain an IBAN"),
		}
	}

	record.Status = "SUCCESS"
	record.DurationMS = durationMS(started)
	record.ExternalHTTPStatus = result.ExternalHTTPStatus
	record.ResponseJSON = sanitizeExternalResponse(result.RawResponse, card)
	if err := s.saveAudit(ctx, record); err != nil {
		return model.CardToIBANResponse{}, s.persistenceError(err)
	}

	return model.CardToIBANResponse{
		Data: model.CardToIBANData{
			IBAN:     result.IBAN,
			BankName: result.BankName,
		},
	}, nil
}

func (s *Service) RecordInvalidRequest(ctx context.Context, requestID, clientIP string, durationMS int64) error {
	record := model.AuditRecord{
		RequestID:  requestID,
		CreatedAt:  time.Now().UTC(),
		DurationMS: durationMS,
		ClientIP:   clientIP,
		CardMasked: "********",
		Status:     "INVALID_REQUEST",
	}
	return s.saveAudit(ctx, record)
}

func (s *Service) saveAudit(ctx context.Context, record model.AuditRecord) error {
	if s.repo == nil {
		return errors.New("audit repository is unavailable")
	}
	return s.repo.SaveAudit(ctx, record)
}

func (s *Service) fromClientError(err error) *Error {
	var clientErr *client.Error
	if !errors.As(err, &clientErr) {
		return s.internalError(err)
	}

	externalStatus := clientErr.ExternalHTTPStatus
	switch clientErr.Kind {
	case client.ErrorAuth:
		return &Error{
			Code: CodeExternalAuth, Message: "سرویس خارجی پذیرش نشد", HTTPStatus: http.StatusBadGateway,
			Category: "external_authentication", ExternalStatus: externalStatus, Err: err,
		}
	case client.ErrorTimeout:
		return &Error{
			Code: CodeExternalTimeout, Message: "زمان پاسخ‌گویی سرویس خارجی به پایان رسید", HTTPStatus: http.StatusGatewayTimeout,
			Category: "external_timeout", ExternalStatus: externalStatus, Err: err,
		}
	case client.ErrorRejected:
		return &Error{
			Code: CodeExternalRejected, Message: "سرویس خارجی درخواست را رد کرد", HTTPStatus: http.StatusBadGateway,
			Category: "external_rejected", ExternalStatus: externalStatus, Err: err,
		}
	case client.ErrorInvalidResponse:
		return &Error{
			Code: CodeExternalUnavailable, Message: "پاسخ سرویس خارجی معتبر نبود", HTTPStatus: http.StatusBadGateway,
			Category: "external_invalid_response", ExternalStatus: externalStatus, Err: err,
		}
	case client.ErrorUnavailable:
		return &Error{
			Code: CodeExternalUnavailable, Message: "سرویس خارجی در دسترس نیست", HTTPStatus: http.StatusBadGateway,
			Category: "external_unavailable", ExternalStatus: externalStatus, Err: err,
		}
	default:
		return s.internalError(err)
	}
}

func (s *Service) persistenceError(err error) *Error {
	s.logFailure("", "persistence")
	return &Error{
		Code: CodePersistence, Message: "ذخیره‌سازی نتیجه انجام نشد", HTTPStatus: http.StatusInternalServerError,
		Category: "persistence", Err: err,
	}
}

func (s *Service) internalError(err error) *Error {
	s.logFailure("", "internal")
	return &Error{
		Code: CodeInternal, Message: "خطای داخلی سرور", HTTPStatus: http.StatusInternalServerError,
		Category: "internal", Err: err,
	}
}

func (s *Service) logFailure(requestID, category string) {
	if s.logger == nil {
		return
	}
	attributes := []any{"error_category", category}
	if requestID != "" {
		attributes = append(attributes, "request_id", requestID)
	}
	s.logger.Error("card-to-iban failed", attributes...)
}

func durationMS(started time.Time) int64 {
	return time.Since(started).Milliseconds()
}

func externalStatusFromError(err error) int {
	var clientErr *client.Error
	if !errors.As(err, &clientErr) {
		return 0
	}
	return clientErr.ExternalHTTPStatus
}

func rawResponseFromError(err error, card string) string {
	var clientErr *client.Error
	if !errors.As(err, &clientErr) {
		return ""
	}
	return sanitizeExternalResponse(clientErr.RawResponse, card)
}

func sanitizeExternalResponse(raw []byte, card string) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return `{"error":"invalid_external_response"}`
	}
	value = sanitizeValue(value, card)
	safe, err := json.Marshal(value)
	if err != nil {
		return `{"error":"invalid_external_response"}`
	}
	return string(safe)
}

func sanitizeValue(value any, card string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			keyLower := strings.ToLower(key)
			if keyLower == "card" || strings.Contains(keyLower, "cardnumber") || isSecretKey(keyLower) {
				typed[key] = maskedValue(card)
				continue
			}
			typed[key] = sanitizeValue(child, card)
		}
		return typed
	case []any:
		for index := range typed {
			typed[index] = sanitizeValue(typed[index], card)
		}
		return typed
	case string:
		if card != "" {
			return strings.ReplaceAll(typed, card, validation.Mask(card))
		}
	}
	return value
}

func isSecretKey(key string) bool {
	switch key {
	case "authorization", "token", "accesstoken", "access_token", "apikey", "api_key", "secret":
		return true
	default:
		return false
	}
}

func maskedValue(card string) string {
	if card == "" {
		return "********"
	}
	return validation.Mask(card)
}
