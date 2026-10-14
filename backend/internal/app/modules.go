package app

import (
	"fmt"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/riverqueue/river"
)

// module adalah kontrak yang dipenuhi API publik setiap modul untuk didaftarkan.
type module interface {
	Name() string
	RegisterRoutes(r chi.Router)
	RegisterWorkers(w *river.Workers)
	PeriodicJobs() []*river.PeriodicJob
}

// KnownModules adalah nama modul yang boleh dipakai di ENABLED_MODULES (Technical Guideline 2.2).
// Modul yang belum dibangun tetap terdaftar agar konfigurasi rilis berikutnya tidak perlu diubah.
var KnownModules = []string{
	// Domain bisnis
	"catalog", "pricing", "party", "procurement", "warehouse", "inventory", "logistics", "sales",
	"pipeline", "crm", "reseller", "vending", "storefront", "hris", "payroll", "accounting", "tax",
	// Lintas fungsi
	"identity", "approval", "notification", "document", "filestore", "dataexchange", "audit", "setting",
}

// validateModules menolak nama modul yang tidak dikenal, supaya salah ketik tidak diam-diam mematikan modul.
func validateModules(enabled []string) error {
	for _, m := range enabled {
		if !slices.Contains(KnownModules, m) {
			return fmt.Errorf("ENABLED_MODULES: modul %q tidak dikenal", m)
		}
	}
	return nil
}
