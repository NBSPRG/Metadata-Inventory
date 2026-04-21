package observability

import "time"

// Timer provides a simple way to measure elapsed time for metrics.
type Timer struct {
	start time.Time
}

// NewTimer starts a new timer.
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// Elapsed returns the elapsed time in seconds since the timer was created.
func (t *Timer) Elapsed() float64 {
	return time.Since(t.start).Seconds()
}
