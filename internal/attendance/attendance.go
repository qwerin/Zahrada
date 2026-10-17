// Package attendance je doménová logika docházky dodavatelů (SPEC §4.8): validace záznamu,
// překryvy, zaokrouhlení, náklady, schvalovací stavy, uzávěrka a měsíční souhrn.
// Čistá logika bez DB a HTTP.
//
// Čas záznamu je místní čas (Europe/Prague) jako minuty od půlnoci; záznam nepřekračuje půlnoc.
// Díky tomu nehraje roli přechod na letní/zimní čas ani časová zóna serveru.
package attendance

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qwerin/zahrada/internal/calc"
)

// Status je stav záznamu docházky.
type Status string

const (
	StatusDraft     Status = "draft"     // rozpracovaný (dodavatel ještě upravuje)
	StatusSubmitted Status = "submitted" // odeslaný ke schválení
	StatusApproved  Status = "approved"  // schválený vedoucím — vstupuje do souhrnu k proplacení
	StatusRejected  Status = "rejected"  // vrácený k opravě
)

// Entry je jeden záznam docházky (jeden dodavatel, jeden den, jeden úsek práce).
type Entry struct {
	ID           uint
	ContractorID uint
	JobID        uint   // 0 = režie (sklad, servis strojů)
	Date         string // YYYY-MM-DD
	StartMin     int    // minuty od půlnoci, 07:30 = 450
	EndMin       int
	BreakMin     int
	KM           int64 // ujeté km (celé)
	Status       Status

	RateSnapshot   int64 // hodinová sazba dodavatele v době záznamu (haléře/h)
	KMRateSnapshot int64 // sazba za km (haléře/km)
}

var (
	ErrClock   = errors.New("invalid time of day")
	ErrDate    = errors.New("invalid date")
	ErrRange   = errors.New("end must be after start, same day")
	ErrBreak   = errors.New("break must be shorter than the time worked")
	ErrKM      = errors.New("km must not be negative")
	ErrLocked  = errors.New("period is closed")
	ErrOverlap = errors.New("overlaps another entry")
	ErrState   = errors.New("status change not allowed")
)

// ParseClock převede "7:30" / "07:30" na minuty od půlnoci. "24:00" je povolené jako konec dne.
func ParseClock(s string) (int, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok || len(m) != 2 || len(h) == 0 || len(h) > 2 {
		return 0, fmt.Errorf("%w: %q", ErrClock, s)
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || mm < 0 || mm > 59 || hh > 24 || (hh == 24 && mm != 0) {
		return 0, fmt.Errorf("%w: %q", ErrClock, s)
	}
	return hh*60 + mm, nil
}

// FormatClock převede minuty od půlnoci na "HH:MM".
func FormatClock(min int) string { return fmt.Sprintf("%02d:%02d", min/60, min%60) }

// MinuteOfDay vrátí minutu dne okamžiku t v dané zóně (pro „Příchod/Odchod“ z mobilu).
// Binárka musí importovat time/tzdata — distroless image nemá systémovou databázi zón.
func MinuteOfDay(t time.Time, loc *time.Location) (date string, minute int) {
	lt := t.In(loc)
	return lt.Format(time.DateOnly), lt.Hour()*60 + lt.Minute()
}

// Validate zkontroluje záznam sám o sobě (bez ohledu na ostatní záznamy).
func (e Entry) Validate() error {
	if _, err := time.Parse(time.DateOnly, e.Date); err != nil {
		return fmt.Errorf("%w: %q", ErrDate, e.Date)
	}
	if e.StartMin < 0 || e.EndMin > 24*60 || e.EndMin <= e.StartMin {
		return ErrRange
	}
	if e.BreakMin < 0 || e.BreakMin >= e.EndMin-e.StartMin {
		return ErrBreak
	}
	if e.KM < 0 {
		return ErrKM
	}
	return nil
}

// WorkedMinutes je odpracovaná doba bez přestávky a bez zaokrouhlení.
func (e Entry) WorkedMinutes() int { return e.EndMin - e.StartMin - e.BreakMin }

// RoundMode určuje směr zaokrouhlení odpracované doby.
type RoundMode string

const (
	RoundNearest RoundMode = "nearest"
	RoundUp      RoundMode = "up"
	RoundDown    RoundMode = "down"
)

// Rounding je nastavení účtu: krok v minutách (0 nebo 1 = bez zaokrouhlení) a směr.
// Zaokrouhluje se každý záznam zvlášť, ne denní ani měsíční součet (SPEC §4.8).
type Rounding struct {
	StepMin int
	Mode    RoundMode
}

// Apply zaokrouhlí počet minut.
func (r Rounding) Apply(min int) int {
	if r.StepMin <= 1 {
		return min
	}
	q, rem := min/r.StepMin, min%r.StepMin
	switch r.Mode {
	case RoundUp:
		if rem > 0 {
			q++
		}
	case RoundDown:
	default: // nearest, polovina nahoru
		if rem*2 >= r.StepMin {
			q++
		}
	}
	return q * r.StepMin
}

// Overlaps říká, zda se dva záznamy téhož dodavatele ve stejný den časově překrývají.
// Navazující úseky (konec 12:00, začátek 12:00) se nepřekrývají.
func Overlaps(a, b Entry) bool {
	if a.ContractorID != b.ContractorID || a.Date != b.Date || (a.ID != 0 && a.ID == b.ID) {
		return false
	}
	return a.StartMin < b.EndMin && b.StartMin < a.EndMin
}

// FindOverlap vrátí první existující záznam, se kterým se e překrývá (nebo nil).
func FindOverlap(existing []Entry, e Entry) *Entry {
	for i := range existing {
		if Overlaps(existing[i], e) {
			return &existing[i]
		}
	}
	return nil
}

// Cost je cena odpracovaných minut při hodinové sazbě (haléře, zaokrouhleno).
func Cost(minutes int, hourlyRate int64) int64 {
	return calc.MulDivRound(int64(minutes), hourlyRate, 60)
}

// Locked říká, zda datum spadá do uzavřeného období. lockedThrough je poslední uzavřený den
// (nastavení účtu „docházka uzavřena do“); prázdný = nic uzavřeno.
func Locked(date, lockedThrough string) bool {
	return lockedThrough != "" && date <= lockedThrough
}

// Editable říká, zda smí uživatel záznam upravit. Dodavatel jen rozpracovaný nebo vrácený,
// vedoucí (owner/admin/member) cokoli mimo uzavřené období.
func Editable(e Entry, manager bool, lockedThrough string) bool {
	if Locked(e.Date, lockedThrough) {
		return false
	}
	return manager || e.Status == StatusDraft || e.Status == StatusRejected
}

// Transition ověří změnu stavu. manager = role owner/admin/member, jinak dodavatel.
func Transition(from, to Status, manager bool) error {
	ok := false
	switch {
	case from == StatusDraft && to == StatusSubmitted:
		ok = true
	case from == StatusRejected && to == StatusSubmitted:
		ok = true
	case from == StatusSubmitted && to == StatusDraft: // dodavatel si odeslaný záznam vezme zpět
		ok = true
	case from == StatusSubmitted && (to == StatusApproved || to == StatusRejected):
		ok = manager
	case from == StatusDraft && to == StatusApproved: // vedoucí zapisuje rovnou schválený záznam
		ok = manager
	case from == StatusApproved && to == StatusSubmitted: // vedoucí znovu otevře
		ok = manager
	}
	if !ok {
		return fmt.Errorf("%w: %s → %s", ErrState, from, to)
	}
	return nil
}

// JobLine je řádek souhrnu dodavatele za jednu zakázku.
type JobLine struct {
	JobID   uint  `json:"job_id"`
	Minutes int   `json:"minutes"`
	Cost    int64 `json:"cost"`
	KM      int64 `json:"km"`
	KMCost  int64 `json:"km_cost"`
}

// ContractorSummary je souhrn dodavatele za období (podklad k jeho faktuře / proplacení).
type ContractorSummary struct {
	ContractorID uint      `json:"contractor_id"`
	Days         int       `json:"days"`
	Minutes      int       `json:"minutes"`
	Cost         int64     `json:"cost"`
	KM           int64     `json:"km"`
	KMCost       int64     `json:"km_cost"`
	Total        int64     `json:"total"`
	Jobs         []JobLine `json:"jobs"`
}

// Summarize sečte záznamy po dodavatelích a zakázkách. Volající předává jen záznamy, které
// do souhrnu patří (typicky StatusApproved za měsíc). Každý záznam se zaokrouhlí zvlášť
// a ocení svou sazbou v době zápisu.
func Summarize(entries []Entry, r Rounding) []ContractorSummary {
	type key struct{ c, j uint }
	lines := map[key]*JobLine{}
	sums := map[uint]*ContractorSummary{}
	days := map[uint]map[string]bool{}

	for _, e := range entries {
		min := r.Apply(e.WorkedMinutes())
		cost := Cost(min, e.RateSnapshot)
		kmCost := e.KM * e.KMRateSnapshot

		s := sums[e.ContractorID]
		if s == nil {
			s = &ContractorSummary{ContractorID: e.ContractorID, Jobs: []JobLine{}}
			sums[e.ContractorID] = s
			days[e.ContractorID] = map[string]bool{}
		}
		days[e.ContractorID][e.Date] = true
		s.Minutes += min
		s.Cost += cost
		s.KM += e.KM
		s.KMCost += kmCost

		k := key{e.ContractorID, e.JobID}
		l := lines[k]
		if l == nil {
			l = &JobLine{JobID: e.JobID}
			lines[k] = l
		}
		l.Minutes += min
		l.Cost += cost
		l.KM += e.KM
		l.KMCost += kmCost
	}

	out := make([]ContractorSummary, 0, len(sums))
	for id, s := range sums {
		s.Days = len(days[id])
		s.Total = s.Cost + s.KMCost
		for k, l := range lines {
			if k.c == id {
				s.Jobs = append(s.Jobs, *l)
			}
		}
		sort.Slice(s.Jobs, func(a, b int) bool { return s.Jobs[a].JobID < s.Jobs[b].JobID })
		out = append(out, *s)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ContractorID < out[b].ContractorID })
	return out
}
