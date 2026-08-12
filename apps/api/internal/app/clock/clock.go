package clock

import "time"

type Clock interface {
	Now() time.Time
}

type System struct{}

func (System) Now() time.Time {
	return time.Now()
}

type Fixed struct {
	Time time.Time
}

func (f Fixed) Now() time.Time {
	return f.Time
}

func Stamp(source Clock) string {
	return source.Now().UTC().Format(time.RFC3339Nano)
}
