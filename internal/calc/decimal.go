// Package calc je výpočetní jádro cenových kalkulací (SPEC §4.6): položky, ztratné,
// přirážky, slevy, DPH po sazbách, marže a odhad pracnosti. Čistá logika bez DB a HTTP.
//
// Konvence (SPEC §2): peníze int64 v haléřích, množství int64 v tisícinách (milli),
// procenta v basis points (2100 = 21 %). Nikdy float.
package calc

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// ErrQuantity vrací ParseMilli pro neplatný zápis množství.
var ErrQuantity = errors.New("invalid quantity")

// ParseMilli převede decimální string ("12", "12.5", "12,5", "-0.125") na tisíciny.
// Povoluje desetinnou čárku i tečku, mezery jako oddělovač tisíců, max. 3 desetinná místa.
func ParseMilli(s string) (int64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	s = strings.ReplaceAll(s, "\u00a0", "")
	s = strings.Replace(s, ",", ".", 1)
	if s == "" {
		return 0, fmt.Errorf("%w: empty", ErrQuantity)
	}
	neg := false
	if s[0] == '-' || s[0] == '+' {
		neg = s[0] == '-'
		s = s[1:]
	}
	intPart, frac, hasDot := strings.Cut(s, ".")
	if intPart == "" && frac == "" {
		return 0, fmt.Errorf("%w: %q", ErrQuantity, s)
	}
	if hasDot && frac == "" {
		return 0, fmt.Errorf("%w: trailing dot", ErrQuantity)
	}
	if len(frac) > 3 {
		return 0, fmt.Errorf("%w: max 3 decimal places", ErrQuantity)
	}
	if intPart == "" {
		intPart = "0"
	}
	for _, r := range intPart + frac {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("%w: %q", ErrQuantity, s)
		}
	}
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil || whole > 1_000_000_000_000 {
		return 0, fmt.Errorf("%w: out of range", ErrQuantity)
	}
	frac += strings.Repeat("0", 3-len(frac))
	f, _ := strconv.ParseInt(frac, 10, 64)
	v := whole*1000 + f
	if neg {
		v = -v
	}
	return v, nil
}

// FormatMilli převede tisíciny zpět na decimální string bez zbytečných nul ("12.5", "3").
func FormatMilli(v int64) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	whole, frac := v/1000, v%1000
	if frac == 0 {
		return sign + strconv.FormatInt(whole, 10)
	}
	fs := strings.TrimRight(fmt.Sprintf("%03d", frac), "0")
	return sign + strconv.FormatInt(whole, 10) + "." + fs
}

// MulDivRound spočítá a*b/d zaokrouhleně na celé číslo (polovina od nuly) bez rizika přetečení.
func MulDivRound(a, b, d int64) int64 {
	if d == 0 {
		panic("calc: division by zero")
	}
	n := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	den := big.NewInt(d)
	if den.Sign() < 0 {
		n.Neg(n)
		den.Neg(den)
	}
	q, r := new(big.Int).QuoRem(n, den, new(big.Int))
	// |2r| >= d → zaokrouhlit od nuly
	r2 := new(big.Int).Abs(r)
	r2.Lsh(r2, 1)
	if r2.Cmp(den) >= 0 {
		if n.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	if !q.IsInt64() {
		panic("calc: result overflows int64")
	}
	return q.Int64()
}

// ApplyBps vrátí amount × (10000 + bps) / 10000, zaokrouhleno.
func ApplyBps(amount int64, bps int32) int64 {
	return MulDivRound(amount, 10_000+int64(bps), 10_000)
}

// PartBps vrátí amount × bps / 10000, zaokrouhleno (např. výše slevy nebo DPH).
func PartBps(amount int64, bps int32) int64 {
	return MulDivRound(amount, int64(bps), 10_000)
}
