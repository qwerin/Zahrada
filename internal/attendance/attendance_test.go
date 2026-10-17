package attendance

import (
	"errors"
	"testing"
	"time"
)

func TestParseFormatClock(t *testing.T) {
	ok := map[string]int{"7:30": 450, "07:30": 450, "00:00": 0, "24:00": 1440, "15:45": 945}
	for in, want := range ok {
		if got, err := ParseClock(in); err != nil || got != want {
			t.Errorf("ParseClock(%q) = %d, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "7", "7:5", "25:00", "24:01", "12:60", "ab:cd", "123:00"} {
		if _, err := ParseClock(in); !errors.Is(err, ErrClock) {
			t.Errorf("ParseClock(%q) err = %v", in, err)
		}
	}
	if FormatClock(450) != "07:30" || FormatClock(1440) != "24:00" {
		t.Error("FormatClock")
	}
}

func TestMinuteOfDayAcrossDST(t *testing.T) {
	prague, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		t.Skip("no tzdata")
	}
	// 30. 3. 2026 je už letní čas (UTC+2): 05:30 UTC = 07:30 místně
	d, m := MinuteOfDay(time.Date(2026, 3, 30, 5, 30, 0, 0, time.UTC), prague)
	if d != "2026-03-30" || m != 450 {
		t.Errorf("summer: %s %d", d, m)
	}
	// 2. 11. 2026 zimní čas (UTC+1): 23:30 UTC 1. 11. = 00:30 místně 2. 11.
	d, m = MinuteOfDay(time.Date(2026, 11, 1, 23, 30, 0, 0, time.UTC), prague)
	if d != "2026-11-02" || m != 30 {
		t.Errorf("winter: %s %d", d, m)
	}
}

func TestValidate(t *testing.T) {
	good := Entry{Date: "2026-05-04", StartMin: 450, EndMin: 945, BreakMin: 30}
	if err := good.Validate(); err != nil || good.WorkedMinutes() != 465 {
		t.Fatalf("good: %v %d", err, good.WorkedMinutes())
	}
	cases := []struct {
		e    Entry
		want error
	}{
		{Entry{Date: "2026-02-30", StartMin: 1, EndMin: 2}, ErrDate},
		{Entry{Date: "2026-05-04", StartMin: 600, EndMin: 600}, ErrRange},
		{Entry{Date: "2026-05-04", StartMin: 1200, EndMin: 1500}, ErrRange},
		{Entry{Date: "2026-05-04", StartMin: 600, EndMin: 660, BreakMin: 60}, ErrBreak},
		{Entry{Date: "2026-05-04", StartMin: 600, EndMin: 660, KM: -1}, ErrKM},
	}
	for _, c := range cases {
		if err := c.e.Validate(); !errors.Is(err, c.want) {
			t.Errorf("%+v: %v; want %v", c.e, err, c.want)
		}
	}
}

func TestRounding(t *testing.T) {
	cases := []struct {
		r    Rounding
		in   int
		want int
	}{
		{Rounding{}, 467, 467},
		{Rounding{15, RoundNearest}, 467, 465},
		{Rounding{15, RoundNearest}, 472, 465},
		{Rounding{15, RoundNearest}, 473, 480}, // 8 z 15 → nahoru
		{Rounding{15, RoundUp}, 466, 480},
		{Rounding{15, RoundUp}, 465, 465},
		{Rounding{15, RoundDown}, 479, 465},
		{Rounding{30, RoundNearest}, 465, 480}, // přesně polovina → nahoru
	}
	for _, c := range cases {
		if got := c.r.Apply(c.in); got != c.want {
			t.Errorf("%+v.Apply(%d) = %d; want %d", c.r, c.in, got, c.want)
		}
	}
}

func TestOverlap(t *testing.T) {
	existing := []Entry{
		{ID: 1, ContractorID: 7, Date: "2026-05-04", StartMin: 420, EndMin: 720},
		{ID: 2, ContractorID: 7, Date: "2026-05-04", StartMin: 750, EndMin: 900},
		{ID: 3, ContractorID: 8, Date: "2026-05-04", StartMin: 420, EndMin: 900},
	}
	if FindOverlap(existing, Entry{ContractorID: 7, Date: "2026-05-04", StartMin: 720, EndMin: 750}) != nil {
		t.Error("adjacent slot reported as overlap")
	}
	if o := FindOverlap(existing, Entry{ContractorID: 7, Date: "2026-05-04", StartMin: 700, EndMin: 760}); o == nil || o.ID != 1 {
		t.Errorf("overlap not found: %+v", o)
	}
	if FindOverlap(existing, Entry{ContractorID: 7, Date: "2026-05-05", StartMin: 420, EndMin: 720}) != nil {
		t.Error("other day reported")
	}
	// úprava záznamu 1 se nepřekrývá sama se sebou
	if FindOverlap(existing, Entry{ID: 1, ContractorID: 7, Date: "2026-05-04", StartMin: 430, EndMin: 700}) != nil {
		t.Error("entry overlaps itself")
	}
}

func TestCostLockedEditable(t *testing.T) {
	if got := Cost(465, 35_000); got != 271_250 { // 7 h 45 min × 350 Kč
		t.Errorf("Cost = %d", got)
	}
	if got := Cost(10, 35_000); got != 5_833 { // 58.333 Kč → 58.33
		t.Errorf("Cost = %d", got)
	}
	if !Locked("2026-04-30", "2026-04-30") || Locked("2026-05-01", "2026-04-30") || Locked("2026-01-01", "") {
		t.Error("Locked")
	}
	e := Entry{Date: "2026-05-04", Status: StatusSubmitted}
	if Editable(e, false, "") || !Editable(e, true, "") || Editable(e, true, "2026-05-31") {
		t.Error("Editable submitted")
	}
	e.Status = StatusRejected
	if !Editable(e, false, "") {
		t.Error("contractor cannot fix rejected entry")
	}
}

func TestTransition(t *testing.T) {
	allowed := []struct {
		from, to Status
		manager  bool
	}{
		{StatusDraft, StatusSubmitted, false},
		{StatusSubmitted, StatusDraft, false},
		{StatusRejected, StatusSubmitted, false},
		{StatusSubmitted, StatusApproved, true},
		{StatusSubmitted, StatusRejected, true},
		{StatusDraft, StatusApproved, true},
		{StatusApproved, StatusSubmitted, true},
	}
	for _, c := range allowed {
		if err := Transition(c.from, c.to, c.manager); err != nil {
			t.Errorf("%s→%s manager=%v: %v", c.from, c.to, c.manager, err)
		}
	}
	denied := []struct {
		from, to Status
		manager  bool
	}{
		{StatusSubmitted, StatusApproved, false}, // dodavatel si neschválí sám
		{StatusDraft, StatusApproved, false},
		{StatusApproved, StatusDraft, true},
		{StatusApproved, StatusSubmitted, false},
	}
	for _, c := range denied {
		if err := Transition(c.from, c.to, c.manager); !errors.Is(err, ErrState) {
			t.Errorf("%s→%s manager=%v allowed", c.from, c.to, c.manager)
		}
	}
}

func TestSummarize(t *testing.T) {
	entries := []Entry{
		{ContractorID: 2, JobID: 10, Date: "2026-05-04", StartMin: 420, EndMin: 900, BreakMin: 30, KM: 24, RateSnapshot: 35_000, KMRateSnapshot: 600},
		{ContractorID: 2, JobID: 11, Date: "2026-05-04", StartMin: 900, EndMin: 1000, RateSnapshot: 35_000},
		{ContractorID: 2, JobID: 10, Date: "2026-05-05", StartMin: 420, EndMin: 727, RateSnapshot: 40_000}, // sazba se změnila
		{ContractorID: 1, JobID: 0, Date: "2026-05-05", StartMin: 480, EndMin: 600, RateSnapshot: 30_000},
	}
	got := Summarize(entries, Rounding{StepMin: 15, Mode: RoundNearest})
	if len(got) != 2 || got[0].ContractorID != 1 || got[1].ContractorID != 2 {
		t.Fatalf("order: %+v", got)
	}
	c := got[1]
	// 450 min (7.5 h × 350) + 105 min (100 → 105; 1.75 h × 350) + 300 min (307 → 300; 5 h × 400)
	if c.Days != 2 || c.Minutes != 855 || c.Cost != 262_500+61_250+200_000 || c.KM != 24 || c.KMCost != 14_400 {
		t.Errorf("contractor 2: %+v", c)
	}
	if c.Total != c.Cost+c.KMCost || len(c.Jobs) != 2 || c.Jobs[0].JobID != 10 || c.Jobs[0].Minutes != 750 || c.Jobs[1].Minutes != 105 {
		t.Errorf("contractor 2 jobs: %+v", c.Jobs)
	}
	if got[0].Jobs[0].JobID != 0 || got[0].Cost != 60_000 {
		t.Errorf("contractor 1 (režie): %+v", got[0])
	}
}
