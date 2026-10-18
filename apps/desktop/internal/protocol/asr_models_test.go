package protocol

import (
	"testing"

	"github.com/nexmoe/DayBee/apps/desktop/internal/speechanalyzer"
)

func TestASRDefaultFollowsSystemSpeechAvailability(t *testing.T) {
	if defaultASRModel(true) != ASRSpeechAnalyzer || defaultASRModel(false) != ASRWhisperTiny {
		t.Fatal("system speech must be preferred only when available")
	}
	if DefaultASRModel != defaultASRModel(speechanalyzer.Available()) {
		t.Fatal("default disagrees with the native runtime")
	}
	if ValidASRModel(ASRSpeechAnalyzer) != speechanalyzer.Available() {
		t.Fatal("unsupported devices must not accept the system model")
	}
}
