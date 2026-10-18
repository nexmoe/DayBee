package audio

import (
	"errors"
	"strings"
	"testing"
)

func TestSystemFailurePreservesNativeReason(t *testing.T) {
	detail := "The stream connection was interrupted (com.apple.ScreenCaptureKit.SCStreamErrorDomain -3805)"
	err := systemFailureError(1, detail)
	if !errors.Is(err, ErrInterrupted) || !strings.Contains(err.Error(), detail) {
		t.Fatalf("native reason was replaced by a generic message: %v", err)
	}
	stopped := "The user stopped the stream (com.apple.ScreenCaptureKit.SCStreamErrorDomain -3817)"
	if err := systemFailureError(4, stopped); errors.Is(err, ErrInterrupted) || !strings.Contains(err.Error(), stopped) {
		t.Fatalf("intentional stop lost its native reason: %v", err)
	}
	tap := "System audio output device changed (OSStatus 560947818)"
	if err := systemFailureError(1, tap); !errors.Is(err, ErrInterrupted) || !strings.Contains(err.Error(), "OSStatus 560947818") {
		t.Fatalf("process tap OSStatus was dropped: %v", err)
	}
	denied := "系统音频录制权限未授予 (OSStatus -2003329396)"
	if err := systemFailureError(1, denied); !errors.Is(err, ErrInterrupted) || !strings.Contains(err.Error(), "系统音频录制权限") {
		t.Fatalf("tap permission denial was rewritten: %v", err)
	}
}
