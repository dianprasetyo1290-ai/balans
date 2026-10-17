package domain

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
)

var courierCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Courier adalah master kurir (LOG-03): JNE, J&T, SiCepat, GoSend, Grab, dan kurir internal (Merchandiser).
type Courier struct {
	ID         uuid.UUID
	Code       string
	Name       string
	IsInternal bool
	IsActive   bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Version    int
}

// CourierInput adalah isian master kurir.
type CourierInput struct {
	Code       string
	Name       string
	IsInternal bool
	IsActive   bool
}

// NewCourier memvalidasi kurir baru.
func NewCourier(in CourierInput) (Courier, error) {
	c := Courier{}
	return c, c.apply(in, true)
}

// Update mengubah nama, jenis, dan status kurir; kode tidak bisa diubah.
func (c *Courier) Update(in CourierInput) error {
	in.Code = c.Code
	return c.apply(in, false)
}

func (c *Courier) apply(in CourierInput, isNew bool) error {
	fields := map[string]string{}
	code := strings.ToLower(strings.TrimSpace(in.Code))
	name := strings.TrimSpace(in.Name)
	if isNew && (!courierCodePattern.MatchString(code) || len(code) > 30) {
		fields["code"] = "Kode huruf kecil/angka, maks. 30 karakter"
	}
	if name == "" || len([]rune(name)) > 100 {
		fields["name"] = "Nama wajib diisi (maks. 100 karakter)"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	c.Code, c.Name, c.IsInternal, c.IsActive = code, name, in.IsInternal, in.IsActive
	return nil
}
