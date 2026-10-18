package infer

import "runtime"

// Threads bounds one ONNX session. ASR and the voiceprint extractor each
// call this, so the combined budget stays at most four cores and at least one.
func Threads() int {
	n := runtime.NumCPU() / 4
	if n < 1 {
		n = 1
	}
	if n > 2 {
		n = 2
	}
	return n
}
