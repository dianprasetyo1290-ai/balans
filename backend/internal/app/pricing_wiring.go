package app

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"balans/internal/approval"
	"balans/internal/catalog"
	"balans/internal/identity"
	"balans/internal/party"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/pricing"
)

// buildPricing menyusun modul pricing (R1.1) setelah catalog, party, dan approval. Handler approval
// price_list_version didaftarkan setelah approval dibuat sehingga menggantikan handler pengembangan.
func (a *App) buildPricing() error {
	if !a.Config.IsEnabled(pricing.Name) {
		return nil
	}
	if a.approval == nil || a.identity == nil {
		return errors.New("modul pricing membutuhkan modul identity dan approval (approval daftar harga)")
	}
	d := pricing.Deps{
		Pool: a.Pool, Tx: a.Tx, Clock: a.Clock, Responder: a.Responder, Audit: a.audit,
		Roles: a.identity, Approvals: a.approval,
	}
	if a.catalog != nil {
		d.Products = pricingProducts{catalog: a.catalog}
	}
	if a.party != nil {
		d.Parties = pricingParties{party: a.party}
		d.Eligibility = pricingEligibility{party: a.party}
	}
	a.pricing = pricing.New(d)
	a.approval.RegisterHandler(approval.SubjectPriceListVersion, pricing.PermPriceListRead, a.pricing.ApprovalHandler())
	registerAuditEntities(a.audit, pricing.AuditEntities())
	a.modules = append(a.modules, a.pricing)
	return nil
}

// pricingProducts menyesuaikan API publik catalog: hanya Produk Jadi (termasuk bundel) aktif yang bisa diberi
// harga baru.
type pricingProducts struct {
	catalog *catalog.Module
}

func (p pricingProducts) Products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]pricing.ProductRef, error) {
	list, err := p.catalog.Products(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]pricing.ProductRef, len(list))
	for id, pr := range list {
		ref := pricing.ProductRef{ID: id, SKU: pr.SKU, Name: pr.Name}
		switch {
		case pr.Deleted():
			ref.Unusable = "Produk " + pr.SKU + " sudah dihapus"
		case !pr.IsActive:
			ref.Unusable = "Produk " + pr.SKU + " nonaktif"
		case pr.Type != catalog.TypeFinishedGood:
			ref.Unusable = "Hanya Produk Jadi yang punya harga jual"
		}
		out[id] = ref
	}
	return out, nil
}

// pricingParties membaca nama pelanggan dan mitra untuk harga khusus dan daftar harga mitra.
type pricingParties struct {
	party *party.Module
}

func (p pricingParties) Customers(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]pricing.PartyRef, error) {
	out := make(map[uuid.UUID]pricing.PartyRef, len(ids))
	for _, id := range ids {
		c, err := p.party.CustomerByID(ctx, id)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = pricing.PartyRef{ID: id, Code: c.Code, Name: c.FullName}
	}
	return out, nil
}

func (p pricingParties) Partners(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]pricing.PartyRef, error) {
	out := make(map[uuid.UUID]pricing.PartyRef, len(ids))
	for _, id := range ids {
		m, err := p.party.PartnerByID(ctx, id)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id] = pricing.PartyRef{ID: id, Code: m.Code, Name: m.Name}
	}
	return out, nil
}

// pricingEligibility membaca hak harga karyawan dan daftar harga di PKS mitra dari modul party.
type pricingEligibility struct {
	party *party.Module
}

func (p pricingEligibility) EmployeePriceEligible(ctx context.Context, customerID uuid.UUID, on clock.BusinessDate) (bool, error) {
	ok, err := p.party.EmployeePriceEligible(ctx, customerID, on)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return false, nil
	}
	return ok, err
}

// PartnerAgreementPriceList mengembalikan daftar harga di PKS mitra (konsinyasi atau MT sesuai jenis mitra) yang
// berlaku pada tanggal; nil bila tidak ada.
func (p pricingEligibility) PartnerAgreementPriceList(ctx context.Context, partnerID uuid.UUID, on clock.BusinessDate) (*uuid.UUID, error) {
	m, err := p.party.PartnerByID(ctx, partnerID)
	if apperr.HasCode(err, apperr.CodeNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t party.AgreementType
	switch string(m.Type) {
	case string(party.AgreementConsignment):
		t = party.AgreementConsignment
	case string(party.AgreementModernTrade):
		t = party.AgreementModernTrade
	default:
		return nil, nil
	}
	ag, ok, err := p.party.ActiveAgreement(ctx, partnerID, t, on)
	if err != nil || !ok {
		return nil, err
	}
	return ag.PriceListID, nil
}

var (
	_ pricing.Roles     = (*identity.Module)(nil)
	_ pricing.Approvals = (*approval.Module)(nil)
)
