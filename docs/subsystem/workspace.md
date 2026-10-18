# 工作区

Bun workspace 放桌面端、营销站和前后端共用的默认值。桌面端用 mygo 把 Go 服务和 React 渲染器打成一个原生应用。

## 职责与边界

- 根目录 `package.json` 声明 workspace `apps/*` 与 `packages/*`，包管理器钉在 Bun 1.3.2。
- `apps/desktop` 是桌面应用。Go 入口是 `apps/desktop/main.go` 的 `main`。前端在 `apps/desktop/src/renderer`。
- `apps/web` 是静态营销站，不链接桌面进程。见 [web.md](web.md)。
- `packages/shared` 只导出生成的默认设置、默认状态和契约类型。包名 `@daybee/shared`。
- `apps/desktop/scripts/desktop.mjs` 包住 generate、dev、build、test、vet。它拒绝交叉编译，并要求本机 Go 1.27.1。
- 仓库规则是单个源文件不超过 500 行，写在 `AGENTS.md`。
- 仓库里没有 Electron 运行时。旧版只作为配置导入来源和 `e2e/baseline` 里的截图基准存在。

## 权威来源

- 项目名称为 DayBee，源码仓库为 `nexmoe/DayBee`；workspace 包名沿用小写的 `daybee-*` 和 `@daybee/*`。
- 脚本入口是仓库根 `package.json` 和 `apps/desktop/package.json`。
- 打包身份在 `apps/desktop/mygo.config.ts` 的 `defineConfig`：产品名取 `productName`，标识 `build.nexmoe.daybee.desktop`，版本取 `apps/desktop/package.json` 的 `version`（当前 1.1.0）。
- 前后端契约的类型在 `apps/desktop/src/renderer/src/generated/mygo.ts`。默认值在 `packages/shared/src/defaults.ts`，由 `apps/desktop/cmd/contracts/main.go` 的 `main` 写出。
- Go 模块路径是 `github.com/nexmoe/DayBee/apps/desktop`，见 `apps/desktop/go.mod`。

## 运行路径

### 安装与生成

`bun install --frozen-lockfile` 安装 workspace。

`bun run --cwd apps/desktop generate` 进入 `apps/desktop/scripts/desktop.mjs`。它先跑 `prepareNative`，再执行 `go run ./cmd/contracts`，然后调用 mygo CLI 的 `generate`。

`cmd/contracts` 调用 `protocol.DefaultSettingsForPlatform("recordings", "darwin")` 和 `protocol.DefaultStatus`，把 JSON 写进 `packages/shared/src/defaults.ts`。共享文件固定使用参考平台的默认值，并将 ASR 设为 Tiny 后备值，不能随生成机器变化；应用启动后的设置来自原生 `Bootstrap`，仍遵守本机能力。mygo 根据 `mygo.config.ts` 的 `bindings` 生成 `src/renderer/src/generated/mygo.ts`。CI 在生成后对这两个文件做 `git diff --exit-code`。

改契约要改 Go 声明，不要手改生成文件。

### 开发

根脚本 `dev` 和 `dev:desktop` 都进 `apps/desktop` 的 `dev`。该脚本固定走 portless：`PORTLESS=1 portless run --name daybee node scripts/desktop.mjs dev`。

`desktop.mjs` 在 `dev` 时要求已有 `PORTLESS_URL` 和 `PORT`。缺了就退出。`PORTLESS=0` 绕不过这个脚本。

`mygo.config.ts` 把 `devUrl` 设为 `PORTLESS_URL`，`devCommand` 是 `bunx --no-install vite`。Vite 配置在 `apps/desktop/vite.config.ts`：根目录 `src/renderer`，监听 `127.0.0.1` 上的 `PORT`，HMR 走 portless 的源。非 E2E 的 `serve` 同样要求这两个环境变量。

macOS 在 `mygo dev` 使用 HTTP 开发地址时，`mygo.config.ts` 只为该地址的主机名写入 ATS 的 `NSExceptionAllowsInsecureHTTPLoads` 例外，让 WebView 能加载本地页面。HTTPS 开发地址、generate 和生产 build 不写入这项例外。

`apps/desktop` 的 `dev:web` 只起 Vite，同样经过 portless。Playwright 是例外，见 [renderer.md](renderer.md)。

### 构建

`build` 与 `build:desktop` 调用 `node scripts/desktop.mjs build`。macOS 上它设置 `CGO_ENABLED`（1）、`MACOSX_DEPLOYMENT_TARGET`（13.3），以及同样的 `CGO_CFLAGS` 和 `CGO_LDFLAGS`。

`prepare-native.mjs` 的 `prepareNative` 只接受本机目标：`darwin-arm64`、`darwin-amd64`、`windows-amd64`。它下载锁定的 sherpa 平台模块 1.13.8，把许可证和 `native-versions.json` 放到 `apps/desktop/resources/<target>`。macOS 在 `.generated` 里复制一份 sherpa 模块，把绝对 rpath 换成 `@executable_path/../Resources/lib`，并用 `-modfile` 指向这份副本。Go 模块缓存不被改写。dylib 先去掉上游签名，再尝试 `strip -x`；旧 Xcode 无法处理上游库的 `__LINKEDIT` 时，从缓存恢复干净副本并去掉签名，保留完整符号，记录警告后继续构建。签名移除失败仍终止构建。

Windows 将原生库目录添加到已有的 `Path` 或 `PATH` 前面，保留原来的键名和搜索目录，并移除大小写冲突的重复键；不能因为 DLL 路径覆盖 Go、Bun 和 C 编译器的搜索路径。

Windows 的 Go 测试通过 `apps/desktop/scripts/native-test-exec.mjs` 启动：先将锁定的 DLL 复制到每个临时测试可执行文件旁，再执行测试，避免 Windows 优先加载 System32 里的旧 ONNX Runtime。打包应用本来就把 DLL 放在可执行文件旁。原生版本校验仍要求 1.28.2，不接受系统旧版本。

mygo 再执行 `buildCommand`（`bun run build:web`）并把前端产物从 `out/renderer` 打进 `dist`。macOS 最低系统版本、签名身份、entitlements 和可选公证都在 `mygo.config.ts`。Windows 证书和签名命令只在对应环境变量存在时写入。

macOS 同时由 `apps/desktop/scripts/prepare-speech.mjs` 编译 `apps/desktop/internal/speechanalyzer/native/SpeechBridge.swift`，需要包含 SpeechAnalyzer 的 macOS 26+ SDK。产物 `libdaybee-speech.dylib` 放进同一资源 `lib` 目录，开发和测试通过 `DAYBEE_SPEECH_LIBRARY` 定位，打包后从可执行文件旁的 Resources/lib 加载。桥接的最低目标仍为 13.3，运行时检查 26+ 的 API；Windows 不编译或加载它。`mygo.config.ts` 提供系统语音识别用途说明。

macOS 配置了公证 profile 时，`desktop.mjs` 先用 mygo 的 `--skip-notarize` 完成签名构建，再由 `apps/desktop/scripts/notarize.mjs` 提交最终架构命名的 DMG。Apple 必须返回 `Accepted`；票据装订错误 65 或 68、验证票据时的网络错误 68，各自最多重试三次，分别等待 5、10、20 秒。重试不重新提交或重新签名，验证重试也不重新装订。最后必须通过 `stapler validate`。拒绝、公证失败、其他装订或验证错误、重试耗尽均以非零退出，阻止发布。

`DAYBEE_DEFER_NOTARIZATION`（1）只推迟公证，不影响 Developer ID 签名。CI 的 Intel Mac 构建使用它，避免该 runner 访问 Apple 票据服务时持续断连；三平台原生检查完成后，`notarize-intel` 在 Apple Silicon runner 上下载已签名的 Intel 安装包，再公证、装订和验证。该作业不编译或执行 Intel 应用，凭据通过 `signing.mjs --notary-only` 导入，只需要已有的 Apple API 密钥。

更新公钥默认读 `apps/desktop/build/mygo-update.pub`，可用 `DAYBEE_UPDATER_PUBLIC_KEY` 覆盖。配置指向 GitHub `nexmoe/DayBee`，标签前缀 `v`。

### 发布检查

`.github/workflows/desktop-release.yml` 在 `macos-15`、`macos-15-intel`、`windows-latest` 上跑同一套原生检查。标签 `v*` 才为 macOS 导入 Developer ID 和公证材料，并为三个平台带上 `MYGO_UPDATER_PRIVATE_KEY`。Windows 安装包不做 Authenticode 证书签名；更新归档仍必须通过 Ed25519 签名校验。

三个 native job（含打包后的原生窗口与 MCP 标准输入输出验收）和 `notarize-intel` 都通过后，`publish` 用 `apps/desktop/scripts/release-gates.mjs --report` 在日志及 Actions summary 中报告未完成的人工迁移验收。它先下载原生制品，再把已公证的 Intel DMG 下载到同一目标目录，替换原始 DMG 后收集发布文件。沿用已确认的自动检查发布约定，人工报告不阻止发布，也不改写 `docs/migration-verification.json` 的结果。直接运行脚本仍严格检查每个目标的界面、原生推理、最低系统、干净安装、签名更新恢复及至少 8 小时实录，未完成时返回非零。

`apps/desktop/scripts/collect-release.mjs` 必须验证标签与包版本一致、三个目标的更新版本与包版本一致、更新归档的长度与签名正确、各平台安装包齐全且无同名内容冲突，才会生成校验和并交给 GitHub Release 发布。

GitHub 会规范化上传资产的文件名。收集阶段先将空格替换为点号，再检查重名、复制文件及生成校验和，保证 `SHA256SUMS.txt` 引用实际公开下载名；原生构建目录里的文件名不受影响，更新归档及签名内容不变。

## 约束与失败行为

- `desktop.mjs` 比对 `go version` 输出里的 `go1.27.1 `。版本不对就退出。
- `DAYBEE_BUILD_TARGET` 必须等于当前平台和架构。否则报交叉编译不支持。
- `go run ./cmd/contracts` 失败时，`desktop.mjs` 用它的退出码结束，不继续 generate 或 build。
- 契约生成器或 mygo/Go 子进程无法启动时，脚本输出底层启动错误，避免只留下退出码 1。
- 首次构建需要网络拉取 Go 模块。之后复用模块缓存和 `.generated`。
- 编解码不依赖 FFmpeg。`prepareNative` 会删掉资源目录里遗留的 ffmpeg 路径。版本钉在 `prepare-native.mjs`：sherpa `version`（1.13.8），ONNX Runtime 1.28.2，malgo 0.11.26，FLAC 1.0.14。Go 源里的 `OnnxRuntimeVersion`（1.28.2）在 `apps/desktop/internal/asr/runtime.go`。
- 不支持 Linux 和交叉编译。Windows x64 用 MSYS2 的 MINGW64 GCC、`make`、`diffutils`，CI 里由 `msys2/setup-msys2` 安装；应用用 WebView2。
- 开发与打包默认共用 DayBee 数据目录。隔离方式见 [desktop-shell.md](desktop-shell.md)。

## 验证

这些命令都在仓库根执行，除非另写工作目录。

```bash
bun install --frozen-lockfile
bun run --cwd apps/desktop generate
git diff --exit-code -- apps/desktop/src/renderer/src/generated/mygo.ts packages/shared/src/defaults.ts
bun run typecheck
bun run test
bun run --cwd apps/desktop test:release
node apps/desktop/scripts/desktop.mjs vet
node apps/desktop/scripts/release-gates.mjs
```

`test:release` 跑 `apps/desktop/scripts/release-artifacts.test.mjs`、`apps/desktop/scripts/prepare-native.test.mjs` 和 `apps/desktop/scripts/notarize.test.mjs`，覆盖版本与标签匹配、安装包完整性、更新签名、文件冲突、Windows 路径保留、strip 失败后的原始库恢复及公证票据装订、验证的有限重试。`release-gates.mjs` 默认在人工门未通过时以非零退出；`--report` 保留这些事实并正常退出。

链接：`bun run check:docs`
