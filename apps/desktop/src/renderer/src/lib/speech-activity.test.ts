import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RecordingHistoryItem, TranscriptChunk } from "@daybee/shared";
import { buildSpeechActivity, countSpeechCharacters, filterHistoryByDate, localDateKey, shiftDate, recentDateKeys } from "./speech-activity";

const chunk = (text: string, startedAt = "2026-10-13T10:00:00+08:00", voiceIdentity?: string, voiceMatch?: string): TranscriptChunk => ({
  text, startedAt, endedAt: new Date(Date.parse(startedAt) + 5000).toISOString(), voiceIdentity, voiceMatch
});
const recording = (chunks: TranscriptChunk[], patch: Partial<RecordingHistoryItem> = {}): RecordingHistoryItem => ({
  id: "recording", sessionId: "session", audioSource: "microphone", audioPath: "/recordings/test.flac",
  folderPath: "/recordings", inputDevice: null, status: "ok", startedAt: "2026-10-13T10:00:00+08:00",
  textPreview: chunks.map(chunk => chunk.text).join("\n"), transcriptChunks: chunks, ...patch
});

beforeEach(() => vi.stubEnv("TZ", "Asia/Shanghai"));
afterEach(() => vi.unstubAllEnvs());

describe("spoken character statistics", () => {
  it("counts Unicode letters and numbers without punctuation, whitespace, emoji or doubled combining accents", () => {
    expect(countSpeechCharacters("你好，world! 42🙂𠮷 e\u0301")).toBe(11);
    expect(countSpeechCharacters(" \n，。!?🙂")).toBe(0);
  });

  it.each([undefined, "me"])("counts full and partial matches as me with partial identity %s and excludes playback", partialIdentity => {
    const microphone = recording([
      chunk("你好！", undefined, "me"), chunk("Hello world."), chunk("部分本人", undefined, partialIdentity, "partial")
    ]);
    const system = recording([chunk("不应计入。", undefined, "me")], { id: "system", audioSource: "system" });
    const day = buildSpeechActivity([microphone, system]).get("2026-10-13")!;
    expect(day).toMatchObject({ me: 6, other: 10 });
    // textPreview is a fallback, not another copy to count alongside chunks.
    expect(day.me + day.other).toBe(16);
  });

  it("buckets individual chunks by local calendar date across UTC midnight offsets", () => {
    const items = [recording([
      chunk("夜话", "2026-10-12T15:59:55Z", "me"),
      chunk("新的日子", "2026-10-12T16:00:05Z"),
      chunk("早上好", "2026-10-12T23:00:00Z", "me")
    ], { startedAt: "2026-10-12T15:00:00Z" })];
    const days = buildSpeechActivity(items);
    expect(days.get("2026-10-12")).toMatchObject({ me: 2, other: 0 });
    expect(days.get("2026-10-13")).toMatchObject({ me: 3, other: 4 });
  });

  it("counts old text-only recordings as other and ignores records without a valid date", () => {
    const legacy = recording([], { textPreview: "旧 录音。" });
    const invalid = recording([], { textPreview: "未知日期", startedAt: "invalid" });
    expect([...buildSpeechActivity([legacy, invalid])]).toMatchObject([["2026-10-13", { me: 0, other: 3 }]]);
    expect(buildSpeechActivity([]).size).toBe(0);
  });

  it("updates totals when saved transcription or speaker labels change without double counting", () => {
    const initial = recording([chunk("已经保存", undefined, "me")], { status: "recording" });
    const next = recording([chunk("已经保存", undefined, "me"), chunk("刚刚说完")], { status: "recording" });
    expect(buildSpeechActivity([initial]).get("2026-10-13")).toMatchObject({ me: 4, other: 0 });
    expect(buildSpeechActivity([next]).get("2026-10-13")).toMatchObject({ me: 4, other: 4 });
    next.transcriptChunks[1].voiceMatch = "partial";
    expect(buildSpeechActivity([next]).get("2026-10-13")).toMatchObject({ me: 8, other: 0 });
    next.transcriptChunks[1].voiceIdentity = "me";
    expect(buildSpeechActivity([next]).get("2026-10-13")).toMatchObject({ me: 8, other: 0 });
  });

});

describe("date selection", () => {
  it("filters the chunks inside a recording that spans multiple calendar days", () => {
    const item = recording([chunk("昨天", "2026-10-12T23:59:00+08:00", "me"), chunk("今天", undefined, "me")],
      { startedAt: "2026-10-12T23:00:00+08:00" });
    const filtered = filterHistoryByDate([item], "2026-10-13");
    expect(filtered).toHaveLength(1);
    expect(filtered[0].transcriptChunks.map(chunk => chunk.text)).toEqual(["今天"]);
    expect(filtered[0].textPreview).toBe("今天");
    expect(item.transcriptChunks).toHaveLength(2);
    expect(filterHistoryByDate([item], "2026-10-11")).toEqual([]);
  });

  it("retains legacy and system transcripts on their date, and all original data in the all-dates view", () => {
    const items = [recording([], { textPreview: "旧录音" }), recording([chunk("系统声音")], { id: "system", audioSource: "system" })];
    expect(filterHistoryByDate(items, "2026-10-13")).toHaveLength(2);
    expect(filterHistoryByDate(items, null)).toBe(items);
  });

  it("shows seven consecutive dates ending at the anchor across week, month and year boundaries", () => {
    expect(recentDateKeys("2026-10-13")).toEqual(["2026-10-07", "2026-10-08", "2026-10-09", "2026-10-10", "2026-10-11", "2026-10-12", "2026-10-13"]);
    expect(recentDateKeys("2026-01-01")).toEqual(["2025-12-26", "2025-12-27", "2025-12-28", "2025-12-29", "2025-12-30", "2025-12-31", "2026-01-01"]);
    expect(shiftDate("2026-01-01", -1)).toBe("2025-12-31");
    expect(shiftDate("2026-03-01", -1)).toBe("2026-02-28");
    expect(localDateKey("2026-10-12T16:00:00Z")).toBe("2026-10-13");
    expect(localDateKey("invalid")).toBe("");
  });
});
