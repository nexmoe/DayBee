import { describe, expect, it } from "vitest";
import type { MicrophoneUsage } from "@daybee/shared";
import { availableRecordingContexts, createRecordingContextIndex } from "./recording-context";

const at = (seconds: number) => new Date(Date.parse("2026-10-09T10:00:00Z") + seconds * 1000).toISOString();
const usage = (appId: string, start: number, end: number | null): MicrophoneUsage => ({
  id: `${appId}-${start}`, appId, appName: appId, appIcon: "", startedAt: at(start), endedAt: end === null ? null : at(end)
});
describe("recording contexts", () => {
  it("ignores unidentified legacy processes in both annotations and live app lists", () => {
    const usages = [usage("pid:14559", 9, 20), usage("", 9, 20), usage("meeting", 9, 20), usage("meeting", 25, null)];
    expect(availableRecordingContexts(usages).map(app => app.appId)).toEqual(["meeting"]);
    expect(createRecordingContextIndex(usages).contexts([{ startedAt: at(10), endedAt: at(15) }]).map(app => app.appId)).toEqual(["meeting"]);
  });
});
