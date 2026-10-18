import { memo, useCallback, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import type { AppLanguage } from "@daybee/shared";
import { SettingsPage, type WorkspacePage } from "@/components/settings-page";
import { StatusOverview } from "@/components/status-overview";
import { RecorderHeader } from "@/components/recorder-header";
import { SpeechTimeline } from "@/components/speech-timeline";
import { VoiceprintGuide } from "@/components/voiceprint-guide";
import { WindowsWindowControls } from "@/components/windows-window-controls";
import { ToastViewport } from "@/components/ui/toast";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { ScrollArea } from "@/components/ui/legacy-scroll-area";
import { createT } from "@/lib/i18n";
import { desktopActions, useDesktopSelector } from "@/lib/desktop-store";

const destinations = [
  { id: "home", label: "workspaceHome" },
  { id: "general", label: "settingsTabGeneral" },
  { id: "input", label: "settingsTabInput" },
  { id: "transcribe", label: "settingsTabTranscribe" }
] as const;

export function CompactWindow() {
  const [page, setPage] = useState<WorkspacePage>("home");
  const language = useDesktopSelector(current => current.settings.desktop.language);
  const phase = useDesktopSelector(current => current.voiceprint.phase);
  const pending = useDesktopSelector(current => current.settings.desktop.voiceprintSetup === "pending");
  const showGuide = page === "home" || (phase !== "idle" && phase !== "ready") || pending;
  const t = createT(language);
  const contentRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    contentRef.current?.querySelector("[data-radix-scroll-area-viewport]")?.scrollTo(0, 0);
  }, [page]);

  return (
    <main className="compact-window" data-window="compact">
      <div className="drag-region" />
      <ToastViewport />
      <WindowsWindowControls />
      <Tabs value={page} onValueChange={value => setPage(value as WorkspacePage)} className="compact-pages">
        <header className="compact-navigation">
          <div className="compact-recorder">
            <RecorderHeader />
            <TabsList className="compact-tabs" aria-label={t("workspaceNavigation")}>
              {destinations.map(item => <TabsTrigger key={item.id} value={item.id}>{t(item.label)}</TabsTrigger>)}
            </TabsList>
          </div>
        </header>
        <div className="compact-content" ref={contentRef}>
          <ScrollArea className="h-full min-h-0">
            <TabsContent key={page} value={page} className="compact-page">
              {page === "home" ? <CompactHome language={language} /> : <>
                {showGuide && <VoiceprintGuide />}
                <SettingsPage page={page} onInitializeVoiceprint={() => {
                  setPage("home");
                  void desktopActions.startVoiceprintEnrollment();
                }} />
              </>}
            </TabsContent>
          </ScrollArea>
        </div>
      </Tabs>
    </main>
  );
}

// Recording ticks stay inside the recorder panels; the timeline only subscribes to
// history and activity, so switching tabs never restarts recording.
const CompactHome = memo(function CompactHome({ language }: { language: AppLanguage }) {
  const history = useDesktopSelector(current => current.history);
  const microphoneActivity = useDesktopSelector(current => current.microphoneActivity);
  const t = createT(language);
  const panels = useCallback(({ activity, timeline }: { activity: ReactNode; timeline: ReactNode }) => <>
    <StatusOverview
      onOpenPrivacy={desktopActions.openMicrophoneSettings}
      onRequestPermission={desktopActions.requestPermission}
      guide={<VoiceprintGuide />}
    >{activity}</StatusOverview>
    <section className="mt-4" aria-label={t("timelineLabel")}>{timeline}</section>
  </>, [t]);
  return <section aria-label={t("workspaceHome")}>
    <SpeechTimeline items={history} microphoneActivity={microphoneActivity} language={language}>
      {panels}
    </SpeechTimeline>
  </section>;
});
