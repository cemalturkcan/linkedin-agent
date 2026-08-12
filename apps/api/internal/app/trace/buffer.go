package trace

import "unicode/utf8"

type buffer struct {
	maxFrame   int
	maxPreview int
	pending    string
	sent       int
	capped     bool
}

func (b *buffer) add(fragment string) {
	if b.capped || fragment == "" {
		return
	}
	b.pending += fragment
}

func (b *buffer) pendingLen() int {
	return len(b.pending)
}

func (b *buffer) take() (chunk string, capped bool, more bool) {
	if b.capped || b.pending == "" {
		return "", false, false
	}

	chunk = b.pending
	if len(chunk) > b.maxFrame {
		cut := runeBoundary(chunk, b.maxFrame)
		chunk, b.pending = chunk[:cut], chunk[cut:]
		more = true
	} else {
		b.pending = ""
	}

	if b.sent+len(chunk) >= b.maxPreview {
		cut := runeBoundary(chunk, max(0, b.maxPreview-b.sent))
		chunk = chunk[:cut]
		b.capped = true
		b.pending = ""
		more = false
	}
	b.sent += len(chunk)
	return chunk, b.capped, more
}

func runeBoundary(value string, limit int) int {
	if limit >= len(value) {
		return len(value)
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return limit
}
