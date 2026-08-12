package indexer

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const DefaultBinary = "pdftotext"

var (
	carriageReturns = regexp.MustCompile(`\r\n?`)
	trailingSpace   = regexp.MustCompile(`[ \t]+\n`)
	blankLines      = regexp.MustCompile(`\n{3,}`)
)

type Extractor struct {
	binary     string
	timeout    time.Duration
	maxBytes   int
	maxRunes   int
	lookupPath func(string) (string, error)
}

func NewExtractor(binary string, timeout time.Duration, maxBytes, maxRunes int) *Extractor {
	if binary == "" {
		binary = DefaultBinary
	}
	return &Extractor{
		binary:     binary,
		timeout:    timeout,
		maxBytes:   maxBytes,
		maxRunes:   maxRunes,
		lookupPath: exec.LookPath,
	}
}

func (e *Extractor) missing() error {
	return refuse(
		"cannot read the cv pdfs: poppler's " + e.binary + " is not on this machine, " +
			"nothing is indexed until it is installed",
	)
}

func (e *Extractor) Available() error {
	if _, err := e.lookupPath(e.binary); err != nil {
		return e.missing()
	}
	return nil
}

type capped struct {
	limit   int
	written int
	buffer  strings.Builder
}

func (c *capped) Write(chunk []byte) (int, error) {
	room := c.limit - c.written
	if room <= 0 {
		return len(chunk), nil
	}
	if room < len(chunk) {
		chunk = chunk[:room]
	}
	c.written += len(chunk)
	c.buffer.Write(chunk)
	return len(chunk), nil
}

func (e *Extractor) Text(ctx context.Context, path string) (string, error) {
	if err := e.Available(); err != nil {
		return "", err
	}
	bounded, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	stdout := &capped{limit: e.maxBytes}
	stderr := &capped{limit: 4096}
	command := exec.CommandContext(bounded, e.binary, "-layout", "-enc", "UTF-8", path, "-")
	command.Stdout = stdout
	command.Stderr = stderr

	if err := command.Run(); err != nil {
		return "", e.failure(bounded, path, stderr.buffer.String(), err)
	}
	text := clean(stdout.buffer.String())
	if text == "" {
		return "", refuse(
			"cannot read " + path + ": it carries no extractable text, it may be a scanned image",
		)
	}
	return clip(text, e.maxRunes), nil
}

func (e *Extractor) failure(ctx context.Context, path, stderr string, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return e.missing()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return refuse(
			"cannot read " + path + ": " + e.binary + " ran past " +
				strconv.Itoa(int(e.timeout.Seconds())) + " seconds",
		)
	}
	reason, _, _ := strings.Cut(strings.TrimSpace(stderr), "\n")
	if reason == "" {
		reason = e.binary + " failed"
	}
	return refuse("cannot read " + path + ": " + reason)
}

func clean(text string) string {
	text = carriageReturns.ReplaceAllString(text, "\n")
	text = trailingSpace.ReplaceAllString(text, "\n")
	return strings.TrimSpace(blankLines.ReplaceAllString(text, "\n\n"))
}

func clip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
