package service

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/tax"
)

// WithholdingFilter adalah filter rekap bukti potong.
type WithholdingFilter = postgres.WithholdingFilter

// WithholdingMonth adalah ringkasan bukti potong per jenis per masa.
type WithholdingMonth = postgres.WithholdingMonth

// --- Tarif effective-dated --------------------------------------------------------------------

// ListTaxRates mengembalikan semua versi tarif PPh (taxType kosong = semua jenis).
func (s *Service) ListTaxRates(ctx context.Context, taxType string) ([]domain.RateVersion, error) {
	if _, err := actor.RequireAny(ctx, PermWithholdingRead, PermFinalTaxRead, PermExpenseClaimPost); err != nil {
		return nil, err
	}
	return s.Finance.ListRates(ctx, taxType)
}

// CreateTaxRate menambah versi tarif baru. Versi terakhir objek pajak yang sama ditutup sehari sebelum versi
// baru berlaku (data tidak di-update, Technical Guideline 5.14).
func (s *Service) CreateTaxRate(ctx context.Context, r tax.Rate, isDefault bool, note string) (domain.RateVersion, error) {
	perm := PermWithholdingWrite
	if r.Type == tax.FinalUMKM {
		perm = PermFinalTaxWrite
	}
	a, err := actor.Require(ctx, perm)
	if err != nil {
		return domain.RateVersion{}, err
	}
	v, err := domain.NewRateVersion(r, isDefault, note)
	if err != nil {
		return domain.RateVersion{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		prev, ok, err := s.Finance.LatestRate(ctx, v.Type, v.ObjectCode)
		if err != nil {
			return err
		}
		if ok && (prev.ValidTo == nil || !prev.ValidTo.Before(v.ValidFrom)) {
			end, err := prev.SupersededBy(v.ValidFrom)
			if err != nil {
				return err
			}
			if err := s.Finance.EndRate(ctx, prev.ID, end, a.UserID); err != nil {
				return err
			}
		}
		if v.ID, err = s.Finance.InsertRate(ctx, v, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityTaxRate, EntityID: v.ID, EntityLabel: v.Type.Label() + " " + v.ObjectCode,
			After: map[string]any{
				"name": v.Name, "ratePercent": v.RatePercent.String(), "basePercent": v.BasePercent.String(),
				"brackets": len(v.Brackets), "validFrom": v.ValidFrom.String(), "isDefault": v.IsDefault,
			},
		})
	})
	return v, err
}

// --- PPh potong untuk modul lain (tagihan pemasok) --------------------------------------------

// WithholdingRequest adalah permintaan pemotongan PPh satu dokumen sumber (tagihan pemasok, pengajuan biaya).
type WithholdingRequest struct {
	Subject domain.WithholdingSubject
	Type    domain.WithholdingType
	// ObjectCode kosong = objek default jenis PPh yang berlaku pada tanggal dokumen.
	ObjectCode string
	Gross      money.Amount
}

// WithholdingResult adalah hasil pemotongan: data bukti potong dan akun utang PPh untuk jurnal dokumen sumber
// (mis. A-18 `withholding_value` → 2302).
type WithholdingResult struct {
	Record      domain.WithholdingRecord
	AccountCode string
}

// QuoteWithholding menghitung PPh tanpa menyimpan (pratinjau di form). Jenis none menghasilkan nol.
func (s *Service) QuoteWithholding(ctx context.Context, req WithholdingRequest) (WithholdingResult, error) {
	if req.Type == "" || req.Type == domain.WithholdingNone {
		return WithholdingResult{Record: domain.WithholdingRecord{Gross: req.Gross, TaxAmount: money.Zero, TaxableBase: money.Zero}}, nil
	}
	rates, err := s.Finance.ListRates(ctx, string(domain.TaxTypeOf(req.Type)))
	if err != nil {
		return WithholdingResult{}, err
	}
	rec, err := s.calculateWithholding(rates, req.Subject, domain.TaxTypeOf(req.Type), req.ObjectCode, req.Gross)
	if err != nil {
		return WithholdingResult{}, err
	}
	return WithholdingResult{Record: rec, AccountCode: domain.WithholdingAccountCode(req.Type)}, nil
}

// Withhold menghitung dan mencatat data bukti potong di transaksi aktif tanpa cek permission (pemanggil
// memeriksa aksi user-nya). Jurnal utang PPh tetap diposting dokumen sumber lewat rule-nya.
func (s *Service) Withhold(ctx context.Context, req WithholdingRequest) (WithholdingResult, error) {
	res, err := s.QuoteWithholding(ctx, req)
	if err != nil || res.Record.TaxAmount.IsZero() {
		return res, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		id, err := s.Finance.InsertWithholding(ctx, res.Record, actor.UserID(ctx))
		res.Record.ID = id
		return err
	})
	return res, err
}

// CancelWithholdings membatalkan data bukti potong dokumen sumber yang dibatalkan. Ditolak bila sudah disetor.
func (s *Service) CancelWithholdings(ctx context.Context, sourceType string, sourceID uuid.UUID) error {
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		list, err := s.Finance.WithholdingsBySource(ctx, sourceType, sourceID)
		if err != nil {
			return err
		}
		for _, w := range list {
			if w.Status == domain.WithholdingCancelled {
				continue
			}
			locked, err := s.Finance.GetWithholdingForUpdate(ctx, w.ID)
			if err != nil {
				return err
			}
			if err := locked.Cancel(); err != nil {
				return err
			}
			if err := s.Finance.SaveWithholding(ctx, locked, locked.Version, actor.UserID(ctx)); err != nil {
				return err
			}
		}
		return nil
	})
}

// --- Rekap bukti potong & Coretax -------------------------------------------------------------

// ListWithholdings mengembalikan bukti potong satu masa.
func (s *Service) ListWithholdings(ctx context.Context, f WithholdingFilter, page paging.Page) (paging.Result[domain.WithholdingRecord], error) {
	if _, err := actor.Require(ctx, PermWithholdingRead); err != nil {
		return paging.Result[domain.WithholdingRecord]{}, err
	}
	if err := f.Period.Validate(); err != nil {
		return paging.Result[domain.WithholdingRecord]{}, err
	}
	return s.Finance.ListWithholdings(ctx, f, page)
}

// WithholdingSummary meringkas bukti potong per masa setahun (utang pajak terbuka dan sudah disetor).
func (s *Service) WithholdingSummary(ctx context.Context, year int) ([]WithholdingMonth, error) {
	if _, err := actor.Require(ctx, PermWithholdingRead); err != nil {
		return nil, err
	}
	return s.Finance.WithholdingSummary(ctx, year)
}

// SetSlipNumber mencatat nomor bukti potong dari Coretax.
func (s *Service) SetSlipNumber(ctx context.Context, id uuid.UUID, version int, number string) (domain.WithholdingRecord, error) {
	a, err := actor.Require(ctx, PermWithholdingWrite)
	if err != nil {
		return domain.WithholdingRecord{}, err
	}
	var w domain.WithholdingRecord
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if w, err = s.Finance.GetWithholdingForUpdate(ctx, id); err != nil {
			return err
		}
		if w.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := w.SlipNumber
		if err := w.SetSlipNumber(number); err != nil {
			return err
		}
		if err := s.Finance.SaveWithholding(ctx, w, version, a.UserID); err != nil {
			return err
		}
		w.Version++
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: EntityWithholding, EntityID: w.ID, EntityLabel: w.SourceLabel,
			Before: map[string]any{"slipNumber": before}, After: map[string]any{"slipNumber": w.SlipNumber},
		})
	})
	return w, err
}

// RecapColumns adalah kolom ekspor rekap bukti potong untuk diinput ke Coretax.
var RecapColumns = []string{
	"Masa Pajak", "Tahun Pajak", "Jenis PPh", "Kode Objek Pajak", "Nama Objek Pajak", "NPWP/NIK Penerima",
	"Nama Penerima", "Dokumen Referensi", "Tanggal Dokumen", "Penghasilan Bruto", "Dasar Pengenaan Pajak",
	"Tarif (%)", "PPh Dipotong", "Fasilitas", "Nomor Bukti Potong", "Status",
}

// ExportWithholdingRecap menulis rekap bukti potong satu masa ke Excel (sheet per jenis PPh) dan mengembalikan
// tautan unduhan bertanda tangan.
func (s *Service) ExportWithholdingRecap(ctx context.Context, p tax.Period, taxType string) (DownloadLink, error) {
	if _, err := actor.Require(ctx, PermWithholdingExport); err != nil {
		return DownloadLink{}, err
	}
	if err := p.Validate(); err != nil {
		return DownloadLink{}, err
	}
	res, err := s.Finance.ListWithholdings(ctx, WithholdingFilter{Period: p, TaxType: taxType}, paging.Page{Number: 1, Size: 100000})
	if err != nil {
		return DownloadLink{}, err
	}
	data, err := recapWorkbook(p, res.Items)
	if err != nil {
		return DownloadLink{}, err
	}
	name := fmt.Sprintf("rekap-bukti-potong-%s.xlsx", p.String())
	if taxType != "" {
		name = fmt.Sprintf("rekap-bukti-potong-%s-%s.xlsx", strings.ReplaceAll(taxType, "_", ""), p.String())
	}
	exportID, err := uuid.NewV7()
	if err != nil {
		return DownloadLink{}, fmt.Errorf("export id: %w", err)
	}
	var link DownloadLink
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if link, err = s.Files.SaveExport(ctx, FileOwnerWithholdingSlip, exportID, name, ContentTypeXLSX, data); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionExport, EntityType: EntityWithholding, EntityID: exportID, EntityLabel: name,
			After: map[string]any{"period": p.String(), "taxType": taxType, "rows": len(res.Items)},
		})
	})
	return link, err
}

func recapWorkbook(p tax.Period, items []domain.WithholdingRecord) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"#E2E8F0"}, Pattern: 1}})
	if err != nil {
		return nil, fmt.Errorf("style: %w", err)
	}
	sheets := map[tax.Type]string{tax.PPh21: "PPh 21", tax.PPh23: "PPh 23", tax.PPh42: "PPh 4(2)"}
	rowsBy := map[tax.Type]int{}
	first := true
	for _, t := range []tax.Type{tax.PPh21, tax.PPh23, tax.PPh42} {
		name := sheets[t]
		if first {
			if err := f.SetSheetName("Sheet1", name); err != nil {
				return nil, fmt.Errorf("sheet: %w", err)
			}
			first = false
		} else if _, err := f.NewSheet(name); err != nil {
			return nil, fmt.Errorf("sheet: %w", err)
		}
		for i, h := range RecapColumns {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			if err := f.SetCellValue(name, cell, h); err != nil {
				return nil, fmt.Errorf("header: %w", err)
			}
		}
		last, _ := excelize.CoordinatesToCellName(len(RecapColumns), 1)
		if err := f.SetCellStyle(name, "A1", last, bold); err != nil {
			return nil, fmt.Errorf("header style: %w", err)
		}
		_ = f.SetColWidth(name, "A", "P", 18)
		rowsBy[t] = 1
	}
	statusLabel := map[domain.WithholdingStatus]string{
		domain.WithholdingRecorded: "Belum disetor", domain.WithholdingDeposited: "Sudah disetor", domain.WithholdingCancelled: "Dibatalkan",
	}
	for _, w := range items {
		name, ok := sheets[w.TaxType]
		if !ok {
			continue
		}
		rowsBy[w.TaxType]++
		row := rowsBy[w.TaxType]
		facility := ""
		if !w.HasTaxID {
			facility = "Tanpa NPWP/NIK (tarif lebih tinggi)"
		}
		values := []any{
			int(p.Month), p.Year, w.TaxType.Label(), w.ObjectCode, w.ObjectName, w.TaxpayerTaxID, w.TaxpayerName,
			w.SourceLabel, w.BusinessDate.String(), w.Gross.Decimal().InexactFloat64(), w.TaxableBase.Decimal().InexactFloat64(),
			w.RatePercent.InexactFloat64(), w.TaxAmount.Decimal().InexactFloat64(), facility, w.SlipNumber, statusLabel[w.Status],
		}
		for i, v := range values {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			if i == 5 {
				// NPWP/NIK disimpan sebagai teks agar angka 0 di depan tidak hilang.
				if err := f.SetCellStr(name, cell, w.TaxpayerTaxID); err != nil {
					return nil, fmt.Errorf("cell: %w", err)
				}
				continue
			}
			if err := f.SetCellValue(name, cell, v); err != nil {
				return nil, fmt.Errorf("cell: %w", err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("write xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

// --- Penyetoran (A-29) ------------------------------------------------------------------------

// ListTaxDeposits mengembalikan penyetoran pajak setahun.
func (s *Service) ListTaxDeposits(ctx context.Context, year int) ([]domain.TaxDeposit, error) {
	if _, err := actor.RequireAny(ctx, PermWithholdingRead, PermFinalTaxRead); err != nil {
		return nil, err
	}
	return s.Finance.ListDeposits(ctx, year)
}

// CreateTaxDeposit menyetor pajak satu jenis satu masa: nominal = PPh dipotong yang belum disetor (atau PPh Final
// masa itu), jurnal A-29 (Dr utang PPh / Cr bank) dengan nomor BYR. Dana keluar → re-autentikasi (★).
func (s *Service) CreateTaxDeposit(ctx context.Context, in domain.TaxDepositInput) (domain.TaxDeposit, error) {
	perm := PermWithholdingPost
	if in.TaxType == tax.FinalUMKM {
		perm = PermFinalTaxPost
	}
	a, err := actor.RequireReauth(ctx, s.Clock.Now(), perm)
	if err != nil {
		return domain.TaxDeposit{}, err
	}
	if err := in.Validate(); err != nil {
		return domain.TaxDeposit{}, err
	}
	var d domain.TaxDeposit
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		b, err := s.usableBank(ctx, in.BankAccountID, "bankAccountId")
		if err != nil {
			return err
		}
		if exists, err := s.Finance.DepositExists(ctx, in.TaxType, in.Period); err != nil {
			return err
		} else if exists {
			return apperr.WithMessage(apperr.CodeInvalidStateTransition, in.TaxType.Label()+" masa "+in.Period.String()+" sudah disetor.")
		}
		var (
			ids    []uuid.UUID
			amount money.Amount
			runID  uuid.UUID
		)
		if in.TaxType == tax.FinalUMKM {
			id, total, ok, deposited, err := s.Finance.FinalTaxRunForDeposit(ctx, in.Period)
			if err != nil {
				return err
			}
			if !ok || deposited {
				return apperr.Validation(map[string]string{"period": "PPh Final masa " + in.Period.String() + " belum dihitung"})
			}
			runID, amount = id, total
		} else if ids, amount, err = s.Finance.OpenWithholdings(ctx, in.TaxType, in.Period); err != nil {
			return err
		}
		if !amount.IsPositive() {
			return apperr.Validation(map[string]string{"period": "Tidak ada " + in.TaxType.Label() + " yang belum disetor pada masa " + in.Period.String()})
		}
		number, err := s.DocNumbers.NextDocNumber(ctx, DocPayment, in.DepositDate)
		if err != nil {
			return err
		}
		if d.ID, err = uuid.NewV7(); err != nil {
			return fmt.Errorf("deposit id: %w", err)
		}
		entry, err := s.Post(ctx, domain.PostingRequest{
			RuleCode: domain.RuleTaxDeposit, SourceType: domain.SourceTaxDeposit, SourceID: d.ID, SourceLabel: number,
			BusinessDate: in.DepositDate, Amounts: map[string]money.Amount{domain.AmountTax: amount},
			Accounts: map[domain.Resolver]string{
				domain.ResolverWithholding: domain.TaxPayableCode(in.TaxType), domain.ResolverBank: b.AccountCode,
			},
			Description: fmt.Sprintf("Setor %s masa %s %s", in.TaxType.Label(), in.Period.String(), strings.TrimSpace(in.NTPN)),
		})
		if err != nil {
			return err
		}
		d.Number, d.TaxType, d.Period, d.BankAccountID, d.BankAccountName = number, in.TaxType, in.Period, b.ID, b.Name
		d.DepositDate, d.Amount, d.BillingCode, d.NTPN = in.DepositDate, amount, strings.TrimSpace(in.BillingCode), strings.TrimSpace(in.NTPN)
		d.JournalEntryID, d.JournalNumber, d.CreatedBy = entry.ID, entry.Number, a.UserID
		if _, d.CreatedAt, err = s.Finance.InsertDeposit(ctx, d, a.UserID); err != nil {
			return err
		}
		if in.TaxType == tax.FinalUMKM {
			if err := s.Finance.MarkFinalTaxRunDeposited(ctx, runID, d.ID); err != nil {
				return err
			}
		} else if err := s.Finance.MarkDeposited(ctx, ids, d.ID, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionPost, EntityType: EntityTaxDeposit, EntityID: d.ID, EntityLabel: number,
			After: map[string]any{
				"taxType": string(d.TaxType), "period": d.Period.String(), "amount": d.Amount.String(), "bankAccountCode": b.Code,
				"ntpn": d.NTPN, "billingCode": d.BillingCode, "slips": len(ids),
			},
		})
	})
	return d, err
}

// --- PPh Final UMKM (S-407) -------------------------------------------------------------------

// FinalTaxSummary adalah pemantauan PPh Final UMKM setahun.
type FinalTaxSummary struct {
	Year      int
	Setting   *domain.FinalTaxSetting
	Settings  []domain.FinalTaxSetting
	Rate      *domain.RateVersion
	Opening   money.Amount
	Months    []domain.FinalTaxMonth
	Threshold tax.Threshold
}

// effectiveSetting memilih status PPh Final yang berlaku pada tanggal (valid_from terakhir ≤ tanggal).
func effectiveSetting(list []domain.FinalTaxSetting, on clock.BusinessDate) *domain.FinalTaxSetting {
	for i := range list {
		if !list[i].ValidFrom.After(on) {
			return &list[i]
		}
	}
	return nil
}

// FinalTaxSummary menghitung omzet bruto bulanan (kredit − debit akun pendapatan 4xxx), PPh Final yang sudah
// dihitung, dan posisi omzet kumulatif terhadap batas Rp4,8 M.
func (s *Service) FinalTaxSummary(ctx context.Context, year int) (FinalTaxSummary, error) {
	if _, err := actor.Require(ctx, PermFinalTaxRead); err != nil {
		return FinalTaxSummary{}, err
	}
	if year < 2000 || year > 9999 {
		return FinalTaxSummary{}, apperr.Validation(map[string]string{"year": "Tahun tidak valid"})
	}
	settings, err := s.Finance.ListFinalTaxSettings(ctx)
	if err != nil {
		return FinalTaxSummary{}, err
	}
	revenue, err := s.Finance.MonthlyRevenue(ctx, year)
	if err != nil {
		return FinalTaxSummary{}, err
	}
	runs, err := s.Finance.ListFinalTaxRuns(ctx, year)
	if err != nil {
		return FinalTaxSummary{}, err
	}
	rates, err := s.Finance.ListRates(ctx, string(tax.FinalUMKM))
	if err != nil {
		return FinalTaxSummary{}, err
	}
	out := FinalTaxSummary{Year: year, Settings: settings, Opening: money.Zero}
	ref := s.today()
	if ref.Year() != year {
		ref = clock.NewBusinessDate(year, time.December, 31)
	}
	out.Setting = effectiveSetting(settings, ref)
	for _, st := range settings {
		out.Opening = out.Opening.Add(st.OpeningFor(year))
		if !out.Opening.IsZero() {
			break
		}
	}
	if code, ok := domain.DefaultObject(rates, tax.FinalUMKM, ref); ok {
		if r, err := domain.FindRate(rates, tax.FinalUMKM, code, ref); err == nil {
			out.Rate = &r
		}
	}
	cumulative := out.Opening
	for m := time.January; m <= time.December; m++ {
		gross := revenue[m]
		cumulative = cumulative.Add(gross)
		month := domain.FinalTaxMonth{Period: tax.Period{Year: year, Month: m}, GrossRevenue: gross, Cumulative: cumulative}
		for i := range runs {
			if runs[i].Period.Month == m {
				month.Run = &runs[i]
			}
		}
		out.Months = append(out.Months, month)
	}
	out.Threshold = tax.CheckThreshold(cumulative, tax.UMKMAnnualThreshold, tax.UMKMWarningPercent)
	return out, nil
}

// SetFinalTaxSetting mengaktifkan/menonaktifkan PPh Final UMKM mulai tanggal tertentu (riwayat tersimpan).
func (s *Service) SetFinalTaxSetting(ctx context.Context, st domain.FinalTaxSetting) (domain.FinalTaxSetting, error) {
	a, err := actor.Require(ctx, PermFinalTaxWrite)
	if err != nil {
		return domain.FinalTaxSetting{}, err
	}
	st.Note = strings.TrimSpace(st.Note)
	if err := st.Validate(); err != nil {
		return domain.FinalTaxSetting{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if st.ID, err = s.Finance.InsertFinalTaxSetting(ctx, st, a.UserID); err != nil {
			return err
		}
		st.CreatedBy = a.UserID
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityFinalTax, EntityID: st.ID, EntityLabel: "PPh Final UMKM " + st.ValidFrom.String(),
			After: map[string]any{
				"isEnabled": st.IsEnabled, "validFrom": st.ValidFrom.String(), "openingTurnover": st.OpeningTurnover.String(),
				"openingYear": st.OpeningYear, "note": st.Note,
			},
		})
	})
	return st, err
}

// RunFinalTax menghitung PPh Final UMKM satu masa yang sudah berakhir: omzet bruto × tarif berlaku pada akhir
// masa (kalkulator tax), jurnal A-30 (Dr 7203 / Cr 2304) bertanggal akhir masa. Satu kali per masa.
func (s *Service) RunFinalTax(ctx context.Context, p tax.Period) (domain.FinalTaxRun, error) {
	a, err := actor.Require(ctx, PermFinalTaxPost)
	if err != nil {
		return domain.FinalTaxRun{}, err
	}
	var run domain.FinalTaxRun
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		settings, err := s.Finance.ListFinalTaxSettings(ctx)
		if err != nil {
			return err
		}
		end := p.LastDay()
		if err := domain.CheckFinalTaxRun(effectiveSetting(settings, end), p, s.today()); err != nil {
			return err
		}
		rates, err := s.Finance.ListRates(ctx, string(tax.FinalUMKM))
		if err != nil {
			return err
		}
		code, ok := domain.DefaultObject(rates, tax.FinalUMKM, end)
		if !ok {
			return apperr.Validation(map[string]string{"period": "Tarif PPh Final UMKM belum ada untuk masa " + p.String()})
		}
		rate, err := domain.FindRate(rates, tax.FinalUMKM, code, end)
		if err != nil {
			return err
		}
		revenue, err := s.Finance.MonthlyRevenue(ctx, p.Year)
		if err != nil {
			return err
		}
		gross := revenue[p.Month]
		amount, err := tax.FinalTax(gross, rate.Rate)
		if err != nil {
			return err
		}
		if run.ID, err = uuid.NewV7(); err != nil {
			return fmt.Errorf("final tax run id: %w", err)
		}
		run.Period, run.GrossRevenue, run.RateID, run.RatePercent, run.TaxAmount = p, gross, rate.ID, rate.RatePercent, amount
		if amount.IsPositive() {
			entry, err := s.Post(ctx, domain.PostingRequest{
				RuleCode: domain.RuleFinalTax, SourceType: domain.SourceFinalTaxRun, SourceID: run.ID,
				SourceLabel: "PPh Final " + p.String(), BusinessDate: end,
				Amounts:     map[string]money.Amount{domain.AmountTax: amount},
				Description: fmt.Sprintf("PPh Final UMKM masa %s: %s%% × omzet %s", p.String(), rate.RatePercent.String(), gross.Rupiah()),
			})
			if err != nil {
				return err
			}
			run.JournalEntryID, run.JournalNumber = &entry.ID, entry.Number
		}
		if _, run.CreatedAt, err = s.Finance.InsertFinalTaxRun(ctx, run, a.UserID); err != nil {
			return err
		}
		run.CreatedBy = a.UserID
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionPost, EntityType: EntityFinalTax, EntityID: run.ID, EntityLabel: "PPh Final " + p.String(),
			After: map[string]any{"grossRevenue": gross.String(), "ratePercent": rate.RatePercent.String(), "tax": amount.String()},
		})
	})
	return run, err
}
