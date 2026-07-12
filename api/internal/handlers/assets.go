package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/erhan/falcon/api/internal/models"
	"github.com/erhan/falcon/api/internal/scope"
	"github.com/jmoiron/sqlx"
)

type AssetsHandler struct {
	DB    *sqlx.DB
	Cache *scope.Cache
}

// ListHosts returns hosts for a program. Hosts are the canonical group key —
// each row is one FQDN with aggregate counts of services / endpoints / ips.
//
//	@Summary		List hosts
//	@Tags			assets
//	@Produce		json
//	@Param			id			path		int		true	"program id"
//	@Param			q			query		string	false	"fuzzy match on host value"
//	@Param			oos			query		bool	false	"include out-of-scope hosts"
//	@Param			page		query		int		false	"page"
//	@Param			page_size	query		int		false	"page size"
//	@Success		200			{object}	pageResp
//	@Failure		500			{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/hosts [get]
func (h *AssetsHandler) ListHosts(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	qs := r.URL.Query()
	q := strings.TrimSpace(qs.Get("q"))
	showOOS := qs.Get("oos") == "1"
	pg, offset := parsePagination(r, 50, 200)

	// Filter primitives — every flag is optional and AND-ed.
	hasIPs       := qs.Get("has_ips") == "1"
	hasServices  := qs.Get("has_services") == "1"
	hasEndpoints := qs.Get("has_endpoints") == "1"
	hasCDN       := qs.Get("cdn") == "1"
	noCDN        := qs.Get("cdn") == "0"
	minIPs, _    := strconv.Atoi(qs.Get("min_ips"))
	minServices, _   := strconv.Atoi(qs.Get("min_services"))
	minEndpoints, _  := strconv.Atoi(qs.Get("min_endpoints"))
	tech       := strings.TrimSpace(qs.Get("tech"))
	statusCode := qs.Get("status")
	port, _    := strconv.Atoi(qs.Get("port"))

	args := []any{pid}
	conds := []string{"h.program_id=$1"}
	if !showOOS {
		conds = append(conds, "h.out_of_scope=false")
	}
	order := "h.last_seen_at DESC, h.id DESC"
	if q != "" {
		args = append(args, q)
		conds = append(conds, "h.value % $"+strconv.Itoa(len(args)))
		order = "similarity(h.value, $" + strconv.Itoa(len(args)) + ") DESC, h.id DESC"
	}

	// EXISTS filters compose without bloating the count query.
	if hasIPs {
		conds = append(conds, "EXISTS (SELECT 1 FROM host_ips hi WHERE hi.host_id = h.id)")
	}
	if hasServices {
		conds = append(conds, "EXISTS (SELECT 1 FROM services s WHERE s.host_id = h.id AND s.out_of_scope=false)")
	}
	if hasEndpoints {
		conds = append(conds, "EXISTS (SELECT 1 FROM endpoints e WHERE e.host_id = h.id AND e.out_of_scope=false)")
	}
	if hasCDN {
		conds = append(conds, "EXISTS (SELECT 1 FROM services s WHERE s.host_id = h.id AND (s.meta->>'cdn')::bool = true)")
	}
	if noCDN {
		conds = append(conds, "NOT EXISTS (SELECT 1 FROM services s WHERE s.host_id = h.id AND (s.meta->>'cdn')::bool = true)")
	}
	if tech != "" {
		args = append(args, "%"+strings.ToLower(tech)+"%")
		conds = append(conds, "EXISTS (SELECT 1 FROM services s, unnest(s.tech) tt WHERE s.host_id = h.id AND lower(tt) LIKE $"+strconv.Itoa(len(args))+")")
	}
	if statusCode != "" {
		args = append(args, statusCode)
		conds = append(conds, "EXISTS (SELECT 1 FROM services s WHERE s.host_id = h.id AND s.status_code::text = $"+strconv.Itoa(len(args))+")")
	}
	if port > 0 {
		args = append(args, port)
		conds = append(conds, "EXISTS (SELECT 1 FROM services s WHERE s.host_id = h.id AND s.port = $"+strconv.Itoa(len(args))+")")
	}

	// Numeric thresholds use the aggregates we already compute below — but
	// we apply them as HAVING-style WHEREs after the join. Easier path:
	// pre-aggregate counts in CTEs and filter on those.
	havings := []string{}
	if minIPs > 0 {
		havings = append(havings, "COALESCE(ip_agg.cnt, 0) >= "+strconv.Itoa(minIPs))
	}
	if minServices > 0 {
		havings = append(havings, "COALESCE(svc_agg.cnt, 0) >= "+strconv.Itoa(minServices))
	}
	if minEndpoints > 0 {
		havings = append(havings, "COALESCE(ep_agg.cnt, 0) >= "+strconv.Itoa(minEndpoints))
	}
	whereClause := strings.Join(conds, " AND ")
	havingClause := ""
	if len(havings) > 0 {
		havingClause = " AND " + strings.Join(havings, " AND ")
	}

	// COUNT runs against a derived join (so the min_* filters affect the
	// total). Cheap because we don't pull all the asset blobs.
	countSQL := `
SELECT COUNT(*) FROM (
  SELECT h.id
  FROM hosts h
  LEFT JOIN (SELECT host_id, COUNT(*) AS cnt FROM host_ips GROUP BY host_id) ip_agg ON ip_agg.host_id = h.id
  LEFT JOIN (SELECT host_id, COUNT(*) AS cnt FROM services WHERE host_id IS NOT NULL GROUP BY host_id) svc_agg ON svc_agg.host_id = h.id
  LEFT JOIN (SELECT host_id, COUNT(*) AS cnt FROM endpoints WHERE host_id IS NOT NULL GROUP BY host_id) ep_agg ON ep_agg.host_id = h.id
  WHERE ` + whereClause + havingClause + `
) c`

	var total int
	if err := h.DB.GetContext(r.Context(), &total, countSQL, args...); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	args = append(args, pg.PageSize)
	limitIdx := strconv.Itoa(len(args))
	args = append(args, offset)
	offsetIdx := strconv.Itoa(len(args))

	type hostRow struct {
		models.Host
		IPCount       int      `db:"ip_count" json:"ip_count"`
		ServiceCount  int      `db:"service_count" json:"service_count"`
		EndpointCount int      `db:"endpoint_count" json:"endpoint_count"`
		IPs           string   `db:"ips" json:"-"`
		IPsJSON       []string `db:"-" json:"ips"`
		// Surface a compact summary so the UI can render badges without
		// fetching per-host details. Top status code, tech list (deduped
		// across services), CDN flag, primary title.
		TopStatus    sql.NullInt64  `db:"top_status" json:"-"`
		TopStatusOut *int64         `db:"-" json:"top_status,omitempty"`
		Tech         string         `db:"tech_csv" json:"-"`
		TechJSON     []string       `db:"-" json:"tech"`
		HasCDN       bool           `db:"has_cdn" json:"has_cdn"`
		Title        sql.NullString `db:"primary_title" json:"-"`
		TitleOut     string         `db:"-" json:"title,omitempty"`
	}
	sqlStr := `
SELECT h.*,
       COALESCE(ip_agg.cnt, 0)  AS ip_count,
       COALESCE(svc_agg.cnt, 0) AS service_count,
       COALESCE(ep_agg.cnt, 0)  AS endpoint_count,
       COALESCE(ip_agg.ips, '') AS ips,
       svc_agg.top_status       AS top_status,
       COALESCE(svc_agg.tech_csv, '') AS tech_csv,
       COALESCE(svc_agg.has_cdn, false) AS has_cdn,
       svc_agg.primary_title    AS primary_title
FROM hosts h
LEFT JOIN (
  SELECT hi.host_id, COUNT(*) AS cnt,
         string_agg(host(i.value), ',') AS ips
  FROM host_ips hi JOIN ips i ON i.id = hi.ip_id
  GROUP BY hi.host_id
) ip_agg ON ip_agg.host_id = h.id
LEFT JOIN (
  SELECT host_id,
         COUNT(*) AS cnt,
         (array_agg(status_code ORDER BY (CASE WHEN scheme = 'https' THEN 0 ELSE 1 END), port))[1] AS top_status,
         (array_agg(title ORDER BY (CASE WHEN scheme = 'https' THEN 0 ELSE 1 END), port))[1] AS primary_title,
         string_agg(distinct_tech, ',') AS tech_csv,
         BOOL_OR((meta->>'cdn')::bool) AS has_cdn
  FROM services,
       LATERAL (SELECT string_agg(t, ',') AS distinct_tech FROM unnest(tech) t) lt
  WHERE host_id IS NOT NULL
  GROUP BY host_id
) svc_agg ON svc_agg.host_id = h.id
LEFT JOIN (
  SELECT host_id, COUNT(*) AS cnt FROM endpoints
  WHERE host_id IS NOT NULL GROUP BY host_id
) ep_agg ON ep_agg.host_id = h.id
WHERE ` + whereClause + havingClause + `
ORDER BY ` + order + `
LIMIT $` + limitIdx + ` OFFSET $` + offsetIdx

	var rows []hostRow
	if err := h.DB.SelectContext(r.Context(), &rows, sqlStr, args...); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i, row := range rows {
		if row.IPs == "" {
			rows[i].IPsJSON = []string{}
		} else {
			rows[i].IPsJSON = strings.Split(row.IPs, ",")
		}
		if row.Tech == "" {
			rows[i].TechJSON = []string{}
		} else {
			seen := map[string]struct{}{}
			out := make([]string, 0, 8)
			for _, t := range strings.Split(row.Tech, ",") {
				t = strings.TrimSpace(t)
				if t == "" {
					continue
				}
				k := strings.ToLower(t)
				if _, ok := seen[k]; ok {
					continue
				}
				seen[k] = struct{}{}
				out = append(out, t)
			}
			rows[i].TechJSON = out
		}
		if row.TopStatus.Valid {
			v := row.TopStatus.Int64
			rows[i].TopStatusOut = &v
		}
		if row.Title.Valid {
			rows[i].TitleOut = row.Title.String
		}
	}

	writeJSON(w, http.StatusOK, pageResponse(rows, total, pg))
}

// UpdateHost toggles flags on a single host. Currently supports flipping
// out_of_scope. When marking a host OOS we also persist an exact rule in
// out_of_scopes so subsequent worker runs keep the host (and any newly
// discovered services/endpoints) out of scope. We also cascade the flag to
// existing services and endpoints so the UI reflects the change immediately.
//
//	@Summary		Update host (toggle out_of_scope)
//	@Tags			assets
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int					true	"program id"
//	@Param			hostId	path		int					true	"host id"
//	@Param			body	body		map[string]bool		true	"{\"out_of_scope\": true}"
//	@Success		200		{object}	models.Host
//	@Failure		400		{object}	errorResp
//	@Failure		404		{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/hosts/{hostId} [patch]
func (h *AssetsHandler) UpdateHost(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	hostID, err := paramInt(r, "hostId")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		OutOfScope *bool `json:"out_of_scope"`
	}
	if err := decode(r, &req); err != nil || req.OutOfScope == nil {
		writeErr(w, http.StatusBadRequest, "out_of_scope required")
		return
	}

	var host models.Host
	if err := h.DB.GetContext(r.Context(), &host,
		`SELECT * FROM hosts WHERE id=$1 AND program_id=$2`, hostID, pid); err != nil {
		if err == sql.ErrNoRows {
			writeErr(w, http.StatusNotFound, "host not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	tx, err := h.DB.BeginTxx(r.Context(), nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	oos := *req.OutOfScope
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE hosts SET out_of_scope=$1 WHERE id=$2`, oos, hostID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE services SET out_of_scope=$1 WHERE host_id=$2`, oos, hostID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE endpoints SET out_of_scope=$1 WHERE host_id=$2`, oos, hostID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if oos {
		if _, err := tx.ExecContext(r.Context(), `
INSERT INTO out_of_scopes(program_id, pattern, kind) VALUES($1,$2,'exact')
ON CONFLICT (program_id, pattern) DO NOTHING`, pid, host.Value); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		if _, err := tx.ExecContext(r.Context(),
			`DELETE FROM out_of_scopes WHERE program_id=$1 AND pattern=$2`,
			pid, host.Value); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.Cache.Invalidate(pid)

	host.OutOfScope = oos
	writeJSON(w, http.StatusOK, host)
}

// HostDetail returns a single host with all its IPs, services and endpoints inlined.
//
//	@Summary		Get host detail (with services & endpoints)
//	@Tags			assets
//	@Produce		json
//	@Param			id		path		int	true	"program id"
//	@Param			hostId	path		int	true	"host id"
//	@Success		200		{object}	map[string]interface{}
//	@Failure		404		{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/hosts/{hostId} [get]
func (h *AssetsHandler) HostDetail(w http.ResponseWriter, r *http.Request) {
	hostID, err := paramInt(r, "hostId")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var host models.Host
	if err := h.DB.GetContext(r.Context(), &host, "SELECT * FROM hosts WHERE id=$1", hostID); err != nil {
		if err == sql.ErrNoRows {
			writeErr(w, http.StatusNotFound, "host not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type ipRow struct {
		models.IP
		Source string `db:"source" json:"source"`
	}
	ips := []ipRow{}
	_ = h.DB.SelectContext(r.Context(), &ips, `
SELECT i.*, hi.source FROM ips i
JOIN host_ips hi ON hi.ip_id = i.id
WHERE hi.host_id = $1
ORDER BY i.value`, hostID)

	services := []models.Service{}
	_ = h.DB.SelectContext(r.Context(), &services, `
SELECT * FROM services WHERE host_id = $1
ORDER BY port, proto`, hostID)

	endpoints := []models.Endpoint{}
	_ = h.DB.SelectContext(r.Context(), &endpoints, `
SELECT * FROM endpoints WHERE host_id = $1
ORDER BY url
LIMIT 500`, hostID)

	writeJSON(w, http.StatusOK, map[string]any{
		"host":      host,
		"ips":       ips,
		"services":  services,
		"endpoints": endpoints,
	})
}

// ListServices — flat services list for a program (joined with host/ip for display).
//
//	@Summary		List services
//	@Tags			assets
//	@Produce		json
//	@Param			id			path		int		true	"program id"
//	@Param			oos			query		bool	false	"include out-of-scope"
//	@Param			page		query		int		false	"page"
//	@Param			page_size	query		int		false	"page size"
//	@Success		200			{object}	pageResp
//	@Failure		500			{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/services [get]
func (h *AssetsHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	showOOS := r.URL.Query().Get("oos") == "1"
	pg, offset := parsePagination(r, 50, 200)

	conds := []string{"s.program_id=$1"}
	args := []any{pid}
	if !showOOS {
		conds = append(conds, "s.out_of_scope=false")
	}
	whereClause := strings.Join(conds, " AND ")

	var total int
	if err := h.DB.GetContext(r.Context(), &total,
		"SELECT COUNT(*) FROM services s WHERE "+whereClause, args...); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	args = append(args, pg.PageSize, offset)
	type svcRow struct {
		models.Service
		HostValue sql.NullString `db:"host_value" json:"host_value"`
		IPValue   sql.NullString `db:"ip_value" json:"ip_value"`
	}
	var rows []svcRow
	err = h.DB.SelectContext(r.Context(), &rows, `
SELECT s.*, h.value AS host_value, host(i.value) AS ip_value
FROM services s
LEFT JOIN hosts h ON h.id = s.host_id
LEFT JOIN ips i ON i.id = s.ip_id
WHERE `+whereClause+`
ORDER BY s.last_seen_at DESC, s.id DESC
LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(rows, total, pg))
}

// ListEndpoints — flat endpoint list with optional fuzzy search on url.
//
//	@Summary		List endpoints
//	@Tags			assets
//	@Produce		json
//	@Param			id			path		int		true	"program id"
//	@Param			q			query		string	false	"fuzzy match on URL"
//	@Param			oos			query		bool	false	"include out-of-scope"
//	@Param			page		query		int		false	"page"
//	@Param			page_size	query		int		false	"page size"
//	@Success		200			{object}	pageResp
//	@Failure		500			{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/programs/{id}/endpoints [get]
func (h *AssetsHandler) ListEndpoints(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	showOOS := r.URL.Query().Get("oos") == "1"
	pg, offset := parsePagination(r, 50, 200)

	conds := []string{"program_id=$1"}
	args := []any{pid}
	if !showOOS {
		conds = append(conds, "out_of_scope=false")
	}
	order := "last_seen_at DESC, id DESC"
	if q != "" {
		args = append(args, q)
		conds = append(conds, "url % $"+strconv.Itoa(len(args)))
		order = "similarity(url, $" + strconv.Itoa(len(args)) + ") DESC, id DESC"
	}
	whereClause := strings.Join(conds, " AND ")

	var total int
	if err := h.DB.GetContext(r.Context(), &total,
		"SELECT COUNT(*) FROM endpoints WHERE "+whereClause, args...); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	args = append(args, pg.PageSize, offset)
	var rows []models.Endpoint
	err = h.DB.SelectContext(r.Context(), &rows, `
SELECT * FROM endpoints WHERE `+whereClause+`
ORDER BY `+order+`
LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pageResponse(rows, total, pg))
}

// Search globally across programs, scope, hosts, services, endpoints and reports.
// Each result row carries enough context (program_id, host_id, etc.) for the
// frontend to deep-link straight to the item.
//
//	@Summary		Global search
//	@Tags			assets
//	@Produce		json
//	@Param			q	query		string	true	"query string (min 2 chars)"
//	@Success		200	{object}	map[string]interface{}
//	@Security		BearerAuth
//	@Router			/api/search [get]
func (h *AssetsHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	empty := map[string]any{
		"programs": []any{}, "scope": []any{}, "hosts": []any{},
		"services": []any{}, "endpoints": []any{}, "reports": []any{},
	}
	if q == "" {
		writeJSON(w, http.StatusOK, empty)
		return
	}

	type programHit struct {
		ID          int64  `db:"id" json:"id"`
		Name        string `db:"name" json:"name"`
		Slug        string `db:"slug" json:"slug"`
		PlatformURL string `db:"platform_url" json:"platform_url"`
		Description string `db:"description" json:"description"`
	}
	programs := []programHit{}
	// ILIKE catches substring hits ("phantom" → "Phantom"); trigram %
	// catches typos / fuzzy matches. UNION dedupes by id.
	_ = h.DB.SelectContext(r.Context(), &programs, `
SELECT id, name, slug, platform_url, description FROM (
  SELECT *, similarity(name, $1) AS sim FROM programs
  WHERE name ILIKE '%' || $1 || '%'
     OR slug ILIKE '%' || $1 || '%'
     OR COALESCE(description,'') ILIKE '%' || $1 || '%'
     OR (name || ' ' || slug || ' ' || COALESCE(description,'')) % $1
) p ORDER BY sim DESC NULLS LAST, name LIMIT 25`, q)

	type scopeHit struct {
		ID        int64  `db:"id" json:"id"`
		ProgramID int64  `db:"program_id" json:"program_id"`
		Value     string `db:"value" json:"value"`
		Kind      string `db:"kind" json:"kind"`
	}
	scopeHits := []scopeHit{}
	_ = h.DB.SelectContext(r.Context(), &scopeHits, `
SELECT id, program_id, value, kind FROM scope WHERE value % $1
ORDER BY similarity(value, $1) DESC LIMIT 25`, q)

	type hostHit struct {
		ID         int64  `db:"id" json:"id"`
		ProgramID  int64  `db:"program_id" json:"program_id"`
		Value      string `db:"value" json:"value"`
		OutOfScope bool   `db:"out_of_scope" json:"out_of_scope"`
	}
	hosts := []hostHit{}
	_ = h.DB.SelectContext(r.Context(), &hosts, `
SELECT id, program_id, value, out_of_scope FROM hosts
WHERE value % $1 AND out_of_scope=false
ORDER BY similarity(value, $1) DESC LIMIT 50`, q)

	type serviceHit struct {
		ID         int64  `db:"id" json:"id"`
		ProgramID  int64  `db:"program_id" json:"program_id"`
		HostID     sql.NullInt64 `db:"host_id" json:"host_id"`
		HostValue  sql.NullString `db:"host_value" json:"host_value"`
		Port       int    `db:"port" json:"port"`
		Scheme     string `db:"scheme" json:"scheme"`
		Title      string `db:"title" json:"title"`
	}
	services := []serviceHit{}
	_ = h.DB.SelectContext(r.Context(), &services, `
SELECT s.id, s.program_id, s.host_id, h.value AS host_value, s.port, s.scheme, s.title
FROM services s
LEFT JOIN hosts h ON h.id = s.host_id
WHERE s.out_of_scope=false AND (
  COALESCE(s.title,'') % $1 OR COALESCE(h.value,'') % $1
)
ORDER BY similarity(COALESCE(h.value,'') || ' ' || COALESCE(s.title,''), $1) DESC
LIMIT 25`, q)

	type endpointHit struct {
		ID        int64  `db:"id" json:"id"`
		ProgramID int64  `db:"program_id" json:"program_id"`
		HostID    sql.NullInt64 `db:"host_id" json:"host_id"`
		HostValue sql.NullString `db:"host_value" json:"host_value"`
		URL       string `db:"url" json:"url"`
	}
	endpoints := []endpointHit{}
	_ = h.DB.SelectContext(r.Context(), &endpoints, `
SELECT e.id, e.program_id, e.host_id, h.value AS host_value, e.url
FROM endpoints e
LEFT JOIN hosts h ON h.id = e.host_id
WHERE e.url % $1 AND e.out_of_scope=false
ORDER BY similarity(e.url, $1) DESC LIMIT 50`, q)

	type reportHit struct {
		ID        int64  `db:"id" json:"id"`
		ProgramID int64  `db:"program_id" json:"program_id"`
		Title     string `db:"title" json:"title"`
		Severity  string `db:"severity" json:"severity"`
		Status    string `db:"status" json:"status"`
	}
	reports := []reportHit{}
	_ = h.DB.SelectContext(r.Context(), &reports, `
SELECT id, program_id, title, severity, status FROM reports
WHERE (title || ' ' || COALESCE(description_md,'')) % $1
ORDER BY created_at DESC LIMIT 50`, q)

	writeJSON(w, http.StatusOK, map[string]any{
		"programs":  programs,
		"scope":    scopeHits,
		"hosts":     hosts,
		"services":  services,
		"endpoints": endpoints,
		"reports":   reports,
	})
}

// ----- Internal endpoints (worker -> api) -----

type bulkAssetsReq struct {
	ScopeID *int64          `json:"scope_id"`
	Items    []bulkAssetItem `json:"items"`
}

// bulkAssetItem is a tagged-union: only the fields relevant to `kind` matter.
//
// kind = "host"     → uses Value (FQDN)
// kind = "ip"       → uses Value (IP); optional Host links it
// kind = "service"  → uses Host or IP, Port, Proto, Scheme, Status, Title, Tech
// kind = "endpoint" → uses URL (Value as fallback); Host pins to a host
type bulkAssetItem struct {
	Kind   string          `json:"kind"`
	Value  string          `json:"value,omitempty"`
	Host   string          `json:"host,omitempty"`
	IP     string          `json:"ip,omitempty"`
	Port   int             `json:"port,omitempty"`
	Proto  string          `json:"proto,omitempty"`
	Scheme string          `json:"scheme,omitempty"`
	Status int             `json:"status,omitempty"`
	Title  string          `json:"title,omitempty"`
	Tech   []string        `json:"tech,omitempty"`
	URL    string          `json:"url,omitempty"`
	Method string          `json:"method,omitempty"`
	Source string          `json:"source,omitempty"`
	Meta   json.RawMessage `json:"meta,omitempty"`
}

type bulkAssetsResp struct {
	Hosts     int `json:"hosts"`
	IPs       int `json:"ips"`
	Services  int `json:"services"`
	Endpoints int `json:"endpoints"`
	OOS       int `json:"oos_count"`
}

// BulkUpsert classifies items by kind and upserts each into the appropriate table.
// Out-of-scope matching is applied to the matchable string for each kind:
// host value, IP value, host or service URL form, endpoint URL.
func (h *AssetsHandler) BulkUpsert(w http.ResponseWriter, r *http.Request) {
	pid, err := paramInt(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var req bulkAssetsReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Items) == 0 {
		writeJSON(w, http.StatusOK, bulkAssetsResp{})
		return
	}
	matcher, err := LoadMatcher(h.DB, h.Cache, pid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	tx, err := h.DB.BeginTxx(r.Context(), nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	resp := bulkAssetsResp{}
	hostCache := map[string]int64{} // FQDN → host_id (for this request)
	ipCache := map[string]int64{}   // IP    → ip_id

	upsertHost := func(value string) (int64, error) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return 0, nil
		}
		if id, ok := hostCache[value]; ok {
			return id, nil
		}
		oos := matcher.Match(value)
		if oos {
			resp.OOS++
		}
		var id int64
		var inserted bool
		err := tx.QueryRowxContext(r.Context(), `
INSERT INTO hosts(program_id, scope_id, value, out_of_scope)
VALUES ($1, $2, $3, $4)
ON CONFLICT (program_id, value) DO UPDATE
  SET out_of_scope = EXCLUDED.out_of_scope,
      scope_id = COALESCE(hosts.scope_id, EXCLUDED.scope_id),
      last_seen_at = NOW()
RETURNING id, (xmax = 0) AS inserted`, pid, req.ScopeID, value, oos).Scan(&id, &inserted)
		if err != nil {
			return 0, err
		}
		if inserted {
			resp.Hosts++
		}
		hostCache[value] = id
		return id, nil
	}

	upsertIP := func(value string) (int64, error) {
		value = strings.TrimSpace(value)
		if value == "" {
			return 0, nil
		}
		if id, ok := ipCache[value]; ok {
			return id, nil
		}
		version := 4
		if strings.Contains(value, ":") {
			version = 6
		}
		oos := matcher.Match(value)
		if oos {
			resp.OOS++
		}
		var id int64
		var inserted bool
		err := tx.QueryRowxContext(r.Context(), `
INSERT INTO ips(program_id, value, version, out_of_scope)
VALUES ($1, $2::inet, $3, $4)
ON CONFLICT (program_id, value) DO UPDATE
  SET out_of_scope = EXCLUDED.out_of_scope,
      last_seen_at = NOW()
RETURNING id, (xmax = 0) AS inserted`, pid, value, version, oos).Scan(&id, &inserted)
		if err != nil {
			return 0, err
		}
		if inserted {
			resp.IPs++
		}
		ipCache[value] = id
		return id, nil
	}

	linkHostIP := func(hostID, ipID int64, source string) error {
		if hostID == 0 || ipID == 0 {
			return nil
		}
		_, err := tx.ExecContext(r.Context(), `
INSERT INTO host_ips(host_id, ip_id, source)
VALUES ($1, $2, $3)
ON CONFLICT (host_id, ip_id) DO UPDATE SET last_seen_at = NOW()`, hostID, ipID, source)
		return err
	}

	for _, it := range req.Items {
		switch it.Kind {
		case "host", "subdomain":
			value := it.Value
			if value == "" {
				value = it.Host
			}
			if _, err := upsertHost(value); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}

		case "ip":
			value := it.Value
			if value == "" {
				value = it.IP
			}
			ipID, err := upsertIP(value)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if it.Host != "" {
				hostID, err := upsertHost(it.Host)
				if err != nil {
					writeErr(w, http.StatusInternalServerError, err.Error())
					return
				}
				if err := linkHostIP(hostID, ipID, it.Source); err != nil {
					writeErr(w, http.StatusInternalServerError, err.Error())
					return
				}
			}

		case "service", "http", "port":
			if err := h.upsertService(r.Context(), tx, pid, it, matcher, &resp, upsertHost, upsertIP, linkHostIP); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}

		case "endpoint", "js_endpoint":
			if err := h.upsertEndpoint(r.Context(), tx, pid, it, matcher, &resp, upsertHost); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AssetsHandler) upsertService(
	ctx context.Context, tx *sqlx.Tx, pid int64, it bulkAssetItem,
	matcher *scope.Matcher, resp *bulkAssetsResp,
	upsertHost func(string) (int64, error),
	upsertIP func(string) (int64, error),
	linkHostIP func(int64, int64, string) error,
) error {
	host, ip, port, scheme := parseServiceTuple(it)
	if port == 0 {
		return nil
	}
	proto := it.Proto
	if proto == "" {
		proto = "tcp"
	}
	var hostID, ipID int64
	if host != "" {
		id, err := upsertHost(host)
		if err != nil {
			return err
		}
		hostID = id
	}
	if ip != "" {
		id, err := upsertIP(ip)
		if err != nil {
			return err
		}
		ipID = id
	}
	if err := linkHostIP(hostID, ipID, "httpx"); err != nil {
		return err
	}
	if hostID == 0 && ipID == 0 {
		return nil
	}

	matchTarget := host
	if matchTarget == "" {
		matchTarget = ip
	}
	oos := matcher.Match(matchTarget)
	if oos {
		resp.OOS++
	}
	meta := []byte("{}")
	if len(it.Meta) > 0 {
		meta = it.Meta
	}
	tech := models.StringArray(it.Tech)
	if tech == nil {
		tech = models.StringArray{}
	}
	techVal, _ := tech.Value()
	var inserted bool
	if hostID != 0 {
		// Service tied to a host.
		err := tx.QueryRowxContext(ctx, `
INSERT INTO services(program_id, host_id, ip_id, port, proto, scheme, status_code, title, tech, meta, out_of_scope)
VALUES ($1, $2, NULLIF($3,0), $4, $5, $6, NULLIF($7,0), $8, $9, $10, $11)
ON CONFLICT (program_id, host_id, port, proto) WHERE host_id IS NOT NULL DO UPDATE
  SET ip_id = COALESCE(EXCLUDED.ip_id, services.ip_id),
      scheme = CASE WHEN EXCLUDED.scheme <> '' THEN EXCLUDED.scheme ELSE services.scheme END,
      status_code = COALESCE(EXCLUDED.status_code, services.status_code),
      title = CASE WHEN EXCLUDED.title <> '' THEN EXCLUDED.title ELSE services.title END,
      tech = CASE WHEN array_length(EXCLUDED.tech,1) IS NOT NULL THEN EXCLUDED.tech ELSE services.tech END,
      meta = EXCLUDED.meta,
      out_of_scope = EXCLUDED.out_of_scope,
      last_seen_at = NOW()
RETURNING (xmax = 0)`, pid, hostID, ipID, port, proto, scheme, it.Status, it.Title, techVal, meta, oos).Scan(&inserted)
		if err != nil {
			return err
		}
	} else {
		err := tx.QueryRowxContext(ctx, `
INSERT INTO services(program_id, ip_id, port, proto, scheme, status_code, title, tech, meta, out_of_scope)
VALUES ($1, $2, $3, $4, $5, NULLIF($6,0), $7, $8, $9, $10)
ON CONFLICT (program_id, ip_id, port, proto) WHERE host_id IS NULL AND ip_id IS NOT NULL DO UPDATE
  SET scheme = CASE WHEN EXCLUDED.scheme <> '' THEN EXCLUDED.scheme ELSE services.scheme END,
      status_code = COALESCE(EXCLUDED.status_code, services.status_code),
      title = CASE WHEN EXCLUDED.title <> '' THEN EXCLUDED.title ELSE services.title END,
      tech = CASE WHEN array_length(EXCLUDED.tech,1) IS NOT NULL THEN EXCLUDED.tech ELSE services.tech END,
      meta = EXCLUDED.meta,
      out_of_scope = EXCLUDED.out_of_scope,
      last_seen_at = NOW()
RETURNING (xmax = 0)`, pid, ipID, port, proto, scheme, it.Status, it.Title, techVal, meta, oos).Scan(&inserted)
		if err != nil {
			return err
		}
	}
	if inserted {
		resp.Services++
	}
	return nil
}

func (h *AssetsHandler) upsertEndpoint(
	ctx context.Context, tx *sqlx.Tx, pid int64, it bulkAssetItem,
	matcher *scope.Matcher, resp *bulkAssetsResp,
	upsertHost func(string) (int64, error),
) error {
	urlStr := it.URL
	if urlStr == "" {
		urlStr = it.Value
	}
	urlStr = strings.TrimSpace(urlStr)
	if urlStr == "" {
		return nil
	}
	hostName := it.Host
	if hostName == "" {
		hostName = hostFromURL(urlStr)
	}
	var hostID int64
	if hostName != "" {
		id, err := upsertHost(hostName)
		if err != nil {
			return err
		}
		hostID = id
	}
	oos := matcher.Match(urlStr)
	if oos {
		resp.OOS++
	}
	meta := []byte("{}")
	if len(it.Meta) > 0 {
		meta = it.Meta
	}
	var inserted bool
	err := tx.QueryRowxContext(ctx, `
INSERT INTO endpoints(program_id, host_id, url, method, status_code, source, meta, out_of_scope)
VALUES ($1, NULLIF($2,0), $3, $4, NULLIF($5,0), $6, $7, $8)
ON CONFLICT (program_id, url) DO UPDATE
  SET host_id = COALESCE(EXCLUDED.host_id, endpoints.host_id),
      method = CASE WHEN EXCLUDED.method <> '' THEN EXCLUDED.method ELSE endpoints.method END,
      status_code = COALESCE(EXCLUDED.status_code, endpoints.status_code),
      source = CASE WHEN EXCLUDED.source <> '' THEN EXCLUDED.source ELSE endpoints.source END,
      meta = EXCLUDED.meta,
      out_of_scope = EXCLUDED.out_of_scope,
      last_seen_at = NOW()
RETURNING (xmax = 0)`, pid, hostID, urlStr, it.Method, it.Status, it.Source, meta, oos).Scan(&inserted)
	if err != nil {
		return err
	}
	if inserted {
		resp.Endpoints++
	}
	return nil
}

// parseServiceTuple resolves (host, ip, port, scheme) from a bulkAssetItem.
// Worker may send an explicit tuple; or a Value like "1.2.3.4:443" (port asset);
// or a URL like "https://api.example.com/" (http asset).
func parseServiceTuple(it bulkAssetItem) (host, ip string, port int, scheme string) {
	host = it.Host
	ip = it.IP
	port = it.Port
	scheme = it.Scheme

	if it.URL != "" || (it.Value != "" && (strings.HasPrefix(it.Value, "http://") || strings.HasPrefix(it.Value, "https://"))) {
		raw := it.URL
		if raw == "" {
			raw = it.Value
		}
		if u, err := url.Parse(raw); err == nil {
			if scheme == "" {
				scheme = u.Scheme
			}
			if host == "" {
				host = u.Hostname()
			}
			if port == 0 {
				if p := u.Port(); p != "" {
					port, _ = strconv.Atoi(p)
				} else if scheme == "https" {
					port = 443
				} else if scheme == "http" {
					port = 80
				}
			}
		}
		return
	}
	// "ip:port" form (naabu)
	if it.Value != "" && port == 0 {
		parts := strings.Split(it.Value, ":")
		if len(parts) == 2 {
			if ip == "" {
				ip = parts[0]
			}
			port, _ = strconv.Atoi(parts[1])
		}
	}
	return
}

func hostFromURL(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return strings.ToLower(u.Hostname())
	}
	return ""
}

// stub to keep the import alive when unused under partial builds.
var _ = fmt.Sprintf
