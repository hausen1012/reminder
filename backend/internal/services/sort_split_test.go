package services

import "testing"

// splitOrderClause 只接受 buildOrderClause 产出的形态。
func TestSplitOrderClause(t *testing.T) {
	cases := []struct {
		clause string
		field  string
		dir    string
		ok     bool
	}{
		{"next_fire_at ASC", "next_fire_at", "ASC", true},
		{"created_at desc", "created_at", "DESC", true},
		{"title", "", "", false},
		{"", "", "", false},
		{"next_fire_at SIDEWAYS", "", "", false},
		{"a b c", "", "", false},
	}
	for _, c := range cases {
		field, dir, ok := splitOrderClause(c.clause)
		if ok != c.ok || field != c.field || dir != c.dir {
			t.Errorf("splitOrderClause(%q) = (%q,%q,%v)，期望 (%q,%q,%v)",
				c.clause, field, dir, ok, c.field, c.dir, c.ok)
		}
	}
}
