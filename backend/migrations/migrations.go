package migrations

import _ "embed"

//go:embed 001_create_audit.sql
var CreateAudit string
