// Package postgres adalah repository modul audit.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"balans/internal/audit/domain"
	"balans/internal/audit/postgres/sqlcgen"
	"balans/internal/platform/db"
	"balans/internal/platform/paging"
)

// LogStore menyimpan audit_logs (append-only).
type LogStore struct {
	pool *pgxpool.Pool
}

// NewLogStore membuat LogStore.
func NewLogStore(pool *pgxpool.Pool) *LogStore { return &LogStore{pool: pool} }

func (s *LogStore) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

// Insert menyimpan satu entri di transaksi aktif.
func (s *LogStore) Insert(ctx context.Context, l domain.Log) error {
	changes, err := json.Marshal(l.Changes)
	if err != nil {
		return fmt.Errorf("marshal audit changes: %w", err)
	}
	if err := s.q(ctx).InsertAuditLog(ctx, sqlcgen.InsertAuditLogParams{
		ActorID:     l.ActorID,
		ActorName:   l.ActorName,
		Action:      string(l.Action),
		EntityType:  l.EntityType,
		EntityID:    l.EntityID,
		EntityLabel: l.EntityLabel,
		Changes:     changes,
		Note:        l.Note,
		RequestID:   l.RequestID,
		Ip:          l.IP,
	}); err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

// List mengembalikan audit trail terbaru lebih dulu.
func (s *LogStore) List(ctx context.Context, f domain.Filter, page paging.Page) (paging.Result[domain.Log], error) {
	text := func(v string) pgtype.Text { return pgtype.Text{String: v, Valid: v != ""} }
	id := func(v uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: v, Valid: v != uuid.Nil} }
	params := sqlcgen.ListAuditLogsParams{
		EntityType:  text(f.EntityType),
		EntityID:    id(f.EntityID),
		ActorID:     id(f.ActorID),
		Action:      text(string(f.Action)),
		CreatedFrom: f.From,
		CreatedTo:   f.To,
		RowLimit:    page.Limit(),
		RowOffset:   page.Offset(),
	}
	rows, err := s.q(ctx).ListAuditLogs(ctx, params)
	if err != nil {
		return paging.Result[domain.Log]{}, fmt.Errorf("list audit logs: %w", err)
	}
	total, err := s.q(ctx).CountAuditLogs(ctx, sqlcgen.CountAuditLogsParams{
		EntityType: params.EntityType, EntityID: params.EntityID, ActorID: params.ActorID,
		Action: params.Action, CreatedFrom: params.CreatedFrom, CreatedTo: params.CreatedTo,
	})
	if err != nil {
		return paging.Result[domain.Log]{}, fmt.Errorf("count audit logs: %w", err)
	}
	items := make([]domain.Log, 0, len(rows))
	for _, r := range rows {
		l := domain.Log{
			ID: r.ID, ActorID: r.ActorID, ActorName: r.ActorName, Action: domain.Action(r.Action),
			EntityType: r.EntityType, EntityID: r.EntityID, EntityLabel: r.EntityLabel, Note: r.Note,
			RequestID: r.RequestID, IP: r.Ip, CreatedAt: r.CreatedAt,
		}
		if err := json.Unmarshal(r.Changes, &l.Changes); err != nil {
			return paging.Result[domain.Log]{}, fmt.Errorf("unmarshal audit changes: %w", err)
		}
		items = append(items, l)
	}
	return paging.Result[domain.Log]{Items: items, Page: page, Total: total}, nil
}
