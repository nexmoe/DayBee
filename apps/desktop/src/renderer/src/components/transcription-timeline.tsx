import { memo, useMemo } from "react";
import type { AppLanguage, MicrophoneActivitySnapshot, RecordingHistoryItem } from "@daybee/shared";
import { ActiveMicrophoneApps } from "@/components/recording-context";
import { TranscriptEntry } from "@/components/transcript-entry";
import { createT } from "@/lib/i18n";
import { typeClass } from "@/lib/size-context";
import { buildActivityTimeline } from "@/lib/activity-timeline";
import { useTimelineVirtualizer } from "@/hooks/use-timeline-virtualizer";
import { filterHistoryByDate } from "@/lib/speech-activity";
import "@/transcription-timeline.css";

export const TranscriptionTimeline = memo(function TranscriptionTimeline({ items, microphoneActivity, language, dateKey = null, active = true }: {
  items: RecordingHistoryItem[]; microphoneActivity: MicrophoneActivitySnapshot;
  language: AppLanguage; dateKey?: string | null; active?: boolean;
}) {
  const t = createT(language);
  const datedItems = useMemo(() => filterHistoryByDate(items, dateKey), [items, dateKey]);
  const events = useMemo(() => buildActivityTimeline(datedItems, microphoneActivity.items), [datedItems, microphoneActivity.items]);
  const { listRef, virtualizer } = useTimelineVirtualizer(events, active);
  const notice = microphoneActivity.error ? t("microphoneActivityError") :
    !microphoneActivity.supported ? t("microphoneActivityUnsupported") : null;

  if (events.length === 0) {
    const title = dateKey ? "timelineDateEmptyTitle" : "timelineEmptyTitle";
    const description = dateKey ? "timelineDateEmptyDescription" : "timelineEmptyDescription";
    return (
      <div className="px-1">
      <ActiveMicrophoneApps activity={microphoneActivity} language={language} />
      <div className="transcript-empty" role={dateKey ? "status" : undefined}>
        <h2 className={typeClass("subtitle")}>{t(title)}</h2>
        <p className={`${typeClass("body")} text-muted-foreground`}>{t(description)}</p>
        {notice && <p role="status" className={`${typeClass("caption")} text-muted-foreground`}>{notice}</p>}
      </div>
      </div>
    );
  }

  return (
    <div className="px-1">
    <ActiveMicrophoneApps activity={microphoneActivity} language={language} />
    {notice && <p role="status" className={`microphone-notice ${typeClass("caption")}`}>{notice}</p>}
    <ol ref={listRef} className="transcription-timeline" aria-label={t("timelineLabel")} style={{ height: virtualizer.getTotalSize() }}>
      {virtualizer.getVirtualItems().map(row => {
        const index = row.index;
        const event = events[index];
        return (
          <li key={row.key} ref={virtualizer.measureElement} className="transcript-event" data-index={index}
            data-last={index === events.length - 1} data-audio-source={event.paragraph.audioSource}
            aria-posinset={index + 1} aria-setsize={events.length}
            style={{ transform: `translateY(${row.start - virtualizer.options.scrollMargin}px)` }}>
            <TranscriptEntry entry={event} language={language} />
          </li>
        );
      })}
    </ol>
    </div>
  );
});
