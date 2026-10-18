import type { DesktopSnapshot } from "@daybee/shared";
import type { MessageKey } from "@/lib/i18n";
import { ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { VoiceprintAdaptationSettings } from "@/components/voiceprint-adaptation-settings";
import { VoiceprintRuntimeHints } from "@/components/voiceprint-runtime-hints";
import { desktopActions } from "@/lib/desktop-store";
import { fontWeights } from "@/lib/font-weight";
import { typeClass } from "@/lib/type-scale";

type Translator = (key: MessageKey, params?: Record<string, number | string>) => string;

export function VoiceprintSettings({ snapshot, t, onAdaptation, onInitialize }: {
  snapshot: DesktopSnapshot;
  t: Translator;
  onAdaptation: (enabled: boolean) => void;
  onInitialize: () => void;
}) {
  const enrolled = snapshot.voiceprint.enrolled;
  const adaptationOn = snapshot.settings.recording.voiceprintAdaptation === true;
  const referencePaused = snapshot.status.recording &&
    ["unavailable", "off"].includes(snapshot.status.voiceprintReference ?? "");
  return (
    <section data-testid="voiceprint-manage-field" className="flex min-w-0 flex-col gap-4 rounded-2xl border border-[color:var(--border)] bg-[color:var(--surface)] px-4 py-4 shadow-[var(--shadow-raised-sm)]">
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
        <div className="min-w-0 flex-1 basis-64">
          <h2 className={`${typeClass("subtitle")} text-[color:var(--foreground)]`} style={{ fontVariationSettings: fontWeights.semibold }}>{t("voiceprintManageTitle")}</h2>
          <p className={`mt-1 max-w-prose ${typeClass("caption")} text-[color:var(--muted)]`}>{t(enrolled ? "voiceprintEnrolledDescription" : "voiceprintMissingDescription")}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button size="compact" className={enrolled ? undefined : "text-surface-1 [--background:var(--surface-1)]"}
            variant={enrolled ? "tertiary" : "primary"} onClick={onInitialize}>
            {t(enrolled ? "voiceprintRerecord" : "voiceprintInitialize")}
          </Button>
          {enrolled && (
            <Button size="compact" variant="ghost" onClick={() => void desktopActions.clearVoiceprint()}>{t("voiceprintClear")}</Button>
          )}
        </div>
      </div>
      <VoiceprintRuntimeHints snapshot={snapshot} t={t} alertsOnly />
      <details className="group border-t border-[color:var(--border)] pt-3" data-testid="voiceprint-advanced">
        <summary className={`flex cursor-pointer list-none items-center gap-2 ${typeClass("body")} text-[color:var(--foreground)] [&::-webkit-details-marker]:hidden`} data-testid="voiceprint-advanced-toggle">
          <ChevronRight aria-hidden className="size-3.5 shrink-0 group-open:rotate-90" />
          <span className="shrink-0">{t("voiceprintAdvanced")}</span>
          {adaptationOn && <span className={`ml-auto text-right ${typeClass("caption")} text-[color:var(--muted)]`}>{t(referencePaused ? "voiceprintAdaptationPaused" : "voiceprintAdaptationOn")}</span>}
        </summary>
        <VoiceprintAdaptationSettings snapshot={snapshot} t={t} onChange={onAdaptation} />
      </details>
    </section>
  );
}
