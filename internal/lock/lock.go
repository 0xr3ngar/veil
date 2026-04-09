package lock

import (
	"crypto/hmac"
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

func machineKey() []byte {
	hostname, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	h := sha256.Sum256([]byte(hostname + home + "veil-lock-key"))
	return h[:]
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
	return os.WriteFile(lockPath(), data, 0644)
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
	return os.WriteFile(lockPath(), data, 0644)
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
