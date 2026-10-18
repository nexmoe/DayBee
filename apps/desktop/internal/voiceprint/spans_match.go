package voiceprint

// MatchSpans labels [0, n) without dropping samples.
// Score failures and audio shorter than a window become one uncertain span.
// Echo rejection reads the raw cosine on each window. DecisionScores are a
// separate scale and are the only scores passed to IdentitySpans.
func (m *Manager) MatchSpans(p *Profile, samples []float32, rate int, ref *Reference) []IdentitySpan {
	return m.MatchSpansRelaxed(p, samples, rate, ref, false)
}

// MatchSpansRelaxed lowers the enter-me threshold only when relax is set and
// the personal band has room. A me span still has to pass DecideProfile, so
// hysteresis cannot keep me alive after the 70% vote fails.
func (m *Manager) MatchSpansRelaxed(p *Profile, samples []float32, rate int, ref *Reference, relax bool) (spans []IdentitySpan) {
	n := len(samples)
	defer func() {
		if recover() != nil {
			spans = uncertainAudio(n)
		}
	}()
	if n == 0 || m == nil || p == nil {
		return uncertainAudio(n)
	}
	wins, err := m.Score(p, samples, rate)
	if err != nil || len(wins) == 0 {
		return uncertainAudio(n)
	}
	scores := DecisionScores(p, wins)
	var echo []bool
	if ref != nil && EchoWindows != nil {
		echo = EchoWindows(m, samples, ref, wins, rate)
	}
	return labelScoredSpans(p, wins, scores, echo, n, rate, relax)
}

// labelScoredSpans applies hysteresis at the possibly lowered high threshold,
// then rechecks me with that same high. Propagation still requires the
// original DecideProfile high, so a rescued span cannot seed the next chunk.
func labelScoredSpans(p *Profile, wins []WindowScore, scores []float32, echo []bool, n, rate int, relax bool) []IdentitySpan {
	low, high := DecisionThresholds(p)
	voteHigh := high
	if relax {
		voteHigh = relaxHigh(low, high)
	}
	spans := IdentitySpans(wins, scores, echo, n, rate, low, voteHigh)
	if len(spans) == 0 {
		return uncertainAudio(n)
	}
	spans = reviewMeSpans(p, wins, scores, echo, spans, n, voteHigh)
	return guardBoundaryOverlap(p, wins, scores, echo, spans, n, voteHigh)
}

func relaxHigh(low, high float32) float32 {
	if !(high > low) {
		return high
	}
	cut := (high - low) / 2
	if cut > 0.3 {
		cut = 0.3
	}
	return high - cut
}

func reviewMeSpans(p *Profile, wins []WindowScore, scores []float32, echo []bool, spans []IdentitySpan, n int, voteHigh float32) []IdentitySpan {
	voter := profileForVote(p, voteHigh)
	bounds := spanBounds(wins, n)
	for i, span := range spans {
		if span.Identity != spanMe {
			continue
		}
		owned, flags := scoresForSpan(bounds, scores, echo, span)
		if len(owned) == 0 {
			spans[i].Identity = spanUncertain
			spans[i].Score = 0
			spans[i].Anchor = false
			continue
		}
		identity, mean := DecideProfile(voter, owned, flags)
		if identity != spanMe {
			if identity != spanNotMe && identity != spanUncertain {
				identity = spanUncertain
			}
			spans[i].Identity = identity
			spans[i].Score = mean
			spans[i].Anchor = false
			continue
		}
		confirmed, _ := DecideProfile(p, owned, flags)
		spans[i].Anchor = confirmed == spanMe
	}
	return spans
}

// profileForVote clones a calibrated profile so the 70% vote uses voteHigh.
// The caller's profile and its calibration stay unchanged.
func profileForVote(p *Profile, voteHigh float32) *Profile {
	if p == nil || p.Calibration == nil || p.Calibration.THigh == voteHigh {
		return p
	}
	voted := clone(p)
	voted.Calibration.THigh = voteHigh
	return voted
}

func scoresForSpan(bounds []int, scores []float32, echo []bool, span IdentitySpan) ([]float32, []bool) {
	owned := make([]float32, 0, len(scores))
	flags := make([]bool, 0, len(scores))
	for i := 0; i < len(scores) && i+1 < len(bounds); i++ {
		if bounds[i+1] <= span.Start || bounds[i] >= span.End || bounds[i+1] <= bounds[i] {
			continue
		}
		owned = append(owned, scores[i])
		flags = append(flags, echo != nil && i < len(echo) && echo[i])
	}
	return owned, flags
}

func uncertainAudio(n int) []IdentitySpan {
	if n <= 0 {
		return nil
	}
	return []IdentitySpan{{Start: 0, End: n, Identity: spanUncertain}}
}
