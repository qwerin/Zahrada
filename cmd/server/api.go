package main

// Zaváděcí API (fáze B): jen health a náhled kalkulace, aby šlo od prvního commitu ověřit
// celý řetězec CI → GHCR → Portainer → Traefik. Ve fázi F0 ho nahradí huma API podle SPEC.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/qwerin/zahrada/internal/calc"
)

type problem struct {
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
	Code   string `json:"code"`
	Field  string `json:"field,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeProblem(w http.ResponseWriter, p problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version})
}

// ── POST /api/calc/preview ────────────────────────────────────────────────────

type previewItem struct {
	Kind        calc.Kind `json:"kind"`
	Section     string    `json:"section,omitempty"`
	Quantity    string    `json:"quantity"`
	WasteBps    int32     `json:"waste_bps,omitempty"`
	UnitCost    int64     `json:"unit_cost,omitempty"`
	UnitPrice   *int64    `json:"unit_price,omitempty"`
	MarkupBps   int32     `json:"markup_bps,omitempty"`
	DiscountBps int32     `json:"discount_bps,omitempty"`
	VATRateBps  int32     `json:"vat_rate_bps"`
	NormHours   string    `json:"norm_hours,omitempty"`
}

type previewIn struct {
	VATPayer    bool          `json:"vat_payer"`
	DiscountBps int32         `json:"discount_bps,omitempty"`
	Items       []previewItem `json:"items"`
}

type previewItemOut struct {
	EffectiveQuantity string `json:"effective_quantity"`
	UnitPrice         int64  `json:"unit_price"`
	Cost              int64  `json:"cost"`
	Discount          int64  `json:"discount"`
	Net               int64  `json:"net"`
	Hours             string `json:"hours"`
	VATRateBps        int32  `json:"vat_rate_bps"`
}

type previewOut struct {
	Items         []previewItemOut `json:"items"`
	Sections      []calc.Subtotal  `json:"sections"`
	ByKind        []calc.Subtotal  `json:"by_kind"`
	VAT           []calc.VATLine   `json:"vat"`
	Subtotal      int64            `json:"subtotal"`
	QuoteDiscount int64            `json:"quote_discount"`
	Net           int64            `json:"net"`
	VATTotal      int64            `json:"vat_total"`
	Gross         int64            `json:"gross"`
	Cost          int64            `json:"cost"`
	Margin        int64            `json:"margin"`
	MarginBps     int32            `json:"margin_bps"`
	Hours         string           `json:"hours"`
}

const maxPreviewItems = 500

func invalid(w http.ResponseWriter, field, detail string) {
	writeProblem(w, problem{Title: "Unprocessable Entity", Status: http.StatusUnprocessableEntity,
		Code: "validation_failed", Field: field, Detail: detail})
}

func calcPreviewHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var in previewIn
	if err := dec.Decode(&in); err != nil {
		writeProblem(w, problem{Title: "Bad Request", Status: http.StatusBadRequest, Code: "bad_request", Detail: err.Error()})
		return
	}
	if len(in.Items) > maxPreviewItems {
		invalid(w, "items", "too many items")
		return
	}

	items := make([]calc.Item, len(in.Items))
	for i, it := range in.Items {
		q, err := calc.ParseMilli(it.Quantity)
		if err != nil {
			invalid(w, fieldName(i, "quantity"), err.Error())
			return
		}
		var norm int64
		if it.NormHours != "" {
			if norm, err = calc.ParseMilli(it.NormHours); err != nil {
				invalid(w, fieldName(i, "norm_hours"), err.Error())
				return
			}
		}
		items[i] = calc.Item{
			Kind: it.Kind, Section: it.Section, QuantityMilli: q, WasteBps: it.WasteBps,
			UnitCost: it.UnitCost, UnitPrice: it.UnitPrice, MarkupBps: it.MarkupBps,
			DiscountBps: it.DiscountBps, VATRateBps: it.VATRateBps, NormHoursMilli: norm,
		}
	}

	t, err := calc.Compute(items, calc.Options{VATPayer: in.VATPayer, DiscountBps: in.DiscountBps})
	var ie *calc.ItemError
	if errors.As(err, &ie) {
		field := ie.Field
		if ie.Index >= 0 {
			field = fieldName(ie.Index, ie.Field)
		}
		invalid(w, field, ie.Msg)
		return
	} else if err != nil {
		invalid(w, "", err.Error())
		return
	}

	out := previewOut{
		Items: make([]previewItemOut, len(t.Items)), Sections: t.Sections, ByKind: t.ByKind, VAT: t.VAT,
		Subtotal: t.Subtotal, QuoteDiscount: t.QuoteDiscount, Net: t.Net, VATTotal: t.VATTotal,
		Gross: t.Gross, Cost: t.Cost, Margin: t.Margin, MarginBps: t.MarginBps, Hours: calc.FormatMilli(t.HoursMilli),
	}
	for i, r := range t.Items {
		out.Items[i] = previewItemOut{
			EffectiveQuantity: calc.FormatMilli(r.EffectiveQuantityMilli), UnitPrice: r.UnitPrice, Cost: r.Cost,
			Discount: r.Discount, Net: r.Net, Hours: calc.FormatMilli(r.HoursMilli), VATRateBps: r.VATRateBps,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func fieldName(i int, f string) string {
	return "items[" + strconv.Itoa(i) + "]." + f
}
