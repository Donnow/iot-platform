package devicesim

import "math/rand"

type Random interface {
	Float64() float64
}

func newRandom(seed int64, index int) Random {
	return rand.New(rand.NewSource(seed + int64(index)))
}

func randomDelta(random Random, fluctuation float64) float64 {
	if fluctuation == 0 {
		return 0
	}
	return (random.Float64()*2 - 1) * fluctuation
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
