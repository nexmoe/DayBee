# Voiceprint annotation verification

Date: 2026-10-11. Platform: macOS arm64. Scope: local speaker enrollment and automatic owner annotations in RecBee.

## Behavior

The Home voiceprint card remains visible after canceling, skipping, successful enrollment, navigation and relaunch. Cancel stops only the current enrollment and returns the card to its starting state; the existing voiceprint is retained. An enrolled card shows automatic-use status and a record-again action. The first-use setup decision can be skipped persistently. Enrollment requires two five-second speech samples and a separate three-second verification sample. Silence cannot complete a sample. Any saved valid profile is used automatically, including after relaunch. The annotation switch and its configuration field have been removed; a legacy `voiceprintEnabled: false` preference does not disable matching. Settings support initialization, recording again and clearing the profile.

All audio and transcript text remain available. Only matching microphone chunks receive `voiceIdentity: "me"`; unknown speakers, short samples and matching errors retain text without an owner identity in storage. The UI repeats Me before each matching sentence and System audio before playback sentences. Unconfirmed speech retains its text with no identity badge, per the user's preference. System audio retains its own source and never receives an owner annotation. Manual microphone re-transcription checks individual speech pieces and retains every decoded piece. Old history is not automatically rewritten.

The microphone capture callback only queues PCM. Speech detection and enrollment inference run on the enrollment goroutine. Live speaker matching runs after successful ASR, outside the capture callback and recorder engine loop. Profile access and native extractor lifetime are serialized. Profiles are atomically written with mode `0600`; enrollment PCM is retained only in memory.

## Results

| Check | Result |
| --- | --- |
| Frontend unit tests | 41 passed, including identity boundaries, long app scenes, legacy transcripts and system exclusion |
| TypeScript | Node and renderer checks passed |
| Go race suite | Passed across all packages, including enrollment cancel/silence/verification failure, recording restoration, settings-write rollback, profile persistence, retired-switch migration, model download coordination, automatic live and batch annotation |
| Native speaker verification | Passed with the official 192-dimensional CAMPPlus model and offline 16 kHz fixtures; same-speaker speech matched and a contrasting voice did not |
| Native enrollment replay | Passed using real Silero detection and CAMPPlus inference with offline PCM replay, including sample advancement, validation, profile save and capture cleanup |
| Playwright | Current targeted checks: all seven voiceprint cases passed, covering persistent setup, cancellation, automatic use, matching-only sentence badges, retained text and search context in both windows and languages. Both 1,000-entry virtual timeline cases passed three consecutive runs after removing redundant width-driven height resets. The earlier broader 45-case baseline is retained as prior evidence. Playwright now launches its native backend with EVE_TEST_BACKGROUND=1 and asserts the window remains hidden and unfocused; native smoke tests also default to background mode, with visible capture requiring --interactive |
| Native two-window smoke | Passed in background mode with actual WebViews, shortcut, singleton main window, shared settings/recording, immediate transcripts, close/reopen/reload; the main test window remained hidden and unfocused |
| Production build and package | Passed; macOS arm64 app and DMG, development code signature, native library loading, licenses, legacy audio and resampling |
| Source constraints | No source file exceeds 500 lines; whitespace checks passed |

Browser captures are stored in `apps/desktop/e2e/results/voiceprint/` (`intro.png`, `sample.png`, `canceled.png`, `settings.png`, `identities.png`). These show the guide, sample progress, the retained card after cancellation, automatic-use settings without an annotation switch, and identity/source separation.

The native UI and browser harnesses use isolated mock recording engines and a deterministic enrollment controller. The separate native inference checks use real models with offline fixtures. Neither establishes live accuracy for this user's voice or video playback environment. Windows and Intel Mac hardware were not tested.

## Local installation

The updated `/Applications/RecBee.app` matches the verified production build (SHA-256 `ce5a18af8486a211bef36952a45e2c8b9d3414408138e362c3981b085827c0ea`). The previous bundle and installation manifest are backed up in `/var/folders/lm/ks505cxx0_s218_907yf_pgm0000gn/T/recbee-sentence-ui-jp8gdrjz`. All 136 recording files present immediately before this replacement retained their hashes, and the saved voiceprint hash is unchanged. No old recording or voiceprint was rewritten.

The user had previously granted microphone access, and that earlier installed process was verified recording with its timer advancing. After this latest production replacement, the app showed idle and a microphone permission action; recording on the new production binary was not established. No authorization database was edited. Live recognition accuracy for the user's voice and video playback remains unmeasured.

At the user's subsequent request, the local development app is now running with its native Home window loaded from `http://127.0.0.1:5174/?window=main`, Vite hot reload and the isolated development data directory. Port 5173 belongs to another project and was left running. The visible development card has one short instruction and small buttons. Production microphone authorization was not reset or changed during this step.

See [the user flow and implementation contract](../subsystem/voiceprint.md).
