package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"card-service/backend/internal/model"
)

func TestSQLiteRepositorySaveAudit(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewSQLiteRepository(db)
	record := model.AuditRecord{
		RequestID:          "request-1",
		CreatedAt:          time.Now().UTC(),
		DurationMS:         12,
		ClientIP:           "127.0.0.1",
		CardMasked:         "4532********0366",
		Status:             "SUCCESS",
		ExternalHTTPStatus: 200,
		ResponseJSON:       `{"data":{"card":"4532********0366"}}`,
	}
	if err := repo.SaveAudit(context.Background(), record); err != nil {
		t.Fatalf("SaveAudit() error = %v", err)
	}

	var stored model.AuditRecord
	err = db.QueryRow(`SELECT request_id, card_masked, status, external_http_status, response_json FROM audit_events WHERE request_id = ?`, record.RequestID).
		Scan(&stored.RequestID, &stored.CardMasked, &stored.Status, &stored.ExternalHTTPStatus, &stored.ResponseJSON)
	if err != nil {
		t.Fatalf("query audit record: %v", err)
	}
	if stored.CardMasked != record.CardMasked || stored.Status != record.Status {
		t.Fatalf("stored record = %+v, want %+v", stored, record)
	}
	if strings.Contains(stored.ResponseJSON, "4532015112830366") {
		t.Fatal("stored response contains a full card number")
	}
}
