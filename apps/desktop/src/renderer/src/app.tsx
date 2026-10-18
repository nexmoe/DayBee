import { useDesktopSelector, desktopActions } from "@/lib/desktop-store";
import { ScrollArea } from "@/components/ui/legacy-scroll-area";
import { StatusOverview } from "@/components/status-overview";
import { SpeechTimeline } from "@/components/speech-timeline";
import { lazy, memo, Suspense, useCallback, useEffect, type ReactNode } from "react";
import { ToastViewport } from "@/components/ui/toast";
import { createT } from "@/lib/i18n";
import type { AppLanguage } from "@daybee/shared";
import { windowRole } from "@/lib/window-role";
import { useWindowTheme } from "@/lib/use-window-theme";
import { WindowsWindowControls } from "@/components/windows-window-controls";

const MainWindow = lazy(() => import("@/components/main-window").then(module => ({ default: module.MainWindow })));

export function App() {
  const language = useDesktopSelector((current) => current.settings.desktop.language);
  const theme = useDesktopSelector((current) => current.settings.desktop.theme);
  const appName = useDesktopSelector((current) => current.app.name);
  const t = createT(language);
  useWindowTheme(theme);
  useEffect(() => {
    document.title = windowRole === "main" ? `${appName} — ${t("workspaceLabel")}` : appName;
  }, [appName, language, t]);

  if (windowRole === "main") {
    return <Suspense fallback={<main className="h-screen content-center p-8" aria-busy="true" role="status">{t("interfaceLoading")}</main>}><MainWindow /></Suspense>;
  }

  return (
    <main className="h-screen text-[color:var(--foreground)]" data-window="compact">
      {/* Draggable title bar region for frameless window */}
      <div className="drag-region" />
      <ToastViewport />
      <WindowsWindowControls />

      <ScrollArea className="h-screen w-full">
        <div className="tray-panel min-w-0 max-w-full overflow-x-hidden">
          <CompactPanels language={language} />
        </div>
      </ScrollArea>
    </main>
  );
}

// History and activity stay here. Status ticks subscribe inside StatusOverview,
// and the render prop stays stable so SpeechTimeline's memo can skip them.
const CompactPanels = memo(function CompactPanels({ language }: { language: AppLanguage }) {
  const t = createT(language);
  const history = useDesktopSelector((current) => current.history);
  const microphoneActivity = useDesktopSelector((current) => current.microphoneActivity);
  const panels = useCallback(({ activity, timeline }: { activity: ReactNode; timeline: ReactNode }) => <>
    <div className="panel-section">
      <StatusOverview
        onInstallUpdate={desktopActions.installUpdate}
        onOpenMainWindow={desktopActions.openMainWindow}
        onTogglePinned={desktopActions.setWindowPinned}
        onOpenPrivacy={desktopActions.openMicrophoneSettings}
        onRequestPermission={desktopActions.requestPermission}
        onStart={desktopActions.startRecording}
        onStop={desktopActions.stopRecording}
      >{activity}</StatusOverview>
    </div>
    <section className="panel-section" aria-label={t("timelineLabel")}>
      {timeline}
    </section>
  </>, [t]);

  return (
    <SpeechTimeline
      items={history}
      microphoneActivity={microphoneActivity}
      language={language}
      showRecorderShortcut={false}
    >{panels}</SpeechTimeline>
  );
});
