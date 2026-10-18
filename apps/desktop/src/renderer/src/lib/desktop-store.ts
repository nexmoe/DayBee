import { desktopApi, desktopEvents } from "@/lib/desktop-api";
import { windowRole } from "@/lib/window-role";
import {
  DEFAULT_SETTINGS,
  DEFAULT_STATUS,
  type AppSettings,
  type ASRModel,
  type DesktopSnapshot,
  type MicrophonePermissionStatus,
  type RecorderStatusSnapshot
} from "@daybee/shared";
import { useRef, useSyncExternalStore } from "react";
import { createT } from "@/lib/i18n";
import { toastActions } from "@/lib/toast-store";
import { voiceprintErrorCopy, voiceprintExportErrorCopy } from "@/lib/voiceprint-copy";

const defaultPermission = (): MicrophonePermissionStatus => ({
  message: createT(DEFAULT_SETTINGS.desktop.language)("permissionPending"),
  state: "not-determined",
  supported: true
});

let snapshot: DesktopSnapshot = {
  asrModels: { items: [], busy: false, downloadingModel: null },
  app: {
    name: "DayBee",
    repositoryUrl: "https://github.com/nexmoe/DayBee",
    version: "0.0.0"
  },
  devices: [],
  engineReady: false,
  history: [],
  microphoneActivity: { supported: true, error: null, items: [] },
  permission: defaultPermission(),
  settings: DEFAULT_SETTINGS,
  status: DEFAULT_STATUS,
  updater: {
    currentVersion: "0.0.0",
    downloadedVersion: null,
    downloadedVersionReady: false,
    errorMessage: null,
    installDeferredUntilIdle: false,
    latestVersion: null,
    phase: "idle",
    statusMessage: "Automatic updates are ready."
  },
  windowPinned: false,
  voiceprint: {
    enrolled: false,
    phase: "idle",
    step: 0,
    speechSeconds: 0,
    targetSeconds: 8,
    level: 0,
    progress: null,
    message: "",
    error: null
  }
};

const listeners = new Set<() => void>();

const subscribe = (listener: () => void): (() => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};
const bridge = desktopApi;
let saveSequence = 0;
let lastStatusError: string | null = null;
let lastVoiceprintError: string | null = null;
let pendingSettings: AppSettings | null = null;

const toastVoiceprint = (message: string): void => {
  const text = message.trim();
  if (!text || text === lastVoiceprintError) return;
  lastVoiceprintError = text;
  const t = createT(snapshot.settings.desktop.language);
  toastActions.show({
    message: voiceprintErrorCopy(text, t, snapshot.voiceprint.targetSeconds).message,
    title: t("errorVoiceprintFailed")
  });
};

const publish = (): void => {
  for (const listener of listeners) {
    listener();
  }
};

const updateSnapshot = (nextSnapshot: DesktopSnapshot): void => {
  const nextError = nextSnapshot.status.error?.trim() || null;
  const previousError = snapshot.status.error?.trim() || null;
  snapshot = nextSnapshot;
  if (nextError && nextError !== previousError && nextError !== lastStatusError) {
    lastStatusError = nextError;
    toastActions.show({
      message: nextError,
      title: createT(nextSnapshot.settings.desktop.language)("errorStartRecordingFailed")
    });
  }
  if (!nextError) {
    lastStatusError = null;
  }
  const voiceError = nextSnapshot.voiceprint.error?.trim() || null;
  // Enrollment errors already have an actionable message in the main guide.
  if (voiceError && windowRole === "main" && nextSnapshot.voiceprint.phase === "error") lastVoiceprintError = voiceError;
  else if (voiceError) toastVoiceprint(voiceError);
  else lastVoiceprintError = null;
  publish();
};

if (!bridge) {
  console.error("[daybee-renderer] native desktop bridge is unavailable.");
  updateSnapshot({
    ...snapshot,
    permission: {
      message: createT(snapshot.settings.desktop.language)("errorBridgeMissing"),
      state: "denied",
      supported: true
    }
  });
} else {
  desktopEvents.onSnapshot(updateSnapshot);
  desktopEvents.onStatus(({ status, voiceprint }) => updateSnapshot({ ...snapshot, status, voiceprint }));
  void bridge.bootstrap().then(updateSnapshot);
}

export const useDesktopSnapshot = (): DesktopSnapshot => {
  return useSyncExternalStore(subscribe, () => snapshot);
};

// `use-sync-external-store/with-selector` is not a dependency. Cache the last
// selected value so a 10 Hz status publish does not re-render a subscriber
// whose slice is still equal.
export const useDesktopSelector = <T>(
  selector: (current: DesktopSnapshot) => T,
  isEqual: (left: T, right: T) => boolean = Object.is
): T => {
  const selectorRef = useRef(selector);
  const isEqualRef = useRef(isEqual);
  const selectedRef = useRef<T>(undefined as T);
  const hasSelectedRef = useRef(false);
  selectorRef.current = selector;
  isEqualRef.current = isEqual;

  return useSyncExternalStore(subscribe, () => {
    const next = selectorRef.current(snapshot);
    if (hasSelectedRef.current && isEqualRef.current(selectedRef.current, next)) {
      return selectedRef.current;
    }
    hasSelectedRef.current = true;
    selectedRef.current = next;
    return next;
  });
};

export const getDesktopSnapshot = (): DesktopSnapshot => snapshot;

const rejectBridgeCall = async <T>(): Promise<T> => {
  throw new Error(createT(snapshot.settings.desktop.language)("errorBridgeUnavailable"));
};

const getErrorMessage = (error: unknown): string => {
  if (error instanceof Error && error.message.trim().length > 0) {
    return error.message;
  }
  return createT(snapshot.settings.desktop.language)("errorUnknown");
};

const showDownloadBlockedToast = (): void => {
  const t = createT(snapshot.settings.desktop.language);
  const detail = snapshot.status.downloadMessage.trim();
  toastActions.show({
    message:
      detail.length > 0
        ? t("modelDownloadBlockingStartMessageWithDetail", { detail })
        : t("modelDownloadBlockingStartMessage"),
    title: t("modelDownloadBlockingStartTitle")
  });
};

const withToast = async <T>(title: string, task: () => Promise<T>): Promise<T> => {
  try {
    return await task();
  } catch (error) {
    toastActions.show({
      message: getErrorMessage(error),
      title
    });
    throw error;
  }
};

export const desktopActions = {
  async selectASRModel(id: ASRModel, activate = true): Promise<void> {
    updateSnapshot(await withToast(createT(snapshot.settings.desktop.language)("errorASRModelFailed"), () =>
      bridge?.prepareASRModel(id) ?? rejectBridgeCall()
    ));
    // Enrollment or offline transcription may start during the download.
    // Live recording can activate a prepared model at a segment boundary.
    if (!activate || (snapshot.asrModels.busy && !snapshot.status.recording)) return;
    await desktopActions.saveSettings({
      ...snapshot.settings,
      recording: { ...snapshot.settings.recording, asrModel: id }
    });
  },
  async installUpdate(): Promise<void> {
    await withToast(createT(snapshot.settings.desktop.language)("errorInstallUpdateFailed"), () =>
      bridge?.installUpdate() ?? rejectBridgeCall()
    );
  },
  async closeWindow(): Promise<void> {
    await withToast(createT(snapshot.settings.desktop.language)("errorUpdateWindowStateFailed"), () =>
      bridge?.closeWindow(windowRole) ?? rejectBridgeCall()
    );
  },
  async minimizeWindow(): Promise<void> {
    await withToast(createT(snapshot.settings.desktop.language)("errorUpdateWindowStateFailed"), () =>
      bridge?.minimizeWindow(windowRole) ?? rejectBridgeCall()
    );
  },
  async openMainWindow(): Promise<void> {
    await withToast(createT(snapshot.settings.desktop.language)("errorUpdateWindowStateFailed"), () =>
      bridge?.openMainWindow() ?? rejectBridgeCall()
    );
  },
  async openRecorderWindow(): Promise<void> {
    await withToast(createT(snapshot.settings.desktop.language)("errorUpdateWindowStateFailed"), () =>
      bridge?.openRecorderWindow() ?? rejectBridgeCall()
    );
  },
  async openExternal(target: string): Promise<void> {
    await withToast(createT(snapshot.settings.desktop.language)("errorOpenLinkFailed"), () =>
      bridge?.openExternal(target) ?? rejectBridgeCall()
    );
  },
  async openRecordingFolder(target: string): Promise<void> {
    await withToast(createT(snapshot.settings.desktop.language)("errorOpenRecordingFolderFailed"), () =>
      bridge?.openRecordingFolder(target) ?? rejectBridgeCall()
    );
  },
  async pickDirectory(defaultPath?: string): Promise<string | null> {
    return withToast(createT(snapshot.settings.desktop.language)("errorOpenDirectoryPickerFailed"), () =>
      bridge?.pickDirectory(defaultPath) ?? rejectBridgeCall()
    );
  },
  async exportVoiceprintSamples(recordingID: string, destinationDir: string): Promise<string> {
    const t = createT(snapshot.settings.desktop.language);
    try {
      return await bridge?.exportVoiceprintSamples(recordingID, destinationDir) ?? rejectBridgeCall();
    } catch (error) {
      toastActions.show({
        message: voiceprintExportErrorCopy(getErrorMessage(error), t),
        title: t("voiceprintExportFailed")
      });
      throw error;
    }
  },
  async runTranscribe(inputDir: string): Promise<void> {
    updateSnapshot(
      await withToast(createT(snapshot.settings.desktop.language)("errorRunTranscribeFailed"), () =>
        bridge?.runTranscribe(inputDir) ?? rejectBridgeCall()
      )
    );
  },
  async requestPermission(): Promise<MicrophonePermissionStatus> {
    const permission = await withToast(
      createT(snapshot.settings.desktop.language)("errorRequestPermissionFailed"),
      async () => {
        return bridge?.requestMicrophonePermission() ?? rejectBridgeCall<MicrophonePermissionStatus>();
      }
    );
    updateSnapshot({ ...snapshot, permission });
    return permission;
  },
  async saveSettings(settings: AppSettings): Promise<void> {
    const requestId = ++saveSequence;
    pendingSettings = settings;
    updateSnapshot({ ...snapshot, settings: pendingSettings });
    try {
      const saved = await withToast<AppSettings>(createT(settings.desktop.language)("errorSaveSettingsFailed"), () =>
        bridge?.saveSettings(pendingSettings ?? settings) ?? rejectBridgeCall()
      );
      if (requestId === saveSequence) {
        pendingSettings = null;
        updateSnapshot({ ...snapshot, settings: saved });
      }
    } catch (error) {
      if (requestId === saveSequence && bridge) {
        pendingSettings = null;
        const restored = await bridge.bootstrap();
        if (requestId === saveSequence) updateSnapshot(restored);
      }
      throw error;
    }
  },
  async setWindowPinned(pinned: boolean): Promise<void> {
    updateSnapshot(
      await withToast(createT(snapshot.settings.desktop.language)("errorUpdateWindowStateFailed"), () =>
        bridge?.setWindowPinned(pinned) ?? rejectBridgeCall()
      )
    );
  },
  async startRecording(): Promise<void> {
    if (snapshot.settings.desktop.voiceprintSetup === "pending") {
      updateSnapshot(await withToast(createT(snapshot.settings.desktop.language)("errorStartRecordingFailed"), () =>
        bridge?.startRecording() ?? rejectBridgeCall()
      ));
      return;
    }
    const title = createT(snapshot.settings.desktop.language)("errorStartRecordingFailed");
    if (snapshot.status.downloading && !snapshot.settings.recording.disableAsr &&
      (!snapshot.status.vadReady || !snapshot.status.senseVoiceReady)) {
      showDownloadBlockedToast();
      return;
    }
    if (snapshot.permission.state === "not-determined") {
      await desktopActions.requestPermission();
    }
    const nextSnapshot = await withToast<DesktopSnapshot>(title, () =>
      bridge?.startRecording() ?? rejectBridgeCall()
    );
    updateSnapshot(nextSnapshot);
    if (nextSnapshot.status.recording) {
      return;
    }
    const message =
      nextSnapshot.status.error?.trim() ||
      nextSnapshot.status.statusMessage.trim() ||
      createT(nextSnapshot.settings.desktop.language)("errorUnknown");
    if (message !== lastStatusError) {
      lastStatusError = message;
      toastActions.show({
        message,
        title
      });
    }
    throw new Error(message);
  },
  async stopRecording(): Promise<void> {
    updateSnapshot(
      await withToast(createT(snapshot.settings.desktop.language)("errorStopRecordingFailed"), () =>
        bridge?.stopRecording() ?? rejectBridgeCall()
      )
    );
  },
  async openMicrophoneSettings(): Promise<boolean> {
    return withToast(createT(snapshot.settings.desktop.language)("errorOpenSystemSettingsFailed"), () =>
      bridge?.openMicrophoneSettings() ?? rejectBridgeCall()
    );
  },
  async openSystemAudioSettings(): Promise<boolean> {
    const title = createT(snapshot.settings.desktop.language)("errorOpenSystemSettingsFailed");
    return withToast(title, async () => {
      const opened = await (bridge?.openSystemAudioSettings() ?? rejectBridgeCall<boolean>());
      if (!opened) throw new Error(title);
      return opened;
    });
  },
  async retryVoiceprintReference(): Promise<void> {
    updateSnapshot(await withToast(createT(snapshot.settings.desktop.language)("voiceprintReferenceRetryFailed"), () =>
      bridge?.retryVoiceprintReference() ?? rejectBridgeCall()
    ));
  },
  async startVoiceprintEnrollment(): Promise<void> {
    await runVoiceprint(() => bridge?.startVoiceprintEnrollment() ?? rejectBridgeCall());
  },
  async advanceVoiceprintEnrollment(): Promise<void> {
    await runVoiceprint(() => bridge?.advanceVoiceprintEnrollment() ?? rejectBridgeCall());
  },
  async cancelVoiceprintEnrollment(): Promise<void> {
    await runVoiceprint(() => bridge?.cancelVoiceprintEnrollment() ?? rejectBridgeCall());
  },
  async skipVoiceprintSetup(): Promise<void> {
    await runVoiceprint(() => bridge?.skipVoiceprintSetup() ?? rejectBridgeCall());
  },
  async clearVoiceprint(): Promise<void> {
    await runVoiceprint(() => bridge?.clearVoiceprint() ?? rejectBridgeCall());
  },
  async clearVoiceprintAdaptation(): Promise<void> {
    await runVoiceprint(() => bridge?.clearVoiceprintAdaptation() ?? rejectBridgeCall());
  }
};

const runVoiceprint = async (task: () => Promise<DesktopSnapshot>): Promise<void> => {
  try {
    updateSnapshot(await task());
  } catch (error) {
    toastVoiceprint(getErrorMessage(error));
    throw error;
  }
};
