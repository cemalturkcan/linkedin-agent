package testutil

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"api/internal/app/clock"
	"api/internal/app/store"
)

const storeOpenBudget = 10 * time.Second

func RequireIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: it touches the store, so it does not run under -short")
	}
}

func NewStore(t *testing.T) *store.Store {
	t.Helper()
	RequireIntegration(t)
	return OpenStore(t, filepath.Join(t.TempDir(), "agent.db"))
}

func OpenStore(t *testing.T, path string) *store.Store {
	t.Helper()
	RequireIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), storeOpenBudget)
	defer cancel()

	opened, err := store.Open(ctx, path)
	if err != nil {
		t.Fatalf("open the store at %s: %v", path, err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	return opened
}

func FixedClock(stamp string) clock.Clock {
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		panic("testutil: " + stamp + " is not an RFC3339 instant")
	}
	return clock.Fixed{Time: at}
}

type MovingClock struct {
	at time.Time
}

func NewMovingClock(stamp string) *MovingClock {
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		panic("testutil: " + stamp + " is not an RFC3339 instant")
	}
	return &MovingClock{at: at}
}

func (c *MovingClock) Now() time.Time {
	return c.at
}

func (c *MovingClock) Advance(by time.Duration) {
	c.at = c.at.Add(by)
}
