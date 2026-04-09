package blocker

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xr3ngar/veil/internal/categories"
	"github.com/0xr3ngar/veil/internal/config"
	"github.com/0xr3ngar/veil/internal/quotes"
)

type LogEntry struct {
	Domain      string    `json:"domain"`
	ClientIP    string    `json:"client_ip"`
	Timestamp   time.Time `json:"timestamp"`
	QuoteText   string    `json:"quote_text"`
	QuoteSource string    `json:"quote_source"`
}

type Blocker struct {
	mu      sync.RWMutex
	blocked map[string]struct{}
	allowed map[string]struct{}
	log     []LogEntry
	logPos  int
	logFull bool

	TotalBlocked atomic.Int64
	TotalAllowed atomic.Int64
}

const maxLogSize = 1000

func New() *Blocker {
	return &Blocker{
		blocked: make(map[string]struct{}),
		allowed: make(map[string]struct{}),
		log:     make([]LogEntry, maxLogSize),
	}
}

func (b *Blocker) Reload(cfg *config.Config) {
	blocked := make(map[string]struct{})
	allowed := make(map[string]struct{})

	for _, d := range cfg.CustomAllowed {
		allowed[normalize(d)] = struct{}{}
	}

	for _, d := range cfg.CustomBlocked {
		blocked[normalize(d)] = struct{}{}
	}

	for name, enabled := range cfg.Categories {
		if !enabled {
			continue
		}
		if categories.IsExternalList(name) {
			domains, err := categories.LoadExternalList(name)
			if err == nil {
				for _, d := range domains {
					blocked[normalize(d)] = struct{}{}
				}
			}
			continue
		}
		if list, ok := categories.All[name]; ok {
			for _, d := range list {
				blocked[normalize(d)] = struct{}{}
			}
		}
	}

	b.mu.Lock()
	b.blocked = blocked
	b.allowed = allowed
	b.mu.Unlock()
}

func (b *Blocker) IsBlocked(domain string) bool {
	domain = normalize(domain)

	b.mu.RLock()
	defer b.mu.RUnlock()

	parts := strings.Split(domain, ".")
	for i := range parts {
		check := strings.Join(parts[i:], ".")
		if _, ok := b.allowed[check]; ok {
			return false
		}
	}

	for i := range parts {
		check := strings.Join(parts[i:], ".")
		if _, ok := b.blocked[check]; ok {
			return true
		}
	}

	return false
}

func (b *Blocker) AddBlocked(domain string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.blocked[normalize(domain)] = struct{}{}
}

func (b *Blocker) RemoveBlocked(domain string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.blocked, normalize(domain))
}

func (b *Blocker) AddAllowed(domain string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.allowed[normalize(domain)] = struct{}{}
}

func (b *Blocker) RemoveAllowed(domain string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.allowed, normalize(domain))
}

func (b *Blocker) LogBlock(domain, clientIP string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	q := quotes.Random()
	b.log[b.logPos] = LogEntry{
		Domain:      domain,
		ClientIP:    clientIP,
		QuoteText:   q.Text,
		QuoteSource: q.Source,
		Timestamp:   time.Now(),
	}
	b.logPos = (b.logPos + 1) % maxLogSize
	if b.logPos == 0 {
		b.logFull = true
	}
}

func (b *Blocker) RecentLogs(n int) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	total := b.logPos
	if b.logFull {
		total = maxLogSize
	}
	if n > total {
		n = total
	}

	result := make([]LogEntry, n)
	for i := range n {
		idx := b.logPos - 1 - i
		if idx < 0 {
			idx += maxLogSize
		}
		result[i] = b.log[idx]
	}
	return result
}

func (b *Blocker) BlockedDomains() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	domains := make([]string, 0, len(b.blocked))
	for d := range b.blocked {
		domains = append(domains, d)
	}
	return domains
}

func normalize(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimSuffix(domain, ".")
	return domain
}
