package trace

import (
	"strings"
	"testing"
)

func TestBufferCoalescesWhatArrivedBetweenFlushes(t *testing.T) {
	buffer := buffer{maxFrame: 100, maxPreview: 1000}
	buffer.add(`{"score":`)
	buffer.add(`88`)
	buffer.add(`}`)

	chunk, capped, more := buffer.take()
	if chunk != `{"score":88}` {
		t.Fatalf("chunk = %q", chunk)
	}
	if capped || more {
		t.Fatalf("capped = %v, more = %v", capped, more)
	}
}

func TestBufferSplitsOversizedFramesAndAsksForAnother(t *testing.T) {
	buffer := buffer{maxFrame: 4, maxPreview: 1000}
	buffer.add("abcdefghij")

	chunk, _, more := buffer.take()
	if chunk != "abcd" || !more {
		t.Fatalf("first chunk = %q, more = %v", chunk, more)
	}
	chunk, _, more = buffer.take()
	if chunk != "efgh" || !more {
		t.Fatalf("second chunk = %q, more = %v", chunk, more)
	}
	chunk, _, more = buffer.take()
	if chunk != "ij" || more {
		t.Fatalf("third chunk = %q, more = %v", chunk, more)
	}
}

func TestBufferStopsThePreviewAtItsCap(t *testing.T) {
	buffer := buffer{maxFrame: 100, maxPreview: 6}
	buffer.add("abcdefghij")

	chunk, capped, more := buffer.take()
	if chunk != "abcdef" {
		t.Fatalf("chunk = %q", chunk)
	}
	if !capped || more {
		t.Fatalf("capped = %v, more = %v", capped, more)
	}

	buffer.add("more text")
	chunk, capped, more = buffer.take()
	if chunk != "" || capped || more {
		t.Fatalf("a capped buffer still produced %q, %v, %v", chunk, capped, more)
	}
}

func TestBufferCutsOnRuneBoundaries(t *testing.T) {
	buffer := buffer{maxFrame: 3, maxPreview: 1000}
	buffer.add("çğü")

	chunk, _, more := buffer.take()
	if !more {
		t.Fatal("the split did not ask for another frame")
	}
	if chunk != "ç" {
		t.Fatalf("chunk = %q, want a whole rune", chunk)
	}
	if !strings.HasPrefix(buffer.pending, "ğ") {
		t.Fatalf("pending = %q", buffer.pending)
	}
}

func TestAnEmptyBufferProducesNothing(t *testing.T) {
	buffer := buffer{maxFrame: 10, maxPreview: 10}
	chunk, capped, more := buffer.take()
	if chunk != "" || capped || more {
		t.Fatalf("empty buffer produced %q, %v, %v", chunk, capped, more)
	}
}
