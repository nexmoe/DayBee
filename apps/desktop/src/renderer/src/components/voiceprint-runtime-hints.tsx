import type { DesktopSnapshot } from "@daybee/shared";
import type { MessageKey } from "@/lib/i18n";

type Translator = (key: MessageKey, params?: Record<string, number | string>) => string;

const referenceKeys: Record<string, MessageKey> = {
  off: "voiceprintReferenceOff",
  starting: "voiceprintReferenceStarting",
  available: "voiceprintReferenceAvailable",
  unavailable: "voiceprintReferenceUnavailable"
};

export function VoiceprintRuntimeHints({ snapshot, t, alertsOnly = false }: { snapshot: DesktopSnapshot; t: Translator; alertsOnly?: boolean }) {
  const asrOn = snapshot.status.asrEnabled && !snapshot.settings.recording.disableAsr;
  if (!snapshot.voiceprint.enrolled || !asrOn) {
    return null;
  }
  const reference = snapshot.status.voiceprintReference ?? "";
  const recording = snapshot.status.recording || snapshot.status.recovering;
  const referenceKey = referenceKeys[reference];
  const showReference = recording && referenceKey !== undefined && (!alertsOnly || reference === "unavailable");
  const mismatch = snapshot.status.voiceprintContextMismatch === true;
  if (!showReference && !mismatch) {
    return null;
  }
  return (
    <div className="space-y-1 px-1 text-xs leading-5 text-[color:var(--muted)]" data-testid="voiceprint-runtime-hints">
      {showReference && referenceKey && <p role="status">{t(referenceKey)}</p>}
      {mismatch && <p role="alert">{t("voiceprintContextMismatch")}</p>}
    </div>
  );
}
