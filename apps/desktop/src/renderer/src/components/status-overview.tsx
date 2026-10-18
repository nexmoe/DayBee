import { AppWindow, Pin, ShieldCheck } from "lucide-react";
import { memo, type ReactNode } from "react";
import { Button } from "@/components/ui/legacy-button";
import { LiveWaveform } from "@/components/ui/live-waveform";
import type { DesktopSnapshot } from "@daybee/shared";
import { statusTone } from "@/lib/status-tone";
import { createT } from "@/lib/i18n";
import { useDesktopSelector } from "@/lib/desktop-store";
import { UpdateButton } from "@/components/update-button";
import { VoiceprintRuntimeHints } from "@/components/voiceprint-runtime-hints";

export const StatusOverview = memo(function StatusOverview({
  onInstallUpdate,
  onOpenMainWindow,
  onTogglePinned,
  onOpenPrivacy,
  onRequestPermission,
  onStart,
  onStop,
  children
}: {
  onInstallUpdate: () => Promise<void>;
  onOpenMainWindow: () => Promise<void>;
  onTogglePinned: (pinned: boolean) => Promise<void>;
  onOpenPrivacy: () => Promise<boolean>;
  onRequestPermission: () => Promise<unknown>;
  onStart: () => Promise<void>;
  onStop: () => Promise<void>;
  children?: ReactNode;
}) {
  const language = useDesktopSelector((current) => current.settings.desktop.language);
  const disableAsr = useDesktopSelector((current) => current.settings.recording.disableAsr);
  const status = useDesktopSelector((current) => current.status);
  const updater = useDesktopSelector((current) => current.updater);
  const windowPinned = useDesktopSelector((current) => current.windowPinned);
  const permission = useDesktopSelector((current) => current.permission);
  const voiceprint = useDesktopSelector((current) => current.voiceprint);
  const hints = { settings: { recording: { disableAsr } }, status, voiceprint } as DesktopSnapshot;
  const t = createT(language);
  const tone = statusTone(status);
  const recording = status.recording;
  const recovering = status.recovering;
  const canStop = recording || recovering;
  const preparingModels = status.downloading && (!status.vadReady ||
    (!disableAsr && !status.senseVoiceReady));

  return (
    <div className="space-y-4">
      {/* Header: status + action */}
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2.5">
          <span className="truncate text-sm font-semibold tracking-tight text-[color:var(--foreground)]">
            {recovering ? t("statusRecovering") : recording
              ? t("statusRecording", { elapsed: status.elapsed })
              : t("statusIdle")}
          </span>
          <div className="status-dot shrink-0" data-tone={tone} />
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <UpdateButton updater={updater} onInstall={onInstallUpdate} t={t} />
          <Button
            aria-label={t("openMainWindow")}
            className="open-workspace w-8 px-0"
            size="sm"
            title={t("openMainWindow")}
            variant="ghost"
            onClick={() => void onOpenMainWindow()}
          >
            <AppWindow aria-hidden="true" className="h-3.5 w-3.5" />
          </Button>
          <Button
            aria-pressed={windowPinned}
            className="w-8 px-0"
            size="sm"
            title={windowPinned ? t("unpinWindow") : t("pinWindow")}
            variant={windowPinned ? "subtle" : "ghost"}
            onClick={() => onTogglePinned(!windowPinned)}
          >
            <Pin className="h-3.5 w-3.5" />
          </Button>
          <Button
            size="sm"
            disabled={updater.phase === "downloading" || updater.phase === "downloaded"}
            title={
              !recording && preparingModels
                ? status.downloadMessage || t("modelDownloadPreparing")
                : undefined
            }
            onClick={canStop ? onStop : onStart}
          >
            {canStop ? t("stopRecording") : t("startRecording")}
          </Button>
        </div>
      </div>

      {/* Waveform — mirrors Viora RecordingWaveform usage */}
      <LiveWaveform
        active={recording}
        processing={recovering || (!recording && preparingModels)}
        mode="static"
        centerStaticBars
        barWidth={2.5}
        barGap={1.5}
        barRadius={1.5}
        fadeEdges
        fadeWidth={20}
        height={72}
        className="w-full"
      />

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
