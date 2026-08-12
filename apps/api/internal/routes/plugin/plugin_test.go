package plugin_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"api/internal/app/events"
	"api/internal/routes/plugin"
	"api/internal/routes/settings"
	"api/internal/testutil"
)

const boot = "2026-08-11T09:00:00Z"

func newService(t *testing.T, source *testutil.MovingClock) *plugin.Service {
	t.Helper()
	opened := testutil.NewStore(t)
	hub := events.NewHub(source)
	t.Cleanup(hub.Close)

	service := plugin.NewService(plugin.Dependencies{
		Reads:  opened.Reads(),
		Hub:    hub,
		Clock:  source,
		Config: plugin.Config{ConnectedWindow: plugin.ConnectedWindow},
	})
	t.Cleanup(service.Close)
	return service
}

func TestWithNoHelloTheExtensionHasNeverBeenSeen(t *testing.T) {
	service := newService(t, testutil.NewMovingClock(boot))

	state, err := service.State(context.Background())
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.EverSeen || state.Connected {
		t.Fatalf("state = %+v", state)
	}
	if state.Capabilities != nil {
		t.Fatal("an unseen extension reported a capability manifest")
	}
}

func TestAHelloReportsItsOwnIdAndManifest(t *testing.T) {
	service := newService(t, testutil.NewMovingClock(boot))
	id := strings.Repeat("b", 32)

	state, err := service.Hello(context.Background(), plugin.Hello{
		ID:      id,
		Version: "2.1.0",
		Capabilities: plugin.RawCapabilities{
			Fetch: plugin.RawFetch{
				Rings:  []string{"city", "worldwide"},
				Ranges: []string{"r86400"},
			},
		},
	})
	if err != nil {
		t.Fatalf("hello: %v", err)
	}

	if !state.EverSeen || !state.Connected {
		t.Fatalf("state = %+v", state)
	}
	if state.ID != id {
		t.Fatalf("id = %q, want the id the extension reported", state.ID)
	}
	if state.Version == nil || *state.Version != "2.1.0" {
		t.Fatalf("version = %v", state.Version)
	}
	if state.Hellos != 1 {
		t.Fatalf("hellos = %d", state.Hellos)
	}
	rings := state.Capabilities.Fetch.Rings
	if len(rings) != 2 || rings[0] != settings.RingCity || rings[1] != settings.RingWorldwide {
		t.Fatalf("rings = %v, want only what the extension reported", rings)
	}
	if len(state.Capabilities.Fetch.Ranges) != 1 {
		t.Fatalf("ranges = %v", state.Capabilities.Fetch.Ranges)
	}
}

func TestAQuietExtensionFlipsToDisconnected(t *testing.T) {
	source := testutil.NewMovingClock(boot)
	service := newService(t, source)

	if _, err := service.Hello(context.Background(), plugin.Hello{
		ID:      strings.Repeat("c", 32),
		Version: "1.0.0",
	}); err != nil {
		t.Fatalf("hello: %v", err)
	}

	source.Advance(plugin.ConnectedWindow - time.Minute)
	state, err := service.State(context.Background())
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if !state.Connected {
		t.Fatal("the extension fell off inside the window")
	}

	source.Advance(2 * time.Minute)
	state, err = service.State(context.Background())
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state.Connected {
		t.Fatal("a quiet extension stayed connected past the window")
	}
	if !state.EverSeen {
		t.Fatal("a quiet extension was forgotten rather than marked disconnected")
	}
}

func TestASecondHelloKeepsTheFirstSeenStampAndCountsUp(t *testing.T) {
	source := testutil.NewMovingClock(boot)
	service := newService(t, source)
	id := strings.Repeat("d", 32)

	first, err := service.Hello(context.Background(), plugin.Hello{ID: id, Version: "1.0.0"})
	if err != nil {
		t.Fatalf("first hello: %v", err)
	}
	source.Advance(time.Minute)
	second, err := service.Hello(context.Background(), plugin.Hello{ID: id, Version: "1.0.1"})
	if err != nil {
		t.Fatalf("second hello: %v", err)
	}

	if *first.FirstSeen != *second.FirstSeen {
		t.Fatalf("firstSeen moved from %q to %q", *first.FirstSeen, *second.FirstSeen)
	}
	if second.Hellos != 2 {
		t.Fatalf("hellos = %d", second.Hellos)
	}
	if *second.Version != "1.0.1" {
		t.Fatalf("version = %q", *second.Version)
	}
}

func TestAnUnusableIdIsIgnoredRatherThanStored(t *testing.T) {
	service := newService(t, testutil.NewMovingClock(boot))

	state, err := service.Hello(context.Background(), plugin.Hello{
		ID:      "not-a-chrome-extension-id",
		Version: "1.0.0",
	})
	if err != nil {
		t.Fatalf("hello: %v", err)
	}
	if state.ID != "" {
		t.Fatalf("id = %q, want it refused", state.ID)
	}
}

func TestAnAbsentManifestFallsBackToEverythingKnown(t *testing.T) {
	manifest := plugin.Normalize(plugin.RawCapabilities{})

	if len(manifest.Fetch.Rings) != len(plugin.KnownRings) {
		t.Fatalf("rings = %v", manifest.Fetch.Rings)
	}
	if len(manifest.Fetch.Ranges) != len(plugin.KnownRanges) {
		t.Fatalf("ranges = %v", manifest.Fetch.Ranges)
	}
	if manifest.Fetch.PageSize != 25 || manifest.Fetch.MaxPagesPerQuery != 8 {
		t.Fatalf("paging = %+v", manifest.Fetch)
	}
	if manifest.Fetch.EasyApply || manifest.Fetch.RemoteOnly || manifest.Fetch.Keyword {
		t.Fatalf("an absent manifest claimed a knob: %+v", manifest)
	}
}

func TestAReportedPageSizeIsBounded(t *testing.T) {
	huge := 5000
	manifest := plugin.Normalize(plugin.RawCapabilities{
		Fetch: plugin.RawFetch{PageSize: &huge},
	})
	if manifest.Fetch.PageSize != 100 {
		t.Fatalf("pageSize = %d, want it clamped", manifest.Fetch.PageSize)
	}
}
