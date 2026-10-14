"""Menghasilkan lembar keputusan workshop BRD (PRD F0-APR-01 s.d. F0-APR-03, bagian 9).

Sumber: tabel usulan di BRD (bagian 12) dan pernyataan K-01 s.d. K-16 di Daftar Konfirmasi Balans.
Jalankan: python scripts/baseline_approval_sheet.py
"""

from __future__ import annotations

import re
from pathlib import Path

from openpyxl import Workbook, load_workbook
from openpyxl.styles import Alignment, Font, PatternFill
from openpyxl.utils import get_column_letter
from openpyxl.worksheet.datavalidation import DataValidation

ROOT = Path(__file__).resolve().parent.parent
BRD = ROOT / "docs" / "BRD-Balans-Unified-Business-Platform-v0.7.md"
KONFIRMASI = ROOT / "docs" / "Daftar-Konfirmasi-Balans.xlsx"
OUT = ROOT / "docs" / "Lembar-Persetujuan-Baseline-BRD.xlsx"

HEADER_FILL = PatternFill("solid", fgColor="C2410C")
WRAP = Alignment(wrap_text=True, vertical="top")

DIVISI = {
    "Logistik & F&A": ["U-01", "U-04", "U-06", "U-07", "U-08", "U-09", "U-10", "U-11", "U-13", "U-14", "U-15",
                       "U-24", "U-25", "U-26", "U-27"],
    "Sales": ["U-02", "U-03", "U-05", "U-12", "U-16", "U-17", "U-18", "U-19", "U-20", "U-21"],
    "HRGA": ["U-22", "U-23"],
}

K_DIVISI = {"K-09": "Sales", "K-10": "Sales", "K-05": "Sales"}

PARAMETERS = [
    ("Batas nominal approval PO, pengajuan biaya, pemakaian internal, penyesuaian stok, pemusnahan",
     "Sesuai matriks BRD 5.17", "BRD 5.17"),
    ("Toleransi 3-way match", "0% qty, 1% harga", "BRD 5.2"),
    ("Masa berlaku Proforma Invoice", "14 hari", "BRD 5.4"),
    ("Kas kecil", "Plafon Rp5 jt per pemegang, maks. Rp1 jt per transaksi, isi ulang saat saldo < 30% atau mingguan",
     "BRD 5.14"),
    ("Diskon karyawan", "20% dari harga website", "BRD 5.13"),
    ("Batas diskon manual per role", "Ditetapkan di workshop Sales", "BRD 5.18, MD-12"),
    ("Target tutup buku", "Paling lambat hari kerja ke-10 bulan berikutnya", "BRD 5.15"),
    ("Ketentuan selisih konsinyasi default",
     "Hilang/rusak karena partner ditagih; cacat produk/mendekati kedaluwarsa ditanggung Balans", "BRD 5.5"),
    ("Jadwal stock count", "Cycle count gudang berkala; konsinyasi & vending bulanan; penuh akhir tahun", "WMS-11, U-08"),
]

DATA_BALANS = [
    ("Persetujuan BRD v1.0, pilihan opsional, nama approver", "F0-APR, F0-DSN-04", "Wajib exit (diterima)"),
    ("Daftar SKU, bundel, harga dasar/HET, price list yang berlaku", "F0-MIG-01, F0-MIG-02", "Wajib exit"),
    ("Daftar partner MT/konsinyasi beserta PKS", "F0-MIG-01, MD-07", "Wajib exit"),
    ("COA yang dipakai saat ini (jika ada) dan laporan keuangan terakhir", "F0-FIN-01", "Wajib exit"),
    ("Bentuk badan usaha, tahun terdaftar NPWP, status PPh Final UMKM", "F0-FIN-04", "Wajib exit (diterima)"),
    ("Alamat gudang fisik, supplier, dan maklon aktif", "F0-MIG-01", "Wajib exit"),
    ("Pemilihan provider VPS & object storage; akses DNS findyourbalans.com", "F0-TEK-07, F0-EXT-01, F0-EXT-02",
     "Wajib exit (diterima)"),
    ("Kontak vendor vending; pemilik akun Midtrans vending", "F0-EXT-04, F0-EXT-05", "Disarankan"),
    ("Saldo stok per batch, saldo awal akuntansi, AR/AP terbuka, aset tetap", "Cut-over Fase 1",
     "Bukan gate Fase 0"),
]


def brd_proposals() -> list[tuple[str, str, str, str]]:
    """Mengambil (ID, topik, usulan, requirement) U-01 s.d. U-32 dari tabel BRD bagian 12."""
    rows = []
    for line in BRD.read_text(encoding="utf-8").splitlines():
        if not re.match(r"\| U-\d\d \|", line):
            continue
        parts = [c.strip() for c in line.strip().strip("|").split("|")]
        uid, cells = parts[0], parts[1:]
        num = int(uid[2:])
        if num <= 23:  # Ref | Topik | Jawaban | Usulan | Requirement
            topic, proposal, req = cells[1], cells[3], cells[4]
        elif num <= 27:  # Topik | Usulan | Kaitan | Requirement
            topic, proposal, req = cells[0], cells[1], cells[3]
        else:  # Topik | Usulan | Manfaat | Requirement | Diambil?
            topic, proposal, req = cells[0], cells[1], cells[3]
        rows.append((uid, topic, re.sub(r"\*\*", "", proposal), req))
    return rows


def k_statements() -> list[tuple[str, str, str]]:
    ws = load_workbook(KONFIRMASI, data_only=True)["Konfirmasi"]
    out = []
    for row in ws.iter_rows(min_row=2, values_only=True):
        if row[1] and str(row[1]).startswith("K-"):
            out.append((row[1], row[2], row[3]))
    return out


def header(ws, titles: list[str], widths: list[int]) -> None:
    for i, (t, w) in enumerate(zip(titles, widths), start=1):
        cell = ws.cell(row=1, column=i, value=t)
        cell.font = Font(bold=True, color="FFFFFF")
        cell.fill = HEADER_FILL
        cell.alignment = Alignment(wrap_text=True, vertical="center")
        ws.column_dimensions[get_column_letter(i)].width = w
    ws.freeze_panes = "A2"
    ws.row_dimensions[1].height = 30


def dropdown(ws, col: int, choices: list[str], last_row: int) -> None:
    letter = get_column_letter(col)
    dv = DataValidation(type="list", formula1='"' + ",".join(choices) + '"', allow_blank=True)
    dv.add(f"{letter}2:{letter}{last_row}")
    ws.add_data_validation(dv)


def write_rows(ws, rows: list[list]) -> None:
    for r, values in enumerate(rows, start=2):
        for c, v in enumerate(values, start=1):
            ws.cell(row=r, column=c, value=v).alignment = WRAP


def main() -> None:
    proposals = brd_proposals()
    division_of = {uid: d for d, ids in DIVISI.items() for uid in ids}

    wb = Workbook()
    ws = wb.active
    ws.title = "Petunjuk"
    lines = [
        ("Lembar Persetujuan Baseline BRD v0.7 → v1.0", True),
        ("Balans Nutrition — Unified Business Platform · PRD Fase 0 (F0-APR-01 s.d. F0-APR-04)", False),
        ("", False),
        ("1. Isi kolom Status untuk setiap usulan (U) dan pernyataan (K) saat workshop per divisi: Setuju atau Koreksi.", False),
        ("2. Jika Koreksi, tulis isi koreksi yang disepakati. Koreksi dimasukkan ke BRD v1.0.", False),
        ("3. Sheet Opsional: isi Diambil? (Ya/Tidak) untuk U-28 s.d. U-32 dan anggaran biaya (ACC-23).", False),
        ("4. Sheet Parameter: pilih 'Nilai awal disetujui' atau 'Diganti', lalu isi nilai pengganti.", False),
        ("5. Sheet Data Balans: catat status data yang diserahkan dan tanggal yang disepakati.", False),
        ("6. Sheet Ringkasan terhitung otomatis dan dipakai di gate exit Fase 0.", False),
    ]
    for i, (text, bold) in enumerate(lines, start=1):
        ws.cell(row=i, column=1, value=text).font = Font(bold=bold, size=14 if i == 1 else 11)
    ws.column_dimensions["A"].width = 120

    ws = wb.create_sheet("Usulan")
    header(ws, ["ID", "Topik", "Usulan", "Requirement", "Divisi workshop", "Status", "Isi koreksi", "PIC Balans", "Tanggal"],
           [8, 26, 70, 22, 16, 12, 40, 16, 12])
    base = [p for p in proposals if int(p[0][2:]) <= 27]
    write_rows(ws, [[u, t, p, r, division_of.get(u, "Semua"), None, None, None, None] for u, t, p, r in base])
    dropdown(ws, 6, ["Setuju", "Koreksi"], len(base) + 1)

    ws = wb.create_sheet("Konfirmasi")
    header(ws, ["ID", "Topik", "Pernyataan", "Divisi workshop", "Status", "Isi koreksi", "PIC Balans", "Tanggal"],
           [8, 26, 80, 16, 12, 40, 16, 12])
    ks = k_statements()
    write_rows(ws, [[k, t, s, K_DIVISI.get(k, "Logistik & F&A"), None, None, None, None] for k, t, s in ks])
    dropdown(ws, 5, ["Setuju", "Koreksi"], len(ks) + 1)

    ws = wb.create_sheet("Opsional")
    header(ws, ["ID", "Topik", "Usulan", "Requirement", "Rilis jika diambil", "Diambil?", "Catatan", "Tanggal"],
           [8, 26, 70, 18, 22, 10, 40, 12])
    release = {"U-28": "R1.2", "U-29": "R1.2", "U-30": "R1.3", "U-31": "Kuota R2.2; potong gaji R5.2", "U-32": "R2.3"}
    optional = [[u, t, p, r, release.get(u, ""), None, None, None] for u, t, p, r in proposals if int(u[2:]) >= 28]
    optional.append(["U-13 (ACC-23)", "Anggaran biaya per divisi", "Realisasi biaya vs anggaran bulanan dengan peringatan",
                     "ACC-23", "R3.2", None, None, None])
    write_rows(ws, optional)
    dropdown(ws, 6, ["Ya", "Tidak"], len(optional) + 1)

    ws = wb.create_sheet("Parameter")
    header(ws, ["Parameter", "Nilai awal di BRD", "Acuan", "Status", "Nilai pengganti", "PIC Balans", "Tanggal"],
           [50, 60, 16, 22, 40, 16, 12])
    write_rows(ws, [[p, v, a, None, None, None, None] for p, v, a in PARAMETERS])
    dropdown(ws, 4, ["Nilai awal disetujui", "Diganti"], len(PARAMETERS) + 1)

    ws = wb.create_sheet("Data Balans")
    header(ws, ["Data / akses", "Dipakai untuk", "Status gate", "Status", "Tanggal diterima / dijadwalkan", "PIC Balans"],
           [60, 30, 22, 14, 22, 16])
    write_rows(ws, [[d, u, g, None, None, None] for d, u, g in DATA_BALANS])
    dropdown(ws, 4, ["Diterima", "Dijadwalkan", "Belum"], len(DATA_BALANS) + 1)

    ws = wb.create_sheet("Ringkasan")
    header(ws, ["Kelompok", "Total", "Setuju / Ya / Diterima", "Koreksi / Tidak / Dijadwalkan", "Belum diisi"], [34, 10, 22, 26, 14])
    summary = [
        ("Usulan U-01 s.d. U-27", "Usulan", "F", "Setuju", "Koreksi", len(base)),
        ("Pernyataan K-01 s.d. K-16", "Konfirmasi", "E", "Setuju", "Koreksi", len(ks)),
        ("Usulan opsional", "Opsional", "F", "Ya", "Tidak", len(optional)),
        ("Parameter awal", "Parameter", "D", "Nilai awal disetujui", "Diganti", len(PARAMETERS)),
        ("Data dari Balans", "Data Balans", "D", "Diterima", "Dijadwalkan", len(DATA_BALANS)),
    ]
    for r, (label, sheet, col, yes, no, total) in enumerate(summary, start=2):
        rng = f"'{sheet}'!{col}2:{col}{total + 1}"
        ws.cell(row=r, column=1, value=label)
        ws.cell(row=r, column=2, value=total)
        ws.cell(row=r, column=3, value=f'=COUNTIF({rng},"{yes}")')
        ws.cell(row=r, column=4, value=f'=COUNTIF({rng},"{no}")')
        ws.cell(row=r, column=5, value=f"=B{r}-C{r}-D{r}")

    wb.save(OUT)
    print(f"ditulis: {OUT} ({len(base)} usulan, {len(ks)} konfirmasi, {len(optional)} opsional)")


if __name__ == "__main__":
    main()
