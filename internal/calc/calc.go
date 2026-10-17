package calc

import (
	"fmt"
	"sort"
)

// Kind je druh položky kalkulace.
type Kind string

const (
	KindWork        Kind = "work"        // práce (má normu hodin)
	KindMaterial    Kind = "material"    // materiál, rostliny, substráty (má ztratné)
	KindMachine     Kind = "machine"     // mechanizace (bagr, fréza, štěpkovač)
	KindTransport   Kind = "transport"   // doprava, odvoz (km, fůra)
	KindSubcontract Kind = "subcontract" // subdodávka (závlaha, kamenické práce)
	KindOther       Kind = "other"       // ostatní (likvidace odpadu, poplatky)
)

// Kinds je pevné pořadí druhů pro přehledy.
var Kinds = []Kind{KindWork, KindMaterial, KindMachine, KindTransport, KindSubcontract, KindOther}

// Valid říká, zda je druh známý.
func (k Kind) Valid() bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Item je vstupní položka kalkulace. Ceny jsou za jednotku v haléřích bez DPH.
type Item struct {
	Kind    Kind
	Section string // oddíl kalkulace ("Příprava terénu", "Trávník" …), jen pro mezisoučty

	QuantityMilli int64 // množství v tisícinách (12.5 m² = 12500)
	WasteBps      int32 // ztratné/prořez — navýší účtované i nákupní množství (1000 = +10 %)

	UnitCost  int64  // nákupní cena za jednotku
	UnitPrice *int64 // prodejní cena za jednotku; nil = UnitCost × (1 + MarkupBps)
	MarkupBps int32  // přirážka k nákupní ceně, použije se jen bez UnitPrice

	DiscountBps int32 // sleva na položku
	VATRateBps  int32 // sazba DPH položky

	NormHoursMilli int64 // pracnost: tisíciny hodiny na jednotku (0.05 h/m² = 50)
}

// Options jsou parametry celé kalkulace.
type Options struct {
	VATPayer    bool  // neplátce DPH → všechny sazby 0
	DiscountBps int32 // sleva na celou kalkulaci (rozpočítá se do základů po sazbách)
}

// ItemResult je spočítaná položka.
type ItemResult struct {
	EffectiveQuantityMilli int64 // množství vč. ztratného
	UnitPrice              int64 // použitá prodejní cena za jednotku
	Cost                   int64 // nákladová cena řádku
	Discount               int64 // sleva řádku
	Net                    int64 // prodejní cena řádku po slevě, bez DPH
	HoursMilli             int64 // odhad pracnosti řádku v tisícinách hodiny
	VATRateBps             int32 // účinná sazba (0 u neplátce)
}

// VATLine je rekapitulace jedné sazby DPH (po slevě na kalkulaci).
type VATLine struct {
	RateBps int32 `json:"rate_bps"`
	Base    int64 `json:"base"`
	VAT     int64 `json:"vat"`
}

// Subtotal je mezisoučet (oddíl nebo druh položky) před slevou na kalkulaci.
type Subtotal struct {
	Name string `json:"name"`
	Cost int64  `json:"cost"`
	Net  int64  `json:"net"`
}

// Totals je výsledek kalkulace.
type Totals struct {
	Items         []ItemResult
	Sections      []Subtotal // v pořadí prvního výskytu
	ByKind        []Subtotal // v pořadí Kinds, jen nenulové
	VAT           []VATLine  // od nejvyšší sazby
	Subtotal      int64      // součet řádků po slevách položek
	QuoteDiscount int64      // sleva na kalkulaci
	Net           int64      // základ celkem (bez DPH)
	VATTotal      int64
	Gross         int64 // celkem s DPH
	Cost          int64 // náklady celkem
	Margin        int64 // Net − Cost
	MarginBps     int32 // marže v % z prodejní ceny (0, když Net ≤ 0)
	HoursMilli    int64 // odhad pracnosti celkem
}

// ItemError popisuje neplatnou položku; API ji mapuje na 422 s polem items[i].field.
type ItemError struct {
	Index int
	Field string
	Msg   string
}

func (e *ItemError) Error() string {
	return fmt.Sprintf("items[%d].%s: %s", e.Index, e.Field, e.Msg)
}

func validate(i int, it Item) error {
	bad := func(field, msg string) error { return &ItemError{Index: i, Field: field, Msg: msg} }
	switch {
	case !it.Kind.Valid():
		return bad("kind", "unknown kind")
	case it.QuantityMilli < 0:
		return bad("quantity", "must not be negative")
	case it.WasteBps < 0 || it.WasteBps > 10_000:
		return bad("waste_bps", "must be 0–10000")
	case it.UnitCost < 0:
		return bad("unit_cost", "must not be negative")
	case it.UnitPrice != nil && *it.UnitPrice < 0:
		return bad("unit_price", "must not be negative")
	case it.MarkupBps < 0 || it.MarkupBps > 100_000:
		return bad("markup_bps", "must be 0–100000")
	case it.DiscountBps < 0 || it.DiscountBps > 10_000:
		return bad("discount_bps", "must be 0–10000")
	case it.VATRateBps < 0 || it.VATRateBps > 10_000:
		return bad("vat_rate_bps", "must be 0–10000")
	case it.NormHoursMilli < 0:
		return bad("norm_hours", "must not be negative")
	}
	return nil
}

// Compute spočítá kalkulaci. Pořadí zaokrouhlení je závazné (SPEC §4.6):
// množství vč. ztratného → cena řádku → sleva řádku → základy po sazbách → sleva kalkulace → DPH.
func Compute(items []Item, opt Options) (Totals, error) {
	if opt.DiscountBps < 0 || opt.DiscountBps > 10_000 {
		return Totals{}, &ItemError{Index: -1, Field: "discount_bps", Msg: "must be 0–10000"}
	}
	t := Totals{Items: make([]ItemResult, len(items))}
	bases := map[int32]int64{}
	sectionIdx := map[string]int{}
	kindSum := map[Kind]*Subtotal{}

	for i, it := range items {
		if err := validate(i, it); err != nil {
			return Totals{}, err
		}
		r := ItemResult{VATRateBps: it.VATRateBps}
		if !opt.VATPayer {
			r.VATRateBps = 0
		}
		r.EffectiveQuantityMilli = ApplyBps(it.QuantityMilli, it.WasteBps)
		if it.UnitPrice != nil {
			r.UnitPrice = *it.UnitPrice
		} else {
			r.UnitPrice = ApplyBps(it.UnitCost, it.MarkupBps)
		}
		r.Cost = MulDivRound(r.EffectiveQuantityMilli, it.UnitCost, 1000)
		line := MulDivRound(r.EffectiveQuantityMilli, r.UnitPrice, 1000)
		r.Discount = PartBps(line, it.DiscountBps)
		r.Net = line - r.Discount
		// pracnost se počítá z odvedeného množství, ne z materiálu navíc
		r.HoursMilli = MulDivRound(it.QuantityMilli, it.NormHoursMilli, 1000)
		t.Items[i] = r

		t.Subtotal += r.Net
		t.Cost += r.Cost
		t.HoursMilli += r.HoursMilli
		bases[r.VATRateBps] += r.Net

		if it.Section != "" {
			idx, ok := sectionIdx[it.Section]
			if !ok {
				idx = len(t.Sections)
				sectionIdx[it.Section] = idx
				t.Sections = append(t.Sections, Subtotal{Name: it.Section})
			}
			t.Sections[idx].Cost += r.Cost
			t.Sections[idx].Net += r.Net
		}
		ks := kindSum[it.Kind]
		if ks == nil {
			ks = &Subtotal{Name: string(it.Kind)}
			kindSum[it.Kind] = ks
		}
		ks.Cost += r.Cost
		ks.Net += r.Net
	}

	for _, k := range Kinds {
		if ks := kindSum[k]; ks != nil {
			t.ByKind = append(t.ByKind, *ks)
		}
	}
	if t.Sections == nil {
		t.Sections = []Subtotal{}
	}
	if t.ByKind == nil {
		t.ByKind = []Subtotal{}
	}

	rates := make([]int32, 0, len(bases))
	for r := range bases {
		rates = append(rates, r)
	}
	sort.Slice(rates, func(a, b int) bool { return rates[a] > rates[b] })
	t.VAT = make([]VATLine, 0, len(rates))
	for _, rate := range rates {
		base := bases[rate]
		d := PartBps(base, opt.DiscountBps)
		base -= d
		vat := PartBps(base, rate)
		t.QuoteDiscount += d
		t.Net += base
		t.VATTotal += vat
		t.VAT = append(t.VAT, VATLine{RateBps: rate, Base: base, VAT: vat})
	}
	t.Gross = t.Net + t.VATTotal
	t.Margin = t.Net - t.Cost
	if t.Net > 0 {
		t.MarginBps = int32(MulDivRound(t.Margin, 10_000, t.Net))
	}
	return t, nil
}

// WorkUnitPrice spočítá cenu práce za jednotku z normy a hodinové sazby
// (0.05 h/m² × 450 Kč/h = 22.50 Kč/m²). Pro ceník prací (SPEC §4.5).
func WorkUnitPrice(normHoursMilli, hourlyRate int64) int64 {
	return MulDivRound(normHoursMilli, hourlyRate, 1000)
}

// Expand rozbalí sestavu (receptura na 1 jednotku, např. „Trávník výsevem / m²“) na položky
// pro dané množství. Množství komponent jsou v sestavě uvedena na 1 jednotku sestavy.
func Expand(components []Item, quantityMilli int64) []Item {
	out := make([]Item, len(components))
	for i, c := range components {
		c.QuantityMilli = MulDivRound(c.QuantityMilli, quantityMilli, 1000)
		out[i] = c
	}
	return out
}
