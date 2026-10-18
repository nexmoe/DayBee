# Changelog

All notable changes to this project are documented in this file.

## Unreleased

### Maintenance

- Publish after all automated platform checks and signed-update verification pass, while reporting incomplete manual migration acceptance. Windows installers do not require Authenticode certificates.
- Rename the GitHub repository to `nexmoe/DayBee` and update the Go module path, app update feed, website links and subsystem documentation.

### Fixes

- Define Windows Core Audio GUIDs when compiling microphone activity detection, and allow the system font fallback when optional static PingFang faces are absent.
- Reject releases with mismatched tag/update versions or missing installers.
- Restore an intact native library when older Xcode tools cannot strip its symbols, and preserve Windows executable search paths while preparing native dependencies.
- Use version 1.0.0 as the fallback when launching the Go entry point directly.

## 1.0.0 - 2026-10-18

### Features

- Add Whisper Tiny, Base and Small INT8 models, defaulting new installations and incomplete settings to Whisper Tiny while preserving existing model choices. Show verified archive download sizes in model settings.
- Add the DayBee marketing website with product information, model descriptions, download links and search metadata.

### Performance

- Release idle ASR models after two minutes and on recording shutdown, pause hidden-window updates and waveform work, and use a faster FFT for the audio spectrum.

### Fixes

- Persist transcript capture timing before notifying history listeners so FLAC finalization preserves live transcript scene boundaries.

### Desktop migration

- Rename the project and desktop product to DayBee, including workspace packages, application identifiers, permissions, development routes and installers. Use DayBee data and recording folders for new installations; import existing settings, voiceprints and microphone activity once, reuse model caches and retain old recordings. Keep the GitHub repository at `nexmoe/eve`.

- Send recording meters and voiceprint progress separately from saved data; publish full snapshots only when history, application annotations or settings change, and reuse unchanged timelines to reduce CPU and memory usage across both windows.

- Share the packaged app's settings, recordings, model caches and voiceprints in development by default; keep tests and explicit `DAYBEE_STORE_DIR` runs isolated and skip login-item changes in development.

- Clarify voiceprint recording with one description, complete passages to read aloud, explicit sample and verification actions, and localized recovery messages that retain the failed step.

- Add ASR model selection to Transcribe settings with Qwen3 ASR 0.6B, FireRedASR2 AED/CTC and SenseVoice Small (2025). Show download progress and cache status, persist the selection, switch recognizers safely while idle, and identify the selected model in transcript metadata.

- Repeat Me before each matching sentence, leave unidentified speech unlabelled, retain system audio labels, and simplify the voiceprint card to one status line with standard small buttons. Keep E2E windows hidden and unfocused by default.

- Add optional voiceprint enrollment and verification, automatically labeling matched microphone transcript chunks as “Me” whenever a saved profile is available while retaining all text and audio. Keep the Home voiceprint card visible after canceling, skipping and enrollment; support re-enrolling and clearing the profile, and keep system-audio transcription independent.

- Add shared virtual scrolling to the compact and main timelines, keeping all history searchable and preserving the reading position as new transcripts arrive.

- Reduce macOS installation size by stripping local symbols from bundled native libraries before signing; verify each library's signature separately.
- Group timeline entries by application microphone capture sessions, keeping silence, long transcripts and recording-file changes inside one entry; app filters still include only overlapping speech.
- Keep continuous speech in one timeline entry when microphone activity begins inside the first utterance of the same app session.
- Show only clock times in the timeline, with full dates on hover, and list apps currently using the microphone above it.
- Show a highlighted amber update icon to the left of Pin when a new version is detected. Click to save the current recording, install and restart; failed updates can be retried without restarting.
- Annotate transcripts with the names and icons of apps using the microphone at the same time; separate scene changes, filter transcripts and audio files by app, and retain capture timing for audio-only recordings (macOS 14.2+ on Mac).

- Add a main window with the Fluid Functionalism Sidebar for History, General, Input and Transcribe. Keep recording controls and a main-window shortcut in the compact recorder; both windows share settings and recording state.
- Open the main window on a new Home timeline that joins segmented transcriptions, reflows the full text into readable paragraphs and shows the latest content first with its source times.
- Show the same full transcription timeline in the compact recorder, replacing recording-history summaries. New transcripts update automatically; file history remains in the main window.
- Remove the compact recorder's separate live-transcript card. Persist each completed recognition and push the updated timeline to both windows immediately, including before recording stops. Add Home transcript search with highlighted matches and full paragraph context.
- Without an application context, keep timeline entries within recording sessions, split speech gaps over one minute and cap merged groups at five minutes. Preserve per-utterance capture times in new recordings and original recording times when reprocessing older audio.
- Migrate the desktop runtime to mygo 0.3.4 and Go 1.27.1, with malgo native microphone capture and application-owned recording tasks.
- Upgrade sherpa Go/native dependencies to 1.13.8 and ONNX Runtime to 1.28.2; retain the existing Qwen3/Silero model caches and React interface.
- Preserve speech-only WAV/FLAC archives, add bounded inference queues and safe shutdown, and import existing Electron settings without moving recordings.
- Replace Electron packaging with native macOS/Windows builds, signed mygo updates and explicit UI/inference/endurance release gates.
- Remove FFmpeg entirely: stream lossless FLAC encoding and WAV/FLAC decoding in Go with mewkiz/flac 1.0.14, retaining legacy recordings. Remove the obsolete FFmpeg status row as approved.
- Migration acceptance is still in progress; see `docs/migration-verification.md` before release.

## 0.8.6 - 2026-04-08

### Fixes
- macOS: keep the tray-revealed window visible by suppressing transient blur events during Dock hide.

## 0.8.5 - 2026-04-05

### Documentation
- README: rename project to Eve Recorder, refresh feature list (Silero VAD, VAD-only mode, history, themes), document macOS `.dmg`/`.zip` and Windows NSIS installers, simplify runtime notes, and link the changelog.

### Build
- Desktop: configure `electron-builder` auto-update publish as generic URL pointing at GitHub Releases downloads.

## 0.8.4 - 2026-04-05

### Features
- Desktop: system tray menu shows app version, auto-update status, manual check, and quit-and-install when an update is ready.

## 0.8.3 - 2026-04-03

### Fixes
- macOS: activate the app and briefly toggle `setVisibleOnAllWorkspaces` when revealing from the tray so the window receives focus while the Dock icon stays hidden.

### Features
- Home: place the GitHub star card in the settings panel stack and simplify its styling (no fixed overlay).

## 0.8.2 - 2026-04-03

### Features
- Windows: custom title-bar minimize and close controls wired to main-process window actions.
- Home: GitHub star affordance for the desktop shell.

### Fixes
- Throttle desktop snapshot IPC emissions while recording to cut renderer churn from high-frequency audio status updates.
- Deliver live audio chunks to the engine with transferable `ArrayBuffer` payloads instead of large numeric arrays.
- Refine macOS Dock visibility around microphone permission prompts.

### Maintenance
- Harden Sherpa runtime preparation and FFmpeg setup scripts; minor `electron-builder` metadata tweaks.

## 0.8.1 - 2026-03-28

### Fixes
- Hide the main window from the tray toggle only when it is visible and focused; otherwise reveal and focus it.
- On Windows, center the window on reveal, set a distinct App User Model ID, and use the packaged window icon on non-macOS.

### Features
- Rebrand the desktop app display name to Eve Recorder across the bundle metadata, window title, tray tooltips, and microphone permission strings.

## 0.8.0 - 2026-03-28

### Features
- VAD-only recording: append only VAD speech segments to saved audio, awaiting flush and drain on stop and segment rotation.
- Keep the VAD pipeline aligned with ASR throttling without decoding throttled segments.
- Add `AppInfoSnapshot` and `AutoUpdateSnapshot` to `DesktopSnapshot` from the main process.
- Track auto-update phase, defer install while recording, and install after recording stops.
- General settings: version and update status plus GitHub star and releases actions.

## 0.7.0 - 2026-03-28

### Features
- Make desktop `applySettings` asynchronous: rotate recording segments when format, output path, or ASR settings change, and recreate the recognizer when language or ASR-disable toggles change.
- When ASR is disabled, use a null JSON path, skip JSON writes, and clear ASR preview and history; track pending settings in the renderer store during save to avoid races on rapid saves.
- Add DesktopEngine unit tests covering ASR-disable sidecar behavior.

### Maintenance
- Bump `actions/checkout` and `actions/setup-node` to v5 in the desktop release workflow.

## 0.6.0 - 2026-03-27

### Breaking Changes
- Migrate the desktop runtime from the previous Python sidecar pipeline to a Sherpa + FFmpeg architecture, including a new audio processing flow and runtime management.

### Refactor
- Refactor the desktop runtime internals to align with the Sherpa/FFmpeg pipeline and simplify the main-process execution path.

### Fixes
- Avoid double-initializing the packaged `electron-log` preload bridge so the desktop renderer no longer crashes into a blank settings window on macOS.
- Disable the translucent glass tray window style in packaged macOS builds and fall back to a solid background for more stable desktop window rendering.
- Log packaged renderer load failures and renderer-process exits in the desktop main process to make post-release crash diagnosis easier.
- Await the macOS sign hook completion in release builds to ensure deterministic signing steps.
- Skip entitlements for the crashpad helper in macOS signing to avoid invalid signing metadata.
- Split macOS inherit entitlements handling from app entitlements for more stable release signing.
- Track and apply macOS signing entitlements consistently across release artifacts.
- Sign shared macOS runtime binaries during release packaging.
- Drop the unsigned macOS pkg artifact from release outputs to avoid distributing invalid installers.

## 0.5.15 - 2026-03-24

### Fixes
- Correct the custom macOS `electron-builder` sign-hook path so release builds launched from `apps/desktop` can load the signing script in CI instead of resolving a duplicated `apps/desktop/apps/desktop/...` path.

## 0.5.14 - 2026-03-24

### Fixes
- Route macOS desktop signing through a custom `electron-builder` sign hook that strips entitlements from `.pak` resource blobs while keeping the app-level hardened-runtime signing flow intact, and align the bundled shared Python runtime filters with the proven `Linkhoard` exclusions used during release packaging.

## 0.5.13 - 2026-03-24

### Fixes
- Pass absolute macOS entitlements paths to electron-builder in CI so codesign can always read the plist during release signing, matching the locally verified invocation.

## 0.5.12 - 2026-03-24

### Fixes
- Remove bundled `torch/include` and `torch/share` from the shared Python runtime so macOS release signing no longer exhausts the file descriptor limit while scanning the app bundle.

## 0.5.11 - 2026-03-24

### Fixes
- Patch the CI-installed `isbinaryfile` dependency before macOS packaging so electron-builder no longer crashes inside the upstream protobuf parser when signing the desktop app bundle.

## 0.5.10 - 2026-03-24

### Fixes
- Normalize bundled shared-Python symlinks, prune development-only runtime files, and exclude non-runtime test/docs resources so macOS desktop release builds can finish packaging without hitting the `isbinaryfile` crash during signing.

## 0.5.9 - 2026-03-24

### Fixes
- Avoid signing the bundled shared Python `site-packages` tree during macOS packaging so the desktop release workflow no longer crashes inside `isbinaryfile` while scanning runtime resources.

## 0.5.8 - 2026-03-24

### Fixes
- Update the desktop release workflow to use a temporary macOS keychain with `MAC_CERT_P12_BASE64`, enable notarization, avoid unsigned macOS ad-hoc signing, and fix the empty-array shell expansion that broke the latest release run.

## 0.5.7 - 2026-03-24

### Fixes
- Refactor macOS signing in CI to use a temporary keychain and enable notarization in the desktop release workflow.

## 0.5.6 - 2026-03-24

### Fixes
- Publish the desktop release from the commit that actually contains the macOS unsigned-build workflow fix, so tagged builds use the explicit `mac.identity=null` and `pkg.identity=null` overrides.

## 0.5.5 - 2026-03-24

### Fixes
- Desktop Release: when macOS signing certificates are absent in CI, explicitly disable both `mac.identity` and `pkg.identity` so arm64 release builds skip implicit ad-hoc signing and avoid the `isbinaryfile` crash during packaging.

## 0.5.4 - 2026-03-23

### Performance
- Reduce desktop IPC: skip snapshot emission while the main window is hidden or destroyed; refresh with a full snapshot when the window becomes visible again.
- Drop redundant waveform and transcript-preview messages from the sidecar status loop (already covered by the status payload).

## 0.5.3 - 2026-03-23

### Fixes
- Desktop Release: fix macOS `electron-builder` failure when Apple signing secrets are not configured by skipping empty `CSC_*` exports and using ad-hoc signing (`CSC_IDENTITY_AUTO_DISCOVERY=false`).

## 0.5.2 - 2026-03-23

### Fixes
- Run local `electron-builder` macOS/Windows scripts with `--publish never` to avoid unintended publishing during builds.

### Documentation
- Add `description` and `author` metadata to the desktop `package.json`.

## 0.5.1 - 2026-03-23

### Performance
- Cache tray snapshot state and refresh devices/history only when needed to reduce unnecessary work in the desktop main process.

## 0.5.0 - 2026-03-23

### Features
- Introduce the Electron desktop client with tray UI, sidecar runtime, electron-builder packaging, and tests; add a Bun workspace root, `packages/shared`, and a desktop-release GitHub Action in place of the previous installer workflow.
- Show VAD state (not started / speech / silence) in the status overview with a tone dot next to device and level metrics; refactor metrics into a stacked layout with tightened typography; add English and Chinese strings for VAD labels.

## 0.4.3 - 2026-03-19

### Fixes
- Extract shared installer script helpers and add macOS microphone usage metadata so recording permission prompts are declared correctly.

## 0.4.2 - 2026-03-19

### Fixes
- Improve macOS installer packaging by splitting CLI/Desktop packages and setting bundle version metadata.

## 0.4.1 - 2026-03-18

### CI
- Temporarily remove Ubuntu installer builds from GitHub Actions matrix.

## 0.4.0 - 2026-03-18

### Features
- Add desktop app runtime and skill-packaged workflow.

### Fixes
- Fix speech segment loss during segment rotation (by @jeasonzhang-eth).
- Fix macOS recorder stalls and improve default switching behavior.

## 0.3.6 - 2026-02-28

### Features
- Add `--version` / `-v` flags to show CLI version from both the root command and subcommands.

### Fixes
- Improve packaged runtime resilience by normalizing working directory and model/resource path resolution.

## 0.3.5 - 2026-02-23

### Features
- Add startup console UI with welcome panels and spinner statuses for record and transcribe commands

### Fixes
- Fix ASR packaging for frozen builds by including required data files for nagisa, qwen_asr, and silero_vad

## 0.3.4 - 2026-02-23

### CI
- Drop Ubuntu/Linux installer builds from CI pipeline.

## 0.3.3 - 2026-02-23

### CI
- Publish installer artifacts automatically on tagged releases.

## 0.3.2 - 2026-02-21

### Fixes
- Fix installer packaging for large PyInstaller builds by switching to onedir mode.

## 0.3.1 - 2026-02-21

### Documentation
- Make README default to English with separate Chinese translation.

## 0.3.0 - 2026-02-21

### Features
- Add cross-platform installer workflow for macOS, Linux, and Windows.
- Unify CLI into a single `eve` entrypoint and support `eve transcribe`.

### Documentation
- Add installer build and CI workflow usage in README.
- Update command examples from `eve-transcribe` to `eve transcribe`.

## 0.2.0 - 2026-02-20

### Features
- Add lossless FLAC archive support with `--audio-format` (`flac` default, `wav` optional).
- Support both WAV and FLAC input scanning in `eve-transcribe`.
- Improve recorder resilience and live recording UX.
- Improve ASR console output and history rendering with timestamps.

### Fixes
- Handle empty audio files safely in the offline transcription pipeline.

### Documentation
- Update README device examples to be cross-platform.
- Add OneDrive sync usage examples.
- Document FLAC as default archive format and refresh JSON examples.

### Refactor
- Rename project to `eve` and reorganize modules.

## 0.1.0 - 2026-02-01

### Features
- Initial release with continuous recording, VAD-based speech capture, and Qwen ASR transcription.
