import type { MicrophoneUsage } from "@daybee/shared";
import { transcriptTime } from "./transcript-paragraphs";

export type RecordingContext = Pick<MicrophoneUsage, "appId" | "appName" | "appIcon">;
export type TimeRange = { startedAt: string; endedAt: string };

const identifiedApp = (app: RecordingContext) => Boolean(app.appId.trim()) && !/^pid:/i.test(app.appId.trim());

export function createRecordingContextIndex(usages: MicrophoneUsage[], now = Date.now()) {
  const intervals = usages.filter(identifiedApp).map(usage => ({
    usage, start: transcriptTime(usage.startedAt),
    end: usage.endedAt === null ? now : transcriptTime(usage.endedAt)
  })).filter(item => Number.isFinite(item.start) && Number.isFinite(item.end) && item.end >= item.start);

  const matching = (range: TimeRange) => {
    const start = transcriptTime(range.startedAt), end = transcriptTime(range.endedAt);
    if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return [];
    // Intervals are half-open: speech starting exactly when an app stops is
    // outside that context. Legacy records without durations remain points.
    return intervals.filter(item => start === end ?
      item.start <= start && (start < item.end || (item.usage.endedAt === null && start === item.end)) :
      start < item.end && item.start < end);
  };

  const contexts = (ranges: TimeRange[]): RecordingContext[] => {
    const apps = new Map<string, RecordingContext>();
    for (const range of ranges) for (const { usage } of matching(range)) {
      if (!apps.has(usage.appId)) apps.set(usage.appId, usage);
    }
    return [...apps.values()].sort((a, b) => a.appId.localeCompare(b.appId));
  };

  return {
    contexts,
    // Keep the current capture session as the timeline anchor while it overlaps
    // speech. Other apps joining it add annotations without splitting the scene.
    // With no current anchor, use the earliest observed overlapping session.
    key: (range: TimeRange, ranges: TimeRange[] = [range], previous = "") => {
      const matches = [...new Map(ranges.flatMap(source => matching(source))
        .map(item => [item.usage.id, item])).values()];
      if (matches.some(item => item.usage.id === previous)) return previous;
      matches.sort((a, b) => a.start - b.start || b.end - a.end || a.usage.id.localeCompare(b.usage.id));
      return matches[0]?.usage.id ?? "";
    }
  };
}

export function availableRecordingContexts(usages: MicrophoneUsage[]): RecordingContext[] {
  const apps = new Map<string, RecordingContext>();
  for (const app of usages.filter(identifiedApp)) if (!apps.has(app.appId)) apps.set(app.appId, app);
  return [...apps.values()].sort((a, b) => a.appName.localeCompare(b.appName));
}
