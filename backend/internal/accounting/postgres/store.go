// Package postgres adalah repository modul accounting.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/money"
)

// ErrAlreadyPosted: dokumen sumber sudah punya jurnal untuk rule yang sama (uq_journal_entries_source).
var ErrAlreadyPosted = errors.New("accounting: dokumen sumber sudah dijurnal untuk rule ini")

// Store menyimpan COA, dimensi, periode, posting rule, dan jurnal.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func uuidPtr(id uuid.NullUUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	v := id.UUID
	return &v
}

func nullAmount(a *money.Amount) decimal.NullDecimal {
	if a == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: a.Decimal(), Valid: true}
}

func amountPtr(d decimal.NullDecimal) *money.Amount {
	if !d.Valid {
		return nil
	}
	a := money.FromDecimal(d.Decimal)
	return &a
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func date(d *clock.BusinessDate) pgtype.Date {
	if d == nil || d.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: d.Time(), Valid: true}
}

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

func pgError(err error) *pgconn.PgError {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr
	}
	return nil
}

// mapWriteError menerjemahkan pelanggaran constraint dan trigger menjadi error untuk user.
func mapWriteError(err error, what string) error {
	pe := pgError(err)
	if pe == nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	switch {
	case pe.Code == "23505" && pe.ConstraintName == "uq_journal_entries_source":
		return &apperr.Error{Code: apperr.CodeInvalidStateTransition, Message: "Dokumen sumber sudah dijurnal untuk rule ini.", Err: ErrAlreadyPosted}
	case pe.Code == "23505" && (pe.ConstraintName == "uq_accounts_code" || pe.ConstraintName == "uq_channels_code" ||
		pe.ConstraintName == "uq_sub_channels_code" || pe.ConstraintName == "uq_divisions_code" || pe.ConstraintName == "uq_campaigns_code" ||
		pe.ConstraintName == "uq_expense_types_code"):
		return &apperr.Error{Code: apperr.CodeDuplicateCode, Message: apperr.CodeDuplicateCode.DefaultMessage(),
			Fields: map[string]string{"code": "Kode sudah dipakai (termasuk data yang sudah dihapus)"}, Err: err}
	case pe.Code == "BL001":
		return apperr.Wrap(apperr.CodeInvalidStateTransition, err)
	case pe.Code == "BL002":
		return &apperr.Error{Code: apperr.CodeValidationFailed, Message: apperr.CodeValidationFailed.DefaultMessage(),
			Fields: map[string]string{"lines": "Jurnal tidak balance"}, Err: err}
	}
	return fmt.Errorf("%s: %w", what, err)
}

// --- Akun -------------------------------------------------------------------------------------

func toAccount(r sqlcgen.Account) domain.Account {
	return domain.Account{
		ID: r.ID, Code: r.Code, Name: r.Name, Group: domain.Group(r.AccountGroup), NormalBalance: domain.NormalBalance(r.NormalBalance),
		ParentID: uuidPtr(r.ParentID), IsPostable: r.IsPostable, AllowManualPosting: r.AllowManualPosting, IsActive: r.IsActive,
		Description: r.Description, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

// ListAccounts mengembalikan COA urut kode.
func (s *Store) ListAccounts(ctx context.Context, includeInactive bool) ([]domain.Account, error) {
	rows, err := s.q(ctx).ListAccounts(ctx, includeInactive)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	out := make([]domain.Account, len(rows))
	for i, r := range rows {
		out[i] = toAccount(r)
	}
	return out, nil
}

// GetAccount membaca satu akun; forUpdate mengunci baris.
func (s *Store) GetAccount(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Account, error) {
	var (
		r   sqlcgen.Account
		err error
	)
	if forUpdate {
		r, err = s.q(ctx).GetAccountForUpdate(ctx, id)
	} else {
		r, err = s.q(ctx).GetAccount(ctx, id)
	}
	if err != nil {
		return domain.Account{}, notFound(err, "get account")
	}
	return toAccount(r), nil
}

// AccountsByIDs membaca akun aktif/nonaktif yang belum dihapus.
func (s *Store) AccountsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.Account, error) {
	rows, err := s.q(ctx).GetAccountsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get accounts by ids: %w", err)
	}
	out := make(map[uuid.UUID]domain.Account, len(rows))
	for _, r := range rows {
		out[r.ID] = toAccount(r)
	}
	return out, nil
}

// AccountsByCodes membaca akun berdasarkan kode.
func (s *Store) AccountsByCodes(ctx context.Context, codes []string) (map[string]domain.Account, error) {
	rows, err := s.q(ctx).GetAccountsByCodes(ctx, codes)
	if err != nil {
		return nil, fmt.Errorf("get accounts by codes: %w", err)
	}
	out := make(map[string]domain.Account, len(rows))
	for _, r := range rows {
		out[r.Code] = toAccount(r)
	}
	return out, nil
}

// AccountAncestors mengembalikan rantai akun dari id ke atas (termasuk id).
func (s *Store) AccountAncestors(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	ids, err := s.q(ctx).AccountAncestors(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("account ancestors: %w", err)
	}
	return ids, nil
}

// InsertAccount menyimpan akun baru.
func (s *Store) InsertAccount(ctx context.Context, a domain.Account, actorID uuid.UUID) (domain.Account, error) {
	r, err := s.q(ctx).InsertAccount(ctx, sqlcgen.InsertAccountParams{
		Code: a.Code, Name: a.Name, AccountGroup: int16(a.Group), NormalBalance: string(a.NormalBalance), //nolint:gosec // kelompok 1–7
		ParentID: nullUUID(a.ParentID), IsPostable: a.IsPostable, AllowManualPosting: a.AllowManualPosting,
		IsActive: a.IsActive, Description: a.Description, ActorID: actorID,
	})
	if err != nil {
		return domain.Account{}, mapWriteError(err, "insert account")
	}
	return toAccount(r), nil
}

// UpdateAccount menyimpan perubahan akun dengan optimistic locking.
func (s *Store) UpdateAccount(ctx context.Context, a domain.Account, version int, actorID uuid.UUID) (domain.Account, error) {
	r, err := s.q(ctx).UpdateAccount(ctx, sqlcgen.UpdateAccountParams{
		Name: a.Name, NormalBalance: string(a.NormalBalance), ParentID: nullUUID(a.ParentID), IsPostable: a.IsPostable,
		AllowManualPosting: a.AllowManualPosting, IsActive: a.IsActive, Description: a.Description, ActorID: actorID,
		ID: a.ID, Version: int32(version), //nolint:gosec // versi kecil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, apperr.New(apperr.CodeVersionConflict)
	}
	if err != nil {
		return domain.Account{}, mapWriteError(err, "update account")
	}
	return toAccount(r), nil
}

// DeleteAccount melakukan soft delete.
func (s *Store) DeleteAccount(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).SoftDeleteAccount(ctx, sqlcgen.SoftDeleteAccountParams{ActorID: actorID, ID: id, Version: int32(version)}) //nolint:gosec // versi kecil
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// AccountUsage menghitung pemakaian akun.
func (s *Store) AccountUsage(ctx context.Context, id uuid.UUID) (domain.AccountUsage, error) {
	r, err := s.q(ctx).AccountUsage(ctx, id)
	if err != nil {
		return domain.AccountUsage{}, fmt.Errorf("account usage: %w", err)
	}
	return domain.AccountUsage{
		JournalLines: r.JournalLines, Children: r.Children, RuleMappings: r.RuleMappings, ExpenseTypes: r.ExpenseTypes,
	}, nil
}

// --- Dimensi ----------------------------------------------------------------------------------

func fromChannel(r sqlcgen.Channel) domain.DimensionValue {
	return domain.DimensionValue{ID: r.ID, Kind: domain.KindChannel, Code: r.Code, Name: r.Name, IsActive: r.IsActive,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version)}
}

func fromSubChannel(r sqlcgen.SubChannel) domain.DimensionValue {
	ch := r.ChannelID
	return domain.DimensionValue{ID: r.ID, Kind: domain.KindSubChannel, Code: r.Code, Name: r.Name, ChannelID: &ch,
		PartyType: r.PartyType, PartyID: uuidPtr(r.PartyID), IsActive: r.IsActive, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt, Version: int(r.Version)}
}

func fromDivision(r sqlcgen.Division) domain.DimensionValue {
	return domain.DimensionValue{ID: r.ID, Kind: domain.KindDivision, Code: r.Code, Name: r.Name, IsActive: r.IsActive,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version)}
}

func fromCampaign(r sqlcgen.Campaign) domain.DimensionValue {
	return domain.DimensionValue{ID: r.ID, Kind: domain.KindCampaign, Code: r.Code, Name: r.Name, ValidFrom: r.ValidFrom,
		ValidTo: r.ValidTo, Budget: amountPtr(r.BudgetAmount), Description: r.Description, IsActive: r.IsActive,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: int(r.Version)}
}

func mapRows[T any](rows []T, f func(T) domain.DimensionValue) []domain.DimensionValue {
	out := make([]domain.DimensionValue, len(rows))
	for i, r := range rows {
		out[i] = f(r)
	}
	return out
}

// DimensionFilter adalah filter list master dimensi.
type DimensionFilter struct {
	IncludeInactive bool
	ChannelID       *uuid.UUID
	Query           string
}

// ListDimensions mengembalikan master dimensi satu jenis.
func (s *Store) ListDimensions(ctx context.Context, kind domain.DimensionKind, f DimensionFilter) ([]domain.DimensionValue, error) {
	q := s.q(ctx)
	var (
		out []domain.DimensionValue
		err error
	)
	switch kind {
	case domain.KindChannel:
		var rows []sqlcgen.Channel
		rows, err = q.ListChannels(ctx, f.IncludeInactive)
		out = mapRows(rows, fromChannel)
	case domain.KindSubChannel:
		var rows []sqlcgen.SubChannel
		rows, err = q.ListSubChannels(ctx, sqlcgen.ListSubChannelsParams{IncludeInactive: f.IncludeInactive, ChannelID: nullUUID(f.ChannelID)})
		out = mapRows(rows, fromSubChannel)
	case domain.KindDivision:
		var rows []sqlcgen.Division
		rows, err = q.ListDivisions(ctx, f.IncludeInactive)
		out = mapRows(rows, fromDivision)
	case domain.KindCampaign:
		var rows []sqlcgen.Campaign
		rows, err = q.ListCampaigns(ctx, sqlcgen.ListCampaignsParams{IncludeInactive: f.IncludeInactive, Q: text(f.Query)})
		out = mapRows(rows, fromCampaign)
	default:
		return nil, fmt.Errorf("unknown dimension kind %q", kind)
	}
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", kind, err)
	}
	return out, nil
}

// GetDimensionForUpdate mengunci satu master dimensi.
func (s *Store) GetDimensionForUpdate(ctx context.Context, kind domain.DimensionKind, id uuid.UUID) (domain.DimensionValue, error) {
	q := s.q(ctx)
	var (
		v   domain.DimensionValue
		err error
	)
	switch kind {
	case domain.KindChannel:
		var r sqlcgen.Channel
		r, err = q.GetChannelForUpdate(ctx, id)
		v = fromChannel(r)
	case domain.KindSubChannel:
		var r sqlcgen.SubChannel
		r, err = q.GetSubChannelForUpdate(ctx, id)
		v = fromSubChannel(r)
	case domain.KindDivision:
		var r sqlcgen.Division
		r, err = q.GetDivisionForUpdate(ctx, id)
		v = fromDivision(r)
	case domain.KindCampaign:
		var r sqlcgen.Campaign
		r, err = q.GetCampaignForUpdate(ctx, id)
		v = fromCampaign(r)
	default:
		return v, fmt.Errorf("unknown dimension kind %q", kind)
	}
	if err != nil {
		return domain.DimensionValue{}, notFound(err, "get "+string(kind))
	}
	return v, nil
}

// InsertDimension menyimpan master dimensi baru.
func (s *Store) InsertDimension(ctx context.Context, v domain.DimensionValue, actorID uuid.UUID) (domain.DimensionValue, error) {
	q := s.q(ctx)
	var (
		out domain.DimensionValue
		err error
	)
	switch v.Kind {
	case domain.KindChannel:
		var r sqlcgen.Channel
		r, err = q.InsertChannel(ctx, sqlcgen.InsertChannelParams{Code: v.Code, Name: v.Name, IsActive: v.IsActive, ActorID: actorID})
		out = fromChannel(r)
	case domain.KindSubChannel:
		var r sqlcgen.SubChannel
		r, err = q.InsertSubChannel(ctx, sqlcgen.InsertSubChannelParams{
			ChannelID: *v.ChannelID, Code: v.Code, Name: v.Name, PartyType: v.PartyType, PartyID: nullUUID(v.PartyID),
			IsActive: v.IsActive, ActorID: actorID,
		})
		if pe := pgError(err); pe != nil && pe.Code == "23503" {
			return domain.DimensionValue{}, apperr.Validation(map[string]string{"channelId": "Channel tidak ditemukan"})
		}
		out = fromSubChannel(r)
	case domain.KindDivision:
		var r sqlcgen.Division
		r, err = q.InsertDivision(ctx, sqlcgen.InsertDivisionParams{Code: v.Code, Name: v.Name, IsActive: v.IsActive, ActorID: actorID})
		out = fromDivision(r)
	case domain.KindCampaign:
		var r sqlcgen.Campaign
		r, err = q.InsertCampaign(ctx, sqlcgen.InsertCampaignParams{
			Code: v.Code, Name: v.Name, ValidFrom: v.ValidFrom, ValidTo: v.ValidTo, BudgetAmount: nullAmount(v.Budget),
			Description: v.Description, IsActive: v.IsActive, ActorID: actorID,
		})
		out = fromCampaign(r)
	default:
		return out, fmt.Errorf("unknown dimension kind %q", v.Kind)
	}
	if err != nil {
		return domain.DimensionValue{}, mapWriteError(err, "insert "+string(v.Kind))
	}
	return out, nil
}

// UpdateDimension menyimpan perubahan master dimensi dengan optimistic locking.
func (s *Store) UpdateDimension(ctx context.Context, v domain.DimensionValue, version int, actorID uuid.UUID) (domain.DimensionValue, error) {
	q := s.q(ctx)
	ver := int32(version) //nolint:gosec // versi kecil
	var (
		out domain.DimensionValue
		err error
	)
	switch v.Kind {
	case domain.KindChannel:
		var r sqlcgen.Channel
		r, err = q.UpdateChannel(ctx, sqlcgen.UpdateChannelParams{Name: v.Name, IsActive: v.IsActive, ActorID: actorID, ID: v.ID, Version: ver})
		out = fromChannel(r)
	case domain.KindSubChannel:
		var r sqlcgen.SubChannel
		r, err = q.UpdateSubChannel(ctx, sqlcgen.UpdateSubChannelParams{
			Name: v.Name, PartyType: v.PartyType, PartyID: nullUUID(v.PartyID), IsActive: v.IsActive, ActorID: actorID, ID: v.ID, Version: ver,
		})
		out = fromSubChannel(r)
	case domain.KindDivision:
		var r sqlcgen.Division
		r, err = q.UpdateDivision(ctx, sqlcgen.UpdateDivisionParams{Name: v.Name, IsActive: v.IsActive, ActorID: actorID, ID: v.ID, Version: ver})
		out = fromDivision(r)
	case domain.KindCampaign:
		var r sqlcgen.Campaign
		r, err = q.UpdateCampaign(ctx, sqlcgen.UpdateCampaignParams{
			Name: v.Name, ValidFrom: v.ValidFrom, ValidTo: v.ValidTo, BudgetAmount: nullAmount(v.Budget),
			Description: v.Description, IsActive: v.IsActive, ActorID: actorID, ID: v.ID, Version: ver,
		})
		out = fromCampaign(r)
	default:
		return out, fmt.Errorf("unknown dimension kind %q", v.Kind)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DimensionValue{}, apperr.New(apperr.CodeVersionConflict)
	}
	if err != nil {
		return domain.DimensionValue{}, mapWriteError(err, "update "+string(v.Kind))
	}
	return out, nil
}

// DeleteDimension melakukan soft delete master dimensi.
func (s *Store) DeleteDimension(ctx context.Context, kind domain.DimensionKind, id uuid.UUID, version int, actorID uuid.UUID) error {
	q := s.q(ctx)
	ver := int32(version) //nolint:gosec // versi kecil
	var (
		n   int64
		err error
	)
	switch kind {
	case domain.KindChannel:
		n, err = q.SoftDeleteChannel(ctx, sqlcgen.SoftDeleteChannelParams{ActorID: actorID, ID: id, Version: ver})
	case domain.KindSubChannel:
		n, err = q.SoftDeleteSubChannel(ctx, sqlcgen.SoftDeleteSubChannelParams{ActorID: actorID, ID: id, Version: ver})
	case domain.KindDivision:
		n, err = q.SoftDeleteDivision(ctx, sqlcgen.SoftDeleteDivisionParams{ActorID: actorID, ID: id, Version: ver})
	case domain.KindCampaign:
		n, err = q.SoftDeleteCampaign(ctx, sqlcgen.SoftDeleteCampaignParams{ActorID: actorID, ID: id, Version: ver})
	default:
		return fmt.Errorf("unknown dimension kind %q", kind)
	}
	if err != nil {
		return fmt.Errorf("delete %s: %w", kind, err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// DimensionUsage memeriksa apakah master sudah dipakai jurnal dan jumlah sub-channel aktif (channel).
func (s *Store) DimensionUsage(ctx context.Context, kind domain.DimensionKind, id uuid.UUID) (bool, int64, error) {
	q := s.q(ctx)
	var (
		used bool
		err  error
	)
	switch kind {
	case domain.KindChannel:
		r, e := q.ChannelUsage(ctx, id)
		if e != nil {
			return false, 0, fmt.Errorf("channel usage: %w", e)
		}
		return r.Used, r.Children, nil
	case domain.KindSubChannel:
		used, err = q.SubChannelUsed(ctx, id)
	case domain.KindDivision:
		used, err = q.DivisionUsed(ctx, id)
	case domain.KindCampaign:
		used, err = q.CampaignUsed(ctx, id)
	default:
		return false, 0, fmt.Errorf("unknown dimension kind %q", kind)
	}
	if err != nil {
		return false, 0, fmt.Errorf("%s usage: %w", kind, err)
	}
	return used, 0, nil
}

// CheckDimensions memeriksa dimensi di DB pada tanggal transaksi.
func (s *Store) CheckDimensions(ctx context.Context, d domain.Dimensions, at clock.BusinessDate) (domain.DimensionCheck, error) {
	r, err := s.q(ctx).CheckDimensions(ctx, sqlcgen.CheckDimensionsParams{
		ChannelID: nullUUID(d.ChannelID), SubChannelID: nullUUID(d.SubChannelID), DivisionID: nullUUID(d.DivisionID),
		CampaignID: nullUUID(d.CampaignID), BusinessDate: at,
	})
	if err != nil {
		return domain.DimensionCheck{}, fmt.Errorf("check dimensions: %w", err)
	}
	return domain.DimensionCheck{ChannelOK: r.ChannelOk, SubChannelOK: r.SubChannelOk, DivisionOK: r.DivisionOk, CampaignOK: r.CampaignOk}, nil
}

// DimensionName adalah kode dan nama dimensi untuk tampilan.
type DimensionName struct {
	Kind domain.DimensionKind
	Code string
	Name string
}

// DimensionNames membaca kode & nama dimensi (termasuk yang sudah dihapus).
func (s *Store) DimensionNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]DimensionName, error) {
	out := map[uuid.UUID]DimensionName{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q(ctx).DimensionNames(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("dimension names: %w", err)
	}
	for _, r := range rows {
		out[r.ID] = DimensionName{Kind: domain.DimensionKind(r.Kind), Code: r.Code, Name: r.Name}
	}
	return out, nil
}

// --- Periode ----------------------------------------------------------------------------------

func toPeriod(r sqlcgen.FiscalPeriod) domain.Period {
	return domain.Period{
		ID: r.ID, Year: int(r.Year), Month: time.Month(r.Month), Status: domain.PeriodStatus(r.Status), ClosedAt: r.ClosedAt,
		ClosedBy: uuidPtr(r.ClosedBy), ReopenedCount: int(r.ReopenedCount), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		Version: int(r.Version),
	}
}

// ListPeriods mengembalikan periode terbaru lebih dulu; year 0 = semua tahun.
func (s *Store) ListPeriods(ctx context.Context, year int) ([]domain.Period, error) {
	rows, err := s.q(ctx).ListPeriods(ctx, pgtype.Int4{Int32: int32(year), Valid: year != 0}) //nolint:gosec // tahun 4 digit
	if err != nil {
		return nil, fmt.Errorf("list fiscal periods: %w", err)
	}
	out := make([]domain.Period, len(rows))
	for i, r := range rows {
		out[i] = toPeriod(r)
	}
	return out, nil
}

// EnsurePeriod membuat periode (status terbuka) jika belum ada.
func (s *Store) EnsurePeriod(ctx context.Context, k domain.PeriodKey, actorID uuid.UUID) error {
	err := s.q(ctx).EnsurePeriod(ctx, sqlcgen.EnsurePeriodParams{Year: int32(k.Year), Month: int32(k.Month), ActorID: actorID}) //nolint:gosec // tahun & bulan kecil
	if err != nil {
		return fmt.Errorf("ensure fiscal period: %w", err)
	}
	return nil
}

// GetPeriod membaca periode; shared mengunci FOR SHARE sampai transaksi selesai.
func (s *Store) GetPeriod(ctx context.Context, k domain.PeriodKey, shared bool) (domain.Period, error) {
	y, m := int32(k.Year), int32(k.Month) //nolint:gosec // tahun & bulan kecil
	var (
		r   sqlcgen.FiscalPeriod
		err error
	)
	if shared {
		r, err = s.q(ctx).GetPeriodShared(ctx, sqlcgen.GetPeriodSharedParams{Year: y, Month: m})
	} else {
		r, err = s.q(ctx).GetPeriod(ctx, sqlcgen.GetPeriodParams{Year: y, Month: m})
	}
	if err != nil {
		return domain.Period{}, notFound(err, "get fiscal period")
	}
	return toPeriod(r), nil
}

// ClosedPeriodAfter memeriksa apakah ada periode tertutup setelah k.
func (s *Store) ClosedPeriodAfter(ctx context.Context, k domain.PeriodKey) (bool, error) {
	ok, err := s.q(ctx).ClosedPeriodAfterExists(ctx, int32(k.Index())) //nolint:gosec // indeks bulan kecil
	if err != nil {
		return false, fmt.Errorf("closed period after: %w", err)
	}
	return ok, nil
}

// --- Posting rule -----------------------------------------------------------------------------

func toRule(r sqlcgen.PostingRule) domain.PostingRule {
	dims := make([]domain.Dimension, len(r.RequiredDimensions))
	for i, d := range r.RequiredDimensions {
		dims[i] = domain.Dimension(d)
	}
	return domain.PostingRule{Code: r.Code, Name: r.Name, Description: r.Description, RequiredDimensions: dims, Release: r.Release, IsActive: r.IsActive}
}

func toRuleAccount(r sqlcgen.ListRuleAccountsRow) domain.RuleAccount {
	return domain.RuleAccount{AmountKey: r.AmountKey, Side: domain.Side(r.Side), Resolver: domain.Resolver(r.AccountResolver), AccountCode: r.AccountCode}
}

// GetPostingRule membaca rule beserta pemetaan akun yang berlaku pada tanggal posting.
func (s *Store) GetPostingRule(ctx context.Context, code string, at clock.BusinessDate) (domain.PostingRule, error) {
	r, err := s.q(ctx).GetPostingRule(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PostingRule{}, apperr.Validation(map[string]string{"postingRule": fmt.Sprintf("Posting rule %q tidak dikenal", code)})
	}
	if err != nil {
		return domain.PostingRule{}, fmt.Errorf("get posting rule: %w", err)
	}
	rule := toRule(r)
	rows, err := s.q(ctx).ListRuleAccounts(ctx, sqlcgen.ListRuleAccountsParams{RuleCode: text(code), At: at})
	if err != nil {
		return domain.PostingRule{}, fmt.Errorf("list posting rule accounts: %w", err)
	}
	for _, ra := range rows {
		rule.Accounts = append(rule.Accounts, toRuleAccount(ra))
	}
	return rule, nil
}

// RuleAccountView adalah pemetaan akun beserta nama akun untuk halaman daftar rule.
type RuleAccountView struct {
	domain.RuleAccount
	AccountName string
}

// ListPostingRules mengembalikan semua rule beserta pemetaan yang berlaku pada tanggal at.
func (s *Store) ListPostingRules(ctx context.Context, at clock.BusinessDate) ([]domain.PostingRule, map[string][]RuleAccountView, error) {
	rules, err := s.q(ctx).ListPostingRules(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list posting rules: %w", err)
	}
	rows, err := s.q(ctx).ListRuleAccounts(ctx, sqlcgen.ListRuleAccountsParams{At: at})
	if err != nil {
		return nil, nil, fmt.Errorf("list posting rule accounts: %w", err)
	}
	views := map[string][]RuleAccountView{}
	for _, r := range rows {
		views[r.PostingRuleCode] = append(views[r.PostingRuleCode], RuleAccountView{RuleAccount: toRuleAccount(r), AccountName: r.AccountName})
	}
	out := make([]domain.PostingRule, len(rules))
	for i, r := range rules {
		out[i] = toRule(r)
		for _, v := range views[r.Code] {
			out[i].Accounts = append(out[i].Accounts, v.RuleAccount)
		}
	}
	return out, views, nil
}
