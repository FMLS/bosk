// Package clock implements the time port.
package clock

import "time"

// System reads the wall clock.
type System struct{}

// New returns a clock backed by the system time.
func New() System { return System{} }

func (System) Now() time.Time { return time.Now() }
