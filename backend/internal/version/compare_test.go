package version

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v1.4.1", "v1.4.0", true},
		{"v1.4.0", "v1.4.1", false},
		{"v1.4.1", "v1.4.1", false},
		{"v1.10.0", "v1.9.9", true},
		{"v2", "v1.99.99", true},
		{"v1.2", "v1.2.0", false},
		{"1.4.1", "v1.4.0", true},
		{"v1.5.0-rc1", "v1.4.0", true},
		{"v1.4.0+meta", "v1.4.0", false},
		{"dev", "v1.4.0", false},
		{"v1.4.0", "dev", false},
		{"", "v1.4.0", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.latest, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v，期望 %v", c.latest, c.current, got, c.want)
		}
	}
}
