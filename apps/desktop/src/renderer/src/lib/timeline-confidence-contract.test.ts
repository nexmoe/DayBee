import { describe, expect, it } from "vitest";
import type { RecordingHistoryItem } from "@daybee/shared";
import { buildActivityTimeline } from "./activity-timeline";

const at = (second: number) => new Date(Date.UTC(2026, 9, 12, 0, 0, 0) + second * 1000).toISOString();
const speech = (): RecordingHistoryItem => ({
  id: "confidence-contract", audioSource: "microphone", audioPath: "/synthetic.wav", folderPath: "/synthetic",
  inputDevice: null, status: "ok", sessionId: "session", startedAt: at(0),
  textPreview: "This is one\ncontinuous\nutterance.",
  capturedRanges: [{ startedAt: at(0), endedAt: at(8) }],
  transcriptChunks: [
    { text: "This is one", startedAt: at(0), endedAt: at(4), voiceIdentity: "me" },
    { text: "continuous", startedAt: at(4), endedAt: at(6) },
    { text: "utterance.", startedAt: at(6), endedAt: at(8), voiceIdentity: "me" }
  ]
});

describe("confidence is not a speaker turn", () => {
  it("keeps one capture together without assigning its unconfirmed text to me", () => {
    const entries = buildActivityTimeline([speech()], []);
    expect(entries).toHaveLength(1);
    const paragraph = entries[0].paragraph;
    expect(paragraph).toMatchObject({ startedAt: at(0), endedAt: at(8) });
    expect(paragraph.text.replace(/\s+/gu, " ")).toBe("This is one continuous utterance.");
    expect(paragraph.segments.map(segment => segment.text).join("").replace(/\s+/gu, " ")).toBe(paragraph.text.replace(/\s+/gu, " "));
    expect(paragraph.segments.map(segment => segment.voiceIdentity)).toEqual(["me", undefined, "me"]);
  });

  it("does not use capture continuity to absorb the next unconfirmed utterance", () => {
    const item = speech();
    item.capturedRanges!.push({ startedAt: at(9), endedAt: at(12) });
    item.transcriptChunks.push({ text: "Another voice.", startedAt: at(9), endedAt: at(12) });
    const entries = buildActivityTimeline([item], []);
    expect(entries).toHaveLength(2);
    expect(entries[0].paragraph.text).toBe("Another voice.");
    expect(entries[0].paragraph.segments[0].voiceIdentity).toBeUndefined();
    expect(entries[1].paragraph.endedAt).toBe(at(8));
  });

  it("does not infer a capture boundary when none was saved", () => {
    const item = speech();
    delete item.capturedRanges;
    expect(buildActivityTimeline([item], [])).toHaveLength(3);
  });

  it("keeps system playback separate from microphone evidence", () => {
    const system = { ...speech(), id: "playback", audioSource: "system" as const };
    for (const chunk of system.transcriptChunks) Object.assign(chunk, { voiceMatch: "partial" });
    const entries = buildActivityTimeline([system], []);
    expect(entries).toHaveLength(1);
    expect(entries[0].paragraph.segments.every(segment => !segment.voiceIdentity)).toBe(true);
    expect(entries[0].paragraph.segments.every(segment => !(segment as { voiceMatch?: string }).voiceMatch)).toBe(true);
  });

  it("does not merge an explicit partial match with the next unconfirmed capture", () => {
    const item = speech();
    item.capturedRanges = [{ startedAt: at(0), endedAt: at(8) }, { startedAt: at(9), endedAt: at(12) }];
    item.transcriptChunks = [
      Object.assign({ text: "A complete utterance.", startedAt: at(0), endedAt: at(8) }, { voiceMatch: "partial" }),
      { text: "An unconfirmed utterance.", startedAt: at(9), endedAt: at(12) }
    ];
    const entries = buildActivityTimeline([item], []);
    expect(entries).toHaveLength(2);
    expect(entries[0].paragraph.text).toBe("An unconfirmed utterance.");
    expect(entries[1].paragraph.text).toBe("A complete utterance.");
    expect(entries.every(entry => entry.paragraph.segments.every(segment => !segment.voiceIdentity))).toBe(true);
  });

  it("does not bridge a missing interval using the capture envelope", () => {
    const item = speech();
    item.transcriptChunks = [item.transcriptChunks[0], item.transcriptChunks[2]];
    // Known evidence ends at 4s and resumes at 6s. The missing interval is
    // not evidence that a speaker or their identity stayed the same.
    const entries = buildActivityTimeline([item], []);
    expect(entries).toHaveLength(2);
  });

  it("does not treat overlapping chunks as continuous confidence evidence", () => {
    const item = speech();
    item.transcriptChunks[1].startedAt = at(3);
    const entries = buildActivityTimeline([item], []);
    expect(entries).toHaveLength(2);
    expect(entries[0].paragraph.text.replace(/\s+/gu, " ")).toBe("continuous utterance.");
    expect(entries[1].paragraph.text).toBe("This is one");
  });

  it("does not collapse confidence changes using ambiguous capture ranges", () => {
    const item = speech();
    item.capturedRanges!.push({ startedAt: at(0), endedAt: at(8) });
    expect(buildActivityTimeline([item], [])).toHaveLength(3);
  });
});
