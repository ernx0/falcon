package runner

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
)

// Gau harvests historical URLs from public archives (Wayback Machine,
// Common Crawl, AlienVault OTX, URLScan) for the scope item root domain.
// Yields a flood of subdomains (host items) and endpoint URLs that
// subfinder/assetfinder typically miss.
type Gau struct{}

func (Gau) Name() string { return "gau" }

func (Gau) Run(ctx context.Context, in Input) (*Result, error) {
	root := RootDomain(in.ScopeValue)
	if root == "" {
		return &Result{}, nil
	}
	out := filepath.Join(in.ArtifactsDir, "gau.txt")
	// gau accepts a domain on stdin or via positional arg. We use the
	// positional form for simplicity. --subs widens to all subdomains.
	_, err := runCmd(ctx, out, "gau",
		"--subs",
		"--threads", "8",
		"--timeout", "30",
		"--blacklist", "png,jpg,jpeg,gif,svg,ico,woff,woff2,ttf,css,js.map",
		root,
	)
	// Tolerate non-zero exits — providers occasionally rate-limit and gau
	// returns 1 even when partial results made it to disk.
	_ = err

	lines, err := readLines(out)
	if err != nil {
		return nil, err
	}

	hostsSeen := map[string]struct{}{}
	hosts := make([]string, 0, 64)
	endpoints := make([]string, 0, len(lines))
	items := make([]Item, 0, len(lines))
	for _, raw := range lines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		host := strings.ToLower(u.Hostname())
		if host == "" {
			continue
		}
		// Restrict to assets under the scope item root so we don't pollute the
		// asset graph with third-party hosts found in archived URLs.
		if host != root && !strings.HasSuffix(host, "."+root) {
			continue
		}
		if _, seen := hostsSeen[host]; !seen {
			hostsSeen[host] = struct{}{}
			hosts = append(hosts, host)
			items = append(items, Item{Kind: "host", Value: host, Source: "gau"})
		}
		items = append(items, Item{
			Kind:   "endpoint",
			URL:    raw,
			Host:   host,
			Source: "gau",
		})
		endpoints = append(endpoints, raw)
	}
	return &Result{
		ArtifactPath: out,
		Items:        items,
		Stats: map[string]any{
			"endpoints": len(endpoints),
			"hosts":     hosts,
		},
	}, nil
}
