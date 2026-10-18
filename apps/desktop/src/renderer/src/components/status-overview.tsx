import { ShieldCheck } from "lucide-react";
import { memo, type ReactNode } from "react";
import { Button } from "@/components/ui/legacy-button";
import type { DesktopSnapshot } from "@daybee/shared";
import { createT } from "@/lib/i18n";
import { useDesktopSelector } from "@/lib/desktop-store";
import { VoiceprintRuntimeHints } from "@/components/voiceprint-runtime-hints";

export const StatusOverview = memo(function StatusOverview({
  onOpenPrivacy,
  onRequestPermission,
  children,
  guide
}: {
  onOpenPrivacy: () => Promise<boolean>;
  onRequestPermission: () => Promise<unknown>;
  children?: ReactNode;
  guide?: ReactNode;
}) {
  const language = useDesktopSelector((current) => current.settings.desktop.language);
  const disableAsr = useDesktopSelector((current) => current.settings.recording.disableAsr);
  const status = useDesktopSelector((current) => current.status);
  const permission = useDesktopSelector((current) => current.permission);
  const voiceprint = useDesktopSelector((current) => current.voiceprint);
  const hints = { settings: { recording: { disableAsr } }, status, voiceprint } as DesktopSnapshot;
  const t = createT(language);

  return (
    <div className="space-y-4">
      {status.downloading && (
        <div className="rounded-xl border border-[color:var(--border)] bg-[color:var(--surface)] shadow-[var(--shadow-raised-sm)] px-3.5 py-3">
          <p className="text-xs font-semibold tracking-tight text-[color:var(--foreground)]">
            {t("modelDownloadTitle")}
          </p>
          <p className="mt-1 text-[11px] leading-5 text-[color:var(--muted)]">
            {status.downloadMessage || t("modelDownloadPreparing")}
          </p>
          {status.downloadProgress !== null && (
            <p className="mt-1 text-[11px] leading-5 text-[color:var(--foreground)]">
              {t("modelDownloadProgress", { percent: status.downloadProgress })}
            </p>
          )}
        </div>
      )}

      {guide}
      <div className="capture-card">{children}</div>

      <VoiceprintRuntimeHints snapshot={hints} t={t} />

      {/* Permission banner (only when needed) */}
      {permission.state !== "authorized" && (
        <div className="flex items-center gap-2.5 rounded-xl border border-[color:var(--border)] bg-[color:var(--surface)] shadow-[var(--shadow-raised-sm)] px-3.5 py-3">
          <ShieldCheck className="h-4 w-4 shrink-0 text-[color:var(--accent)]" />
          <span className="flex-1 text-xs leading-5 text-[color:var(--muted)]">
            {permissionMessage(permission.state, t)}
          </span>
          {permission.state === "not-determined" ? (
            <Button size="sm" variant="ghost" onClick={onRequestPermission}>
              {t("permissionRequestAction")}
            </Button>
          ) : (
            <Button size="sm" variant="ghost" onClick={onOpenPrivacy}>
              {t("permissionOpenSettingsAction")}
            </Button>
          )}
        </div>
      )}
    </div>
  );
});

function permissionMessage(
  state: DesktopSnapshot["permission"]["state"],
  t: ReturnType<typeof createT>
): string {
  if (state === "authorized") {
    return t("permissionAuthorized");
  }
  if (state === "denied") {
    return t("permissionDenied");
  }
  if (state === "restricted") {
    return t("permissionRestricted");
  }
  if (state === "not-determined") {
    return t("permissionNotDetermined");
  }
  return t("permissionUnsupported");
}
