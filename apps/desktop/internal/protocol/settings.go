// Package protocol owns the JSON contract shared by Go and the generated client.
package protocol

import "runtime"

const SupportsEchoCancellation = runtime.GOOS == "darwin"
const SupportsSystemAudio = runtime.GOOS == "darwin"

type AudioSource string

const (
	SourceMicrophone AudioSource = "microphone"
	SourceSystem     AudioSource = "system"
)

func NormalizeAudioSource(source AudioSource) AudioSource {
	if source == SourceSystem {
		return SourceSystem
	}
	return SourceMicrophone
}

type AppLanguage string

const (
	LanguageEnglish AppLanguage = "en-US"
	LanguageSystem  AppLanguage = "system"
	LanguageChinese AppLanguage = "zh-CN"
)

type ThemeMode string

const (
	ThemeDark   ThemeMode = "dark"
	ThemeLight  ThemeMode = "light"
	ThemeSystem ThemeMode = "system"
)

type DesktopSettings struct {
	HideWindowOnClose bool                 `json:"hideWindowOnClose"`
	Language          AppLanguage          `json:"language"`
	LaunchAtLogin     bool                 `json:"launchAtLogin"`
	Theme             ThemeMode            `json:"theme"`
	VoiceprintSetup   VoiceprintSetupState `json:"voiceprintSetup"`
}

type RecordingSettings struct {
	ASRLanguage             string   `json:"asrLanguage"`
	ASRModel                ASRModel `json:"asrModel"`
	AutoSwitchConfirmations int      `json:"autoSwitchConfirmations"`
	AutoSwitchDevice        bool     `json:"autoSwitchDevice"`
	Device                  string   `json:"device"`
	EchoCancellation        bool     `json:"echoCancellation"`
	RecordSystemAudio       bool     `json:"recordSystemAudio"`
	DisableASR              bool     `json:"disableAsr"`
	ExcludeDeviceKeywords   string   `json:"excludeDeviceKeywords"`
	OutputDir               string   `json:"outputDir"`
	SegmentMinutes          float64  `json:"segmentMinutes"`
	VoiceprintAdaptation    bool     `json:"voiceprintAdaptation"`
}

type AppSettings struct {
	Desktop   DesktopSettings   `json:"desktop"`
	Recording RecordingSettings `json:"recording"`
}

func DefaultSettings(outputDir string) AppSettings {
	return DefaultSettingsForPlatform(outputDir, runtime.GOOS)
}

// DefaultSettingsForPlatform also gives contract generation a stable reference platform.
func DefaultSettingsForPlatform(outputDir, platform string) AppSettings {
	return AppSettings{
		Desktop: DesktopSettings{HideWindowOnClose: true, Language: LanguageSystem, LaunchAtLogin: true, Theme: ThemeSystem, VoiceprintSetup: VoiceprintPending},
		Recording: RecordingSettings{
			ASRLanguage: "auto", ASRModel: DefaultASRModel, AutoSwitchConfirmations: 2,
			AutoSwitchDevice: true, Device: "default", ExcludeDeviceKeywords: "iphone,continuity",
			EchoCancellation: platform == "darwin",
			OutputDir:        outputDir, SegmentMinutes: 60, VoiceprintAdaptation: false,
		},
	}
}
