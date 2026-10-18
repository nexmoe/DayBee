import { AudioSourceLabel } from "@/components/audio-source-label";
import { memo, useMemo, useState } from "react";
import { FolderCog, FolderOpen, Mic } from "lucide-react";
import type { AppLanguage, MicrophoneUsage, RecordingHistoryItem } from "@daybee/shared";
import { RecordingContexts } from "@/components/recording-context";
import { createRecordingContextIndex } from "@/lib/recording-context";
import { desktopActions } from "@/lib/desktop-store";
import { Button } from "@/components/ui/legacy-button";
import { createT, resolveIntlLocale } from "@/lib/i18n";
import { toastActions } from "@/lib/toast-store";

type DayGroup = {
  dateKey: string;
  dayLabel: string;
  folderPath: string;
  total: number;
  transcribed: number;
  microphone: number;
  system: number;
  pending: number;
  recording: number;
};

const formatDayLabel = (value: string, language: AppLanguage): string => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value.slice(0, 10) || value;
  }
  const label = new Intl.DateTimeFormat(resolveIntlLocale(language), {
    day: "numeric",
    month: "short",
    weekday: "short"
  }).format(date);
  // WebKit's ICU adds a space before the weekday; keep Chromium's labels.
  return language === "zh-CN" ? label.replace(/\s*(?:星期|周)([一二三四五六日天])/g, "周$1") : label;
};

const toDateKey = (value: string): string => {
  const date = new Date(value);
  if (!Number.isNaN(date.getTime())) {
    return date.toISOString().slice(0, 10);
  }
  return value.slice(0, 10);
};

const summarizeDays = (items: RecordingHistoryItem[], language: AppLanguage): DayGroup[] => {
  const groups = new Map<string, DayGroup>();

  for (const item of items) {
    const dateKey = toDateKey(item.startedAt);
    const current =
      groups.get(dateKey) ??
      {
        dateKey,
        dayLabel: formatDayLabel(item.startedAt, language),
        folderPath: item.folderPath,
        pending: 0,
        recording: 0,
        total: 0,
        microphone: 0,
        system: 0,
        transcribed: 0
      };

    current.total += 1;
    if (item.audioSource === "system") current.system += 1;
    else current.microphone += 1;
    current.folderPath = item.folderPath || current.folderPath;

    if (item.status === "ok") {
      current.transcribed += 1;
    } else if (item.status === "recording") {
      current.recording += 1;
    } else {
      current.pending += 1;
    }

    groups.set(dateKey, current);
  }

  return [...groups.values()].sort((left, right) => right.dateKey.localeCompare(left.dateKey));
};

const buildSummary = (group: DayGroup, language: AppLanguage): string => {
  const t = createT(language);
  const parts = [t("historySummaryTotal", { count: group.total })];

  if (group.transcribed > 0) {
    parts.push(t("historySummaryTranscribed", { count: group.transcribed }));
  }
  if (group.pending > 0) {
    parts.push(t("historySummaryPending", { count: group.pending }));
  }
  if (group.recording > 0) {
    parts.push(t("historySummaryRecording", { count: group.recording }));
  }

  return parts.join(" · ");
};

export const RecordingHistory = memo(function RecordingHistory({
  items,
  language,
  maxDays,
  microphoneActivity = [],
  appFilter = "",
  onSelectApp
}: {
  items: RecordingHistoryItem[];
  language: AppLanguage;
  maxDays?: number;
  microphoneActivity?: MicrophoneUsage[];
  appFilter?: string;
  onSelectApp?: (appId: string) => void;
}) {
  const t = createT(language);
  const [runningDays, setRunningDays] = useState<string[]>([]);
  const contexts = useMemo(() => createRecordingContextIndex(microphoneActivity), [microphoneActivity]);
  const timeFormat = useMemo(() => new Intl.DateTimeFormat(resolveIntlLocale(language), {
    hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23"
  }), [language]);
  const matchingItems = useMemo(
    () => items.filter(item => !appFilter || contexts.recording(item).some(app => app.appId === appFilter)),
    [items, appFilter, contexts]
  );
  const groups = useMemo(() => summarizeDays(matchingItems, language).slice(0, maxDays), [matchingItems, language, maxDays]);
  const filesByDay = useMemo(() => {
    if (!appFilter) return new Map<string, RecordingHistoryItem[]>();
    const grouped = new Map<string, RecordingHistoryItem[]>();
    for (const item of matchingItems) {
      const dateKey = toDateKey(item.startedAt);
      const current = grouped.get(dateKey);
      if (current) current.push(item);
      else grouped.set(dateKey, [item]);
    }
    return grouped;
  }, [appFilter, matchingItems]);

  if (matchingItems.length === 0) {
    return (
      <div className="rounded-2xl border border-[color:var(--border)] bg-[color:var(--surface)] px-4 py-5">
        <p className="text-sm font-medium text-[color:var(--foreground)]">{t("historyTitle")}</p>
        <p className="mt-1 text-xs leading-5 text-[color:var(--muted)]">
          {t(appFilter ? "historyContextEmptyDescription" : "historyEmptyDescription")}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {appFilter && <p className="text-xs leading-5 text-[color:var(--muted)]">{t("recordingContextFilesHint")}</p>}
      {groups.map((group) => (
        <div
          key={group.dateKey}
          className="overflow-hidden rounded-2xl border border-[color:var(--border)] bg-[color:var(--surface)] px-4 py-3.5"
        >
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0 flex-1">
              <div className="flex min-w-0 items-center gap-2">
                <Mic className="h-3.5 w-3.5 shrink-0 text-[color:var(--accent)]" />
                <p className="text-sm font-medium text-[color:var(--foreground)]">
                  {group.dayLabel}
                </p>
              </div>
              <p className="mt-1 text-xs leading-5 text-[color:var(--foreground)]">
                {buildSummary(group, language)}
                <span className="block text-[color:var(--muted)]" data-testid="history-source-summary">
                  {t("historySourceSummary", { microphone: group.microphone, system: group.system })}
                </span>
              </p>
              <p
                className="mt-1 break-all text-xs leading-5 text-[color:var(--muted)]"
                title={group.folderPath}
              >
                {group.folderPath}
              </p>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              {group.pending > 0 && !appFilter && (
                <Button
                  aria-label={
                    runningDays.includes(group.dateKey)
                      ? t("historyProcessing")
                      : t("historyRunTranscribeTitle")
                  }
                  className="w-8 shrink-0 px-0"
                  disabled={runningDays.includes(group.dateKey)}
                  size="sm"
                  title={
                    runningDays.includes(group.dateKey)
                      ? t("historyProcessing")
                      : t("historyRunTranscribeTitle")
                  }
                  variant="subtle"
                  onClick={async () => {
                    setRunningDays((current) => [...current, group.dateKey]);
                    try {
                      await desktopActions.runTranscribe(group.folderPath);
                      toastActions.show({
                        message: t("historyTranscribeStartedMessage", {
                          dayLabel: group.dayLabel
                        }),
                        title: t("historyTranscribeStartedTitle")
                      });
                    } finally {
                      setRunningDays((current) =>
                        current.filter((value) => value !== group.dateKey)
                      );
                    }
                  }}
                >
                  <FolderCog className="h-3.5 w-3.5" />
                </Button>
              )}
              <Button
                aria-label={t("historyOpenFolderTitle")}
                className="w-8 shrink-0 px-0"
                size="sm"
                title={t("historyOpenFolderTitle")}
                variant="subtle"
                onClick={() => desktopActions.openRecordingFolder(group.folderPath)}
              >
                <FolderOpen className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
          {appFilter && <ul className="recording-context-files">
            {(filesByDay.get(group.dateKey) ?? []).map(item => (
              <li key={item.id} className="recording-context-file" data-audio-path={item.audioPath}>
                <div className="min-w-0 flex-1">
                  <AudioSourceLabel source={item.audioSource} language={language} />
                  <p className="break-all text-xs">{item.audioPath.split(/[\\/]/).at(-1)}</p>
                  <time className="text-xs text-[color:var(--muted)]" dateTime={item.startedAt}>
                    {Number.isFinite(Date.parse(item.startedAt)) ? timeFormat.format(new Date(item.startedAt)) : t("timelineUnknownTime")}
                  </time>
                  <RecordingContexts apps={contexts.recording(item)} language={language} onSelect={onSelectApp} />
                </div>
                <Button variant="subtle" size="sm" className="w-8 shrink-0 px-0"
                  aria-label={t("historyOpenRecordingTitle", { name: item.audioPath.split(/[\\/]/).at(-1) ?? "" })}
                  onClick={() => void desktopActions.openRecordingFolder(item.audioPath)}>
                  <FolderOpen className="h-3.5 w-3.5" />
                </Button>
              </li>
            ))}
          </ul>}
        </div>
      ))}
    </div>
  );
});
