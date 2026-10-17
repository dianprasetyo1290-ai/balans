// Command cutover mengimpor data awal saat go-live (U-10, A-36, Technical Guideline 5.19, Panduan Migrasi Data
// Fase 1): master data, aset tetap, piutang/utang terbuka, saldo stok per batch per lokasi, dan Neraca Saldo Awal,
// berurutan lewat importer dataexchange yang sama dengan Integrasi › Import/Export Excel. Setelah impor, tool
// menjalankan pengecekan rekonsiliasi (persediaan, piutang, utang, aset tetap vs GL; neraca balance).
//
// Contoh:
//
//	cutover -user headfinance@balans.id -cutover-date 2026-12-31 Template-Migrasi-Master-Data.xlsx Template-Migrasi-Saldo-Awal.xlsx
//	cutover -user headfinance@balans.id -commit -post Template-Migrasi-Saldo-Awal.xlsx
//
// Tanpa -commit tool berjalan dry-run: semua impor di satu transaksi yang dibatalkan. Dengan -commit impor
// idempotent per file (SHA-256) sehingga mock migration bisa diulang. -post memposting jurnal A-36 bila
// rekonsiliasi lolos. Jalankan dengan worker berhenti (freeze) agar job impor tidak diproses ganda.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"balans/internal/app"
	"balans/internal/platform/clock"
	"balans/internal/platform/config"
	"balans/internal/platform/db"
)

func main() {
	app.Main("cutover", func(ctx context.Context, cfg config.Config, log *slog.Logger, report func(context.Context, error)) error {
		fs := flag.NewFlagSet("cutover", flag.ContinueOnError)
		user := fs.String("user", "", "email user pelaksana (permission impor & audit trail)")
		date := fs.String("cutover-date", "", "tanggal cut-over YYYY-MM-DD (akhir bulan); kosong = yang sudah diisi di Saldo Awal")
		commit := fs.Bool("commit", false, "simpan data (tanpa flag = dry-run)")
		post := fs.Bool("post", false, "posting Neraca Saldo Awal (A-36) bila rekonsiliasi lolos")
		if err := fs.Parse(os.Args[1:]); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			return errors.New("sebutkan minimal satu file xlsx migrasi")
		}
		opts := app.CutoverOptions{Files: fs.Args(), UserEmail: *user, Commit: *commit, Post: *post, Out: os.Stdout}
		if *date != "" {
			d, err := clock.ParseBusinessDate(*date)
			if err != nil {
				return fmt.Errorf("-cutover-date: %w", err)
			}
			opts.CutoverDate = d
		}

		if err := checkSchema(ctx, cfg); err != nil {
			return err
		}
		a, err := app.New(ctx, cfg, log, report)
		if err != nil {
			return err
		}
		defer a.Close()
		return a.Cutover(ctx, opts)
	})
}

// checkSchema memastikan migrasi sudah dijalankan sebelum impor.
func checkSchema(ctx context.Context, cfg config.Config) error {
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	m, err := db.NewMigrator(pool)
	if err != nil {
		return err
	}
	defer m.Close() //nolint:errcheck // hanya menutup pembungkus database/sql
	status, err := m.Status(ctx)
	if err != nil {
		return err
	}
	if !status.UpToDate() {
		return errors.New("jalankan migrate up sebelum cutover")
	}
	return nil
}
