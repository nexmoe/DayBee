import { isMyGo } from "mygo-runtime";
import { DesktopService, events } from "@/generated/mygo";

// Keep the existing action contract while all DTOs and calls come from Go.
export const desktopApi = isMyGo() ? {
  ...DesktopService,
  pickDirectory: (defaultPath?: string) => DesktopService.pickDirectory(defaultPath ?? null)
} : undefined;

export const desktopEvents = {
  onSnapshot: events.desktopSnapshot.on,
  onStatus: events.desktopStatus.on,
  onWaveform: events.desktopWaveform.on
};
