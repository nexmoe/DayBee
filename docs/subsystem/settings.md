# 设置

设置是一份 JSON 文档。Go 在写入前补齐缺字段并拒绝非法值。渲染器拿到的是规范化之后的结果。

## 职责与边界

- 结构在 `apps/desktop/internal/protocol/settings.go`。`AppSettings` 分成 `Desktop` 和 `Recording`。
- 磁盘读写在 `apps/desktop/internal/config/store.go` 的 `Store`。原子替换在 `atomic.go` 的 `WriteJSON`。
- 壳的保存入口是 `apps/desktop/internal/desktop/service.go` 的 `SaveSettings`。它先写盘，再交给引擎 `Apply`。
- 渲染器不维护第二份设置文件。它通过生成的 `saveSettings` 把整份 `AppSettings` 送回去。见 [renderer.md](renderer.md)。
- 录音引擎如何使用设备、分段和转写开关，不在这页。

## 权威来源

- 文件是数据根目录下的 `desktop-settings.json`。根目录规则见 [desktop-shell.md](desktop-shell.md)。
- 文档含 `settings`、`windowState`、`nativeDevices`。当前默认把 `nativeDevices` 写成 true。
- 原生默认值由 `protocol.DefaultSettings` 按本机平台定义。`cmd/contracts` 用 `protocol.DefaultSettingsForPlatform("recordings", "darwin")` 生成 `packages/shared/src/defaults.ts` 的 `DEFAULT_SETTINGS`，保证各平台得到相同的共享文件。共享值用于页面初始化，原生 `Bootstrap` 和 Store 规范化仍提供实际平台的设置；Windows 回声消除默认关闭。真正打开 Store 时，输出目录是文稿目录下的 `DayBee`。

当前默认：

- `HideWindowOnClose`（true）。规范化时总是写回 true。
- `Language`（`LanguageSystem`）、`LaunchAtLogin`（true）、`StartRecordingOnLaunch`（true）、`Theme`（`ThemeSystem`）、`VoiceprintSetup`（`VoiceprintPending`）。
- `AudioFormat`（`FLAC`）、`ASRLanguage`（`auto`）、`ASRModel`（`DefaultASRModel`，即 Whisper Tiny）、`AutoSwitchConfirmations`（2）、`AutoSwitchDevice`（true）、`Device`（`default`）。
- `EchoCancellation` 等于 `SupportsEchoCancellation`。该常量只在 darwin 为真。
- `RecordSystemAudio`（false）。`SupportsSystemAudio` 同样只在 darwin 为真。
- `DisableASR`（false）、`ExcludeDeviceKeywords`（`iphone,continuity`）、`SegmentMinutes`（60）、`VoiceprintAdaptation`（false）。

## 运行路径

### 打开

`Store.Open` 先确保根目录存在。查找顺序是当前根，然后是传入的旧目录。每个目录只读 `desktop-settings.json`。

文件不存在就看下一个。JSON 不是对象，或 `settings` 不是对象：当前根的原文写到 `desktop-settings.corrupt-backup.json`，然后跳过。旧目录损坏只记警告并跳过。全部候选都没有合法文档时，若曾遇到损坏则返回错误；否则写入默认文档。

找到合法文档后，`normalize` 把缺字段补成默认值。`windowState` 单独解出来。来自旧目录，或 `nativeDevices` 还不是 true，就先把原文独占写入 `desktop-settings.electron-backup.json`，再 `saveLocked` 写到当前根。`migrateDevices` 为真且还不是原生设备、设备又不是 `default` 时，设备重置为 `default`。测试打开时 `migrateDevices` 为假。

### 规范化

`normalize` 用默认值做底，只接受对象、非空字符串、布尔和数字。然后：

- 语言必须是 `en-US`、`system`、`zh-CN`。主题必须是 `dark`、`light`、`system`。声纹设置必须是 `pending`、`skipped`、`ready`。否则回到默认。
- 音频格式不是 `wav` 就是 `flac`。
- 缺失或 `ValidASRModel` 不接受的模型回到 `DefaultASRModel`（Whisper Tiny）。已有合法选择不变，包括旧版保存的 Qwen3 和 FireRed。完整模型清单见 [语音识别](asr.md)。
- 输出目录不是绝对路径就回到默认目录。
- `AutoSwitchConfirmations` 小于 1 变成 2。`SegmentMinutes` 小于等于 0 变成 60。
- 平台不支持回声消除或系统音频时，对应字段强制为 false。

### 保存

`Save` 把传入值规范化后写入。`WriteJSON` 先写同目录临时文件，`Sync` 后再 `Rename`。编码、写入或同步失败时，旧文件还在。`Save` 在写盘失败时把内存里的设置改回去。

`SaveSettings` 用 `saves` 串行。声纹注册进行中时，更换 ASR 模型或采集相关字段会拒绝，采集字段包括设备、回声消除、系统音频、自动切换和排除关键字。

写盘成功后调用 `engine.Apply`。`Apply` 失败就把上一份设置再 `Save` 一次并返回旧值。成功后 `applySettingsTheme` 把 `Desktop.Theme` 交给 `mygo.Theme.SetSource`。输出目录变了就 `refreshHistory`。非开发且非隔离时，`SetOpenAtLogin` 跟随 `LaunchAtLogin`。

`SaveWindow` 只替换 `windowState`，失败时恢复内存中的上一份。

## 约束与失败行为

- 备份用 `writeExclusive`。目标已存在则保持原文件，不覆盖。
- 损坏备份只在当前根做。旧目录的坏文件留在原地。
- 登录项写入发生在设置已经落盘之后。它失败时返回已保存的设置，不回滚文件。
- `Apply` 失败后的回滚 `Save` 忽略了它自己的错误。调用方仍拿到上一份设置。
- 渲染器保存失败时会再要一次 `bootstrap`。那次读到的是盘上的规范化结果，不是用户刚提交的原文。
- `HideWindowOnClose` 不能被用户改成 false。规范化每次把它写回 true。窗口关闭仍由壳隐藏，不读这个字段做分支。

## 验证

在 `apps/desktop` 下：

```bash
go test ./internal/config/ -count=1 -run 'TestImportPreservesDataAndIsIdempotent|TestCanonicalPrecedenceAndCorruptFallback|TestDefaultsIsolationAndValidation|TestNonObjectConfigurationFallsBackWithoutLosingSource|TestAtomicReplace'
go test ./internal/config/ -count=1 -run 'TestEcho|TestSystemAudio|TestVoiceprint|TestASR'
```

后面一条覆盖 `echo_cancellation_test.go`、`system_audio_test.go`、`voiceprint_test.go`、`asr_models_test.go` 里的平台和模型约束。

链接：`bun run check:docs`
