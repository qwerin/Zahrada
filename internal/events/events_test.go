package events

import "testing"

func TestMatches(t *testing.T) {
	for _, c := range []struct {
		patterns []string
		name     string
		want     bool
	}{
		{[]string{"*"}, "job.status_changed", true},
		{[]string{"job.*"}, "job.status_changed", true},
		{[]string{"job.*"}, "jobs.x", false},
		{[]string{"job.status_changed"}, "job.status_changed", true},
		{[]string{"job.status_changed"}, "job.created", false},
		{nil, "job.status_changed", false},
	} {
		if got := Matches(c.patterns, c.name); got != c.want {
			t.Errorf("Matches(%v, %s) = %v", c.patterns, c.name, got)
		}
	}
	for p, want := range map[string]bool{"*": true, "job.*": true, "job.status_changed": true, "foo.*": false, "job.nope": false} {
		if ValidPattern(p) != want {
			t.Errorf("ValidPattern(%s) != %v", p, want)
		}
	}
}
