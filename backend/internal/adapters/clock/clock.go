package clockadapter

import "time"

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type FixedClock struct {
	Fixed time.Time
}

func (c FixedClock) Now() time.Time { return c.Fixed }
