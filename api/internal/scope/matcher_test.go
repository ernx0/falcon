package scope

import "testing"

func TestMatcher(t *testing.T) {
	m := NewMatcher([]Pattern{
		{Kind: "exact", Pattern: "admin.example.com"},
		{Kind: "wildcard", Pattern: "*.staging.example.com"},
		{Kind: "regex", Pattern: `^test[0-9]+\.example\.com$`},
	})
	cases := map[string]bool{
		"admin.example.com":          true,
		"foo.staging.example.com":    true,
		"deep.foo.staging.example.com": true,
		"staging.example.com":        false,
		"test123.example.com":        true,
		"https://admin.example.com/x": true,
		"safe.example.com":            false,
		"":                            false,
	}
	for in, want := range cases {
		if got := m.Match(in); got != want {
			t.Errorf("Match(%q) = %v, want %v", in, got, want)
		}
	}
}
