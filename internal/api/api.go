package api

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/0xr3ngar/veil/internal/blocker"
	"github.com/0xr3ngar/veil/internal/config"
	"github.com/0xr3ngar/veil/internal/lock"
)

type API struct {
	blocker   *blocker.Blocker
	config    *config.Config
	startTime time.Time
	mux       *http.ServeMux
}

func New(b *blocker.Blocker, cfg *config.Config) *API {
	a := &API{
		blocker:   b,
		config:    cfg,
		startTime: time.Now(),
		mux:       http.NewServeMux(),
	}
	a.routes()
	return a
}

func (a *API) Handler() http.Handler {
	return a.mux
}

func (a *API) routes() {
	a.mux.HandleFunc("GET /api/stats", a.handleStats)
	a.mux.HandleFunc("GET /api/logs", a.handleLogs)
	a.mux.HandleFunc("GET /api/config", a.handleGetConfig)
	a.mux.HandleFunc("PUT /api/config", a.handleUpdateConfig)
	a.mux.HandleFunc("GET /api/categories", a.handleGetCategories)
	a.mux.HandleFunc("PUT /api/categories/{name}", a.handleToggleCategory)
	a.mux.HandleFunc("GET /api/domains", a.handleGetDomains)
	a.mux.HandleFunc("POST /api/domains/block", a.handleBlockDomain)
	a.mux.HandleFunc("POST /api/domains/allow", a.handleAllowDomain)
	a.mux.HandleFunc("DELETE /api/domains/{domain}", a.handleRemoveDomain)
	a.mux.HandleFunc("GET /api/lock", a.handleGetLock)
	a.mux.HandleFunc("GET /api/pending", a.handleGetPending)
}

func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"total_blocked":  a.blocker.TotalBlocked.Load(),
		"total_allowed":  a.blocker.TotalAllowed.Load(),
		"uptime_seconds": int(time.Since(a.startTime).Seconds()),
		"locked":         lock.IsLocked(),
		"lock_remaining": lock.Remaining().String(),
	})
}

func (a *API) handleLogs(w http.ResponseWriter, r *http.Request) {
	logs := a.blocker.RecentLogs(100)
	writeJSON(w, logs)
}

func (a *API) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	a.config.Read(func(cfg *config.Config) {
		writeJSON(w, map[string]any{
			"upstream_dns": cfg.UpstreamDNS,
			"redirect_to":  cfg.RedirectTo,
			"dns_listen":   cfg.DNSListen,
			"api_listen":   cfg.APIListen,
			"log_blocked":  cfg.LogBlocked,
		})
	})
}

func (a *API) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if lock.IsLocked() {
		http.Error(w, "config is locked", http.StatusForbidden)
		return
	}

	var update struct {
		UpstreamDNS *string `json:"upstream_dns"`
		RedirectTo  *string `json:"redirect_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	a.config.Update(func(cfg *config.Config) {
		if update.UpstreamDNS != nil {
			cfg.UpstreamDNS = *update.UpstreamDNS
		}
		if update.RedirectTo != nil {
			cfg.RedirectTo = *update.RedirectTo
		}
	})

	if err := a.config.Save(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handleGetCategories(w http.ResponseWriter, r *http.Request) {
	a.config.Read(func(cfg *config.Config) {
		writeJSON(w, cfg.Categories)
	})
}

func (a *API) handleToggleCategory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if lock.IsLocked() && !body.Enabled {
		http.Error(w, "cannot disable categories while locked", http.StatusForbidden)
		return
	}

	a.config.Update(func(cfg *config.Config) {
		cfg.Categories[name] = body.Enabled
	})

	if err := a.config.Save(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.blocker.Reload(a.config)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handleGetDomains(w http.ResponseWriter, r *http.Request) {
	a.config.Read(func(cfg *config.Config) {
		writeJSON(w, map[string]any{
			"blocked": cfg.CustomBlocked,
			"allowed": cfg.CustomAllowed,
		})
	})
}

func (a *API) handleBlockDomain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	domain := strings.TrimSpace(strings.ToLower(body.Domain))
	if domain == "" || !isValidDomain(domain) {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}

	a.config.Update(func(cfg *config.Config) {
		cfg.CustomBlocked = appendUnique(cfg.CustomBlocked, domain)
	})

	if err := a.config.Save(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.blocker.AddBlocked(domain)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handleAllowDomain(w http.ResponseWriter, r *http.Request) {
	if lock.IsLocked() {
		http.Error(w, "cannot add allowed domains while locked", http.StatusForbidden)
		return
	}

	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	domain := strings.TrimSpace(strings.ToLower(body.Domain))
	if domain == "" || !isValidDomain(domain) {
		http.Error(w, "invalid domain", http.StatusBadRequest)
		return
	}

	a.config.Update(func(cfg *config.Config) {
		cfg.CustomAllowed = appendUnique(cfg.CustomAllowed, domain)
	})

	if err := a.config.Save(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.blocker.AddAllowed(domain)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handleRemoveDomain(w http.ResponseWriter, r *http.Request) {
	if lock.IsLocked() {
		http.Error(w, "cannot remove domains while locked", http.StatusForbidden)
		return
	}

	domain := r.PathValue("domain")

	a.config.Update(func(cfg *config.Config) {
		cfg.CustomBlocked = removeStr(cfg.CustomBlocked, domain)
		cfg.CustomAllowed = removeStr(cfg.CustomAllowed, domain)
	})

	if err := a.config.Save(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	a.blocker.RemoveBlocked(domain)
	a.blocker.RemoveAllowed(domain)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handleGetLock(w http.ResponseWriter, r *http.Request) {
	state, err := lock.GetLock()
	if err != nil {
		writeJSON(w, map[string]any{
			"locked":    false,
			"remaining": "0s",
		})
		return
	}
	writeJSON(w, map[string]any{
		"locked":       lock.IsLocked(),
		"locked_at":    state.LockedAt,
		"locked_until": state.LockedUntil,
		"remaining":    lock.Remaining().String(),
	})
}

func (a *API) handleGetPending(w http.ResponseWriter, r *http.Request) {
	pending, err := lock.GetPending()
	if err != nil {
		writeJSON(w, []any{})
		return
	}
	writeJSON(w, pending)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

func appendUnique(slice []string, val string) []string {
	if slices.Contains(slice, val) {
		return slice
	}
	return append(slice, val)
}

var validDomain = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

func isValidDomain(domain string) bool {
	return len(domain) <= 253 && validDomain.MatchString(domain)
}

func removeStr(slice []string, val string) []string {
	result := slice[:0]
	for _, s := range slice {
		if s != val {
			result = append(result, s)
		}
	}
	return result
}
