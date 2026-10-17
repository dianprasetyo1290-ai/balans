package accounting

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/platform/apperr"
)

// DivisionIDByCode mengembalikan ID divisi aktif berdasarkan kode (mis. "LOG") tanpa cek permission, untuk
// dimensi jurnal otomatis modul sumber. NOT_FOUND bila tidak ada.
func (m *Module) DivisionIDByCode(ctx context.Context, code string) (uuid.UUID, error) {
	list, err := m.svc.Store.ListDimensions(ctx, domain.KindDivision, postgres.DimensionFilter{})
	if err != nil {
		return uuid.Nil, err
	}
	for _, d := range list {
		if d.Code == code {
			return d.ID, nil
		}
	}
	return uuid.Nil, apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("divisi %s", code))
}
