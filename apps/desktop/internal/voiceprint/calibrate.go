package voiceprint

import (
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

const calibratedVersion = 3

const (
	acceptFalseRate = 0.01
	rejectFalseRate = 0.05
	hysteresisCap   = 0.3
)

var (
	ErrEnrollmentInseparable = errors.New("The enrollment does not separate from other speakers. Record your voice again in a quiet place.")
	ErrEnrollmentDevice      = errors.New("A specific microphone is required to save this voiceprint.")
	ErrEnrollmentSegments    = errors.New("Voiceprint enrollment needs separate recording segments.")
)

// EnrollmentContext is the capture device and echo setting used for enrollment.
// Device must be a resolved device ID. An empty ID or the literal "default" is rejected.
type EnrollmentContext struct {
	Device           string
	EchoCancellation bool
}

// Calibration is the personal decision band on the normalized score scale.
// CohortID is the SHA-256 of the bundled cohort blob used to place the band.
type Calibration struct {
	TLow     float32 `json:"tLow"`
	THigh    float32 `json:"tHigh"`
	Model    string  `json:"model"`
	CohortID string  `json:"cohortId"`
}

// Calibrate builds a version 3 profile from independent enrollment segments.
// Each group is one segment. Windows inside a group are not scored against each other.
func Calibrate(groups [][][]float32, context EnrollmentContext) (*Profile, error) {
	if !resolvedDevice(context.Device) {
		return nil, ErrEnrollmentDevice
	}
	if err := validateGroups(groups); err != nil {
		return nil, err
	}
	cohort, meta, err := LoadCohort()
	if err != nil {
		return nil, err
	}
	if meta.BlobSHA256 == "" {
		return nil, errCohortHash
	}
	genuine, impostor, err := crossScores(groups, cohort, meta.Windows)
	if err != nil {
		return nil, err
	}
	fa, fr, err := rateBounds(genuine, impostor)
	if err != nil {
		return nil, err
	}
	low, high, err := hysteresisThresholds(fa, fr)
	if err != nil {
		return nil, err
	}
	if !bandOK(&Calibration{TLow: low, THigh: high, Model: ModelName}) {
		return nil, ErrEnrollmentInseparable
	}
	if falseAccept(impostor, float64(low)) > acceptFalseRate+1e-12 || falseReject(genuine, float64(high)) > rejectFalseRate+1e-12 {
		return nil, ErrEnrollmentInseparable
	}
	return &Profile{
		Version: calibratedVersion, Model: ModelName, Embeddings: collectEmbeddings(groups),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Device: strings.TrimSpace(context.Device),
		EchoCancellation: context.EchoCancellation,
		Calibration:      &Calibration{TLow: low, THigh: high, Model: ModelName, CohortID: meta.BlobSHA256},
	}, nil
}

// Compatible reports whether a profile may label audio captured in this context.
// Version 1 and 2 profiles stay usable. Version 3 must match the resolved device and echo setting.
func Compatible(profile *Profile, context EnrollmentContext) bool {
	if !profileAccepted(profile) {
		return false
	}
	if profile.Version < calibratedVersion || profile.Calibration == nil {
		return true
	}
	return resolvedDevice(context.Device) &&
		strings.TrimSpace(profile.Device) == strings.TrimSpace(context.Device) &&
		profile.EchoCancellation == context.EchoCancellation
}

// DecisionScores returns one score per window on the scale used by DecisionThresholds.
// Version 1 and 2 keep the raw cosine. A version 3 profile whose calibration or cohort
// ID cannot be used returns an unacceptable score instead of that cosine.
func DecisionScores(profile *Profile, windows []WindowScore) []float32 {
	out := make([]float32, len(windows))
	if legacyProfile(profile) {
		for i, window := range windows {
			out[i] = window.Score
		}
		return out
	}
	cohort, ok := loadedCohortFor(profile)
	if !ok {
		for i := range out {
			out[i] = asNormMin
		}
		return out
	}
	enrollMean, enrollStd, normOK := normTop(cohortProfileScores(profile, cohort))
	if !normOK {
		for i := range out {
			out[i] = asNormMin
		}
		return out
	}
	for i, window := range windows {
		out[i] = normalizedWindow(profile, window.Embedding, cohort, enrollMean, enrollStd)
	}
	return out
}

// DecisionThresholds returns the low and high cutoffs on the DecisionScores scale.
// Version 1 and 2 use the single raw-cosine threshold. A version 3 profile with a
// numeric band keeps that band. A broken version 3 band is inverted, not legacy.
func DecisionThresholds(profile *Profile) (low, high float32) {
	if legacyProfile(profile) {
		threshold := Threshold(profile.Model)
		return threshold, threshold
	}
	if profile != nil && profile.Version >= calibratedVersion && bandOK(profile.Calibration) {
		return profile.Calibration.TLow, profile.Calibration.THigh
	}
	return 1, 0
}

func legacyProfile(profile *Profile) bool {
	return profile != nil && (profile.Version == 1 || profile.Version == 2)
}

func bandOK(calibration *Calibration) bool {
	return calibration != nil && calibration.Model == ModelName && finite32(calibration.TLow) && finite32(calibration.THigh) && asNormMin < calibration.TLow && calibration.TLow < calibration.THigh && calibration.THigh <= asNormMax
}

func loadedCohortFor(profile *Profile) ([][]float32, bool) {
	if profile == nil || profile.Version < calibratedVersion || !calibrationOK(profile) {
		return nil, false
	}
	vectors, meta, err := LoadCohort()
	if err != nil || meta.BlobSHA256 == "" || meta.BlobSHA256 != profile.Calibration.CohortID {
		return nil, false
	}
	return vectors, true
}

func validateGroups(groups [][][]float32) error {
	if len(groups) < 3 {
		return ErrEnrollmentSegments
	}
	for _, group := range groups {
		if len(group) == 0 {
			return ErrEnrollmentSegments
		}
		for _, vector := range group {
			if !validEmbedding(vector) {
				return errors.New("A valid speaker embedding is required.")
			}
		}
	}
	return nil
}

func crossScores(groups [][][]float32, cohort [][]float32, windows []CohortWindow) (genuine, impostor []float64, err error) {
	refs, err := speakerReferences(cohort, windows)
	if err != nil {
		return nil, nil, err
	}
	testSide, err := negativeTestNorms(cohort, windows, refs)
	if err != nil {
		return nil, nil, err
	}
	for held := range groups {
		model := &Profile{Version: calibratedVersion, Model: ModelName, Embeddings: otherEmbeddings(groups, held)}
		if len(model.Embeddings) == 0 {
			return nil, nil, ErrEnrollmentSegments
		}
		enrollMean, enrollStd, enrollOK := normTop(cohortProfileScores(model, cohort))
		if !enrollOK {
			return nil, nil, ErrEnrollmentInseparable
		}
		for _, vector := range groups[held] {
			score, scoreOK := normalizedValue(model, vector, cohort, enrollMean, enrollStd)
			if !scoreOK {
				return nil, nil, ErrEnrollmentInseparable
			}
			genuine = append(genuine, score)
		}
	}
	saved := &Profile{Version: calibratedVersion, Model: ModelName, Embeddings: collectEmbeddings(groups)}
	enrollSide, err := negativeEnrollNorms(saved, windows, refs)
	if err != nil {
		return nil, nil, err
	}
	for i, vector := range cohort {
		stats, known := enrollSide[windows[i].Speaker]
		if !known {
			return nil, nil, errCohortSize
		}
		score, scoreOK := gatedNorm(ProfileScore(saved, vector), stats.mean, stats.std, testSide[i].mean, testSide[i].std)
		if !scoreOK {
			return nil, nil, ErrEnrollmentInseparable
		}
		impostor = append(impostor, score)
	}
	if len(genuine) == 0 || len(impostor) == 0 {
		return nil, nil, ErrEnrollmentInseparable
	}
	return genuine, impostor, nil
}

func normalizedWindow(profile *Profile, vector []float32, cohort [][]float32, enrollMean, enrollStd float64) float32 {
	value, ok := normalizedValue(profile, vector, cohort, enrollMean, enrollStd)
	if !ok {
		return asNormMin
	}
	return float32(value)
}

func normalizedValue(profile *Profile, vector []float32, cohort [][]float32, enrollMean, enrollStd float64) (float64, bool) {
	raw := ProfileScore(profile, vector)
	if !finite32(raw) || raw < Threshold(ModelName) {
		return float64(asNormMin), true
	}
	testMean, testStd, ok := normTop(cohortCosines(vector, cohort))
	if !ok {
		return 0, false
	}
	return gatedNorm(raw, enrollMean, enrollStd, testMean, testStd)
}

func gatedNorm(raw float32, enrollMean, enrollStd, testMean, testStd float64) (float64, bool) {
	if !finite32(raw) || raw < Threshold(ModelName) {
		return float64(asNormMin), true
	}
	value := asNorm(float64(raw), enrollMean, enrollStd, testMean, testStd)
	if !finite32(value) {
		return 0, false
	}
	return float64(value), true
}

func rateBounds(genuine, impostor []float64) (fa, fr float64, err error) {
	fa, okFA := thresholdAtFA(impostor, acceptFalseRate)
	fr, okFR := thresholdFRUpper(genuine, rejectFalseRate)
	if !okFA || !okFR || fa > fr {
		return 0, 0, ErrEnrollmentInseparable
	}
	return fa, fr, nil
}

// hysteresisThresholds sets low at the FA≤1% point and keeps a short band above it.
// high = fa + min(0.3, (fr-fa)/2). A float32 band that collapses is rejected.
func hysteresisThresholds(fa, fr float64) (low, high float32, err error) {
	if math.IsNaN(fa) || math.IsInf(fa, 0) || math.IsNaN(fr) || math.IsInf(fr, 0) || fa > fr {
		return 0, 0, ErrEnrollmentInseparable
	}
	band := (fr - fa) / 2
	if band > hysteresisCap {
		band = hysteresisCap
	}
	low = float32(fa)
	high = float32(fa + band)
	if !finite32(low) || !finite32(high) || high <= low {
		return 0, 0, ErrEnrollmentInseparable
	}
	return low, high, nil
}

func thresholdAtFA(nontarget []float64, maxFA float64) (float64, bool) {
	n := len(nontarget)
	if n == 0 {
		return 0, false
	}
	scores := append([]float64(nil), nontarget...)
	sort.Float64s(scores)
	for _, score := range scores {
		if math.IsNaN(score) || math.IsInf(score, 0) {
			return 0, false
		}
	}
	allowed := int(math.Floor(maxFA*float64(n) + 1e-12))
	if allowed >= n {
		return 0, false
	}
	best := math.Inf(1)
	for i := n - 1; i >= 0; {
		j := i
		for j > 0 && scores[j-1] == scores[i] {
			j--
		}
		if n-j > allowed {
			break
		}
		best = scores[i]
		i = j - 1
	}
	if math.IsInf(best, 1) {
		max32 := float32(scores[n-1])
		if !finite32(max32) {
			return 0, false
		}
		best = float64(math.Nextafter32(max32, float32(math.Inf(1))))
	}
	return stabilizeFA(best, scores, maxFA)
}

// stabilizeFA keeps the false-accept cutoff on a float32 boundary whose measured FAR still holds.
// Nextafter(float64) can fall back to the original max once it is cast to float32.
func stabilizeFA(threshold float64, scores []float64, maxFA float64) (float64, bool) {
	low := float32(threshold)
	if !finite32(low) {
		return 0, false
	}
	if float64(low) < threshold {
		low = math.Nextafter32(low, float32(math.Inf(1)))
	}
	for finite32(low) && falseAccept(scores, float64(low)) > maxFA+1e-12 {
		next := math.Nextafter32(low, float32(math.Inf(1)))
		if next == low || !finite32(next) {
			return 0, false
		}
		low = next
	}
	if !finite32(low) || falseAccept(scores, float64(low)) > maxFA+1e-12 {
		return 0, false
	}
	return float64(low), true
}

func thresholdFRUpper(target []float64, maxFR float64) (float64, bool) {
	n := len(target)
	if n == 0 {
		return 0, false
	}
	scores := append([]float64(nil), target...)
	sort.Float64s(scores)
	for _, score := range scores {
		if math.IsNaN(score) || math.IsInf(score, 0) {
			return 0, false
		}
	}
	allowed := int(math.Floor(maxFR*float64(n) + 1e-12))
	best := math.Inf(-1)
	found := false
	for i := 0; i < n; {
		j := i + 1
		for j < n && scores[j] == scores[i] {
			j++
		}
		if i <= allowed {
			best = scores[i]
			found = true
		}
		i = j
	}
	if !found || math.IsNaN(best) || math.IsInf(best, 0) {
		return 0, false
	}
	return best, true
}

func falseAccept(nontarget []float64, threshold float64) float64 {
	if len(nontarget) == 0 {
		return 1
	}
	accepted := 0
	for _, score := range nontarget {
		if score >= threshold {
			accepted++
		}
	}
	return float64(accepted) / float64(len(nontarget))
}

func falseReject(target []float64, threshold float64) float64 {
	if len(target) == 0 {
		return 1
	}
	rejected := 0
	for _, score := range target {
		if score < threshold {
			rejected++
		}
	}
	return float64(rejected) / float64(len(target))
}

func otherEmbeddings(groups [][][]float32, held int) [][]float32 {
	out := make([][]float32, 0)
	for i, group := range groups {
		if i == held {
			continue
		}
		out = append(out, group...)
	}
	return out
}

func collectEmbeddings(groups [][][]float32) [][]float32 {
	all := make([][]float32, 0)
	for _, group := range groups {
		for _, vector := range group {
			all = append(all, append([]float32(nil), vector...))
		}
	}
	if len(all) <= 64 {
		return all
	}
	kept := make([][]float32, 64)
	last := len(kept) - 1
	span := len(all) - 1
	for i := range kept {
		kept[i] = all[i*span/last]
	}
	return kept
}

func resolvedDevice(id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && id != "default"
}
