package domain

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// Side adalah sisi jurnal.
type Side string

const (
	SideDebit  Side = "debit"
	SideCredit Side = "credit"
)

// Opposite mengembalikan sisi lawan.
func (s Side) Opposite() Side {
	if s == SideDebit {
		return SideCredit
	}
	return SideDebit
}

// Resolver menentukan sumber akun satu baris posting rule. `fixed` memakai akun di posting_rule_accounts;
// resolver lain memakai akun yang dikirim dokumen sumber, dengan akun di mapping sebagai default.
type Resolver string

const (
	ResolverFixed                   Resolver = "fixed"
	ResolverLocationInventory       Resolver = "location_inventory_account"
	ResolverMaterial                Resolver = "material_account"
	ResolverReceivable              Resolver = "receivable_account"
	ResolverPayable                 Resolver = "payable_account"
	ResolverBank                    Resolver = "bank_account"
	ResolverExpenseType             Resolver = "expense_type_account"
	ResolverWithholding             Resolver = "withholding_account"
	ResolverPayment                 Resolver = "payment_account"
	ResolverFixedAsset              Resolver = "fixed_asset_account"
	ResolverAccumulatedDepreciation Resolver = "accumulated_depreciation_account"
	ResolverCounter                 Resolver = "counter_account"
	ResolverOpeningBalance          Resolver = "opening_balance_account"
)

// RuleAccount adalah satu baris pemetaan posting rule: nilai bernama → sisi → akun.
type RuleAccount struct {
	AmountKey   string
	Side        Side
	Resolver    Resolver
	AccountCode string
}

// PostingRule adalah rule jurnal otomatis A-xx beserta pemetaan akun yang berlaku pada tanggal posting.
type PostingRule struct {
	Code               string
	Name               string
	Description        string
	RequiredDimensions []Dimension
	Release            string
	IsActive           bool
	Accounts           []RuleAccount
}

// PostingSplit memecah satu nilai bernama ke beberapa akun, mis. saldo awal (A-36) per akun neraca.
// AccountCode menggantikan akun resolver (non-fixed) pada key tersebut.
type PostingSplit struct {
	AmountKey   string
	AccountCode string
	Amount      money.Amount
	Memo        string
}

// PostingRequest adalah permintaan jurnal otomatis dari modul sumber (Technical Guideline 5.8.1). Modul sumber
// hanya mengirim nilai bernama, tidak pernah menyusun debit/kredit sendiri.
type PostingRequest struct {
	// RuleCode, mis. "A-01".
	RuleCode string
	// SourceType dan SourceID adalah dokumen sumber, mis. "goods_receipt" + ID GR. Satu dokumen sumber hanya
	// diposting sekali per rule (uq_journal_entries_source).
	SourceType string
	SourceID   uuid.UUID
	// SourceLabel adalah nomor dokumen sumber untuk ditampilkan di jurnal, mis. "GR/2027/01/00010".
	SourceLabel  string
	BusinessDate clock.BusinessDate
	// Amounts adalah nilai bernama sesuai amount_key rule. Nilai nol dilewati; nilai negatif membalik sisi
	// (mis. selisih stock count lebih). Maksimal 2 desimal.
	Amounts map[string]money.Amount
	// Accounts adalah kode akun per resolver, mis. {"location_inventory_account": "1312"}. Kosong = akun default.
	Accounts map[Resolver]string
	// Splits dipakai jika satu nilai bernama dibukukan ke beberapa akun.
	Splits     []PostingSplit
	Dimensions Dimensions
	// Party menautkan baris ke pihak sub-ledger (pelanggan/pemasok), opsional.
	Party       *Party
	Description string
}

// PlannedLine adalah baris jurnal hasil rule, sebelum akun diubah menjadi ID.
type PlannedLine struct {
	AmountKey   string
	AccountCode string
	Side        Side
	Amount      money.Amount
	Memo        string
}

func ruleError(code, msg string) error {
	return apperr.Validation(map[string]string{"postingRule": fmt.Sprintf("Rule %s: %s", code, msg)})
}

// Validate memeriksa kelengkapan permintaan yang tidak bergantung pada rule.
func (r PostingRequest) Validate() error {
	fields := map[string]string{}
	if strings.TrimSpace(r.RuleCode) == "" {
		fields["postingRule"] = "Kode posting rule wajib diisi"
	}
	if strings.TrimSpace(r.SourceType) == "" {
		fields["sourceType"] = "Jenis dokumen sumber wajib diisi"
	}
	if r.SourceID == uuid.Nil {
		fields["sourceId"] = "Dokumen sumber wajib diisi"
	}
	if r.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal bisnis wajib diisi"
	}
	if r.Party != nil && (strings.TrimSpace(r.Party.Type) == "" || r.Party.ID == uuid.Nil) {
		fields["party"] = "Jenis dan ID pihak wajib diisi bersamaan"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// Plan menyusun baris jurnal dari rule dan permintaan, lalu menolak jurnal yang tidak balance atau dimensi
// wajibnya kosong (Technical Guideline 5.8.1). Fungsi murni: akun diperiksa service setelahnya.
func (rule PostingRule) Plan(req PostingRequest) ([]PlannedLine, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if !rule.IsActive {
		return nil, ruleError(rule.Code, "rule tidak aktif")
	}
	if len(rule.Accounts) == 0 {
		return nil, ruleError(rule.Code, "rule ini tidak menghasilkan jurnal")
	}
	if missing := req.Dimensions.Missing(rule.RequiredDimensions, "dimensions.", "rule "+rule.Code); len(missing) > 0 {
		return nil, apperr.Validation(missing)
	}

	byKey := map[string][]RuleAccount{}
	var keys []string
	for _, ra := range rule.Accounts {
		if _, ok := byKey[ra.AmountKey]; !ok {
			keys = append(keys, ra.AmountKey)
		}
		byKey[ra.AmountKey] = append(byKey[ra.AmountKey], ra)
	}

	unknown := []string{}
	for k := range req.Amounts {
		if _, ok := byKey[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	splitKeys := map[string]bool{}
	for _, s := range req.Splits {
		if _, ok := byKey[s.AmountKey]; !ok {
			unknown = append(unknown, s.AmountKey)
		}
		if _, dup := req.Amounts[s.AmountKey]; dup {
			return nil, ruleError(rule.Code, fmt.Sprintf("nilai %q dikirim di Amounts dan Splits sekaligus", s.AmountKey))
		}
		splitKeys[s.AmountKey] = true
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, ruleError(rule.Code, "nilai tidak dikenal: "+strings.Join(slices.Compact(unknown), ", "))
	}
	for res := range req.Accounts {
		if !slices.ContainsFunc(rule.Accounts, func(ra RuleAccount) bool { return ra.Resolver == res && res != ResolverFixed }) {
			return nil, ruleError(rule.Code, fmt.Sprintf("akun %q tidak dipakai rule ini", res))
		}
	}

	var lines []PlannedLine
	add := func(ra RuleAccount, amount money.Amount, account, memo string) error {
		if !isWholeCents(amount) {
			return ruleError(rule.Code, fmt.Sprintf("nilai %q lebih dari 2 angka di belakang koma", ra.AmountKey))
		}
		if account == "" {
			return ruleError(rule.Code, fmt.Sprintf("akun %q untuk nilai %q wajib dikirim dokumen sumber", ra.Resolver, ra.AmountKey))
		}
		side := ra.Side
		if amount.IsNegative() {
			side, amount = side.Opposite(), amount.Neg()
		}
		lines = append(lines, PlannedLine{AmountKey: ra.AmountKey, AccountCode: account, Side: side, Amount: amount, Memo: memo})
		return nil
	}
	resolve := func(ra RuleAccount) string {
		if ra.Resolver == ResolverFixed {
			return ra.AccountCode
		}
		if code, ok := req.Accounts[ra.Resolver]; ok && code != "" {
			return code
		}
		return ra.AccountCode
	}

	for _, key := range keys {
		if splitKeys[key] {
			for _, s := range req.Splits {
				if s.AmountKey != key || s.Amount.IsZero() {
					continue
				}
				for _, ra := range byKey[key] {
					account := resolve(ra)
					if ra.Resolver != ResolverFixed && s.AccountCode != "" {
						account = s.AccountCode
					}
					if err := add(ra, s.Amount, account, s.Memo); err != nil {
						return nil, err
					}
				}
			}
			continue
		}
		amount, ok := req.Amounts[key]
		if !ok || amount.IsZero() {
			continue
		}
		for _, ra := range byKey[key] {
			if err := add(ra, amount, resolve(ra), ""); err != nil {
				return nil, err
			}
		}
	}

	debit, credit := money.Zero, money.Zero
	for _, l := range lines {
		if l.Side == SideDebit {
			debit = debit.Add(l.Amount)
		} else {
			credit = credit.Add(l.Amount)
		}
	}
	if !debit.IsPositive() {
		return nil, ruleError(rule.Code, "tidak ada nilai yang dijurnal")
	}
	if !debit.Equal(credit) {
		return nil, apperr.Validation(map[string]string{
			"amounts": fmt.Sprintf("Jurnal rule %s tidak balance: debit %s, kredit %s", rule.Code, debit.String(), credit.String()),
		})
	}
	return lines, nil
}

// HasDimension memeriksa apakah rule mewajibkan dimensi.
func (rule PostingRule) HasDimension(d Dimension) bool {
	return slices.Contains(rule.RequiredDimensions, d)
}
