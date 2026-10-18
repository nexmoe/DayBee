package voiceprint

import "testing"

// The selected float32 low must still honor the advertised FAR bound after
// conversion. A float64 nextafter that rounds back onto a tie is insufficient.
func TestCalibrationBandHonorsTiedImpostorScores(t *testing.T) {
	for _, negatives := range [][]float64{{.7, .7, .7, .6}, {.5, .5, .5}, {float64(float32(.7))}, {.2}, {.1, .2, .3}} {
		genuine := []float64{1.2, 1.25, 1.3}
		fa, fr, err := rateBounds(genuine, negatives)
		if err != nil {
			t.Fatal("clearly separable distributions rejected:", err)
		}
		low, high, err := hysteresisThresholds(fa, fr)
		if err != nil {
			t.Fatal("representable band rejected:", err)
		}
		if got := falseAccept(negatives, float64(low)); got > .01 {
			t.Fatalf("float32 low %g accepts %.2f%% of negatives %v", low, 100*got, negatives)
		}
		if got := falseReject(genuine, float64(high)); got > .05 {
			t.Fatalf("high %g rejects %.2f%% of genuine scores", high, 100*got)
		}
		if !(low < high) {
			t.Fatalf("collapsed band: %g %g", low, high)
		}
	}
}
