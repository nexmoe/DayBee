import type { createT, MessageKey } from "./i18n";

const errors: Record<string, MessageKey> = {
  "The speech sample was too short. Please try again.": "voiceprintErrorShort",
  "Not enough speech was detected. Check your microphone and try again.": "voiceprintErrorNoSpeech",
  "Not enough speech to identify the speaker.": "voiceprintErrorShort",
  "At least 0.8 seconds of 16 kHz speech is required.": "voiceprintErrorShort",
  "At least one second of 16 kHz speech is required.": "voiceprintErrorShort",
  "The recording is too loud. Move away from the microphone and try again.": "voiceprintErrorLoud",
  "The recording contained invalid audio. Please try again.": "voiceprintErrorInvalid",
  "The verification voice did not match. Record your voice again in a quiet place.": "voiceprintErrorMismatch",
  "Wait for the current transcription to finish before enrolling.": "voiceprintErrorBusy",
  "The room was not quiet long enough to measure noise. Stay quiet and try again.": "voiceprintErrorNoise",
  "Background noise is too high. Move to a quieter place and record again.": "voiceprintErrorSNR",
  "The microphone or echo cancellation changed. Record again with the same input.": "voiceprintErrorDevice",
  "The selected microphone could not be identified. Choose an input device and try again.": "voiceprintErrorDeviceUnknown",
  "These samples do not sound like the same person. Record all three samples again.": "voiceprintErrorInconsistent",
  "The enrollment does not separate from other speakers. Record your voice again in a quiet place.": "voiceprintErrorInseparable",
  "A specific microphone is required to save this voiceprint.": "voiceprintErrorDevice",
  "Voiceprint enrollment needs separate recording segments.": "voiceprintErrorSegments",
  "A valid speaker embedding is required.": "voiceprintErrorEmbedding",
  "Finish or cancel voiceprint enrollment before clearing learned samples.": "voiceprintAdaptationEnrollmentActive",
  "Voiceprint initialization is unavailable.": "voiceprintAdaptationUnavailable",
  "Voiceprint manager is unavailable.": "voiceprintAdaptationUnavailable",
  "Voiceprint manager is closed.": "voiceprintAdaptationUnavailable",
  "Finish recording and transcription before exporting voiceprint samples.": "voiceprintExportBusyError",
  "That recording is not in the current history.": "voiceprintExportNotFound",
  "That recording is not finished.": "voiceprintExportNotFinished",
  "Voiceprint export only accepts regular WAV or FLAC files.": "voiceprintExportRegular",
  "Choose a folder for the voiceprint samples.": "voiceprintExportFolder",
  "Could not create a new voiceprint sample directory.": "voiceprintExportFailed"
};

const qualities: Record<string, { key: MessageKey; kind: "ambient" | "paused" | "reference" }> = {
  "Stay quiet for a moment while the room is measured.": { key: "voiceprintAmbient", kind: "ambient" },
  "Playback was detected. Enrollment is paused and this audio will not be saved.": { key: "voiceprintPlaybackPaused", kind: "paused" },
  "System playback cannot be checked. Pause videos yourself. Recording will continue.": { key: "voiceprintReferenceLimited", kind: "reference" }
};

export function voiceprintQualityCopy(message: string, t: ReturnType<typeof createT>) {
  const item = qualities[message];
  if (!item) {
    return null;
  }
  return { text: t(item.key), kind: item.kind };
}

export function voiceprintExportErrorCopy(error: string, t: ReturnType<typeof createT>) {
  const key = errors[error.trim()];
  return t(key ?? "voiceprintExportFailed");
}

export function voiceprintErrorCopy(error: string, t: ReturnType<typeof createT>, target: number) {
  const key = errors[error];
  return {
    message: t(key ?? "voiceprintErrorGeneric", { target: Math.round(target) }),
    detail: key ? null : error
  };
}
