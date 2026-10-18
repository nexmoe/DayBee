# 渲染器

渲染器是 mygo 窗口里的 React 页面。它不持有录音。全部桌面能力来自生成的 Go 绑定。

## 职责与边界

- 入口是 `apps/desktop/src/renderer/src/main.tsx`。它挂上 `AppErrorBoundary` 和 `MotionConfig`，把 `document.body.dataset.platform` 设为 `win32` 或其他。
- `app.tsx` 的 `App` 加载 `components/compact-window.tsx` 的 `CompactWindow`，所有页面共用一套布局。
- 进程内快照在 `lib/desktop-store.ts`。调用封装在 `lib/desktop-api.ts` 的 `desktopApi`。
- 类型和客户端由 mygo 生成到 `src/renderer/src/generated/mygo.ts`。默认值从 `@daybee/shared` 来，见 [workspace.md](workspace.md)。
- 页面不打开麦克风，不写录音文件，不决定更新何时安装。那些在 Go 壳里，见 [desktop-shell.md](desktop-shell.md)。

## 权威来源

- 原生窗口角色固定为 `compact`。地址查询参数不再选择窗口布局，旧 `?window=main` 地址也显示同一紧凑页面。
- 当前快照是模块级变量 `snapshot`。React 通过 `useDesktopSnapshot` 或 `useDesktopSelector` 订阅。
- 没有桥时不伪造一台可用的桌面。`desktopApi` 在 `isMyGo()` 为假时是 `undefined`，快照权限被标成拒绝。

## 运行路径

### 启动

有桥时，模块加载就做三件事：订阅 `desktop:snapshot`，订阅 `desktop:status`，调用 `DesktopService.bootstrap`。

全量事件替换整份快照。状态事件只替换 `status` 和 `voiceprint`，历史和其他字段留在上一份快照里。这和壳里每个窗口独立的 `snapshotPublisher` 一致：状态或声纹单独变化时不重复发送历史。

原生窗口隐藏或最小化后不接收状态或波形事件。重新显示时收到当前完整快照，补上隐藏期间新增的录音和设置；新开或重载页面仍走 `bootstrap`。后台原生验收模式保留隐藏窗口事件，供隔离测试使用。发送策略见 `apps/desktop/internal/desktop/window_updates.go`。

`bootstrap` 在服务 `ready` 之后返回完整快照。新开或重新加载的窗口靠它补上中间错过的历史。

波形不进快照。`components/ui/live-waveform.tsx` 直接订阅 `desktop:waveform`，把 `spectrum` 放进画布。

### 窗口内页面切换

`App` 用 `React.lazy` 加载 `CompactWindow`，加载中显示 `interfaceLoading`。顶部固定显示 `components/recorder-header.tsx` 的录音控制和音波，音波下方的 tab 提供 home、general、input、transcribe 四个页面；使用 `components/ui/tabs.tsx` 的 Radix tab 处理选中状态、焦点、左右方向键、Home 和 End。选中的 tab 为面板提供可访问名称，内容区不再重复显示页面标题。录音头部和导航位于滚动区外，切换页面不卸载音波；380×480 最小尺寸下仍可切换所有页面，不再使用侧栏或抽屉。

当前页面保存在 `CompactWindow` 的 React 状态，默认 home；切换页面会卸载旧面板并重置滚动位置。首页直接显示活动图和完整转写时间线，活动图仍支持选择日期。首页不再提供搜索、应用筛选或搜索高亮；应用名称与图标仅作静态场景标注，正在使用麦克风的应用也不触发筛选。独立历史页、按天文件列表和文件列表筛选代码已移除；已保存的文稿仍用于首页时间线、声纹导出与 MCP。声纹引导在未保存声纹时显示，完成后自动隐藏；重新录制期间仍显示进度和错误，完成或取消后收起。三个设置页复用 `SettingsPage`。声纹初始化未完成或注册进行中时，引导也显示在其他页面；转写设置的声纹操作返回首页并直接开始录制。

通用页的 `components/mcp-settings.tsx` 调用 `GetMCPConfig` 展示连接配置，用户点击复制时才调用 `CopyMCPConfig` 写入系统剪贴板。MCP 服务不依赖页面保持打开，见 [mcp.md](mcp.md)。

主题由 `useWindowTheme` 把 `dark` 类写到 `documentElement`。`system` 跟随 `prefers-color-scheme`。语言文案来自 `lib/i18n.ts`。`system` 时看 `navigator.language` 是否以 `zh` 开头。

`AppErrorBoundary` 只接 React 渲染错误。它显示重新加载按钮，调用 `window.location.reload`。整个页面进程消失时它画不出来，恢复在壳的 `pageRecovery`。

### 调用

`desktop-api.ts` 把生成的 `DesktopService` 摊开。唯一的形状适配是 `pickDirectory`：缺省路径时传 `null`，因为 Go 侧参数是 `*string`。

`desktopActions` 是页面用的动作。失败走 `toast-store`。`saveSettings` 用 `saveSequence` 丢掉过期响应。等待期间先把待保存值写进本地快照。失败且仍是最新一次请求时，再调 `bootstrap` 恢复盘上的值。

`startRecording` 在声纹设置是 `pending` 时直接把调用交给 Go，由壳显示已有紧凑窗及声纹引导。模型还在下载且 VAD 或所选 ASR 模型未就绪时，页面自己弹 toast，不发开始请求。权限是 `not-determined` 时先 `requestMicrophonePermission`。开始调用返回后若 `recording` 仍为假，把状态错误或状态文案再提示一次。

状态错误和声纹错误只在文本变化时提示。阶段已是 `error` 的声纹错误不再弹 toast，因为注册引导里已有说明。

生成客户端里的方法与 `DesktopService` 导出方法一一对应，包括录音、设置、目录选择、更新安装、窗口角色、ASR 模型准备和声纹注册。事件名是 `desktop:snapshot`、`desktop:status`、`desktop:waveform`。

### 开发与 E2E

开发服要求 portless，见 [workspace.md](workspace.md)。`index.html` 的 CSP 允许 `daybee-font:`，开发时 Vite 插件把 portless 的 WebSocket 源补进 `connect-src`。

`playwright.config.ts` 起的是另一台 Vite：`bunx --no-install vite --port 41739`，环境是 `DAYBEE_E2E_TEST`（1）、`PORT`（41739）、空的 `PORTLESS_URL`。这台服务器不走 portless。E2E 夹具再按 `e2e/native-driver.ts` 拉起已构建的原生应用，用测试服务喂快照。那条服务的门槛见 [desktop-shell.md](desktop-shell.md)。

浏览器夹具等待上一次快照请求完成后，再间隔 100 毫秒请求下一次，页面卸载时停止轮询；Node 侧只把变化的快照传过绑定，避免慢速 CI 在千条历史场景堆积整份数据。原生驱动持续读取 stdout 和 stderr，只保留最近 64 KiB 诊断日志。

设置重启验收在点击开关后，先等待原生 `Bootstrap` 读到已保存值，再结束隔离测试进程；界面的乐观状态本身不代表写盘完成。

实时转写验收先等待原生状态同时满足 `recording` 和 `asrEnabled`，再注入已完成识别的语音。浏览器点击返回只代表事件已发送，不能用空时间线代替录音已启动的确认；两窗和搜索用例共用该步骤。

Vitest 只跑 `src/**/*.test.ts`，环境是 node，不挂浏览器。

直接向隔离输出目录写入 JSON 的 E2E 用例要调用 `e2e/fixtures.ts` 的 `app.refreshHistory`。它通过测试引擎的 `RunTranscribe` 走正常的历史刷新路径，不等待五分钟兜底扫描。实时转写用例仍调用 `app.completeSpeech`，验证 `HistoryChanges` 事件即时刷新。

布局与恢复检查遵守平台规则：Windows 紧凑页为窗口按钮留出顶部空间，仍须检查内容不被遮挡及左右留白一致；系统音频设置入口只在 macOS 显示，重试和错误详情两边都检查。`DAYBEE_TEST_RENDERER_PLATFORM` 可设为 `windows` 或 `darwin`，让浏览器夹具同时覆盖 user agent 与桥平台以复现这些界面差异；Go 服务和原生烟测仍使用当前主机平台。

首页的响应式布局检查先等待字体就绪，再在同一次 DOM 读取中取得声纹引导、活动图与时间线的位置；用自动重试等待布局稳定，仍严格检查不重叠及小于 48px 的间距，避免跨帧测量误报。

`e2e/virtual-timeline.spec.ts` 的单例完整覆盖 1000 条历史、拖动、变宽、实时更新、搜索、筛选与导航，使用 90 秒总看门狗，为 Intel CI 的原生桥和渲染预留余量。其他测试仍是 30 秒；该用例的每项等待断言及行数、位置、间距检查不放宽，它不作为单项交互延迟基准。

阅读位置验收先选择与视口相交的段落，包含顶部被部分裁切的行；更新后按同一段全文定位，在一次 DOM 读取中取得位置，仍要求位移小于 2 像素。避免每次选取“首个顶部完整可见行”，因为 1 像素取整就可能换行或误判没有锚点；测量使用单层轮询。

完整文稿验收在窄屏滚动到底后，按 `data-last` 检查实际历史末项的文本及视口交集，不把虚拟列表当前挂载的最后一个 DOM 节点当作固定记录；挂载范围会随滚动和行高测量变化。

## 约束与失败行为

- 生成文件首部标明不要手改。绑定函数名变化后，页面里的 `desktopActions` 必须跟着改。
- `useDesktopSelector` 用上次选出的值做相等判断。10 Hz 的状态广播不会让切片没变的订阅者重渲染。
- 桥缺失时每个动作都拒绝，文案是 `errorBridgeUnavailable`。
- 保存设置不是补丁。整份对象进 Go，再经 `normalize` 返回。本地乐观值可能与最终值不同。
- 波形订阅不检查桥。非 mygo 页面里 `desktopEvents.onWaveform` 仍然注册，只是没有帧。
- 错误边界的重新加载只重载当前页面，不重启 Go 服务，也不停止录音。

## 验证

在仓库根：

```bash
bun run --cwd apps/desktop test -- src/renderer/src/lib/desktop-store.test.ts
bun run typecheck
```

`desktop-store.test.ts` 覆盖快照与保存序列。类型检查同时编 `src/renderer` 和 `packages/shared`。

渲染器对真实 Go 服务的浏览器检查：

```bash
bun run --cwd apps/desktop test:e2e
```

链接：`bun run check:docs`
