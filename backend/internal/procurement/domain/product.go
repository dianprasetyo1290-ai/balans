package domain

import (
	"fmt"
	"slices"

	"github.com/google/uuid"
)

// Jenis produk katalog yang dibeli (sama dengan kode catalog.ProductType).
const (
	ProductFinishedGood      = "finished_good"
	ProductPackaging         = "packaging"
	ProductReprocessMaterial = "reprocess_material"
)

var productTypeLabels = map[string]string{
	ProductFinishedGood: "Produk Jadi", ProductPackaging: "Packaging", ProductReprocessMaterial: "Material (GM)",
}

// ProductInfo adalah data produk katalog yang dibutuhkan validasi dokumen pengadaan.
type ProductInfo struct {
	ID       uuid.UUID
	Type     string
	IsBundle bool
	IsActive bool
	Deleted  bool
}

// checkProduct memeriksa produk boleh dipakai dokumen baru dengan jenis yang diizinkan (kosong = semua jenis).
func checkProduct(id uuid.UUID, products map[uuid.UUID]ProductInfo, allowed []string) string {
	p, ok := products[id]
	switch {
	case !ok || p.Deleted:
		return "Produk tidak ditemukan"
	case !p.IsActive:
		return "Produk nonaktif"
	case p.IsBundle:
		return "Bundel virtual tidak bisa dibeli; pilih produk komponennya"
	case len(allowed) > 0 && !slices.Contains(allowed, p.Type):
		labels := make([]string, len(allowed))
		for i, t := range allowed {
			labels[i] = productTypeLabels[t]
		}
		return fmt.Sprintf("Produk harus berjenis %s", joinOr(labels))
	}
	return ""
}

func joinOr(labels []string) string {
	switch len(labels) {
	case 0:
		return ""
	case 1:
		return labels[0]
	}
	out := labels[0]
	for _, l := range labels[1 : len(labels)-1] {
		out += ", " + l
	}
	return out + " atau " + labels[len(labels)-1]
}
