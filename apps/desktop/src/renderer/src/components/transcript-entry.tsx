import { useMemo } from "react";
import type { AppLanguage } from "@daybee/shared";
import { AudioSourceLabel } from "@/components/audio-source-label";
import { RecordingContexts } from "@/components/recording-context";
import { TranscriptSentences } from "@/components/transcript-sentences";
import type { TimelineEntry } from "@/lib/activity-timeline";
import { createT, resolveIntlLocale } from "@/lib/i18n";
import { transcriptTime, transcriptVoiceSummary } from "@/lib/transcript-paragraphs";

export function TranscriptEntry({ entry, language }: {
  entry: TimelineEntry;
  language: AppLanguage;
}) {
  const t = createT(language);
  const { paragraph, contexts } = entry;
  const formats = useMemo(() => {
    const locale = resolveIntlLocale(language);
    return {
      date: new Intl.DateTimeFormat(locale, { year: "numeric", month: "short", day: "numeric" }),
      time: new Intl.DateTimeFormat(locale, { hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23" })
    };
  }, [language]);
  const renderTime = (value: string) => {
    const at = transcriptTime(value);
    const valid = Number.isFinite(at);
    const fullTime = valid ? `${formats.date.format(at)} ${formats.time.format(at)}` : t("timelineUnknownTime");
    return <time className="transcript-clock" dateTime={valid ? new Date(at).toISOString() : undefined}
      title={fullTime} aria-label={fullTime}>{valid ? formats.time.format(at) : fullTime}</time>;
  };
  const voice = transcriptVoiceSummary(paragraph);
  const label = t(voice === "me" ? "voiceIdentityMe" : voice === "partial" ? "voiceIdentityPartial" : "voiceIdentityUnknown");
  const title = voice === "partial" ? t("voiceIdentityPartialDescription") : voice === "unknown" ? t("voiceIdentityUnknownDescription") : undefined;

  return (
    <article className="transcript-entry transcript-turn" data-voice-identity={voice}>
      <header className="transcript-entry-header">
        <span className="transcript-speaker-name" title={title}>{label}</span>
        <AudioSourceLabel source={paragraph.audioSource} language={language} />
        <span className="transcript-time-range">
          {renderTime(paragraph.startedAt)}
          {paragraph.endedAt !== paragraph.startedAt && <><span aria-hidden="true">–</span>{renderTime(paragraph.endedAt)}</>}
        </span>
      </header>
      {contexts.length > 0 && <RecordingContexts apps={contexts} language={language} />}
      <TranscriptSentences segment={{ text: paragraph.text }} />
    </article>
  );
}
