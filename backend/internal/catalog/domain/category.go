package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/softdelete"
)

// Batas panjang kategori.
const (
	MaxCategoryCodeLength = 20
	MaxCategoryNameLength = 80
)

// Category adalah kategori produk (dipakai margin per kategori, RPT-15). Kode unik permanen.
type Category struct {
	ID   uuid.UUID
	Code string
	Name string
	softdelete.Mark
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// CategoryInput adalah isian kategori.
type CategoryInput struct {
	Code string
	Name string
}

// NewCategory memvalidasi kategori baru.
func NewCategory(in CategoryInput) (Category, error) {
	c := Category{Code: NormalizeCode(in.Code)}
	fields := map[string]string{}
	switch {
	case c.Code == "":
		fields["code"] = "Kode wajib diisi"
	case len(c.Code) > MaxCategoryCodeLength:
		fields["code"] = fmt.Sprintf("Kode maksimal %d karakter", MaxCategoryCodeLength)
	case !codePattern.MatchString(c.Code):
		fields["code"] = "Kode hanya boleh huruf, angka, dan tanda minus"
	}
	c.setName(in.Name, fields)
	if len(fields) > 0 {
		return Category{}, apperr.Validation(fields)
	}
	return c, nil
}

// Rename mengubah nama kategori; kode tidak bisa diubah.
func (c *Category) Rename(name string) error {
	if err := c.EnsureActive(); err != nil {
		return err
	}
	fields := map[string]string{}
	next := *c
	next.setName(name, fields)
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	*c = next
	return nil
}

func (c *Category) setName(name string, fields map[string]string) {
	c.Name = strings.TrimSpace(name)
	switch {
	case c.Name == "":
		fields["name"] = "Nama wajib diisi"
	case len([]rune(c.Name)) > MaxCategoryNameLength:
		fields["name"] = fmt.Sprintf("Nama maksimal %d karakter", MaxCategoryNameLength)
	}
}
