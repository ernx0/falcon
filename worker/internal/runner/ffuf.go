package runner

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Ffuf does a tiny, broad-strokes content-discovery sweep against every
// live HTTP host found by httpx. It uses the in-binary wordlist (or an
// override at /usr/local/share/falcon/paths.txt). Hits with status codes
// outside [200, 204, 301, 302, 401, 403] are filtered.
type Ffuf struct{}

func (Ffuf) Name() string { return "ffuf" }

type ffufResult struct {
	Results []ffufHit `json:"results"`
}
type ffufHit struct {
	URL          string `json:"url"`
	Status       int    `json:"status"`
	Length       int    `json:"length"`
	Words        int    `json:"words"`
	Lines        int    `json:"lines"`
	ContentType  string `json:"content-type"`
	RedirectLoc  string `json:"redirectlocation"`
	ResultFile   string `json:"resultfile"`
	Input        map[string]string `json:"input"`
}

func (Ffuf) Run(ctx context.Context, in Input) (*Result, error) {
	if len(in.HTTPHosts) == 0 {
		return &Result{}, nil
	}

	// Resolve the wordlist: prefer the operator-supplied file when present
	// (lets ops swap in a bigger list without rebuilding) and fall back to
	// the in-binary one so the step is never blocked on a missing file.
	wordlist := "/usr/local/share/falcon/paths.txt"
	if _, err := os.Stat(wordlist); err != nil {
		// Materialize the built-in list to disk because ffuf only reads
		// wordlists from a path.
		wordlist = filepath.Join(in.ArtifactsDir, "ffuf_wordlist.txt")
		_ = os.MkdirAll(filepath.Dir(wordlist), 0o755)
		if f, err := os.Create(wordlist); err == nil {
			for _, w := range builtinPathWordlist {
				f.WriteString(w + "\n")
			}
			f.Close()
		}
	}

	allItems := make([]Item, 0, 64)
	totalHits := 0
	totalScanned := 0
	for _, base := range in.HTTPHosts {
		base = strings.TrimRight(base, "/")
		// Some httpx URLs already include a path — strip to host root for
		// fuzzing; the original URL is still in the asset graph.
		if u, err := url.Parse(base); err == nil {
			u.Path = ""
			u.RawQuery = ""
			u.Fragment = ""
			base = u.String()
		}
		host := hostFromRawURL(base)
		outFile := filepath.Join(in.ArtifactsDir, "ffuf_"+sanitizeFilename(host)+".json")

		_, _ = runCmd(ctx, outFile+".log", "ffuf",
			"-u", base+"/FUZZ",
			"-w", wordlist,
			"-mc", "200,204,301,302,401,403",
			"-fs", "0", // ignore zero-length responses
			"-t", "40",
			"-timeout", "8",
			"-rate", "100",
			"-of", "json",
			"-o", outFile,
			"-s", // silent
		)
		totalScanned++

		body, err := os.ReadFile(outFile)
		if err != nil || len(body) == 0 {
			continue
		}
		var parsed ffufResult
		if err := json.Unmarshal(body, &parsed); err != nil {
			continue
		}
		for _, h := range parsed.Results {
			if h.URL == "" {
				continue
			}
			meta := map[string]any{
				"status":       h.Status,
				"length":       h.Length,
				"words":        h.Words,
				"lines":        h.Lines,
				"content_type": h.ContentType,
			}
			if h.RedirectLoc != "" {
				meta["location"] = h.RedirectLoc
			}
			allItems = append(allItems, Item{
				Kind:   "endpoint",
				URL:    h.URL,
				Host:   host,
				Status: h.Status,
				Source: "ffuf",
				Meta:   meta,
			})
			totalHits++
		}
	}
	return &Result{
		Items: allItems,
		Stats: map[string]any{
			"hosts_scanned": totalScanned,
			"hits":          totalHits,
		},
	}, nil
}

// sanitizeFilename keeps a hostname filesystem-safe.
func sanitizeFilename(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			out = append(out, c)
		case c == '.' || c == '-' || c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
