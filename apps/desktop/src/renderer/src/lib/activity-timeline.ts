import type { MicrophoneUsage, RecordingHistoryItem } from "@daybee/shared";
import { buildTranscriptParagraphs, type TranscriptParagraph } from "./transcript-paragraphs";
import { createRecordingContextIndex, type RecordingContext } from "./recording-context";

export type TimelineEntry = {
  id: string;
  paragraph: TranscriptParagraph;
  contexts: RecordingContext[];
};

// Applications annotate saved speech rather than adding events. Voiceprint
// confidence changes inside one capture do not establish a new speaker turn.
export function buildActivityTimeline(items: RecordingHistoryItem[], usages: MicrophoneUsage[], query = "", appId = "", now = Date.now()): TimelineEntry[] {
  const index = createRecordingContextIndex(usages, now);
  const term = query.trim().toLocaleLowerCase();
  return buildTranscriptParagraphs(items, {
    splitNodes: true,
    key: (range, ranges) => {
      const capture = index.key(range, ranges);
      return capture ? JSON.stringify([capture, index.contexts(ranges).map(app => app.appId)]) : "";
    }
  }).map(paragraph => ({
    id: paragraph.id, paragraph, contexts: paragraph.audioSource === "system" ? [] : index.contexts(paragraph.sourceRanges)
  })).filter(entry => (!appId || entry.contexts.some(app => app.appId === appId)) &&
    (!term || entry.paragraph.text.toLocaleLowerCase().includes(term) ||
      entry.contexts.some(app => app.appName.toLocaleLowerCase().includes(term))));
}
