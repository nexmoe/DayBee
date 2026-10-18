package audio

import (
	"slices"
	"testing"
)

func TestVoiceGateRejectsResidualAudioWithoutChangingTime(t *testing.T) {
	gate := voiceActivityGate{}
	var output []float32
	for range 100 {
		input := make([]float32, 160)
		for i := range input {
			input[i] = .03
		}
		output = append(output, gate.Push(input, 0)...)
	}
	output = append(output, gate.Flush()...)
	if len(output) != SampleRate || RMS(output) != 0 {
		t.Fatalf("residual echo passed or capture timing changed: samples=%d RMS=%f", len(output), RMS(output))
	}
}

func TestVoiceGatePreservesSpeechOnsetsPausesAndFinalWords(t *testing.T) {
	gate := voiceActivityGate{}
	var input, output []float32
	for frame := range 100 {
		activity := 0
		if frame >= 30 && frame < 50 {
			activity = 1
		}
		packet := make([]float32, 160)
		for i := range packet {
			packet[i] = float32(frame+1) / 1000
		}
		input = append(input, packet...)
		output = append(output, gate.Push(packet, activity)...)
	}
	output = append(output, gate.Flush()...)
	// Speech starts before the detector fires and continues briefly after it
	// stops. Unrelated audio outside the padding is rejected.
	if len(output) != len(input) || !slices.Equal(output[10*160:70*160], input[10*160:70*160]) ||
		RMS(output[:10*160]) != 0 || RMS(output[70*160:]) != 0 {
		t.Fatal("speech padding, final drain, or residual suppression was lost")
	}
}

func TestVoiceGateUnavailableDetectorPreservesPendingAndQuietSpeech(t *testing.T) {
	gate := voiceActivityGate{}
	quiet := []float32{.0001, -.0001, .0002}
	if got := gate.Push(quiet, 0); len(got) != 0 {
		t.Fatal("expected onset lookahead")
	}
	got := gate.Push(quiet, -1)
	want := append(slices.Clone(quiet), quiet...)
	if !slices.Equal(got, want) || len(gate.Flush()) != 0 {
		t.Fatal("an unavailable detector muted quiet speech or duplicated the drain")
	}
}

func TestVoiceGateFlushPreservesShortSpeech(t *testing.T) {
	gate := voiceActivityGate{}
	input := []float32{.01, .02, .03}
	if got := gate.Push(input, 1); len(got) != 0 {
		t.Fatal("expected onset lookahead")
	}
	if !slices.Equal(gate.Flush(), input) || len(gate.Flush()) != 0 {
		t.Fatal("stopping recording lost or duplicated the final speech samples")
	}
}
