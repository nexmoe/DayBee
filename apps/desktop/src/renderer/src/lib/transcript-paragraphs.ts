import type { AudioSource, RecordingHistoryItem } from "@daybee/shared";

export type TranscriptVoiceMatch = "partial";
export type TranscriptVoice = "me" | "partial" | "unknown";
export type TranscriptSegment = {
  text: string;
  voiceIdentity?: "me";
  voiceMatch?: TranscriptVoiceMatch;
  startedAt?: string;
  endedAt?: string;
};

export type TranscriptParagraph = {
  audioSource: AudioSource;
  id: string;
  groupId: string;
  text: string;
  startedAt: string;
  endedAt: string;
  sourceRanges: { startedAt: string; endedAt: string }[];
  segments: TranscriptSegment[];
};

export const transcriptTime = (value: string) => {
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : Number.NEGATIVE_INFINITY;
};

type TextRange = { start: number; end: number };
type TranscriptSource = {
  audioSource: AudioSource;
  id: string; audioId: string; sessionId: string; contextKey: string; text: string; startedAt: string; endedAt: string;
  ranges: { startedAt: string; endedAt: string }[];
  voiceIdentity?: "me";
  voiceMatch?: TranscriptVoiceMatch;
  captureKey: string;
};
type TranscriptGrouping = {
  key: (range: { startedAt: string; endedAt: string }, ranges: TranscriptParagraph["sourceRanges"], previous: string) => string;
  include?: (ranges: TranscriptParagraph["sourceRanges"]) => boolean;
  splitNodes?: boolean;
};
const MAX_PARAGRAPH_LENGTH = 360;
const MAX_SPEECH_GAP = 60_000;
const MAX_GROUP_DURATION = 5 * 60_000;
const MAX_NODE_LENGTH = 240;
const MAX_NODE_GAP = 15_000;
const MAX_NODE_DURATION = 60_000;

function paragraphRanges(text: string, maxLength = MAX_PARAGRAPH_LENGTH): TextRange[] {
  const ranges: TextRange[] = [];
  let start = 0, previousEnd = 0;
  const add = (from: number, to: number) => {
    while (from < to && /\s/u.test(text[from])) from++;
    while (to > from && /\s/u.test(text[to - 1])) to--;
    if (from < to) ranges.push({ start: from, end: to });
  };
  const append = (end: number, hardBreak = false) => {
    if (end - start > maxLength && previousEnd > start) {
      add(start, previousEnd);
      start = previousEnd;
    }
    while (end - start > maxLength) {
      let cut = start + maxLength;
      const space = text.lastIndexOf(" ", cut);
      if (space > start + maxLength / 2) cut = space + 1;
      // Never split a UTF-16 surrogate pair, including emoji in a transcript.
      if (/[\uD800-\uDBFF]/u.test(text[cut - 1])) cut--;
      add(start, cut);
      start = cut;
    }
    previousEnd = end;
    if (hardBreak) {
      add(start, end);
      start = end;
    }
  };
  const boundaries = /[。！？]+[”’"')）\]]*|[.!?]+[”’"')）\]]*(?=\s|$)|\n[ \t]*\n/gu;
  for (const boundary of text.matchAll(boundaries)) {
    append(boundary.index + boundary[0].length, boundary[0].includes("\n"));
  }
  append(text.length);
  add(start, text.length);
  return ranges;
}

function captureKey(id: string, ranges: { startedAt: string; endedAt: string }[], startedAt: string, endedAt: string) {
  const start = transcriptTime(startedAt), end = transcriptTime(endedAt);
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return "";
  const matches = ranges.flatMap((range, index) => {
    const from = transcriptTime(range.startedAt), to = transcriptTime(range.endedAt);
    return Number.isFinite(from) && Number.isFinite(to) && to > from && start >= from && end <= to ? [index] : [];
  });
  return matches.length === 1 ? `${id}:${matches[0]}` : "";
}

export function transcriptVoiceSummary(paragraph: Pick<TranscriptParagraph, "audioSource" | "segments">): TranscriptVoice {
  if (paragraph.audioSource === "system") return "unknown";
  let me = false, other = false, partial = false;
  for (const segment of paragraph.segments) {
    if (segment.voiceMatch === "partial") partial = true;
    if (segment.voiceIdentity === "me") me = true;
    else other = true;
  }
  if (partial || (me && other)) return "partial";
  return me ? "me" : "unknown";
}

function appendSegment(segments: TranscriptSegment[], segment: TranscriptSegment) {
  if (!segment.text) return;
  const last = segments.at(-1);
  if (!last || last.voiceIdentity !== segment.voiceIdentity || last.voiceMatch !== segment.voiceMatch) {
    segments.push({ ...segment });
    return;
  }
  last.text += segment.text;
  last.startedAt ??= segment.startedAt;
  if (segment.endedAt && (!last.endedAt || transcriptTime(segment.endedAt) > transcriptTime(last.endedAt))) {
    last.endedAt = segment.endedAt;
  }
}

function voiceSegments(text: string, spans: (TranscriptSegment & { start: number; end: number })[], from: number, to: number): TranscriptSegment[] {
  const segments: TranscriptSegment[] = [];
  let cursor = from;
  for (const span of spans) {
    const start = Math.max(span.start, from);
    const end = Math.min(span.end, to);
    if (end <= from || start >= to) continue;
    if (start > cursor) {
      const gap = text.slice(cursor, start);
      const last = segments.at(-1);
      if (last && /^\s+$/u.test(gap)) last.text += gap;
      else appendSegment(segments, { text: gap });
    }
    appendSegment(segments, {
      text: text.slice(start, end), voiceIdentity: span.voiceIdentity, voiceMatch: span.voiceMatch,
      startedAt: span.startedAt, endedAt: span.endedAt
    });
    cursor = Math.max(cursor, end);
  }
  if (cursor < to) appendSegment(segments, { text: text.slice(cursor, to) });
  return segments.length ? segments : [{ text: text.slice(from, to) }];
}

function reflow(sources: TranscriptSource[], splitNodes = false): TranscriptParagraph[] {
  let text = "";
  const spans = sources.map((item, index) => {
    const next = item.text.trim();
    if (text) {
      if (sources[index - 1].audioId === item.audioId) text += "\n";
      else if (!(/[\u3400-\u9fff。！？]$/u.test(text) && /^[\u3400-\u9fff]/u.test(next))) text += " ";
    }
    const start = text.length;
    text += next;
    return { start, end: text.length, voiceIdentity: item.voiceIdentity, voiceMatch: item.voiceMatch, item };
  });
  const ranges = paragraphRanges(text, splitNodes ? MAX_NODE_LENGTH : MAX_PARAGRAPH_LENGTH);
  const segmentSpans = spans.map(({ start, end, voiceIdentity, voiceMatch, item }) => ({
    text: item.text, start, end, voiceIdentity, voiceMatch, startedAt: item.startedAt, endedAt: item.endedAt
  }));
  if (sources[0].contextKey && !splitNodes) {
    // Reading paragraphs belong inside one app-capture entry, with one clock
    // and annotation. Text length and silent gaps do not create new events.
    const id = `microphone:${sources[0].contextKey}:${sources[0].id}`;
    const last = sources.reduce((latest, source) => transcriptTime(source.endedAt) > transcriptTime(latest.endedAt) ? source : latest);
    const pieces = ranges.map(range => text.slice(range.start, range.end));
    const segments: TranscriptSegment[] = [];
    for (const range of ranges) {
      const previous = segments.at(-1);
      if (previous) previous.text += "\n\n";
      const part = voiceSegments(text, segmentSpans, range.start, range.end);
      for (const segment of part) appendSegment(segments, segment);
    }
    return [{ id, groupId: id, audioSource: sources[0].audioSource, text: pieces.join("\n\n"), segments,
      startedAt: sources[0].startedAt, endedAt: last.endedAt, sourceRanges: sources.flatMap(source => source.ranges) }];
  }
  let sourceIndex = 0;
  return ranges.map(range => {
    while (spans[sourceIndex].end <= range.start) sourceIndex++;
    let last = sourceIndex;
    while (last + 1 < spans.length && spans[last + 1].start < range.end) last++;
    const selected = spans.slice(sourceIndex, last + 1);
    const latest = selected.reduce((latest, span) => transcriptTime(span.item.endedAt) > transcriptTime(latest.item.endedAt) ? span : latest);
    const slice = text.slice(range.start, range.end);
    return {
      id: `${spans[sourceIndex].item.id}:${range.start - spans[sourceIndex].start}`,
      groupId: sources[0].id,
      audioSource: sources[0].audioSource,
      text: slice,
      segments: voiceSegments(text, segmentSpans, range.start, range.end),
      startedAt: spans[sourceIndex].item.startedAt,
      endedAt: latest.item.endedAt,
      sourceRanges: selected.flatMap(span => span.item.ranges)
    };
  });
}

// App capture sessions preserve speech continuity. Timeline nodes additionally
// split at speaker, context and source changes in the combined stream.
export function buildTranscriptParagraphs(items: RecordingHistoryItem[], grouping?: TranscriptGrouping): TranscriptParagraph[] {
  const sources: TranscriptSource[] = items.flatMap(item => {
    const hasChunks = Boolean(item.transcriptChunks?.length);
    const captures = item.capturedRanges ?? [];
    const chunks = hasChunks ? item.transcriptChunks : [{
      text: item.textPreview, startedAt: captures[0]?.startedAt ?? item.startedAt,
      endedAt: captures.at(-1)?.endedAt ?? item.startedAt
    }];
    return chunks.filter(chunk => chunk.text.trim()).map((chunk, index) => {
      const ranges = !hasChunks && captures.length ? captures : [{ startedAt: chunk.startedAt, endedAt: chunk.endedAt }];
      const microphone = item.audioSource !== "system";
      const voiceIdentity = microphone && chunk.voiceIdentity === "me" ? "me" as const : undefined;
      const voiceMatch = microphone && chunk.voiceMatch === "partial" ? "partial" as const : undefined;
      return { ...chunk, voiceIdentity, voiceMatch, audioSource: microphone ? "microphone" as const : "system" as const, id: `${item.id}:${index}`, audioId: item.id, sessionId: item.sessionId || "",
        ranges, captureKey: captureKey(item.id, captures, chunk.startedAt, chunk.endedAt), contextKey: "" };
    });
  }).filter(source => source.audioSource === "system" ? !grouping?.include : grouping?.include?.(source.ranges) ?? true).sort((left, right) => {
    const a = transcriptTime(left.startedAt), b = transcriptTime(right.startedAt);
    return a === b ? left.id.localeCompare(right.id, undefined, { numeric: true }) : a < b ? -1 : 1;
  });
  const groups: TranscriptSource[][] = [];
  const maxGap = grouping?.splitNodes ? MAX_NODE_GAP : MAX_SPEECH_GAP;
  const maxDuration = grouping?.splitNodes ? MAX_NODE_DURATION : MAX_GROUP_DURATION;
  // Reading paragraphs can keep continuity per source. Timeline nodes instead
  // follow the combined stream so a source change always starts a new node.
  const current = new Map<AudioSource, { sources: TranscriptSource[]; end: number }>();
  let latest: { sources: TranscriptSource[]; end: number } | undefined;
  for (const source of sources) {
    const active = grouping?.splitNodes ? latest : current.get(source.audioSource);
    const group = active?.sources;
    const groupEnd = active?.end ?? Number.NEGATIVE_INFINITY;
    source.contextKey = source.audioSource === "system" ? "" : grouping?.key(source, source.ranges, group?.[0].contextKey ?? "") ?? "";
    const start = transcriptTime(source.startedAt), end = transcriptTime(source.endedAt);
    const groupStart = group ? transcriptTime(group[0].startedAt) : Number.NEGATIVE_INFINITY;
    const sameScene = Boolean(source.contextKey && source.contextKey === group?.[0].contextKey);
    const last = group?.at(-1);
    const previousEnd = last ? transcriptTime(last.endedAt) : Number.NaN;
    const sameCapture = Boolean(source.captureKey && last && source.captureKey === last.captureKey && source.audioId === last.audioId);
    const abuts = Boolean(last && start === previousEnd);
    const hole = Boolean(last && start > previousEnd);
    const voiceChanged = Boolean(last && (source.voiceIdentity !== last.voiceIdentity || source.voiceMatch !== last.voiceMatch));
    const confidenceSplit = Boolean(grouping?.splitNodes && ((voiceChanged && !(sameCapture && abuts)) || (sameCapture && hole)));
    if (!group || source.audioSource !== group[0].audioSource || source.contextKey !== group[0].contextKey ||
      confidenceSplit || !Number.isFinite(start) || !Number.isFinite(groupStart) ||
      (!sameScene && source.sessionId !== group[0].sessionId) ||
      ((grouping?.splitNodes || !sameScene) && (start - groupEnd > maxGap || Math.max(end, groupEnd) - groupStart > maxDuration))) {
      const next = [source];
      groups.push(next);
      current.set(source.audioSource, { sources: next, end });
      latest = current.get(source.audioSource);
    } else {
      group.push(source);
      active!.end = Math.max(groupEnd, end);
    }
  }
  const paragraphs = groups.flatMap(group => reflow(group, grouping?.splitNodes)).map((paragraph, order) => ({ ...paragraph, order }));
  return paragraphs.sort((left, right) => {
    const a = transcriptTime(left.endedAt), b = transcriptTime(right.endedAt);
    // Paragraphs from the same source time retain their natural reading order.
    return a === b ? left.order - right.order : a > b ? -1 : 1;
  });
}
