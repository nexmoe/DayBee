# DayBee

English | [中文](README.zh.md)

DayBee is a tray-first desktop recorder built with Go and mygo. It records microphone audio for long sessions, segments files automatically, and uses selectable `sherpa-onnx` ASR models plus Silero VAD for live transcription and offline reprocessing.

## Features

- Long-running microphone recording with automatic segmentation
- Live transcription with lightweight Whisper Tiny/Base/Small, SenseVoice Small, Qwen3 ASR 0.6B or FireRedASR2 AED/CTC (can be disabled)
- VAD-only recording — only speech segments are saved to disk
- Real-time waveform visualization
- Manual microphone selection with automatic switch to the more active input
- Native echo cancellation on macOS to reduce speaker playback in recordings and transcripts
- Optional macOS system audio capture, saved and displayed separately from microphone speech
- WAV and FLAC output
- Batch transcription of existing WAV/FLAC recordings
- Recording history viewer grouped by day
- Annotate transcripts with app names and icons from the same recording period; filter transcripts and recordings by app
- System tray app with launch-at-login, start-on-launch, and auto-update
- Bilingual UI (English / 中文)
- Light / Dark / System theme

## Desktop Preview

![DayBee desktop app](docs/images/desktop-gui-preview.png)

## Download

- Release page: [DayBee Releases](https://github.com/nexmoe/eve/releases)
- macOS 13.3+: Apple Silicon / Intel `.dmg`
- Windows: `.exe` (NSIS installer)

The packaged app checks for signed updates on startup and every six hours, then installs and restarts when recording and transcription are idle. The first upgrade from Electron requires a manual installation; see the [migration verification and upgrade notes](docs/migration-verification.md). Migration builds remain unreleased until the acceptance gates pass.

## Runtime Notes

- New installations start with lightweight Whisper Tiny (INT8). Whisper Base/Small, SenseVoice Small (2025), Qwen3 ASR 0.6B and FireRedASR2 AED/CTC are also available in Transcribe settings. Existing model choices are preserved. The page shows download size, progress and installed status; models can be switched while recording, and queued speech finishes with its original model.
- Silero VAD downloads when recording starts. The selected ASR model downloads when chosen or when live/batch transcription first needs it. Existing model caches are reused; failed downloads preserve the previous selection.
- Native audio capture, sherpa-onnx 1.13.8 and ONNX Runtime 1.28.2 are bundled. Installed apps need no Node.js, Bun or Go runtime.
- WAV/FLAC encoding, decoding, downmixing and resampling run directly in Go. Existing FLAC recordings remain readable; no FFmpeg executable or system installation is required.
- On macOS, echo cancellation is enabled by default, including for existing settings. Turn it off in Input for unprocessed capture. Auto-switch input remains independently configurable and keeps echo cancellation enabled after switching; disconnect recovery remains available. See [echo cancellation](docs/subsystem/audio.md). System audio recording is opt-in on the same page; see [source separation](docs/subsystem/audio.md).
- Microphone app detection runs independently while DayBee is open, checks once per second, and requires macOS 14.2+ on Mac. Daily archives are stored under `microphone-activity/` in the app data directory. See [microphone activity notes](docs/subsystem/microphone-activity.md).

## Output

- Audio and transcript JSON files are archived under `recordings/YYYYMMDD/`
- Filenames look like `daybee_YYYYMMDD_HHMMSS.wav` or `.flac`
- With ASR enabled, each segment writes a matching `.json` with transcript text and metadata. ASR-disabled recording saves audio and capture timing metadata without transcript text.

Example JSON:

```json
{
  "audio_file": "daybee_20260201_120513.wav",
  "audio_path": "/path/to/recordings/20260201/daybee_20260201_120513.wav",
  "backend": "sherpa-onnx",
  "created_at": "2026-02-01T12:05:13.000Z",
  "input_device": "MacBook Air Microphone",
  "model": "Qwen3 ASR 0.6B",
  "segment_start_time": "2026-02-01T12:05:13.000Z",
  "status": "ok",
  "text": "Please stand up and stretch for a minute."
}
```

## More Documentation

- [Subsystem reference](docs/subsystem/README.md)
- [Changelog](CHANGELOG.md)
