# DayBee 后台资源占用

2026-10-16，macOS 27.0 / Apple M5 Pro，sherpa-onnx 1.13.8、ONNX Runtime 1.28.2。

## 现场与原因

用户截图为进程组约 3.10 GB、CPU 25.1%，其中主进程约 2.49 GB。随后对已安装的 0.8.6 做只读采样：主进程 physical footprint 约 2.6 GiB，原生 malloc 已分配约 2.5 GiB。设置选中 FireRed ASR2 AED，开启声纹及适应；这些模型和原生推理缓存属于主进程。5 秒采样期间见到 VAD、原生音频处理、麦克风活动扫描及 WebKit IPC，未据此认定 CPU 全部来自 ASR，也未证明长期内存泄漏。

确定的生命周期问题：停止录音后 ASR 识别器仍被 worker 持有。新增回归在修改前失败，报 `stopped recording retained the native model: created=1 released=0`。此外，原实现每 30ms 计算频谱并向所有窗口广播波形，每 100ms 检查并发送变化状态，没有原生窗口可见性门控。

## 改动

- [ASR worker](../../apps/desktop/internal/asr/worker.go) 在任务清空并连续空闲 2 分钟后释放识别器，下次按任务绑定的模型路径重载。停止、休眠及安全停录在归档后请求释放；不打断推理或清掉排队语音。
- [窗口更新](../../apps/desktop/internal/desktop/window_updates.go) 只向显示中的窗口发送事件，每个窗口独立记录已发送状态。隐藏、最小化或隐藏应用时停止更新，再次显示时发送完整快照。没有可见窗口时也跳过快照构造和频谱计算。后台录音、VAD、归档和转写继续；托盘文字只在变化时更新。
- [频谱](../../apps/desktop/internal/audio/spectrum.go) 换成 256 点 radix-2 FFT，保留 Blackman 窗、平滑、归一化和字节映射。独立 DFT 的随机输入、环形缓冲和衰减对照，以及既有 Chromium fixture 均通过。

## 测量

频谱基准各跑三轮：

| 实现 | 每帧时间 | 分配 |
| --- | --- | --- |
| 原 DFT | 31.645 / 31.269 / 31.668 µs | 1024 B，1 次 |
| FFT | 2.945 / 3.446 / 3.358 µs | 1024 B，1 次 |

均值约快 9.7 倍。这只是频谱函数的改进；原先每秒约 33 帧，其自身耗时不足以解释截图里的全部 CPU。

真实模型测试使用缓存 FireRed ASR2 AED 和官方 Qwen3 包里的 `test_wavs/fast1.wav`，不读取用户录音或打开麦克风。这个 WAV 为 44.1 kHz，由 sherpa 在解码时重采样。测量为独立 Go 测试进程的 macOS physical footprint，包含原生分配，没有 WebView、声纹或现场采集：

| 阶段 | Physical footprint |
| --- | --- |
| 加载前 | 14.3 MiB |
| 加载并转写后 | 2764.8 MiB |
| 空闲 2 分钟卸载后 | 631.5 MiB |
| 下一条任务重载后 | 2764.8 MiB |

卸载释放约 2133 MiB（77%）。原生分配器仍有保留内存，不能承诺卸载后整个应用只剩几十 MB。重载前后的识别结果相同。第一次任务及重载任务分别耗时 5.42 和 5.23 秒，均包含加载及整段解码，不能用作纯加载延迟。活跃转写期间模型仍会占用数 GB；本次不更改用户选择的模型。

## 验证

- `go test -race -count=1 -timeout=180s ./...`：通过，包括停止后释放、虚拟时间的空闲释放与排队保护、隐藏波形期间音频照常归档。
- `bun run test`：82 项通过；`bun run typecheck`、`bun run check:docs` 和本地 macOS 构建通过。
- `bun run test:native`：隔离假录音引擎的双窗口、导航、关闭重开、重载和共享状态通过。
- [原生可见性检查](../../apps/desktop/e2e/resource-usage.mjs)：仅主窗口可见时紧凑窗不收更新；两个窗口隐藏后新增语音的 full/status 事件增量均为 0；重新显示、最小化后恢复和应用隐藏后恢复均拿到最新语音。
- 原有断线恢复测试的等待时间小于生产设备轮询间隔（5s 对 10s），已改为允许一个完整轮询周期；没有改变生产恢复间隔。

下面命令在 `apps/desktop` 执行；Go 需要本地 1.27.1 工具链和原生库。可见性检查会短暂显示隔离测试窗口：

```bash
go test ./internal/audio -run '^TestSpectrum' -bench '^BenchmarkSpectrumFrame$' -count=3
go test ./internal/asr -run '^TestIdle' -count=1
node e2e/resource-usage.mjs --interactive
DAYBEE_TEST_IDLE_MODEL_DIR=/absolute/cached/model \
DAYBEE_TEST_IDLE_WAV=/absolute/public/fixture.wav \
DAYBEE_TEST_IDLE_REPORT=/tmp/daybee-idle-native.json \
go test ./internal/asr -run '^TestNativeIdleModelRelease$' -v -count=1 -timeout=4m
```

当前已构建新应用，没有替换或重启用户正在运行的版本。未测整应用长期运行的 CPU/内存降幅，未做 Windows 硬件验证。
