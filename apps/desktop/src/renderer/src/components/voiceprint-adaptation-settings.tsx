import { useId } from "react";
import type { DesktopSnapshot } from "@daybee/shared";
import type { MessageKey } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { VoiceprintAdaptationStatus } from "@/components/voiceprint-adaptation-status";
import { desktopActions } from "@/lib/desktop-store";
import { fontWeights } from "@/lib/font-weight";
import { typeClass } from "@/lib/type-scale";

type Translator = (key: MessageKey, params?: Record<string, number | string>) => string;

const enrollingPhases = new Set(["preparing", "recording", "sample-ready", "processing"]);

export function VoiceprintAdaptationSettings({ snapshot, t, onChange }: {
  snapshot: DesktopSnapshot;
  t: Translator;
  onChange: (enabled: boolean) => void;
}) {
  const enrolling = enrollingPhases.has(snapshot.voiceprint.phase);
  const locked = !snapshot.voiceprint.enrolled || enrolling;
  const count = snapshot.voiceprint.adaptationCount ?? 0;
  const progress = snapshot.voiceprint.adaptation;
  const enabled = snapshot.settings.recording.voiceprintAdaptation === true;
  const hasTotals = Boolean(progress) || count === 0;
  const hasData = count > 0 || (progress?.learnedClips ?? 0) > 0 || (progress?.assistedClips ?? 0) > 0;
  const descriptionId = useId();
  const state: MessageKey = locked ? "voiceprintAdaptationWaitingEnrollment"
    : snapshot.settings.recording.disableAsr ? "voiceprintAdaptationWaitingASR"
    : snapshot.status.voiceprintContextMismatch ? "voiceprintContextMismatch"
    : !snapshot.status.recording || snapshot.status.recovering ? "voiceprintAdaptationWaitingRecording"
    : snapshot.status.voiceprintReference === "unavailable" || snapshot.status.voiceprintReference === "off" ? "voiceprintAdaptationPausedReference"
    : snapshot.status.voiceprintReference !== "available" ? "voiceprintAdaptationWaitingReference"
    : "voiceprintAdaptationLearning";
  return (
    <div className="flex min-w-0 flex-col gap-4 pt-4" data-testid="voiceprint-adaptation-field">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
            <p className={`${typeClass("body")} text-[color:var(--foreground)]`} style={{ fontVariationSettings: fontWeights.semibold }}>{t("voiceprintAdaptationTitle")}</p>
            <span className={`${typeClass("caption")} text-[color:var(--muted)]`}>{t("voiceprintExperimental")}</span>
          </div>
          <p id={descriptionId} className={`mt-1 max-w-prose ${typeClass("caption")} text-[color:var(--muted)]`}>{t(enabled ? "voiceprintAdaptationEnabledDescription" : "voiceprintAdaptationDescription")}</p>
        </div>
        <Switch
          aria-label={t("voiceprintAdaptationTitle")}
          aria-describedby={descriptionId}
          checked={enabled}
          disabled={locked}
          onCheckedChange={(enabled: boolean) => onChange(enabled)}
        />
      </div>
      {enabled && (
        <div role="status" aria-live="polite" aria-atomic="true" className="flex flex-col gap-2" data-testid="voiceprint-adaptation-progress">
          <dl className="flex flex-wrap gap-x-8 gap-y-3">
            <div className="flex min-w-0 flex-col gap-1" data-testid="voiceprint-adaptation-learned">
              <dt className={`${typeClass("caption")} text-[color:var(--muted)]`}>{t(hasTotals ? "voiceprintAdaptationLearnedLabel" : "voiceprintAdaptationSamplesLabel")}</dt>
              <dd className={`${typeClass("body")} tabular-nums text-[color:var(--foreground)]`} style={{ fontVariationSettings: fontWeights.semibold }}>{t(hasTotals ? "voiceprintAdaptationLearnedValue" : "voiceprintAdaptationSamplesValue", { count: progress?.learnedClips ?? count })}</dd>
            </div>
            <div className="flex min-w-0 flex-col gap-1" data-testid="voiceprint-adaptation-assisted">
              <dt className={`${typeClass("caption")} text-[color:var(--muted)]`}>{t("voiceprintAdaptationAssistedLabel")}</dt>
              <dd className={`${typeClass("body")} tabular-nums text-[color:var(--foreground)]`} style={{ fontVariationSettings: fontWeights.semibold }}>{t("voiceprintAdaptationAssistedValue", { count: progress?.assistedClips ?? 0 })}</dd>
            </div>
          </dl>
          <VoiceprintAdaptationStatus snapshot={snapshot} state={state} t={t} />
        </div>
      )}
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-t border-[color:var(--border)] pt-3">
        <p className={`min-w-0 flex-1 basis-52 ${typeClass("caption")} text-[color:var(--muted)]`}>{t("voiceprintAdaptationClearNote")}</p>
        <Button
          data-testid="voiceprint-adaptation-clear"
          disabled={locked || !hasData}
          size="compact"
          variant="tertiary"
          onClick={() => void desktopActions.clearVoiceprintAdaptation()}
        >
          {t("voiceprintAdaptationClear")}
        </Button>
      </div>
    </div>
  );
}
