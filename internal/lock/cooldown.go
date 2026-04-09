package lock

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const CooldownDuration = 24 * time.Hour

type PendingUnblock struct {
	Domain    string    `json:"domain"`
	RequestAt time.Time `json:"request_at"`
	EffectAt  time.Time `json:"effect_at"`
}

type CooldownState struct {
	Pending []PendingUnblock `json:"pending"`
}

var cooldownMu sync.Mutex

func cooldownPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".veil", "pending.json")
}

func loadCooldown() (*CooldownState, error) {
	data, err := os.ReadFile(cooldownPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &CooldownState{}, nil
		}
		return nil, err
	}
	var state CooldownState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func saveCooldown(state *CooldownState) error {
	if err := os.MkdirAll(filepath.Dir(cooldownPath()), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cooldownPath(), data, 0644)
}

func RequestUnblock(domain string) (*PendingUnblock, error) {
	cooldownMu.Lock()
	defer cooldownMu.Unlock()

	state, err := loadCooldown()
	if err != nil {
		return nil, err
	}

	for _, p := range state.Pending {
		if p.Domain == domain {
			return &p, nil
		}
	}

	now := time.Now()
	pending := PendingUnblock{
		Domain:    domain,
		RequestAt: now,
		EffectAt:  now.Add(CooldownDuration),
	}
	state.Pending = append(state.Pending, pending)

	if err := saveCooldown(state); err != nil {
		return nil, err
	}
	return &pending, nil
}

func CancelUnblock(domain string) error {
	cooldownMu.Lock()
	defer cooldownMu.Unlock()

	state, err := loadCooldown()
	if err != nil {
		return err
	}

	filtered := state.Pending[:0]
	for _, p := range state.Pending {
		if p.Domain != domain {
			filtered = append(filtered, p)
		}
	}
	state.Pending = filtered
	return saveCooldown(state)
}

func GetPending() ([]PendingUnblock, error) {
	cooldownMu.Lock()
	defer cooldownMu.Unlock()

	state, err := loadCooldown()
	if err != nil {
		return nil, err
	}
	return state.Pending, nil
}

func ProcessExpired() ([]string, error) {
	cooldownMu.Lock()
	defer cooldownMu.Unlock()

	state, err := loadCooldown()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var expired []string
	remaining := state.Pending[:0]
	for _, p := range state.Pending {
		if now.After(p.EffectAt) {
			expired = append(expired, p.Domain)
		} else {
			remaining = append(remaining, p)
		}
	}
	state.Pending = remaining

	if err := saveCooldown(state); err != nil {
		return nil, err
	}
	return expired, nil
}
