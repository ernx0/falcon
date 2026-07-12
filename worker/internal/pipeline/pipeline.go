package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/erhan/falcon/worker/internal/apiclient"
	"github.com/erhan/falcon/worker/internal/runner"
)

type Pipeline struct {
	API          *apiclient.Client
	ArtifactsDir string
	Log          *slog.Logger
}

type Job struct {
	RunID      int64
	ProgramID  int64
	ScopeID   int64
	ScopeVal  string
	ScopeKind string
}

func (p *Pipeline) Run(ctx context.Context, j Job) (resultErr error) {
	artDir := filepath.Join(
		p.ArtifactsDir,
		"program-"+strconv.FormatInt(j.ProgramID, 10),
		"scope-"+strconv.FormatInt(j.ScopeID, 10),
		"run-"+strconv.FormatInt(j.RunID, 10),
	)

	if err := p.API.RunStart(ctx, j.RunID); err != nil {
		return fmt.Errorf("run start: %w", err)
	}
	defer func() {
		status := "success"
		errMsg := ""
		if resultErr != nil {
			status = "partial"
			errMsg = resultErr.Error()
		}
		_ = p.API.RunFinish(ctx, j.RunID, status, errMsg)
	}()

	in := runner.Input{
		ScopeValue:  j.ScopeVal,
		ScopeKind:   j.ScopeKind,
		ArtifactsDir: artDir,
	}

	subs := []string{}
	// Discovery: subfinder + assetfinder + gau, merge results. gau pulls
	// historical URLs from public archives — typically surfaces subdomains
	// and endpoints that the active enumerators miss.
	for _, t := range []runner.Tool{runner.Subfinder{}, runner.Assetfinder{}, runner.Gau{}} {
		res, err := p.runStep(ctx, j.RunID, j.ProgramID, &j.ScopeID, t, in)
		if err != nil {
			p.Log.Warn("step failed", "tool", t.Name(), "err", err)
			continue
		}
		if res == nil {
			continue
		}
		for _, it := range res.Items {
			if it.Kind == "host" {
				subs = append(subs, it.Value)
			}
		}
	}
	subs = uniqueLower(subs)

	// Always include the base hostname so single-domain targets still get probed.
	root := runner.RootDomain(j.ScopeVal)
	if root != "" {
		subs = append(subs, root)
		subs = uniqueLower(subs)
	}

	// For a wildcard scope, keep only hosts that actually fall under the
	// pattern. This matters for label-inner wildcards like
	// "prod-*.nubank.com.br": subfinder enumerates the whole root
	// (nubank.com.br), but only prod-*.nubank.com.br hosts are in scope.
	if strings.Contains(j.ScopeVal, "*") {
		kept := subs[:0]
		for _, s := range subs {
			if runner.WildcardMatch(s, j.ScopeVal) {
				kept = append(kept, s)
			}
		}
		subs = kept
	}
	in.Subdomains = subs

	// DNS resolution. Captures A/AAAA/CNAME records for every subdomain —
	// including hosts that don't speak HTTP. Replaces the subdomain set
	// with only the resolvable ones to avoid wasting httpx connections,
	// and seeds in.IPs so naabu can still run when httpx gets blocked by
	// a CDN.
	if dnsxRes, err := p.runStep(ctx, j.RunID, j.ProgramID, &j.ScopeID, runner.Dnsx{}, in); err == nil && dnsxRes != nil {
		if hostsAny, ok := dnsxRes.Stats["hosts"]; ok {
			if resolved, ok := hostsAny.([]string); ok && len(resolved) > 0 {
				in.Subdomains = resolved
			}
		}
		// Collect all dns-discovered IPs from the items list so the worker
		// has something to feed naabu even when httpx is silenced.
		ipSeen := map[string]struct{}{}
		for _, it := range dnsxRes.Items {
			if it.Kind == "ip" && it.Value != "" {
				if _, ok := ipSeen[it.Value]; !ok {
					ipSeen[it.Value] = struct{}{}
					in.IPs = append(in.IPs, it.Value)
				}
			}
		}
	}

	// Probing: httpx. May be blocked by CDNs — we don't bail when it
	// returns nothing; downstream steps fall back to dns-derived IPs.
	httpxRes, err := p.runStep(ctx, j.RunID, j.ProgramID, &j.ScopeID, runner.Httpx{}, in)
	if err == nil && httpxRes != nil {
		if hostsAny, ok := httpxRes.Stats["hosts"]; ok {
			if hh, ok := hostsAny.([]string); ok && len(hh) > 0 {
				in.HTTPHosts = hh
			}
		}
		if ipsAny, ok := httpxRes.Stats["ips"]; ok {
			if ii, ok := ipsAny.([]string); ok {
				// Merge httpx IPs into the dns-derived set rather than
				// overwriting — every distinct IP is a naabu candidate.
				seen := map[string]struct{}{}
				for _, ip := range in.IPs {
					seen[ip] = struct{}{}
				}
				for _, ip := range ii {
					if _, ok := seen[ip]; !ok {
						seen[ip] = struct{}{}
						in.IPs = append(in.IPs, ip)
					}
				}
			}
		}
	}

	// Ports: naabu (skip if no IPs)
	if len(in.IPs) > 0 {
		_, _ = p.runStep(ctx, j.RunID, j.ProgramID, &j.ScopeID, runner.Naabu{}, in)
	}

	// Crawl: katana
	if len(in.HTTPHosts) > 0 {
		_, _ = p.runStep(ctx, j.RunID, j.ProgramID, &j.ScopeID, runner.Katana{}, in)
	}

	// Content discovery: ffuf with a small built-in path wordlist against
	// every live HTTP host. Cheap, fail-soft — produces extra endpoint
	// items that katana might never reach (admin panels, .git, .env, etc.).
	if len(in.HTTPHosts) > 0 {
		_, _ = p.runStep(ctx, j.RunID, j.ProgramID, &j.ScopeID, runner.Ffuf{}, in)
	}

	// Browser: playwright/chromedp deep JS recon
	if len(in.HTTPHosts) > 0 {
		_, _ = p.runStep(ctx, j.RunID, j.ProgramID, &j.ScopeID, runner.Playwright{}, in)
	}

	return nil
}

// runStep wraps tool execution, posts step events and bulk-upserts assets.
func (p *Pipeline) runStep(ctx context.Context, runID, programID int64, targetID *int64, t runner.Tool, in runner.Input) (*runner.Result, error) {
	tool := t.Name()
	p.Log.Info("step start", "run", runID, "tool", tool)
	if err := p.API.RunStep(ctx, runID, apiclient.StepReq{Tool: tool, Status: "running"}); err != nil {
		return nil, err
	}

	res, runErr := t.Run(ctx, in)
	stats := map[string]any{}
	artifact := ""
	if res != nil {
		// Strip non-asset internal stats values (slices) — they're for pipeline use.
		for k, v := range res.Stats {
			if k == "hosts" || k == "ips" {
				continue
			}
			stats[k] = v
		}
		artifact = res.ArtifactPath

		if len(res.Items) > 0 {
			items := make([]apiclient.AssetItem, 0, len(res.Items))
			for _, it := range res.Items {
				meta := json.RawMessage("{}")
				if len(it.Meta) > 0 {
					if b, err := json.Marshal(it.Meta); err == nil {
						meta = b
					}
				}
				items = append(items, apiclient.AssetItem{
					Kind:   it.Kind,
					Value:  it.Value,
					Host:   it.Host,
					IP:     it.IP,
					Port:   it.Port,
					Proto:  it.Proto,
					Scheme: it.Scheme,
					Status: it.Status,
					Title:  it.Title,
					Tech:   it.Tech,
					URL:    it.URL,
					Method: it.Method,
					Source: it.Source,
					Meta:   meta,
				})
			}
			if up, err := p.API.BulkAssets(ctx, programID, apiclient.BulkAssetsReq{
				ScopeID: targetID,
				Items:    items,
			}); err == nil {
				stats["hosts"] = up.Hosts
				stats["ips"] = up.IPs
				stats["services"] = up.Services
				stats["endpoints"] = up.Endpoints
				stats["oos"] = up.OOSCount
			} else {
				p.Log.Warn("bulk upsert failed", "tool", tool, "err", err)
			}
		}
	}

	statusStr := "success"
	errMsg := ""
	if runErr != nil {
		statusStr = "failed"
		errMsg = runErr.Error()
	}
	statsJSON, _ := json.Marshal(stats)
	_ = p.API.RunStep(ctx, runID, apiclient.StepReq{
		Tool:         tool,
		Status:       statusStr,
		ArtifactPath: artifact,
		Stats:        statsJSON,
		Error:        errMsg,
	})

	if runErr != nil {
		return res, runErr
	}
	return res, nil
}

func uniqueLower(in []string) []string {
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
