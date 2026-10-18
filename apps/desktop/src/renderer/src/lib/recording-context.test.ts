import { describe, expect, it } from "vitest";
import type { MicrophoneUsage, RecordingHistoryItem } from "@daybee/shared";
import { availableRecordingContexts, createRecordingContextIndex } from "./recording-context";
import { buildActivityTimeline } from "./activity-timeline";

const at = (seconds: number) => new Date(Date.parse("2026-10-09T10:00:00Z") + seconds * 1000).toISOString();
const usage = (appId: string, start: number, end: number | null): MicrophoneUsage => ({
  id: `${appId}-${start}`, appId, appName: appId, appIcon: "", startedAt: at(start), endedAt: end === null ? null : at(end)
});
const recording = (start: number, end: number, text = "Meeting notes."): RecordingHistoryItem => ({
  id: `recording-${start}`, audioSource: "microphone", audioPath: "/recordings/meeting.wav", folderPath: "/recordings",
  startedAt: at(start), status: text ? "ok" : "audio_only", inputDevice: null, sessionId: "meeting", textPreview: text,
  transcriptChunks: text ? [{ startedAt: at(start), endedAt: at(end), text }] : [],
  capturedRanges: [{ startedAt: at(start), endedAt: at(end) }]
});

describe("application filter options", () => {
  it("only offers apps with saved speech on Home and saved audio in History", () => {
    const items = [recording(10, 15), recording(30, 35, ""), { ...recording(50, 55), audioSource: "system" as const }];
    const usages = [usage("meeting", 9, 20), usage("audio-only", 29, 40), usage("unused", 0, 5),
      usage("system-only", 49, 60), usage("live-without-recording", 70, null), usage("pid:14559", 9, 20)];
    const home = availableRecordingContexts(usages, items, "transcripts", Date.parse(at(80)));
    const history = availableRecordingContexts(usages, items, "recordings", Date.parse(at(80)));
    expect(home.map(app => app.appId)).toEqual(["meeting"]);
    expect(history.map(app => app.appId)).toEqual(["audio-only", "meeting"]);
    for (const app of home) expect(buildActivityTimeline(items, usages, "", app.appId, Date.parse(at(80))).length).toBeGreaterThan(0);
    const index = createRecordingContextIndex(usages, Date.parse(at(80)));
    for (const app of history) expect(items.some(item => index.recording(item).some(context => context.appId === app.appId))).toBe(true);
    expect(availableRecordingContexts(usages, [], "transcripts")).toEqual([]);
    expect(availableRecordingContexts(usages, [], "recordings")).toEqual([]);
  });

  it("does not offer apps that only overlap silence or the end boundary", () => {
    const item = { ...recording(10, 35), capturedRanges: [{ startedAt: at(10), endedAt: at(15) }, { startedAt: at(30), endedAt: at(35) }],
      transcriptChunks: [{ startedAt: at(10), endedAt: at(15), text: "Opening." }, { startedAt: at(30), endedAt: at(35), text: "Closing." }] };
    const usages = [usage("silence", 16, 29), usage("ended", 0, 10), usage("meeting", 9, 36)];
    for (const scope of ["transcripts", "recordings"] as const) {
      expect(availableRecordingContexts(usages, [item], scope).map(app => app.appId)).toEqual(["meeting"]);
    }
  });

  it("keeps batch and legacy transcripts discoverable using their known capture times", () => {
    const batch = { ...recording(10, 15), transcriptChunks: [] };
    const legacy = { ...recording(30, 35), transcriptChunks: [], capturedRanges: [] };
    const usages = [usage("batch", 11, 14), usage("legacy", 29, 31)];
    expect(availableRecordingContexts(usages, [batch, legacy], "transcripts").map(app => app.appId)).toEqual(["batch", "legacy"]);
  });

  it("ignores unidentified legacy processes in both annotations and live app lists", () => {
    const usages = [usage("pid:14559", 9, 20), usage("", 9, 20), usage("meeting", 9, 20), usage("meeting", 25, null)];
    expect(availableRecordingContexts(usages).map(app => app.appId)).toEqual(["meeting"]);
    expect(createRecordingContextIndex(usages).recording(recording(10, 15)).map(app => app.appId)).toEqual(["meeting"]);
  });
});
