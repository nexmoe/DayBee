package audio

// Apple HAL detects speech after echo cancellation independently of Silero.
// Retain 200 ms before and after speech so detector latency and brief pauses do
// not clip words. Keep the sample clock intact by zeroing rejected audio.
type voiceActivityGate struct {
	pending  []float32
	keep     []bool
	hangover int
}

const voiceGatePadding = SampleRate / 5

func (g *voiceActivityGate) Push(samples []float32, activity int) []float32 {
	start := len(g.pending)
	g.pending = append(g.pending, samples...)
	g.keep = append(g.keep, make([]bool, len(samples))...)
	switch activity {
	case 1:
		for i := range g.keep {
			g.keep[i] = true
		}
		g.hangover = voiceGatePadding
	case 0:
		count := min(len(samples), g.hangover)
		for i := start; i < start+count; i++ {
			g.keep[i] = true
		}
		g.hangover -= count
	default:
		// Unsupported devices and read failures must preserve microphone audio.
		for i := range g.keep {
			g.keep[i] = true
		}
		g.hangover = 0
		return g.release(len(g.pending))
	}
	return g.release(max(0, len(g.pending)-voiceGatePadding))
}

func (g *voiceActivityGate) Flush() []float32 { return g.release(len(g.pending)) }

func (g *voiceActivityGate) release(count int) []float32 {
	if count == 0 {
		return nil
	}
	output := make([]float32, count)
	for i := range output {
		if g.keep[i] {
			output[i] = g.pending[i]
		}
	}
	g.pending = g.pending[:copy(g.pending, g.pending[count:])]
	g.keep = g.keep[:copy(g.keep, g.keep[count:])]
	return output
}
