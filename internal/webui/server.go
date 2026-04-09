package webui

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/0xr3ngar/veil/internal/api"
	"github.com/0xr3ngar/veil/internal/blocker"
	"github.com/0xr3ngar/veil/internal/categories"
	"github.com/0xr3ngar/veil/internal/config"
	"github.com/0xr3ngar/veil/internal/lock"
	"github.com/0xr3ngar/veil/internal/quotes"
	webstatic "github.com/0xr3ngar/veil/web"
)

type Server struct {
	blocker *blocker.Blocker
	config  *config.Config
	api     *api.API
	start   time.Time
}

func NewServer(b *blocker.Blocker, cfg *config.Config, a *api.API) *Server {
	return &Server{
		blocker: b,
		config:  cfg,
		api:     a,
		start:   time.Now(),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// JSON API (kept for CLI) — register first with specific methods
	mux.HandleFunc("GET /api/stats", s.apiProxy)
	mux.HandleFunc("GET /api/logs", s.apiProxy)
	mux.HandleFunc("GET /api/config", s.apiProxy)
	mux.HandleFunc("PUT /api/config", s.apiProxy)
	mux.HandleFunc("GET /api/categories", s.apiProxy)
	mux.HandleFunc("PUT /api/categories/{name}", s.apiProxy)
	mux.HandleFunc("GET /api/domains", s.apiProxy)
	mux.HandleFunc("POST /api/domains/block", s.apiProxy)
	mux.HandleFunc("POST /api/domains/allow", s.apiProxy)
	mux.HandleFunc("DELETE /api/domains/{domain}", s.apiProxy)
	mux.HandleFunc("GET /api/lock", s.apiProxy)
	mux.HandleFunc("GET /api/pending", s.apiProxy)

	// Pages
	mux.HandleFunc("GET /{$}", s.servePage("index.html"))
	mux.HandleFunc("GET /domains", s.servePage("domains.html"))
	mux.HandleFunc("GET /settings", s.servePage("settings.html"))

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(webstatic.StaticFS)))

	// htmx partials
	mux.HandleFunc("GET /partial/stats", s.partialStats)
	mux.HandleFunc("GET /partial/logs", s.partialLogs)
	mux.HandleFunc("GET /partial/lock-badge", s.partialLockBadge)
	mux.HandleFunc("GET /partial/categories", s.partialCategories)
	mux.HandleFunc("GET /partial/domains/blocked", s.partialBlockedDomains)
	mux.HandleFunc("GET /partial/domains/allowed", s.partialAllowedDomains)
	mux.HandleFunc("GET /partial/pending", s.partialPending)
	mux.HandleFunc("GET /partial/settings-form", s.partialSettingsForm)
	mux.HandleFunc("GET /partial/lock-status", s.partialLockStatus)

	// htmx actions
	mux.HandleFunc("POST /partial/domains/block", s.actionBlockDomain)
	mux.HandleFunc("POST /partial/domains/allow", s.actionAllowDomain)
	mux.HandleFunc("DELETE /partial/domains/{domain}", s.actionRemoveDomain)
	mux.HandleFunc("PUT /partial/categories/{name}", s.actionToggleCategory)
	mux.HandleFunc("POST /partial/settings", s.actionSaveSettings)
	mux.HandleFunc("POST /partial/lock", s.actionSetLock)

	return rateLimitMiddleware(csrfMiddleware(mux))
}

func (s *Server) BlockedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve static CSS for the blocked page
		if strings.HasPrefix(r.URL.Path, "/static/") {
			http.StripPrefix("/static/", http.FileServerFS(webstatic.StaticFS)).ServeHTTP(w, r)
			return
		}

		q := quotes.Random()
		var apiPort string
		s.config.Read(func(cfg *config.Config) {
			apiPort = cfg.APIListen
		})
		// Extract just the port number
		if idx := strings.LastIndex(apiPort, ":"); idx >= 0 {
			apiPort = apiPort[idx+1:]
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data, _ := webstatic.StaticFS.ReadFile("blocked.html")
		page := strings.Replace(string(data), "{{PORT}}", apiPort, 1)
		page = strings.Replace(page, `id="quote"></div>`, fmt.Sprintf(`id="quote">"%s"</div>`, html.EscapeString(q.Text)), 1)
		page = strings.Replace(page, `id="source"></div>`, fmt.Sprintf(`id="source">— %s</div>`, html.EscapeString(q.Source)), 1)
		fmt.Fprint(w, page)
	})
}

func (s *Server) apiProxy(w http.ResponseWriter, r *http.Request) {
	s.api.Handler().ServeHTTP(w, r)
}

func (s *Server) servePage(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := webstatic.StaticFS.ReadFile(path)
		if err != nil {
			http.Error(w, "page not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	}
}

func (s *Server) partialStats(w http.ResponseWriter, r *http.Request) {
	blocked := s.blocker.TotalBlocked.Load()
	allowed := s.blocker.TotalAllowed.Load()
	total := blocked + allowed
	rate := 0.0
	if total > 0 {
		rate = float64(blocked) / float64(total) * 100
	}
	uptime := time.Since(s.start).Round(time.Second)

	fmt.Fprintf(w, `
<div class="stat-card blocked">
    <div class="stat-value">%d</div>
    <div class="stat-label">Blocked</div>
</div>
<div class="stat-card allowed">
    <div class="stat-value">%d</div>
    <div class="stat-label">Allowed</div>
</div>
<div class="stat-card ratio">
    <div class="stat-value">%.0f%%</div>
    <div class="stat-label">Block Rate</div>
</div>
<div class="stat-card uptime">
    <div class="stat-value">%s</div>
    <div class="stat-label">Uptime</div>
</div>`, blocked, allowed, rate, formatDuration(uptime))
}

func (s *Server) partialLogs(w http.ResponseWriter, r *http.Request) {
	logs := s.blocker.RecentLogs(50)
	if len(logs) == 0 {
		fmt.Fprint(w, `<tr><td colspan="3" class="empty">No blocked requests yet</td></tr>`)
		return
	}
	for _, entry := range logs {
		quoteHTML := ""
		if entry.QuoteText != "" {
			quoteHTML = fmt.Sprintf(`<div class="quote-text">"%s" — <em>%s</em></div>`,
				html.EscapeString(entry.QuoteText), html.EscapeString(entry.QuoteSource))
		}
		fmt.Fprintf(w, `<tr>
    <td class="time-cell">%s</td>
    <td class="domain-cell">%s%s</td>
    <td>%s</td>
</tr>`, entry.Timestamp.Format("15:04:05"), html.EscapeString(entry.Domain), quoteHTML, html.EscapeString(entry.ClientIP))
	}
}

func (s *Server) partialLockBadge(w http.ResponseWriter, r *http.Request) {
	if lock.IsLocked() {
		rem := lock.Remaining()
		fmt.Fprintf(w, `<span class="lock-icon locked">&#9670;</span><span>Locked — %s</span>`, formatDuration(rem))
	} else {
		fmt.Fprint(w, `<span class="lock-icon unlocked">&#9670;</span><span>Unlocked</span>`)
	}
}

func (s *Server) partialCategories(w http.ResponseWriter, r *http.Request) {
	s.config.Read(func(cfg *config.Config) {
		for name, enabled := range cfg.Categories {
			checked := ""
			if enabled {
				checked = "checked"
			}
			count := len(categories.All[name])
			if name == "adult" {
				count = 447000 // approximate
			}
			fmt.Fprintf(w, `
<div class="category-card">
    <div>
        <div class="category-name">%s</div>
        <div class="category-count">~%d domains</div>
    </div>
    <label class="toggle">
        <input type="checkbox" %s
               hx-put="/partial/categories/%s"
               hx-vals='js:{"enabled": event.target.checked}'
               hx-swap="none">
        <span class="toggle-slider"></span>
    </label>
</div>`, html.EscapeString(name), count, checked, html.EscapeString(name))
		}
	})
}

func (s *Server) partialBlockedDomains(w http.ResponseWriter, r *http.Request) {
	s.config.Read(func(cfg *config.Config) {
		if len(cfg.CustomBlocked) == 0 {
			fmt.Fprint(w, `<li class="domain-item"><span class="domain-name" style="color:var(--text-muted)">No custom blocked domains</span></li>`)
			return
		}
		for _, d := range cfg.CustomBlocked {
			fmt.Fprintf(w, `
<li class="domain-item">
    <span class="domain-name">%s</span>
    <button class="btn btn-remove"
            hx-delete="/partial/domains/%s"
            hx-target="#blocked-list"
            hx-swap="innerHTML">remove</button>
</li>`, html.EscapeString(d), html.EscapeString(d))
		}
	})
}

func (s *Server) partialAllowedDomains(w http.ResponseWriter, r *http.Request) {
	s.config.Read(func(cfg *config.Config) {
		if len(cfg.CustomAllowed) == 0 {
			fmt.Fprint(w, `<li class="domain-item"><span class="domain-name" style="color:var(--text-muted)">No allowed domains</span></li>`)
			return
		}
		for _, d := range cfg.CustomAllowed {
			fmt.Fprintf(w, `
<li class="domain-item">
    <span class="domain-name">%s</span>
    <button class="btn btn-remove"
            hx-delete="/partial/domains/%s"
            hx-target="#allowed-list"
            hx-swap="innerHTML">remove</button>
</li>`, html.EscapeString(d), html.EscapeString(d))
		}
	})
}

func (s *Server) partialPending(w http.ResponseWriter, r *http.Request) {
	pending, _ := lock.GetPending()
	if len(pending) == 0 {
		fmt.Fprint(w, `<li class="domain-item"><span class="domain-name" style="color:var(--text-muted)">No pending unblocks</span></li>`)
		return
	}
	for _, p := range pending {
		remaining := time.Until(p.EffectAt).Round(time.Minute)
		fmt.Fprintf(w, `
<li class="domain-item">
    <span><span class="domain-name">%s</span><span class="pending-time">unblocks in %s</span></span>
    <button class="btn btn-remove"
            onclick="fetch('/api/domains/%s', {method:'DELETE'}).then(()=>htmx.trigger(this.closest('ul'),'load'))">cancel</button>
</li>`, html.EscapeString(p.Domain), formatDuration(remaining), html.EscapeString(p.Domain))
	}
}

func (s *Server) partialSettingsForm(w http.ResponseWriter, r *http.Request) {
	s.config.Read(func(cfg *config.Config) {
		fmt.Fprintf(w, `
<h2>Configuration</h2>
<form class="settings-form" hx-post="/partial/settings" hx-swap="none">
    <div class="form-group">
        <label>Upstream DNS</label>
        <input type="text" name="upstream_dns" value="%s">
    </div>
    <div class="form-group">
        <label>Redirect To</label>
        <input type="text" name="redirect_to" value="%s">
    </div>
    <div class="form-group">
        <label>DNS Listen Address</label>
        <input type="text" name="dns_listen" value="%s" disabled>
    </div>
    <div class="form-group">
        <label>API Listen Address</label>
        <input type="text" name="api_listen" value="%s" disabled>
    </div>
    <button type="submit" class="btn btn-save">Save</button>
</form>`,
			html.EscapeString(cfg.UpstreamDNS),
			html.EscapeString(cfg.RedirectTo),
			html.EscapeString(cfg.DNSListen),
			html.EscapeString(cfg.APIListen))
	})
}

func (s *Server) partialLockStatus(w http.ResponseWriter, r *http.Request) {
	if lock.IsLocked() {
		state, _ := lock.GetLock()
		remaining := lock.Remaining()
		fmt.Fprintf(w, `
<div class="lock-info">
    <div class="lock-timer">%s remaining</div>
    <div class="lock-detail">Locked until %s</div>
    <div class="lock-detail">Config changes are restricted. You can still add domains to block.</div>
</div>`, formatDuration(remaining), state.LockedUntil.Format("Jan 2, 2006 15:04"))
	} else {
		fmt.Fprint(w, `
<div class="lock-info">
    <div class="lock-detail">Not locked. Set a time lock to prevent config changes.</div>
    <form class="lock-form" hx-post="/partial/lock" hx-target="closest .lock-section" hx-select=".lock-info" hx-swap="innerHTML">
        <select name="duration">
            <option value="1d">1 Day</option>
            <option value="7d">1 Week</option>
            <option value="14d">2 Weeks</option>
            <option value="30d">1 Month</option>
        </select>
        <button type="submit" class="btn btn-lock">Lock</button>
    </form>
</div>`)
	}
}

func (s *Server) actionBlockDomain(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimSpace(strings.ToLower(r.FormValue("domain")))
	if domain == "" {
		http.Error(w, "domain required", http.StatusBadRequest)
		return
	}
	s.config.Update(func(cfg *config.Config) {
		for _, d := range cfg.CustomBlocked {
			if d == domain {
				return
			}
		}
		cfg.CustomBlocked = append(cfg.CustomBlocked, domain)
	})
	s.config.Save()
	s.blocker.AddBlocked(domain)
	s.partialBlockedDomains(w, r)
}

func (s *Server) actionAllowDomain(w http.ResponseWriter, r *http.Request) {
	if lock.IsLocked() {
		http.Error(w, "locked", http.StatusForbidden)
		return
	}
	domain := strings.TrimSpace(strings.ToLower(r.FormValue("domain")))
	if domain == "" {
		http.Error(w, "domain required", http.StatusBadRequest)
		return
	}
	s.config.Update(func(cfg *config.Config) {
		for _, d := range cfg.CustomAllowed {
			if d == domain {
				return
			}
		}
		cfg.CustomAllowed = append(cfg.CustomAllowed, domain)
	})
	s.config.Save()
	s.blocker.AddAllowed(domain)
	s.partialAllowedDomains(w, r)
}

func (s *Server) actionRemoveDomain(w http.ResponseWriter, r *http.Request) {
	if lock.IsLocked() {
		http.Error(w, "locked", http.StatusForbidden)
		return
	}
	domain := r.PathValue("domain")
	s.config.Update(func(cfg *config.Config) {
		cfg.CustomBlocked = removeStr(cfg.CustomBlocked, domain)
		cfg.CustomAllowed = removeStr(cfg.CustomAllowed, domain)
	})
	s.config.Save()
	s.blocker.RemoveBlocked(domain)
	s.blocker.RemoveAllowed(domain)
	s.partialBlockedDomains(w, r)
}

func (s *Server) actionToggleCategory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	enabled := r.FormValue("enabled") == "true"

	if lock.IsLocked() && !enabled {
		http.Error(w, "cannot disable while locked", http.StatusForbidden)
		return
	}

	s.config.Update(func(cfg *config.Config) {
		cfg.Categories[name] = enabled
	})
	s.config.Save()
	s.blocker.Reload(s.config)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) actionSaveSettings(w http.ResponseWriter, r *http.Request) {
	if lock.IsLocked() {
		http.Error(w, "locked", http.StatusForbidden)
		return
	}
	s.config.Update(func(cfg *config.Config) {
		if v := r.FormValue("upstream_dns"); v != "" {
			cfg.UpstreamDNS = v
		}
		if v := r.FormValue("redirect_to"); v != "" {
			cfg.RedirectTo = v
		}
	})
	s.config.Save()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) actionSetLock(w http.ResponseWriter, r *http.Request) {
	dur := r.FormValue("duration")
	var d time.Duration
	switch dur {
	case "1d":
		d = 24 * time.Hour
	case "7d":
		d = 7 * 24 * time.Hour
	case "14d":
		d = 14 * 24 * time.Hour
	case "30d":
		d = 30 * 24 * time.Hour
	default:
		d = 24 * time.Hour
	}
	lock.SetLock(d)
	s.partialLockStatus(w, r)
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

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
