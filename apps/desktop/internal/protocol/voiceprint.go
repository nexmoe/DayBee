package protocol

type VoiceprintSetupState string

const (
	VoiceprintPending    VoiceprintSetupState = "pending"
	VoiceprintSkipped    VoiceprintSetupState = "skipped"
	VoiceprintConfigured VoiceprintSetupState = "ready"
)

type VoiceprintPhase string

const (
	VoiceprintIdle        VoiceprintPhase = "idle"
	VoiceprintPreparing   VoiceprintPhase = "preparing"
	VoiceprintRecording   VoiceprintPhase = "recording"
	VoiceprintSampleReady VoiceprintPhase = "sample-ready"
	VoiceprintProcessing  VoiceprintPhase = "processing"
	VoiceprintReady       VoiceprintPhase = "ready"
	VoiceprintError       VoiceprintPhase = "error"
)

type VoiceprintAdaptationProgress struct {
	LearnedClips  uint64 `json:"learnedClips"`
	AssistedClips uint64 `json:"assistedClips"`
}

// Only progress and counters cross the bridge. Speaker embeddings stay local.
type VoiceprintSnapshot struct {
	Enrolled        bool                          `json:"enrolled"`
	Phase           VoiceprintPhase               `json:"phase"`
	Step            int                           `json:"step"`
	SpeechSeconds   float64                       `json:"speechSeconds"`
	TargetSeconds   float64                       `json:"targetSeconds"`
	Level           float64                       `json:"level"`
	Progress        *float64                      `json:"progress"`
	Message         string                        `json:"message"`
	Error           *string                       `json:"error"`
	AdaptationCount int                           `json:"adaptationCount,omitempty"`
	Adaptation      *VoiceprintAdaptationProgress `json:"adaptation,omitempty"`
}

func DefaultVoiceprint() VoiceprintSnapshot {
	return VoiceprintSnapshot{Phase: VoiceprintIdle, TargetSeconds: 8}
}
