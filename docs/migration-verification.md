# mygo migration verification

Updated: 2026-10-18. **Manual migration acceptance remains incomplete.** Local backend, inference and package checks pass on Apple Silicon; visible-window parity and the remaining platform acceptance runs are outstanding. For 1.0.0 the user explicitly approved publication after all automated checks pass, while retaining these incomplete manual results. `migration-verification.json` is unchanged; CI reports its outstanding items without treating them as a publication gate.

## Implementation and versions

| Component | Locked version |
| --- | --- |
| Go | 1.27.1, CGO enabled |
| mygo / runtime / CLI | 0.3.4 |
| malgo | 0.11.26 |
| sherpa Go and platform modules | 1.13.8 |
| ONNX Runtime | 1.28.2 |
| Qwen3 model | 0.6B int8, 2026-03-25, unchanged |
| Go FLAC codec | mewkiz/flac 1.0.14; no FFmpeg dependency |

The Go process owns capture, VAD, the serialized ASR worker, speech-only archives and shutdown. Reloading or hiding a page does not own or cancel recordings. The recording controls and canvas drawing parameters are retained. The user subsequently requested a separate main window using the Fluid Functionalism Sidebar: Home, History, General, Input and Transcribe now live there, while the compact recorder provides recent history and a shortcut to it. Home combines segmented transcripts into complete paragraphs with source times, latest first. See [workspace verification](verification/workspace-window.md). The user explicitly approved deleting the obsolete FFmpeg status row; the corresponding unavailable-FFmpeg warning is also removed. Other frontend changes handle desktop bindings, spectrum delivery and WebView compatibility. No new partial-transcription status is displayed.

The engine also preserves the original recording status messages and clears the current transcript when rotating segments. Disabling ASR clears preview/history; an old segment's late inference result cannot restore them.

## Local checks

Host: macOS 27.0 (26A428), arm64. This is newer than the minimum supported OS and does not establish macOS 13.3 compatibility.

| Check | Result / scope |
| --- | --- |
| TypeScript / Vitest | Pass |
| Go tests with race detector / vet | Pass; native inference is opt-in and was also run separately |
| Playwright | 15 pass; renderer against the native Go service with a fake recording engine, including the Home timeline, transcript merging/reflow and two-window navigation |
| Native WebView smoke | Bindings, recording, pure recording, window/application hide and reload continuity, 420×620 viewport pass; visible desktop acceptance remains separate |
| Configuration | Canonical/legacy precedence, backup, idempotence, invalid configuration, browser device fallback and failed-write rollback pass |
| Recording lifecycle | VAD flush, repeated start/stop, setting rotation, device failures, sleep intent, bounded queues, overflow stop, batch cancellation and explicit native release pass |
| Spectrum | 128 values; FFT 256 / Blackman / smoothing 0.8 compared with a real Chromium analyser fixture, within one byte |
| FLAC failure | WAV remains available; diagnostics are recorded without adding a UI state |
| Package | arm64 app and DMG build; executable diagnostic confirms loaded sherpa 1.13.8 / ONNX Runtime 1.28.2; relative rpaths, minimum-OS load commands and ad hoc code signature verified |
| Audio conversion | Pure Go codecs pass 42 independent Xiph decoder checks with identical PCM; the packaged binary encodes and decodes with an empty PATH, reads legacy FFmpeg-generated FLAC, and downmixes/resamples to 16 kHz mono with anti-alias filtering |
| Real microphone / batch | Native UI starts and stops the production engine: waveform, Chinese transcript, 94,816 archived speech samples and a valid FLAC/JSON pair; existing WAV and FLAC batch inputs produce identical nonempty text |
| Signed updates | Archive size and Ed25519 signature pass; corrupted signatures/digests fail; three-target collection checks filename collisions |
| Update scheduling | Tests defer installation while recording/operations are active; an installation failure does not restart the app |

The local Apple Silicon app is about 36.1 MB; the DMG is about 10.5 MB (decimal MB). Stripping local native-library symbols reduced the measured app from 47.4 MB (23.9%) and the DMG from 11.4 MB (7.4%). The library exports and code/data sections are unchanged. Upstream signatures are removed before stripping so mygo signs the modified copies; package verification checks every bundled library's signature individually. This is an ad hoc signed development build, not a notarized distribution acceptance result. The application package contains no Electron runtime, Node runtime, Node sherpa package, FFmpeg or ffprobe executable.

The native recording integration substitutes fixed PCM for the microphone and uses the production sherpa VAD, Qwen recognizer, WAV writer, Go FLAC encoder, segment finalizers and JSON writer. All four WAV/FLAC × transcription/pure-recording cases pass. It archived 176,544 speech samples from the 182,086-sample Chinese fixture and finished a nonempty transcript. With ASR disabled, it wrote speech audio without creating a recognizer or transcription JSON.

A separate short hardware run used the built application without `EVE_E2E_TEST`, the MacBook Pro microphone and real UI controls. It displayed changing levels and a live waveform, recognized Chinese speech, stopped successfully, and wrote a 109,556-byte FLAC with nonempty JSON. A real history-page batch action also processed fixed WAV/FLAC inputs with identical transcripts; see [real-recording.json](verification/real-recording.json). Device unplugging and long recordings remain outstanding. The native visual smoke harness uses a mock engine and cannot verify hardware capture or ASR; its frozen screenshot fixtures must not be presented as a usable recorder. The smoke harness closes its mock windows after testing.

## UI evidence and remaining acceptance

The baseline is the unchanged Electron source at commit `a7216969f8a69ca89fae9da33475681b65a2c41d`. Its 52 fixtures cover four tabs, both themes, both languages, recording/transcription, pure recording, stop animation, downloads, permission denial, errors/toasts, populated history and dropdowns. Source images and checksums are under `apps/desktop/e2e/baseline/electron-darwin-arm64`; migrated output never replaces them. Four separately checksummed transcription-tab references under `approved-no-ffmpeg-darwin-arm64` remove exactly the user-approved row from the unchanged old app; the other 48 fixtures retain their original references.

Before the user-requested two-window layout change, all 52 visible-page comparisons passed for text, color, element geometry (0.125 CSS-pixel rounding allowance; observed maximum 0.109375) and the original canvas operation stream (1e-10 floating-point allowance). Reload capture waits for a new document and font readiness, avoiding a stale first-frame measurement. See [ui-comparison.json](verification/ui-comparison.json). Matching these does not establish pixel or native-window parity.

The macOS compatibility layer resolves the installed static PingFang collection through CoreText and serves that file through an application font resource. WebKit loads its Regular, Medium and Semibold members by PostScript fragment, bypassing ambiguous installed-name matching. Apple fonts and version-specific system paths are not bundled. The 12 px semibold string “Failed to start recording” is 138.53125 px in both the original Electron capture and the new custom-font path; installed-family lookup in WKWebView produced 141.671875 px. Application unhide now also precedes window reveal, as in the original app, and the native smoke test exercises that behavior.

After unlocking, desktop control obtained real window screenshots and exercised production recording and batch controls. The strict visual capture rejects hidden pages; foreground/occlusion changes can still interrupt a run. See [native-window-state.json](verification/native-window-state.json). Traffic-light buttons, native material, drag/focus behavior, tray placement, native scaling and raster differences remain unapproved. A title-bar drag regression was reproduced; its compatibility handler has not yet passed real-input acceptance. Windows and Intel Mac also need original baselines on their own platforms.

The spectrum fixture uses the same 16 kHz PCM in both implementations. It verifies the FFT and smoothing calculation; it does not prove that every microphone's old browser-selected hardware sample rate or browser audio processing produces identical input. The initial migration used unprocessed input. Following the reported speaker-playback contamination, macOS now defaults to native voice processing before VAD and ASR, with an explicit unprocessed option in Input. See [echo cancellation](subsystem/audio.md); the earlier recording evidence predates this processing change.

## sherpa regression evidence

The unchanged model archive was verified against SHA-256 `393f8a14e2f5fb96746aaab342997a40641001fbd5bf9592a080a8329178ee96`. Fixtures come from its official Chinese sample, a three-second excerpt, silence and a longer repeated-speech sample. Hashes are in [audio-fixtures.json](verification/audio-fixtures.json); complete outputs are in [sherpa-regression.json](verification/sherpa-regression.json).

| Fixture | Duration | Old decode, ms | New decode, ms | Text |
| --- | ---: | ---: | ---: | --- |
| Chinese | 11.380 s | 1802 | 1724 | Changed |
| Short | 3 s | 493 | 455 | Changed |
| Silence | 3 s | 244 | 221 | Empty in both |
| Long repetitive speech | 36.141 s | 2522 | 2207 | Incomplete in both versions |

These are local diagnostics, not controlled performance or accuracy guarantees. The old baseline used Node sherpa 1.12.34 / ONNX Runtime 1.23.2 with its old settings. The new runtime uses CPU, two threads, 128 feature dimensions and explicit seed 42; each fixture decoded twice with identical results within that process. Transcripts differ, so equal recognition quality has not been established by these samples alone.

Whole-process peak RSS for two-pass runs was 3,507,339,264 bytes with Node and 4,271,276,032 bytes with Go, approximately 22% higher. This includes initialization and transient allocations; Node GC and explicit Go/native stream disposal differ. Memory improvement is not claimed. See [new results](verification/new-asr-results.json) and [old two-pass results](verification/old-asr-two-passes-results.json).

## Packaging and upgrade

FFmpeg has been removed from runtime, build preparation, packaging and release artifacts. Go streams lossless FLAC encoding and WAV/FLAC decoding, validates frame integrity and PCM checksums, and downmixes/resamples batch input. Encoding cancellation or failure retains the source WAV and removes staging files. Integer PCM WAV supports 8/16/24/32 bits; FLAC encoding supports 8/16/24 bits and decoding up to 24 bits, including existing recorder archives. Independent Xiph FLAC 1.5.0 checks verify exact decoded PCM; see [go-flac.json](verification/go-flac.json). The fixed-predictor Go encoder can produce larger FLAC files than the former LPC-capable encoder; equal compression ratios are not claimed. Required Unlicense/Apache notices accompany the Go dependencies. No external codec is required at build or runtime.

Normal builds and development use the same `Eve Recorder` data by default, preferring valid settings there before importing `eve-desktop`. Settings are backed up before migration; absolute recording paths and old recordings stay in place. Model caches are searched in both directories without deleting the old cache. Tests and explicit `EVE_STORE_DIR` runs stay isolated. Development and isolated runs do not apply login items.

The first Electron→mygo upgrade requires manual installation: quit and replace the app on macOS; on Windows, quit and uninstall the old client while preserving user data, then install the new client. Subsequent versions use mygo signed updates, deferred until recording, finalization and batch transcription are idle.

The public update key is checked in at `apps/desktop/build/mygo-update.pub`; the private key is outside the repository and configured as the CI secret `MYGO_UPDATER_PRIVATE_KEY`. Distribution signing/notarization and real signed-update recovery have not been accepted locally. macOS releases use the configured Developer ID and notarization credentials. Windows installers intentionally have no Authenticode certificate signature, as confirmed by the user; their updater archives must still pass Ed25519 verification.

## Manual acceptance still open

The remaining manual acceptance covers visible UI parity, native inference, the minimum supported OS, clean-machine installation, signed-update recovery and at least eight hours of real recording on each target. Currently only arm64 native inference is accepted in the report. No minimum-OS, Windows, Intel Mac, clean-machine, notarization or endurance result is inferred from the local build. `apps/desktop/scripts/release-gates.mjs` still fails by default for incomplete acceptance; CI uses its `--report` mode under the approved 1.0.0 release policy. Publication requires all three native CI jobs, complete installers, matching tag/package/update versions and verified update signatures.

Reproduction commands and the native inference environment variables are documented in [workspace](subsystem/workspace.md) and [asr](subsystem/asr.md).
