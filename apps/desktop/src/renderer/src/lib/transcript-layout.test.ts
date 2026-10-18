import { describe, expect, it } from "vitest";
import { sentenceHighlights, transcriptReadingParagraphs } from "./transcript-sentences";

describe("automatic reading paragraphs", () => {
  it("groups short capture fragments instead of displaying one line per fragment", () => {
    const text = Array.from({ length: 10 }, (_, index) => `第${index}句。`).join("\n");
    const paragraphs = transcriptReadingParagraphs({ text });
    expect(paragraphs.map(paragraph => paragraph.length)).toEqual([4, 4, 2]);
    expect(paragraphs.flat().map(sentence => sentence.text).join("")).toBe(text.replace(/\n/gu, ""));
  });

  it("starts the next paragraph after a complete sentence reaches the reading length", () => {
    const sentence = `${"完整的内容".repeat(13)}。`;
    const paragraphs = transcriptReadingParagraphs({ text: sentence.repeat(5) });
    expect(paragraphs.map(paragraph => paragraph.length)).toEqual([2, 2, 1]);
    expect(paragraphs.flat().every(part => part.text === sentence)).toBe(true);
  });

  it("preserves explicit blank lines and English sentence spacing", () => {
    const text = "First sentence. Second sentence.\n\nA new paragraph. Another sentence.";
    const paragraphs = transcriptReadingParagraphs({ text });
    expect(paragraphs.map(paragraph => paragraph.map(sentence => sentence.text))).toEqual([
      ["First sentence.", "Second sentence."], ["A new paragraph.", "Another sentence."]
    ]);
    expect(paragraphs.flat().every(sentence => text.slice(sentence.from, sentence.to) === sentence.text)).toBe(true);
  });

  it("keeps search offsets intact across an automatic paragraph break", () => {
    const text = "One. Two. Three. Four. Five.";
    const paragraphs = transcriptReadingParagraphs({ text });
    const parts = paragraphs.flat().flatMap(sentence => sentenceHighlights(text, sentence.from, sentence.to, /Four\. Five/iu));
    expect(paragraphs).toHaveLength(2);
    expect(parts.filter(part => part.highlighted).map(part => part.text)).toEqual(["Four.", "Five"]);
    expect(transcriptReadingParagraphs({ text: "  \n" })).toEqual([]);
  });
});
