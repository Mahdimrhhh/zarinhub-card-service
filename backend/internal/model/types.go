package model

import "time"

type CardToIBANRequest struct {
	Card string `json:"card"`
}

type CardToIBANData struct {
	IBAN     string `json:"iban"`
	BankName string `json:"bank_name,omitempty"`
}

type CardToIBANResponse struct {
	Data CardToIBANData `json:"data"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

type AuditRecord struct {
	RequestID          string
	CreatedAt          time.Time
	DurationMS         int64
	ClientIP           string
	CardMasked         string
	Status             string
	ExternalHTTPStatus int
	ResponseJSON       string
}
