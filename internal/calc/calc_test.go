package calc

import (
	"errors"
	"testing"
)

func p(v int64) *int64 { return &v }

func TestParseFormatMilli(t *testing.T) {
	ok := map[string]int64{
		"12": 12000, "12.5": 12500, "12,5": 12500, "0.125": 125, ".5": 500,
		"-3.25": -3250, "1 250,75": 1250750, "+7": 7000, "0": 0,
	}
	for in, want := range ok {
		got, err := ParseMilli(in)
		if err != nil || got != want {
			t.Errorf("ParseMilli(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "1.2345", "1.", "1.2.3", "-", "1e3"} {
		if _, err := ParseMilli(in); !errors.Is(err, ErrQuantity) {
			t.Errorf("ParseMilli(%q) err = %v; want ErrQuantity", in, err)
		}
	}
	fmts := map[int64]string{12500: "12.5", 3000: "3", 125: "0.125", -3250: "-3.25", 0: "0", 1010: "1.01"}
	for in, want := range fmts {
		if got := FormatMilli(in); got != want {
			t.Errorf("FormatMilli(%d) = %q; want %q", in, got, want)
		}
	}
}

func TestMulDivRound(t *testing.T) {
	cases := []struct{ a, b, d, want int64 }{
		{5, 1, 2, 3}, {-5, 1, 2, -3}, {7, 1, 3, 2}, {-7, 1, 3, -2},
		{808050, 300, 10000, 24242}, {1531875, 300, 10000, 45956},
		{9_000_000_000, 1_000_000_000, 1_000_000_000, 9_000_000_000}, // mezivýsledek přeteče int64
	}
	for _, c := range cases {
		if got := MulDivRound(c.a, c.b, c.d); got != c.want {
			t.Errorf("MulDivRound(%d,%d,%d) = %d; want %d", c.a, c.b, c.d, got, c.want)
		}
	}
}

// Realizace: frézování, osivo se ztratným a přirážkou, substrát s pevnou cenou a slevou
// v nižší sazbě, doprava. Čísla jsou ručně ověřená (viz SPEC §4.6, příklad).
func sampleItems() []Item {
	return []Item{
		{Kind: KindWork, Section: "Příprava terénu", QuantityMilli: 250_000, UnitCost: 600, UnitPrice: p(1500), VATRateBps: 2100, NormHoursMilli: 20},
		{Kind: KindMaterial, Section: "Trávník", QuantityMilli: 7_500, WasteBps: 1000, UnitCost: 18_000, MarkupBps: 3000, VATRateBps: 2100},
		{Kind: KindMaterial, Section: "Trávník", QuantityMilli: 12_500, UnitCost: 95_000, UnitPrice: p(129_000), DiscountBps: 500, VATRateBps: 1200},
		{Kind: KindTransport, QuantityMilli: 2_000, UnitCost: 120_000, VATRateBps: 2100},
	}
}

func TestComputeVATPayer(t *testing.T) {
	got, err := Compute(sampleItems(), Options{VATPayer: true, DiscountBps: 300})
	if err != nil {
		t.Fatal(err)
	}
	it := got.Items
	if it[1].EffectiveQuantityMilli != 8250 || it[1].UnitPrice != 23_400 || it[1].Cost != 148_500 || it[1].Net != 193_050 {
		t.Errorf("osivo: %+v", it[1])
	}
	if it[2].Discount != 80_625 || it[2].Net != 1_531_875 {
		t.Errorf("substrát: %+v", it[2])
	}
	if it[0].HoursMilli != 5000 || got.HoursMilli != 5000 {
		t.Errorf("hodiny: item %d, total %d", it[0].HoursMilli, got.HoursMilli)
	}
	want := Totals{Subtotal: 2_339_925, QuoteDiscount: 70_198, Net: 2_269_727, VATTotal: 342_910,
		Gross: 2_612_637, Cost: 1_726_000, Margin: 543_727, MarginBps: 2396}
	if got.Subtotal != want.Subtotal || got.QuoteDiscount != want.QuoteDiscount || got.Net != want.Net ||
		got.VATTotal != want.VATTotal || got.Gross != want.Gross || got.Cost != want.Cost ||
		got.Margin != want.Margin || got.MarginBps != want.MarginBps {
		t.Errorf("totals:\n got %+v\nwant %+v", got, want)
	}
	if len(got.VAT) != 2 || got.VAT[0] != (VATLine{2100, 783_808, 164_600}) || got.VAT[1] != (VATLine{1200, 1_485_919, 178_310}) {
		t.Errorf("VAT: %+v", got.VAT)
	}
	if len(got.Sections) != 2 || got.Sections[0].Name != "Příprava terénu" || got.Sections[1].Net != 193_050+1_531_875 {
		t.Errorf("sections: %+v", got.Sections)
	}
	if len(got.ByKind) != 3 || got.ByKind[0].Name != "work" || got.ByKind[2].Name != "transport" {
		t.Errorf("by kind: %+v", got.ByKind)
	}
}

func TestComputeNonPayer(t *testing.T) {
	got, err := Compute(sampleItems(), Options{VATPayer: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.VAT) != 1 || got.VAT[0].RateBps != 0 || got.VATTotal != 0 || got.Gross != got.Net || got.Net != 2_339_925 {
		t.Errorf("non-payer: %+v", got)
	}
	for i, r := range got.Items {
		if r.VATRateBps != 0 {
			t.Errorf("item %d rate %d", i, r.VATRateBps)
		}
	}
}

func TestComputeEmpty(t *testing.T) {
	got, err := Compute(nil, Options{VATPayer: true})
	if err != nil || got.Net != 0 || got.MarginBps != 0 || got.VAT == nil || got.Sections == nil || got.ByKind == nil {
		t.Errorf("empty: %+v, %v", got, err)
	}
}

func TestComputeValidation(t *testing.T) {
	bad := []struct {
		item  Item
		field string
	}{
		{Item{Kind: "tree"}, "kind"},
		{Item{Kind: KindWork, QuantityMilli: -1}, "quantity"},
		{Item{Kind: KindMaterial, WasteBps: 10_001}, "waste_bps"},
		{Item{Kind: KindMaterial, UnitPrice: p(-1)}, "unit_price"},
		{Item{Kind: KindMaterial, DiscountBps: -5}, "discount_bps"},
	}
	for _, c := range bad {
		_, err := Compute([]Item{{Kind: KindOther}, c.item}, Options{})
		var ie *ItemError
		if !errors.As(err, &ie) || ie.Index != 1 || ie.Field != c.field {
			t.Errorf("%+v: err %v; want field %s", c.item, err, c.field)
		}
	}
	if _, err := Compute(nil, Options{DiscountBps: 20_000}); err == nil {
		t.Error("quote discount 200 % accepted")
	}
}

func TestWorkUnitPriceAndExpand(t *testing.T) {
	// 0.05 h/m² × 450 Kč/h = 22.50 Kč/m²
	if got := WorkUnitPrice(50, 45_000); got != 2_250 {
		t.Errorf("WorkUnitPrice = %d", got)
	}
	recipe := []Item{ // „Trávník výsevem“ na 1 m²
		{Kind: KindWork, QuantityMilli: 1000, NormHoursMilli: 20},
		{Kind: KindMaterial, QuantityMilli: 30, WasteBps: 1000}, // 0.03 kg osiva
		{Kind: KindMaterial, QuantityMilli: 50},                 // 0.05 m³ substrátu
	}
	got := Expand(recipe, 250_000)
	if got[0].QuantityMilli != 250_000 || got[1].QuantityMilli != 7_500 || got[2].QuantityMilli != 12_500 || got[1].WasteBps != 1000 {
		t.Errorf("Expand: %+v", got)
	}
	if recipe[1].QuantityMilli != 30 {
		t.Error("Expand mutated the recipe")
	}
}
