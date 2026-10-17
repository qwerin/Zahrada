// Package recurrence generuje termíny pravidelné údržby (SPEC §4.7): „každý týden v úterý
// od dubna do října“, „každých 14 dní v pondělí a čtvrtek“, „v červnu a září druhou středu“.
// Zohledňuje české státní svátky. Čistá logika bez DB.
package recurrence

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Freq je typ opakování.
type Freq string

const (
	Weekly  Freq = "weekly"  // každých N týdnů ve zvolené dny, volitelně jen v sezóně
	Monthly Freq = "monthly" // ve zvolených měsících N-tý den v týdnu (1.–4. nebo poslední)
)

// HolidayPolicy říká, co s termínem připadajícím na státní svátek.
type HolidayPolicy string

const (
	HolidayKeep        HolidayPolicy = "keep"         // nechat (výchozí)
	HolidaySkip        HolidayPolicy = "skip"         // vynechat
	HolidayNextWorkday HolidayPolicy = "next_workday" // posunout na nejbližší další pracovní den (po–pá)
)

// MaxSpanDays omezuje délku generovaného období (ochrana proti neúmyslně obřím dotazům).
const MaxSpanDays = 731

// Rule je pravidlo opakování úkonu údržby.
type Rule struct {
	Freq  Freq
	Start string // YYYY-MM-DD, první možný termín; kotva pro počítání týdnů
	Until string // YYYY-MM-DD včetně, "" = bez konce

	IntervalWeeks int            // Weekly: 1 = každý týden, 2 = každých 14 dní …
	Weekdays      []time.Weekday // Weekly: jeden či více dní; Monthly: právě jeden
	Months        []time.Month   // Weekly: sezóna (prázdné = celý rok); Monthly: povinné
	WeekOfMonth   int            // Monthly: 1–4, -1 = poslední

	Holidays HolidayPolicy // "" = keep
}

var (
	ErrRule  = errors.New("invalid recurrence rule")
	ErrRange = errors.New("invalid date range")
)

func parseDate(s string) (time.Time, error) {
	return time.Parse(time.DateOnly, s) // UTC půlnoc — žádné potíže se změnou času
}

func fmtDate(t time.Time) string { return t.Format(time.DateOnly) }

// Validate zkontroluje pravidlo.
func (r Rule) Validate() error {
	bad := func(msg string) error { return fmt.Errorf("%w: %s", ErrRule, msg) }
	start, err := parseDate(r.Start)
	if err != nil {
		return bad("start must be YYYY-MM-DD")
	}
	if r.Until != "" {
		until, err := parseDate(r.Until)
		if err != nil || until.Before(start) {
			return bad("until must be a date on or after start")
		}
	}
	for _, m := range r.Months {
		if m < time.January || m > time.December {
			return bad("months must be 1–12")
		}
	}
	for _, d := range r.Weekdays {
		if d < time.Sunday || d > time.Saturday {
			return bad("weekdays must be 0–6 (0 = neděle)")
		}
	}
	switch r.Holidays {
	case "", HolidayKeep, HolidaySkip, HolidayNextWorkday:
	default:
		return bad("unknown holiday policy")
	}
	switch r.Freq {
	case Weekly:
		if r.IntervalWeeks < 1 || r.IntervalWeeks > 52 {
			return bad("interval_weeks must be 1–52")
		}
		if len(r.Weekdays) == 0 {
			return bad("weekly rule needs at least one weekday")
		}
	case Monthly:
		if len(r.Months) == 0 {
			return bad("monthly rule needs months")
		}
		if len(r.Weekdays) != 1 {
			return bad("monthly rule needs exactly one weekday")
		}
		if r.WeekOfMonth != -1 && (r.WeekOfMonth < 1 || r.WeekOfMonth > 4) {
			return bad("week_of_month must be 1–4 or -1")
		}
	default:
		return bad("freq must be weekly or monthly")
	}
	return nil
}

// Occurrences vrátí termíny v intervalu [from, to] (YYYY-MM-DD, včetně), seřazené a bez duplicit.
// Termín posunutý kvůli svátku se vrací, i když po posunu padne za `to` (patří k výskytu v období).
func (r Rule) Occurrences(from, to string) ([]string, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	f, err1 := parseDate(from)
	t, err2 := parseDate(to)
	if err1 != nil || err2 != nil || t.Before(f) || t.Sub(f) > MaxSpanDays*24*time.Hour {
		return nil, fmt.Errorf("%w: from/to must be dates, from ≤ to, at most %d days", ErrRange, MaxSpanDays)
	}
	start, _ := parseDate(r.Start)
	if f.Before(start) {
		f = start
	}
	if r.Until != "" {
		until, _ := parseDate(r.Until)
		if until.Before(t) {
			t = until
		}
	}

	var raw []time.Time
	switch r.Freq {
	case Weekly:
		raw = r.weekly(start, f, t)
	case Monthly:
		raw = r.monthly(f, t)
	}

	seen := map[string]bool{}
	out := []string{}
	for _, d := range raw {
		if IsCZHoliday(d) {
			switch r.Holidays {
			case HolidaySkip:
				continue
			case HolidayNextWorkday:
				d = NextWorkday(d)
			}
		}
		s := fmtDate(d)
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out, nil
}

func mondayOf(d time.Time) time.Time {
	offset := (int(d.Weekday()) + 6) % 7 // po = 0 … ne = 6
	return d.AddDate(0, 0, -offset)
}

func (r Rule) inSeason(m time.Month) bool {
	if len(r.Months) == 0 {
		return true
	}
	for _, x := range r.Months {
		if x == m {
			return true
		}
	}
	return false
}

func (r Rule) weekly(start, from, to time.Time) []time.Time {
	anchor := mondayOf(start)
	days := map[time.Weekday]bool{}
	for _, d := range r.Weekdays {
		days[d] = true
	}
	var out []time.Time
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if !days[d.Weekday()] || !r.inSeason(d.Month()) {
			continue
		}
		weeks := int(mondayOf(d).Sub(anchor).Hours()/24) / 7
		if weeks%r.IntervalWeeks == 0 {
			out = append(out, d)
		}
	}
	return out
}

func (r Rule) monthly(from, to time.Time) []time.Time {
	wd := r.Weekdays[0]
	var out []time.Time
	for m := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC); !m.After(to); m = m.AddDate(0, 1, 0) {
		if !r.inSeason(m.Month()) {
			continue
		}
		d := nthWeekday(m, wd, r.WeekOfMonth)
		if !d.Before(from) && !d.After(to) {
			out = append(out, d)
		}
	}
	return out
}

// nthWeekday vrátí n-tý (1–4) nebo poslední (-1) daný den v týdnu v měsíci, kde first je 1. den měsíce.
func nthWeekday(first time.Time, wd time.Weekday, n int) time.Time {
	if n == -1 {
		last := first.AddDate(0, 1, -1)
		back := (int(last.Weekday()) - int(wd) + 7) % 7
		return last.AddDate(0, 0, -back)
	}
	fwd := (int(wd) - int(first.Weekday()) + 7) % 7
	return first.AddDate(0, 0, fwd+7*(n-1))
}

// IsCZHoliday říká, zda je den státním svátkem nebo dnem pracovního klidu v ČR.
func IsCZHoliday(d time.Time) bool {
	type md struct {
		m time.Month
		d int
	}
	fixed := map[md]bool{
		{time.January, 1}: true, {time.May, 1}: true, {time.May, 8}: true, {time.July, 5}: true,
		{time.July, 6}: true, {time.September, 28}: true, {time.October, 28}: true, {time.November, 17}: true,
		{time.December, 24}: true, {time.December, 25}: true, {time.December, 26}: true,
	}
	if fixed[md{d.Month(), d.Day()}] {
		return true
	}
	easter := EasterSunday(d.Year())
	y, m, day := d.Date()
	date := time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
	return date.Equal(easter.AddDate(0, 0, -2)) || date.Equal(easter.AddDate(0, 0, 1)) // Velký pátek, Velikonoční pondělí
}

// EasterSunday spočítá Velikonoční neděli (gregoriánský kalendář, anonymní algoritmus).
func EasterSunday(year int) time.Time {
	a := year % 19
	b, c := year/100, year%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

// NextWorkday vrátí nejbližší následující den po–pá, který není svátkem.
func NextWorkday(d time.Time) time.Time {
	for {
		d = d.AddDate(0, 0, 1)
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday && !IsCZHoliday(d) {
			return d
		}
	}
}
