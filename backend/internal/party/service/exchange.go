package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"balans/internal/party/domain"
	"balans/internal/party/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/platform/pii"
	"balans/internal/platform/tabular"
)

// Permission impor/ekspor (matriks Desain Role & Approval 3.1, kode I).
const (
	PermCustomerImport    = "party.customer.import"
	PermCustomerExport    = "party.customer.export"
	PermSupplierImport    = "party.supplier.import"
	PermSupplierExport    = "party.supplier.export"
	PermProductCodeImport = "party.partner_product_code.import"
	PermProductCodeExport = "party.partner_product_code.export"
)

// EmployeeNumbers opsional pada Employees: mencari karyawan dari nomor karyawan (impor pelanggan).
type EmployeeNumbers interface {
	EmployeeIDByNumber(ctx context.Context, number string) (uuid.UUID, bool, error)
}

// ProductSKUs opsional pada Products: mencari produk yang belum dihapus dari SKU (impor kode produk mitra).
type ProductSKUs interface {
	ProductIDBySKU(ctx context.Context, sku string) (uuid.UUID, bool, error)
}

// exportPageSize adalah ukuran halaman saat mengalirkan data ekspor.
const exportPageSize = paging.MaxSize

// eachPage memanggil fetch per halaman sampai seluruh data terbaca.
func eachPage[T any](fetch func(paging.Page) (paging.Result[T], error), fn func(T) error) error {
	page := paging.Page{Number: 1, Size: exportPageSize}
	for {
		res, err := fetch(page)
		if err != nil {
			return err
		}
		for _, item := range res.Items {
			if err := fn(item); err != nil {
				return err
			}
		}
		if int64(page.Number*page.Size) >= res.Total || len(res.Items) == 0 {
			return nil
		}
		page.Number++
	}
}

// remapFields memetakan field error domain (mis. `addresses.0.city`) ke key kolom impor.
func remapFields(err error, mapping func(string) string) error {
	appErr := apperr.As(err)
	if appErr.Code != apperr.CodeValidationFailed || len(appErr.Fields) == 0 {
		return err
	}
	fields := make(map[string]string, len(appErr.Fields))
	for k, v := range appErr.Fields {
		k = mapping(k)
		if _, dup := fields[k]; !dup {
			fields[k] = v
		}
	}
	return apperr.Validation(fields)
}

// unchangedSecret: nilai tersamar hasil ekspor (mis. •••1234) berarti data pribadi tidak diubah.
func unchangedSecret(v string) bool { return strings.Contains(v, "•") }

// --- Pelanggan ----------------------------------------------------------------------------------

var customerTypeOptions = []tabular.Option{
	{Value: string(domain.CustomerRetail), Label: "Retail"},
	{Value: string(domain.CustomerB2B), Label: "B2B"},
	{Value: string(domain.CustomerReseller), Label: "Reseller"},
	{Value: string(domain.CustomerEmployee), Label: "Karyawan"},
}

// CustomerColumns adalah kolom template pelanggan (MD-06), mengikuti template migrasi sheet Pelanggan.
var CustomerColumns = []tabular.Column{
	{Key: "code", Header: "Kode Pelanggan", Kind: tabular.KindText, Example: "",
		Note: "Kosongkan untuk pelanggan baru (kode dibuat sistem). Isi untuk memperbarui pelanggan yang ada."},
	{Key: "fullName", Header: "Nama Lengkap", Kind: tabular.KindText, Required: true, Example: "Siti Rahma"},
	{Key: "phone", Header: "Nomor WA/HP", Kind: tabular.KindText, Required: true, Identity: true, Example: "081234567890",
		Note: "Unik per pelanggan; pelanggan dengan nomor yang sama diperbarui."},
	{Key: "email", Header: "Email", Kind: tabular.KindText, Example: "siti@contoh.id"},
	{Key: "customerType", Header: "Tipe Pelanggan", Kind: tabular.KindChoice, Required: true, Options: customerTypeOptions, Example: "Retail"},
	{Key: "addressLine", Header: "Alamat Pengiriman", Kind: tabular.KindText, Example: "Jl. Melati No. 5"},
	{Key: "district", Header: "Kecamatan", Kind: tabular.KindText, Example: "Coblong"},
	{Key: "city", Header: "Kota/Kabupaten", Kind: tabular.KindText, Example: "Kota Bandung", Note: "Wajib bila alamat diisi."},
	{Key: "province", Header: "Provinsi", Kind: tabular.KindText, Example: "Jawa Barat"},
	{Key: "postalCode", Header: "Kode Pos", Kind: tabular.KindText, Example: "40132"},
	{Key: "employeeNumber", Header: "Nomor Karyawan", Kind: tabular.KindText, Example: "",
		Note: "Wajib untuk tipe Karyawan; karyawan harus aktif."},
	{Key: "notes", Header: "Catatan", Kind: tabular.KindText},
}

func customerField(k string) string {
	switch {
	case k == "employeeId":
		return "employeeNumber"
	case strings.HasPrefix(k, "addresses."):
		parts := strings.Split(k, ".")
		if f := parts[len(parts)-1]; f != "recipientName" && f != "label" && f != "id" {
			return f
		}
		return "addressLine"
	}
	return k
}

func addressInput(a domain.Address) domain.AddressInput {
	in := domain.AddressInput{
		ID: a.ID, Label: a.Label, RecipientName: a.RecipientName, Phone: a.Phone, AddressLine: a.AddressLine,
		District: a.District, City: a.City, Province: a.Province, PostalCode: a.PostalCode, IsDefault: a.IsDefault,
	}
	if a.Latitude != nil && a.Longitude != nil {
		lat, lng := a.Latitude.String(), a.Longitude.String()
		in.Latitude, in.Longitude = &lat, &lng
	}
	return in
}

// customerFromRow menyusun isian pelanggan dari baris Excel. cur berisi pelanggan lama bila baris memperbarui
// (kode atau nomor HP sama).
func (s *Service) customerFromRow(ctx context.Context, row tabular.Row) (domain.CustomerInput, *domain.Customer, error) {
	p := tabular.Parse(row)
	in := domain.CustomerInput{
		FullName: p.Text("fullName"), Phone: p.Text("phone"), Email: p.Text("email"),
		Type: domain.CustomerType(p.Choice("customerType", customerTypeOptions)), Notes: p.Text("notes"),
	}
	if num := p.Text("employeeNumber"); num != "" {
		lookup, ok := s.Employees.(EmployeeNumbers)
		if !ok {
			p.Fail("employeeNumber", "Modul karyawan belum aktif")
		} else if id, found, err := lookup.EmployeeIDByNumber(ctx, strings.ToUpper(num)); err != nil {
			return in, nil, err
		} else if !found {
			p.Fail("employeeNumber", "Nomor karyawan tidak ditemukan")
		} else {
			in.EmployeeID = &id
		}
	}
	var cur *domain.Customer
	if code := strings.ToUpper(p.Text("code")); code != "" {
		id, found, err := s.Store.CustomerIDByCode(ctx, code)
		if err != nil {
			return in, nil, err
		}
		if !found {
			p.Fail("code", "Kode pelanggan tidak ditemukan. Kosongkan untuk pelanggan baru")
		} else {
			c, err := s.Store.GetCustomer(ctx, id, true)
			if err != nil {
				return in, nil, err
			}
			cur = &c
		}
	} else if phone, ok := domain.NormalizePhone(in.Phone); ok {
		c, found, err := s.Store.CustomerByPhone(ctx, phone)
		if err != nil {
			return in, nil, err
		}
		if found {
			full, err := s.Store.GetCustomer(ctx, c.ID, true)
			if err != nil {
				return in, nil, err
			}
			cur = &full
		}
	}
	if err := p.Err(); err != nil {
		return in, nil, err
	}
	var addr *domain.AddressInput
	if p.Text("addressLine") != "" || p.Text("city") != "" || p.Text("district") != "" || p.Text("province") != "" || p.Text("postalCode") != "" {
		addr = &domain.AddressInput{
			RecipientName: in.FullName, Phone: in.Phone, AddressLine: p.Text("addressLine"), District: p.Text("district"),
			City: p.Text("city"), Province: p.Text("province"), PostalCode: p.Text("postalCode"), IsDefault: true,
		}
	}
	if cur == nil {
		if addr != nil {
			in.Addresses = []domain.AddressInput{*addr}
		}
		return in, nil, nil
	}
	// Pelanggan lama: persetujuan pemasaran dipertahankan; alamat utama diganti isi baris, alamat lain tetap.
	in.MarketingConsent, in.MarketingConsentVersion = cur.HasMarketingConsent(), cur.MarketingConsentVersion
	replaced := false
	for _, a := range cur.Addresses {
		ai := addressInput(a)
		if addr != nil && a.IsDefault && !replaced {
			addr.ID, addr.Label = a.ID, a.Label
			ai, replaced = *addr, true
		}
		in.Addresses = append(in.Addresses, ai)
	}
	if addr != nil && !replaced {
		for i := range in.Addresses {
			in.Addresses[i].IsDefault = false
		}
		in.Addresses = append(in.Addresses, *addr)
	}
	return in, cur, nil
}

// ValidateCustomerRow memeriksa baris pelanggan tanpa menyimpan (kode pelanggan dibuat dari sequence yang
// tidak ikut dibatalkan, sehingga pemeriksaan tidak boleh menyisipkan data).
func (s *Service) ValidateCustomerRow(ctx context.Context, row tabular.Row) error {
	if _, err := actor.Require(ctx, PermCustomerImport); err != nil {
		return err
	}
	if _, err := actor.Require(ctx, PermCustomerWrite); err != nil {
		return err
	}
	in, cur, err := s.customerFromRow(ctx, row)
	if err != nil {
		return remapFields(err, customerField)
	}
	if cur == nil {
		c, err := domain.NewCustomer(in, s.Clock.Now())
		if err != nil {
			return remapFields(err, customerField)
		}
		return remapFields(s.checkCustomer(ctx, c, nil), customerField)
	}
	next := *cur
	next.Addresses = append([]domain.Address(nil), cur.Addresses...)
	if err := next.Update(in, s.Clock.Now()); err != nil {
		return remapFields(err, customerField)
	}
	return remapFields(s.checkCustomer(ctx, next, cur.EmployeeID), customerField)
}

// ImportCustomerRow membuat pelanggan baru atau memperbarui pelanggan dengan kode/nomor HP yang sama.
func (s *Service) ImportCustomerRow(ctx context.Context, row tabular.Row) error {
	if _, err := actor.Require(ctx, PermCustomerImport); err != nil {
		return err
	}
	in, cur, err := s.customerFromRow(ctx, row)
	if err != nil {
		return remapFields(err, customerField)
	}
	if cur == nil {
		_, err = s.CreateCustomer(ctx, in)
	} else {
		_, err = s.UpdateCustomer(ctx, cur.ID, cur.Version, in)
	}
	if err != nil {
		return remapFields(err, customerField)
	}
	return nil
}

// ExportCustomers mengalirkan pelanggan urut kode dengan alamat utama dan nomor karyawan yang tertaut.
func (s *Service) ExportCustomers(ctx context.Context, emit func(tabular.Row) error) error {
	if _, err := actor.Require(ctx, PermCustomerExport); err != nil {
		return err
	}
	page := paging.Page{Number: 1, Size: exportPageSize}
	for {
		res, err := s.Store.ListCustomers(ctx, postgres.CustomerFilter{Sort: "code"}, page)
		if err != nil {
			return err
		}
		var ids []uuid.UUID
		for _, c := range res.Items {
			if c.EmployeeID != nil {
				ids = append(ids, *c.EmployeeID)
			}
		}
		refs, err := s.EmployeeRefs(ctx, ids)
		if err != nil {
			return err
		}
		for _, c := range res.Items {
			r := tabular.NewRow(0)
			r.Set("code", c.Code).Set("fullName", c.FullName).Set("phone", c.Phone).Set("email", c.Email)
			r.Set("customerType", tabular.Label(customerTypeOptions, string(c.Type))).Set("notes", c.Notes)
			if c.EmployeeID != nil {
				r.Set("employeeNumber", refs[*c.EmployeeID].Number)
			}
			for _, a := range c.Addresses {
				if a.IsDefault {
					r.Set("addressLine", a.AddressLine).Set("district", a.District).Set("city", a.City)
					r.Set("province", a.Province).Set("postalCode", a.PostalCode)
				}
			}
			if err := emit(r); err != nil {
				return err
			}
		}
		if int64(page.Number*page.Size) >= res.Total || len(res.Items) == 0 {
			return nil
		}
		page.Number++
	}
}

// --- Pemasok & maklon --------------------------------------------------------------------------

var supplierTypeOptions = []tabular.Option{
	{Value: string(domain.SupplierRegular), Label: "Pemasok"},
	{Value: string(domain.SupplierTollManufacturer), Label: "Maklon"},
}

var withholdingOptions = []tabular.Option{
	{Value: string(domain.WithholdingNone), Label: "Tidak Ada"},
	{Value: string(domain.WithholdingPPh21), Label: "PPh 21"},
	{Value: string(domain.WithholdingPPh23), Label: "PPh 23"},
	{Value: string(domain.WithholdingPPh42), Label: "PPh 4(2)"},
}

// SupplierColumns adalah kolom template pemasok & maklon (MD-08), mengikuti template migrasi.
var SupplierColumns = []tabular.Column{
	{Key: "code", Header: "Kode Pemasok", Kind: tabular.KindText, Required: true, Identity: true, Example: "SUP-001"},
	{Key: "name", Header: "Nama", Kind: tabular.KindText, Required: true, Example: "PT Kemasan Jaya"},
	{Key: "supplierType", Header: "Jenis", Kind: tabular.KindChoice, Required: true, Options: supplierTypeOptions, Example: "Pemasok"},
	{Key: "isBusinessEntity", Header: "Badan Usaha", Kind: tabular.KindBool, Required: true, Example: tabular.Yes},
	{Key: "npwp", Header: "NPWP", Kind: tabular.KindText, Sensitive: true, Example: "012345678901234",
		Note: "15/16 digit. Nilai tersamar dari file ekspor (•••) berarti tidak diubah."},
	{Key: "defaultWithholding", Header: "PPh Default", Kind: tabular.KindChoice, Options: withholdingOptions, Example: "PPh 23",
		Note: "Kosong = Tidak Ada."},
	{Key: "paymentTermDays", Header: "Termin Pembayaran (hari)", Kind: tabular.KindInteger, Example: "30", Note: "Kosong = 0."},
	{Key: "address", Header: "Alamat", Kind: tabular.KindText, Example: "Jl. Industri No. 1, Bekasi"},
	{Key: "phone", Header: "Telepon", Kind: tabular.KindText, Example: "0211234567"},
	{Key: "email", Header: "Email", Kind: tabular.KindText},
	{Key: "picName", Header: "Nama PIC", Kind: tabular.KindText},
	{Key: "bankName", Header: "Bank", Kind: tabular.KindText, Example: "BCA"},
	{Key: "accountNumber", Header: "Nomor Rekening", Kind: tabular.KindText, Sensitive: true, Example: "1234567890",
		Note: "Rekening utama. Nilai tersamar dari file ekspor (•••) berarti tidak diubah."},
	{Key: "accountName", Header: "Nama Pemilik Rekening", Kind: tabular.KindText, Example: "PT Kemasan Jaya"},
	{Key: "notes", Header: "Catatan", Kind: tabular.KindText},
}

func supplierField(k string) string {
	if strings.HasPrefix(k, "bankAccounts.") {
		parts := strings.Split(k, ".")
		if f := parts[len(parts)-1]; f == "bankName" || f == "accountName" || f == "accountNumber" {
			return f
		}
		return "accountNumber"
	}
	if k == "bankAccounts" {
		return "accountNumber"
	}
	return k
}

func (s *Service) supplierFromRow(ctx context.Context, row tabular.Row) (domain.SupplierInput, *domain.Supplier, error) {
	p := tabular.Parse(row)
	in := domain.SupplierInput{
		Code: p.Text("code"), Name: p.Text("name"), Type: domain.SupplierType(p.Choice("supplierType", supplierTypeOptions)),
		IsBusinessEntity:   p.Bool("isBusinessEntity", false),
		DefaultWithholding: domain.WithholdingType(p.Choice("defaultWithholding", withholdingOptions)),
		Address:            p.Text("address"), Phone: p.Text("phone"), Email: p.Text("email"), PICName: p.Text("picName"),
		Notes: p.Text("notes"),
	}
	if in.DefaultWithholding == "" {
		in.DefaultWithholding = domain.WithholdingNone
	}
	if t := p.Int("paymentTermDays"); t != nil {
		in.PaymentTermDays = *t
	}
	if v := p.Text("npwp"); !unchangedSecret(v) {
		in.NPWP = &v
	}
	if err := p.Err(); err != nil {
		return in, nil, err
	}
	var cur *domain.Supplier
	id, found, err := s.Store.SupplierIDByCode(ctx, strings.ToUpper(in.Code))
	if err != nil {
		return in, nil, err
	}
	if found {
		c, err := s.Store.GetSupplier(ctx, id, true)
		if err != nil {
			return in, nil, err
		}
		cur = &c
	}
	number, bank, holder := p.Text("accountNumber"), p.Text("bankName"), p.Text("accountName")
	hasAccount := number != "" || bank != "" || holder != ""
	if cur == nil {
		if in.NPWP != nil && *in.NPWP == "" {
			in.NPWP = nil
		}
		if hasAccount {
			if unchangedSecret(number) {
				number = ""
			}
			in.BankAccounts = []domain.BankAccountInput{{BankName: bank, AccountNumber: number, AccountName: holder, IsDefault: true}}
		}
		return in, nil, nil
	}
	// Pemasok lama: rekening lain dipertahankan; rekening utama diganti/ditambah sesuai baris.
	normalized, _ := domain.NormalizeAccountNumber(number)
	matched := false
	for _, b := range cur.BankAccounts {
		bi := domain.BankAccountInput{ID: b.ID, BankName: b.BankName, AccountName: b.AccountName, IsDefault: b.IsDefault && !hasAccount}
		isTarget := hasAccount && !matched &&
			((unchangedSecret(number) && b.IsDefault) || (normalized != "" && b.AccountNumber == normalized))
		if isTarget {
			bi.BankName, bi.AccountName, bi.IsDefault, matched = bank, holder, true, true
		}
		in.BankAccounts = append(in.BankAccounts, bi)
	}
	if hasAccount && !matched {
		if unchangedSecret(number) {
			number = ""
		}
		in.BankAccounts = append(in.BankAccounts, domain.BankAccountInput{BankName: bank, AccountNumber: number, AccountName: holder, IsDefault: true})
	}
	return in, cur, nil
}

// ImportSupplierRow membuat atau memperbarui pemasok/maklon dengan kode yang sama.
func (s *Service) ImportSupplierRow(ctx context.Context, row tabular.Row) error {
	if _, err := actor.Require(ctx, PermSupplierImport); err != nil {
		return err
	}
	in, cur, err := s.supplierFromRow(ctx, row)
	if err != nil {
		return remapFields(err, supplierField)
	}
	if cur == nil {
		_, err = s.CreateSupplier(ctx, in)
	} else {
		_, err = s.UpdateSupplier(ctx, cur.ID, cur.Version, in)
	}
	if err != nil {
		return remapFields(err, supplierField)
	}
	return nil
}

// ExportSuppliers mengalirkan pemasok urut kode. NPWP dan nomor rekening ditulis tersamar (TG 6.6).
func (s *Service) ExportSuppliers(ctx context.Context, emit func(tabular.Row) error) error {
	if _, err := actor.Require(ctx, PermSupplierExport); err != nil {
		return err
	}
	return eachPage(func(page paging.Page) (paging.Result[domain.Supplier], error) {
		return s.Store.ListSuppliers(ctx, postgres.SupplierFilter{Sort: "code"}, page)
	}, func(sup domain.Supplier) error {
		r := tabular.NewRow(0)
		r.Set("code", sup.Code).Set("name", sup.Name).Set("supplierType", tabular.Label(supplierTypeOptions, string(sup.Type)))
		r.Set("isBusinessEntity", tabular.BoolLabel(sup.IsBusinessEntity))
		if sup.NPWP != "" {
			r.Set("npwp", pii.Mask(sup.NPWP))
		}
		r.Set("defaultWithholding", tabular.Label(withholdingOptions, string(sup.DefaultWithholding)))
		r.Set("paymentTermDays", tabular.FormatInt(&sup.PaymentTermDays)).Set("address", sup.Address).Set("phone", sup.Phone)
		r.Set("email", sup.Email).Set("picName", sup.PICName).Set("notes", sup.Notes)
		if b, ok := sup.DefaultBankAccount(); ok {
			r.Set("bankName", b.BankName).Set("accountNumber", pii.Mask(b.AccountNumber)).Set("accountName", b.AccountName)
		}
		return emit(r)
	})
}

// --- Kode produk mitra (S-134) --------------------------------------------------------------------

// ProductCodeColumns adalah kolom template kode artikel & barcode retailer per SKU per mitra (MD-07).
var ProductCodeColumns = []tabular.Column{
	{Key: "partnerCode", Header: "Kode Mitra", Kind: tabular.KindText, Required: true, Identity: true, Example: "MT-ALFA"},
	{Key: "sku", Header: "SKU", Kind: tabular.KindText, Required: true, Identity: true, Example: "MS-COK-30"},
	{Key: "articleCode", Header: "Kode Artikel", Kind: tabular.KindText, Required: true, Example: "ALF-778812",
		Note: "Unik per mitra."},
	{Key: "barcode", Header: "Barcode Retailer", Kind: tabular.KindText, Example: "2000123456789"},
	{Key: "description", Header: "Keterangan", Kind: tabular.KindText},
}

func productCodeField(k string) string {
	switch k {
	case "partnerId":
		return "partnerCode"
	case "productId":
		return "sku"
	}
	return k
}

// ImportProductCodeRow membuat atau memperbarui kode produk mitra untuk pasangan mitra–SKU.
func (s *Service) ImportProductCodeRow(ctx context.Context, row tabular.Row) error {
	if _, err := actor.Require(ctx, PermProductCodeImport); err != nil {
		return err
	}
	p := tabular.Parse(row)
	partnerID, found, err := s.Store.PartnerIDByCode(ctx, strings.ToUpper(p.Text("partnerCode")))
	if err != nil {
		return err
	}
	if !found {
		p.Fail("partnerCode", "Kode mitra tidak ditemukan")
	}
	skus, ok := s.Products.(ProductSKUs)
	var productID uuid.UUID
	if !ok {
		p.Fail("sku", "Modul produk belum aktif")
	} else if id, found, err := skus.ProductIDBySKU(ctx, strings.ToUpper(p.Text("sku"))); err != nil {
		return err
	} else if !found {
		p.Fail("sku", "SKU tidak ditemukan")
	} else {
		productID = id
	}
	if err := p.Err(); err != nil {
		return err
	}
	in := domain.ProductCodeInput{ArticleCode: p.Text("articleCode"), Barcode: p.Text("barcode"), Description: p.Text("description")}
	cur, exists, err := s.Store.ProductCodeFor(ctx, partnerID, productID)
	if err != nil {
		return err
	}
	if exists {
		_, err = s.UpdateProductCode(ctx, cur.ID, cur.Version, in)
	} else {
		_, err = s.CreateProductCode(ctx, partnerID, productID, in)
	}
	if err != nil {
		return remapFields(err, productCodeField)
	}
	return nil
}

// ExportProductCodes mengalirkan kode produk mitra urut kode mitra dan kode artikel.
func (s *Service) ExportProductCodes(ctx context.Context, emit func(tabular.Row) error) error {
	if _, err := actor.Require(ctx, PermProductCodeExport); err != nil {
		return err
	}
	return eachPage(func(page paging.Page) (paging.Result[domain.ProductCode], error) {
		return s.Store.ListProductCodes(ctx, postgres.ProductCodeFilter{}, page)
	}, func(c domain.ProductCode) error {
		partners, err := s.PartnerRefs(ctx, []uuid.UUID{c.PartnerID})
		if err != nil {
			return err
		}
		products, err := s.ProductRefs(ctx, []uuid.UUID{c.ProductID})
		if err != nil {
			return err
		}
		r := tabular.NewRow(0)
		r.Set("partnerCode", partners[c.PartnerID].Code).Set("sku", products[c.ProductID].SKU)
		r.Set("articleCode", c.ArticleCode).Set("barcode", c.Barcode).Set("description", c.Description)
		return emit(r)
	})
}
