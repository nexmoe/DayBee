import { useState } from "react";
import type { DesktopSnapshot } from "@daybee/shared";
import { Button } from "@/components/ui/button";
import { desktopActions } from "@/lib/desktop-store";
import type { MessageKey } from "@/lib/i18n";
import { typeClass } from "@/lib/type-scale";

export function VoiceprintAdaptationStatus({ snapshot, state, t }: {
  snapshot: DesktopSnapshot;
  state: MessageKey;
  t: (key: MessageKey) => string;
}) {
  const [retrying, setRetrying] = useState(false);
  const paused = state === "voiceprintAdaptationPausedReference";
  const retry = async () => {
    setRetrying(true);
    try {
      await desktopActions.retryVoiceprintReference();
    } catch {
      // The action reports errors through the shared toast.
    } finally {
      setRetrying(false);
    }
  };
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <p className={`${typeClass("caption")} ${paused ? "text-[color:var(--foreground)]" : "text-[color:var(--muted)]"}`}>{t(state)}</p>
      {paused && (
        <>
          <div className="flex flex-wrap items-center gap-2">
            {window.mygo?.platform === "darwin" && (
              <Button data-testid="voiceprint-reference-settings" variant="tertiary" size="compact"
                onClick={() => void desktopActions.openSystemAudioSettings().catch(() => {})}>
                {t("voiceprintReferenceSettings")}
              </Button>
            )}
            <Button data-testid="voiceprint-reference-retry" variant="tertiary" size="compact"
              disabled={retrying} onClick={() => void retry()}>
              {t(retrying ? "voiceprintReferenceRetrying" : "voiceprintReferenceRetry")}
            </Button>
          </div>
          {snapshot.status.voiceprintReferenceError && (
            <details className={`${typeClass("caption")} text-[color:var(--muted)]`} data-testid="voiceprint-reference-error">
              <summary className="cursor-pointer">{t("voiceprintErrorDetails")}</summary>
              <p className="mt-1 whitespace-pre-wrap break-words">{snapshot.status.voiceprintReferenceError}</p>
            </details>
          )}
        </>
      )}
    </div>
  );
}
