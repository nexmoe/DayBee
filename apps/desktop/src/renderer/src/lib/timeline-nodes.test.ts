import { describe, expect, it } from "vitest";
import type { MicrophoneUsage, RecordingHistoryItem } from "@daybee/shared";
import { buildActivityTimeline } from "./activity-timeline";

const at = (seconds: number) => new Date(Date.parse("2026-10-09T10:00:00Z") + seconds * 1000).toISOString();
const record = (id: string, chunks: [number, string, "me"?][], audioSource: "microphone" | "system" = "microphone"): RecordingHistoryItem => ({
  id, audioSource, sessionId: "one-session", audioPath: `${id}.wav`, folderPath: "/recordings", inputDevice: null,
  status: "ok", startedAt: at(chunks[0][0]), textPreview: chunks.map(chunk => chunk[1]).join("\n"),
  transcriptChunks: chunks.map(([start, text, voiceIdentity]) => ({ startedAt: at(start), endedAt: at(start + 2), text, voiceIdentity }))
});
const app = (id: string, start: number, end: number): MicrophoneUsage => ({
  id, appId: id, appName: id, appIcon: "", startedAt: at(start), endedAt: at(end)
});

describe("timeline nodes", () => {
  for (const usages of [[], [app("zoom", 0, 40)]]) it(`starts a new node at each speaker change ${usages.length ? "with" : "without"} app context`, () => {
    const entries = buildActivityTimeline([record("speech", [
      [1, "Hello.", "me"], [4, "Still me.", "me"], [7, "Someone else."], [10, "Back again.", "me"]
    ])], usages);
    expect(entries.map(entry => entry.paragraph.text)).toEqual(["Back again.", "Someone else.", "Hello.\nStill me."]);
    expect(entries.map(entry => entry.paragraph.segments[0].voiceIdentity)).toEqual(["me", undefined, "me"]);
    expect(entries.every(entry => entry.paragraph.segments.length === 1)).toBe(true);
    expect(entries.map(entry => [entry.paragraph.startedAt, entry.paragraph.endedAt])).toEqual([
      [at(10), at(12)], [at(7), at(9)], [at(1), at(6)]
    ]);
    expect(new Set(entries.map(entry => entry.id)).size).toBe(3);
  });

  it("splits when the apps annotating speech change, including secondary apps", () => {
    const items = [record("speech", [[1, "Meeting.", "me"], [10, "Dictation.", "me"], [20, "Meeting again.", "me"]])];
    const usages = [app("zoom", 0, 30), app("dictation", 9, 15)];
    const entries = buildActivityTimeline(items, usages);
    expect(entries.map(entry => entry.paragraph.text)).toEqual(["Meeting again.", "Dictation.", "Meeting."]);
    expect(entries.map(entry => entry.contexts.map(context => context.appId))).toEqual([["zoom"], ["dictation", "zoom"], ["zoom"]]);
    expect(buildActivityTimeline(items, usages, "", "dictation").map(entry => entry.paragraph.text)).toEqual(["Dictation."]);
  });

  it("keeps source changes as separate nodes even when the same speaker and app resume", () => {
    const items = [record("mic", [[1, "Before playback.", "me"], [10, "After playback.", "me"]]),
      record("system", [[5, "Playback.", "me"]], "system")];
    const usages = [app("zoom", 0, 30)];
    const entries = buildActivityTimeline(items, usages);
    expect(entries.map(entry => entry.paragraph.text)).toEqual(["After playback.", "Playback.", "Before playback."]);
    expect(entries.map(entry => entry.paragraph.audioSource)).toEqual(["microphone", "system", "microphone"]);
    expect(entries[1].paragraph.segments[0].voiceIdentity).toBeUndefined();
    expect(entries[1].contexts).toEqual([]);
    // Filtering hides playback without merging the microphone turns around it.
    expect(buildActivityTimeline(items, usages, "", "zoom").map(entry => entry.paragraph.text)).toEqual(["After playback.", "Before playback."]);
  });

  it("searches individual nodes without pulling other speakers into the result", () => {
    const items = [record("speech", [[1, "My proposal.", "me"], [5, "Another response."]])];
    expect(buildActivityTimeline(items, [app("zoom", 0, 30)], "proposal").map(entry => entry.paragraph.text)).toEqual(["My proposal."]);
  });

  for (const usages of [[], [app("zoom", 0, 300)]]) {
    it(`caps continuous nodes at one minute ${usages.length ? "with" : "without"} app context`, () => {
      const chunks: [number, string, "me"][] = Array.from({ length: 18 }, (_, index) => [1 + index * 8, `第${index}句。`, "me"]);
      const entries = buildActivityTimeline([record("long", chunks)], usages);
      expect(entries.length).toBeGreaterThan(1);
      expect(entries.every(entry => Date.parse(entry.paragraph.endedAt) - Date.parse(entry.paragraph.startedAt) <= 60_000)).toBe(true);
      expect([...entries].reverse().map(entry => entry.paragraph.text).join("").replace(/\s/gu, "")).toBe(chunks.map(chunk => chunk[1]).join(""));
      expect(entries.every(entry => entry.paragraph.segments[0].voiceIdentity === "me")).toBe(true);
    });

    it(`starts a fresh node after fifteen seconds of silence ${usages.length ? "with" : "without"} app context`, () => {
      const entries = buildActivityTimeline([record("pause", [[1, "Before the pause."], [19, "After the pause."]])], usages);
      expect(entries.map(entry => entry.paragraph.text)).toEqual(["After the pause.", "Before the pause."]);
    });

    it(`splits long transcripts at sentence boundaries ${usages.length ? "with" : "without"} app context`, () => {
      const text = "这是一段完整的话。".repeat(80);
      const entries = buildActivityTimeline([record("batch", [[1, text, "me"]])], usages);
      expect(entries.length).toBeGreaterThan(2);
      expect(entries.every(entry => entry.paragraph.text.length <= 240 && entry.paragraph.text.endsWith("。"))).toBe(true);
      expect(entries.map(entry => entry.paragraph.text).join("")).toBe(text);
      expect(new Set(entries.map(entry => entry.id)).size).toBe(entries.length);
      expect(entries.every(entry => entry.paragraph.startedAt === at(1) && entry.paragraph.endedAt === at(3))).toBe(true);
      expect(entries.every(entry => entry.contexts.length === usages.length)).toBe(true);
    });
  }

  it("retains all text and intact emoji when a long utterance has no punctuation", () => {
    const text = "长词🙂".repeat(200);
    const entries = buildActivityTimeline([record("no-boundaries", [[1, text]])], [app("zoom", 0, 10)]);
    expect(entries.map(entry => entry.paragraph.text).join("")).toBe(text);
    expect(entries.every(entry => entry.paragraph.text.length <= 240 && !/[\uD800-\uDBFF]$|^[\uDC00-\uDFFF]/u.test(entry.paragraph.text))).toBe(true);
  });

  it("retains the full time range and ordering when a shorter capture overlaps", () => {
    const speech = record("overlap", [[1, "Long utterance.", "me"], [5, "Short overlap.", "me"]]);
    speech.transcriptChunks[0].endedAt = at(30);
    const entries = buildActivityTimeline([speech, record("playback", [[20, "System playback."]], "system")], [app("zoom", 0, 60)]);
    expect(entries[0].paragraph).toMatchObject({ audioSource: "microphone", startedAt: at(1), endedAt: at(30) });
    expect(entries[0].paragraph.segments[0].endedAt).toBe(at(30));
    expect(entries[1].paragraph.audioSource).toBe("system");
  });
});
