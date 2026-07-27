package jellycompat

import (
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
}

func TestDeleteByUserID(t *testing.T) {
	t.Parallel()
	store := NewSessionStore(24*time.Hour, fixedNow)

	// Insert sessions for two different users.
	_ = store.Put(Session{Token: "aaa", StreamAppUserID: 1, Username: "alice"})
	_ = store.Put(Session{Token: "bbb", StreamAppUserID: 1, Username: "alice"})
	_ = store.Put(Session{Token: "ccc", StreamAppUserID: 2, Username: "bob"})

	store.DeleteByUserID(1)

	if _, ok := store.Get("aaa"); ok {
		t.Error("expected session aaa to be deleted")
	}
	if _, ok := store.Get("bbb"); ok {
		t.Error("expected session bbb to be deleted")
	}
	if _, ok := store.Get("ccc"); !ok {
		t.Error("expected session ccc to still exist")
	}
}

func TestGetSlidingWindow_ExtendsWhenBelowHalfTTL(t *testing.T) {
	t.Parallel()
	ttl := 30 * 24 * time.Hour // 30 days
	now := fixedNow()
	clock := func() time.Time { return now }
	store := NewSessionStore(ttl, clock)

	_ = store.Put(Session{Token: "tok1", StreamAppUserID: 1})

	// Advance time to 20 days (past the halfway point of 15 days).
	now = now.Add(20 * 24 * time.Hour)

	session, ok := store.Get("tok1")
	if !ok {
		t.Fatal("expected session to exist")
	}

	// ExpiresAt should be extended to now + ttl.
	expected := now.Add(ttl)
	if !session.ExpiresAt.Equal(expected) {
		t.Errorf("expected ExpiresAt = %v, got %v", expected, session.ExpiresAt)
	}
}

func TestGetSlidingWindow_NoExtensionAboveHalfTTL(t *testing.T) {
	t.Parallel()
	ttl := 30 * 24 * time.Hour
	now := fixedNow()
	clock := func() time.Time { return now }
	store := NewSessionStore(ttl, clock)

	_ = store.Put(Session{Token: "tok2", StreamAppUserID: 1})
	originalExpiry := now.Add(ttl)

	// Advance time to 10 days (before the halfway point of 15 days).
	now = now.Add(10 * 24 * time.Hour)

	session, ok := store.Get("tok2")
	if !ok {
		t.Fatal("expected session to exist")
	}

	// ExpiresAt should NOT have changed.
	if !session.ExpiresAt.Equal(originalExpiry) {
		t.Errorf("expected ExpiresAt = %v, got %v", originalExpiry, session.ExpiresAt)
	}
}

func TestGet_ExpiredSession_ReturnsNotFound(t *testing.T) {
	t.Parallel()
	now := fixedNow()
	clock := func() time.Time { return now }
	store := NewSessionStore(1*time.Hour, clock)

	_ = store.Put(Session{Token: "short-lived", StreamAppUserID: 1})

	// Advance past TTL.
	now = now.Add(2 * time.Hour)

	if _, ok := store.Get("short-lived"); ok {
		t.Error("expected expired session to not be returned")
	}
}
