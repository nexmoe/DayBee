import { beforeEach, describe, expect, it, vi } from "vitest";
import { DEFAULT_STATUS, type DesktopSnapshot } from "@daybee/shared";

const handlers = vi.hoisted(() => ({
  snapshot: undefined as ((snapshot: DesktopSnapshot) => void) | undefined,
  start: vi.fn<() => Promise<DesktopSnapshot>>()
}));

vi.mock("./desktop-api", () => ({
  desktopApi: { bootstrap: () => new Promise(() => {}), startRecording: handlers.start },
  desktopEvents: {
    onSnapshot: (handler: typeof handlers.snapshot) => { handlers.snapshot = handler; },
    onStatus: () => {}
  }
}));
vi.mock("./toast-store", () => ({ toastActions: { show: vi.fn() } }));

import { desktopActions, getDesktopSnapshot } from "./desktop-store";
import { toastActions } from "./toast-store";

describe("recording while downloading another model", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    const initial = getDesktopSnapshot();
    handlers.snapshot!({
      ...initial,
      permission: { state: "authorized", supported: true, message: "" },
      settings: { ...initial.settings, desktop: { ...initial.settings.desktop, voiceprintSetup: "skipped" },
        recording: { ...initial.settings.recording, disableAsr: false } },
      status: { ...DEFAULT_STATUS, downloading: true, vadReady: true, senseVoiceReady: true },
      asrModels: { ...initial.asrModels, busy: false, downloadingModel: "firered-asr2-ctc" }
    });
    handlers.start.mockImplementation(async () => {
      const snapshot = getDesktopSnapshot();
      return { ...snapshot, status: { ...snapshot.status, recording: true } };
    });
  });

  it("starts with the installed active model without waiting for an unrelated download", async () => {
    await desktopActions.startRecording();
    expect(handlers.start).toHaveBeenCalledOnce();
    expect(getDesktopSnapshot().status.recording).toBe(true);
    expect(toastActions.show).not.toHaveBeenCalled();
  });

  it("still blocks starting while the active model is missing", async () => {
    const snapshot = getDesktopSnapshot();
    handlers.snapshot!({ ...snapshot, status: { ...snapshot.status, senseVoiceReady: false } });
    await desktopActions.startRecording();
    expect(handlers.start).not.toHaveBeenCalled();
    expect(toastActions.show).toHaveBeenCalledOnce();
  });
});
