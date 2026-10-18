package desktop

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
)

func publicationFixture() protocol.DesktopSnapshot {
	snapshot := protocol.DesktopSnapshot{Settings: protocol.DefaultSettings("recordings"),
		Status: protocol.DefaultStatus(), Voiceprint: protocol.DefaultVoiceprint()}
	for i := 0; i < 166; i++ {
		item := protocol.RecordingHistoryItem{ID: string(rune(i + 1)), TextPreview: strings.Repeat("Saved transcript. ", 100)}
		for j := 0; j < 25; j++ {
			item.TranscriptChunks = append(item.TranscriptChunks, protocol.TranscriptChunk{Text: "Saved speech", StartedAt: "2026-10-12T10:00:00Z"})
		}
		snapshot.History = append(snapshot.History, item)
	}
	for i := 0; i < 700; i++ {
		snapshot.MicrophoneActivity.Items = append(snapshot.MicrophoneActivity.Items, protocol.MicrophoneUsage{
			ID: string(rune(i + 1)), AppID: "meeting", AppIcon: strings.Repeat("a", 3000)})
	}
	return snapshot
}

func TestStatusUpdatesDoNotResendSavedData(t *testing.T) {
	snapshot := publicationFixture()
	var publisher snapshotPublisher
	if publisher.next(snapshot, 0) != publishFullSnapshot {
		t.Fatal("first publication must initialize all window data")
	}
	full, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var transmitted int
	for i := 0; i < 100; i++ {
		snapshot.Status.LevelRatio = float64(i+1) / 100
		snapshot.Status.Elapsed = "00:00:10"
		if publisher.next(snapshot, 0) != publishStatusSnapshot {
			t.Fatal("audio status update carried saved history")
		}
		payload, err := json.Marshal(protocol.DesktopStatusSnapshot{Status: snapshot.Status, Voiceprint: snapshot.Voiceprint})
		if err != nil {
			t.Fatal(err)
		}
		transmitted += len(payload)
	}
	if transmitted >= len(full) {
		t.Fatalf("100 status messages (%d bytes) should be smaller than one full snapshot (%d bytes)", transmitted, len(full))
	}
	t.Logf("full snapshot: %d bytes; mean status update: %d bytes", len(full), transmitted/100)
	if publisher.next(snapshot, 0) != publishNothing {
		t.Fatal("unchanged state should not publish")
	}
	snapshot.Voiceprint.SpeechSeconds = 2
	if publisher.next(snapshot, 0) != publishStatusSnapshot {
		t.Fatal("enrollment progress should use the lightweight update")
	}
}

func TestSnapshotPublishesDataChangesAndSkipsIdenticalHistoryReload(t *testing.T) {
	snapshot := publicationFixture()
	var publisher snapshotPublisher
	publisher.next(snapshot, 1)
	// A history scan returns new slices even when no file has changed.
	raw, _ := json.Marshal(snapshot)
	var reloaded protocol.DesktopSnapshot
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatal(err)
	}
	if publisher.next(reloaded, 1) != publishNothing {
		t.Fatal("an identical disk scan should not trigger window updates")
	}
	skipped := reloaded
	skipped.History[0].TextPreview = "unread while the history version is unchanged"
	if publisher.next(skipped, 1) != publishNothing {
		t.Fatal("an unchanged history version still compared every saved transcript")
	}
	// Decode into fresh nested slices, as the next native history scan does.
	var changed protocol.DesktopSnapshot
	if err := json.Unmarshal(raw, &changed); err != nil {
		t.Fatal(err)
	}
	reloaded = changed
	reloaded.History[0].TranscriptChunks[0].Text = "New transcription"
	if publisher.next(reloaded, 2) != publishFullSnapshot {
		t.Fatal("new speech must update both timelines")
	}
	reloaded.MicrophoneActivity.Items = append(reloaded.MicrophoneActivity.Items, protocol.MicrophoneUsage{ID: "new-app"})
	if publisher.next(reloaded, 2) != publishFullSnapshot {
		t.Fatal("microphone application changes must update both windows")
	}
	reloaded.Settings.Recording.DisableASR = true
	if publisher.next(reloaded, 2) != publishFullSnapshot {
		t.Fatal("settings changes must update both windows")
	}
	reloaded.ASRModels.Busy = true
	if publisher.next(reloaded, 2) != publishFullSnapshot {
		t.Fatal("model switching availability must stay current")
	}
}

func BenchmarkStatusPublicationWithSavedHistory(b *testing.B) {
	snapshot := publicationFixture()
	var publisher snapshotPublisher
	publisher.next(snapshot, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snapshot.Status.LevelRatio = float64(i)
		publisher.next(snapshot, 1)
	}
}
