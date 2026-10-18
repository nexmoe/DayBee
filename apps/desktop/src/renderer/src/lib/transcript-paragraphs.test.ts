import { describe, expect, it } from "vitest";
import type { RecordingHistoryItem } from "@daybee/shared";
import { buildTranscriptParagraphs } from "./transcript-paragraphs";

function source(id: string, startedAt: string, textPreview: string, sessionId = ""): RecordingHistoryItem {
  return { id, audioSource: "microphone", audioPath: id, folderPath: "/recordings", inputDevice: null, status: "ok",
    startedAt, textPreview, sessionId, transcriptChunks: [] };
}

describe("transcript timeline continuity", () => {
  it("separates the reported hour-long gaps and shows the latest speech first", () => {
    const paragraphs = buildTranscriptParagraphs([
      source("first", "2026-10-09T11:14:33+08:00", "然后。谁？你是谁？"),
      source("second", "2026-10-09T12:18:57+08:00", "是是是。真的好。"),
      source("third", "2026-10-09T12:56:20+08:00", "测试测试。\n行行行。")
    ]);
    expect(paragraphs.map(item => item.text)).toEqual(["测试测试。\n行行行。", "是是是。真的好。", "然后。谁？你是谁？"]);
    expect(paragraphs.every(item => item.startedAt === item.endedAt)).toBe(true);
  });

  it("still merges adjacent fragments into a complete readable sentence", () => {
    expect(buildTranscriptParagraphs([
      source("first", "2026-10-09T11:00:00Z", "今天讨论录音"),
      source("second", "2026-10-09T11:00:30Z", "质量和转写结果。")
    ])).toMatchObject([{ text: "今天讨论录音质量和转写结果。", startedAt: "2026-10-09T11:00:00Z", endedAt: "2026-10-09T11:00:30Z" }]);
  });

  it("never merges distinct recording sessions, including legacy/new boundaries", () => {
    const items = [
      source("legacy", "2026-10-09T11:00:00Z", "旧记录。"),
      source("a", "2026-10-09T11:00:01Z", "第一次录音。", "session-a"),
      source("b", "2026-10-09T11:00:02Z", "第二次录音。", "session-b")
    ];
    expect(buildTranscriptParagraphs(items).map(item => item.text)).toEqual(["第二次录音。", "第一次录音。", "旧记录。"]);
  });

  it("caps a chain of nearby fragments at five minutes", () => {
    const start = Date.parse("2026-10-09T11:00:00Z");
    const items = Array.from({ length: 16 }, (_, index) =>
      source(String(index), new Date(start + index * 45_000).toISOString(), `第${index}句。`, "same-session"));
    const paragraphs = buildTranscriptParagraphs(items);
    expect(paragraphs.length).toBeGreaterThan(1);
    expect(paragraphs.every(item => Date.parse(item.endedAt) - Date.parse(item.startedAt) <= 300_000)).toBe(true);
    expect([...paragraphs].reverse().map(item => item.text).join("")).toBe(items.map(item => item.textPreview).join(""));
  });

  it("splits speech after silence inside one long audio file using capture times", () => {
    const item = { ...source("long-audio", "2026-10-09T11:00:00Z", "前面的语音。\n停顿后的语音。", "same-session"),
      transcriptChunks: [
        { text: "前面的语音。", startedAt: "2026-10-09T11:01:00Z", endedAt: "2026-10-09T11:01:05Z" },
        { text: "停顿后的语音。", startedAt: "2026-10-09T12:40:00Z", endedAt: "2026-10-09T12:40:04Z" }
      ] };
    expect(buildTranscriptParagraphs([item])).toMatchObject([
      { text: "停顿后的语音。", startedAt: "2026-10-09T12:40:00Z", endedAt: "2026-10-09T12:40:04Z" },
      { text: "前面的语音。", startedAt: "2026-10-09T11:01:00Z", endedAt: "2026-10-09T11:01:05Z" }
    ]);
  });

  it("uses actual instants across time zones and midnight for continuous speech", () => {
    expect(buildTranscriptParagraphs([
      source("second", "2026-10-10T00:00:10+08:00", "仍然连续。"),
      source("first", "2026-10-09T15:59:50Z", "跨越午夜")
    ]).map(item => item.text)).toEqual(["跨越午夜仍然连续。"]);
  });

  it("keeps continuous live speech together and retains its identity when later speech arrives", () => {
    const first = { ...source("live", "2026-10-09T11:00:00Z", "第一句。\n第二句。", "live-session"),
      transcriptChunks: [
        { text: "第一句。", startedAt: "2026-10-09T11:00:02Z", endedAt: "2026-10-09T11:00:05Z" },
        { text: "第二句。", startedAt: "2026-10-09T11:00:10Z", endedAt: "2026-10-09T11:00:14Z" }
      ] };
    const paragraph = buildTranscriptParagraphs([first])[0];
    expect(paragraph).toMatchObject({ text: "第一句。\n第二句。", startedAt: "2026-10-09T11:00:02Z", endedAt: "2026-10-09T11:00:14Z" });
    const updated = buildTranscriptParagraphs([first, source("later", "2026-10-09T12:00:00Z", "后来的录音。", "later-session")]);
    expect(updated[1]).toMatchObject(paragraph);
  });

  it("keeps unknown timestamps separate instead of inventing continuity", () => {
    expect(buildTranscriptParagraphs([
      source("one", "unknown", "第一条。"), source("two", "unknown", "第二条。")
    ])).toHaveLength(2);
    expect(buildTranscriptParagraphs([])).toEqual([]);
  });

  it("preserves long text, paragraph breaks and emoji while splitting", () => {
    const text = `${"完整的转写内容。".repeat(100)}\n\n${"LongWord🙂".repeat(100)}`;
    const paragraphs = buildTranscriptParagraphs([source("long", "2026-10-09T11:00:00Z", text)]);
    expect(paragraphs.length).toBeGreaterThan(3);
    expect(paragraphs.map(item => item.text).join("").replace(/\s/gu, "")).toBe(text.replace(/\s/gu, ""));
    expect(paragraphs.every(item => !/[\uD800-\uDBFF]$|^[\uDC00-\uDFFF]/u.test(item.text))).toBe(true);
  });
});


describe("voice identity segments", () => {
  it("keeps long app scenes readable without blank identity segments or repeated badges", () => {
    const item = {
      ...source("long-scene", "2026-10-09T11:00:00Z", "", "same"),
      transcriptChunks: [
        { text: "This is my sentence. ".repeat(30), startedAt: "2026-10-09T11:00:00Z", endedAt: "2026-10-09T11:00:30Z", voiceIdentity: "me" },
        { text: "A different voice.", startedAt: "2026-10-09T11:01:00Z", endedAt: "2026-10-09T11:01:02Z" }
      ]
    };
    const [paragraph] = buildTranscriptParagraphs([item], { key: () => "app-scene" });
    expect(paragraph.text).toContain("\n\n");
    expect(paragraph.segments.map(segment => segment.voiceIdentity ?? "")).toEqual(["me", ""]);
    expect(paragraph.segments.every(segment => segment.text.trim().length > 0)).toBe(true);
    expect(paragraph.segments.map(segment => segment.text).join("")).toBe(paragraph.text);
    expect(paragraph.segments.map(({ startedAt, endedAt }) => [startedAt, endedAt])).toEqual([
      ["2026-10-09T11:00:00Z", "2026-10-09T11:00:30Z"],
      ["2026-10-09T11:01:00Z", "2026-10-09T11:01:02Z"]
    ]);
  });

  it("marks only matching microphone segments inside one scene and leaves the text unchanged", () => {
    const item: RecordingHistoryItem = {
      ...source("talk", "2026-10-09T11:00:00Z", "", "same"),
      transcriptChunks: [
        { text: "Hello from me.", startedAt: "2026-10-09T11:00:00Z", endedAt: "2026-10-09T11:00:02Z", voiceIdentity: "me" },
        { text: "Someone else.", startedAt: "2026-10-09T11:00:03Z", endedAt: "2026-10-09T11:00:05Z" },
        { text: "Back again.", startedAt: "2026-10-09T11:00:06Z", endedAt: "2026-10-09T11:00:08Z", voiceIdentity: "other" }
      ]
    };
    const [paragraph] = buildTranscriptParagraphs([item]);
    expect(paragraph.text).toBe("Hello from me.\nSomeone else.\nBack again.");
    expect(paragraph.text).not.toContain("我");
    expect(paragraph.segments.map(segment => segment.voiceIdentity ?? "")).toEqual(["me", ""]);
    expect(paragraph.segments.map(segment => segment.text).join("")).toBe(paragraph.text);
    expect(buildTranscriptParagraphs([item]).every(entry => entry.groupId === paragraph.groupId)).toBe(true);
  });

  it("shows old transcripts that have no voice identity", () => {
    const [paragraph] = buildTranscriptParagraphs([source("legacy", "2026-10-09T11:00:00Z", "旧记录没有声纹字段。")]);
    expect(paragraph.text).toBe("旧记录没有声纹字段。");
    expect(paragraph.segments).toEqual([{
      text: "旧记录没有声纹字段。", voiceIdentity: undefined,
      startedAt: "2026-10-09T11:00:00Z", endedAt: "2026-10-09T11:00:00Z"
    }]);
  });

  it("never labels system audio as me", () => {
    const system: RecordingHistoryItem = {
      ...source("playback", "2026-10-09T11:00:00Z", "", "same"),
      audioSource: "system",
      transcriptChunks: [{ text: "System playback.", startedAt: "2026-10-09T11:00:00Z", endedAt: "2026-10-09T11:00:02Z", voiceIdentity: "me" }]
    };
    expect(buildTranscriptParagraphs([system])[0].segments).toEqual([{
      text: "System playback.", voiceIdentity: undefined,
      startedAt: "2026-10-09T11:00:00Z", endedAt: "2026-10-09T11:00:02Z"
    }]);
  });

  it("retains each speaker turn's capture times while joining consecutive matching speech", () => {
    const item: RecordingHistoryItem = {
      ...source("turns", "2026-10-09T11:00:00Z", "", "same"),
      transcriptChunks: [
        { text: "Hello.", startedAt: "2026-10-09T11:00:00Z", endedAt: "2026-10-09T11:00:02Z", voiceIdentity: "me" },
        { text: "Still me.", startedAt: "2026-10-09T11:00:03Z", endedAt: "2026-10-09T11:00:05Z", voiceIdentity: "me" },
        { text: "Another voice.", startedAt: "2026-10-09T11:00:06Z", endedAt: "2026-10-09T11:00:08Z" },
        { text: "Back again.", startedAt: "2026-10-09T11:00:09Z", endedAt: "2026-10-09T11:00:11Z", voiceIdentity: "me" }
      ]
    };
    const [paragraph] = buildTranscriptParagraphs([item], { key: () => "meeting" });
    expect(paragraph.segments).toEqual([
      { text: "Hello.\nStill me.\n", voiceIdentity: "me", startedAt: "2026-10-09T11:00:00Z", endedAt: "2026-10-09T11:00:05Z" },
      { text: "Another voice.\n", voiceIdentity: undefined, startedAt: "2026-10-09T11:00:06Z", endedAt: "2026-10-09T11:00:08Z" },
      { text: "Back again.", voiceIdentity: "me", startedAt: "2026-10-09T11:00:09Z", endedAt: "2026-10-09T11:00:11Z" }
    ]);
    expect(paragraph.segments.map(segment => segment.text).join("")).toBe(paragraph.text);
  });

  it("keeps the latest end time when matching speech ranges overlap", () => {
    const item: RecordingHistoryItem = {
      ...source("overlap", "2026-10-09T11:00:00Z", "", "same"),
      transcriptChunks: [
        { text: "Long utterance.", startedAt: "2026-10-09T11:00:00Z", endedAt: "2026-10-09T11:00:30Z", voiceIdentity: "me" },
        { text: "Short overlap.", startedAt: "2026-10-09T11:00:05Z", endedAt: "2026-10-09T11:00:07Z", voiceIdentity: "me" }
      ]
    };
    expect(buildTranscriptParagraphs([item])[0].segments).toMatchObject([
      { startedAt: "2026-10-09T11:00:00Z", endedAt: "2026-10-09T11:00:30Z" }
    ]);
  });
});

describe("audio source separation", () => {
  it("never merges microphone and system speech from the same session", () => {
    const mic = source("mic", "2026-10-09T11:00:00Z", "我的声音。", "one-session");
    const system: RecordingHistoryItem = { ...source("system", "2026-10-09T11:00:01Z", "视频声音。", "one-session"), audioSource: "system" };
    expect(buildTranscriptParagraphs([mic, system])).toMatchObject([
      { audioSource: "system", text: "视频声音。" }, { audioSource: "microphone", text: "我的声音。" }
    ]);
    // Missing source in old saved payloads represents microphone input.
    expect(buildTranscriptParagraphs([{ ...mic, audioSource: undefined! }])[0].audioSource).toBe("microphone");
  });
});
