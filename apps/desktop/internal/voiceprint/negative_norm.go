package voiceprint

// speakerReferences drops the 32 windows of each speaker.
// Negative trials use the reference that excludes the query speaker.
func speakerReferences(cohort [][]float32, windows []CohortWindow) (map[string][][]float32, error) {
	if len(cohort) == 0 || len(cohort) != len(windows) {
		return nil, errCohortSize
	}
	counts := map[string]int{}
	for _, window := range windows {
		if window.Speaker == "" {
			return nil, errCohortSize
		}
		counts[window.Speaker]++
	}
	refs := make(map[string][][]float32, len(counts))
	for speaker, count := range counts {
		if count != cohortPerSpeaker {
			return nil, errCohortSize
		}
		ref, err := referenceWithoutSpeaker(cohort, windows, speaker)
		if err != nil {
			return nil, err
		}
		refs[speaker] = ref
	}
	return refs, nil
}

func referenceWithoutSpeaker(cohort [][]float32, windows []CohortWindow, speaker string) ([][]float32, error) {
	out := make([][]float32, 0, len(cohort)-cohortPerSpeaker)
	dropped := 0
	for i, window := range windows {
		if window.Speaker == speaker {
			dropped++
			continue
		}
		out = append(out, cohort[i])
	}
	if dropped != cohortPerSpeaker {
		return nil, errCohortSize
	}
	return out, nil
}

func negativeTestNorms(cohort [][]float32, windows []CohortWindow, refs map[string][][]float32) ([]sideStats, error) {
	out := make([]sideStats, len(cohort))
	for i, vector := range cohort {
		ref, ok := refs[windows[i].Speaker]
		if !ok {
			return nil, errCohortSize
		}
		mean, std, normOK := normTop(cohortCosines(vector, ref))
		if !normOK {
			return nil, ErrEnrollmentInseparable
		}
		out[i] = sideStats{mean, std}
	}
	return out, nil
}

func negativeEnrollNorms(model *Profile, windows []CohortWindow, refs map[string][][]float32) (map[string]sideStats, error) {
	out := make(map[string]sideStats, len(refs))
	seen := map[string]bool{}
	for _, window := range windows {
		if seen[window.Speaker] {
			continue
		}
		seen[window.Speaker] = true
		ref, ok := refs[window.Speaker]
		if !ok {
			return nil, errCohortSize
		}
		mean, std, normOK := normTop(cohortProfileScores(model, ref))
		if !normOK {
			return nil, ErrEnrollmentInseparable
		}
		out[window.Speaker] = sideStats{mean, std}
	}
	return out, nil
}

// negativeTrialNorms is the enroll-side and test-side normalization for one cohort impostor.
// Both sides omit that impostor's 32 cohort windows.
func negativeTrialNorms(model *Profile, cohort [][]float32, windows []CohortWindow, index int) (enroll, test sideStats, err error) {
	if index < 0 || index >= len(cohort) || index >= len(windows) {
		return sideStats{}, sideStats{}, errCohortSize
	}
	ref, err := referenceWithoutSpeaker(cohort, windows, windows[index].Speaker)
	if err != nil {
		return sideStats{}, sideStats{}, err
	}
	enrollMean, enrollStd, ok := normTop(cohortProfileScores(model, ref))
	if !ok {
		return sideStats{}, sideStats{}, ErrEnrollmentInseparable
	}
	testMean, testStd, ok := normTop(cohortCosines(cohort[index], ref))
	if !ok {
		return sideStats{}, sideStats{}, ErrEnrollmentInseparable
	}
	return sideStats{enrollMean, enrollStd}, sideStats{testMean, testStd}, nil
}
