package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Playwright is misleadingly named — it uses Chrome DevTools Protocol via chromedp.
// The runner is a recursive BFS crawler that, for every alive HTTP host, renders
// pages in headless Chromium and captures three signal sources:
//
//   1. Anchor / form / iframe targets in the rendered DOM.
//   2. Network requests intercepted via CDP (XHR, fetch, document, script…).
//   3. Inline + external <script> bodies, regex-mined for endpoint strings.
//
// Same-origin links discovered in (1) feed back into the queue so the crawler
// walks the entire app up to a per-host page cap. Tunable via env:
//
//   FALCON_PW_MAX_HOSTS              — cap on number of hosts to render (0 = no cap, default 0)
//   FALCON_PW_MAX_PAGES_PER_HOST     — cap on pages visited per host (default 50)
//   FALCON_PW_PAGE_TIMEOUT_SECONDS   — per-page navigation+settle budget (default 20)
//   FALCON_PW_INCLUDE_SUBDOMAINS     — "1" to follow links to sister subdomains (default "0")
type Playwright struct{}

func (Playwright) Name() string { return "playwright" }

// Endpoint extraction regex — matches "/api/...", absolute URLs, and template-y paths.
var endpointRe = regexp.MustCompile(`["'](\/[a-zA-Z0-9_\-\/\.\?\=&%]{3,}|https?:\/\/[a-zA-Z0-9_\-\.]+\/[a-zA-Z0-9_\-\/\.\?\=&%]+)["']`)

type pwSummary struct {
	URL        string   `json:"url"`
	Pages      []string `json:"pages_visited"`
	Endpoints  []string `json:"endpoints"`
	JSScripts  []string `json:"js_scripts"`
	Errors     []string `json:"errors,omitempty"`
}

func (Playwright) Run(ctx context.Context, in Input) (*Result, error) {
	hosts := in.HTTPHosts
	if len(hosts) == 0 {
		return &Result{}, nil
	}
	// Conservative defaults — rendering thousands of hosts deeply is genuinely
	// slow (50 hosts × 30 pages × ~5s ≈ 2h walltime). Operator should opt in to
	// wider crawls via env when they're ready to spend the time budget. Set
	// FALCON_PW_MAX_HOSTS=0 to disable the cap.
	maxHosts := envInt("FALCON_PW_MAX_HOSTS", 100)
	if maxHosts > 0 && len(hosts) > maxHosts {
		hosts = hosts[:maxHosts]
	}
	maxPagesPerHost := envInt("FALCON_PW_MAX_PAGES_PER_HOST", 30)
	pageTimeout := time.Duration(envInt("FALCON_PW_PAGE_TIMEOUT_SECONDS", 20)) * time.Second
	includeSubs := strings.TrimSpace(os.Getenv("FALCON_PW_INCLUDE_SUBDOMAINS")) == "1"

	dir := filepath.Join(in.ArtifactsDir, "playwright")
	_ = os.MkdirAll(dir, 0o755)
	outPath := filepath.Join(dir, "summary.jsonl")
	out, err := os.Create(outPath)
	if err != nil {
		return nil, err
	}
	defer out.Close()

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx,
		chromedp.NoSandbox,
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.IgnoreCertErrors,
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent("Mozilla/5.0 (X11; Linux x86_64) Falcon/1.0 Chromium"),
	)
	defer cancelAlloc()

	enc := json.NewEncoder(out)
	items := make([]Item, 0, 512)

crawlLoop:
	for _, hostURL := range hosts {
		select {
		case <-ctx.Done():
			break crawlLoop
		default:
		}
		summary := crawlSite(allocCtx, hostURL, maxPagesPerHost, pageTimeout, includeSubs, &items)
		_ = enc.Encode(summary)
	}

	return &Result{
		ArtifactPath: outPath,
		Items:        items,
		Stats: map[string]any{
			"hosts_visited":    len(hosts),
			"endpoints_total":  len(items),
			"max_pages_perhst": maxPagesPerHost,
		},
	}, nil
}

// crawlSite walks one host URL via BFS, up to maxPages, and appends discovered
// endpoint items to *items. Same-origin scope: same hostname (or sister
// subdomains if includeSubs).
func crawlSite(parent context.Context, root string, maxPages int, perPage time.Duration, includeSubs bool, items *[]Item) pwSummary {
	summary := pwSummary{URL: root}
	rootURL, err := url.Parse(root)
	if err != nil || rootURL.Hostname() == "" {
		summary.Errors = append(summary.Errors, "invalid root: "+err.Error())
		return summary
	}

	rootHost := strings.ToLower(rootURL.Hostname())
	rootPSL := publicSuffixHint(rootHost)

	visited := map[string]struct{}{}
	queue := []string{root}
	emitted := map[string]struct{}{}

	emit := func(rawURL, src string) {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			return
		}
		if _, ok := emitted[rawURL]; ok {
			return
		}
		emitted[rawURL] = struct{}{}
		host := hostFromRawURL(rawURL)
		meta := map[string]any{
			"discovery": src,
			"context":   root,
		}
		if src == "script" || isJSURL(rawURL) {
			meta["asset_type"] = "js"
		}
		*items = append(*items, Item{
			Kind:   "endpoint",
			URL:    rawURL,
			Host:   host,
			Source: "playwright",
			Meta:   meta,
		})
		summary.Endpoints = append(summary.Endpoints, rawURL)
	}

	for len(queue) > 0 && len(visited) < maxPages {
		select {
		case <-parent.Done():
			summary.Errors = append(summary.Errors, "context cancelled")
			return summary
		default:
		}

		page := queue[0]
		queue = queue[1:]
		if _, seen := visited[page]; seen {
			continue
		}
		visited[page] = struct{}{}
		summary.Pages = append(summary.Pages, page)
		emit(page, "navigated")

		links, scripts, netURLs, scriptBodies, perr := visitPage(parent, page, perPage)
		if perr != "" {
			summary.Errors = append(summary.Errors, page+": "+perr)
			continue
		}
		// External + inline JS bodies — regex-mined for endpoints.
		for src, body := range scriptBodies {
			summary.JSScripts = append(summary.JSScripts, src)
			pageBase, _ := url.Parse(page)
			for _, m := range endpointRe.FindAllStringSubmatch(body, -1) {
				ep := normalizeEndpoint(m[1], pageBase)
				if ep != "" {
					emit(ep, "js")
				}
			}
		}
		// Network log — every fetch/XHR/document/script captured.
		for _, n := range netURLs {
			emit(n, "network")
		}
		// Anchor / form / iframe targets — enqueue same-origin ones.
		for _, l := range links {
			absLink := absURL(l, page)
			if absLink == "" {
				continue
			}
			emit(absLink, "link")
			lu, err := url.Parse(absLink)
			if err != nil || lu.Hostname() == "" {
				continue
			}
			lh := strings.ToLower(lu.Hostname())
			inScope := lh == rootHost
			if !inScope && includeSubs && rootPSL != "" {
				inScope = strings.HasSuffix(lh, "."+rootPSL) || lh == rootPSL
			}
			if inScope {
				if _, seen := visited[absLink]; !seen && len(visited)+len(queue) < maxPages*2 {
					queue = append(queue, absLink)
				}
			}
		}
		// Capture script srcs even if we couldn't fetch their bodies.
		for _, s := range scripts {
			absS := absURL(s, page)
			if absS != "" {
				emit(absS, "script")
			}
		}
	}
	return summary
}

// visitPage navigates one URL, enables network capture, returns:
//   links     — <a href>, <form action>, <iframe src>
//   scripts   — <script src> URLs
//   netURLs   — every network request URL captured by CDP
//   bodies    — map[scriptURL]body for fetched .js bodies
//   errMsg    — non-empty on failure
func visitPage(parent context.Context, target string, perPage time.Duration) (links, scripts, netURLs []string, bodies map[string]string, errMsg string) {
	tabCtx, cancelTab := chromedp.NewContext(parent)
	defer cancelTab()
	tabCtx, cancelTimeout := context.WithTimeout(tabCtx, perPage)
	defer cancelTimeout()

	bodies = map[string]string{}
	netSet := sync.Map{}

	chromedp.ListenTarget(tabCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventResponseReceived:
			if e == nil || e.Response == nil {
				return
			}
			u := e.Response.URL
			if u == "" || strings.HasPrefix(u, "data:") || strings.HasPrefix(u, "blob:") {
				return
			}
			netSet.Store(u, struct{}{})
		case *network.EventRequestWillBeSent:
			if e == nil || e.Request == nil {
				return
			}
			u := e.Request.URL
			if u == "" || strings.HasPrefix(u, "data:") || strings.HasPrefix(u, "blob:") {
				return
			}
			netSet.Store(u, struct{}{})
		}
	})

	err := chromedp.Run(tabCtx,
		network.Enable(),
		chromedp.Navigate(target),
		chromedp.Sleep(2*time.Second), // SPA settle
	)
	if err != nil {
		errMsg = err.Error()
		// Fall through — we may still have partial data from the listener.
	}

	// DOM extraction.
	_ = chromedp.Run(tabCtx,
		chromedp.Evaluate(`Array.from(document.querySelectorAll('a[href]')).map(a => a.href)`, &links),
		chromedp.Evaluate(`Array.from(document.querySelectorAll('script[src]')).map(s => s.src)`, &scripts),
	)
	var formActions []string
	_ = chromedp.Run(tabCtx,
		chromedp.Evaluate(`Array.from(document.querySelectorAll('form[action]')).map(f => f.action)`, &formActions),
	)
	links = append(links, formActions...)
	var iframeSrcs []string
	_ = chromedp.Run(tabCtx,
		chromedp.Evaluate(`Array.from(document.querySelectorAll('iframe[src]')).map(i => i.src)`, &iframeSrcs),
	)
	links = append(links, iframeSrcs...)

	// Inline scripts — bodies for regex mining.
	var inlineScripts []string
	_ = chromedp.Run(tabCtx,
		chromedp.Evaluate(`Array.from(document.querySelectorAll('script:not([src])')).map(s => s.textContent)`, &inlineScripts),
	)
	for i, body := range inlineScripts {
		bodies[fmt.Sprintf("%s#inline-%d", target, i)] = body
	}

	// External scripts — fetch bodies via in-page fetch (cookies + auth context).
	for _, src := range scripts {
		if !strings.Contains(src, ".js") && !strings.Contains(src, "/api/") {
			continue
		}
		var body string
		jsFetch := `fetch(` + jsString(src) + `).then(r=>r.text()).catch(_=>'')`
		if err := chromedp.Run(tabCtx, chromedp.EvaluateAsDevTools(jsFetch, &body)); err == nil && body != "" {
			bodies[src] = body
		}
	}

	netSet.Range(func(k, _ any) bool {
		netURLs = append(netURLs, k.(string))
		return true
	})
	return
}

func absURL(href, base string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") || strings.HasPrefix(href, "#") {
		return ""
	}
	if u, err := url.Parse(href); err == nil && u.IsAbs() {
		return u.String()
	}
	bu, err := url.Parse(base)
	if err != nil {
		return ""
	}
	hu, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return bu.ResolveReference(hu).String()
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func normalizeEndpoint(ep string, base *url.URL) string {
	ep = strings.TrimSpace(ep)
	if ep == "" {
		return ""
	}
	if strings.Contains(ep, " ") {
		return ""
	}
	if strings.HasPrefix(ep, "//") {
		ep = "https:" + ep
	}
	if strings.HasPrefix(ep, "/") && base != nil {
		ep = base.Scheme + "://" + base.Host + ep
	}
	if !(strings.HasPrefix(ep, "http://") || strings.HasPrefix(ep, "https://")) {
		return ""
	}
	if u, err := url.Parse(ep); err == nil {
		return u.String()
	}
	return ""
}

// isJSURL returns true if the URL path looks like a JavaScript asset. Used to
// tag endpoints whose origin we couldn't infer from <script src> directly (e.g.
// JS files surfaced via the network listener).
func isJSURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	p := strings.ToLower(u.Path)
	if strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".mjs") || strings.HasSuffix(p, ".cjs") {
		return true
	}
	if strings.Contains(p, ".js?") || strings.Contains(p, ".mjs?") {
		return true
	}
	return false
}

// publicSuffixHint returns a naive eTLD+1 ("foo.example.com" → "example.com")
// using the right-most two labels. Good enough for sister-subdomain scope checks
// without pulling in golang.org/x/net/publicsuffix.
func publicSuffixHint(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
}
