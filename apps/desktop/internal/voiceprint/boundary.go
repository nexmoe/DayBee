package voiceprint

import "sort"

// guardBoundaryOverlap runs after the production span review.
// A not_me window keeps its own samples. Where that window's [Start, End)
// still covers the following me span, those me samples become uncertain.
// The cut is the adjacent window's End, not a fixed delay, and it stops at
// the me span. Echo and raw-floor not_me are left as not_me.
func guardBoundaryOverlap(p *Profile, wins []WindowScore, scores []float32, echo []bool, spans []IdentitySpan, n int, voteHigh float32) []IdentitySpan {
	cuts := adjacentNotMeSupport(wins, spans)
	if len(cuts) == 0 {
		return spans
	}
	out := make([]IdentitySpan, 0, len(spans)+len(cuts))
	for _, span := range spans {
		if span.Identity != spanMe {
			out = append(out, span)
			continue
		}
		for _, part := range cutMeSpan(span, cuts) {
			out = append(out, recheckPiece(p, wins, scores, echo, part, n, voteHigh))
		}
	}
	return mergeTouching(out, wins, scores, echo, p, n, voteHigh)
}

func adjacentNotMeSupport(wins []WindowScore, spans []IdentitySpan) [][2]int {
	var cuts [][2]int
	for i := 0; i+1 < len(spans); i++ {
		if spans[i].Identity != spanNotMe || spans[i+1].Identity != spanMe || spans[i].End != spans[i+1].Start {
			continue
		}
		boundary := spans[i].End
		win := -1
		for w := range wins {
			if wins[w].Start < spans[i].Start || wins[w].Start >= boundary {
				continue
			}
			if win < 0 || wins[w].Start >= wins[win].Start {
				win = w
			}
		}
		if win < 0 || wins[win].End <= boundary {
			continue
		}
		end := wins[win].End
		if end > spans[i+1].End {
			end = spans[i+1].End
		}
		if end > boundary {
			cuts = append(cuts, [2]int{boundary, end})
		}
	}
	return cuts
}

func cutMeSpan(span IdentitySpan, cuts [][2]int) []IdentitySpan {
	ordered := append([][2]int(nil), cuts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i][0] < ordered[j][0] })
	cursor := span.Start
	var out []IdentitySpan
	for _, cut := range ordered {
		start, end := cut[0], cut[1]
		if end <= span.Start || start >= span.End {
			continue
		}
		if start < span.Start {
			start = span.Start
		}
		if end > span.End {
			end = span.End
		}
		if start > cursor {
			out = append(out, IdentitySpan{Start: cursor, End: start, Identity: spanMe})
		}
		if end > start {
			out = append(out, IdentitySpan{Start: start, End: end, Identity: spanUncertain})
			if end > cursor {
				cursor = end
			}
		}
	}
	if cursor < span.End {
		out = append(out, IdentitySpan{Start: cursor, End: span.End, Identity: spanMe})
	}
	if len(out) == 0 {
		return []IdentitySpan{span}
	}
	return out
}

func recheckPiece(p *Profile, wins []WindowScore, scores []float32, echo []bool, span IdentitySpan, n int, voteHigh float32) IdentitySpan {
	span.Score = ownedScore(wins, smoothScores(scores), n, span.Start, span.End)
	span.Anchor = false
	if span.Identity != spanMe {
		return span
	}
	bounds := spanBounds(wins, n)
	owned, flags := scoresForSpan(bounds, scores, echo, span)
	if len(owned) == 0 {
		span.Identity = spanUncertain
		span.Score = 0
		return span
	}
	voter := profileForVote(p, voteHigh)
	identity, _ := DecideProfile(voter, owned, flags)
	if identity != spanMe {
		if identity != spanNotMe && identity != spanUncertain {
			identity = spanUncertain
		}
		span.Identity = identity
		return span
	}
	confirmed, _ := DecideProfile(p, owned, flags)
	span.Anchor = confirmed == spanMe
	return span
}

func ownedScore(wins []WindowScore, scores []float32, n, start, end int) float32 {
	bounds := spanBounds(wins, n)
	var sum float64
	weight := 0
	for i := 0; i < len(scores) && i+1 < len(bounds); i++ {
		lo, hi := bounds[i], bounds[i+1]
		if lo < start {
			lo = start
		}
		if hi > end {
			hi = end
		}
		if hi <= lo {
			continue
		}
		sum += float64(scores[i]) * float64(hi-lo)
		weight += hi - lo
	}
	if weight == 0 {
		return 0
	}
	return float32(sum / float64(weight))
}

func mergeTouching(spans []IdentitySpan, wins []WindowScore, scores []float32, echo []bool, p *Profile, n int, voteHigh float32) []IdentitySpan {
	if len(spans) == 0 {
		return spans
	}
	out := []IdentitySpan{spans[0]}
	for _, span := range spans[1:] {
		last := &out[len(out)-1]
		if last.Identity == span.Identity && last.End == span.Start {
			last.End = span.End
			*last = recheckPiece(p, wins, scores, echo, *last, n, voteHigh)
			continue
		}
		out = append(out, span)
	}
	return out
}
