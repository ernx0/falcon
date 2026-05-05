package runner

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Dns resolves the subdomain list with Go's stdlib resolver. Originally
// shelled out to projectdiscovery/dnsx, but the Alpine + musl + ARM64
// build of dnsx silently no-ops on this image regardless of version. The
// in-process resolver is reliable, dependency-free, and lets us emit the
// exact Item shape the rest of the pipeline expects. Step is still named
// "dns" so it shows up clearly in the run timeline.
type Dnsx struct{}

func (Dnsx) Name() string { return "dns" }

// dnsRow is the shape we write to the artifact JSONL. Mirrors what the
// upstream dnsx tool produced for downstream tooling that may parse it.
type dnsRow struct {
	Host    string   `json:"host"`
	A       []string `json:"a,omitempty"`
	AAAA    []string `json:"aaaa,omitempty"`
	CNAME   []string `json:"cname,omitempty"`
	Status  string   `json:"status_code"`
	Took    string   `json:"took"`
}

const (
	dnsConcurrency = 50
	dnsTimeoutPer  = 4 * time.Second
)

func (Dnsx) Run(ctx context.Context, in Input) (*Result, error) {
	if len(in.Subdomains) == 0 {
		return &Result{}, nil
	}
	out := filepath.Join(in.ArtifactsDir, "dns.jsonl")
	_ = os.MkdirAll(filepath.Dir(out), 0o755)
	f, err := os.Create(out)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	resolver := &net.Resolver{PreferGo: false}

	var (
		mu        sync.Mutex
		items     []Item
		resolved  []string
		allIPs    []string
		sem       = make(chan struct{}, dnsConcurrency)
		wg        sync.WaitGroup
	)

	for _, sub := range in.Subdomains {
		host := strings.ToLower(strings.TrimSpace(sub))
		if host == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(host string) {
			defer wg.Done()
			defer func() { <-sem }()

			start := time.Now()
			qctx, cancel := context.WithTimeout(ctx, dnsTimeoutPer)
			defer cancel()

			ips, err := resolver.LookupHost(qctx, host)
			cname, _ := resolver.LookupCNAME(qctx, host)
			cname = strings.TrimSuffix(cname, ".")

			row := dnsRow{
				Host: host,
				Took: time.Since(start).String(),
			}
			var as, aaaas []string
			for _, ip := range ips {
				if strings.Contains(ip, ":") {
					aaaas = append(aaaas, ip)
				} else {
					as = append(as, ip)
				}
			}
			row.A = as
			row.AAAA = aaaas
			if cname != "" && cname != host {
				row.CNAME = []string{cname}
			}
			if err != nil {
				row.Status = "FAIL"
			} else {
				row.Status = "NOERROR"
			}

			// Don't pollute artifact with non-resolving rows.
			if len(as) == 0 && len(aaaas) == 0 && len(row.CNAME) == 0 {
				return
			}
			line, _ := json.Marshal(row)

			mu.Lock()
			f.Write(line)
			f.Write([]byte("\n"))

			meta := map[string]any{}
			if len(row.CNAME) > 0 {
				meta["cname"] = row.CNAME
			}
			items = append(items, Item{
				Kind:   "host",
				Value:  host,
				Source: "dns",
				Meta:   meta,
			})
			for _, ip := range append(append([]string{}, as...), aaaas...) {
				items = append(items, Item{
					Kind:   "ip",
					Value:  ip,
					Host:   host,
					Source: "dns",
				})
				allIPs = append(allIPs, ip)
			}
			resolved = append(resolved, host)
			mu.Unlock()
		}(host)
	}
	wg.Wait()

	return &Result{
		ArtifactPath: out,
		Items:        items,
		Stats: map[string]any{
			"resolved": len(resolved),
			"ips":      len(dedupe(allIPs)),
			"hosts":    dedupe(resolved),
		},
	}, nil
}
