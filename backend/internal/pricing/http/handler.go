// Package http adalah adapter HTTP modul pricing (StrictServerInterface hasil oapi-codegen).
package http

import (
	"context"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/httpx"
	"balans/internal/platform/money"
	"balans/internal/pricing/domain"
	"balans/internal/pricing/service"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/pricing.yaml ../../../api/openapi.bundle.yaml

// Simulator menjalankan resolver harga dan pemeriksaan diskon untuk halaman Harga & Diskon (cek permission di
// dalamnya).
type Simulator interface {
	QuotePrice(ctx context.Context, req domain.PriceRequest) (domain.PriceQuote, error)
	CheckDiscountForActor(ctx context.Context, lines []domain.LineDiscount, doc domain.Discount) (domain.DiscountCheck, money.Amount, error)
}

// PricingHandler mengimplementasikan StrictServerInterface.
type PricingHandler struct {
	svc *service.Service
	sim Simulator
}

var _ StrictServerInterface = (*PricingHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service, sim Simulator) *PricingHandler {
	return &PricingHandler{svc: svc, sim: sim}
}

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *PricingHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

func toDate(d clock.BusinessDate) BusinessDate { return BusinessDate{Time: d.Time()} }

func optDate(d *clock.BusinessDate) *BusinessDate {
	if d == nil {
		return nil
	}
	v := toDate(*d)
	return &v
}

func fromDate(d BusinessDate) clock.BusinessDate {
	if d.IsZero() {
		return clock.BusinessDate{}
	}
	return clock.NewBusinessDate(d.Year(), d.Month(), d.Day())
}

func fromOptDate(d *BusinessDate) *clock.BusinessDate {
	if d == nil {
		return nil
	}
	v := fromDate(*d)
	return &v
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func optMoney(a *money.Amount) *Money {
	if a == nil {
		return nil
	}
	s := a.String()
	return &s
}

func percentText(d decimal.Decimal) string { return d.Round(money.RateScale).String() }

func optPercent(d *decimal.Decimal) *PricingPercent {
	if d == nil {
		return nil
	}
	s := percentText(*d)
	return &s
}

func boolValue(p *bool) bool { return p != nil && *p }

func productFields(refs map[uuid.UUID]service.ProductRef, id uuid.UUID) (string, string) {
	r := refs[id]
	return r.SKU, r.Name
}

// parseDecimal membaca string desimal request; error menjadi VALIDATION_FAILED pada field.
func parseDecimal(field, s string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return decimal.Zero, apperr.Validation(map[string]string{field: "Angka tidak valid"})
	}
	return d, nil
}

func itoa(i int) string { return strconv.Itoa(i) }
