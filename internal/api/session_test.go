package api

import (
	"testing"
	"time"
)

// resetSessions clears global session state so each test starts clean.
func resetSessions(t *testing.T) {
	t.Helper()
	sessionsMu.Lock()
	sessions = make(map[string]time.Time)
	sessionsMu.Unlock()
}

func TestValidSessionAcceptsLiveToken(t *testing.T) {
	resetSessions(t)

	addSession("live-token")

	if !validSession("live-token") {
		t.Error("expected a freshly added token to be valid")
	}
	if validSession("never-issued") {
		t.Error("expected an unknown token to be rejected")
	}
}

func TestValidSessionRejectsAndEvictsExpiredToken(t *testing.T) {
	resetSessions(t)

	sessionsMu.Lock()
	sessions["stale-token"] = time.Now().Add(-time.Minute)
	sessionsMu.Unlock()

	if validSession("stale-token") {
		t.Error("expected an expired token to be rejected")
	}

	sessionsMu.RLock()
	_, still := sessions["stale-token"]
	sessionsMu.RUnlock()
	if still {
		t.Error("expected an expired token to be deleted on access")
	}
}

func TestAddSessionUsesConfiguredTTL(t *testing.T) {
	resetSessions(t)

	before := time.Now()
	addSession("ttl-token")

	sessionsMu.RLock()
	expiry := sessions["ttl-token"]
	sessionsMu.RUnlock()

	// The expiry must land within sessionTTL of when it was created, which also
	// pins it to the cookie MaxAge the login handler sets.
	if expiry.Before(before.Add(sessionTTL)) || expiry.After(time.Now().Add(sessionTTL)) {
		t.Errorf("expiry %v is not ~%v from now", expiry, sessionTTL)
	}
}

func TestAddSessionPrunesExpiredBeforeEvicting(t *testing.T) {
	resetSessions(t)

	// Fill to capacity with entries that have already expired.
	sessionsMu.Lock()
	for i := 0; i < maxSessions; i++ {
		sessions[string(rune(i))+"-expired"] = time.Now().Add(-time.Hour)
	}
	sessionsMu.Unlock()

	addSession("fresh-token")

	sessionsMu.RLock()
	size := len(sessions)
	_, ok := sessions["fresh-token"]
	sessionsMu.RUnlock()

	if !ok {
		t.Error("expected the new token to be stored")
	}
	// Every pre-existing entry was expired, so pruning alone should have made room.
	if size != 1 {
		t.Errorf("expected expired entries to be pruned, got %d entries", size)
	}
}

func TestAddSessionBoundsMapWhenAllEntriesAreLive(t *testing.T) {
	resetSessions(t)

	sessionsMu.Lock()
	for i := 0; i < maxSessions; i++ {
		sessions[string(rune(i))+"-live"] = time.Now().Add(sessionTTL)
	}
	sessionsMu.Unlock()

	addSession("overflow-token")

	sessionsMu.RLock()
	size := len(sessions)
	_, ok := sessions["overflow-token"]
	sessionsMu.RUnlock()

	if !ok {
		t.Error("expected the new token to be stored even at capacity")
	}
	if size > maxSessions {
		t.Errorf("session map grew past the %d bound: %d entries", maxSessions, size)
	}
}

func TestEvictLoginAttemptsBoundsMap(t *testing.T) {
	loginAttemptsMu.Lock()
	loginAttempts = make(map[string]*loginAttempt)
	// Fill past capacity with entries that are blocked and freshly active, so
	// the opportunistic prune cannot free anything.
	for i := 0; i < maxLimiterEntries; i++ {
		loginAttempts[string(rune(i))] = &loginAttempt{
			blockedUntil: time.Now().Add(time.Hour),
			lastAttempt:  time.Now(),
		}
	}
	evictLoginAttemptsLocked()
	size := len(loginAttempts)
	loginAttempts = make(map[string]*loginAttempt)
	loginAttemptsMu.Unlock()

	if size >= maxLimiterEntries {
		t.Errorf("expected eviction to free a slot below %d, got %d", maxLimiterEntries, size)
	}
}
