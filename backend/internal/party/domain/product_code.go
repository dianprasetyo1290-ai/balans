package domain

import (
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
)

// ProductCode adalah kode artikel dan barcode retailer untuk satu SKU di satu mitra (MD-07). Dicetak di label
// produk dan DO mitra tersebut.
type ProductCode struct {
	ID          uuid.UUID
	PartnerID   uuid.UUID
	ProductID   uuid.UUID
	ArticleCode string
	Barcode     string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
}

// ProductCodeInput adalah isian kode produk mitra.
type ProductCodeInput struct {
	ArticleCode string
	Barcode     string
	Description string
}

func (c *ProductCode) apply(f fieldErrors, in ProductCodeInput) error {
	c.ArticleCode = f.text("articleCode", in.ArticleCode, 40, true, "Kode artikel")
	c.Barcode = f.text("barcode", in.Barcode, 40, false, "Barcode")
	c.Description = f.text("description", in.Description, 200, false, "Keterangan")
	if len(f) > 0 {
		return apperr.Validation(f)
	}
	return nil
}

// NewProductCode memvalidasi kode produk mitra baru.
func NewProductCode(partnerID, productID uuid.UUID, in ProductCodeInput) (ProductCode, error) {
	f := fieldErrors{}
	if partnerID == uuid.Nil {
		f.add("partnerId", "Mitra wajib dipilih")
	}
	if productID == uuid.Nil {
		f.add("productId", "Produk wajib dipilih")
	}
	c := ProductCode{PartnerID: partnerID, ProductID: productID}
	if err := c.apply(f, in); err != nil {
		return ProductCode{}, err
	}
	return c, nil
}

// Update mengubah kode artikel, barcode, atau keterangan; mitra dan produk tetap.
func (c *ProductCode) Update(in ProductCodeInput) error { return c.apply(fieldErrors{}, in) }
