//go:build !windows

package awake

import "time"

// awakeTime is Go's monotonic clock, which stops while the computer
// sleeps here.
func awakeTime() time.Duration { return goAwakeTime() }
