package scope

import (
	"regexp"
	"strings"
	"sync"
)

// Pattern represents one out-of-scope rule.
type Pattern struct {
	Kind    string // exact | wildcard | regex
	Pattern string
	rx      *regexp.Regexp
}

// Matcher evaluates a value (subdomain, URL, etc.) against a list of patterns.
type Matcher struct {
	patterns []Pattern
}

func NewMatcher(patterns []Pattern) *Matcher {
	out := make([]Pattern, 0, len(patterns))
	for _, p := range patterns {
		if p.Kind == "regex" {
			rx, err := regexp.Compile(p.Pattern)
			if err != nil {
				continue
			}
			p.rx = rx
		}
		out = append(out, p)
	}
	return &Matcher{patterns: out}
}

// Match returns true if value matches any pattern (i.e. is out of scope).
func (m *Matcher) Match(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return false
	}
	// strip scheme + path so wildcard matchers see hostnames consistently.
	host := hostOf(v)
	for _, p := range m.patterns {
		switch p.Kind {
		case "exact":
			if host == strings.ToLower(p.Pattern) || v == strings.ToLower(p.Pattern) {
				return true
			}
		case "wildcard":
			if matchWildcard(host, strings.ToLower(p.Pattern)) {
				return true
			}
		case "regex":
			if p.rx != nil && p.rx.MatchString(value) {
				return true
			}
		}
	}
	return false
}

// matchWildcard supports patterns like "*.staging.example.com" or "example.com".
func matchWildcard(host, pat string) bool {
	if !strings.HasPrefix(pat, "*.") {
		return host == pat
	}
	suffix := pat[1:] // ".staging.example.com"
	return strings.HasSuffix(host, suffix) && len(host) > len(suffix)
}

func hostOf(v string) string {
	if i := strings.Index(v, "://"); i >= 0 {
		v = v[i+3:]
	}
	if i := strings.IndexAny(v, "/?#"); i >= 0 {
		v = v[:i]
	}
	if i := strings.Index(v, ":"); i >= 0 {
		v = v[:i]
	}
	return v
}

// Cache caches a Matcher per program with simple invalidation.
type Cache struct {
	mu sync.RWMutex
	m  map[int64]*Matcher
}

func NewCache() *Cache {
	return &Cache{m: map[int64]*Matcher{}}
}

func (c *Cache) Set(programID int64, matcher *Matcher) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[programID] = matcher
}

func (c *Cache) Get(programID int64) (*Matcher, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.m[programID]
	return m, ok
}

func (c *Cache) Invalidate(programID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, programID)
}
