package recurrence

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestOccurrences(t *testing.T) {
	cases := []struct {
		name     string
		rule     Rule
		from, to string
		want     []string
	}{
		{
			name: "sečení každé úterý v sezóně duben–říjen",
			rule: Rule{Freq: Weekly, Start: "2026-01-01", IntervalWeeks: 1, Weekdays: []time.Weekday{time.Tuesday},
				Months: []time.Month{4, 5, 6, 7, 8, 9, 10}},
			from: "2026-03-25", to: "2026-04-30",
			want: []string{"2026-04-07", "2026-04-14", "2026-04-21", "2026-04-28"}, // 31. 3. je mimo sezónu
		},
		{
			name: "každých 14 dní po+čt, Velikonoční pondělí posunout",
			rule: Rule{Freq: Weekly, Start: "2026-04-06", IntervalWeeks: 2,
				Weekdays: []time.Weekday{time.Monday, time.Thursday}, Holidays: HolidayNextWorkday},
			from: "2026-04-01", to: "2026-04-30",
			want: []string{"2026-04-07", "2026-04-09", "2026-04-20", "2026-04-23"},
		},
		{
			name: "totéž, svátek vynechat",
			rule: Rule{Freq: Weekly, Start: "2026-04-06", IntervalWeeks: 2,
				Weekdays: []time.Weekday{time.Monday, time.Thursday}, Holidays: HolidaySkip},
			from: "2026-04-01", to: "2026-04-30",
			want: []string{"2026-04-09", "2026-04-20", "2026-04-23"},
		},
		{
			name: "stříhání plotu: červen a září, druhá středa",
			rule: Rule{Freq: Monthly, Start: "2026-01-01", Months: []time.Month{6, 9}, WeekOfMonth: 2,
				Weekdays: []time.Weekday{time.Wednesday}},
			from: "2026-01-01", to: "2026-12-31",
			want: []string{"2026-06-10", "2026-09-09"},
		},
		{
			name: "poslední pátek v říjnu (31. 10. je sobota)",
			rule: Rule{Freq: Monthly, Start: "2026-01-01", Months: []time.Month{10}, WeekOfMonth: -1,
				Weekdays: []time.Weekday{time.Friday}},
			from: "2026-10-01", to: "2026-10-31",
			want: []string{"2026-10-30"},
		},
		{
			name: "poslední pondělí v září = 28. 9. (svátek) → úterý",
			rule: Rule{Freq: Monthly, Start: "2026-01-01", Months: []time.Month{9}, WeekOfMonth: -1,
				Weekdays: []time.Weekday{time.Monday}, Holidays: HolidayNextWorkday},
			from: "2026-09-01", to: "2026-09-30",
			want: []string{"2026-09-29"},
		},
		{
			name: "until ořízne konec",
			rule: Rule{Freq: Weekly, Start: "2026-04-01", Until: "2026-04-15", IntervalWeeks: 1,
				Weekdays: []time.Weekday{time.Tuesday}},
			from: "2026-04-01", to: "2026-05-31",
			want: []string{"2026-04-07", "2026-04-14"},
		},
		{
			name: "mimo období = prázdný seznam, ne nil",
			rule: Rule{Freq: Weekly, Start: "2026-04-01", IntervalWeeks: 1, Weekdays: []time.Weekday{time.Tuesday},
				Months: []time.Month{7}},
			from: "2026-04-01", to: "2026-05-31",
			want: []string{},
		},
	}
	for _, c := range cases {
		got, err := c.rule.Occurrences(c.from, c.to)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %v\nwant %v", c.name, got, c.want)
		}
	}
}

func TestValidate(t *testing.T) {
	bad := []Rule{
		{Freq: "daily", Start: "2026-01-01"},
		{Freq: Weekly, Start: "2026-13-01", IntervalWeeks: 1, Weekdays: []time.Weekday{1}},
		{Freq: Weekly, Start: "2026-01-01", IntervalWeeks: 0, Weekdays: []time.Weekday{1}},
		{Freq: Weekly, Start: "2026-01-01", IntervalWeeks: 1},
		{Freq: Weekly, Start: "2026-05-01", Until: "2026-04-01", IntervalWeeks: 1, Weekdays: []time.Weekday{1}},
		{Freq: Monthly, Start: "2026-01-01", Weekdays: []time.Weekday{1}, WeekOfMonth: 1},
		{Freq: Monthly, Start: "2026-01-01", Months: []time.Month{6}, Weekdays: []time.Weekday{1}, WeekOfMonth: 5},
		{Freq: Monthly, Start: "2026-01-01", Months: []time.Month{13}, Weekdays: []time.Weekday{1}, WeekOfMonth: 1},
		{Freq: Weekly, Start: "2026-01-01", IntervalWeeks: 1, Weekdays: []time.Weekday{1}, Holidays: "maybe"},
	}
	for i, r := range bad {
		if err := r.Validate(); !errors.Is(err, ErrRule) {
			t.Errorf("rule %d accepted: %+v", i, r)
		}
	}
	ok := Rule{Freq: Weekly, Start: "2026-01-01", IntervalWeeks: 1, Weekdays: []time.Weekday{time.Tuesday}}
	if _, err := ok.Occurrences("2026-01-01", "2028-06-01"); !errors.Is(err, ErrRange) {
		t.Error("span over MaxSpanDays accepted")
	}
	if _, err := ok.Occurrences("2026-02-01", "2026-01-01"); !errors.Is(err, ErrRange) {
		t.Error("to < from accepted")
	}
}

func TestHolidays(t *testing.T) {
	easter := map[int]string{2024: "2024-03-31", 2025: "2025-04-20", 2026: "2026-04-05", 2027: "2027-03-28", 2030: "2030-04-21"}
	for y, want := range easter {
		if got := fmtDate(EasterSunday(y)); got != want {
			t.Errorf("Easter %d = %s; want %s", y, got, want)
		}
	}
	d := func(s string) time.Time { t, _ := parseDate(s); return t }
	for _, s := range []string{"2026-04-03", "2026-04-06", "2026-05-08", "2026-07-06", "2026-10-28", "2026-12-24"} {
		if !IsCZHoliday(d(s)) {
			t.Errorf("%s should be a holiday", s)
		}
	}
	for _, s := range []string{"2026-04-05", "2026-04-07", "2026-05-09"} {
		if IsCZHoliday(d(s)) {
			t.Errorf("%s is not a holiday", s)
		}
	}
	if got := fmtDate(NextWorkday(d("2027-12-24"))); got != "2027-12-27" {
		t.Errorf("NextWorkday after Štědrý den 2027 = %s", got)
	}
}
