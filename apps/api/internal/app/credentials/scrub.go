package credentials

import "regexp"

var tokenPattern = regexp.MustCompile(`sk-ant-[a-z0-9-]*-[A-Za-z0-9_-]{12,}`)

func Scrub(text string) string {
	return tokenPattern.ReplaceAllString(text, "[redacted]")
}
