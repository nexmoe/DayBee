import { totalSpeechCount, type SpeechCount } from "@/lib/speech-activity";

export function StackedSpeechBar({ count, maximum }: { count: SpeechCount; maximum: number }) {
  const total = totalSpeechCount(count);
  return <span className="speech-bar-track" aria-hidden="true">
    <span className="speech-stacked-bar" data-empty={total === 0} style={{ height: total ? `${Math.max(8, total / maximum * 100)}%` : "3px" }}>
      {count.other > 0 && <span data-speaker="other" style={{ flexGrow: count.other }} />}
      {count.me > 0 && <span data-speaker="me" style={{ flexGrow: count.me }} />}
    </span>
  </span>;
}
