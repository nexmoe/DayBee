import type { TranscriptSegment } from "./transcript-paragraphs";

export interface TranscriptSentence extends TranscriptSegment { from: number; to: number }

// Preserve the original offsets for search, including leading whitespace and
// matches that cross a sentence boundary. Do not merge identity boundaries.
export function transcriptSentences(segment: TranscriptSegment): TranscriptSentence[] {
  const sentences: TranscriptSentence[] = [];
  const boundaries = /[。！？!?]+[”’"')）\]]*\s*|[.]+[”’"')）\]]*(?=\s|$)\s*|\r?\n+/gu;
  let start = 0;
  const append = (end: number) => {
    const text = segment.text.slice(start, end);
    const from = start + text.length - text.trimStart().length;
    const to = start + text.trimEnd().length;
    if (from < to) sentences.push({ ...segment, text: segment.text.slice(from, to), from, to });
    start = end;
  };
  for (const boundary of segment.text.matchAll(boundaries)) append(boundary.index + boundary[0].length);
  append(segment.text.length);
  return sentences;
}

// Capture chunks are not reading paragraphs. Group complete sentences while
// retaining their original offsets for search and respecting explicit blanks.
export function transcriptReadingParagraphs(segment: TranscriptSegment): TranscriptSentence[][] {
  const paragraphs: TranscriptSentence[][] = [];
  let current: TranscriptSentence[] = [];
  let length = 0;
  for (const sentence of transcriptSentences(segment)) {
    const previous = current.at(-1);
    const gap = previous ? segment.text.slice(previous.to, sentence.from) : "";
    if (current.length && (length >= 120 || current.length >= 4 || /\r?\n[ \t]*\r?\n/u.test(gap))) {
      paragraphs.push(current);
      current = [];
      length = 0;
    }
    current.push(sentence);
    length += sentence.text.length;
  }
  if (current.length) paragraphs.push(current);
  return paragraphs;
}

export function sentenceHighlights(text: string, from: number, to: number, search: RegExp | null) {
  const parts: { text: string; highlighted: boolean }[] = [];
  let cursor = from;
  if (search) for (const match of text.matchAll(new RegExp(search.source, "giu"))) {
    const start = Math.max(from, match.index);
    const end = Math.min(to, match.index + match[0].length);
    if (start >= end) continue;
    if (start > cursor) parts.push({ text: text.slice(cursor, start), highlighted: false });
    parts.push({ text: text.slice(start, end), highlighted: true });
    cursor = end;
  }
  if (cursor < to) parts.push({ text: text.slice(cursor, to), highlighted: false });
  return parts;
}
