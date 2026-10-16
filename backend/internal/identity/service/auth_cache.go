package service

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/actor"
	"balans/internal/platform/clock"
)

// authCacheTTL: perubahan user (nonaktif, ganti role) berlaku paling lambat 30 detik (Technical Guideline 9.1).
// Perubahan lewat proses ini langsung meng-invalidate cache.
const authCacheTTL = 30 * time.Second

type authEntry struct {
	userID       uuid.UUID
	revoked      bool
	active       bool
	tokenVersion int
	mfaPending   bool
	actor        actor.Actor
	loadedAt     time.Time
}

type authCache struct {
	clock   clock.Clock
	mu      sync.Mutex
	entries map[uuid.UUID]authEntry
}

func newAuthCache(c clock.Clock) *authCache {
	return &authCache{clock: c, entries: map[uuid.UUID]authEntry{}}
}

func (c *authCache) get(ctx context.Context, sessionID uuid.UUID, load func(context.Context, uuid.UUID) (authEntry, error)) (authEntry, error) {
	now := c.clock.Now()
	c.mu.Lock()
	e, ok := c.entries[sessionID]
	c.mu.Unlock()
	if ok && now.Sub(e.loadedAt) < authCacheTTL {
		return e, nil
	}
	e, err := load(ctx, sessionID)
	if err != nil {
		return authEntry{}, err
	}
	e.loadedAt = now
	c.mu.Lock()
	// Bersihkan entri kedaluwarsa agar map tidak tumbuh tanpa batas.
	if len(c.entries) > 1000 {
		for k, v := range c.entries {
			if now.Sub(v.loadedAt) >= authCacheTTL {
				delete(c.entries, k)
			}
		}
	}
	c.entries[sessionID] = e
	c.mu.Unlock()
	return e, nil
}

func (c *authCache) invalidateUser(userID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range c.entries {
		if v.userID == userID {
			delete(c.entries, k)
		}
	}
}

func (c *authCache) invalidateSessions(ids []uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range ids {
		delete(c.entries, id)
	}
}

func (c *authCache) invalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.entries)
}
