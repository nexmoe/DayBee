# RecBee Dev 主窗口白屏与开发态内存积累

日期：2026-10-12。原生 macOS arm64、MyGo 0.3.4、WebKit。

## 实际白屏证据

应用日志记录主窗口 WebContent 在 17:05:42 和 17:21:42 终止。第一次执行了自动 reload；第二次因一次性恢复额度已用完，记录 `reload=false`。原生进程和紧凑窗口仍然存活。整个页面进程退出后，React 错误边界无法绘制。

MyGo 的 Darwin 回调对所有 `webViewWebContentProcessDidTerminate:` 都使用 `terminated` 字符串。系统记录的 2048MB 是软限制，不是实际占用或超限证据；未发现对应的 CrashReport、jetsam 或 footprint。两次真实退出的底层原因仍未得到系统日志确认。

## 可复现的内存问题

使用隔离 store、假录音引擎和 250 条合成历史。快照约 664KB；每秒更新约 5 次，只循环改变 60 个 elapsed 值，文本不变。两个原生窗口同时打开，无 HMR、无开发者工具、无真实录音或转写数据。主窗口始终只挂载 10 个时间线节点。

React DOM 19.2.4 开发构建的 `logComponentRender` 会将 props 差异复制到 User Timing `PerformanceMeasure.detail`。浏览器缓存保留这些条目；该构建没有调用 `clearMeasures`。

| 时间 | 无清理主窗口 RSS | 第 30 秒开始清理的主窗口 RSS |
| --- | ---: | ---: |
| 0 秒 | 200624 KiB | 200416 KiB |
| 30 秒 | 685232 KiB | 690704 KiB |
| 60 秒 | 813648 KiB | 711744 KiB |
| 110 秒 | 1075344 KiB | 422592 KiB |

清理组在同一页面继续接收相同更新，只从第 30 秒起每 500ms 调用 `performance.clearMeasures()`，没有刷新、停止更新或修改业务数据。此前 20 秒内主窗口 measure 数从 731 增到 30975；清理后采样为 0–1415，marks 始终为 0。RSS 在后半段稳定并发生 GC。

这证明了开发态 measure 缓冲积累与持续内存增长的因果关系。它不能反向证明真实窗口的两次退出由 OOM 引发。RSS 不是回归测试的硬通过条件。

## 上游修复与验证边界

React 上游 [issue 34770](https://github.com/react/react/issues/34770) 和 [PR 34803](https://github.com/react/react/pull/34803) 已确认并修复这类积累。官方修复在生成条目后按名称清理缓存；PerformanceObserver 通知和开发者工具录制仍然有效，`getEntries` 的历史缓存不再保留这些条目。

实际检查 npm 发布包：19.2.4 和 19.2.8 的 `react-dom-client.development.js` 没有 `clearMeasures`；稳定版 19.3.0 包含 8 处调用。采用成对升级 React 与 React DOM 的上游修复，不新增页面级清理 observer，不改恢复额度。

回归应确认：持续的合成更新确实渲染；PerformanceObserver 仍收到 React 条目；React measure 缓存有界；自定义 mark/measure 不丢失；原生双窗口持续显示。短时间启动测试和 React 错误边界测试无法覆盖此问题。

## 修复后的结果

- 旧版原生失败合同：5 秒、24 次更新，React measure 缓存 8125 条，超过 1000 条上限。
- 新版原生持续测试：180 秒、867 次更新，两个窗口每次采样的 measure 缓存均为 0；主窗口始终有 10 个节点，未发生页面进程退出。RSS 仍存在分配高水位和阶跃，不能将这次短测当作整个进程长期内存有界的证明。采样见 [JSON](dev-webkit-memory.json)。
- [持续更新 E2E](../../apps/desktop/e2e/dev-performance.spec.ts)：40 轮均显示新文本和计时；root 复跑的 observer 收到 18416 条 React measure，历史缓存为 0，自定义 mark/measure 各保留 1 条。
- 持续更新与中英文错误恢复 E2E 共 2 项通过；原生双窗口的导航、关闭重开、刷新和共享录音状态检查通过。
- Typecheck、Vitest 71 项、生产前端构建通过。本次没有改 Go 业务逻辑。

dev 已正常退出并重新启动，Vite 日志确认因 lock 改变重新优化了依赖。此前录音保存为 FLAC，metadata 状态为 `ok`；重启后的新 WAV 持续增长。界面控制工具连接原生窗口失败，因此没有将隔离测试的可见内容作为用户当前实际大窗口的视觉确认。
