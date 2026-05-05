package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// chromePath returns the path to a system Chrome/Chromium binary that katana's
// embedded go-rod can launch in headless mode. The slim Alpine image ships
// chromium at /usr/bin/chromium-browser; override via FALCON_CHROME_PATH.
func chromePath() string {
	if v := strings.TrimSpace(os.Getenv("FALCON_CHROME_PATH")); v != "" {
		return v
	}
	for _, p := range []string{
		"/usr/bin/chromium-browser",
		"/usr/bin/chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/chrome",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "/usr/bin/chromium-browser"
}

// envInt reads an integer env var with a fallback default.
func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

type Katana struct{}

func (Katana) Name() string { return "katana" }

type katanaRow struct {
	Timestamp string `json:"timestamp"`
	Request   struct {
		Endpoint string            `json:"endpoint"`
		Method   string            `json:"method"`
		Source   string            `json:"source"`
		Tag      string            `json:"tag"`
		Attribute string           `json:"attribute"`
		Body     string            `json:"body"`
		Headers  map[string]string `json:"headers"`
	} `json:"request"`
	Response struct {
		StatusCode    int               `json:"status_code"`
		Headers       map[string]string `json:"headers"`
		Technologies  []string          `json:"technologies"`
		ContentLength int               `json:"content_length"`
		Title         string            `json:"title"`
		Server        string            `json:"server"`
	} `json:"response"`
}

func (Katana) Run(ctx context.Context, in Input) (*Result, error) {
	if len(in.HTTPHosts) == 0 {
		return &Result{}, nil
	}
	listPath := filepath.Join(in.ArtifactsDir, "katana_input.txt")
	_ = os.MkdirAll(filepath.Dir(listPath), 0o755)
	f, err := os.Create(listPath)
	if err != nil {
		return nil, err
	}
	for _, u := range in.HTTPHosts {
		f.WriteString(u + "\n")
	}
	f.Close()

	depth := envInt("FALCON_KATANA_DEPTH", 3)
	concurrency := envInt("FALCON_KATANA_CONCURRENCY", 10)
	parallelism := envInt("FALCON_KATANA_PARALLELISM", 10)
	rateLimit := envInt("FALCON_KATANA_RATE_LIMIT", 150)

	out := filepath.Join(in.ArtifactsDir, "katana.jsonl")
	_, _ = runCmd(ctx, out, "katana",
		"-list", listPath,
		"-jsonl", "-silent", "-no-color",
		// crawl scope + depth
		"-d", strconv.Itoa(depth),
		"-fs", "rdn", // field-scope: same root domain
		// strategies — pull from every channel katana exposes
		"-jc",        // crawl javascript files for endpoints
		"-jsl",       // emit discovered JS file list
		"-kf", "all", // robots.txt, sitemap.xml, security.txt etc
		"-aff",       // auto form fill
		"-iqp",       // include query parameters in unique-key
		// browser rendering: catches SPA routes that passive crawl misses.
		// Katana ships go-rod which can't auto-download Chromium in our slim
		// container — point it at the system Chromium binary explicitly.
		"-hl",   // headless
		"-nos",  // no-sandbox (required when running as root in docker)
		"-scp", chromePath(),
		"-xhr",  // capture XHR/fetch from rendered pages
		"-fx",   // capture form action URLs
		// performance knobs (overridable via env)
		"-c", strconv.Itoa(concurrency),
		"-p", strconv.Itoa(parallelism),
		"-rl", strconv.Itoa(rateLimit),
		// per-request timeout
		"-timeout", "15",
		"-retry", "1",
		// generous total budget — runCmd's parent ctx still bounds it
		"-ct", "0",
	)

	lines, err := readLines(out)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var row katanaRow
		if err := json.Unmarshal([]byte(ln), &row); err != nil {
			continue
		}
		if row.Request.Endpoint == "" {
			continue
		}
		meta := map[string]any{}
		if row.Request.Source != "" {
			meta["discovery"] = row.Request.Source
		}
		if row.Request.Tag != "" {
			meta["tag"] = row.Request.Tag
		}
		if isJSURL(row.Request.Endpoint) ||
			strings.EqualFold(row.Request.Tag, "script") ||
			strings.EqualFold(row.Request.Source, "script") {
			meta["asset_type"] = "js"
		}
		if row.Request.Attribute != "" {
			meta["attribute"] = row.Request.Attribute
		}
		if len(row.Response.Technologies) > 0 {
			meta["tech"] = row.Response.Technologies
		}
		if row.Response.Title != "" {
			meta["title"] = row.Response.Title
		}
		if row.Response.Server != "" {
			meta["server"] = row.Response.Server
		}
		if row.Response.ContentLength > 0 {
			meta["content_length"] = row.Response.ContentLength
		}
		items = append(items, Item{
			Kind:   "endpoint",
			URL:    row.Request.Endpoint,
			Host:   hostFromRawURL(row.Request.Endpoint),
			Method: row.Request.Method,
			Status: row.Response.StatusCode,
			Source: "katana",
			Meta:   meta,
		})
	}
	return &Result{
		ArtifactPath: out,
		Items:        items,
		Stats:        map[string]any{"endpoints": len(items)},
	}, nil
}
