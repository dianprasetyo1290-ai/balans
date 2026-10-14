// Command cutover akan mengimpor data awal saat go-live: master data, saldo stok per batch per lokasi,
// AR/AP terbuka, PO terbuka, aset tetap, dan saldo awal GL (U-10, A-36, Technical Guideline 5.19).
// Setelah import, tool menjalankan pengecekan rekonsiliasi. Importer per template ditambahkan
// bersama modul pemilik datanya; saat ini hanya pengecekan koneksi dan versi skema.
package main

import (
	"context"
	"errors"
	"log/slog"

	"balans/internal/app"
	"balans/internal/platform/config"
	"balans/internal/platform/db"
)

func main() {
	app.Main("cutover", func(ctx context.Context, cfg config.Config, log *slog.Logger, _ func(context.Context, error)) error {
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
		log.Info("cutover ready; belum ada importer yang terdaftar", "schema_version", status.Current)
		return nil
	})
}
