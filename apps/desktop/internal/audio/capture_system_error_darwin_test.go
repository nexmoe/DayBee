package audio

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSystemStreamDelegateKeepsNativeError(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "system-error-test")
	command := exec.Command("xcrun", "clang", "-fblocks", "-fno-objc-arc", "-fsanitize=address,undefined",
		"testdata/system_capture_error_test.m", "capture_system_tap_darwin.m", "-o", binary,
		"-framework", "ScreenCaptureKit", "-framework", "Foundation",
		"-framework", "CoreMedia", "-framework", "AudioToolbox", "-framework", "CoreAudio")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile native system error test: %v\n%s", err, output)
	}
	if output, err := exec.Command(binary).CombinedOutput(); err != nil {
		t.Fatalf("stream delegate discarded native error: %v\n%s", err, output)
	}
}
