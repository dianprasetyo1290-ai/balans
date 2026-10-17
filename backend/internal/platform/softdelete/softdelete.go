// Package softdelete adalah pola hapus master data (Technical Guideline 5.13, S-106): baris tidak dihapus dari
// database, hanya ditandai deleted_at/deleted_by. Data terhapus tidak tampil di list/dropdown, tetap bisa dibaca
// lewat ID oleh dokumen lama, dan hanya Admin Sistem yang boleh memulihkannya (tercatat di audit trail).
package softdelete

import (
	"context"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
)

// RestoreRole adalah kode role yang boleh memulihkan data terhapus (sama dengan identity RoleSystemAdmin).
const RestoreRole = "system_admin"

// Mark adalah penanda hapus yang disematkan di entity master.
type Mark struct {
	DeletedAt *time.Time
	DeletedBy *uuid.UUID
}

// Deleted memeriksa apakah data sudah dihapus.
func (m Mark) Deleted() bool { return m.DeletedAt != nil }

// Delete menandai data terhapus. Data yang sudah terhapus ditolak INVALID_STATE_TRANSITION.
func (m *Mark) Delete(at time.Time, by uuid.UUID) error {
	if m.Deleted() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Data ini sudah dihapus.")
	}
	m.DeletedAt, m.DeletedBy = &at, &by
	return nil
}

// Restore menghapus penanda hapus. Data yang tidak terhapus ditolak INVALID_STATE_TRANSITION.
func (m *Mark) Restore() error {
	if !m.Deleted() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Data ini tidak sedang dihapus.")
	}
	m.DeletedAt, m.DeletedBy = nil, nil
	return nil
}

// EnsureActive menolak perubahan atas data terhapus (data terhapus harus dipulihkan dulu sebelum diubah).
func (m Mark) EnsureActive() error {
	if m.Deleted() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Data ini sudah dihapus. Pulihkan dulu sebelum diubah.")
	}
	return nil
}

// CanRestore memeriksa apakah actor boleh memulihkan data resource dengan permission tulis tersebut.
func CanRestore(a actor.Actor, writePermission string) bool {
	return a.HasRole(RestoreRole) && a.Has(writePermission)
}

// RequireRestore menolak pemulihan selain oleh Admin Sistem yang memegang permission tulis resource.
func RequireRestore(ctx context.Context, writePermission string) (actor.Actor, error) {
	a, err := actor.Require(ctx, writePermission)
	if err != nil {
		return actor.Actor{}, err
	}
	if !a.HasRole(RestoreRole) {
		return actor.Actor{}, apperr.WithMessage(apperr.CodeForbidden, "Hanya Admin Sistem yang bisa memulihkan data yang dihapus.")
	}
	return a, nil
}
