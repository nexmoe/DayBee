import { Minus, X } from "lucide-react";
import { Button } from "@/components/ui/legacy-button";
import { desktopActions, useDesktopSelector } from "@/lib/desktop-store";
import { createT } from "@/lib/i18n";

const isWindows = navigator.userAgent.toLowerCase().includes("windows");

export function WindowsWindowControls() {
  const language = useDesktopSelector((current) => current.settings.desktop.language);
  const t = createT(language);

  if (!isWindows) {
    return null;
  }

  return (
    <div className="windows-window-controls" aria-label={t("windowControlsLabel")}>
      <Button
        aria-label={t("minimizeWindow")}
        className="windows-window-control"
        size="sm"
        title={t("minimizeWindow")}
        variant="ghost"
        onClick={() => void desktopActions.minimizeWindow()}
      >
        <Minus className="h-3.5 w-3.5" />
      </Button>
      <Button
        aria-label={t("closeWindow")}
        className="windows-window-control windows-window-control--danger"
        size="sm"
        title={t("closeWindow")}
        variant="ghost"
        onClick={() => void desktopActions.closeWindow()}
      >
        <X className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}
