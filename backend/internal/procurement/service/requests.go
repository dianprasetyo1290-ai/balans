package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/procurement/domain"
	"balans/internal/procurement/postgres"
)

// RequestFilter adalah kriteria daftar PR.
type RequestFilter = postgres.RequestFilter

// RequestSortFields adalah field sort daftar PR.
var RequestSortFields = []string{"number", "businessDate", "estimatedTotal"}

// RequestView adalah PR beserta data tampilan.
type RequestView struct {
	domain.PurchaseRequest
	DivisionName  string
	RequesterName string
	Products      map[uuid.UUID]ProductRef
	// Orders adalah baris PO yang dibuat dari PR ini.
	Orders []postgres.RequestLineOrder
}

// RequestSummary adalah baris daftar PR.
type RequestSummary struct {
	domain.PurchaseRequest
	DivisionName  string
	RequesterName string
}

var requestActionPerms = map[domain.Action]string{
	domain.ActionEdit: PermRequestWrite, domain.ActionSubmit: PermRequestSubmit, domain.ActionCancel: PermRequestWrite,
	domain.ActionRevise: PermRequestWrite, domain.ActionClose: PermRequestWrite, domain.ActionCreateOrder: PermOrderWrite,
}

// RequestActions mengembalikan aksi PR yang boleh dilakukan actor (allowedActions).
func RequestActions(ctx context.Context, r domain.PurchaseRequest) []string {
	return filterActions(ctx, r.Actions(), requestActionPerms)
}

func requestSnapshot(r domain.PurchaseRequest) map[string]any {
	lines := make([]map[string]any, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = map[string]any{
			"productId": l.ProductID, "description": l.Description, "qty": l.Qty.String(),
			"estimatedPrice": l.EstimatedPrice.String(),
		}
	}
	m := map[string]any{
		"divisionId": r.DivisionID, "businessDate": r.BusinessDate.String(), "note": r.Note,
		"estimatedTotal": r.EstimatedTotal.String(), "status": string(r.Status), "lines": lines,
	}
	if r.NeededDate != nil {
		m["neededDate"] = r.NeededDate.String()
	}
	return m
}

func requestLabel(r domain.PurchaseRequest) string {
	if r.Number != "" {
		return r.Number
	}
	return "Draft Purchase Request"
}

func (s *Service) requestProducts(ctx context.Context, lines []domain.RequestLineInput) (map[uuid.UUID]domain.ProductInfo, error) {
	ids := make([]uuid.UUID, 0, len(lines))
	for _, l := range lines {
		if l.ProductID != nil {
			ids = append(ids, *l.ProductID)
		}
	}
	refs, err := s.Catalog.Products(ctx, uniqueIDs(ids))
	if err != nil {
		return nil, err
	}
	return productInfos(refs), nil
}

func (s *Service) validateRequest(ctx context.Context, in domain.RequestInput) (map[uuid.UUID]domain.ProductInfo, error) {
	products, err := s.requestProducts(ctx, in.Lines)
	if err != nil {
		return nil, err
	}
	if err := in.Validate(products); err != nil {
		return nil, err
	}
	if err := s.Ledger.CheckDivision(ctx, in.DivisionID, in.BusinessDate, "divisionId"); err != nil {
		return nil, err
	}
	return products, nil
}

// ListRequests mengembalikan daftar PR.
func (s *Service) ListRequests(ctx context.Context, f RequestFilter, page paging.Page) (paging.Result[RequestSummary], error) {
	if _, err := actor.Require(ctx, PermRequestRead); err != nil {
		return paging.Result[RequestSummary]{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	res, err := s.Store.ListRequests(ctx, f, page)
	if err != nil {
		return paging.Result[RequestSummary]{}, err
	}
	divisionIDs, userIDs := make([]uuid.UUID, 0, len(res.Items)), make([]uuid.UUID, 0, len(res.Items))
	for _, r := range res.Items {
		divisionIDs, userIDs = append(divisionIDs, r.DivisionID), append(userIDs, r.RequestedBy)
	}
	divisions, err := s.Ledger.DivisionNames(ctx, uniqueIDs(divisionIDs))
	if err != nil {
		return paging.Result[RequestSummary]{}, err
	}
	users, err := s.Users.UserNames(ctx, uniqueIDs(userIDs))
	if err != nil {
		return paging.Result[RequestSummary]{}, err
	}
	out := paging.Result[RequestSummary]{Page: res.Page, Total: res.Total, Items: make([]RequestSummary, len(res.Items))}
	for i, r := range res.Items {
		out.Items[i] = RequestSummary{PurchaseRequest: r, DivisionName: divisions[r.DivisionID], RequesterName: users[r.RequestedBy]}
	}
	return out, nil
}

// GetRequest mengembalikan detail PR.
func (s *Service) GetRequest(ctx context.Context, id uuid.UUID) (RequestView, error) {
	if _, err := actor.Require(ctx, PermRequestRead); err != nil {
		return RequestView{}, err
	}
	r, err := s.Store.GetRequest(ctx, id, false)
	if err != nil {
		return RequestView{}, err
	}
	return s.requestView(ctx, r)
}

func (s *Service) requestView(ctx context.Context, r domain.PurchaseRequest) (RequestView, error) {
	v := RequestView{PurchaseRequest: r}
	ids := make([]uuid.UUID, 0, len(r.Lines))
	for _, l := range r.Lines {
		if l.ProductID != nil {
			ids = append(ids, *l.ProductID)
		}
	}
	var err error
	if v.Products, err = s.Catalog.Products(ctx, uniqueIDs(ids)); err != nil {
		return RequestView{}, err
	}
	divisions, err := s.Ledger.DivisionNames(ctx, []uuid.UUID{r.DivisionID})
	if err != nil {
		return RequestView{}, err
	}
	users, err := s.Users.UserNames(ctx, []uuid.UUID{r.RequestedBy})
	if err != nil {
		return RequestView{}, err
	}
	v.DivisionName, v.RequesterName = divisions[r.DivisionID], users[r.RequestedBy]
	if r.ID != uuid.Nil {
		if v.Orders, err = s.Store.RequestLineOrders(ctx, r.ID); err != nil {
			return RequestView{}, err
		}
	}
	return v, nil
}

// CreateRequest menyimpan draft PR. Pembuat menjadi peminta.
func (s *Service) CreateRequest(ctx context.Context, in domain.RequestInput) (RequestView, error) {
	a, err := actor.Require(ctx, PermRequestWrite)
	if err != nil {
		return RequestView{}, err
	}
	products, err := s.validateRequest(ctx, in)
	if err != nil {
		return RequestView{}, err
	}
	r, err := domain.NewPurchaseRequest(in, products, a.UserID)
	if err != nil {
		return RequestView{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if r, err = s.Store.InsertRequest(ctx, r, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityPurchaseRequest, EntityID: r.ID, EntityLabel: requestLabel(r),
			After: requestSnapshot(r),
		})
	})
	if err != nil {
		return RequestView{}, err
	}
	return s.requestView(ctx, r)
}

// mutateRequest mengunci PR, memeriksa versi, menjalankan fn, lalu menyimpan dan mencatat audit trail.
func (s *Service) mutateRequest(ctx context.Context, id uuid.UUID, version int, action audit.Action, replaceLines bool,
	fn func(ctx context.Context, r *domain.PurchaseRequest) (string, error),
) (RequestView, error) {
	a, _ := actor.From(ctx)
	var r domain.PurchaseRequest
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		if r, err = s.Store.GetRequest(ctx, id, true); err != nil {
			return err
		}
		if r.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := requestSnapshot(r)
		note, err := fn(ctx, &r)
		if err != nil {
			return err
		}
		if err := s.Store.SaveRequest(ctx, r, version, a.UserID, replaceLines); err != nil {
			return err
		}
		r.Version = version + 1
		return s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityPurchaseRequest, EntityID: r.ID, EntityLabel: requestLabel(r),
			Before: before, After: requestSnapshot(r), Note: note,
		})
	})
	if err != nil {
		return RequestView{}, err
	}
	if r, err = s.Store.GetRequest(ctx, id, false); err != nil {
		return RequestView{}, err
	}
	return s.requestView(ctx, r)
}

// UpdateRequest mengubah draft PR.
func (s *Service) UpdateRequest(ctx context.Context, id uuid.UUID, version int, in domain.RequestInput) (RequestView, error) {
	if _, err := actor.Require(ctx, PermRequestWrite); err != nil {
		return RequestView{}, err
	}
	products, err := s.validateRequest(ctx, in)
	if err != nil {
		return RequestView{}, err
	}
	return s.mutateRequest(ctx, id, version, audit.ActionUpdate, true, func(_ context.Context, r *domain.PurchaseRequest) (string, error) {
		return "", r.Update(in, products)
	})
}

// SubmitRequest mengajukan PR: nomor PR diberikan dan approval Head divisi peminta diminta di transaksi yang sama.
func (s *Service) SubmitRequest(ctx context.Context, id uuid.UUID, version int) (RequestView, error) {
	if _, err := actor.Require(ctx, PermRequestSubmit); err != nil {
		return RequestView{}, err
	}
	return s.mutateRequest(ctx, id, version, audit.ActionSubmit, false, func(ctx context.Context, r *domain.PurchaseRequest) (string, error) {
		// Divisi & produk diperiksa ulang: master bisa berubah sejak draft disimpan.
		if _, err := s.validateRequest(ctx, requestInputOf(*r)); err != nil {
			return "", err
		}
		number := ""
		if r.NeedsNumber() {
			var err error
			if number, err = s.Numberer.Next(ctx, DocPurchaseRequest, r.BusinessDate); err != nil {
				return "", err
			}
		}
		if err := r.Submit(number, s.Clock.Now()); err != nil {
			return "", err
		}
		division := r.DivisionID
		_, err := s.Approvals.Request(ctx, approval.Subject{
			Type: approval.SubjectPurchaseRequest, ID: r.ID, Label: r.Number, Link: RequestPath(r.ID),
			Summary: requestSummary(*r), Amount: r.EstimatedTotal, Division: &division,
		})
		return "", err
	})
}

func requestInputOf(r domain.PurchaseRequest) domain.RequestInput {
	in := domain.RequestInput{DivisionID: r.DivisionID, BusinessDate: r.BusinessDate, NeededDate: r.NeededDate, Note: r.Note}
	for _, l := range r.Lines {
		in.Lines = append(in.Lines, domain.RequestLineInput{
			ProductID: l.ProductID, Description: l.Description, Qty: l.Qty, EstimatedPrice: l.EstimatedPrice, Note: l.Note,
		})
	}
	return in
}

func requestSummary(r domain.PurchaseRequest) string {
	summary := fmt.Sprintf("%d baris", len(r.Lines))
	if r.Note != "" {
		summary += " · " + truncate(r.Note, 120)
	}
	return summary
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

// ReviseRequest mengembalikan PR yang ditolak ke Draft.
func (s *Service) ReviseRequest(ctx context.Context, id uuid.UUID, version int) (RequestView, error) {
	if _, err := actor.Require(ctx, PermRequestWrite); err != nil {
		return RequestView{}, err
	}
	return s.mutateRequest(ctx, id, version, audit.ActionStatusChange, false, func(_ context.Context, r *domain.PurchaseRequest) (string, error) {
		return "Direvisi", r.Revise()
	})
}

// CancelRequest membatalkan draft PR.
func (s *Service) CancelRequest(ctx context.Context, id uuid.UUID, version int) (RequestView, error) {
	if _, err := actor.Require(ctx, PermRequestWrite); err != nil {
		return RequestView{}, err
	}
	return s.mutateRequest(ctx, id, version, audit.ActionCancel, false, func(_ context.Context, r *domain.PurchaseRequest) (string, error) {
		return "", r.Cancel()
	})
}

// CloseRequest menutup PR disetujui yang sisanya tidak dijadikan PO.
func (s *Service) CloseRequest(ctx context.Context, id uuid.UUID, version int, reason string) (RequestView, error) {
	if _, err := actor.Require(ctx, PermRequestWrite); err != nil {
		return RequestView{}, err
	}
	return s.mutateRequest(ctx, id, version, audit.ActionStatusChange, false, func(_ context.Context, r *domain.PurchaseRequest) (string, error) {
		return "Ditutup: " + strings.TrimSpace(reason), r.Close(reason)
	})
}

// decideRequest dipanggil handler approval di transaksi keputusan (tanpa cek permission dokumen).
func (s *Service) decideRequest(ctx context.Context, id uuid.UUID, approve bool) error {
	r, err := s.Store.GetRequest(ctx, id, true)
	if err != nil {
		return err
	}
	version := r.Version
	if approve {
		err = r.Approve(s.Clock.Now())
	} else {
		err = r.Reject(s.Clock.Now())
	}
	if err != nil {
		return err
	}
	return s.Store.SaveRequest(ctx, r, version, actor.UserID(ctx), false)
}

// OnRequestApproved menandai PR Disetujui setelah Head divisi menyetujui.
func (s *Service) OnRequestApproved(ctx context.Context, d approval.Decided) error {
	return s.decideRequest(ctx, d.Subject.ID, true)
}

// OnRequestRejected menandai PR Ditolak.
func (s *Service) OnRequestRejected(ctx context.Context, d approval.Decided) error {
	return s.decideRequest(ctx, d.Subject.ID, false)
}

// recalcRequests menghitung ulang qty PR yang sudah dijadikan PO dan status PR (Selesai/Disetujui).
func (s *Service) recalcRequests(ctx context.Context, requestIDs []uuid.UUID) error {
	for _, id := range uniqueIDs(requestIDs) {
		r, err := s.Store.GetRequest(ctx, id, true)
		if err != nil {
			return err
		}
		rows, err := s.Store.RequestLineOrders(ctx, id)
		if err != nil {
			return err
		}
		fulfilled := map[uuid.UUID]decimal.Decimal{}
		for _, row := range rows {
			l := domain.OrderLine{OrderedQty: row.OrderedQty, ReceivedQty: row.ReceivedQty}
			fulfilled[row.RequestLineID] = fulfilled[row.RequestLineID].Add(l.FulfilledQty(row.OrderStatus))
		}
		before := r.Status
		version := r.Version
		if err := r.ApplyFulfillment(fulfilled); err != nil {
			return err
		}
		if err := s.Store.SetRequestLinesFulfilled(ctx, r.Lines); err != nil {
			return err
		}
		if r.Status == before {
			continue
		}
		if err := s.Store.SaveRequest(ctx, r, version, actor.UserID(ctx), false); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionStatusChange, EntityType: EntityPurchaseRequest, EntityID: r.ID, EntityLabel: r.Number,
			Before: map[string]any{"status": string(before)}, After: map[string]any{"status": string(r.Status)},
			Note: "Dihitung ulang dari Purchase Order",
		}); err != nil {
			return err
		}
	}
	return nil
}
