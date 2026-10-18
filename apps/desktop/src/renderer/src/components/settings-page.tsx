import { useMemo } from "react";
import type { AppSettings, DesktopSnapshot } from "@daybee/shared";
import { desktopActions, useDesktopSelector } from "@/lib/desktop-store";
import { createT } from "@/lib/i18n";
import {
  GeneralTab,
  InputTab,
  TranscribeTab,
  buildSettingsOptions
} from "@/components/settings-tab-panels";
export type WorkspacePage = "home" | "general" | "input" | "transcribe";

export function SettingsPage({ page, onInitializeVoiceprint }: {
  page: Exclude<WorkspacePage, "home">;
  onInitializeVoiceprint: () => void;
}) {
  const draft = useDesktopSelector((current) => current.settings);
  const app = useDesktopSelector((current) => current.app);
  const devices = useDesktopSelector((current) => current.devices);
  const updater = useDesktopSelector((current) => current.updater);
  const t = createT(draft.desktop.language);
  const options = buildSettingsOptions({ app, devices, updater, settings: draft }, t);

  const saveSettings = (nextSettings: AppSettings) => {
    void desktopActions.saveSettings(nextSettings);
  };

  const updateDesktop = (patch: Partial<AppSettings["desktop"]>) => {
    saveSettings({ ...draft, desktop: { ...draft.desktop, ...patch } });
  };

  const updateRecording = (patch: Partial<AppSettings["recording"]>) => {
    saveSettings({ ...draft, recording: { ...draft.recording, ...patch } });
  };

  return (
    <div>
      {page === "general" && <section aria-label={t("settingsTabGeneral")}>
        <GeneralTab
          options={options}
          settings={draft}
          t={t}
          updateDesktop={updateDesktop}
          updateRecording={updateRecording}
        />
      </section>}

      {page === "input" && <section aria-label={t("settingsTabInput")}>
        <InputTab options={options} settings={draft} t={t} updateRecording={updateRecording} />
      </section>}

      {page === "transcribe" && <section aria-label={t("settingsTabTranscribe")}>
        <TranscribePanel
          options={options}
          settings={draft}
          t={t}
          updateRecording={updateRecording}
          onInitializeVoiceprint={onInitializeVoiceprint}
        />
      </section>}
    </div>
  );
}

// Status and voiceprint move with the live recorder. Keep that subscription on
// the transcribe panel so the other pages stay mounted only for their own slices.
function TranscribePanel({ options, settings, t, updateRecording, onInitializeVoiceprint }: {
  options: ReturnType<typeof buildSettingsOptions>;
  settings: AppSettings;
  t: ReturnType<typeof createT>;
  updateRecording: (patch: Partial<AppSettings["recording"]>) => void;
  onInitializeVoiceprint: () => void;
}) {
  const status = useDesktopSelector((current) => current.status);
  const voiceprint = useDesktopSelector((current) => current.voiceprint);
  const asrModels = useDesktopSelector((current) => current.asrModels);
  const history = useDesktopSelector((current) => current.history);
  const snapshot = useMemo(() => ({ status, voiceprint, asrModels, history, settings }) as DesktopSnapshot, [
    status, voiceprint, asrModels, history, settings
  ]);
  return (
    <TranscribeTab
      options={options}
      settings={settings}
      snapshot={snapshot}
      t={t}
      updateRecording={updateRecording}
      onInitializeVoiceprint={onInitializeVoiceprint}
    />
  );
}
