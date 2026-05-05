package runner

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// splitHostPortScheme parses an httpx URL like "https://api.example.com/" and
// returns the bare host, port, and scheme. The fallback Host hint comes from
// httpx's "host" field for cases where parsing fails.
func splitHostPortScheme(rawURL, fallbackHost string) (host string, port int, scheme string) {
	host = strings.ToLower(strings.TrimSpace(fallbackHost))
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	scheme = u.Scheme
	if h := u.Hostname(); h != "" {
		host = strings.ToLower(h)
	}
	if p := u.Port(); p != "" {
		port, _ = strconv.Atoi(p)
	} else if scheme == "https" {
		port = 443
	} else if scheme == "http" {
		port = 80
	}
	return
}

type Httpx struct{}

func (Httpx) Name() string { return "httpx" }

type httpxRow struct {
	URL           string   `json:"url"`
	Input         string   `json:"input"`
	Host          string   `json:"host"`
	Status        int      `json:"status_code"`
	Title         string   `json:"title"`
	Tech          []string `json:"tech"`
	A             []string `json:"a"`
	CName         []string `json:"cname"`
	Method        string   `json:"method"`
	Scheme        string   `json:"scheme"`
	Webserver     string   `json:"webserver"`
	ContentType   string   `json:"content_type"`
	ContentLength int      `json:"content_length"`
	ResponseTime  string   `json:"time"`
	Words         int      `json:"words"`
	Lines         int      `json:"lines"`
	Location      string   `json:"location"`
	Favicon       string   `json:"favicon"`
	JarmHash      string   `json:"jarm_hash"`
	ASN           any      `json:"asn"`
	CDN           bool     `json:"cdn"`
	CDNName       string   `json:"cdn_name"`
	TLSGrab       any      `json:"tls"`
	Hash          any      `json:"hash"`
}

// Run reads in.Subdomains, runs httpx, returns http assets + populates HTTPHosts/IPs.
func (Httpx) Run(ctx context.Context, in Input) (*Result, error) {
	if len(in.Subdomains) == 0 {
		return &Result{}, nil
	}
	listPath := filepath.Join(in.ArtifactsDir, "httpx_input.txt")
	_ = os.MkdirAll(filepath.Dir(listPath), 0o755)
	f, err := os.Create(listPath)
	if err != nil {
		return nil, err
	}
	for _, s := range in.Subdomains {
		f.WriteString(s + "\n")
	}
	f.Close()

	out := filepath.Join(in.ArtifactsDir, "httpx.jsonl")
	_, _ = runCmd(ctx, out, "httpx",
		"-list", listPath,
		"-json", "-silent", "-no-color",
		// identification
		"-status-code", "-title", "-tech-detect", "-method",
		"-server", "-content-type", "-content-length", "-response-time",
		"-word-count", "-line-count", "-location",
		// dns / network
		"-ip", "-cname", "-asn", "-cdn",
		// fingerprinting
		"-favicon", "-jarm", "-tls-grab", "-tls-probe",
		"-hash", "sha256",
		// throughput
		"-threads", "75",
		"-timeout", "10",
		"-retries", "1",
	)

	lines, err := readLines(out)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(lines))
	hosts := make([]string, 0, len(lines))
	ips := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var row httpxRow
		if err := json.Unmarshal([]byte(ln), &row); err != nil {
			continue
		}
		if row.URL == "" {
			continue
		}
		host, port, scheme := splitHostPortScheme(row.URL, row.Host)
		meta := map[string]any{
			"cname":          row.CName,
			"ips":            row.A,
			"method":         row.Method,
			"webserver":      row.Webserver,
			"content_type":   row.ContentType,
			"content_length": row.ContentLength,
			"response_time":  row.ResponseTime,
			"words":          row.Words,
			"lines":          row.Lines,
			"location":       row.Location,
			"favicon":        row.Favicon,
			"jarm":           row.JarmHash,
			"asn":            row.ASN,
			"cdn":            row.CDN,
			"cdn_name":       row.CDNName,
			"tls":            row.TLSGrab,
			"hash":           row.Hash,
		}
		items = append(items, Item{
			Kind:   "service",
			Host:   host,
			Port:   port,
			Proto:  "tcp",
			Scheme: scheme,
			Status: row.Status,
			Title:  row.Title,
			Tech:   row.Tech,
			URL:    row.URL,
			Meta:   meta,
		})
		for _, a := range row.A {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			items = append(items, Item{
				Kind:   "ip",
				Value:  a,
				Host:   host,
				Source: "httpx",
			})
			ips = append(ips, a)
		}
		hosts = append(hosts, row.URL)
	}
	return &Result{
		ArtifactPath: out,
		Items:        items,
		Stats: map[string]any{
			"alive": len(items),
			"hosts": dedupe(hosts),
			"ips":   dedupe(ips),
		},
	}, nil
}
