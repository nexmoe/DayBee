import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { ArrowUpRight, AudioLines, History, House, Mic, SlidersHorizontal, Speech } from "lucide-react";
import { SettingsPage, type WorkspacePage } from "@/components/settings-page";
import { VoiceprintGuide } from "@/components/voiceprint-guide";
import { WindowsWindowControls } from "@/components/windows-window-controls";
import { ToastViewport } from "@/components/ui/toast";
import { TranscriptSearch } from "@/components/transcript-search";
import { RecordingContextFilter } from "@/components/recording-context";
import { availableRecordingContexts } from "@/lib/recording-context";
import {
  SidebarProvider, Sidebar, SidebarHeader, SidebarContent, SidebarFooter,
  SidebarInset, SidebarTrigger, SidebarGroup, SidebarGroupLabel,
  SidebarMenu, SidebarMenuItem, SidebarMenuButton, useSidebar
} from "@/components/ui/sidebar";
import { Button } from "@/components/ui/legacy-button";
import { ScrollArea } from "@/components/ui/legacy-scroll-area";
import { createT } from "@/lib/i18n";
import { desktopActions, useDesktopSelector } from "@/lib/desktop-store";
import { typeClass } from "@/lib/size-context";
import { fontWeights } from "@/lib/font-weight";
import appIcon from "../../../../resources/icon.png";
import "@fontsource-variable/inter/opsz.css";

const destinationGroups = [
  {
    label: "workspaceLabel",
    items: [
      { id: "home", label: "workspaceHome", icon: House },
      { id: "history", label: "settingsTabHistory", icon: History }
    ]
  },
  {
    label: "settingsLabel",
    items: [
      { id: "general", label: "settingsTabGeneral", icon: SlidersHorizontal },
      { id: "input", label: "settingsTabInput", icon: Mic },
      { id: "transcribe", label: "settingsTabTranscribe", icon: Speech }
    ]
  }
] as const;
type Destination = (typeof destinationGroups)[number]["items"][number];
const destinations = destinationGroups.flatMap<Destination>(group => group.items);

// Keep one keyboard sequence across the two independently highlighted groups.
function handleNavigationKeyDown(event: KeyboardEvent<HTMLElement>) {
  if (!["ArrowDown", "ArrowUp", "ArrowRight", "ArrowLeft", "Home", "End"].includes(event.key)) return;
  const buttons = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('[data-sidebar="menu-button"]'));
  const index = buttons.indexOf(event.target as HTMLButtonElement);
  if (index === -1) return;
  event.preventDefault();
  event.stopPropagation();
  const nextIndex = event.key === "Home" ? 0
    : event.key === "End" ? buttons.length - 1
    : (index + (["ArrowDown", "ArrowRight"].includes(event.key) ? 1 : -1) + buttons.length) % buttons.length;
  buttons[nextIndex]?.focus();
}

export function MainWindow() {
  const [page, setPage] = useState<WorkspacePage>("home");
  const phase = useDesktopSelector((current) => current.voiceprint.phase);
  const pending = useDesktopSelector((current) => current.settings.desktop.voiceprintSetup === "pending");
  const enrolling = phase !== "idle" && phase !== "ready";
  const showGuide = page === "home" || enrolling || (pending && phase !== "ready");
  return (
    <div className="workspace-window" data-window="main">
      <div className="drag-region" />
      <ToastViewport />
      <WindowsWindowControls />
      <SidebarProvider defaultOpen persist={false} width="14rem" peek="hover" className="h-screen min-h-0">
        <WorkspaceSidebar page={page} onNavigate={setPage} />
        <WorkspaceContent page={page} showGuide={showGuide}
          onInitializeVoiceprint={() => setPage("home")} />
      </SidebarProvider>
    </div>
  );
}

function WorkspaceSidebar({ page, onNavigate }: {
  page: WorkspacePage; onNavigate: (page: WorkspacePage) => void;
}) {
  const language = useDesktopSelector((current) => current.settings.desktop.language);
  const appName = useDesktopSelector((current) => current.app.name);
  const t = createT(language);
  const { isMobile, setOpenMobile } = useSidebar();
  const navigationId = useId();
  return (
    <Sidebar variant="sidebar" className="workspace-navigation">
      <SidebarHeader className="workspace-brand">
        <img src={appIcon} className="workspace-logo" alt="" width={32} height={32} />
        <p className={typeClass("subtitle")} style={{ fontVariationSettings: fontWeights.semibold }}>{appName}</p>
      </SidebarHeader>
      <SidebarContent>
        <nav className="workspace-navigation-groups" aria-label={t("workspaceNavigation")} onKeyDownCapture={handleNavigationKeyDown}>
          {destinationGroups.map(group => (
            <SidebarGroup key={group.label} className="gap-1 p-0" role="group" aria-labelledby={`${navigationId}-${group.label}`}>
              <SidebarGroupLabel id={`${navigationId}-${group.label}`} className="h-6">{t(group.label)}</SidebarGroupLabel>
              <SidebarMenu className="gap-1">
                {group.items.map(item => (
                  <SidebarMenuItem key={item.id}>
                    <SidebarMenuButton
                      icon={item.icon} isActive={page === item.id} aria-label={t(item.label)}
                      aria-current={page === item.id ? "page" : undefined}
                      onClick={() => {
                        onNavigate(item.id);
                        if (isMobile) setOpenMobile(false);
                      }}
                    >{t(item.label)}</SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroup>
          ))}
        </nav>
      </SidebarContent>
      <SidebarFooter className="workspace-footer">
        <SidebarRecordingStatus language={language} />
        <Button variant="subtle" size="sm" className="w-full justify-between" onClick={() => void desktopActions.openRecorderWindow()}>
          <span className="flex items-center gap-2"><AudioLines size={14} />{t("openRecorderWindow")}</span>
          <ArrowUpRight size={14} />
        </Button>
      </SidebarFooter>
    </Sidebar>
  );
}

function SidebarRecordingStatus({ language }: { language: Parameters<typeof createT>[0] }) {
  const recording = useDesktopSelector((current) => current.status.recording);
  const recovering = useDesktopSelector((current) => current.status.recovering);
  const elapsed = useDesktopSelector((current) => current.status.elapsed);
  const t = createT(language);
  return (
    <div className={`workspace-status ${typeClass("caption")}`}>
      <span className="status-dot" data-tone={recording ? "recording" : "idle"} />
      <span>{recovering ? t("statusRecovering") : recording ? t("statusRecording", { elapsed }) : t("statusIdle")}</span>
    </div>
  );
}

function WorkspaceContent({ page, showGuide, onInitializeVoiceprint }: {
  page: WorkspacePage; showGuide: boolean; onInitializeVoiceprint: () => void;
}) {
  const [query, setQuery] = useState("");
  const [selectedApp, setAppFilter] = useState("");
  const history = useDesktopSelector((current) => current.history);
  const microphoneItems = useDesktopSelector((current) => current.microphoneActivity.items);
  const language = useDesktopSelector((current) => current.settings.desktop.language);
  const recordingApps = useMemo(() => availableRecordingContexts(microphoneItems, history,
    page === "home" ? "transcripts" : "recordings"), [microphoneItems, history, page]);
  const appFilter = recordingApps.some(app => app.appId === selectedApp) ? selectedApp : "";
  useEffect(() => {
    if (selectedApp && !appFilter && (page === "home" || page === "history")) setAppFilter("");
  }, [selectedApp, appFilter, page]);
  const t = createT(language);
  const destination = destinations.find(item => item.id === page)!;
  const contentRef = useRef<HTMLElement>(null);
  useLayoutEffect(() => {
    contentRef.current?.querySelector("[data-radix-scroll-area-viewport]")?.scrollTo(0, 0);
  }, [page, query, appFilter]);
  return (
    <SidebarInset ref={contentRef} className="workspace-inset">
      <header className="workspace-toolbar">
        <SidebarTrigger aria-label={t("toggleSidebar")} title={t("toggleSidebar")} />
        <h1 className={typeClass("subtitle")} style={{ fontVariationSettings: fontWeights.semibold }}>{t(destination.label)}</h1>
        {(page === "home" || page === "history") && <div className="workspace-filters">
          <RecordingContextFilter apps={recordingApps} value={appFilter} onChange={setAppFilter} language={language} />
          {page === "home" && <TranscriptSearch query={query} onChange={setQuery} language={language} />}
        </div>}
      </header>
      <ScrollArea className="min-h-0 flex-1">
        <div className="workspace-page">
          {showGuide && <VoiceprintGuide />}
          <SettingsPage page={page} transcriptQuery={query} appFilter={appFilter} onSelectApp={setAppFilter} onInitializeVoiceprint={onInitializeVoiceprint} />
        </div>
      </ScrollArea>
    </SidebarInset>
  );
}
