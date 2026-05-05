package handlers

import (
	"context"
	"encoding/json"
	_ "embed"
	"io"
	"net/http"
	"sync"
	"time"
)

//go:embed vrt.json
var vrtJSON []byte

const vrtUpstreamURL = "https://raw.githubusercontent.com/bugcrowd/vulnerability-rating-taxonomy/master/vulnerability-rating-taxonomy.json"

// VRTHandler serves the Bugcrowd Vulnerability Rating Taxonomy. The taxonomy
// ships embedded in the binary, but operators can pull a fresher copy from
// upstream at runtime via /api/vrt/refresh — the override is held in memory
// (no persistence across restarts).
type VRTHandler struct {
	mu       sync.RWMutex
	override []byte
	fetchedAt time.Time
}

// current returns the body to serve and the source label.
func (h *VRTHandler) current() ([]byte, string, time.Time) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.override) > 0 {
		return h.override, "upstream", h.fetchedAt
	}
	return vrtJSON, "embedded", time.Time{}
}

// Get returns the VRT taxonomy JSON.
//
//	@Summary		Get Bugcrowd VRT taxonomy
//	@Tags			vrt
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Security		BearerAuth
//	@Router			/api/vrt [get]
func (h *VRTHandler) Get(w http.ResponseWriter, r *http.Request) {
	body, source, fetchedAt := h.current()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-VRT-Source", source)
	if !fetchedAt.IsZero() {
		w.Header().Set("X-VRT-Fetched-At", fetchedAt.UTC().Format(time.RFC3339))
	}
	_, _ = w.Write(body)
}

// Refresh fetches the latest VRT JSON from Bugcrowd's GitHub mirror and stores
// it in memory. Callers must be authenticated. The previous override (or the
// embedded fallback) is replaced atomically on success.
//
//	@Summary		Refresh VRT from upstream
//	@Tags			vrt
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		502	{object}	errorResp
//	@Security		BearerAuth
//	@Router			/api/vrt/refresh [post]
func (h *VRTHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, vrtUpstreamURL, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "fetch failed: "+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeErr(w, http.StatusBadGateway, "upstream returned "+resp.Status)
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "read failed: "+err.Error())
		return
	}
	// Validate it parses and looks like the VRT shape before swapping.
	var probe struct {
		Metadata json.RawMessage `json:"metadata"`
		Content  []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(body, &probe); err != nil || len(probe.Content) == 0 {
		writeErr(w, http.StatusBadGateway, "upstream payload is not a VRT document")
		return
	}

	h.mu.Lock()
	h.override = body
	h.fetchedAt = time.Now()
	h.mu.Unlock()

	// Reset the cached priority index so subsequent finding writes use the new
	// taxonomy.
	resetVRTIndex(body)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-VRT-Source", "upstream")
	w.Header().Set("X-VRT-Fetched-At", time.Now().UTC().Format(time.RFC3339))
	_, _ = w.Write(body)
}
