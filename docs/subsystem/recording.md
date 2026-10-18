# 录音

`recorder.Engine` 采集麦克风，可选采集系统声音，按语音活动切段，写入音频和元数据，并把语音交给共享的 ASR 队列。它不实现声纹模型，也不扫描其他应用的麦克风占用。

## 职责与边界

- 拥有采集循环、分段、历史扫描、设备轮询、自动切换和睡眠恢复。入口是 `recorder.Engine`，见 `apps/desktop/internal/recorder/engine.go`。
- 麦克风和系统声音各有一条 `audioStream`：独立的 VAD、偏移和归档。两条流只共享一个有界 ASR worker。见 `apps/desktop/internal/recorder/streams.go` 的 `audioStream`。
- 系统声音开关来自 `RecordingSettings.RecordSystemAudio`。macOS 才允许打开。见 `apps/desktop/internal/protocol/settings.go` 的 `SupportsSystemAudio`。采集实现见 [audio.md](audio.md)。
- 回声消除开关来自 `RecordingSettings.EchoCancellation`。采集失败会停止录音，不会降级成未处理音频。见 `apps/desktop/internal/recorder/engine.go` 的 `startCapture`。
- 已录入声纹且未关闭 ASR 时，系统播放只作为声纹参考进入环形缓冲，不写入麦克风文件。见 `apps/desktop/internal/recorder/reference_capture.go` 的 `wantsSystemReference`。声纹判定见 [voiceprint.md](voiceprint.md)。
- 其他应用占用麦克风的记录不在这里。见 [microphone-activity.md](microphone-activity.md)。
- 桌面服务把开始、停止、设置和补转写交给引擎。睡眠、唤醒、解锁由 `DesktopService.powerLoop` 调用 `Engine.Suspend` 和 `Engine.Resume`。见 `apps/desktop/internal/desktop/app.go`。

## 权威来源

- 运行中的采集状态是 `Engine.status`，类型为 `protocol.RecorderStatusSnapshot`。公开读取走 `Engine.Status`。
- 用户意图是 `Engine.intent`。只有用户已经开始、且没有手动停止、退出或不可恢复错误的录音才会自动恢复。
- 已保存录音的权威文件是输出目录里的音频、`.recording.json` 和可选的 `.json`。`recorder.History` 每次从磁盘重建列表，不维护第二份索引。
- 采集时间区间写在 `.recording.json` 的 `captured_ranges`。不要用文件时长推断采集时间。见 `apps/desktop/internal/recorder/segment.go` 的 `writeRecordingMetadata`。

## 运行路径

### 开始与消费

`Engine.Start` 先准备 VAD 和 ASR 资源，再在音频循环里执行 `start`。`start` 为麦克风创建 VAD，按设置决定是否创建系统流，然后 `startCapture`。

原生回调把 `audio.Packet` 放进容量 64 的 `packets`。队列满时发送 `ErrPCMOverflow`，录音安全停止，且不再自动恢复。见 `apps/desktop/internal/recorder/audio.go` 的 `startCapture` 和 `safeStop`。

`consume` 按 `packet.Source` 选择流。系统包先进入声纹参考；没有系统归档流时就丢弃。麦克风包更新 RMS、电平和频谱。样本积到 `WindowSize`（512）后送入 VAD。静音窗口也送入，避免后续语音时间前移。

`Engine.SetWaveformEnabled` 只控制 30ms 定时的频谱计算与波形回调。桌面窗口全部隐藏或最小化时关闭，有窗口显示时恢复；采集、VAD、状态电平、归档和转写继续。停止或休眠后，`stopAndWait` 等待归档完成并请求 ASR worker 在任务清空后释放识别器；安全停录也会请求释放。

VAD 弹出的语音由 `appendSpeech` 写入 WAV。第一条语音才创建文件。目录是 `<OutputDir>/<YYYYMMDD>/`，权限 `0700`。文件名是 `daybee_<YYYYMMDD_HHMMSS>_<microphone|system>.wav`。同名 wav、flac 或 json 已存在时，文件名追加 Unix 纳秒。

### 分段与归档

分段时长是 `SegmentMinutes`。配置小于等于 0 时回到 60。见 `apps/desktop/internal/config/store.go` 的 `normalize`。音频循环每 100ms 检查一次，到期调用 `rotate`，再 `newSegment`。

设备变化、回声消除开关、系统声音开关、ASR 开关、模型、格式或输出目录变化，也会先结束当前分段再重开。见 `apps/desktop/internal/recorder/engine.go` 的 `Apply`。

`finishSegment` 关闭 WAV 后，在后台等待该段的 ASR 任务，然后 `archive`。格式为 `flac` 时调用 `assets.Transcode`。转换失败保留 WAV。成功则删除 WAV，并把后续元数据指向 flac。

`.recording.json` 在录音过程中由 `recordingSidecar` 异步刷新，停止时再写一次。字段包括 `audio_path`、`segment_start_time`、`recording_session_id`、`captured_ranges`、`input_device`、`audio_source`。麦克风还会写入 `capture_device_id` 和 `echo_cancellation`。

转写完成时，`voice_job.go` 的 `saveTranscript` 先持久化采集元数据，再持久化文稿，最后通知历史刷新。快速模型不必等待异步 sidecar 恰好先完成，历史读者收到转写通知时即可读取对应采集时间区间。

未关闭 ASR 时再写同名 `.json`：文本、`transcript_chunks`、模型名和 `backend`（`sherpa-onnx`）。实时队列满时，语音仍归档，并在诊断里记下队列已满。实时队列容量是 6。见 `apps/desktop/internal/asr/worker.go` 的 `NewModelWorker`。

同一段的两路文件共用 `recording_session_id`，来源分别是 `microphone` 和 `system`。

### 历史与补转写

`recorder.History` 遍历输出根目录，收集 `.wav` 和 `.flac`，再用同名 `.json` 或路径指向的 JSON 补状态、文本、来源和采集区间。没有 `audio_source` 的记录按麦克风处理。见 `apps/desktop/internal/protocol/settings.go` 的 `NormalizeAudioSource`。

`DesktopService.refreshHistory` 除当前输出目录外，还扫描文档目录下的 `DayBee`，以及未隔离运行时的 `RecBee` 和 `Eve Recorder`。见 `apps/desktop/internal/desktop/app.go`。

`Engine.Transcribe` 补转写一个目录。正在写入的活动文件跳过。已有可标注结果时走标注路径；否则整文件解码，写入不含逐句时间的 JSON，并保留 sidecar 或旧 JSON 里的会话、设备和来源。同一时间只允许一批。见 `apps/desktop/internal/recorder/batch.go`。

### 设备与恢复

`pollDevices` 每 `devicePollInterval`（10s）枚举输入设备。当前设备消失时切到 `default`，不改已保存的设备偏好。见 `apps/desktop/internal/recorder/devices.go`。

自动切换默认开启。当前 RMS 低于 `autoSwitchQuietRMS`（0.01）并持续 `autoSwitchQuiet`（10s）后，才探测候选。候选排除 `ExcludeDeviceKeywords`，默认 `iphone,continuity`，每轮只探测一个，最多保留两个。判定阈值在 `audio.SwitchPolicy.Consider`：候选 RMS 低于 0.006，或不到当前的 1.8 倍，不切换；确认次数默认 `AutoSwitchConfirmations`（2）；成功切换后 8 秒内不再切换。回声消除开启时，当前路和候选路都用原始 `Probe`，探测 250ms，不停止正在录音的处理链。

睡眠调用 `Suspend`：停止采集，若仍有录音意图，状态改为恢复中，文案是 “Recording paused while the computer sleeps.” 唤醒或解锁调用 `Resume`。见 `apps/desktop/internal/desktop/app.go` 的 `powerLoop`。

采集中断若是 `audio.ErrDisconnected` 或 `audio.ErrInterrupted`，先结束当前分段，再 `recoverLocked`。其他错误清除意图，不再自动开始。

恢复失败时，`recoveryDelay` 从 1 秒加倍，上限 30 秒。实际尝试由 10 秒设备轮询触发，到达 `recoveryAt` 才执行。一次成功后至少再等 3 秒才允许下一次自动打开。所选设备已不在列表中时，这次恢复临时用 `default`。用户停止、关闭和 `safeStop` 会清掉意图。界面在 `recovering` 时显示 “正在恢复录音…”，并保留停止。见 `apps/desktop/src/renderer/src/lib/i18n.ts`。

## 约束与失败行为

- 包队列、原生 SPSC 缓冲和 WAV 体积都有上限。溢出或转换失败停止录音并报错，不丢样本继续。原生缓冲见 [audio.md](audio.md)。
- WAV 超过 32 位块大小上限时，`WavWriter.Append` 返回错误。见 `apps/desktop/internal/audio/wav.go`。
- FLAC 转换失败留下 WAV 和诊断，不把不完整的 flac 当成结果。
- 模型切换若发生在非录音但仍忙碌时会被拒绝。录音中切换失败会停掉采集并把内存设置滚回旧值。见 `Engine.Apply`。
- 恢复不重启进程，也不阻止系统睡眠。
- 关闭 ASR 仍写 `.recording.json`。补转写之后保留来源和采集区间。
- 日志写到应用数据目录的 `logs/main.log`。生产数据根是 `DayBee`，开发是 `DayBee Development`。见 `apps/desktop/internal/desktop/data_paths.go` 的 `dataPaths`。

## 验证

在 `apps/desktop` 下执行。`node scripts/desktop.mjs test` 会把参数追加到 `./...` 前面，不能用来只跑一个包。

```bash
cd apps/desktop
go test -count=1 ./internal/recorder/ -run 'TestUnlock|TestWake|TestDisconnected|TestManualStop|TestRecovery|TestTerminalCapture|TestRepeatedInterruptions|TestSystemInterruption|TestHistory|TestAutoSwitch|TestFailedDeviceChange|TestLiveTranscript'
```

界面恢复状态：

```bash
cd apps/desktop
bunx playwright test e2e/recording-recovery.spec.ts
```

链接：`bun run check:docs`

`TestLiveModelSwitchKeepsCaptureAndQueuedSpeech` 在推理 gate 保持关闭时要求模型切换返回，再检查采集连续性、旧队列及两种模型的归档。5 秒看门狗用于发现死锁，给 Windows 文件同步和 race 检测留出余量，不是产品的切换延迟指标。
