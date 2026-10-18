import type { TranscriptSegment } from "./transcript-paragraphs";

export interface TranscriptSentence extends TranscriptSegment { from: number; to: number }

// Preserve original offsets and spacing without merging identity boundaries.
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
// retaining their original spacing and respecting explicit blanks.
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
