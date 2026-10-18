import { Fragment } from "react";
import type { TranscriptSegment } from "@/lib/transcript-paragraphs";
import { transcriptReadingParagraphs } from "@/lib/transcript-sentences";

export function TranscriptSentences({ segment }: { segment: TranscriptSegment }) {
  const paragraphs = transcriptReadingParagraphs(segment);
  return (
    <div className="transcript-prose">
      {paragraphs.map((sentences, index) => (
        <Fragment key={sentences[0].from}>
          {index > 0 && "\n\n"}
          <p className="transcript-paragraph">
            {sentences.map((sentence, index) => (
              <Fragment key={sentence.from}>
                {index > 0 && segment.text.slice(sentences[index - 1].to, sentence.from)}
                <span className="transcript-text">
                  {sentence.text}
                </span>
              </Fragment>
            ))}
          </p>
        </Fragment>
      ))}
    </div>
  );
}
