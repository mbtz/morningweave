package ranking

import (
	"math"
	"time"
)

const DefaultHalfLife = 24 * time.Hour

// RecencyScore returns a 0..1 score based on exponential decay.
func RecencyScore(timestamp time.Time, now time.Time, halfLife time.Duration) float64 {
	if timestamp.IsZero() {
		return 0
	}
	if halfLife <= 0 {
		halfLife = DefaultHalfLife
	}
	age := now.Sub(timestamp)
	if age <= 0 {
		return 1
	}
	decay := math.Exp(-math.Ln2 * float64(age) / float64(halfLife))
	return clamp01(decay)
}
