package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"

	"card-service/backend/internal/model"
	"card-service/backend/migrations"
)

type Repository interface {
	SaveAudit(ctx context.Context, record model.AuditRecord) error
}

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, migrations.CreateAudit)
	return err
}

func (r *SQLiteRepository) SaveAudit(ctx context.Context, record model.AuditRecord) error {
	if r == nil || r.db == nil {
		return errors.New("repository is unavailable")
	}
	var externalStatus any
	if record.ExternalHTTPStatus > 0 {
		externalStatus = record.ExternalHTTPStatus
	}
	var responseJSON any
	if record.ResponseJSON != "" {
		responseJSON = record.ResponseJSON
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_events (
			request_id,
			created_at,
			duration_ms,
			client_ip,
			card_masked,
			status,
			external_http_status,
			response_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, record.RequestID, record.CreatedAt.UTC().Format(time.RFC3339Nano), record.DurationMS, record.ClientIP, record.CardMasked, record.Status, externalStatus, responseJSON)
	return err
}
