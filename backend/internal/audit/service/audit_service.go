// Package service berisi use case audit trail.
package service

import (
	"context"
	"sync"

	"balans/internal/audit/domain"
	"balans/internal/platform/actor"
	"balans/internal/platform/logger"
	"balans/internal/platform/paging"
)

// PermissionRead adalah permission melihat seluruh audit trail.
const PermissionRead = "audit.log.read"

type logStore interface {
	Insert(ctx context.Context, l domain.Log) error
	List(ctx context.Context, f domain.Filter, page paging.Page) (paging.Result[domain.Log], error)
}

// Service mencatat dan membaca audit trail.
type Service struct {
	store logStore

	mu         sync.RWMutex
	entityRead map[string]string
}

// New membuat Service.
func New(store logStore) *Service {
	return &Service{store: store, entityRead: map[string]string{}}
}

// RegisterEntity mendaftarkan permission baca entitas, sehingga user yang boleh melihat dokumen
// juga boleh melihat tab Riwayat dokumen itu tanpa akses audit trail penuh.
func (s *Service) RegisterEntity(entityType, readPermission string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entityRead[entityType] = readPermission
}

// Record mencatat perubahan. Dipanggil di layer service di transaksi yang sama dengan perubahan datanya.
func (s *Service) Record(ctx context.Context, e domain.Entry) error {
	changes, err := e.Diff()
	if err != nil {
		return err
	}
	a, _ := actor.From(ctx)
	name := a.FullName
	if name == "" {
		name = "Sistem"
	}
	return s.store.Insert(ctx, domain.Log{
		ActorID:     a.UserID,
		ActorName:   name,
		Action:      e.Action,
		EntityType:  e.EntityType,
		EntityID:    e.EntityID,
		EntityLabel: e.EntityLabel,
		Changes:     changes,
		Note:        e.Note,
		RequestID:   logger.RequestID(ctx),
		IP:          actor.ClientFrom(ctx).IP,
	})
}

// List membaca audit trail. Riwayat satu entitas boleh dibaca pemilik permission baca entitas tersebut.
func (s *Service) List(ctx context.Context, f domain.Filter, page paging.Page) (paging.Result[domain.Log], error) {
	perms := []string{PermissionRead}
	if f.EntityType != "" && f.EntityID != [16]byte{} {
		s.mu.RLock()
		if p, ok := s.entityRead[f.EntityType]; ok {
			perms = append(perms, p)
		}
		s.mu.RUnlock()
	}
	if _, err := actor.RequireAny(ctx, perms...); err != nil {
		return paging.Result[domain.Log]{}, err
	}
	return s.store.List(ctx, f, page)
}
