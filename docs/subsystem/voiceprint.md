# 声纹

给麦克风转写标「我」。不删音频，不删文本，不按身份切开 PCM。系统录音不标本人。

## 职责与边界

- 档案、嵌入、校准、区间和回声否决在 `apps/desktop/internal/voiceprint`。`Manager` 不采集，不转写。
- 录入、跳过、清除和样本导出在 `apps/desktop/internal/desktop` 的 `voiceprint_enrollment.go`、`voiceprint_service.go`、`voiceprint_export.go`。桌面服务停录音、下模型、提交档案。
- 实时和批量标注在 `apps/desktop/internal/recorder/voice_job.go`、`batch_spans.go`、`batch_voiceprint.go`。一次 VAD 段只解码一次。身份区间不是 ASR 边界。
- 模型文件由 `assets.Manager.EnsureVoiceprint` 下载。文件名是 `voiceprint.ModelName`。缓存按文件名区分，不复用旧 CAM++ 文件。
- 播放参考环在 `apps/desktop/internal/recorder/echo_reference.go`。环的生命周期属于录音，见 [recording.md](recording.md)。本页只写声纹如何使用它。

嵌入模型是 `3dspeaker_speech_eres2netv2_sv_zh-cn_16k-common.onnx`。维度 `embeddingDimension`（192）。Provider `cpu`。线程数 `infer.Threads`：`NumCPU/4`，至少 1，至多 2。采样率 `voiceSampleRate`（16000）。

窗口：`minWindowSamples`（12800，0.8 秒），`windowSamples`（24000，1.5 秒），`hopSamples`（16000，1.0 秒）。短于 0.8 秒不能嵌入。0.8 到 1.5 秒整段一个窗口。更长则 1.5 秒窗、1.0 秒步长，尾部不足一窗的样本不另开窗。cohort 生成脚本的步长是 0.5 秒，只用于打包负例，不用于运行时标注。

## 权威来源

档案文件是应用数据目录下的 `voiceprint/profile.json`。`config.WriteJSON` 用 `os.CreateTemp` 写临时文件再 `Rename`。`CreateTemp` 建文件即为 `0600`，替换后保留。测试在非 Windows 上断言 `0600`。向量不进前端快照，不进录音目录。

`Profile` 字段：

- `Version` 1 或 2：同一模型的原始余弦路径。`Save` 写版本 2。
- `Version` 3：`calibratedVersion`。必须有设备、回声开关和 `Calibration`。
- `Embeddings`：1 到 64 条 192 维有限向量。版本 3 保留各段窗口，不压成一条。
- `Calibration`：`TLow`、`THigh`、`Model`、`CohortID`。`CohortID` 是内嵌 `cohort_eres2netv2.f32` 的 SHA-256。
- `Adaptation`：最多 `adaptationCap`（32）条补充向量。原注册向量不替换。
- `AdaptationProgress`：`LearnedClips`、`AssistedClips`。计接受的音频片段，不计窗口。旧档案没有这个字段时，进度为未知。
- `Revision` 不落盘。重录、清除、清适应池、切换设备或回声时递增。过期任务不能写回。

模型名不是 `ModelName` 的档案拒绝加载，包括 CAM++。损坏的 JSON 让 `New` 返回错误，原文件保留。空档案可以继续转写。

前端只见 `VoiceprintSnapshot`：阶段、步数、秒数、电平和可选的适应计数。设置里的 `desktop.voiceprintSetup` 为 `pending`、`skipped`、`ready`。已录入但设置仍为 `pending`，或未录入却为 `ready`，由 `reconcileVoiceprintSetup` 改齐。`recording.voiceprintAdaptation` 默认关闭。

首页「标记我的声音」卡片仅在未保存声纹时常显，跳过初始化后仍可录入。已保存声纹且阶段为 `idle` 或 `ready` 时隐藏；从转写设置发起重录后显示进度与错误，成功或取消后再次隐藏。清除声纹后首页重新显示录入卡片。

## 运行路径

### 录入

`StartVoiceprintEnrollment` 先要麦克风权限。`session` 停掉当前录音。引擎仍忙则拒绝。然后 `Ensure` 只下 VAD，再 `EnsureVoiceprint`。

`voiceprintSamples`（3）段，每段目标 `voiceprintSampleSeconds`（8）。最后一段独立验证，目标 `voiceprintVerifySeconds`（4），不进入注册。单段超时 `voiceprintEnrollment.timeout`（1 分钟）。

每段先采环境。环境窗口上限 `enrollmentAmbientMax`（32000 样本）。噪声用 VAD 负例功率的低分位，底为 `EnrollmentNoiseFloor`（1e-12）。`AssessEnrollment` 要求信噪比至少 `EnrollmentMinSNRdB`（15）。`Clipped` 拒绝绝对值达到 `fullScale`（0.99）的样本超过 0.1% 的片段。非有限样本在嵌入前拒绝。

设备 ID 不能为空，也不能是 `default`。三段加验证的设备 ID 和回声消除开关必须相同。系统播放时暂停，进度只计已保留人声。参考不可用时继续采集，并提示用户自己暂停播放。参考失败不是麦克风失败。

三段各自 `EmbedWindows`。`InconsistentEnrollment` 看各段质心。某一段对其他段的平均余弦低于中位数超过 `max(2σ, EnrollmentOutlierGap)`，`EnrollmentOutlierGap` 为 0.10，则拒绝。

`Calibrate` 用内嵌 cohort 做留一交叉验证。窗口只和另外两段比，不和同段比。负例归一化去掉该查询说话人自己的 32 条窗口。`acceptFalseRate`（0.01）和 `rejectFalseRate`（0.05）之间没有可行带，或高低阈值倒置，返回 `ErrEnrollmentInseparable`。

验证段对档案 `Score`。`DecideProfile` 不是 `me` 则不保存。`commit` 写档案并把设置标为 `ready`。中途取消或保存失败时 `rollbackEnrollment` 恢复旧档案和旧设置。恢复失败置 `rollbackFailed`，不再自动恢复录音。

跳过把设置写成 `skipped`。若当时已录入则写成 `ready`。首次引导成功完成或跳过后，非测试模式自动开始录音；重录继续按进入引导前的录音状态恢复。清除调用 `Clear`，设置改为 `skipped`。

### 分数

`ProfileScore` 取质心余弦，以及最相近 3 条注册向量余弦的平均，用较大者。

版本 1 和 2 的判定分就是这个余弦。接受线 `modelThresholds[ModelName]`（0.537）。未知模型名用 `defaultThreshold`（0.55）。`Manager.Verify` 的注册验证阈值是另一个数：`matchThreshold`（0.6）。

版本 3 把余弦变成有界 AS-norm：`asNorm` 为两侧 z 分数的一半，限制在 `asNormMin`（-20）到 `asNormMax`（20）。均值和标准差来自 cohort 余弦的最高 `asNormTopK`（100）条。标准差限制在 `asNormSigmaFloor`（0.001）到 `asNormSigmaCeil`（1）。cohort 哈希与档案 `CohortID` 不一致时，判定分全部为 `asNormMin`，不回退到原始余弦。

`Decide` 与 `DecideProfile` 先做三点中值。回声窗算作非本人。`me` 还要同时满足：达到高阈值的窗至少 `meWindowFraction`（0.7），且均分不低于高阈值。版本 3 在低阈值和高阈值之间为 `uncertain`。高低阈值相同则没有不确定带。

### 区间

`IdentitySpans` 覆盖 `[0, n)`，首尾相接，不重叠。窗 `i` 拥有 `[start_i, start_{i+1})`，最后一窗收到 `n`。

迟滞：已是 `me` 时，分数不低于低阈值就保持。进入 `me` 要达到高阈值。低于低阈值是 `not_me`，中间是 `uncertain`。新身份连续不足 1 秒则保持上一身份。回声和原始分小于等于 `asNormMin` 的窗立即是 `not_me`，不参与这 1 秒吸收。

`reviewMeSpans` 再用 `DecideProfile` 审每一段 `me`。70% 投票失败则改成 `not_me` 或 `uncertain`。只有未放宽的高阈值也通过，`Anchor` 才为真。

`guardBoundaryOverlap` 看相邻的 `not_me` 窗。该窗的 `[Start, End)` 伸进下一段 `me` 时，重叠部分改为 `uncertain`，剩余 `me` 再审一次。回声和原始底线造成的 `not_me` 保持否决。不按固定延迟平移。

`MatchSpansRelaxed` 只在 `relax` 为真且高低阈值之间有空间时降低进入阈值。降幅是间隔的一半，最多 0.3。放宽产生的 `me` 不能当 `Anchor`。

失败、空音频、短于一窗、panic，都变成覆盖全长的一段 `uncertain`。不返回空切片来表示失败，除非输入长度小于等于 0。

### 实时标注

`appendSpeech` 在入队前冻结档案快照、设备、回声开关和系统参考。完成回调不再读参考环。采集回调不做嵌入。

`identitySpans` 优先 `MatchSpansContext`。没有该接口才用 `MatchSpansRelaxed`。区间不能覆盖全长时，整段视为 `uncertain`。

`utteranceVerdict`：

- 全部为 `me` 且 `spanMeAllowed` 通过：`voiceIdentity` 为 `me`。每个 `me` 都有 `Anchor` 才允许给下一句做种子。
- 既有 `me` 又有其他身份：`voiceMatch` 为 `partial`。不写 `voiceIdentity`。不能做种子。
- 没有 `me`，或版本 3 的设备、回声与冻结上下文不兼容：两者都空。版本低于 3 且录音没有存采集上下文时，仍允许旧标签。

文本始终保存。分数非有限则整句当未知。同一次 VAD 里有多人时，保留整句，标 partial，不拆说话人。

短间隔放宽由 `voiceAnchor` 记住上一句。条件：同一 `session`、同一设备、间隔大于等于 0 且小于 `relaxGap`（2 秒）、本句参考完整且 RMS 不高于 -50 dBFS、上一句是未放宽的高置信 `me`。设备或回声变化时 `clear`。partial、未知、不完整参考都会清掉种子。

批量路径用 `MatchSpansRelaxed`，不用 `MatchSpansContext`。因此批量不向内存系统池学习，也不写适应池。

### 回声、系统池、适应

`echoWindows` 是默认的 `EchoWindows`。系统窗 RMS 不高于 `echoSilenceDB`（-50）不是回声。否则满足其一即是回声：滞后后的系统嵌入余弦不低于麦克风分数减 `echoMargin`（0.05），或包络相关至少 `echoRhoMin`（0.6）且麦克风分数低于模型阈值加 `echoSlack`（0.1）。滞后搜索 `echoMaxLagMS`（300），帧长 `echoFrameMS`（10）。

已录入且未关闭转写时，`referenceWanted` 为真。此时请求内存中的系统参考，不因此创建系统音频文件。阶段是 `off`、`starting`、`available`、`unavailable`。参考失败只降能力，麦克风录音继续。

`echoReference` 保留最近 `echoKeep`（30 秒 × `audio.SampleRate`）样本，并记录每格是否真实覆盖。缺口补零。`SliceWithCoverage` 只有每格都覆盖才标 `Complete`。参考时钟用包的 `CapturedAt`。与上一包推算时间相差超过 `referenceClockSlack`（150 毫秒）就重新锚定，中间留下未覆盖孔。不用硬件 PTS。停止时先排空队列。`CaptureEpoch` 不符的迟到包在 `consume` 丢弃。

`runtimeState.system` 最多 `systemPoolCap`（500）条，只在内存。与本人分数达到 `Threshold(ModelName)` 的向量不入池。池只追加否决：余弦达到该阈值，且不低于窗口分减 `echoMargin`。池不能把已拒绝的窗改成 `me`。档案、设备或回声变化时清空。

适应默认关。`SetAdaptationEnabled` 改变时使已取出的许可失效。学习条件在 `adaptationCandidates`：未放宽，参考完整且确认静音，原始余弦至少 `adaptationRaw`（0.8），归一化分至少 `max(high+1, adaptationFloor)`，`adaptationFloor` 为 3，连续至少 `adaptationRun`（3）秒，且窗口整个落在未加分的生产 `me` 段内。交界处被改成 `uncertain` 的窗不学习。

`applyBoost` 只加到原始分不低于模型阈值、且当前分不低于低阈值、且未被回声或底线挡住的窗。加分最多 `adaptationBoost`（0.3），不能越过 `asNormMax`。`ClearAdaptation` 清池和计数，保留原注册。录入进行中不能清。写盘失败则不更新内存档案。

`AssistedClips` 只在整段因此从非 `me` 变成 `me` 时加一。它不是识别率。

### Cohort

`LoadCohort` 读内嵌 `cohort_eres2netv2.f32` 和 `cohort_meta.json`。`cohortVectorCount`（288），`cohortPerSpeaker`（32），9 个说话人：`SSB0193`、`SSB0407`、`SSB0686`、`SSB0702`、`SSB0711`、`SSB0716`、`SSB0717`、`SSB0736`、`SSB0749`。留出 `SSB0434`、`SSB0544`、`SSB0693`、`SSB0700`，生成和加载都拒绝他们出现在 cohort 里。

格式 `float32-le`，数据集 AISHELL-3，`https://openslr.org/93/`，许可 Apache-2.0。`LICENSE.txt` 和 `NOTICE.txt` 随包内嵌。元数据要求窗 1.5 秒、最短 0.8 秒、步长 0.5 秒。字节长度或 SHA-256 不符则 `LoadCohort` 失败，版本 3 校准和归一化都不可用。

重新生成用 `apps/desktop/internal/voiceprint/cmd/gen-cohort`。参数 `-wav`、`-model`、`-out`。模型文件名必须是 `ModelName`。每个说话人均匀抽取 32 窗。这是离线工具，应用运行时不调用它。

### 导出

`ExportVoiceprintSamples` 按历史 ID 找已完成录音。录音、转写或归档未结束时拒绝。目标必须是绝对路径上的真实目录，不能是符号链接。

只复制该条和同一会话的系统或麦克风对侧文件。对侧要么时间戳相同，要么采集区间重叠。文件必须是录音根目录内的常规 WAV 或 FLAC。新目录名是 `voiceprint-sample-<UTC 时间>-<序号>`。旁边写 `samples.json`：文件名、音源、会话、设备、回声、采集区间，以及空的 `identityRanges`。不写文稿、预测身份或向量。复制失败删除本次新目录。

## 约束与失败行为

- 声纹错误不清文本，不删 WAV。实时队列满时音频已写入，只缺转写，见 [asr.md](asr.md)。
- 版本 3 换麦克风或回声开关后，`Compatible` 为假。文本仍在，徽章去掉。界面用 `VoiceprintContextMismatch`。
- 批量标注不学习。旧文件要更新标签，必须重新转写。
- 适应不能绕过低阈值、回声、设备绑定和原始余弦底线。
- 校准门槛是这批注册样本和这份 cohort 的门槛。不是房间里的准确率承诺。
- 1.5 秒窗有前瞻。身份边界不是字词边界，也不是说话人切换边界。
- 参考时钟用到达时间。偏差超过 150 毫秒留孔，不把孔当静音。硬件对齐没有在本路径里验证。
- 公开朗读和合成回放不能代替本机房间。四人留出只相对这份 cohort 和本次注册。历史阈值见过同一套公开语料。

## 验证

在 `apps/desktop` 下：

```text
go test ./internal/voiceprint/ -count=1 -timeout 180s -run 'TestProfilePersistenceIsolationAndClear|TestVersion1ProfileStillLoads|TestCAMPlusProfileRequiresReenrollment|TestCalibrateSeparatesStableSegmentsFromThePublicCohort|TestCalibrateRejectsUnresolvedDevicesAndSingleSegment|TestBrokenV3DoesNotAcceptRawMe|TestLegacyProfilesKeepRawScores|TestDecideRequiresFractionAndMean|TestIdentitySpansMeOtherMe|TestBoundaryGuardUsesWindowSupport|TestNegativeTrialNormOmitsQuerySpeaker'
go test ./internal/desktop/ -count=1 -timeout 120s -run 'TestVoiceprintEnrollment|TestVoiceprintExport'
go test ./internal/recorder/ -count=1 -timeout 180s -run 'TestVoiceprintLabelsMicrophoneWithoutDroppingText|TestConfidenceChangesKeepOneUtterance|TestLiveMatchSpansContextReceivesFrozenCapture'
```

需要本机模型和公开语音的测试不作为默认门禁：`TestNativeSpeakerVerification`、`TestNativeCalibratedPipelineContract`、`TestNativeVoiceprintEnrollment`。

录入设备一致性回归：`go test ./internal/desktop/ -race -count=20 -run 'TestEnrollmentRejectsDeviceChangeOnEveryStep|TestEnrollmentRejectsEchoChangeBeforeCapture'`。每个设备或回声变化步骤在虚拟时间中运行，避免加速模拟音频与 CI 调度竞争先触发溢出或超时；仍要求错误类型、嵌入次数、采集关闭及未保存档案均正确。

链接：`bun run check:docs`
