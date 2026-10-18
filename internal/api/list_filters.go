package api

import (
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"
)

// splitStatuses parses a comma-separated status filter.
func splitStatuses(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// checkStatuses validates a comma-separated status filter.
func checkStatuses(s string, allowed []string) []error {
	for _, st := range splitStatuses(s) {
		if !slices.Contains(allowed, st) {
			return []error{&huma.ErrorDetail{Location: "query.status", Value: st,
				Message: "unknown status; allowed: " + strings.Join(allowed, ", ")}}
		}
	}
	return nil
}

// orConditions ORs (sql, args) pairs into one parenthesised WHERE.
func orConditions(q *gorm.DB, conds [][]any) *gorm.DB {
	if len(conds) == 0 {
		return q
	}
	sqls := make([]string, len(conds))
	var args []any
	for i, c := range conds {
		sqls[i] = "(" + c[0].(string) + ")"
		args = append(args, c[1:]...)
	}
	return q.Where("("+strings.Join(sqls, " OR ")+")", args...)
}
