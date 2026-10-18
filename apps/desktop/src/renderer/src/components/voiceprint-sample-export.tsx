import { useState } from "react";
import type { AppLanguage, DesktopSnapshot, RecordingHistoryItem } from "@daybee/shared";
import type { MessageKey } from "@/lib/i18n";
import { Button } from "@/components/ui/legacy-button";
import { resolveIntlLocale } from "@/lib/i18n";
import { desktopActions } from "@/lib/desktop-store";
import { voiceprintExportErrorCopy } from "@/lib/voiceprint-copy";

type Translator = (key: MessageKey, params?: Record<string, number | string>) => string;

function finishedRecordings(snapshot: DesktopSnapshot): RecordingHistoryItem[] {
  return snapshot.history.filter((item) => item.status !== "recording" && item.audioPath.trim() !== "");
}

function clipPreview(text: string): string {
  const points = Array.from(text.trim());
  return points.length <= 32 ? points.join("") : `${points.slice(0, 32).join("")}…`;
}

function formatStartedAt(value: string, language: AppLanguage, unknown: string): string {
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed)) return unknown;
  const locale = resolveIntlLocale(language);
  const at = new Date(parsed);
  const day = new Intl.DateTimeFormat(locale, { month: "short", day: "numeric" }).format(at);
  const time = new Intl.DateTimeFormat(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(at);
  return `${day} ${time}`;
}

function recordingLabel(item: RecordingHistoryItem, language: AppLanguage, t: Translator): string {
  const preview = clipPreview(item.textPreview);
  const source = t(item.audioSource === "system" ? "audioSourceSystem" : "audioSourceMicrophone");
  const when = formatStartedAt(item.startedAt, language, t("timelineUnknownTime"));
  return preview === "" ? `${when} · ${source}` : `${when} · ${source} · ${preview}`;
}

export function VoiceprintSampleExport({ snapshot, t }: { snapshot: DesktopSnapshot; t: Translator }) {
  const completed = finishedRecordings(snapshot);
  const busy = snapshot.status.recording || snapshot.status.recovering || snapshot.asrModels.busy;
  const language = snapshot.settings.desktop.language;
  const [selected, setSelected] = useState(completed[0]?.id ?? "");
  const [created, setCreated] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const current = completed.some((item) => item.id === selected) ? selected : (completed[0]?.id ?? "");
  const disabled = busy || pending || current === "";

  async function onExport() {
    if (disabled) return;
    setError(null);
    setCreated(null);
    setPending(true);
    try {
      const directory = await desktopActions.pickDirectory(snapshot.settings.recording.outputDir);
      if (!directory) return;
      const exported = await desktopActions.exportVoiceprintSamples(current, directory);
      setCreated(exported);
    } catch (caught) {
      const message = caught instanceof Error ? caught.message : "";
      setError(voiceprintExportErrorCopy(message, t));
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="flex min-w-0 flex-col gap-2" data-testid="voiceprint-export-field">
      <p className="text-sm font-medium text-[color:var(--foreground)]">{t("voiceprintExportTitle")}</p>
      <p className="text-xs leading-5 text-[color:var(--muted)]">{t("voiceprintExportDescription")}</p>
      {completed.length > 0 ? (
        <select
          aria-label={t("voiceprintExportSelect")}
          className="h-8 w-full min-w-0 rounded-lg border border-[color:var(--border)] bg-[color:var(--surface)] px-2 text-xs text-[color:var(--foreground)]"
          value={current}
          onChange={(event) => {
            setSelected(event.target.value);
            setCreated(null);
            setError(null);
          }}
        >
          {completed.map((item) => (
            <option key={item.id} value={item.id}>{recordingLabel(item, language, t)}</option>
          ))}
        </select>
      ) : (
        <p className="text-xs leading-5 text-[color:var(--muted)]">{t("voiceprintExportEmpty")}</p>
      )}
      {busy && <p className="text-xs leading-5 text-[color:var(--muted)]">{t("voiceprintExportBusy")}</p>}
      <div className="flex flex-wrap gap-2">
        <Button
          data-testid="voiceprint-export-button"
          disabled={disabled}
          size="sm"
          variant="subtle"
          onClick={() => void onExport()}
        >
          {t("voiceprintExportAction")}
        </Button>
        {created && (
          <Button size="sm" variant="ghost" onClick={() => void desktopActions.openRecordingFolder(created)}>
            {t("voiceprintExportOpen")}
          </Button>
        )}
      </div>
      {created && (
        <p className="break-all text-xs leading-5 text-[color:var(--foreground)]" data-testid="voiceprint-export-result">
          {t("voiceprintExportSuccess", { path: created })}
        </p>
      )}
      {error && <p className="text-xs leading-5 text-[color:var(--foreground)]" role="alert">{error}</p>}
    </div>
  );
}
