package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"balans/internal/dataexchange"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// CutoverOptions adalah parameter cmd/cutover (Technical Guideline 5.19, Panduan Migrasi Data bagian 3).
type CutoverOptions struct {
	// Files adalah file xlsx; satu file boleh memuat banyak sheet (mis. Template Migrasi Saldo Awal).
	Files []string
	// UserEmail adalah user pelaksana impor (permission impor tiap jenis data + audit trail).
	UserEmail string
	// CutoverDate mengisi tanggal cut-over saldo awal bila belum ada (akhir bulan).
	CutoverDate clock.BusinessDate
	// Commit menyimpan data; false = dry-run (semua impor di satu transaksi yang dibatalkan).
	Commit bool
	// Post memposting Neraca Saldo Awal (A-36) setelah impor bila rekonsiliasi lolos.
	Post bool
	Out  io.Writer
}

// CutoverOrder adalah urutan impor sesuai ketergantungan data (Panduan Migrasi Data bagian 2): master dulu,
// harga setelah produk & mitra, saldo terakhir dengan Neraca Saldo Awal paling akhir.
var CutoverOrder = []string{
	"warehouse.location", "accounting.account", "catalog.product", "party.supplier", "party.customer",
	"party.partner_product_code", "hris.employee",
	"accounting.opening_fixed_asset", "accounting.opening_receivable", "accounting.opening_payable",
	"inventory.opening_stock", "accounting.opening_balance",
}

var (
	errCutoverFailed = errors.New("cut-over belum lolos: periksa baris error dan rekonsiliasi di atas")
	errDryRun        = errors.New("dry-run cut-over dibatalkan")
)

// Cutover mengimpor file migrasi berurutan lewat importer dataexchange lalu menjalankan pengecekan rekonsiliasi.
// Dry-run menjalankan semuanya di satu transaksi yang dibatalkan, sehingga jenis data berikutnya melihat hasil
// jenis data sebelumnya. Mode commit idempotent per file (SHA-256) per jenis data.
func (a *App) Cutover(ctx context.Context, opts CutoverOptions) error {
	if a.dataexchange == nil || a.accounting == nil || a.identity == nil {
		return errors.New("cut-over membutuhkan modul dataexchange, accounting, dan identity aktif")
	}
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	ctx, err := a.cutoverActor(ctx, opts.UserEmail)
	if err != nil {
		return err
	}
	type file struct {
		name string
		data []byte
	}
	files := make([]file, 0, len(opts.Files))
	for _, path := range opts.Files {
		data, err := os.ReadFile(path) //nolint:gosec // path dari argumen operator cut-over
		if err != nil {
			return fmt.Errorf("baca %s: %w", path, err)
		}
		files = append(files, file{name: filepath.Base(path), data: data})
	}
	mode := "DRY-RUN (tidak ada data yang disimpan)"
	if opts.Commit {
		mode = "COMMIT"
	}
	_, _ = fmt.Fprintf(out, "Cut-over %s, %d file\n", mode, len(files))

	failed := false
	run := func(ctx context.Context) error {
		if !opts.CutoverDate.IsZero() {
			if err := a.accounting.EnsureCutoverDate(ctx, opts.CutoverDate); err != nil {
				return err
			}
		}
		for _, kind := range CutoverOrder {
			for _, f := range files {
				ok, err := a.dataexchange.HasSheet(kind, f.data)
				if err != nil {
					if apperr.As(err).Fields["kind"] != "" {
						break // jenis data tidak aktif di ENABLED_MODULES
					}
					return fmt.Errorf("%s: %w", f.name, err)
				}
				if !ok {
					continue
				}
				var res dataexchange.FileResult
				if opts.Commit {
					res, err = a.dataexchange.RunFile(ctx, kind, f.name, f.data)
				} else {
					res, err = a.dataexchange.ApplyFileRows(ctx, kind, f.data)
				}
				if err != nil {
					_, _ = fmt.Fprintf(out, "✗ %-28s %s: %v\n", kind, f.name, err)
					failed = true
					continue
				}
				printFileResult(out, f.name, res)
				if res.Errors > 0 {
					failed = true
				}
			}
		}
		if opts.Post && !failed {
			if err := a.postOpeningIfReconciled(ctx, out); err != nil {
				return err
			}
		}
		rec, err := a.accounting.CutoverReconciliation(ctx)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(out, "\nRekonsiliasi cut-over (sub-ledger vs GL):")
		for _, c := range rec.Checks {
			mark := "✓"
			switch c.Status {
			case "failed":
				mark = "✗"
			case "skipped":
				mark = "-"
			}
			_, _ = fmt.Fprintf(out, "%s %-50s %-5s sub-ledger %15s  GL %15s  selisih %15s %s\n",
				mark, c.Label, c.AccountCode, c.Subledger.String(), c.Ledger.String(), c.Difference.String(), c.Note)
		}
		if !rec.Passed {
			failed = true
		}
		if !opts.Commit {
			return errDryRun
		}
		return nil
	}
	if opts.Commit {
		err = run(ctx)
	} else {
		err = a.Tx.WithTx(ctx, run)
		if errors.Is(err, errDryRun) {
			err = nil
		}
	}
	if err != nil {
		return err
	}
	if failed {
		return errCutoverFailed
	}
	_, _ = fmt.Fprintln(out, "\nCut-over lolos.")
	return nil
}

func printFileResult(out io.Writer, name string, r dataexchange.FileResult) {
	if r.Skipped {
		_, _ = fmt.Fprintf(out, "- %-28s %s: dilewati (%s)\n", r.Kind, name, r.SkipReason)
		return
	}
	mark := "✓"
	if r.Errors > 0 {
		mark = "✗"
	}
	_, _ = fmt.Fprintf(out, "%s %-28s %s: %d baris, %d valid, %d error, %d tersimpan\n",
		mark, r.Kind, name, r.Total, r.Valid, r.Errors, r.Committed)
	for _, row := range r.RowErrors {
		msgs := make([]string, len(row.Errors))
		for i, e := range row.Errors {
			msgs[i] = strings.TrimSpace(e.Column + " " + e.Message)
		}
		_, _ = fmt.Fprintf(out, "    baris %d: %s\n", row.Number, strings.Join(msgs, "; "))
	}
}

// postOpeningIfReconciled memposting A-36 hanya bila rekonsiliasi (dengan GL dari Neraca Saldo Awal) lolos.
func (a *App) postOpeningIfReconciled(ctx context.Context, out io.Writer) error {
	rec, err := a.accounting.CutoverReconciliation(ctx)
	if err != nil {
		return err
	}
	if !rec.Passed {
		_, _ = fmt.Fprintln(out, "Saldo awal tidak diposting: rekonsiliasi belum lolos.")
		return nil
	}
	number, err := a.accounting.PostOpeningBalances(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "Saldo awal diposting: %s\n", number)
	return nil
}

// cutoverActor menyusun actor dari email user pelaksana (permission terbaru dari identity).
func (a *App) cutoverActor(ctx context.Context, email string) (context.Context, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return ctx, errors.New("isi -user dengan email user pelaksana cut-over")
	}
	users, err := a.identity.SearchActiveUsers(ctx, email, 20)
	if err != nil {
		return ctx, err
	}
	for _, u := range users {
		if strings.EqualFold(u.Email, email) {
			act, err := a.identity.ActorFor(ctx, u.ID)
			if err != nil {
				return ctx, err
			}
			return actor.With(ctx, act), nil
		}
	}
	return ctx, fmt.Errorf("user aktif %s tidak ditemukan", email)
}
