package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"github.com/qwerin/zahrada/internal/auth"
	"github.com/qwerin/zahrada/internal/calc"
)

// CalcItem is one input line of a calculation (SPEC §4.6). Money in haléře, quantities as decimal strings.
type CalcItem struct {
	Kind        string `json:"kind" enum:"work,material,machine,transport,subcontract,other"`
	Section     string `json:"section,omitempty" maxLength:"200"`
	Quantity    string `json:"quantity" doc:"Decimal string, max 3 decimals; comma accepted" example:"12.5"`
	WasteBps    int32  `json:"waste_bps,omitempty" minimum:"0" maximum:"10000" doc:"Waste allowance, raises billed and purchased quantity"`
	UnitCost    int64  `json:"unit_cost,omitempty" minimum:"0" doc:"Purchase price per unit"`
	UnitPrice   *int64 `json:"unit_price,omitempty" minimum:"0" doc:"Sale price per unit; omitted = unit_cost + markup"`
	MarkupBps   int32  `json:"markup_bps,omitempty" minimum:"0" maximum:"100000"`
	DiscountBps int32  `json:"discount_bps,omitempty" minimum:"0" maximum:"10000"`
	VatRateBps  int32  `json:"vat_rate_bps" minimum:"0" maximum:"10000"`
	NormHours   string `json:"norm_hours,omitempty" doc:"Labour hours per unit as a decimal string" example:"0.05"`
}

type CalcPreviewInput struct {
	DiscountBps int32      `json:"discount_bps,omitempty" minimum:"0" maximum:"10000" doc:"Discount on the whole calculation"`
	Items       []CalcItem `json:"items" maxItems:"500" nullable:"false"`
}

type CalcItemResult struct {
	EffectiveQuantity string `json:"effective_quantity" doc:"Quantity including waste"`
	UnitPrice         int64  `json:"unit_price"`
	Cost              int64  `json:"cost"`
	Discount          int64  `json:"discount"`
	Net               int64  `json:"net"`
	Hours             string `json:"hours"`
	VatRateBps        int32  `json:"vat_rate_bps" doc:"Effective rate (0 for a non-VAT payer)"`
}

type CalcResult struct {
	VatPayer      bool             `json:"vat_payer" doc:"From the account settings"`
	Items         []CalcItemResult `json:"items" nullable:"false"`
	Sections      []calc.Subtotal  `json:"sections" nullable:"false"`
	ByKind        []calc.Subtotal  `json:"by_kind" nullable:"false"`
	VAT           []calc.VATLine   `json:"vat" nullable:"false"`
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

func (s *server) registerCalc(g huma.API) {
	// POST without side effects; contractors never see prices → editors only
	huma.Post(g, "/calc/preview", s.calcPreview, auth.ForEditors)
}

// toCalcItems converts API lines to calc.Item; errors are 422 on items[i].field.
func toCalcItems(in []CalcItem) ([]calc.Item, error) {
	items := make([]calc.Item, len(in))
	for i, it := range in {
		q, err := calc.ParseMilli(it.Quantity)
		if err != nil {
			return nil, invalid(fmt.Sprintf("items[%d].quantity", i), "quantity must be a number with at most 3 decimals")
		}
		var norm int64
		if it.NormHours != "" {
			if norm, err = calc.ParseMilli(it.NormHours); err != nil {
				return nil, invalid(fmt.Sprintf("items[%d].norm_hours", i), "norm_hours must be a number with at most 3 decimals")
			}
		}
		items[i] = calc.Item{
			Kind: calc.Kind(it.Kind), Section: it.Section, QuantityMilli: q, WasteBps: it.WasteBps,
			UnitCost: it.UnitCost, UnitPrice: it.UnitPrice, MarkupBps: it.MarkupBps,
			DiscountBps: it.DiscountBps, VATRateBps: it.VatRateBps, NormHoursMilli: norm,
		}
	}
	return items, nil
}

// computeErr maps calc errors to 422.
func computeErr(err error) error {
	var ie *calc.ItemError
	if errors.As(err, &ie) {
		if ie.Index < 0 {
			return invalid(ie.Field, ie.Msg)
		}
		return invalid(fmt.Sprintf("items[%d].%s", ie.Index, ie.Field), ie.Msg)
	}
	return err
}

func toCalcResult(t calc.Totals, vatPayer bool) CalcResult {
	out := CalcResult{
		VatPayer: vatPayer, Items: make([]CalcItemResult, len(t.Items)), Sections: t.Sections, ByKind: t.ByKind, VAT: t.VAT,
		Subtotal: t.Subtotal, QuoteDiscount: t.QuoteDiscount, Net: t.Net, VATTotal: t.VATTotal, Gross: t.Gross,
		Cost: t.Cost, Margin: t.Margin, MarginBps: t.MarginBps, Hours: calc.FormatMilli(t.HoursMilli),
	}
	for i, r := range t.Items {
		out.Items[i] = CalcItemResult{
			EffectiveQuantity: calc.FormatMilli(r.EffectiveQuantityMilli), UnitPrice: r.UnitPrice, Cost: r.Cost,
			Discount: r.Discount, Net: r.Net, Hours: calc.FormatMilli(r.HoursMilli), VatRateBps: r.VATRateBps,
		}
	}
	return out
}

func (s *server) calcPreview(ctx context.Context, in *struct{ Body CalcPreviewInput }) (*Out[CalcResult], error) {
	items, err := toCalcItems(in.Body.Items)
	if err != nil {
		return nil, err
	}
	vatPayer := auth.AccountFrom(ctx).VatPayer
	t, err := calc.Compute(items, calc.Options{VATPayer: vatPayer, DiscountBps: in.Body.DiscountBps})
	if err != nil {
		return nil, computeErr(err)
	}
	return &Out[CalcResult]{Body: toCalcResult(t, vatPayer)}, nil
}
