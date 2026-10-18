package protocol

import "testing"

func TestDefaultEchoCancellationMatchesPlatformCapabilities(t *testing.T) {
	for _, platform := range []string{"darwin", "windows", "linux"} {
		t.Run(platform, func(t *testing.T) {
			settings := DefaultSettingsForPlatform("recordings", platform)
			if settings.Recording.EchoCancellation != (platform == "darwin") {
				t.Fatalf("unsupported echo cancellation default on %s", platform)
			}
			if settings.Recording.RecordSystemAudio {
				t.Fatal("system audio recording must remain opt-in")
			}
		})
	}
	if DefaultSettings("recordings").Recording.EchoCancellation != SupportsEchoCancellation {
		t.Fatal("native defaults do not match this host's capabilities")
	}
}
