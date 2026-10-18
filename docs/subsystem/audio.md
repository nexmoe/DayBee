# 音频

`audio` 打开麦克风和系统播放，输出 16 kHz 单声道 float32。`codec` 在不调用外部进程的情况下读写 WAV 和 FLAC。录音如何切段、恢复和归档见 [recording.md](recording.md)。

## 职责与边界

- `audio.Capture` 枚举输入设备、开始采集、停止、关闭，并对单个设备做短时音量探测。实现是 `audio.NativeCapture`，见 `apps/desktop/internal/audio/capture.go` 和 `capture_malgo.go`。
- 麦克风在回声消除关闭时走 miniaudio。开启时，macOS 走 Voice Processing；其他平台 `newVoiceCapture` 直接返回不可用。见 `apps/desktop/internal/audio/capture_voice_other.go`。
- 系统声音只在 `CaptureOptions.SystemAudio` 或 `SystemReference` 为真时打开，且只在 macOS 实现。其他平台返回 “System audio recording is currently supported on macOS only”。见 `apps/desktop/internal/audio/capture_system_other.go`。
- `SystemAudio` 失败会中止整次采集。`SystemReference` 失败只通知一次，不中止麦克风。见 `NativeCapture.attachSystem`。
- `codec` 负责把已有 WAV 编成 FLAC，以及把 WAV 或 FLAC 解成 16 kHz 单声道供 ASR 使用。它不采集，也不决定文件名。
- 频谱、RMS 和波形分箱由录音循环在消费包时计算，不在原生回调里计算。见 `apps/desktop/internal/audio/spectrum.go`。

`Spectrum.Frame` 使用 256 点 radix-2 FFT，保留 Blackman 窗、线性幅度平滑（0.8）、128 个频率分箱和 -100 到 -30 dB 的字节映射。频谱只在可见界面需要波形时计算；控制权在录音引擎，见 [recording.md](recording.md)。

## 权威来源

- 设备列表来自 miniaudio 的一次枚举。ID 形如 `native:` 加设备 ID。`default` 表示跟随系统默认输入。见 `NativeCapture.Devices` 和 `initialize`。
- 到达 Go 的样本格式由 `SampleRate`（16000）和 `WindowSize`（512）约束。回声消除和系统采集在原生侧转到同一格式后才交给 Go。
- 系统播放参考的样本时钟在 `recorder.echoReference`，不是 `audio` 包里的硬件时间戳。`Packet.CapturedAt` 只是入队时间。见 `apps/desktop/internal/audio/capture.go` 的 `Packet`。

## 运行路径

### 麦克风

`NativeCapture.start` 先停掉上一次设备和系统句柄，再 `initialize`。未开启回声消除时，设备必须给出 16 kHz、单声道、float32，否则关闭设备并返回错误。

开启回声消除时，`newVoiceCapture` 把 miniaudio 设备 ID 里的 UID 交给 `daybeeVoiceOpen`。`prepareCapture` 启用 Voice Processing，在输入总线 1 选择设备，关闭自动增益，并在 macOS 14 及以上把其他音频压低设为关闭且取系统允许的最小级别。见 `apps/desktop/internal/audio/capture_voice_darwin.m`。

处理图要求客户端输入输出格式一致。输入节点接到主混音器，混音器音量为 0，避免监听回放。代码只向 Voice Processing 要一个声道，避免把带参考声道的聚合设备混成单声道。`AVAudioConverter` 跨回调保留重采样状态，输出 16 kHz 单声道 float32。

原生回调只写入 `DayBeeVoiceBuffer`。容量是 `DAYBEE_VOICE_CAPACITY`（32768）个 float。满了记溢出，不丢前面的样本。Go 侧每 10ms 轮询 `daybeeVoiceRead`。见 `apps/desktop/internal/audio/voice_buffer.h`。

配置变化通知会把缓冲标成断开。录音引擎据此重开。停止时先停原生引擎，等 Go 轮询退出，再释放对象。

`voiceActivityGate` 和 `detectSpeech` 只在测试里打开。生产的 `newVoiceCapture` 不启用。门控保留语音前后 `voiceGatePadding`（`SampleRate / 5`，即 200ms），不可用时保留全部样本。见 `apps/desktop/internal/audio/voice_gate.go`。

### 系统声音

macOS 14.2 及以上走 Core Audio process tap。`daybeeSystemOpen` 在 14.2 可用时直接调用 `daybeeSystemTapOpen`。tap 是排除本进程的全局立体声 tap，经私有 aggregate device 读取。14.x 的 tap 不自行重采样，`AudioConverter` 转为 16 kHz 单声道 float32。默认输出设备变化或 aggregate 失效时上报中断。见 `apps/desktop/internal/audio/capture_system_tap_darwin.m` 的 `prepareTap`。

macOS 13.3 到 14.1 走 ScreenCaptureKit，只注册音频输出，采样率 16000，单声道，并排除本进程音频。macOS 15 及以上额外关闭麦克风采集，避免和 Voice Processing 抢麦克风。见 `apps/desktop/internal/audio/capture_system_darwin.m` 的 `prepareSystemCapture`。

部署目标是 macOS 13.3。见 `apps/desktop/scripts/desktop.mjs`。

系统流失败码由 `systemFailureError` 翻译：2 是缓冲溢出，3 是转换失败，4 是用户停止，其余是 `ErrInterrupted`。溢出、转换失败和用户停止不会被录音引擎当成可恢复中断。见 `apps/desktop/internal/audio/system_failure.go` 和 `apps/desktop/internal/recorder/audio.go` 的 `loop`。

设置页打开的系统设置地址由 `SystemAudioSettingsURL` 决定。14.2 及以上是系统音频录制，更旧系统是屏幕与系统音频录制。

### 系统播放参考

归档用的系统包和仅供声纹使用的参考包都从同一条系统采集进来。参考不要求 `RecordSystemAudio`。录音侧把样本放进约 30 秒的环形缓冲，常量 `echoKeep`（`30 * SampleRate`）。缺口、过期或只覆盖一部分的切片会补零，并标成不完整。见 `apps/desktop/internal/recorder/echo_reference.go`。

参考时钟用入队时间。相邻包相差超过 `referenceClockSlack`（150ms）时留下空洞，而不是连成一段。见 `apps/desktop/internal/recorder/reference_capture.go` 的 `feedSystemReference`。

参考启动失败或运行中失败时，相位变为不可用，并清掉缓冲。`Engine.RetryVoiceprintReference` 只在正在录音、需要参考且参考尚未可用时重开采集，保留当前文件和 VAD 时钟。

### 编解码

录音先写成 16 kHz、单声道、16-bit PCM WAV。`WavWriter` 以 `O_EXCL` 创建，权限 `0600`。见 `apps/desktop/internal/audio/wav.go`。

`codec.EncodeFLAC` 只接受 8、16、24 bit WAV，流式写入临时文件，完成后 `rename`。取消或失败不会留下目标 flac。编码器版本常量是 `FLACVersion`（`1.0.14`）。见 `apps/desktop/internal/codec/flac.go`。

`codec.ReadMono` 解码 WAV 或 FLAC。多声道按样本平均。已是 16 kHz 的文件跳过滤波。其他采样率用 Blackman 窗 sinc，截止在目标奈奎斯特的 0.94 倍。FLAC 超过 24 bit 会拒绝。帧格式变化、样本数超出或 MD5 不符都会失败。见 `apps/desktop/internal/codec/decode.go` 和 `resample.go`。

## 约束与失败行为

- 回声消除启动失败时，错误文本要求检查麦克风或在输入设置里关闭回声消除。调用方停止录音，不回退到原始采集。
- 原生缓冲溢出和转换失败停止录音。这是故意的，满缓冲不是丢样本。
- `Probe` 只打开 250ms 的原始输入，回调只往容量 8 的队列里放，满了丢弃探测样本。它不代表录音链路会丢样本。
- 自动切换的音量阈值、确认次数和冷却在 `audio.SwitchPolicy`。录音页描述它如何被调用。
- WAV 读取支持 8、16、24、32 bit 整数 PCM，以及不降低有效位的 extensible PCM。float WAV 不支持。
- 系统音频权限拒绝、静音数据和 14.2、14.3 tap 的长期稳定性没有在代码里单独证明。权限探测失败会返回错误；探测没拦住时，系统仍可能送来静音。

## 验证

在 `apps/desktop` 下执行。下面的测试不打开麦克风，也不播放音频。

```bash
cd apps/desktop
go test -count=1 ./internal/audio/ ./internal/codec/
```

带硬件的测试需要单独打开环境变量，不要和上面的命令混跑。麦克风生命周期、外放对照和现场语音分别是 `TestVoiceCaptureHardwareLifecycle`、`TestVoiceCaptureSpeakerComparison`、`TestVoiceCapturePreservesNearSpeech`。系统采集硬件测试会请求权限并播放文件。

链接：`bun run check:docs`
