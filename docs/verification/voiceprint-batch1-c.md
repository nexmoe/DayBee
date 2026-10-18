# 声纹第一批 C1：系统参考采集

2026-10-12。本轮只修采集合同，没有改录入。

## 行为

- `SystemAudio` 仍是必须归档的系统录音。启动失败返回错误，`Start` 用原有 `Stop` 路径收尾。
- `SystemReference` 只作可降级参考。`attachSystem` 不在持有 `c.mu` 时调用启动失败通知，而是把通知函数交回 `Start`，解锁后再调用。
- 运行时错误仍从系统回调里通知一次。该回调不在 `c.mu` 的持锁调用栈上，实现里不另起 goroutine，也不在回调中 `Stop`。
- `open` 返回 `(nil, error)` 或 `(nil, nil)` 都不会调用 `Start`。后者使用 `errSystemCapture`。
- 句柄留到 `Stop`/`Close`。重复 `Stop` 不再次释放。

## 检查

```text
/tmp/eve-go-1.27.1/go/bin/go test ./internal/audio/ -count=1 -timeout 60s \
  -run 'TestSystemReferenceNotifiesOnceAndStopReleases|TestReferenceStartNotifyRunsAfterUnlock|TestSystemOpenFailureDoesNotPanic|TestSystemAudioFailureStaysStrict|TestSystemCaptureStaysOffWithoutRequest'
```

结果：`ok github.com/nexmoe/eve/apps/desktop/internal/audio 0.280s`。

覆盖：参考启动失败在解锁前 `notes==0`，解锁后的通知里调用 `Stop` 不与 `c.mu` 自锁；运行时分支在 `system.fail` 前 `notes==0`，两次失败只通知一次；`(nil, error)` 与 `(nil, nil)`；归档失败不走参考回调；未请求时不打开系统捕获；重复 `Stop` 计数保持 1。

## 局限

测试用的是假设备，没有真实 ScreenCaptureKit 会话。运行时通知如果由系统采集线程同步发出，调用方在该回调里 `Stop` 仍可能和采集线程自相等待；本实现不在那个回调里停止设备。

## 录入数值门槛

`MeanSquare` 遇到非有限 PCM 返回 NaN。`NoisePower` 遇到 NaN、Inf 或负功率返回 `+Inf`。`SpeechSNR` 只在两端都有限时返回有限 dB，否则返回 `-Inf`，不会返回 NaN 或 `+Inf`。因此 `snr < 15` 会拒绝坏数据。`AssessEnrollment` 在比较 15 dB 之前先把非有限 PCM 或噪声判为 `QualityInvalid`。`collect` 在返回样本前调用 `screenEnrollment`，`session` 在 `EmbedWindows` 之前再查一次。

```text
/tmp/eve-go-1.27.1/go/bin/go test ./internal/voiceprint/ -count=1 -timeout 90s \
  -run 'TestNoisePower|TestSpeechSNR|TestNonFinite|TestInconsistent'
/tmp/eve-go-1.27.1/go/bin/go test ./internal/desktop/ -count=1 -timeout 180s \
  -run 'TestVoiceprintEnrollment|TestCollectRejects|TestSubsample'
```

结果：voiceprint `ok` 0.280s；desktop `ok` 6.104s。`TestCollectRejectsNonFinitePCMBeforeEmbedding` 对 NaN、`+Inf`、`-Inf` 都得到 invalid 错误，且 `EmbedWindows` 调用次数为 0。当时的 `TestSubsample` 已删除，录入不再把三段摊平后按原始阈值保存。

## C2 录入收尾

三段各自保留。三组必须使用同一个已解析设备和非空的回声消除设置。`InconsistentEnrollment` 不通过就结束，不进入验证。通过后调用 `Calibrate(groups, EnrollmentContext)`。独立的 4 秒只交给 `Score`，再用 `DecisionScores` 和 `DecideProfile`（echo 为 nil）；只有 `"me"` 才 `SaveProfile`。`commit` 先留下完整旧设置。`v.save` 失败或提交返回前上下文取消时恢复档案；只有 store 里的设置已经变了，才再 `v.save(previousSettings)`。恢复失败和原错误一起返回，并且不再把界面标成旧声纹仍可用。store 没变时不会第二次保存。

播放超过 −50 dBFS 时清掉未提交的 VAD，并把 `voiced` 设成已经留下的样本长度。参考变为不可用后不再等系统静音，暂停会解除，录音继续，并显示参考受限说明。已知英文句子映射到中英文；未知 `message` 仍不显示原文。

```text
/tmp/eve-go-1.27.1/go/bin/go test ./internal/voiceprint/ -count=1 -timeout 90s \
  -run 'TestNoisePower|TestSpeechSNR|TestNonFinite|TestInconsistent'
/tmp/eve-go-1.27.1/go/bin/go test ./internal/desktop/ -count=1 -timeout 180s \
  -run 'TestVoiceprintEnrollment|TestCollect|TestEnrollment|TestReference|TestSettings'
/tmp/eve-go-1.27.1/go/bin/go test ./internal/desktop/ -count=1 -timeout 240s -race \
  -run 'TestVoiceprintEnrollment|TestCollect|TestEnrollment|TestReference|TestSettings'
```

结果：voiceprint `ok` 0.277s；desktop `ok` 16.696s；race `ok` 19.430s。C2b 同一组 desktop race：`ok` 22.396s。覆盖第一次 `v.save` 写入 ready 后失败、第二次写回旧设置并恢复完整 v3；store 未变时只保存一次；第二次恢复失败时错误留在界面上，且 `Enrolled` 不再为真。

覆盖：第 1/2/3 组以及验证段的设备变化；这三处在下一次采集前改变回声消除；某一组方向不同则拒绝且不保存；设置失败后原 v3 的设备、回声消除、创建时间和校准带都还在；回滚 `SaveProfile` 失败和没有旧档案时 `Clear` 失败都会带上两个错误；播放暂停时进度与已留样本一致，污染样本不进入结果；参考失效后不会停在暂停；VAD 截短后继续录到真实样本长度；验证音频没有进入 `EmbedWindows`。

## 局限

成功路径的单元向量是测试替身，用来让 `Calibrate` 和 `DecideProfile` 真的跑通，不是房间录音，也不能当成识别准确率。`TestNativeVoiceprintEnrollment` 在未设置 `EVE_TEST_VOICEPRINT_MODEL`、`EVE_TEST_VAD_PATH`、`EVE_TEST_VOICEPRINT_SPEECH` 时跳过。播放和参考用例用的是脚本包，没有真实 loopback。
