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
- 打包身份在 `apps/desktop/mygo.config.ts` 的 `defineConfig`：产品名取 `productName`，标识 `build.nexmoe.daybee.desktop`，版本取 `apps/desktop/package.json` 的 `version`（当前 1.0.0）。
- 前后端契约的类型在 `apps/desktop/src/renderer/src/generated/mygo.ts`。默认值在 `packages/shared/src/defaults.ts`，由 `apps/desktop/cmd/contracts/main.go` 的 `main` 写出。
- Go 模块路径是 `github.com/nexmoe/DayBee/apps/desktop`，见 `apps/desktop/go.mod`。

## 运行路径

### 安装与生成

`bun install --frozen-lockfile` 安装 workspace。

`bun run --cwd apps/desktop generate` 进入 `apps/desktop/scripts/desktop.mjs`。它先跑 `prepareNative`，再执行 `go run ./cmd/contracts`，然后调用 mygo CLI 的 `generate`。

`cmd/contracts` 调用 `protocol.DefaultSettings` 和 `protocol.DefaultStatus`，把 JSON 写进 `packages/shared/src/defaults.ts`。mygo 根据 `mygo.config.ts` 的 `bindings` 生成 `src/renderer/src/generated/mygo.ts`。CI 在生成后对这两个文件做 `git diff --exit-code`。

改契约要改 Go 声明，不要手改生成文件。

### 开发

根脚本 `dev` 和 `dev:desktop` 都进 `apps/desktop` 的 `dev`。该脚本固定走 portless：`PORTLESS=1 portless run --name daybee node scripts/desktop.mjs dev`。

`desktop.mjs` 在 `dev` 时要求已有 `PORTLESS_URL` 和 `PORT`。缺了就退出。`PORTLESS=0` 绕不过这个脚本。

`mygo.config.ts` 把 `devUrl` 设为 `PORTLESS_URL`，`devCommand` 是 `bunx --no-install vite`。Vite 配置在 `apps/desktop/vite.config.ts`：根目录 `src/renderer`，监听 `127.0.0.1` 上的 `PORT`，HMR 走 portless 的源。非 E2E 的 `serve` 同样要求这两个环境变量。

`apps/desktop` 的 `dev:web` 只起 Vite，同样经过 portless。Playwright 是例外，见 [renderer.md](renderer.md)。

### 构建

`build` 与 `build:desktop` 调用 `node scripts/desktop.mjs build`。macOS 上它设置 `CGO_ENABLED`（1）、`MACOSX_DEPLOYMENT_TARGET`（13.3），以及同样的 `CGO_CFLAGS` 和 `CGO_LDFLAGS`。

`prepare-native.mjs` 的 `prepareNative` 只接受本机目标：`darwin-arm64`、`darwin-amd64`、`windows-amd64`。它下载锁定的 sherpa 平台模块 1.13.8，把许可证和 `native-versions.json` 放到 `apps/desktop/resources/<target>`。macOS 在 `.generated` 里复制一份 sherpa 模块，把绝对 rpath 换成 `@executable_path/../Resources/lib`，并用 `-modfile` 指向这份副本。Go 模块缓存不被改写。dylib 先去掉上游签名，再 `strip -x`。

mygo 再执行 `buildCommand`（`bun run build:web`）并把前端产物从 `out/renderer` 打进 `dist`。macOS 最低系统版本、签名身份、entitlements 和可选公证都在 `mygo.config.ts`。Windows 证书和签名命令只在对应环境变量存在时写入。

更新公钥默认读 `apps/desktop/build/mygo-update.pub`，可用 `DAYBEE_UPDATER_PUBLIC_KEY` 覆盖。配置指向 GitHub `nexmoe/DayBee`，标签前缀 `v`。

### 发布检查

`.github/workflows/desktop-release.yml` 在 `macos-15`、`macos-15-intel`、`windows-latest` 上跑同一套原生检查。标签 `v*` 才导入签名材料并带上 `MYGO_UPDATER_PRIVATE_KEY`。

三个 native job 都通过后，`publish` 先跑 `apps/desktop/scripts/release-gates.mjs`。它读 `docs/migration-verification.json`，要求每个目标的界面、原生推理、最低系统、干净安装、签名更新恢复都为真，并且 `enduranceHours` 至少为 8。当前这份 JSON 没有满足这些门，标签发布会被挡住。

## 约束与失败行为

- `desktop.mjs` 比对 `go version` 输出里的 `go1.27.1 `。版本不对就退出。
- `DAYBEE_BUILD_TARGET` 必须等于当前平台和架构。否则报交叉编译不支持。
- `go run ./cmd/contracts` 失败时，`desktop.mjs` 用它的退出码结束，不继续 generate 或 build。
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

`test:release` 跑 `apps/desktop/scripts/release-artifacts.test.mjs`。`release-gates.mjs` 在门未通过时以非零退出，这是当前数据的预期结果。

链接：`bun run check:docs`
