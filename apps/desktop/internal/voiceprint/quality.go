package voiceprint

const fullScale float32 = 0.99

// Clipped is true when more than 0.1% of samples satisfy |x| >= 0.99.
func Clipped(samples []float32) bool {
	n := len(samples)
	if n == 0 {
		return false
	}
	var hits int
	for _, sample := range samples {
		if sample <= -fullScale || sample >= fullScale {
			hits++
		}
	}
	return int64(hits)*1000 > int64(n)
}
