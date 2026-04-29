package sources

import "testing"

func TestHasObjectClass(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		target string
		want   bool
	}{
		{"exact match", []string{"posixGroup", "top"}, "posixGroup", true},
		{"case insensitive", []string{"POSIXGROUP"}, "posixGroup", true},
		{"no false positive on substring", []string{"posixGroupExtended"}, "posixGroup", false},
		{"empty values", nil, "posixGroup", false},
		{"target absent", []string{"groupOfNames"}, "posixGroup", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasObjectClass(tc.values, tc.target); got != tc.want {
				t.Errorf("hasObjectClass(%v, %q) = %v, want %v", tc.values, tc.target, got, tc.want)
			}
		})
	}
}
