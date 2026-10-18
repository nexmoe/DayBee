import { describe, expect, it, vi } from "vitest";
import { DEFAULT_STATUS } from "@daybee/shared";
import type { DesktopSnapshot, DesktopStatusSnapshot } from "@daybee/shared";

const handlers = vi.hoisted(() => ({
  full: undefined as ((snapshot: DesktopSnapshot) => void) | undefined,
  status: undefined as ((snapshot: DesktopStatusSnapshot) => void) | undefined
}));

vi.mock("./desktop-api", () => ({
  desktopApi: { bootstrap: () => new Promise(() => {}) },
  desktopEvents: {
    onSnapshot: (handler: typeof handlers.full) => { handlers.full = handler; },
    onStatus: (handler: typeof handlers.status) => { handlers.status = handler; }
  }
}));
vi.mock("./toast-store", () => ({ toastActions: { show: vi.fn() } }));

import { getDesktopSnapshot } from "./desktop-store";

describe("desktop status stream", () => {
  it("updates meters and enrollment without invalidating the saved timeline, then accepts new speech", () => {
    const initial = getDesktopSnapshot();
    const history: DesktopSnapshot["history"] = [{
      id: "saved", audioSource: "microphone", audioPath: "/recordings/saved.flac", folderPath: "/recordings",
      inputDevice: null, startedAt: "2026-10-12T10:00:00Z", status: "ok", sessionId: "session",
      textPreview: "Saved transcript", transcriptChunks: []
    }];
    const microphoneActivity = { ...initial.microphoneActivity, items: [{
      id: "meeting", appId: "meeting", appName: "Meeting", appIcon: "data:image/png;base64,icon",
      startedAt: "2026-10-12T10:00:00Z", endedAt: null
    }] };
    handlers.full!({ ...initial, history, microphoneActivity });
    for (let index = 0; index < 100; index++) {
      handlers.status!({
        status: { ...DEFAULT_STATUS, recording: true, elapsed: "00:00:10", levelRatio: index / 100 },
        voiceprint: { ...initial.voiceprint, phase: "recording", speechSeconds: index / 20 }
      });
      const updated = getDesktopSnapshot();
      expect(updated.history).toBe(history);
      expect(updated.microphoneActivity).toBe(microphoneActivity);
      expect(updated.status.levelRatio).toBe(index / 100);
      expect(updated.voiceprint.speechSeconds).toBe(index / 20);
    }
    const newHistory = [{ ...history[0], textPreview: "New speech" }, ...history];
    handlers.full!({ ...getDesktopSnapshot(), history: newHistory });
    expect(getDesktopSnapshot().history).toBe(newHistory);
    expect(getDesktopSnapshot().history[0].textPreview).toBe("New speech");
  });
});
