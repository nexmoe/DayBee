import { Pin } from "lucide-react";
import { memo } from "react";
import { Button } from "@/components/ui/legacy-button";
import { LiveWaveform } from "@/components/ui/live-waveform";
import { UpdateButton } from "@/components/update-button";
import { statusTone } from "@/lib/status-tone";
import { createT } from "@/lib/i18n";
import { desktopActions, useDesktopSelector } from "@/lib/desktop-store";

export const RecorderHeader = memo(function RecorderHeader() {
  const language = useDesktopSelector((current) => current.settings.desktop.language);
  const disableAsr = useDesktopSelector((current) => current.settings.recording.disableAsr);
  const status = useDesktopSelector((current) => current.status);
  const updater = useDesktopSelector((current) => current.updater);
  const windowPinned = useDesktopSelector((current) => current.windowPinned);
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
          <UpdateButton updater={updater} onInstall={desktopActions.installUpdate} t={t} />
          <Button
            aria-pressed={windowPinned}
            className="w-8 px-0"
            size="sm"
            title={windowPinned ? t("unpinWindow") : t("pinWindow")}
            variant={windowPinned ? "subtle" : "ghost"}
            onClick={() => desktopActions.setWindowPinned(!windowPinned)}
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
            onClick={canStop ? desktopActions.stopRecording : desktopActions.startRecording}
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
    </div>
  );
});
