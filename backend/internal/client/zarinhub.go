package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultEndpoint = "https://zarin-hub.com/api/v5/KYC/CardToIban"
const maxResponseBytes = 1 << 20

type Result struct {
	IBAN               string
	BankName           string
	ExternalHTTPStatus int
	RawResponse        []byte
}

type Response struct {
	Data ResponseData `json:"data"`
	Meta ResponseMeta `json:"meta"`
}

type ResponseData struct {
	IBAN               string `json:"iban"`
	BankName           string `json:"bankName"`
	Card               string `json:"card"`
	Deposit            string `json:"deposit"`
	DepositDescription string `json:"depositDescription"`
	DepositOwners      string `json:"depositOwners"`
	DepositStatus      string `json:"depositStatus"`
	EnglishBankName    string `json:"englishBankName"`
}

type ResponseMeta struct {
	Code         int    `json:"code"`
	ErrorMessage string `json:"errorMessage"`
	ErrorType    string `json:"errorType"`
	Errors       []any  `json:"errors"`
	IsSuccess    bool   `json:"isSuccess"`
	Message      string `json:"message"`
	Status       string `json:"status"`
	TrackID      string `json:"trackId"`
}

type ErrorKind string

const (
	ErrorAuth            ErrorKind = "authentication"
	ErrorRejected        ErrorKind = "rejected"
	ErrorTimeout         ErrorKind = "timeout"
	ErrorUnavailable     ErrorKind = "unavailable"
	ErrorInvalidResponse ErrorKind = "invalid_response"
)

type Error struct {
	Kind               ErrorKind
	ExternalHTTPStatus int
	RawResponse        []byte
	Err                error
}

func (e *Error) Error() string {
	return "zarin-hub request failed"
}

func (e *Error) Unwrap() error {
	return e.Err
}

type ZarinHubClient interface {
	CardToIBAN(ctx context.Context, card string) (Result, error)
}

type HTTPClient struct {
	endpoint   string
	token      string
	httpClient *http.Client
}

func NewZarinHubClient(endpoint, token string, timeout time.Duration) (*HTTPClient, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("zarin-hub access token is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("zarin-hub endpoint is invalid")
	}
	if timeout <= 0 {
		return nil, errors.New("zarin-hub timeout must be positive")
	}
	return &HTTPClient{
		endpoint:   endpoint,
		token:      token,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func (c *HTTPClient) CardToIBAN(ctx context.Context, card string) (Result, error) {
	body, err := json.Marshal(struct {
		Card string `json:"card"`
	}{Card: card})
	if err != nil {
		return Result{}, newError(ErrorInvalidResponse, 0, nil, err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, newError(ErrorUnavailable, 0, nil, err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		kind := ErrorUnavailable
		if isTimeout(ctx, err) {
			kind = ErrorTimeout
		}
		return Result{}, newError(kind, 0, nil, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return Result{}, newError(ErrorUnavailable, response.StatusCode, nil, err)
	}
	if len(raw) > maxResponseBytes {
		return Result{}, newError(ErrorInvalidResponse, response.StatusCode, raw, errors.New("zarin-hub response is too large"))
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Result{}, newError(classifyHTTPStatus(response.StatusCode), response.StatusCode, raw, nil)
	}

	var external Response
	if err := json.Unmarshal(raw, &external); err != nil {
		return Result{}, newError(ErrorInvalidResponse, response.StatusCode, raw, err)
	}
	if !external.Meta.IsSuccess {
		return Result{}, newError(ErrorRejected, response.StatusCode, raw, errors.New("zarin-hub rejected the request"))
	}
	if external.Data.IBAN == "" {
		return Result{}, newError(ErrorInvalidResponse, response.StatusCode, raw, errors.New("zarin-hub response did not contain an IBAN"))
	}

	return Result{
		IBAN:               external.Data.IBAN,
		BankName:           external.Data.BankName,
		ExternalHTTPStatus: response.StatusCode,
		RawResponse:        raw,
	}, nil
}

func classifyHTTPStatus(status int) ErrorKind {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return ErrorAuth
	}
	if status == http.StatusBadRequest || status == http.StatusUnprocessableEntity || status == http.StatusNotFound || status == http.StatusConflict {
		return ErrorRejected
	}
	return ErrorUnavailable
}

func isTimeout(ctx context.Context, err error) bool {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return true
	}
	return false
}

func newError(kind ErrorKind, status int, raw []byte, err error) *Error {
	return &Error{
		Kind:               kind,
		ExternalHTTPStatus: status,
		RawResponse:        raw,
		Err:                err,
	}
}
