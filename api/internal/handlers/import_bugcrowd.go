package handlers

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/erhan/falcon/api/internal/models"
	"github.com/jmoiron/sqlx"
	"golang.org/x/net/html"
)

type ImportHandler struct {
	DB *sqlx.DB
}

// ImportedAsset is one row of in-scope or out-of-scope content extracted
// from a Bugcrowd panel. HostLike rows go straight to targets/oos; others
// land in scope_assets so the operator still has them.
type ImportedAsset struct {
	Value    string `json:"value"`
	Kind     string `json:"kind"`        // website | api | ios | android | other | network | …
	URL      string `json:"url"`         // anchor href (when present)
	HostLike bool   `json:"host_like"`   // true → routable as a scan target
	InScope  bool   `json:"in_scope"`    // panel scope
}

type bugcrowdParseResult struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Rules       string          `json:"rules"`
	IconURL     string          `json:"icon_url"`
	Assets      []ImportedAsset `json:"assets"`
}

// ImportBugcrowd parses a Bugcrowd program HTML page and creates a new
// program. In-scope host-like values go to `targets`, OOS host-like values
// to `out_of_scopes`, everything else (mobile apps, github repos,
// descriptive labels) to `scope_assets`.
//
//	@Summary		Import Bugcrowd program from HTML
//	@Tags			programs
//	@Accept			plain
//	@Produce		json
//	@Param			body	body		string	true	"raw HTML"
//	@Success		201		{object}	models.Program
//	@Failure		400		{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/import/bugcrowd [post]
func (h *ImportHandler) ImportBugcrowd(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	if len(body) == 0 {
		writeErr(w, http.StatusBadRequest, "empty body")
		return
	}

	parsed, err := parseBugcrowdHTML(string(body))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "parse: "+err.Error())
		return
	}
	if parsed.Name == "" {
		writeErr(w, http.StatusBadRequest, "could not detect program name in HTML")
		return
	}

	tx, err := h.DB.BeginTxx(r.Context(), nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	var p models.Program
	err = tx.GetContext(r.Context(), &p, `
INSERT INTO programs(name, slug, description, rules, platform, icon_url)
VALUES($1, $2, $3, $4, 'bugcrowd', $5)
RETURNING *`, parsed.Name, slugify(parsed.Name), parsed.Description, parsed.Rules, parsed.IconURL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "create program: "+err.Error())
		return
	}

	// Helper inserts. Targets get `enabled=true` only when they're
	// scannable (host/wildcard/CIDR/IP); descriptive items are stored with
	// enabled=false so the scheduler skips them but the operator still
	// has the record.
	insertTarget := func(value, kind, sourceURL string) error {
		scannable := isScannableKind(kind)
		_, err := tx.ExecContext(r.Context(), `
INSERT INTO scope(program_id, value, kind, enabled, source_url)
VALUES($1, $2, $3, $4, $5)
ON CONFLICT (program_id, value) DO UPDATE
  SET source_url = CASE WHEN EXCLUDED.source_url <> '' THEN EXCLUDED.source_url ELSE targets.source_url END,
      kind = EXCLUDED.kind`, p.ID, value, kind, scannable, sourceURL)
		return err
	}
	insertOOS := func(value, kind, sourceURL string) error {
		_, err := tx.ExecContext(r.Context(), `
INSERT INTO out_of_scopes(program_id, pattern, kind, source_url)
VALUES($1, $2, $3, $4)
ON CONFLICT (program_id, pattern) DO UPDATE
  SET source_url = CASE WHEN EXCLUDED.source_url <> '' THEN EXCLUDED.source_url ELSE out_of_scopes.source_url END,
      kind = EXCLUDED.kind`, p.ID, value, kind, sourceURL)
		return err
	}

	for _, a := range parsed.Assets {
		v := strings.TrimSpace(a.Value)
		if v == "" {
			continue
		}

		var kind string
		if a.HostLike {
			kind = detectKind(v)
		} else {
			kind = normalizeAssetKind(a.Kind)
		}

		if a.InScope {
			if err := insertTarget(v, kind, a.URL); err != nil {
				writeErr(w, http.StatusInternalServerError, "insert target: "+err.Error())
				return
			}
		} else {
			oosKind := kind
			if a.HostLike {
				oosKind = "exact"
				if strings.HasPrefix(v, "*") {
					oosKind = "wildcard"
				}
			}
			if err := insertOOS(v, oosKind, a.URL); err != nil {
				writeErr(w, http.StatusInternalServerError, "insert oos: "+err.Error())
				return
			}
		}

		// Some non-host items still attach a clickable URL we can mine for
		// a scannable host (e.g. "Enrollment - (QA)" → member-qa.chime.com).
		// We add that derived host as a separate row so it gets scanned.
		if !a.HostLike && a.URL != "" {
			if h := hostFromImportURL(a.URL); h != "" && shouldDeriveTarget(h, a.Kind) {
				if a.InScope {
					_ = insertTarget(h, detectKind(h), a.URL)
				} else {
					oosKind := "exact"
					if strings.HasPrefix(h, "*") {
						oosKind = "wildcard"
					}
					_ = insertOOS(h, oosKind, a.URL)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// ----- parser -----

var (
	// Bugcrowd's page header h2.
	reHeaderH2       = regexp.MustCompile(`(?is)<h2[^>]*class="[^"]*\bbc-my-0\b[^"]*"[^>]*>(.*?)</h2>`)
	reEngagementName = regexp.MustCompile(`(?is)<h2[^>]*>\s*([^<]+?)\s+Managed Bug Bounty Engagement\s*</h2>`)
	reEngagementAlt  = regexp.MustCompile(`(?is)<h2[^>]*>\s*([^<]+?)\s+Bug Bounty Engagement\s*</h2>`)
	reSuffix         = regexp.MustCompile(`(?i)\s+(Managed\s+)?Bug Bounty( Engagement)?\s*$`)
	reTagStrip       = regexp.MustCompile(`<[^>]+>`)

	// Inside a panel, each target row pairs an asset-type tooltip with an
	// endpoint <code>. We extract them together so labels and types stay
	// aligned even when the panel reorders rows.
	reTargetRow = regexp.MustCompile(
		`(?is)data-tooltip-content="([^"]+)"[^>]*>\s*<code class="cc-rewards-link-table__endpoint">(.*?)</code>`)
	// Some rows wrap the value in <a href="…">value</a>. The href is useful
	// metadata for non-host assets (app store links, github URLs).
	reAnchorHref = regexp.MustCompile(`(?is)<a [^>]*href="([^"]+)"[^>]*>`)
	// Template URLs Bugcrowd authors write into panel markdown to express
	// "any subdomain" — e.g. https://{subdomain}.zendesk.com/. We translate
	// those into wildcard targets.
	reTemplateURL = regexp.MustCompile(`(?i)https?://\{[a-z0-9_-]+\}\.([a-z0-9.-]+\.[a-z]{2,})`)
	// Bugcrowd hosts program logos on logos.bugcrowdusercontent.com — picking
	// the first such <img src="…"> reliably gives us the program icon.
	reLogoURL = regexp.MustCompile(`(?is)<img[^>]+src="(https://logos\.bugcrowdusercontent\.com/[^"]+)"`)
)

// hostLike returns true when value can plausibly be scanned as a host /
// wildcard / CIDR (i.e. usable as a target). Free-form labels like
// "Phantom iOS Mobile App" or app store URLs return false.
func hostLike(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || strings.ContainsAny(v, " \t") {
		return false
	}
	// Strip scheme for the heuristic.
	stripped := v
	for _, p := range []string{"https://", "http://"} {
		if strings.HasPrefix(stripped, p) {
			stripped = stripped[len(p):]
			break
		}
	}
	if !strings.ContainsAny(stripped, ".:/") {
		return false
	}
	// Hostnames don't contain weird chars.
	for _, r := range stripped {
		if r == '/' || r == ':' || r == '.' || r == '-' || r == '_' || r == '*' {
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// isScannableKind returns true for target kinds the worker pipeline can
// actually scan. Other kinds (ios/android/github/etc.) ride along in the
// targets table for record-keeping but the scheduler & run-all skip them.
func isScannableKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "domain", "wildcard", "ip", "cidr", "host", "":
		return true
	}
	return false
}

// hostFromImportURL extracts the hostname from a URL string, returning ""
// when the input isn't parseable or has no host (e.g. a relative URL).
func hostFromImportURL(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// shouldDeriveTarget returns true when the host extracted from a non-host
// asset's URL is worth scanning. We skip well-known third-party stores so
// random Apple/Google/Bitrise URLs aren't shoved into the target list.
func shouldDeriveTarget(host, assetKind string) bool {
	if host == "" {
		return false
	}
	switch normalizeAssetKind(assetKind) {
	case "ios", "android", "mobile", "executable":
		// Mobile asset URLs always point at app stores / dist services we
		// don't want to scan.
		return false
	}
	skip := []string{
		"apps.apple.com", "play.google.com", "chromewebstore.google.com",
		"chrome.google.com", "addons.mozilla.org", "microsoft.com",
		"github.com", "gitlab.com", "bitbucket.org",
		"app.bitrise.io", "testflight.apple.com",
	}
	for _, s := range skip {
		if host == s || strings.HasSuffix(host, "."+s) {
			return false
		}
	}
	return true
}

// normalizeAssetKind maps Bugcrowd's tooltip labels to the kinds we store.
func normalizeAssetKind(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "ios", "android", "mobile", "executable", "hardware",
		"website", "api", "network", "other", "github":
		return s
	}
	if s == "" {
		return "other"
	}
	return s
}

// cleanValue normalizes a target string: drops backslash escapes Bugcrowd
// occasionally emits ("\*.foo.com" → "*.foo.com"), strips schemes, trims
// trailing slashes and unescapes common HTML entities.
func cleanValue(v string) string {
	v = htmlUnescapeFast(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, `\*`, `*`)
	v = strings.ReplaceAll(v, `\.`, `.`)
	for _, p := range []string{"https://", "http://"} {
		if strings.HasPrefix(v, p) {
			v = v[len(p):]
			break
		}
	}
	v = strings.TrimRight(v, "/")
	return strings.TrimSpace(v)
}

func parseBugcrowdHTML(src string) (bugcrowdParseResult, error) {
	res := bugcrowdParseResult{}

	// 1. Program name.
	if m := reHeaderH2.FindStringSubmatch(src); m != nil {
		raw := reTagStrip.ReplaceAllString(m[1], "")
		raw = htmlUnescapeFast(strings.TrimSpace(raw))
		raw = reSuffix.ReplaceAllString(raw, "")
		res.Name = strings.TrimSpace(raw)
	}
	if res.Name == "" {
		if m := reEngagementName.FindStringSubmatch(src); m != nil {
			res.Name = strings.TrimSpace(m[1])
		} else if m := reEngagementAlt.FindStringSubmatch(src); m != nil {
			res.Name = strings.TrimSpace(m[1])
		}
	}

	// 1b. Program logo / icon.
	if m := reLogoURL.FindStringSubmatch(src); m != nil {
		res.IconURL = htmlUnescapeFast(strings.TrimSpace(m[1]))
	}

	// 2. Walk each cc-target-grp panel separately and route by scope pill.
	for _, panel := range extractTargetPanels(src) {
		inScope := !strings.Contains(panel.body, "cc-scope-pill--oos")
		if !strings.Contains(panel.body, "cc-scope-pill") {
			// Not a scope panel (e.g. Known Issues, hall of famers); skip.
			continue
		}
		for _, m := range reTargetRow.FindAllStringSubmatch(panel.body, -1) {
			tooltip := strings.TrimSpace(m[1])
			rawCode := m[2]
			href := ""
			if hm := reAnchorHref.FindStringSubmatch(rawCode); hm != nil {
				href = htmlUnescapeFast(hm[1])
			}
			label := cleanValue(reTagStrip.ReplaceAllString(rawCode, ""))
			if label == "" {
				continue
			}
			res.Assets = append(res.Assets, ImportedAsset{
				Value:    label,
				Kind:     tooltip,
				URL:      href,
				HostLike: hostLike(label),
				InScope:  inScope,
			})
		}

		// Template URLs in the panel markdown ("https://{subdomain}.foo.com/")
		// are explicit signals from the program author that any subdomain of
		// foo.com is in/out of scope — convert each to a wildcard target.
		seen := map[string]bool{}
		for _, m := range reTemplateURL.FindAllStringSubmatch(panel.body, -1) {
			host := strings.ToLower(strings.TrimSpace(m[1]))
			pattern := "*." + host
			if seen[pattern] {
				continue
			}
			seen[pattern] = true
			res.Assets = append(res.Assets, ImportedAsset{
				Value:    pattern,
				Kind:     "website",
				HostLike: true,
				InScope:  inScope,
			})
		}
	}

	// 3. Description + Rules from the bc-markdown blocks. The first block
	// (inside the In Scope panel) is the program description; remaining
	// blocks contain rules / OOS narrative / reward tiers.
	mdBlocks := extractMarkdownBlocks(src)
	if len(mdBlocks) >= 1 {
		res.Description = strings.TrimSpace(htmlToText(mdBlocks[0]))
	}
	if len(mdBlocks) >= 2 {
		var b strings.Builder
		for i := 1; i < len(mdBlocks); i++ {
			if i > 1 {
				b.WriteString("\n\n")
			}
			b.WriteString(strings.TrimSpace(htmlToText(mdBlocks[i])))
		}
		res.Rules = b.String()
	}

	return res, nil
}

// targetPanel holds the body of a single Bugcrowd `cc-target-grp` block
// along with the snippet used to detect its scope pill.
type targetPanel struct {
	body string
}

// extractTargetPanels splits the document into Bugcrowd `cc-target-grp`
// blocks. The naïve "everything between two openers" split is good enough
// because Bugcrowd renders these panels at the same DOM depth.
func extractTargetPanels(src string) []targetPanel {
	const opener = `<div class="cc-target-grp`
	var out []targetPanel
	idx := 0
	for {
		i := strings.Index(src[idx:], opener)
		if i < 0 {
			break
		}
		start := idx + i
		// Find next opener after this one.
		j := strings.Index(src[start+len(opener):], opener)
		var end int
		if j < 0 {
			end = len(src)
		} else {
			end = start + len(opener) + j
		}
		out = append(out, targetPanel{body: src[start:end]})
		idx = end
	}
	return out
}

// extractMarkdownBlocks pulls every <div class="bc-markdown ..."> inner HTML.
func extractMarkdownBlocks(src string) []string {
	var out []string
	rest := src
	for {
		i := strings.Index(rest, `<div class="bc-markdown`)
		if i < 0 {
			break
		}
		open := strings.Index(rest[i:], ">")
		if open < 0 {
			break
		}
		start := i + open + 1
		depth := 1
		j := start
		for j < len(rest) {
			next := indexAny(rest[j:], "<div", "</div")
			if next < 0 {
				break
			}
			pos := j + next
			if strings.HasPrefix(rest[pos:], "<div") {
				depth++
				closeTag := strings.Index(rest[pos:], ">")
				if closeTag < 0 {
					break
				}
				j = pos + closeTag + 1
			} else {
				depth--
				closeTag := strings.Index(rest[pos:], ">")
				if closeTag < 0 {
					break
				}
				if depth == 0 {
					out = append(out, rest[start:pos])
					rest = rest[pos+closeTag+1:]
					break
				}
				j = pos + closeTag + 1
			}
		}
		if depth > 0 {
			break
		}
	}
	return out
}

func indexAny(s string, substrs ...string) int {
	best := -1
	for _, sub := range substrs {
		i := strings.Index(s, sub)
		if i >= 0 && (best == -1 || i < best) {
			best = i
		}
	}
	return best
}

// htmlToText converts a slice of rendered markdown HTML to plain markdown.
func htmlToText(src string) string {
	z := html.NewTokenizer(strings.NewReader(src))
	var b strings.Builder
	listDepth := 0
	pendingHref := ""
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		switch tt {
		case html.TextToken:
			b.WriteString(string(z.Text()))
		case html.StartTagToken, html.SelfClosingTagToken:
			tag, hasAttr := z.TagName()
			name := string(tag)
			switch name {
			case "h1", "h2", "h3", "h4":
				if b.Len() > 0 {
					b.WriteString("\n\n")
				}
				switch name {
				case "h1":
					b.WriteString("# ")
				case "h2":
					b.WriteString("## ")
				case "h3":
					b.WriteString("### ")
				case "h4":
					b.WriteString("#### ")
				}
			case "p":
				if b.Len() > 0 {
					b.WriteString("\n\n")
				}
			case "ul", "ol":
				listDepth++
				if b.Len() > 0 {
					b.WriteString("\n")
				}
			case "li":
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				for i := 1; i < listDepth; i++ {
					b.WriteString("  ")
				}
				b.WriteString("- ")
			case "br":
				b.WriteString("\n")
			case "strong", "b":
				b.WriteString("**")
			case "em", "i":
				b.WriteString("*")
			case "code":
				b.WriteString("`")
			case "a":
				if hasAttr {
					for {
						k, v, more := z.TagAttr()
						if string(k) == "href" {
							pendingHref = string(v)
						}
						if !more {
							break
						}
					}
				}
				b.WriteString("[")
			}
		case html.EndTagToken:
			tag, _ := z.TagName()
			name := string(tag)
			switch name {
			case "h1", "h2", "h3", "h4":
				b.WriteString("\n\n")
			case "p":
				b.WriteString("\n")
			case "ul", "ol":
				listDepth--
				b.WriteString("\n")
			case "strong", "b":
				b.WriteString("**")
			case "em", "i":
				b.WriteString("*")
			case "code":
				b.WriteString("`")
			case "a":
				b.WriteString("](")
				b.WriteString(pendingHref)
				b.WriteString(")")
				pendingHref = ""
			}
		}
	}
	out := b.String()
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(out)
}

func htmlUnescapeFast(s string) string {
	r := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&nbsp;", " ",
	)
	return r.Replace(s)
}
