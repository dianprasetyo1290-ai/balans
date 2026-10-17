import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { describe, expect, it } from "vitest"

import { EmployeeFormDialog } from "@/features/hris/components/employee-form-dialog"

import type {
  Customer,
  Partner,
  PartnerAgreement,
  Supplier,
} from "../api/party-api"
import { AgreementFormDialog } from "./agreement-form-dialog"
import { CustomerFormDialog } from "./customer-form-dialog"
import { PartnerFormDialog } from "./partner-form-dialog"
import { SupplierFormDialog } from "./supplier-form-dialog"

const noop = () => undefined
const stamp = {
  createdAt: "2026-10-01T02:00:00Z",
  updatedAt: "2026-10-01T02:00:00Z",
  version: 3,
}

function wrap(node: ReactNode) {
  return render(
    <QueryClientProvider client={new QueryClient()}>{node}</QueryClientProvider>
  )
}

const customer: Customer = {
  id: "c1",
  code: "PLG-000001",
  fullName: "Rudi Karyawan",
  phone: "+6281234567890",
  email: "",
  customerType: "employee",
  employeeId: "e1",
  employeeName: "Rudi Hartono",
  employeeNumber: "BN-001",
  isEmployeeActive: true,
  marketingConsent: true,
  marketingConsentAt: "2026-10-01T02:00:00Z",
  marketingConsentVersion: "erp-2026-10",
  notes: "",
  addresses: [
    {
      id: "a1",
      label: "Rumah",
      recipientName: "Rudi",
      phone: "",
      addressLine: "Jl. Mawar 1",
      district: "",
      city: "Bandung",
      province: "",
      postalCode: "",
      isDefault: true,
    },
  ],
  ...stamp,
}

const partner: Partner = {
  id: "p1",
  code: "ALFA",
  name: "Alfamart",
  partnerType: "consignment",
  hasNpwp: true,
  npwpMasked: "•••••••••••1000",
  address: "",
  picName: "",
  picPhone: "",
  merchandiserUserId: "u1",
  merchandiserUserName: "Maman",
  notes: "",
  activeAgreements: [],
  ...stamp,
}

const agreement: PartnerAgreement = {
  id: "g1",
  partnerId: "p1",
  agreementNumber: "PKS/01",
  agreementType: "consignment",
  validFrom: "2026-01-01",
  validTo: "2026-12-31",
  status: "active",
  fileId: "f1",
  fileName: "PKS Alfa.pdf",
  consignmentMarginPercent: "25.0000",
  paymentTermDays: 30,
  lossBorneBy: "partner",
  partnerDamageBorneBy: "partner",
  productDefectBorneBy: "balans",
  nearExpiryBorneBy: "balans",
  notes: "",
  ...stamp,
}

const supplier: Supplier = {
  id: "s1",
  code: "MKL-01",
  name: "PT Maklon Sehat",
  supplierType: "toll_manufacturer",
  isBusinessEntity: true,
  hasNpwp: true,
  npwpMasked: "•••••••••••1000",
  defaultWithholding: "pph23",
  paymentTermDays: 30,
  address: "",
  phone: "",
  email: "",
  picName: "",
  notes: "",
  bankAccounts: [
    {
      id: "b1",
      bankName: "BCA",
      accountNumberMasked: "••••••7890",
      accountName: "PT Maklon Sehat",
      isDefault: true,
    },
  ],
  ...stamp,
}

describe("form data master", () => {
  it("pelanggan karyawan menampilkan karyawan tertaut dan alamat", async () => {
    wrap(
      <CustomerFormDialog
        open
        onOpenChange={noop}
        customer={customer}
        employeesEnabled
      />
    )
    expect(screen.getByText("Ubah Pelanggan PLG-000001")).toBeInTheDocument()
    expect(
      screen.getByRole("combobox", { name: /karyawan/i })
    ).toHaveTextContent("Rudi Hartono · BN-001")
    expect(screen.getByDisplayValue("Jl. Mawar 1")).toBeInTheDocument()
    await userEvent.click(screen.getByRole("button", { name: /Tambah Alamat/ }))
    expect(screen.getAllByText("Alamat utama")).toHaveLength(2)
  })

  it("pelanggan baru wajib nama dan HP", async () => {
    wrap(
      <CustomerFormDialog open onOpenChange={noop} employeesEnabled={false} />
    )
    await userEvent.click(screen.getByRole("button", { name: "Simpan" }))
    expect(screen.getByText("Nama lengkap wajib diisi")).toBeInTheDocument()
    expect(screen.getByText("Nomor HP wajib diisi")).toBeInTheDocument()
  })

  it("mitra menampilkan NPWP tersamar dan kode terkunci", () => {
    wrap(<PartnerFormDialog open onOpenChange={noop} partner={partner} />)
    expect(screen.getByLabelText("Kode mitra")).toBeDisabled()
    expect(screen.getByLabelText("NPWP")).toHaveAttribute(
      "placeholder",
      "•••••••••••1000 (kosongkan agar tetap)"
    )
  })

  it("PKS menampilkan PDF dan ketentuan selisih", () => {
    wrap(
      <AgreementFormDialog
        open
        onOpenChange={noop}
        partner={partner}
        agreement={agreement}
      />
    )
    expect(
      screen.getByRole("button", { name: /PKS Alfa.pdf/ })
    ).toBeInTheDocument()
    expect(screen.getByDisplayValue("25")).toBeInTheDocument()
    expect(screen.getByText("Barang hilang ditanggung")).toBeInTheDocument()
  })

  it("pemasok menampilkan rekening tersamar", () => {
    wrap(<SupplierFormDialog open onOpenChange={noop} supplier={supplier} />)
    expect(screen.getByLabelText("Nomor rekening")).toHaveAttribute(
      "placeholder",
      "••••••7890"
    )
  })

  it("karyawan baru wajib nomor, nama, jabatan, dan tanggal bergabung", async () => {
    wrap(<EmployeeFormDialog open onOpenChange={noop} />)
    await userEvent.click(screen.getByRole("button", { name: "Simpan" }))
    expect(screen.getByText("Nomor karyawan wajib diisi")).toBeInTheDocument()
    expect(
      screen.getByText("Tanggal bergabung wajib diisi")
    ).toBeInTheDocument()
  })
})
