import type { ChangeEvent } from "react";
import type {
  AppLanguage,
  AppSettings,
  AutoUpdateSnapshot,
  DesktopSnapshot,
  ThemeMode
} from "@daybee/shared";
import type { MessageKey } from "@/lib/i18n";
import { desktopActions } from "@/lib/desktop-store";
import { FolderOpen, Github, Star } from "lucide-react";
import { SettingField } from "@/components/setting-field";
import { Button } from "@/components/ui/legacy-button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { VoiceprintDiagnostics } from "@/components/voiceprint-diagnostics";
import { VoiceprintSettings } from "@/components/voiceprint-settings";
import { ASRModelSettings } from "@/components/asr-model-settings";
import { MCPSettings } from "@/components/mcp-settings";

type Translator = (key: MessageKey, params?: Record<string, number | string>) => string;
type SelectOption<T extends string | number> = { label: string; value: T };

type TabProps = {
  options: ReturnType<typeof buildSettingsOptions>;
  settings: AppSettings;
  t: Translator;
};

export function GeneralTab({
  options,
  settings,
  t,
  updateDesktop,
  updateRecording
}: TabProps & {
  updateDesktop: (patch: Partial<AppSettings["desktop"]>) => void;
  updateRecording: (patch: Partial<AppSettings["recording"]>) => void;
}) {
  return (
    <div className="space-y-2">
      <SettingField
        control={<SelectField options={options.languageOptions} value={settings.desktop.language} onChange={(value) => updateDesktop({ language: value as AppLanguage })} />}
        description={t("languageDescription")}
        title={t("languageTitle")}
      />
      <SettingField
        control={<SelectField options={options.themeOptions} value={settings.desktop.theme} onChange={(value) => updateDesktop({ theme: value as ThemeMode })} />}
        description={t("themeDescription")}
        title={t("themeTitle")}
      />
      <SettingField
        control={<Switch checked={settings.desktop.launchAtLogin} onCheckedChange={(value: boolean) => updateDesktop({ launchAtLogin: value })} />}
        description={t("launchAtLoginDescription")}
        testId="launch-at-login-field"
        title={t("launchAtLoginTitle")}
      />
      <SettingField
        control={
          <div className="flex min-w-0 max-w-full items-center gap-2">
            <div
              className="min-w-0 flex-1 break-all rounded-lg border border-[color:var(--border)] bg-[color:var(--surface)] px-3 py-2 text-xs leading-5 text-[color:var(--foreground)]"
              title={settings.recording.outputDir}
            >
              {settings.recording.outputDir}
            </div>
            <Button
              className="w-8 shrink-0 px-0"
              size="sm"
              title={t("browseOutputDir")}
              variant="subtle"
              onClick={async () => {
                const selected = await desktopActions.pickDirectory(settings.recording.outputDir);
                if (selected) {
                  updateRecording({ outputDir: selected });
                }
              }}
            >
              <FolderOpen className="h-3.5 w-3.5" />
            </Button>
          </div>
        }
        description={t("outputDirDescription")}
        layout="stacked"
        title={t("outputDirTitle")}
      />
      <SettingField
        control={<SelectNumberField options={options.segmentOptions} value={settings.recording.segmentMinutes} onChange={(value) => updateRecording({ segmentMinutes: value })} />}
        description={t("segmentMinutesDescription")}
        title={t("segmentMinutesTitle")}
      />
      <SettingField
        control={
          <div className="flex min-w-0 flex-col gap-2">
            <div className="rounded-lg border border-[color:var(--border)] bg-[color:var(--surface)] px-3 py-2 text-xs leading-5 text-[color:var(--foreground)]">
              <div className="font-medium">{options.appVersionLabel}</div>
              <div className="text-[color:var(--muted)]">{options.updateStatusLabel}</div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button size="sm" variant="subtle" onClick={() => void desktopActions.openExternal(options.repositoryUrl)}>
                <Star className="h-3.5 w-3.5" />
                {t("githubStarAction")}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => void desktopActions.openExternal(`${options.repositoryUrl}/releases`)}>
                <Github className="h-3.5 w-3.5" />
                {t("githubReleasesAction")}
              </Button>
            </div>
          </div>
        }
        description={t("appVersionDescription")}
        layout="stacked"
        title={t("appVersionTitle")}
      />
      <SettingField
        control={
          <Button size="sm" variant="subtle" onClick={() => void desktopActions.openExternal(options.repositoryUrl)}>
            <Star className="h-3.5 w-3.5" />
            {t("githubStarAction")}
          </Button>
        }
        description={t("githubStarDescription")}
        title={t("githubStarTitle")}
      />
      <MCPSettings t={t} />
    </div>
  );
}

export function InputTab({
  options,
  settings,
  t,
  updateRecording
}: TabProps & {
  updateRecording: (patch: Partial<AppSettings["recording"]>) => void;
}) {
  const supportsEchoCancellation = window.mygo?.platform === "darwin";
  return (
    <div className="space-y-2">
      <SettingField
        control={<SelectField options={options.deviceOptions} value={settings.recording.device} onChange={(value) => updateRecording({ device: value })} />}
        description={t("recordingDeviceDescription")}
        layout="stacked"
        title={t("recordingDeviceTitle")}
      />
      <SettingField
        control={<Switch checked={settings.recording.recordSystemAudio} disabled={!supportsEchoCancellation} onCheckedChange={(value: boolean) => updateRecording({ recordSystemAudio: value })} />}
        description={t(supportsEchoCancellation ? "recordSystemAudioDescription" : "recordSystemAudioUnavailable")}
        testId="system-audio-field"
        title={t("recordSystemAudioTitle")}
      />
      <SettingField
        control={<Switch checked={settings.recording.echoCancellation} disabled={!supportsEchoCancellation} onCheckedChange={(value: boolean) => updateRecording({ echoCancellation: value })} />}
        description={t(supportsEchoCancellation ? "echoCancellationDescription" : "echoCancellationUnavailable")}
        testId="echo-cancellation-field"
        title={t("echoCancellationTitle")}
      />
      <SettingField
        control={<Switch checked={settings.recording.autoSwitchDevice} onCheckedChange={(value: boolean) => updateRecording({ autoSwitchDevice: value })} />}
        description={t(settings.recording.echoCancellation ? "autoSwitchEchoCancellation" : "autoSwitchDescription")}
        testId="auto-switch-input-field"
        title={t("autoSwitchTitle")}
      />
      <SettingField
        control={<Input value={settings.recording.excludeDeviceKeywords} onChange={(event: ChangeEvent<HTMLInputElement>) => updateRecording({ excludeDeviceKeywords: event.currentTarget.value })} />}
        description={t("excludeDeviceKeywordsDescription")}
        layout="stacked"
        title={t("excludeDeviceKeywordsTitle")}
      />
    </div>
  );
}

export function TranscribeTab({
  options,
  settings,
  snapshot,
  t,
  updateRecording,
  onInitializeVoiceprint
}: TabProps & {
  snapshot: DesktopSnapshot;
  updateRecording: (patch: Partial<AppSettings["recording"]>) => void;
  onInitializeVoiceprint: () => void;
}) {
  return (
    <div className="space-y-2">
      <VoiceprintSettings
        snapshot={snapshot}
        t={t}
        onAdaptation={(enabled) => updateRecording({ voiceprintAdaptation: enabled })}
        onInitialize={onInitializeVoiceprint}
      />
      <SettingField control={<Switch checked={!settings.recording.disableAsr} onCheckedChange={(value: boolean) => updateRecording({ disableAsr: !value })} />} description={t("realtimeAsrDescription")} testId="realtime-asr-field" title={t("realtimeAsrTitle")} />
      <ASRModelSettings snapshot={snapshot} t={t} />
      <SettingField control={<SelectField options={options.asrLanguageOptions} value={settings.recording.asrLanguage} onChange={(value) => updateRecording({ asrLanguage: value })} />} description={t(settings.recording.asrModel === "speech-analyzer" ? "asrLanguageSpeechAnalyzerDescription" : "asrLanguageDescription")} title={t("asrLanguageTitle")} />
      <SettingField
        control={
          <div className="rounded-lg border border-[color:var(--border)] bg-[color:var(--surface)] px-3 py-2 text-xs leading-5 text-[color:var(--foreground)]">
            {snapshot.status.senseVoiceReady && snapshot.status.vadReady
              ? t("modelReady")
              : snapshot.status.downloading
                ? snapshot.status.downloadMessage || t("modelDownloadPreparing")
                : t("modelMissing")}
          </div>
        }
        description={t("modelStatusDescription")}
        layout="stacked"
        title={t("modelStatusTitle")}
      />
      <VoiceprintDiagnostics snapshot={snapshot} t={t} />
    </div>
  );
}

export function buildSettingsOptions(snapshot: Pick<DesktopSnapshot, "app" | "devices" | "updater" | "settings">, t: Translator) {
  return {
    appVersionLabel: `${snapshot.app.name} v${snapshot.app.version}`,
    languageOptions: [
      { label: t("languageSystem"), value: "system" },
      { label: t("languageZhCn"), value: "zh-CN" },
      { label: t("languageEnUs"), value: "en-US" }
    ] satisfies SelectOption<AppLanguage>[],
    themeOptions: [
      { label: t("themeSystem"), value: "system" },
      { label: t("themeLight"), value: "light" },
      { label: t("themeDark"), value: "dark" }
    ] satisfies SelectOption<ThemeMode>[],
    deviceOptions: [
      { label: t("recordingDeviceDefault"), value: "default" },
      ...snapshot.devices.map((device) => ({
        label: device.isDefault ? `${device.label} (${t("recordingDeviceDefault")})` : device.label,
        value: device.id
      }))
    ] satisfies SelectOption<string>[],
    asrLanguageOptions: [
      { label: t(snapshot.settings.recording.asrModel === "speech-analyzer" ? "asrLanguageSystem" : "asrLanguageAuto"), value: "auto" },
      { label: t("languageNameChinese"), value: "zh" },
      { label: t("languageNameEnglish"), value: "en" },
      { label: t("languageNameJapanese"), value: "ja" },
      { label: t("languageNameKorean"), value: "ko" }
    ] satisfies SelectOption<string>[],
    repositoryUrl: snapshot.app.repositoryUrl,
    segmentOptions: numberOptions([15, 30, 45, 60, 90, 120]),
    updateStatusLabel: formatUpdateStatus(snapshot.updater, t)
  };
}

function formatUpdateStatus(updater: AutoUpdateSnapshot, t: Translator): string {
  if (updater.phase === "available") {
    return t("updateStatusAvailable", { version: updater.latestVersion ?? updater.currentVersion });
  }
  if (updater.phase === "checking") {
    return t("updateStatusChecking");
  }
  if (updater.phase === "downloading") {
    return t("updateStatusDownloading", {
      version: updater.latestVersion ?? updater.currentVersion
    });
  }
  if (updater.phase === "downloaded") {
    const version = updater.downloadedVersion ?? updater.latestVersion ?? updater.currentVersion;
    return t("updateStatusRestarting", { version });
  }
  if (updater.phase === "error") {
    return t("updateStatusError", {
      message: updater.errorMessage ?? updater.statusMessage
    });
  }
  if (updater.phase === "unavailable") {
    return t("updateStatusUnavailable");
  }
  return t("updateStatusIdle");
}

function SelectField<T extends string>({
  onChange,
  options,
  value
}: {
  onChange: (value: T) => void;
  options: SelectOption<T>[];
  value: T;
}) {
  return (
    <Select value={value} onValueChange={(nextValue) => onChange(nextValue as T)}>
      <SelectTrigger>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function SelectNumberField({
  onChange,
  options,
  value
}: {
  onChange: (value: number) => void;
  options: SelectOption<number>[];
  value: number;
}) {
  return (
    <Select value={String(value)} onValueChange={(nextValue) => onChange(Number(nextValue))}>
      <SelectTrigger>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={String(option.value)}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function numberOptions(values: number[]): SelectOption<number>[] {
  return values.map((value) => ({ label: String(value), value }));
}
