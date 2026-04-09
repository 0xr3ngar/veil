package lock

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type LockState struct {
	LockedAt    time.Time `json:"locked_at"`
	LockedUntil time.Time `json:"locked_until"`
	Signature   string    `json:"signature"`
}

func lockPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".veil", "lock.json")
}

func keyPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".veil", "lock.key")
}

func machineKey() []byte {
	data, err := os.ReadFile(keyPath())
	if err == nil && len(data) == 32 {
		return data
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		hostname, _ := os.Hostname()
		h := sha256.Sum256([]byte(hostname + fmt.Sprintf("%d", time.Now().UnixNano())))
		return h[:]
	}
	os.MkdirAll(filepath.Dir(keyPath()), 0700)
	os.WriteFile(keyPath(), key, 0600)
	return key
}

func sign(lockedAt, lockedUntil time.Time) string {
	data := fmt.Sprintf("%d:%d", lockedAt.Unix(), lockedUntil.Unix())
	mac := hmac.New(sha256.New, machineKey())
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func verify(state *LockState) bool {
	expected := sign(state.LockedAt, state.LockedUntil)
	return hmac.Equal([]byte(state.Signature), []byte(expected))
}

func SetLock(duration time.Duration) error {
	now := time.Now()
	state := &LockState{
		LockedAt:    now,
		LockedUntil: now.Add(duration),
	}
	state.Signature = sign(state.LockedAt, state.LockedUntil)

	if err := os.MkdirAll(filepath.Dir(lockPath()), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(lockPath(), data, 0600)
}

func ExtendLock(duration time.Duration) error {
	state, err := GetLock()
	if err != nil || !IsLocked() {
		return SetLock(duration)
	}

	newUntil := state.LockedUntil.Add(duration)
	if newUntil.Before(time.Now().Add(duration)) {
		newUntil = time.Now().Add(duration)
	}

	state.LockedUntil = newUntil
	state.Signature = sign(state.LockedAt, state.LockedUntil)

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(lockPath(), data, 0600)
}

func GetLock() (*LockState, error) {
	data, err := os.ReadFile(lockPath())
	if err != nil {
		return nil, err
	}

	var state LockState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	if !verify(&state) {
		return nil, fmt.Errorf("lock file has been tampered with")
	}

	return &state, nil
}

func IsLocked() bool {
	state, err := GetLock()
	if err != nil {
		return false
	}
	return time.Now().Before(state.LockedUntil)
}

func Remaining() time.Duration {
	state, err := GetLock()
	if err != nil {
		return 0
	}
	rem := time.Until(state.LockedUntil)
	if rem < 0 {
		return 0
	}
	return rem
}
