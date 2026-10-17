// Uji beban back-office S-419 (NFR-10): ≥ 40 user ERP aktif bersamaan di VPS 4 core / 8 GB.
// Target PRD Fase 1 bagian 9: halaman list & detail p95 ≤ 2 detik, simpan dokumen p95 ≤ 3 detik.
//
// Setiap virtual user (VU) = satu user back-office: login sekali, lalu berulang membuka halaman sesuai
// permission akunnya (list produk/pelanggan/pemasok/PO/jurnal, Persetujuan Saya, saldo stok, detail) dengan
// jeda baca, dan akun ber-permission jurnal memposting Jurnal Umum kecil (Kas Besar ↔ Bank) sebagai beban tulis.
// Cara menjalankan dan template hasil: Runbook bagian 8.3 – 8.4; pembungkus: deploy/loadtest/run.sh.
//
// Env:
//   BASE_URL     mis. http://staging-api:8080 dari jaringan balans-edge di VPS (wajib; lihat Runbook 8.3 —
//                lewat Caddy staging, basic auth bentrok dengan header Authorization Bearer)
//   USERS        daftar email,email,... akun uji tanpa MFA; VU ke-i memakai USERS[i % n] (wajib)
//   PASSWORD     password semua akun uji (wajib)
//   VUS          jumlah user bersamaan (default 40)
//   DURATION     lama beban penuh (default 10m); RAMP lama naik/turun (default 1m)
//   THINK_MIN/THINK_MAX  jeda antar-aksi dalam detik (default 1 / 5) — pola user membaca layar
//   WRITE        0 = tanpa posting jurnal (default 1)
//   WRITE_RATIO  peluang satu aksi adalah posting jurnal untuk akun ber-permission (default 0.15)
import http from "k6/http";
import { check, fail, group, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";
import exec from "k6/execution";

const BASE_URL = (__ENV.BASE_URL || "").replace(/\/$/, "");
const USERS = (__ENV.USERS || "").split(",").map((s) => s.trim()).filter(Boolean);
const PASSWORD = __ENV.PASSWORD || "";
const VUS = Number(__ENV.VUS || 40);
const DURATION = __ENV.DURATION || "10m";
const RAMP = __ENV.RAMP || "1m";
const THINK_MIN = Number(__ENV.THINK_MIN || 1);
const THINK_MAX = Number(__ENV.THINK_MAX || 5);
const WRITE = __ENV.WRITE !== "0";
const WRITE_RATIO = Number(__ENV.WRITE_RATIO || 0.15);

export const options = {
  scenarios: {
    backoffice: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: RAMP, target: VUS },
        { duration: DURATION, target: VUS },
        { duration: RAMP, target: 0 },
      ],
      gracefulRampDown: "30s",
    },
  },
  thresholds: {
    // NFR-10 (PRD Fase 1 bagian 9).
    "http_req_duration{kind:list}": ["p(95)<2000"],
    "http_req_duration{kind:detail}": ["p(95)<2000"],
    "http_req_duration{kind:save}": ["p(95)<3000"],
    // Login tidak punya target NFR; dibatasi sama dengan simpan dokumen (argon2id sengaja mahal).
    "http_req_duration{kind:login}": ["p(95)<3000"],
    http_req_failed: ["rate<0.01"],
    checks: ["rate>0.99"],
  },
  summaryTrendStats: ["avg", "min", "med", "p(90)", "p(95)", "p(99)", "max"],
  noConnectionReuse: false,
  userAgent: "balans-loadtest/1.0 (k6)",
};

// Tanpa import jslib dari internet agar skrip bisa dijalankan dari jaringan tertutup.
function randomItem(items) {
  return items[Math.floor(Math.random() * items.length)];
}

function uuidv4() {
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    const r = Math.floor(Math.random() * 16);
    return (c === "x" ? r : (r & 0x3) | 0x8).toString(16);
  });
}

const journalsPosted = new Counter("balans_journals_posted");
const pageTime = new Trend("balans_page_ms", true);

function baseHeaders() {
  return { "Content-Type": "application/json", Accept: "application/json" };
}

function url(path) {
  return BASE_URL + path;
}

// Status per VU (k6 menjalankan satu instance modul per VU).
const state = { token: "", expiresAt: 0, email: "", perms: new Set(), accounts: null, sample: {} };

function login() {
  state.email = USERS[(exec.vu.idInTest - 1) % USERS.length];
  const res = http.post(url("/api/v1/auth/login"), JSON.stringify({ email: state.email, password: PASSWORD }), {
    headers: baseHeaders(),
    tags: { kind: "login", name: "POST /auth/login" },
  });
  if (!check(res, { "login 200": (r) => r.status === 200 })) {
    fail(`login ${state.email} gagal: ${res.status} ${res.body && res.body.slice(0, 200)}`);
  }
  const body = res.json();
  state.token = body.accessToken;
  // Login ulang 1 menit sebelum access token (15 menit) kedaluwarsa.
  state.expiresAt = Date.parse(body.expiresAt) - 60_000;
  const me = get("/api/v1/me", "detail", "GET /me");
  state.perms = new Set((me && me.permissions) || []);
}

function authHeaders(extra) {
  return Object.assign(baseHeaders(), { Authorization: `Bearer ${state.token}` }, extra || {});
}

function get(path, kind, name) {
  const res = http.get(url(path), { headers: authHeaders(), tags: { kind, name } });
  pageTime.add(res.timings.duration, { kind });
  const ok = check(res, { [`${kind} 200`]: (r) => r.status === 200 });
  if (!ok) {
    console.warn(`${name} → ${res.status} ${res.body && res.body.slice(0, 200)}`);
    return null;
  }
  return res.json();
}

function post(path, payload, name) {
  const res = http.post(url(path), JSON.stringify(payload), {
    headers: authHeaders({ "Idempotency-Key": uuidv4() }),
    tags: { kind: "save", name },
  });
  pageTime.add(res.timings.duration, { kind: "save" });
  const ok = check(res, { "save 2xx": (r) => r.status >= 200 && r.status < 300 });
  if (!ok) {
    console.warn(`${name} → ${res.status} ${res.body && res.body.slice(0, 300)}`);
    return null;
  }
  return res.json();
}

function firstId(list) {
  const items = list && (Array.isArray(list) ? list : list.items);
  return items && items.length ? items[0].id : null;
}

function todayWIB() {
  return new Date(Date.now() + 7 * 3600_000).toISOString().slice(0, 10);
}

// Aksi = satu halaman ERP (permintaan utama halaman tersebut). perm = permission yang dibutuhkan.
const actions = [
  { perm: null, name: "Persetujuan Saya", run: () => {
      get("/api/v1/approvals/inbox/count", "list", "GET /approvals/inbox/count");
      get("/api/v1/approvals/inbox", "list", "GET /approvals/inbox");
      get("/api/v1/notifications/unread-count", "list", "GET /notifications/unread-count");
    } },
  { perm: "catalog.product.read", name: "Produk", run: () => {
      const list = get("/api/v1/products?page=1&pageSize=25", "list", "GET /products");
      const id = firstId(list);
      if (id) get(`/api/v1/products/${id}`, "detail", "GET /products/{id}");
    } },
  { perm: "catalog.product.read", name: "Cari produk", run: () => {
      get(`/api/v1/products?page=1&pageSize=25&search=${randomItem(["meal", "shake", "MS", "coklat", "a"])}`,
        "list", "GET /products?search");
    } },
  { perm: "party.customer.read", name: "Pelanggan", run: () => {
      get("/api/v1/customers?page=1&pageSize=25", "list", "GET /customers");
    } },
  { perm: "party.supplier.read", name: "Pemasok", run: () => {
      get("/api/v1/suppliers?page=1&pageSize=25", "list", "GET /suppliers");
    } },
  { perm: "party.partner.read", name: "Mitra", run: () => {
      get("/api/v1/partners?page=1&pageSize=25", "list", "GET /partners");
    } },
  { perm: "procurement.purchase_order.read", name: "Purchase Order", run: () => {
      const list = get("/api/v1/purchase-orders?page=1&pageSize=25", "list", "GET /purchase-orders");
      const id = firstId(list);
      if (id) get(`/api/v1/purchase-orders/${id}`, "detail", "GET /purchase-orders/{id}");
    } },
  { perm: "procurement.purchase_request.read", name: "Purchase Request", run: () => {
      get("/api/v1/purchase-requests?page=1&pageSize=25", "list", "GET /purchase-requests");
    } },
  { perm: "inventory.stock.read", name: "Stok per Lokasi", run: () => {
      get("/api/v1/stock-balances?page=1&pageSize=50", "list", "GET /stock-balances");
    } },
  { perm: "inventory.stock.read", name: "Kartu Stok", run: () => {
      const balances = get("/api/v1/stock-balances?page=1&pageSize=50", "list", "GET /stock-balances");
      const items = (balances && balances.items) || [];
      if (!items.length) return;
      const productId = randomItem(items).product.id;
      const to = todayWIB();
      const from = new Date(Date.parse(to) - 90 * 86400_000).toISOString().slice(0, 10);
      get(`/api/v1/stock-card?productId=${productId}&dateFrom=${from}&dateTo=${to}`, "detail", "GET /stock-card");
    } },
  { perm: "warehouse.inbound.read", name: "Inbound", run: () => {
      get("/api/v1/inbounds?page=1&pageSize=25", "list", "GET /inbounds");
    } },
  { perm: "warehouse.stock_transfer.read", name: "Transfer Stok", run: () => {
      get("/api/v1/stock-transfers?page=1&pageSize=25", "list", "GET /stock-transfers");
    } },
  { perm: "warehouse.stock_count.read", name: "Stock Count", run: () => {
      get("/api/v1/stock-counts?page=1&pageSize=25", "list", "GET /stock-counts");
    } },
  { perm: "accounting.journal.read", name: "Jurnal", run: () => {
      const list = get("/api/v1/journal-entries?page=1&pageSize=25", "list", "GET /journal-entries");
      const id = firstId(list);
      if (id) get(`/api/v1/journal-entries/${id}`, "detail", "GET /journal-entries/{id}");
    } },
  { perm: "accounting.account.read", name: "Bagan Akun", run: () => {
      get("/api/v1/accounts", "list", "GET /accounts");
    } },
  { perm: "pricing.price_list.read", name: "Daftar Harga", run: () => {
      get("/api/v1/price-lists?page=1&pageSize=25", "list", "GET /price-lists");
    } },
];

// Posting Jurnal Umum: simpan draft lalu posting (dua permintaan simpan dokumen).
function postJournal() {
  if (!state.accounts) {
    const accounts = get("/api/v1/accounts", "list", "GET /accounts") || [];
    // Dua akun Kas & Bank (11xx) yang boleh diposting manual dan tidak butuh dimensi, mis. 1101 Kas Besar & 1111 Bank.
    state.accounts = accounts
      .filter((a) => a.code.startsWith("11") && a.isPostable && a.allowManualPosting && a.isActive)
      .slice(0, 2);
    if (state.accounts.length < 2) console.warn("posting jurnal dilewati: kurang dari 2 akun Kas & Bank manual");
  }
  if (state.accounts.length < 2) return;
  const amount = `${1000 + Math.floor(Math.random() * 9000)}.00`;
  const [debit, credit] = Math.random() < 0.5 ? state.accounts : [...state.accounts].reverse();
  const draft = post("/api/v1/journal-entries", {
    businessDate: todayWIB(),
    description: `UJI BEBAN k6 ${exec.vu.idInTest}-${exec.vu.iterationInScenario}`,
    lines: [
      { accountId: debit.id, debit: amount, credit: "0.00", memo: "uji beban" },
      { accountId: credit.id, debit: "0.00", credit: amount, memo: "uji beban" },
    ],
  }, "POST /journal-entries");
  if (!draft) return;
  const posted = post(`/api/v1/journal-entries/${draft.id}/post`, { version: draft.version },
    "POST /journal-entries/{id}/post");
  if (posted) journalsPosted.add(1);
}

export function setup() {
  if (!BASE_URL || USERS.length === 0 || !PASSWORD) {
    fail("BASE_URL, USERS, dan PASSWORD wajib diisi");
  }
  const res = http.get(url("/readyz"), { tags: { kind: "health", name: "GET /readyz" } });
  if (res.status !== 200) fail(`/readyz ${res.status}: ${res.body}`);
  console.log(`uji beban ${VUS} VU, ${USERS.length} akun, durasi ${DURATION} (+ ramp ${RAMP}), tulis=${WRITE}`);
}

export default function () {
  if (!state.token || Date.now() > state.expiresAt) {
    group("login", login);
    sleep(THINK_MIN);
  }
  const canWrite = WRITE && state.perms.has("accounting.journal.write") && state.perms.has("accounting.journal.post");
  if (canWrite && Math.random() < WRITE_RATIO) {
    group("Posting Jurnal Umum", postJournal);
  } else {
    const allowed = actions.filter((a) => !a.perm || state.perms.has(a.perm));
    const action = randomItem(allowed);
    group(action.name, action.run);
  }
  sleep(THINK_MIN + Math.random() * (THINK_MAX - THINK_MIN));
}

export function handleSummary(data) {
  const out = __ENV.SUMMARY_DIR || ".";
  const m = data.metrics;
  const p95 = (key) => (m[key] ? Math.round(m[key].values["p(95)"]) : "-");
  const line = [
    `VU=${VUS}`,
    `durasi=${DURATION}`,
    `request=${m.http_reqs ? m.http_reqs.values.count : 0}`,
    `rps=${m.http_reqs ? m.http_reqs.values.rate.toFixed(1) : 0}`,
    `gagal=${m.http_req_failed ? (m.http_req_failed.values.rate * 100).toFixed(2) : 0}%`,
    `p95_list=${p95("http_req_duration{kind:list}")}ms`,
    `p95_detail=${p95("http_req_duration{kind:detail}")}ms`,
    `p95_simpan=${p95("http_req_duration{kind:save}")}ms`,
    `p95_login=${p95("http_req_duration{kind:login}")}ms`,
    `jurnal=${m.balans_journals_posted ? m.balans_journals_posted.values.count : 0}`,
  ].join(" ");
  return {
    stdout: `\nRingkasan uji beban: ${line}\n${textThresholds(data)}\n`,
    [`${out}/summary.json`]: JSON.stringify(data, null, 2),
    [`${out}/summary.txt`]: `${new Date().toISOString()} ${line}\n${textThresholds(data)}\n`,
  };
}

function textThresholds(data) {
  const rows = [];
  for (const [name, metric] of Object.entries(data.metrics)) {
    if (!metric.thresholds) continue;
    for (const [expr, res] of Object.entries(metric.thresholds)) {
      rows.push(`  ${res.ok ? "LULUS" : "GAGAL"}  ${name} ${expr}`);
    }
  }
  return rows.join("\n");
}
