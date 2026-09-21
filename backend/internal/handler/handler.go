package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"card-service/backend/internal/model"
	"card-service/backend/internal/service"
)

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type CardService interface {
	CardToIBAN(ctx context.Context, card, requestID, clientIP string) (model.CardToIBANResponse, error)
	RecordInvalidRequest(ctx context.Context, requestID, clientIP string, durationMS int64) error
}

type Handler struct {
	service    CardService
	logger     *slog.Logger
	corsOrigin string
}

func NewHandler(svc CardService, logger *slog.Logger, corsOrigin string) http.Handler {
	if strings.TrimSpace(corsOrigin) == "" {
		corsOrigin = "http://localhost:3000"
	}
	return &Handler{service: svc, logger: logger, corsOrigin: corsOrigin}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	id := requestID(r)
	w.Header().Set("X-Request-ID", id)
	h.setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx := context.WithValue(r.Context(), requestIDKey{}, id)
	r = r.WithContext(ctx)
	h.route(w, r, id, started)
}

func (h *Handler) route(w http.ResponseWriter, r *http.Request, id string, started time.Time) {
	switch {
	case r.URL.Path == "/health" && r.Method == http.MethodGet:
		h.health(w, r)
	case r.URL.Path == "/api/v1/card-to-iban" && r.Method == http.MethodPost:
		h.cardToIBAN(w, r, id, started)
	case r.URL.Path == "/api/v1/card-to-iban":
		h.methodNotAllowed(w, http.MethodPost)
	default:
		h.writeError(w, http.StatusNotFound, model.ErrorResponse{Error: model.ErrorBody{
			Code: "NOT_FOUND", Message: "مسیر پیدا نشد",
		}})
	}
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, model.HealthResponse{Status: "ok"})
}

func (h *Handler) cardToIBAN(w http.ResponseWriter, r *http.Request, id string, started time.Time) {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)

	if h.service == nil {
		h.writeError(w, http.StatusInternalServerError, model.ErrorResponse{Error: model.ErrorBody{
			Code: service.CodeInternal, Message: "خطای داخلی سرور",
		}})
		return
	}

	var request model.CardToIBANRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		h.recordInvalidRequest(r.Context(), id, clientIP(r), time.Since(started).Milliseconds(), err)
		h.writeError(w, http.StatusBadRequest, model.ErrorResponse{Error: model.ErrorBody{
			Code: service.CodeInvalidRequest, Message: "درخواست معتبر نیست",
		}})
		h.logRequest(id, "card_to_iban", started, http.StatusBadRequest, "", "invalid_request", err)
		return
	}
	if err := ensureEOF(decoder); err != nil {
		h.recordInvalidRequest(r.Context(), id, clientIP(r), time.Since(started).Milliseconds(), err)
		h.writeError(w, http.StatusBadRequest, model.ErrorResponse{Error: model.ErrorBody{
			Code: service.CodeInvalidRequest, Message: "درخواست معتبر نیست",
		}})
		h.logRequest(id, "card_to_iban", started, http.StatusBadRequest, "", "invalid_request", err)
		return
	}

	response, err := h.service.CardToIBAN(r.Context(), request.Card, id, clientIP(r))
	if err != nil {
		status, externalStatus := writeServiceError(w, err)
		h.logRequest(id, "card_to_iban", started, status, externalStatus, errorCategory(err), err)
		return
	}

	writeJSON(w, http.StatusOK, response)
	h.logRequest(id, "card_to_iban", started, http.StatusOK, "", "", nil)
}

func writeServiceError(w http.ResponseWriter, err error) (int, string) {
	var serviceErr *service.Error
	if errors.As(err, &serviceErr) {
		status := serviceErr.HTTPStatus
		if status == 0 {
			status = http.StatusInternalServerError
		}
		externalStatus := ""
		if serviceErr.ExternalStatus > 0 {
			externalStatus = strconv.Itoa(serviceErr.ExternalStatus)
		}
		writeJSON(w, status, model.ErrorResponse{Error: model.ErrorBody{
			Code: serviceErr.Code, Message: serviceErr.Message,
		}})
		return status, externalStatus
	}
	writeJSON(w, http.StatusInternalServerError, model.ErrorResponse{Error: model.ErrorBody{
		Code: service.CodeInternal, Message: "خطای داخلی سرور",
	}})
	return http.StatusInternalServerError, ""
}

func (h *Handler) recordInvalidRequest(ctx context.Context, id, ip string, durationMS int64, reason error) {
	if h.service == nil {
		return
	}
	if err := h.service.RecordInvalidRequest(ctx, id, ip, durationMS); err != nil && h.logger != nil {
		h.logger.Error("invalid request audit persistence failed", "error_category", "persistence")
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, response model.ErrorResponse) {
	writeJSON(w, status, response)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (h *Handler) methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	h.writeError(w, http.StatusMethodNotAllowed, model.ErrorResponse{Error: model.ErrorBody{
		Code: "METHOD_NOT_ALLOWED", Message: "متد درخواست مجاز نیست",
	}})
}

func (h *Handler) setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", h.corsOrigin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")
	w.Header().Set("Vary", "Origin")
}

func (h *Handler) logRequest(id, operation string, started time.Time, status int, externalStatus string, category string, err error) {
	if h.logger == nil {
		return
	}
	attributes := []any{
		"request_id", id,
		"operation", operation,
		"duration_ms", time.Since(started).Milliseconds(),
		"status", status,
	}
	if externalStatus != "" {
		attributes = append(attributes, "external_status", externalStatus)
	}
	if category != "" {
		attributes = append(attributes, "error_category", category)
	}
	if err == nil {
		h.logger.Info("http request completed", attributes...)
		return
	}
	h.logger.Error("http request completed", attributes...)
}

func requestID(r *http.Request) string {
	if candidate := r.Header.Get("X-Request-ID"); safeRequestID.MatchString(candidate) {
		return candidate
	}
	return generateRequestID()
}

func generateRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	if r.RemoteAddr == "" {
		return "unknown"
	}
	return r.RemoteAddr
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("request body must contain one JSON object")
	}
	return err
}

func errorCategory(err error) string {
	var serviceErr *service.Error
	if errors.As(err, &serviceErr) {
		return serviceErr.Category
	}
	return "internal"
}

type requestIDKey struct{}
