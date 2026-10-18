import type { RecordingHistoryItem, TranscriptChunk } from "@daybee/shared";

export type SpeechCount = { me: number; other: number };

export const emptySpeechCount = (): SpeechCount => ({ me: 0, other: 0 });
export const totalSpeechCount = (count: SpeechCount) => count.me + count.other;

// Calendar dates follow the user's local clock, including timestamps with offsets.
export function localDateKey(value: Date | string): string {
  const date = typeof value === "string" ? new Date(value) : value;
  if (!Number.isFinite(date.getTime())) return "";
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

export const dateFromKey = (key: string) => new Date(`${key}T12:00:00`);

export function shiftDate(key: string, days: number): string {
  const date = dateFromKey(key);
  date.setDate(date.getDate() + days);
  return localDateKey(date);
}

export function recentDateKeys(endingOn: string): string[] {
  return Array.from({ length: 7 }, (_, index) => shiftDate(endingOn, index - 6));
}

export function countSpeechCharacters(text: string): number {
  // Count letters and numbers once, including astral CJK characters. Whitespace,
  // punctuation, emoji and combining accents do not inflate the speaking count.
  return text.normalize("NFC").match(/[\p{L}\p{N}]/gu)?.length ?? 0;
}

function recordingChunks(item: RecordingHistoryItem): TranscriptChunk[] {
  return item.transcriptChunks?.length ? item.transcriptChunks : [{
    text: item.textPreview,
    startedAt: item.capturedRanges?.[0]?.startedAt ?? item.startedAt,
    endedAt: item.capturedRanges?.at(-1)?.endedAt ?? item.startedAt
  }];
}

function chunkTime(chunk: TranscriptChunk, item: RecordingHistoryItem): Date {
  const date = new Date(chunk.startedAt);
  return Number.isFinite(date.getTime()) ? date : new Date(item.startedAt);
}

export function buildSpeechActivity(items: RecordingHistoryItem[]): Map<string, SpeechCount> {
  const days = new Map<string, SpeechCount>();
  for (const item of items) {
    // Playback can contain the user's voice too, but is not a new spoken utterance.
    if (item.audioSource === "system") continue;
    for (const chunk of recordingChunks(item)) {
      const count = countSpeechCharacters(chunk.text);
      const date = chunkTime(chunk, item);
      const key = localDateKey(date);
      if (!count || !key) continue;
      const day = days.get(key) ?? emptySpeechCount();
      // Full and partial voiceprint matches both belong to the user's category.
      const speaker = chunk.voiceIdentity === "me" || chunk.voiceMatch === "partial" ? "me" : "other";
      day[speaker] += count;
      days.set(key, day);
    }
  }
  return days;
}

export function filterHistoryByDate(items: RecordingHistoryItem[], key: string | null): RecordingHistoryItem[] {
  if (!key) return items;
  return items.flatMap(item => {
    // Keep legacy capture intervals intact instead of turning their silent
    // gaps into one synthetic chunk when a date is selected.
    if (!item.transcriptChunks?.length) {
      return localDateKey(chunkTime(recordingChunks(item)[0], item)) === key ? [item] : [];
    }
    const chunks = recordingChunks(item).filter(chunk => localDateKey(chunkTime(chunk, item)) === key);
    if (!chunks.length) return [];
    return [{ ...item, transcriptChunks: chunks, textPreview: chunks.map(chunk => chunk.text).join("\n") }];
  });
}
