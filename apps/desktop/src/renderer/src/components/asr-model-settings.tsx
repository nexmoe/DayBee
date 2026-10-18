import { useState } from "react";
import { Check } from "lucide-react";
import type { ASRModel, DesktopSnapshot } from "@daybee/shared";
import { SettingField } from "@/components/setting-field";
import { Button } from "@/components/ui/button";
import { desktopActions } from "@/lib/desktop-store";
import type { MessageKey } from "@/lib/i18n";

type Translator = (key: MessageKey, params?: Record<string, number | string>) => string;
const descriptions: Record<ASRModel, MessageKey> = {
  "speech-analyzer": "asrModelSpeechAnalyzerDescription",
  "whisper-tiny": "asrModelWhisperTinyDescription",
  "whisper-base": "asrModelWhisperBaseDescription",
  "whisper-small": "asrModelWhisperSmallDescription",
  "qwen3-0.6b": "asrModelQwenDescription",
  "firered-asr2-aed": "asrModelFireRedAEDDescription",
  "firered-asr2-ctc": "asrModelFireRedCTCDescription",
  "sensevoice-2025": "asrModelSenseVoiceDescription"
};

export function ASRModelSettings({ snapshot, t }: { snapshot: DesktopSnapshot; t: Translator }) {
  const [pending, setPending] = useState<ASRModel | null>(null);
  const models = snapshot.asrModels;
  const downloading = models?.downloadingModel;
  const switchBlocked = Boolean(models?.busy && !snapshot.status.recording);
  const disabled = Boolean(pending || downloading || snapshot.status.downloading);
  const select = async (id: ASRModel) => {
    setPending(id);
    try {
      await desktopActions.selectASRModel(id, !switchBlocked);
    } catch {
      // The action shows the error toast and restores the persisted selection.
    } finally {
      setPending(null);
    }
  };

  return (
    <SettingField
      title={t("asrModelTitle")}
      description={t(switchBlocked ? "asrModelBusy" : "asrModelDescription")}
      layout="stacked"
      testId="asr-model-settings"
      control={
        <div className="divide-y divide-[color:var(--border)]">
          {(models?.items ?? []).map((model) => {
            const system = model.id === "speech-analyzer";
            const active = model.id === snapshot.settings.recording.asrModel;
            const working = downloading === model.id || pending === model.id;
            const action = active && model.ready ? "asrModelCurrent" : model.ready ? "asrModelUse"
              : system ? (switchBlocked ? "asrModelPrepare" : "asrModelPrepareUse")
              : switchBlocked ? "asrModelDownload" : "asrModelDownloadUse";
            const size = model.estimatedBytes >= 1_000_000_000
              ? `${(model.estimatedBytes / 1_000_000_000).toFixed(2)} GB`
              : `${Math.round(model.estimatedBytes / 1_000_000)} MB`;
            return (
              <div key={model.id} data-testid={`asr-model-${model.id}`} className="flex min-w-0 flex-wrap items-center justify-between gap-x-4 gap-y-2 py-3 first:pt-0 last:pb-0">
                <div className="min-w-0 flex-1 basis-52">
                  <div className="flex flex-wrap items-center gap-2 text-sm text-[color:var(--foreground)]">
                    <span>{model.name}</span>
                    {active && <span className="inline-flex items-center gap-1 text-xs text-[color:var(--muted)]"><Check aria-hidden className="size-3" />{t("asrModelCurrent")}</span>}
                  </div>
                  <p className="mt-1 text-xs leading-5 text-[color:var(--muted)]">{t(descriptions[model.id])}</p>
                  <p className="text-xs leading-5 text-[color:var(--muted)]">
                    {system ? t("asrModelSystemSize") : t("asrModelSize", { size })} · {t(working ? "asrModelPreparing"
                      : system ? (model.ready ? "asrModelSystemReady" : "asrModelSystemNotReady")
                      : model.ready ? "asrModelInstalled" : "asrModelNotInstalled")}
                  </p>
                  {downloading === model.id && snapshot.status.downloading && (
                    <progress aria-label={t("modelDownloadTitle")} className="mt-2 h-1.5 w-full accent-[color:var(--foreground)]" max={100} value={snapshot.status.downloadProgress ?? undefined} />
                  )}
                </div>
                <Button size="compact" variant="secondary" loading={working} disabled={disabled || (model.ready && (active || switchBlocked))}
                  aria-label={t(action, { model: model.name })} onClick={() => void select(model.id)}>
                  {t(action, { model: "" }).trim()}
                </Button>
              </div>
            );
          })}
        </div>
      }
    />
  );
}
