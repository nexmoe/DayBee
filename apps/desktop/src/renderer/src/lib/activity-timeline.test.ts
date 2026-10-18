import { describe, expect, it } from "vitest";
import type { MicrophoneUsage, RecordingHistoryItem } from "@daybee/shared";
import { buildActivityTimeline } from "./activity-timeline";
import { createRecordingContextIndex } from "./recording-context";

const at = (seconds: number) => new Date(Date.parse("2026-10-09T10:00:00Z") + seconds * 1000).toISOString();
const app = (id: string, start: number, end: number | null, appId = id): MicrophoneUsage => ({
  id, appId, appName: appId === "zoom" ? "Zoom" : "Teams", appIcon: "logo", startedAt: at(start), endedAt: end === null ? null : at(end)
});
const record = (id: string, chunks: [number, number, string][]): RecordingHistoryItem => ({
  id, startedAt: at(chunks[0][0]), textPreview: chunks.map(chunk => chunk[2]).join("\n"),
  audioSource: "microphone", audioPath: `${id}.wav`, folderPath: "/recordings", inputDevice: null, status: "ok", sessionId: id,
  transcriptChunks: chunks.map(([start, end, text]) => ({ startedAt: at(start), endedAt: at(end), text }))
});

describe("recording scenes in the transcription timeline", () => {
  it("annotates speech with its scene without standalone application events", () => {
    const items = [record("meeting", [[12, 15, "Budget discussion."]]), record("personal", [[60, 65, "A personal note."]])];
    const usage = [app("zoom", 10, 30)];
    const entries = buildActivityTimeline(items, usage);
    expect(entries).toHaveLength(2);
    expect(entries[0].contexts).toEqual([]);
    expect(entries[1]).toMatchObject({ paragraph: { text: "Budget discussion." }, contexts: [{ appName: "Zoom", appIcon: "logo" }] });
    expect(buildActivityTimeline([], usage)).toEqual([]);
  });

  it("finds the meeting transcript by app name even when its text never mentions Zoom", () => {
    const items = [record("meeting", [[12, 15, "Budget discussion."]]), record("other", [[60, 65, "Other discussion."]])];
    expect(buildActivityTimeline(items, [app("zoom", 10, 30)], " zOoM ").map(entry => entry.paragraph.text)).toEqual(["Budget discussion."]);
    expect(buildActivityTimeline(items, [app("zoom", 10, 30)], "discussion", "zoom")).toHaveLength(1);
    expect(buildActivityTimeline(items, [app("zoom", 10, 30)], "[.*")).toEqual([]);
  });

  it("separates adjacent speech when the scene changes inside one audio file", () => {
    const items = [record("long-file", [[1, 4, "Before meeting."], [12, 15, "During meeting."], [31, 35, "After meeting."]])];
    const entries = buildActivityTimeline(items, [app("zoom", 10, 30)]);
    expect(entries.map(entry => entry.paragraph.text)).toEqual(["After meeting.", "During meeting.", "Before meeting."]);
    expect(buildActivityTimeline(items, [app("zoom", 10, 30)], "", "zoom").map(entry => entry.paragraph.text)).toEqual(["During meeting."]);
  });

  it("merges the reported continuous speech when app capture starts inside the first utterance", () => {
    const items = [record("continuous", [[6.664, 12.814, "设置页允许配置是否录制系统声音。"],
      [13.608, 19.758, "然后系统声音和麦克风声音分开。"]])];
    const entries = buildActivityTimeline(items, [app("zoom", 7.445, 22.445)]);
    expect(entries).toHaveLength(1);
    expect(entries[0]).toMatchObject({
      paragraph: { startedAt: at(6.664), endedAt: at(19.758),
        text: "设置页允许配置是否录制系统声音。\n然后系统声音和麦克风声音分开。" },
      contexts: [{ appId: "zoom" }]
    });
  });

  it("ignores another app's boundaries inside silence while the speech context remains the same", () => {
    const items = [record("same-scene", [[2, 4, "Before the short silence."], [8, 10, "Continuing the discussion."]])];
    const entries = buildActivityTimeline(items, [app("zoom", 1, 20), app("teams", 5, 7)]);
    expect(entries).toHaveLength(1);
    expect(entries[0].contexts.map(context => context.appId)).toEqual(["zoom"]);
    expect(buildActivityTimeline(items, [app("zoom", 1, 20), app("teams", 5, 7)], "", "teams")).toEqual([]);
  });

  it("still separates distinct capture sessions belonging to the same app", () => {
    const items = [record("two-meetings", [[6, 8, "First meeting."], [12, 14, "Second meeting."]])];
    const entries = buildActivityTimeline(items, [app("meeting-one", 5, 10, "zoom"), app("meeting-two", 11, 20, "zoom")]);
    expect(entries.map(entry => entry.paragraph.text)).toEqual(["Second meeting.", "First meeting."]);
  });

  it("retains app context while splitting long speech and pauses across archive rotation", () => {
    const opening = "会议开场。".repeat(90);
    const items = [record("first-file", [[12, 18, opening], [180, 190, "讨论方案。"]]),
      { ...record("rotated-file", [[400, 410, "确认下一步。"]]), sessionId: "first-file" },
      record("restarted-recorder", [[720, 730, "会议结束。"]])];
    const entries = buildActivityTimeline(items, [app("one-meeting", 10, 800, "zoom")]);
    expect(entries).toHaveLength(5);
    expect(entries[0].paragraph).toMatchObject({ startedAt: at(720), endedAt: at(730) });
    expect([...entries].sort((a, b) => Date.parse(a.paragraph.startedAt) - Date.parse(b.paragraph.startedAt))
      .map(entry => entry.paragraph.text).join("").replace(/\s/gu, "")).toBe(`${opening}讨论方案。确认下一步。会议结束。`);
    expect(entries.every(entry => entry.paragraph.text.length <= 240 && entry.contexts[0]?.appId === "zoom")).toBe(true);
  });

  it("starts new nodes when another app joins or leaves the microphone", () => {
    const items = [record("meeting", [[12, 15, "Opening."], [40, 45, "Dictating a note."], [70, 75, "Continuing the meeting."]])];
    const usages = [app("zoom-meeting", 10, 90, "zoom"), app("dictation", 35, 50, "teams")];
    const entries = buildActivityTimeline(items, usages);
    expect(entries.map(entry => entry.paragraph.text)).toEqual(["Continuing the meeting.", "Dictating a note.", "Opening."]);
    expect(entries.map(entry => entry.contexts.map(context => context.appId))).toEqual([["zoom"], ["teams", "zoom"], ["zoom"]]);
    // Filtering by the shorter activity still returns only its overlapping speech.
    expect(buildActivityTimeline(items, usages, "", "teams").map(entry => entry.paragraph.text)).toEqual(["Dictating a note."]);
  });

  it("retains completed nodes as later speech arrives during active app capture", () => {
    const first = record("live", [[12, 15, "Opening."]]);
    const usages = [app("live-meeting", 10, null, "zoom")];
    const original = buildActivityTimeline([first], usages, "", "", Date.parse(at(20)))[0];
    const updated = buildActivityTimeline([first, record("later", [[600, 610, "Later discussion."]])], usages, "", "", Date.parse(at(620)));
    expect(updated).toHaveLength(2);
    expect(updated[1]).toEqual(original);
    expect(updated[0].paragraph).toMatchObject({ startedAt: at(600), endedAt: at(610) });
  });

  it("interleaves app speech and system audio as separate timeline nodes", () => {
    const items = [record("microphone", [[12, 15, "Meeting opening."], [600, 610, "Later meeting speech."]]),
      { ...record("speaker", [[20, 25, "Speaker playback."]]), audioSource: "system" as const }];
    const entries = buildActivityTimeline(items, [app("zoom-meeting", 10, 700, "zoom")]);
    expect(entries).toHaveLength(3);
    expect(entries[0]).toMatchObject({ paragraph: { audioSource: "microphone", startedAt: at(600), endedAt: at(610) }, contexts: [{ appId: "zoom" }] });
    expect(entries[1]).toMatchObject({ paragraph: { audioSource: "system", text: "Speaker playback." }, contexts: [] });
    expect(entries[2]).toMatchObject({ paragraph: { audioSource: "microphone", text: "Meeting opening." }, contexts: [{ appId: "zoom" }] });
    expect(buildActivityTimeline(items, [app("zoom-meeting", 10, 700, "zoom")], "", "zoom")).toHaveLength(2);
  });

  it("never assigns a meeting that only happened in a gap between saved speech", () => {
    const items = [record("gap", [[1, 4, "Before."], [31, 35, "After."]])];
    expect(buildActivityTimeline(items, [app("zoom", 10, 30)], "", "zoom")).toEqual([]);
    expect(createRecordingContextIndex([app("zoom", 10, 30)]).recording(items[0])).toEqual([]);
  });

  it("keeps simultaneous apps, but deduplicates the same application's sessions", () => {
    const entries = buildActivityTimeline([record("meeting", [[12, 15, "Discussion."]])],
      [app("zoom-1", 10, 30, "zoom"), app("zoom-2", 11, 20, "zoom"), app("teams", 12, 16)]);
    expect(entries[0].contexts.map(context => context.appId)).toEqual(["teams", "zoom"]);
  });

  it("uses half-open intervals and only the observed part of active sessions", () => {
    const index = createRecordingContextIndex([app("zoom", 10, 30)], Date.parse(at(50)));
    expect(index.contexts([{ startedAt: at(30), endedAt: at(35) }])).toEqual([]);
    expect(index.contexts([{ startedAt: at(5), endedAt: at(10) }])).toEqual([]);
    expect(index.contexts([{ startedAt: at(10), endedAt: at(10) }])).toHaveLength(1);
    expect(index.contexts([{ startedAt: at(30), endedAt: at(30) }])).toEqual([]);
    const active = createRecordingContextIndex([app("zoom", 10, null)], Date.parse(at(50)));
    expect(active.contexts([{ startedAt: at(40), endedAt: at(45) }])).toHaveLength(1);
    expect(active.contexts([{ startedAt: at(60), endedAt: at(65) }])).toEqual([]);
  });

  it("finds audio-only recordings through actual capture ranges rather than file start or silence", () => {
    const item = { ...record("audio-only", [[1, 4, ""]]), textPreview: "", transcriptChunks: [],
      capturedRanges: [{ startedAt: at(1), endedAt: at(4) }, { startedAt: at(31), endedAt: at(35) }] };
    expect(createRecordingContextIndex([app("zoom", 10, 30)]).recording(item)).toEqual([]);
    expect(createRecordingContextIndex([app("zoom", 30, 40)]).recording(item)).toMatchObject([{ appName: "Zoom" }]);
  });

  it("retains scene context after batch transcription loses per-sentence timing", () => {
    const item = { ...record("batch", [[1, 4, "Full reprocessed transcript."]]), transcriptChunks: [],
      capturedRanges: [{ startedAt: at(1), endedAt: at(4) }, { startedAt: at(31), endedAt: at(35) }] };
    expect(buildActivityTimeline([item], [app("zoom", 30, 40)], "", "zoom")).toHaveLength(1);
    expect(buildActivityTimeline([item], [app("zoom", 10, 30)], "", "zoom")).toEqual([]);
  });

  it("uses actual instants across time zones and never invents unknown intervals", () => {
    const index = createRecordingContextIndex([app("zoom", 10, 30)]);
    expect(index.contexts([{ startedAt: "2026-10-09T18:00:12+08:00", endedAt: "2026-10-09T18:00:15+08:00" }])).toHaveLength(1);
    expect(index.contexts([{ startedAt: "unknown", endedAt: "unknown" }])).toEqual([]);
    expect(index.contexts([{ startedAt: at(20), endedAt: at(10) }])).toEqual([]);
  });
});


it("does not misattribute system playback to an app using the microphone", () => {
  const system: RecordingHistoryItem = { ...record("system", [[12, 15, "A video soundtrack."]]), audioSource: "system" };
  const entries = buildActivityTimeline([system, record("mic", [[12, 15, "My speech."]])], [app("zoom", 10, 30)]);
  expect(entries).toHaveLength(2);
  expect(entries.find(entry => entry.paragraph.audioSource === "system")?.contexts).toEqual([]);
  expect(buildActivityTimeline([system], [app("zoom", 10, 30)], "", "zoom")).toEqual([]);
});
