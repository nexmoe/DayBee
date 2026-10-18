import type { RecorderStatusSnapshot } from "@daybee/shared";

export const statusTone = (
  status: RecorderStatusSnapshot
): "idle" | "recording" => {
  return status.recording ? "recording" : "idle";
};
