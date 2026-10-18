# 桌面壳

桌面壳是进程级的 `DesktopService`。它拥有窗口、托盘、设置、录音引擎和更新。页面刷新或隐藏不拥有录音。

## 职责与边界

- `apps/desktop/main.go` 的 `main` 在 `DAYBEE_VERIFY_AUDIO`（1）时走 `codec.Diagnostic`，在 `DAYBEE_VERIFY_NATIVE`（1）时走 `asr.CheckVersion`。否则调用 `desktop.Run(desktop.New())`。
- `--mcp` 走独立的 stdio 文稿读取入口，不创建桌面服务或窗口。连接和读取契约见 [mcp.md](mcp.md)。
- `apps/desktop/internal/desktop/app.go` 的 `Run` 注册生命周期。`initialize` 打开数据目录、设置、引擎和窗口。
- 窗口、托盘、拖拽和字体在 `window.go`、`window_controls.go`、`window_drag_darwin.go`、`fonts_darwin.go`。Windows 对应文件是空实现。
- 页面进程退出后的一次自动重载在 `page_recovery.go`。
- 更新检查和安装在 `updater.go`。
- 原生验收用的回环服务在 `test_server.go`。普通开发和发布不启动它。
- 录音、采集、转写、声纹的内部行为不在这页。壳只持有引擎并转发调用。

## 权威来源

- 进程状态在 `DesktopService`。对外快照由 `snapshot` 组装成 `protocol.DesktopSnapshot`。
- 数据根目录由 `dataPaths` 决定。未设置 `DAYBEE_STORE_DIR` 时用应用数据下的 `DayBee`。测试且未隔离时用 `DayBee Development`。
- `isolated` 在设置了 `DAYBEE_STORE_DIR` 或 `DAYBEE_E2E_TEST`（1）时为真。隔离运行不改登录项，见 `shouldApplyLoginItem`。
- 产品名常量 `AppName`（DayBee）在 `branding.go`。

## 运行路径

### 启动

`Run` 把激活策略设为 accessory，第二个实例调用 `reveal`。`WhenReady` 后 `initialize` 在后台跑。

`initializeEngine` 解析应用数据与文稿目录，把 user data 指到 DayBee 根。日志追加到该根下 `logs/main.log`。`config.Open` 读设置。`importLegacyState` 在没有 `daybee-migration.json` 时，从 `Eve Recorder`、`RecBee`、`eve-desktop` 复制声纹配置和麦克风活动日记。已有 DayBee 文件不覆盖。模型缓存留在旧目录，不搬录音。

非测试路径检查 sherpa 版本，创建采集、模型管理器和 `recorder.Engine`。声纹配置打不开只记警告，转写继续。

窗口创建失败则 `Quit`。初始化成功后：声纹设置仍是 `VoiceprintPending` 就显示紧凑窗口内的声纹引导；否则在非测试模式下立即调用 `StartRecording`。首次引导完成或跳过后也自动开始录音，不再提供启动录音开关。测试模式保持空闲，等待显式录音请求。

`publishLoop` 每 100 毫秒在脏标记为真时广播。引擎的 `HistoryChanges` 立刻刷新历史并广播。另有 5 分钟定时器重新扫描录音目录。

### 单一紧凑窗口

所有页面共用 `createWindow` 创建的一扇紧凑窗口。默认 420×760，最小 380×480，最大宽 520，先隐藏。macOS 无边框、透明、popover 材质。Windows 无边框、acrylic、背景 `#ffffff`。

关闭被拦截并隐藏，直到 `quitReady`。失焦后若未钉住、不是测试、也不是退出中，则隐藏。`reveal` 在 macOS 先把应用取消隐藏，再按托盘 bounds 把窗口放到托盘下方。`revealUntil` 盖住接下来 200 毫秒内的失焦隐藏。

`OpenRecorderWindow` 仅显示并聚焦已有紧凑窗，不创建额外窗口。`windowForRole` 只接受 `compact`。`CloseWindow`、`MinimizeWindow`、`SetWindowPinned` 都作用于这扇窗。录音属于进程，切换 tab、隐藏和重载页面不停止录音。

macOS 上 `installWindowCompatibility` 在内容视图顶部加一层高度 16 的 `DayBeeLegacyDragView`，与 CSS 紧凑窗的 `--panel-gutter`（1rem）拖拽区一致。双击按系统的 `AppleActionOnDoubleClick` 最小化或缩放。Windows 不安装该拖拽层。

### 托盘与电源

`window_updates.go` 用原生显示、隐藏、最小化和恢复事件维护窗口可见性。快照按窗口分别比较，只发给可见窗口；重新显示时强制发完整快照。所有窗口不可见时，发布循环跳过快照构造，关闭录音引擎的频谱计算和波形发送，后台录音继续。托盘 tooltip 独立更新，文本未变时不重复调用原生接口。

`tray.go` 的 `syncTray` 按录音、恢复或后台待命改 tooltip。右键菜单能开始、停止、切换开机启动、显示紧凑窗口、刷新页面、退出。开发者工具开在紧凑窗。

`powerLoop` 在休眠时暂停麦克风活动监视和引擎，并取消进行中的声纹注册。唤醒或解锁时恢复。

退出时 `OnBeforeQuit` 先挡住。后台依次取消上下文、关闭声纹注册、麦克风活动、引擎和声纹管理器，然后再次 `Quit`。

### 页面恢复

`attachPageRecovery` 监听加载失败和渲染进程消失。加载失败只记 `role`、错误码和描述，不记 URL。渲染进程消失时，若 `auto` 仍为真就 `Reload`，然后把 `auto` 设为假。托盘「刷新界面」走 `reloadInterfaces`，对紧凑窗调用 `manual`，重新允许下一次自动重载。窗口关闭时 `release` 摘掉监听。

### 更新

`updateLoop` 在测试或 `mygo.Updater.Enabled` 为假时把阶段设为 `UpdateUnavailable` 并返回。打包应用启动时检查一次，之后每 6 小时检查。`checkUpdate` 超时 2 分钟。已有待安装版本或正在安装时，新的检查直接返回。

发现版本只调用 `updateAvailable`。它不下载、不重启。用户调用 `InstallUpdate` 后才安装。

安装先独占 `operations`，拒绝退出中和进行中的声纹注册。然后 `engine.Stop`。停不下来就不调用 `install`。下载超时 30 分钟。成功后 `relaunch`。失败把阶段设为 `UpdateError`，保留 `pendingUpdate`，应用继续跑，可以重试。

`InstallDeferredUntilIdle` 只在进入可用或下载阶段时被写成 false。当前安装路径不等待转写空闲。

### 字体

macOS 的 `registerFontCompatibility` 用 Core Text 找同一套静态苹方：`PingFangSC-Regular`、`PingFangSC-Medium`、`PingFangSC-Semibold`。三个面必须来自同一个字体文件。协议 `daybee-font` 只提供 `system/pingfang.ttc`。渲染器在 `styles.css` 用这个 URL 加 PostScript 片段注册 `DayBee Legacy System`。Windows 的 `registerFontCompatibility` 返回 nil。

### 测试服务

`nativeTestServer` 只在同时满足 `DAYBEE_E2E_TEST`（1）、`DAYBEE_TEST_TOKEN` 至少 32 字节、`DAYBEE_STORE_DIR` 非空、`DAYBEE_TEST_PORT` 非空时监听 `127.0.0.1`。请求必须带 `Authorization: Bearer <token>`，并且是 POST。

它提供 `/speech`、`/waveform`、`/fixture`、`/eval`、`/capture`、`/snapshot`、`/window`、`/hide-app`、`/hide`、`/show`、`/reload`。`/speech` 要求引擎是 `testEngine`。`DAYBEE_TEST_BACKGROUND`（1）时 `reveal` 不把窗口带到前台。`DAYBEE_TEST_VISUAL`（1）时窗口带上 `visual-preload.js`。

## 约束与失败行为

- 单实例锁失败时 `Run` 返回 nil，不启动第二套服务。生成绑定期间 `MYGO_GENERATE` 非空，跳过这道锁。
- `guard` 在初始化失败、退出中或 `installing` 时拒绝录音和保存设置。
- 初始化错误留在 `initError`，快照的 `Status.Error` 和 `StatusMessage` 携带该文本。窗口仍会创建。
- 迁移标记 `daybee-migration.json` 的 `version`（1）写上之后，后续启动不再从旧目录复制。
- 旧输出目录 `RecBee` 与 `Eve Recorder` 仍在 `recordingRoots` 里，供历史扫描。隔离运行不加入它们。
- 紧凑窗尺寸由 `OnResize` 调用 `Store.SaveWindow`。读回时宽限制在 380 到 520，高低于 480 则回到 760。默认窗口状态与创建尺寸一致，宽 420、高 760。

## 验证

在 `apps/desktop` 下：

```bash
go test ./internal/desktop/ -count=1 -run 'TestRenderGoneReloadsOnceUntilManualRefresh|TestLoadFailureLogOmitsURL|TestRefreshReloadsAndRearmsCompactRecovery|TestReleaseDropsListenersAndClearsRecovery|TestDetectedUpdateWaitsForUser|TestExplicitUpdateSavesRecordingAndRestartsOnce|TestUpdateFailureKeepsRunningApplicationAndAllowsRetry|TestUpdateDoesNotInstallIfRecordingCannotBeSaved'
go test ./internal/config/ -count=1 -run 'TestImportPreservesDataAndIsIdempotent|TestCanonicalPrecedenceAndCorruptFallback|TestDefaultsIsolationAndValidation|TestAtomicReplace'
```

原生壳烟测使用已构建的应用：

```bash
bun run --cwd apps/desktop test:native
```

链接：`bun run check:docs`
