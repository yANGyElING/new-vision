package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Result values recorded in audit_logs.result.
const (
	ResultSuccess = "success"
	ResultDenied  = "denied"
	ResultError   = "error"
)

// Entry is a single audit record.
type Entry struct {
	ActorUserID  *string
	TenantID     *string
	Action       string
	ResourceType string
	ResourceID   string
	Result       string
	IPAddr       net.IP
	Detail       any
}

// Writer persists audit entries best-effort. A write failure is logged but
// never returned to the caller path: auditing must not become an availability
// or latency point of the main flow.
type Writer struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewWriter(pool *pgxpool.Pool, logger *slog.Logger) *Writer {
	return &Writer{pool: pool, logger: logger}
}

func (w *Writer) Record(ctx context.Context, e Entry) {
	if w == nil || w.pool == nil {
		return
	}
	var detail []byte
	if e.Detail != nil {
		var err error
		detail, err = json.Marshal(e.Detail)
		if err != nil {
			w.logger.Warn("audit: marshal detail", "error", err)
		}
	}
	_, err := w.pool.Exec(ctx,
		`INSERT INTO audit_logs (actor_user_id, tenant_id, action, resource_type, resource_id, result, ip_addr, detail)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.ActorUserID, e.TenantID, e.Action, e.ResourceType, e.ResourceID, e.Result, e.IPAddr, detail)
	if err != nil {
		w.logger.Warn("audit: write failed", "action", e.Action, "error", err)
	}
}
