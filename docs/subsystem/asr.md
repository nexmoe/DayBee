# 语音识别

本地离线转写。录音引擎切出人声后，由单一推理 worker 解码。模型文件按需下载，缓存可复用。

## 职责与边界

- `asr.Sherpa` 在 `apps/desktop/internal/asr/runtime.go` 创建离线识别器和 Silero VAD。版本由 `CheckVersion` 钉住：sherpa `Version`（1.13.8），ONNX Runtime `OnnxRuntimeVersion`（1.28.2）。不一致则拒绝创建。
- `asr.Worker` 在 `apps/desktop/internal/asr/worker.go` 只持有一个识别器。实时队列和批量队列分开。VAD 和写文件不等待推理。
- 模型清单在 `apps/desktop/internal/asrmodel/catalog.go` 的 `catalog`。`offlineConfig` 按目录名选架构。未知目录不加载。
- 下载、解压和缓存由 `assets.Manager` 负责，见 `apps/desktop/internal/assets/manager.go`。识别器不下载文件。
- 录音何时切段、何时入队，见 [recording.md](recording.md)。声纹只标注身份，不决定 PCM 边界，见 [voiceprint.md](voiceprint.md)。

当前七种模型；大小是官方压缩下载包大小，不是运行内存：

| ID | 目录 | 下载大小 | 必需文件 |
| --- | --- | --- | --- |
| `whisper-tiny` | `sherpa-onnx-whisper-tiny` | `EstimatedBytes`（116204861） | `tiny-encoder.int8.onnx`、`tiny-decoder.int8.onnx`、`tiny-tokens.txt` |
| `whisper-base` | `sherpa-onnx-whisper-base` | `EstimatedBytes`（207557382） | `base-encoder.int8.onnx`、`base-decoder.int8.onnx`、`base-tokens.txt` |
| `sensevoice-2025` | `sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2025-09-09` | `EstimatedBytes`（165783878） | `model.int8.onnx`、`tokens.txt` |
| `whisper-small` | `sherpa-onnx-whisper-small` | `EstimatedBytes`（639387718） | `small-encoder.int8.onnx`、`small-decoder.int8.onnx`、`small-tokens.txt` |
| `firered-asr2-ctc` | `sherpa-onnx-fire-red-asr2-ctc-zh_en-int8-2026-02-25` | `EstimatedBytes`（520516278） | `model.int8.onnx`、`tokens.txt` |
| `qwen3-0.6b` | `sherpa-onnx-qwen3-asr-0.6B-int8-2026-03-25` | `EstimatedBytes`（878702423） | `conv_frontend.onnx`、`encoder.int8.onnx`、`decoder.int8.onnx`、tokenizer 三件 |
| `firered-asr2-aed` | `sherpa-onnx-fire-red-asr2-zh_en-int8-2026-02-26` | `EstimatedBytes`（838589068） | `encoder.int8.onnx`、`decoder.int8.onnx`、`tokens.txt` |

`protocol.DefaultASRModel` 是 `ASRWhisperTiny`。首次启动、旧配置缺少模型字段，以及无效选择都回到 Tiny，优先降低初始化和基本转写的资源开销。已有合法模型选择在迁移、保存和重启时保留，即使选中的是 Qwen3 或 FireRed。模型列表把 Tiny 放在首位。`ValidASRModel` 只接受表中的 ID。

## 权威来源

- 可选模型 ID 只在 `apps/desktop/internal/protocol/asr_models.go` 的 `ASRModel` 常量。
- 磁盘布局和就绪条件只在 `asrmodel.Model`。`Ready` 要求清单里每个文件是非空普通文件。
- 当前选中 ID 存在 `assets.Manager.selected`。`ASRPath` 按这个 ID 找已就绪目录，否则指向当前 `models/` 根下的目标目录。
- 快照 `ASRModelsSnapshot` 列出每个模型的压缩下载大小和就绪状态。`EstimatedBytes` 不表示运行内存或解压后的磁盘占用。`DownloadingModel` 只在 `EnsureASRModel` 持锁下载期间非空。下载失败不改选中 ID。

`PrepareASRModel` 只下载。激活走设置保存和 `Engine.Apply`。录音中可以下载，不切换当前模型。

## 运行路径

### 模型就绪

`Engine.prepare` 调用 `assets.Manager.Ensure`。VAD 和 ASR 各有容量为 1 的门。VAD 下载不等待 ASR 下载。取消也会打断等门。未显式选择时，ASR 只准备 Tiny，不因大模型已缓存而自动切换。

VAD 文件是 `silero_vad.onnx`。ASR 包是 `releaseRoot` 下的 `<目录>.tar.bz2`。`releaseRoot` 为 `https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/`。

`download` 最多 3 轮。每轮按 `githubDownloadURLs` 依次尝试源站和镜像。`DAYBEE_DOWNLOAD_MIRROR=cn`，或 `zh_CN` 区域，或 `Asia/Shanghai`、`Asia/Chongqing`、`PRC`，或本地时区名为 `CST` 且偏移 8 小时，则先试 `https://ghfast.top/` 和 `https://gh-proxy.com/`。否则源站在前。非 GitHub 主机不走镜像。

单次传输写临时文件。`idleTimeout`（15 秒）内没有响应头或新字节就放弃该源。HTTP 非 200、HTML 正文、空文件或长度不符都失败。进度按 `ContentLength` 计算，至少间隔 100 毫秒发布一次。

解压限制：路径必须是本地相对路径，拒绝链接和其他条目，常规文件总大小不超过 8 GiB。解出目录通过 `Ready` 后才 `Rename` 到目标。旧的不完整目录先挪走，替换失败再挪回。

官方 Whisper 包同时包含 FP32 和 INT8 权重。`apps/desktop/internal/assets/whisper.go` 的 `pruneWhisperFP32` 在暂存目录通过完整性检查后，只清理不使用的 FP32 encoder/decoder，保留 INT8、tokens 和文档，再发布目录。不修改已存在的缓存；完整压缩包仍需下载，因此下载量与最终 INT8 文件体积不同。

查找顺序是当前 `models/`，再是构造时传入的旧 `models/`。命中即复用，不搬文件。

### 识别器

`NewModelWorker` 起一个推理 goroutine。只有它调用工厂并持有识别器。

- `SubmitLive` 写入容量 `6` 的实时队列。满了返回 false，不计 pending。
- `SubmitBatch` 写入容量 `1` 的批量队列。满了每 `batchRetry`（10 毫秒）重试，直到 worker、任务或调用方取消。
- 实时任务优先。批量任务开始前再排空一次实时队列。
- 任务带 `ModelPath`。空路径才读当前 `key`。已加载 key 变化时先 `Close` 旧识别器，再按新路径创建。
- `ReleaseWhenIdle` 在 pending 为 0 时卸载。请求里记下的 key 必须仍是已加载 key，迟到的旧请求不能卸掉新模型。
- 队列清空后连续 `modelIdleTimeout`（2 分钟）没有新任务，worker 卸载识别器及其原生推理缓存。下一条语音按任务的 `ModelPath` 重新加载；普通短暂停顿复用模型，新任务完成后重新计时。不会打断解码或丢弃排队任务。
- `Close` 取消上下文，丢弃队列里的任务，并以 `context.Canceled` 调用各自的 `Complete`。

`Job.Spans` 在持有识别器时串行解码多段。整单只入队一次。`Prepare` 不使用。`Decode` 返回后若任务或 worker 已取消，结果丢弃。

`offlineRecognizer.Decode` 对空样本返回空文本。识别器已关闭、建流失败或无结果返回错误。文本做 `TrimSpace`。

采样率是 `audio.SampleRate`（16000）。特征维默认 80。Whisper 使用对应型号的 INT8 encoder/decoder 和 tokens；空 `Language` 自动识别语言，`Task` 为 `transcribe`，`TailPaddings` 为 -1（原生默认补齐）。Qwen3 为 128，并固定 `Seed`（42）。SenseVoice 语言 `auto`，开启逆文本正则。线程数是 `infer.Threads`：`NumCPU/4`，至少 1，至多 2。Provider 为 `cpu`。

### VAD

`NewVAD` 使用 Silero。`Threshold`（0.5），`WindowSize` 等于 `audio.WindowSize`（512），`MinSilenceDuration`（0.5），`MinSpeechDuration`（0.25），`MaxSpeechDuration`（15），线程数 1，Provider `cpu`。检测器缓冲 60 段。

`Speech.Start` 是上次 `Reset` 以来的采样偏移，含静音。`Pop` 把该偏移交给录音引擎。实时路径在入队前用它冻结起止时间，见 `appendSpeech`。

### 实时与批量

`Engine.New` 用 `asr.NewModelWorker` 绑定 `assets.ASRPath` 和 `runtime.NewRecognizer`。开始录音时 `prepare` 在未关闭转写时同时准备 VAD 和 ASR。模型切换调用 `ReleaseWhenIdle`，再 `SelectASRModel`。停止、休眠、安全停录和关闭转写时同样请求卸载空闲识别器；已有任务仍完成后再卸载。正在转写且未在录音时，拒绝切换模型。

实时：采集回调只入队 PCM。`consume` 按 `WindowSize`（512）喂 VAD，静音窗口也喂。`appendSpeech` 先写 WAV，再提交 `asr.Job`。有 span 匹配器且档案存在时走 `spanJob`，整段只 `Decode` 一次。否则走 `plainJob`。实时队列满则保留音频，写入诊断 `Live ASR queue full; speech was archived.`，不丢文件。

批量：`Transcribe` 先 `EnsureASRModel`，再按文件处理。已录入声纹且是 16 kHz 麦克风文件时，`transcribeLabeled` 按采集区间或 VAD 切段，逐段 `SubmitBatch`。系统音、其他采样率、未录入，整文件提交一次。每段任务在入队时钉住 `ModelPath`。取消会等已经开始的原生解码结束。

解码失败保留原录音。实时路径把错误写入段诊断。批量路径中止并返回该错误。

## 约束与失败行为

- 版本、ONNX Runtime 或模型目录不匹配时不创建识别器。该任务以错误完成。
- worker 内部 panic 变成 `ASR failed` 错误。`Complete` 自身 panic 只记日志。
- 下载失败保留已有缓存和当前选中模型。不完整包不替换目录。
- 一次下载的 HTTP 客户端超时是 1 小时。无进度则由 `idleTimeout`（15 秒）切断，再试下一个源。
- 实时队列不阻塞采集。容量 `6`，超出只丢推理，不丢已写入的 WAV。
- 批量队列容量 `1`。后一个文件等待，不并行加载第二份模型。
- `DisableASR` 不提交推理。已在跑的批量任务结束后调用 `ReleaseWhenIdle`。
- 旧缓存目录只读复用。就绪判断不比较校验和，只看清单文件非空。

## 验证

在 `apps/desktop` 下：

```text
go test ./internal/asr/ -count=1 -timeout 120s -run 'TestOfflineConfigurationsUseCorrectModelArchitecture|TestLivePriorityBoundedQueueAndNativeRelease|TestModelChangeClosesPreviousRecognizerBeforeNextDecode|TestDelayedReleaseDoesNotUnloadNewModel|TestSpansDecodeSeriallyWithoutRequeue|TestSpanDecodeDropsResultWhenContextEndsDuringDecode|TestQueuedJobsKeepTheirModelWhileSelectionChanges'
go test ./internal/assets/ -count=1 -timeout 60s -run 'TestCachesAreReusedWithoutMovingFiles|TestVADPreparationDoesNotWaitForASRAndGateIsCancelable|TestEveryModelReusesItsOwnLegacyCache|TestDownloadCoalescesAndDoesNotSelectBeforeSuccess|TestIncompleteArchivePreservesSelectionAndPreviousFiles|TestDownloadSourceOrdering'
go test ./internal/desktop/ -count=1 -timeout 60s -run 'TestPrepareASRModel'
```

原生模型测试需要本机缓存，不作为默认门禁：`TestNativeVersions`、`TestNativeInference`、`TestNativeSelectableModels`。

轻量初始化回归：`go test ./internal/config/ ./internal/assets/ -count=1 -run 'TestModelSelection|TestLegacyExplicitModelSelection|TestDefaultModelPreparation'`。覆盖新默认、缺字段和非法值、旧用户选择，以及只下载 Tiny 并清理不用的 FP32 权重。真实录音流水线可把 `DAYBEE_TEST_MODEL_DIR` 指向缓存的 Whisper Tiny，运行 `TestNativeRecordingPipeline`，验证真实 VAD、实时与批量转写、WAV/FLAC 归档与模型元数据；采集输入为固定公开语音，不打开麦克风。

资源生命周期回归：`go test ./internal/asr/ -count=1 -run 'TestIdle'`，使用虚拟时间覆盖静音卸载、短暂停顿复用、重新加载和排队任务保护。

下载超时回归：`go test ./internal/assets/ -race -count=20 -run 'TestDownloadFallsBackWhenHeadersOrBodyStall'`。使用虚拟时间分别阻塞响应头和正文，要求恰在空闲期限切到第一个健康镜像，且总共只请求两次，避免 CI 调度及磁盘耗时影响超时断言。

真实模型空闲释放的可选验证为 `TestNativeIdleModelRelease`，需要 macOS、`DAYBEE_TEST_IDLE_MODEL_DIR`（已缓存模型目录）和 `DAYBEE_TEST_IDLE_WAV`（公开测试音频）；可用 `DAYBEE_TEST_IDLE_REPORT` 保存 footprint 测量。它等待真实的 2 分钟超时，检查卸载后能重载且输出一致，不访问麦克风。测量与命令见 [资源验证](../verification/resource-usage.md)。

这些测试读 `apps/desktop/internal/asr/native_test.go` 里的环境变量，未设置就跳过：`DAYBEE_TEST_MODEL_DIR`（Qwen3 模型目录）、`DAYBEE_TEST_AUDIO_DIR`（固定回归 WAV）、`DAYBEE_TEST_VAD_PATH`（Silero VAD）、可选 `DAYBEE_TEST_ASR_REPORT`（报告输出路径）。

```bash
DAYBEE_TEST_MODEL_DIR=/absolute/model \
DAYBEE_TEST_AUDIO_DIR=/absolute/fixtures \
DAYBEE_TEST_VAD_PATH=/absolute/silero_vad.onnx \
bun run --cwd apps/desktop test:go -run 'TestNative' -v
```

链接：`bun run check:docs`
