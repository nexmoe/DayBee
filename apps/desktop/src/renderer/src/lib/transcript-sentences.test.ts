import { describe, expect, it } from "vitest";
import { sentenceHighlights, transcriptSentences } from "./transcript-sentences";

describe("transcript sentence labels", () => {
  it("repeats the matching identity for each Chinese sentence", () => {
    const sentences = transcriptSentences({ text: "你好，测试一下。\n你是谁啊？你好，你好。", voiceIdentity: "me" });
    expect(sentences.map(sentence => sentence.text)).toEqual(["你好，测试一下。", "你是谁啊？", "你好，你好。"]);
    expect(sentences.map(sentence => sentence.voiceIdentity)).toEqual(["me", "me", "me"]);
  });

  it("preserves offsets through leading whitespace, blank lines and unfinished speech", () => {
    const segment = { text: "\n  First line.\r\n\nSecond line\n  Still speaking  " };
    const sentences = transcriptSentences(segment);
    expect(sentences.map(sentence => sentence.text)).toEqual(["First line.", "Second line", "Still speaking"]);
    for (const sentence of sentences) {
      expect(segment.text.slice(sentence.from, sentence.to)).toBe(sentence.text);
      expect(sentence.voiceIdentity).toBeUndefined();
    }
    expect(transcriptSentences({ text: " \n\n " })).toEqual([]);
  });

  it("keeps decimal numbers and closing quotation marks with their sentence", () => {
    expect(transcriptSentences({ text: 'Price is 3.14. She said “好的！” Next?' }).map(sentence => sentence.text))
      .toEqual(["Price is 3.14.", "She said “好的！”", "Next?"]);
  });

  it("does not carry a matching identity across an unknown segment", () => {
    const segments = [{ text: "我的第一句。我的第二句。", voiceIdentity: "me" as const }, { text: "别人的声音。" }];
    expect(segments.flatMap(transcriptSentences).map(sentence => sentence.voiceIdentity)).toEqual(["me", "me", undefined]);
  });

  it("highlights a search match across sentence boundaries using the original text", () => {
    const text = "\n  Hello.\nWorld!  ";
    const sentences = transcriptSentences({ text });
    expect(sentences.map(sentence => sentenceHighlights(text, sentence.from, sentence.to, /lo\.\nWor/iu))).toEqual([
      [{ text: "Hel", highlighted: false }, { text: "lo.", highlighted: true }],
      [{ text: "Wor", highlighted: true }, { text: "ld!", highlighted: false }]
    ]);
    expect(sentenceHighlights(text, sentences[0].from, sentences[0].to, null))
      .toEqual([{ text: "Hello.", highlighted: false }]);
  });
});
