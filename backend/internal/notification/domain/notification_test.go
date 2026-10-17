package domain_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/notification/domain"
	"balans/internal/platform/apperr"
)

func TestMessageValidate(t *testing.T) {
	valid := domain.Message{Kind: domain.KindApprovalPending, Title: "PO menunggu persetujuan", Link: "/approvals"}
	require.NoError(t, valid.Validate())

	tests := []struct {
		name  string
		edit  func(*domain.Message)
		field string
	}{
		{"jenis tidak valid", func(m *domain.Message) { m.Kind = "approval" }, "kind"},
		{"judul kosong", func(m *domain.Message) { m.Title = " " }, "title"},
		{"judul panjang", func(m *domain.Message) { m.Title = strings.Repeat("a", 201) }, "title"},
		{"isi panjang", func(m *domain.Message) { m.Body = strings.Repeat("a", 1001) }, "body"},
		{"tautan luar", func(m *domain.Message) { m.Link = "https://contoh.com" }, "link"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := valid
			tt.edit(&m)
			err := m.Validate()
			require.Error(t, err)
			assert.Contains(t, apperr.As(err).Fields, tt.field)
		})
	}
}

func TestRecipientsDeduplicates(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	assert.Equal(t, []uuid.UUID{a, b}, domain.Recipients([]uuid.UUID{a, uuid.Nil, b, a}))
	assert.Empty(t, domain.Recipients(nil))
}
