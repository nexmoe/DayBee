package protocol

type DeviceInfo struct {
	ID        string `json:"id"`
	Index     int    `json:"index"`
	IsDefault bool   `json:"isDefault"`
	Label     string `json:"label"`
}

type MicrophonePermissionState string

const (
	PermissionAuthorized    MicrophonePermissionState = "authorized"
	PermissionDenied        MicrophonePermissionState = "denied"
	PermissionNotDetermined MicrophonePermissionState = "not-determined"
	PermissionRestricted    MicrophonePermissionState = "restricted"
	PermissionUnsupported   MicrophonePermissionState = "unsupported"
)

type MicrophonePermissionStatus struct {
	Message   string                    `json:"message"`
	State     MicrophonePermissionState `json:"state"`
	Supported bool                      `json:"supported"`
}

type RecorderStatusSnapshot struct {
	ASREnabled        bool     `json:"asrEnabled"`
	ASRHistory        []string `json:"asrHistory"`
	ASRPreview        string   `json:"asrPreview"`
	AutoSwitchEnabled bool     `json:"autoSwitchEnabled"`
	DB                float64  `json:"db"`
	DownloadMessage   string   `json:"downloadMessage"`
	DownloadProgress  *float64 `json:"downloadProgress"`
	DeviceLabel       string   `json:"deviceLabel"`
	Elapsed           string   `json:"elapsed"`
	Error             *string  `json:"error"`
	InSpeech          bool     `json:"inSpeech"`
	LevelRatio        float64  `json:"levelRatio"`
	Downloading       bool     `json:"downloading"`
	Recording         bool     `json:"recording"`
	Recovering        bool     `json:"recovering"`
	RMS               float64  `json:"rms"`
	// SenseVoiceReady is retained for compatibility; it describes the selected ASR model.
	SenseVoiceReady bool      `json:"senseVoiceReady"`
	StatusMessage   string    `json:"statusMessage"`
	VADReady        bool      `json:"vadReady"`
	WaveformBins    []float64 `json:"waveformBins"`
	// VoiceprintReference is off, starting, available, or unavailable.
	// Empty means the engine has not reported a reference state yet.
	VoiceprintReference      string `json:"voiceprintReference,omitempty"`
	VoiceprintReferenceError string `json:"voiceprintReferenceError,omitempty"`
	// VoiceprintContextMismatch is set when the live device or echo setting
	// cannot use a calibrated profile. Legacy profiles stay compatible.
	VoiceprintContextMismatch bool `json:"voiceprintContextMismatch,omitempty"`
}

const (
	VoiceprintReferenceOff         = "off"
	VoiceprintReferenceStarting    = "starting"
	VoiceprintReferenceAvailable   = "available"
	VoiceprintReferenceUnavailable = "unavailable"
)

func DefaultStatus() RecorderStatusSnapshot {
	return RecorderStatusSnapshot{
		ASREnabled: true, ASRHistory: []string{}, AutoSwitchEnabled: true,
		DB: -80, DeviceLabel: "default", Elapsed: "00:00:00",
		StatusMessage: "桌面端已就绪。", VoiceprintReference: VoiceprintReferenceOff, WaveformBins: make([]float64, 48),
	}
}

type RecordingHistoryItem struct {
	AudioSource      AudioSource          `json:"audioSource"`
	AudioPath        string               `json:"audioPath"`
	FolderPath       string               `json:"folderPath"`
	ID               string               `json:"id"`
	InputDevice      *string              `json:"inputDevice"`
	StartedAt        string               `json:"startedAt"`
	Status           string               `json:"status"`
	TextPreview      string               `json:"textPreview"`
	SessionID        string               `json:"sessionId"`
	TranscriptChunks []TranscriptChunk    `json:"transcriptChunks"`
	CapturedRanges   []RecordingTimeRange `json:"capturedRanges,omitempty"`
}

type RecordingTimeRange struct {
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt"`
}

type TranscriptChunk struct {
	Text          string  `json:"text"`
	StartedAt     string  `json:"startedAt"`
	EndedAt       string  `json:"endedAt"`
	VoiceIdentity string  `json:"voiceIdentity,omitempty"`
	VoiceMatch    string  `json:"voiceMatch,omitempty"`
	VoiceScore    float64 `json:"voiceScore,omitempty"`
}

type AppInfoSnapshot struct {
	Name          string `json:"name"`
	RepositoryURL string `json:"repositoryUrl"`
	Version       string `json:"version"`
}

type AutoUpdatePhase string

const (
	UpdateAvailable   AutoUpdatePhase = "available"
	UpdateChecking    AutoUpdatePhase = "checking"
	UpdateDownloaded  AutoUpdatePhase = "downloaded"
	UpdateDownloading AutoUpdatePhase = "downloading"
	UpdateError       AutoUpdatePhase = "error"
	UpdateIdle        AutoUpdatePhase = "idle"
	UpdateUnavailable AutoUpdatePhase = "unavailable"
)

type AutoUpdateSnapshot struct {
	CurrentVersion           string          `json:"currentVersion"`
	DownloadedVersion        *string         `json:"downloadedVersion"`
	DownloadedVersionReady   bool            `json:"downloadedVersionReady"`
	ErrorMessage             *string         `json:"errorMessage"`
	InstallDeferredUntilIdle bool            `json:"installDeferredUntilIdle"`
	LatestVersion            *string         `json:"latestVersion"`
	Phase                    AutoUpdatePhase `json:"phase"`
	StatusMessage            string          `json:"statusMessage"`
}

type DesktopSnapshot struct {
	ASRModels          ASRModelsSnapshot          `json:"asrModels"`
	App                AppInfoSnapshot            `json:"app"`
	Devices            []DeviceInfo               `json:"devices"`
	EngineReady        bool                       `json:"engineReady"`
	History            []RecordingHistoryItem     `json:"history"`
	MicrophoneActivity MicrophoneActivitySnapshot `json:"microphoneActivity"`
	Permission         MicrophonePermissionStatus `json:"permission"`
	Settings           AppSettings                `json:"settings"`
	Status             RecorderStatusSnapshot     `json:"status"`
	Updater            AutoUpdateSnapshot         `json:"updater"`
	WindowPinned       bool                       `json:"windowPinned"`
	Voiceprint         VoiceprintSnapshot         `json:"voiceprint"`
}

// High-frequency audio and enrollment updates never carry saved transcripts or
// application icons. Windows keep their existing history until it changes.
type DesktopStatusSnapshot struct {
	Status     RecorderStatusSnapshot `json:"status"`
	Voiceprint VoiceprintSnapshot     `json:"voiceprint"`
}

// Integers, rather than []byte, keep JSON encoding an array instead of base64.
type WaveformFrame struct {
	Spectrum []int `json:"spectrum"`
}
