package voiceprint

import "testing"

func TestClippedFullScaleShare(t *testing.T) {
	if Clipped(nil) || Clipped([]float32{}) || Clipped([]float32{0, 0.98, -0.98}) {
		t.Fatal("quiet audio was rejected")
	}
	below := make([]float32, 10)
	for i := range below {
		below[i] = 0.989
	}
	if Clipped(below) {
		t.Fatal("samples under full scale were rejected")
	}
	exact := make([]float32, 1000)
	exact[0] = fullScale
	if Clipped(exact) {
		t.Fatal("exactly 0.1% full-scale samples was rejected")
	}
	exact[1] = -fullScale
	if !Clipped(exact) {
		t.Fatal("more than 0.1% full-scale samples was accepted")
	}
	boundary := make([]float32, 10000)
	for i := range 10 {
		boundary[i] = 1
	}
	if Clipped(boundary) {
		t.Fatal("exactly 0.1% full-scale samples was rejected")
	}
	boundary[10] = -1
	if !Clipped(boundary) || !Clipped([]float32{1}) || !Clipped([]float32{-1}) {
		t.Fatal("clipped audio was accepted")
	}
}
