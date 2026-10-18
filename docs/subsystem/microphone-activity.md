# 麦克风活动

`micactivity.Monitor` 记录其他应用使用麦克风的时间。录音历史用这些区间给麦克风文稿和录音文件做场景标注。检测不打开音频流，也不启动录音。

## 职责与边界

- 拥有扫描、存档、崩溃收口和过期清理。入口是 `micactivity.Open` 和 `Monitor.Poll`。见 `apps/desktop/internal/micactivity/monitor.go`。
- 不拥有转写文本，也不决定时间线怎么分组。它只提供 `protocol.MicrophoneUsage` 列表。桌面服务放在快照里。见 `apps/desktop/internal/desktop/microphone_activity.go`。
- 关联只使用麦克风录音的采集区间。`AudioSource` 为 `system` 的历史项被跳过。见 `apps/desktop/internal/micactivity/retention.go` 的 `recordedIntervals`。
- 麦克风权限是另一个包。`permissions.Status` 和 `Request` 只报告或请求本应用的麦克风权限。见 `apps/desktop/internal/permissions/status.go`。
- 采样间隔属于桌面服务，不属于监测器。`microphoneActivityLoop` 每秒调用一次 `Poll`。见 `apps/desktop/internal/desktop/microphone_activity.go`。

## 权威来源

- 磁盘上的权威记录是应用数据目录里的 `microphone-activity/YYYYMMDD.json`。这和录音输出目录分开。见 `DesktopService.initializeMicrophoneActivity`。
- 一条活动是 `entry`：应用标识、名称、图标、起止时间、`lastObservedAt`，以及是否已关联录音的 `recordingLinked`。
- 正在使用的应用由 `Monitor.active` 表示。结束时间以最后一次成功观察为准，不用进程退出后的墙钟补齐。
- 本应用是否被排除，以扫描结果和 `selfPID` 为准。macOS 额外按 bundle 路径和 bundle identifier 排除自己。

## 运行路径

### 采样

`Poll` 调用注入的 `Scan`。生产用 `NativeScan`。扫描失败只记下错误，不结束当前活动。见 `Monitor.Poll`。

一次新的应用标识会追加一条活动，开始时间是本次观察的 UTC 时间。同一标识仍在结果里时，只更新 `lastObservedAt`。从结果消失时写入结束时间。开始和结束会立刻 `save`。没有变化时，距离上次保存达到 5 秒也会写检查点。

`identifiedApplication` 拒绝空标识和 `pid:` 前缀。加载旧档案时直接丢掉这类条目。见 `apps/desktop/internal/micactivity/retention.go`。

macOS 的 `daybeeMicrophoneApps` 读取 `kAudioHardwarePropertyProcessObjectList` 和 `kAudioProcessPropertyIsRunningInput`。没有这组属性时返回 `ErrUnsupported`。helper 和 XPC 通过可执行路径里的 `.app/` 归到外层 bundle。没有稳定标识的进程不入库。见 `apps/desktop/internal/micactivity/native_darwin.m`。

Windows 的 `NativeScan` 枚举 WASAPI 采集会话，用进程可执行路径做标识，并读取文件描述和 32×32 PNG 图标。路径缓存最多 256 条。查不到路径的进程跳过。见 `apps/desktop/internal/micactivity/native_windows.go`。其他平台返回 `ErrUnsupported`。

睡眠或解锁前的挂起由 `DesktopService.powerLoop` 调用 `Monitor.Suspend`，结束当前观察。恢复后 `Resume`，下一次 `Poll` 重新建活动。`Close` 同样结束并保存。见 `apps/desktop/internal/desktop/app.go`。

### 存档与关联

`load` 读取 `????????.json`。没有结束时间的旧条目用 `lastObservedAt` 收口，避免把程序没运行的时间算进去。损坏的 JSON 或非法时间返回错误，不覆盖原文件。见 `apps/desktop/internal/micactivity/storage.go`。

`ReconcileHistory` 用录音历史标记 `recordingLinked`。区间优先取 `captured_ranges`，没有则用转写片段时间，再没有则只用开始时间，且结束等于开始。重叠后的活动长期保留。

已结束、从未关联录音的活动，超过 `unlinkedRetention`（30 天）后删除。仍在进行的活动不过期。某一天删空后，对应 JSON 一并删除。

短于 1 秒的占用可能落在两次采样之间，不会出现。监测器启动之前的占用无法补记。

### 权限

macOS 用 `AVCaptureDevice` 的音频授权状态，映射为未决定、受限、拒绝或已授权。`nativeRequest` 同步等待系统回调。见 `apps/desktop/internal/permissions/native_darwin.go`。

Windows 读注册表 `ConsentStore\microphone` 和 `NonPackaged`。值为 `Deny` 时是拒绝，否则视为已授权。`nativeRequest` 是空操作。见 `apps/desktop/internal/permissions/native_windows.go`。

系统音频录制权限不在这个包里。它的设置地址见 [audio.md](audio.md)。

## 约束与失败行为

- 扫描失败、音频服务不可用，都不表示应用已停止。活动保持打开，快照带上错误文本。
- 档案保存失败留在 `storageError`，下一次快照可见。初始化失败时监测器为空，桌面服务只保留错误字符串。
- 写入或删除档案前，`save` 确认存储目录可用；目录路径被文件占用时保留错误和脏日期，恢复目录后重试。不能把 Windows 的子路径不存在错误当成删除成功。
- 排除自身依赖 pid、可执行路径或 bundle。两边都识别失败时，自身仍可能出现在列表里。代码没有第二套名字黑名单。
- Windows 上的独占设备、系统代理或受保护进程可能没有可执行路径，于是不会归到调用它们的应用。
- 关联不使用压缩后的音频时长。静音被 VAD 去掉之后，场景时间仍来自 `.recording.json`。见 [recording.md](recording.md)。
- 每秒一次的开始和结束时间是首次观察到变化的时间，不是系统回调时间。

## 验证

在 `apps/desktop` 下执行：

```bash
cd apps/desktop
go test -count=1 ./internal/micactivity/
go test -count=1 ./internal/desktop/ -run 'TestMicrophone'
```

界面标注和筛选：

```bash
cd apps/desktop
bunx playwright test e2e/microphone-activity.spec.ts
```

链接：`bun run check:docs`
