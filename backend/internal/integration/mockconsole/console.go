// Package mockconsole adalah halaman simulasi /_mock/* untuk tester di local dan staging (Technical Guideline 5.10).
// Route ini hanya didaftarkan saat minimal satu adapter memakai mock.
package mockconsole

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"balans/internal/integration/mockpayment"
	"balans/internal/integration/mockshipment"
	"balans/internal/logistics"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/sales"
)

//go:embed templates/*.html
var templateFS embed.FS

const listLimit = 50

type emailQueuer interface {
	QueueEmail(ctx context.Context, to, subject, text string) (uuid.UUID, error)
}

// Deps adalah dependensi halaman. Field nil berarti fitur tersebut tidak memakai mock dan disembunyikan.
type Deps struct {
	Payments  *mockpayment.Gateway
	Shipments *mockshipment.Provider
	Email     emailQueuer
	Clock     clock.Clock
	Log       *slog.Logger
}

// Console menyajikan halaman simulasi.
type Console struct {
	d    Deps
	tmpl *template.Template
}

// New membuat console.
func New(d Deps) *Console {
	funcs := template.FuncMap{
		"wib": func(t time.Time) string { return t.In(clock.Jakarta).Format("02 Jan 2006 15.04") },
	}
	return &Console{d: d, tmpl: template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html"))}
}

// Routes mendaftarkan route di bawah /_mock.
func (c *Console) Routes(r chi.Router) {
	r.Get("/", c.index)
	if c.d.Email != nil {
		r.Post("/emails", c.sendTestEmail)
	}
	if c.d.Payments != nil {
		r.Get("/payments", c.listPayments)
		r.Post("/payments", c.createPayment)
		r.Get("/payments/{orderID}", c.showPayment)
		r.Post("/payments/{orderID}/simulate", c.simulatePayment)
	}
	if c.d.Shipments != nil {
		r.Get("/shipments", c.listShipments)
		r.Post("/shipments", c.createShipment)
		r.Post("/shipments/{referenceID}/simulate", c.simulateShipment)
	}
}

type page struct {
	Title      string
	Flash      string
	Error      string
	HasEmail   bool
	HasPayment bool
	HasShip    bool
	Payments   []mockpayment.Payment
	Payment    mockpayment.Payment
	Shipments  []mockshipment.Shipment
	Statuses   []string
}

func (c *Console) render(w http.ResponseWriter, r *http.Request, name string, p page) {
	p.HasEmail = c.d.Email != nil
	p.HasPayment = c.d.Payments != nil
	p.HasShip = c.d.Shipments != nil
	p.Flash = r.URL.Query().Get("flash")
	if p.Error == "" {
		p.Error = r.URL.Query().Get("error")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	if err := c.tmpl.ExecuteTemplate(w, name, p); err != nil {
		c.d.Log.ErrorContext(r.Context(), "render mock page", "template", name, "error", err)
	}
}

func (c *Console) redirect(w http.ResponseWriter, r *http.Request, path string, err error) {
	q := url.Values{}
	if err != nil {
		c.d.Log.WarnContext(r.Context(), "mock action failed", "error", err)
		q.Set("error", apperr.As(err).Message)
	} else {
		q.Set("flash", "Berhasil.")
	}
	http.Redirect(w, r, path+"?"+q.Encode(), http.StatusSeeOther)
}

func (c *Console) index(w http.ResponseWriter, r *http.Request) {
	c.render(w, r, "index.html", page{Title: "Simulasi Pihak Ketiga"})
}

func (c *Console) sendTestEmail(w http.ResponseWriter, r *http.Request) {
	_, err := c.d.Email.QueueEmail(r.Context(), r.FormValue("to"), "Email uji Balans ERP",
		"Email ini dikirim dari halaman simulasi untuk memastikan antrean dan pengiriman email berjalan.")
	c.redirect(w, r, "/_mock/", err)
}

func (c *Console) listPayments(w http.ResponseWriter, r *http.Request) {
	payments, err := c.d.Payments.List(r.Context(), listLimit)
	p := page{Title: "Simulasi Pembayaran", Payments: payments}
	if err != nil {
		p.Error = apperr.As(err).Message
	}
	c.render(w, r, "payments.html", p)
}

func (c *Console) createPayment(w http.ResponseWriter, r *http.Request) {
	amount, err := money.Parse(r.FormValue("grossAmount"))
	if err != nil {
		c.redirect(w, r, "/_mock/payments", apperr.Validation(map[string]string{"grossAmount": "Nominal tidak valid"}))
		return
	}
	orderID := fmt.Sprintf("TEST-%s", c.d.Clock.Now().In(clock.Jakarta).Format("20060102-150405"))
	trx, err := c.d.Payments.CreateTransaction(r.Context(), sales.PaymentRequest{OrderID: orderID, GrossAmount: amount})
	if err != nil {
		c.redirect(w, r, "/_mock/payments", err)
		return
	}
	http.Redirect(w, r, trx.RedirectURL, http.StatusSeeOther)
}

func (c *Console) showPayment(w http.ResponseWriter, r *http.Request) {
	payment, err := c.d.Payments.Get(r.Context(), chi.URLParam(r, "orderID"))
	if err != nil {
		w.WriteHeader(apperr.As(err).Code.HTTPStatus())
		c.render(w, r, "payment.html", page{Title: "Simulasi Pembayaran", Error: apperr.As(err).Message})
		return
	}
	c.render(w, r, "payment.html", page{Title: "Bayar " + payment.OrderID, Payment: payment})
}

func (c *Console) simulatePayment(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderID")
	err := c.d.Payments.Simulate(r.Context(), orderID, mockpayment.Outcome(r.FormValue("outcome")))
	c.redirect(w, r, "/_mock/payments/"+url.PathEscape(orderID), err)
}

func (c *Console) listShipments(w http.ResponseWriter, r *http.Request) {
	shipments, err := c.d.Shipments.List(r.Context(), listLimit)
	p := page{Title: "Simulasi Pengiriman", Shipments: shipments, Statuses: mockshipment.Statuses}
	if err != nil {
		p.Error = apperr.As(err).Message
	}
	c.render(w, r, "shipments.html", p)
}

func (c *Console) createShipment(w http.ResponseWriter, r *http.Request) {
	ref := fmt.Sprintf("TEST-DO-%s", c.d.Clock.Now().In(clock.Jakarta).Format("20060102-150405"))
	_, err := c.d.Shipments.CreateOrder(r.Context(), logistics.ShipmentOrder{
		ReferenceID: ref, CourierCode: "jne", ServiceCode: "reg", WeightGrams: 1000,
	})
	c.redirect(w, r, "/_mock/shipments", err)
}

func (c *Console) simulateShipment(w http.ResponseWriter, r *http.Request) {
	err := c.d.Shipments.Simulate(r.Context(), chi.URLParam(r, "referenceID"), r.FormValue("status"))
	c.redirect(w, r, "/_mock/shipments", err)
}
