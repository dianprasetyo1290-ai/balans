package softdelete_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/softdelete"
)

func TestMarkDeleteAndRestore(t *testing.T) {
	var m softdelete.Mark
	require.NoError(t, m.EnsureActive())
	at, by := time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC), uuid.New()
	require.NoError(t, m.Delete(at, by))
	assert.True(t, m.Deleted())
	assert.Equal(t, at, *m.DeletedAt)
	assert.Equal(t, by, *m.DeletedBy)
	assert.True(t, apperr.HasCode(m.Delete(at, by), apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(m.EnsureActive(), apperr.CodeInvalidStateTransition))

	require.NoError(t, m.Restore())
	assert.False(t, m.Deleted())
	assert.Nil(t, m.DeletedBy)
	assert.True(t, apperr.HasCode(m.Restore(), apperr.CodeInvalidStateTransition))
}

func TestRequireRestore(t *testing.T) {
	perm := "catalog.product.write"
	as := func(roles []string, perms ...string) context.Context {
		set := map[string]struct{}{}
		for _, p := range perms {
			set[p] = struct{}{}
		}
		return actor.With(context.Background(), actor.Actor{UserID: uuid.New(), Roles: roles, Permissions: set})
	}

	_, err := softdelete.RequireRestore(context.Background(), perm)
	assert.True(t, apperr.HasCode(err, apperr.CodeAuthSessionExpired))
	_, err = softdelete.RequireRestore(as([]string{"system_admin"}), perm)
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden), "Admin tanpa permission tulis ditolak")
	_, err = softdelete.RequireRestore(as([]string{"head_logistics"}, perm), perm)
	assert.True(t, apperr.HasCode(err, apperr.CodeForbidden), "bukan Admin Sistem ditolak")
	a, err := softdelete.RequireRestore(as([]string{"system_admin"}, perm), perm)
	require.NoError(t, err)
	assert.True(t, softdelete.CanRestore(a, perm))
	assert.False(t, softdelete.CanRestore(actor.Actor{Roles: []string{"system_admin"}}, perm))
}
