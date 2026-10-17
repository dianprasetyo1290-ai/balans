package app

import (
	"context"

	"balans/internal/audit"
	"balans/internal/document"
	"balans/internal/platform/clock"
)

// journalNumberer mengisi accounting.Numberer dengan sequence JV modul document (Technical Guideline 5.12).
type journalNumberer struct {
	documents *document.Module
}

func (n journalNumberer) NextJournalNumber(ctx context.Context, businessDate clock.BusinessDate) (string, error) {
	return n.documents.Next(ctx, document.TypeJournal, businessDate)
}

// registerAccountingAuditEntities memberi pemegang permission baca entitas akuntansi akses tab Riwayat.
func registerAccountingAuditEntities(a *audit.Module) {
	for entity, perm := range map[string]string{
		"account":       "accounting.account.read",
		"journal_entry": "accounting.journal.read",
		"channel":       "accounting.dimension.read",
		"sub_channel":   "accounting.dimension.read",
		"division":      "accounting.dimension.read",
		"campaign":      "accounting.dimension.read",
		"fiscal_period": "accounting.fiscal_period.read",
		"expense_type":  "accounting.expense_type.read",
	} {
		a.RegisterEntity(entity, perm)
	}
}
