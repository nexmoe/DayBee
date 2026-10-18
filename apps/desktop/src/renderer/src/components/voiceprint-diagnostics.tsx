import type { DesktopSnapshot } from "@daybee/shared";
import type { MessageKey } from "@/lib/i18n";
import { VoiceprintSampleExport } from "@/components/voiceprint-sample-export";

type Translator = (key: MessageKey, params?: Record<string, number | string>) => string;

export function VoiceprintDiagnostics({ snapshot, t }: { snapshot: DesktopSnapshot; t: Translator }) {
  return (
    <details
      className="rounded-2xl border border-[color:var(--border)] bg-[color:var(--surface)] px-4 py-3.5 shadow-[var(--shadow-raised-sm)]"
      data-testid="voiceprint-diagnostics"
    >
      <summary className="cursor-pointer text-sm font-medium text-[color:var(--foreground)]" data-testid="voiceprint-diagnostics-toggle">
        {t("voiceprintDiagnosticsTitle")}
      </summary>
      <p className="mt-1 text-xs leading-5 text-[color:var(--muted)]">{t("voiceprintDiagnosticsDescription")}</p>
      <div className="mt-3 min-w-0">
        <VoiceprintSampleExport snapshot={snapshot} t={t} />
      </div>
    </details>
  );
}
