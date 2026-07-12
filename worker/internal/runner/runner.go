package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Result is the standardized output of a tool runner.
type Result struct {
	ArtifactPath string
	Items        []Item // Asset items to upsert
	Stats        map[string]any
}

// Item is a discovered asset. Kind selects which fields are meaningful:
//   host     → Value (FQDN)
//   ip       → Value (IP);  Host (optional, links host↔ip)
//   service  → Host or IP, Port, Proto, Scheme, Status, Title, Tech
//   endpoint → URL (or Value); Host (optional)
type Item struct {
	Kind   string
	Value  string
	Host   string
	IP     string
	Port   int
	Proto  string
	Scheme string
	Status int
	Title  string
	Tech   []string
	URL    string
	Method string
	Source string
	Meta   map[string]any
}

// Tool is implemented by every recon tool wrapper.
type Tool interface {
	Name() string
	Run(ctx context.Context, in Input) (*Result, error)
}

// Input passed to every runner.
type Input struct {
	ScopeValue   string   // "*.example.com" or "example.com"
	ScopeKind    string   // domain|wildcard|ip|cidr
	ArtifactsDir  string   // /data/artifacts/<program>/<run>/
	Subdomains    []string // populated by previous discovery steps
	HTTPHosts     []string // canlı http(s) URLs from httpx
	IPs           []string // unique IPs from httpx
}

// runCmd runs a command and writes stdout to outPath. Returns line count.
func runCmd(ctx context.Context, outPath string, name string, args ...string) (int, error) {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return 0, err
	}

	count := 0
	br := bufio.NewReader(stdout)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			f.WriteString(line)
			count++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}
	if err := cmd.Wait(); err != nil {
		return count, fmt.Errorf("%s: %w", name, err)
	}
	return count, nil
}

// hostFromRawURL returns the lowercase hostname for a URL, or "" if unparseable.
func hostFromRawURL(raw string) string {
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil {
		return strings.ToLower(u.Hostname())
	}
	return ""
}

// RootDomain returns the enumerable base domain for a scope value by dropping
// any label that contains a wildcard. Handles both the classic full wildcard
// ("*.example.com" -> "example.com") and label-inner wildcards
// ("prod-*.nubank.com.br" -> "nubank.com.br"). Non-wildcard values pass
// through unchanged.
func RootDomain(v string) string {
	v = strings.TrimSpace(v)
	if !strings.Contains(v, "*") {
		return v
	}
	labels := strings.Split(v, ".")
	kept := make([]string, 0, len(labels))
	for _, l := range labels {
		if strings.Contains(l, "*") {
			continue
		}
		kept = append(kept, l)
	}
	return strings.Join(kept, ".")
}

// WildcardMatch reports whether host falls under a scope pattern.
//   - no wildcard        -> exact match
//   - "*.example.com"    -> example.com and any-depth subdomain
//   - "prod-*.nubank.br" -> label-inner glob; "*" matches [a-z0-9-]* within a
//     single label, the rest must match exactly ("prod-api.nubank.br" yes,
//     "prod-api.x.nubank.br" and "api.nubank.br" no)
func WildcardMatch(host, pattern string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if !strings.Contains(pattern, "*") {
		return host == pattern
	}
	if strings.HasPrefix(pattern, "*.") {
		base := pattern[2:]
		return host == base || strings.HasSuffix(host, "."+base)
	}
	re := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, `[a-z0-9-]*`) + "$"
	ok, _ := regexp.MatchString(re, host)
	return ok
}

// dedupe normalizes and removes duplicate strings.
func dedupe(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// ToolTimeout default cap for any subprocess.
const ToolTimeout = 30 * time.Minute
