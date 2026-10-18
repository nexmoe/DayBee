# 营销站

`apps/web` 是 DayBee 的静态英文营销站。它不连接桌面进程，也不读取桌面设置。

## 职责与边界

- 站点只负责介绍、下载入口和搜索元数据。安装包仍从 GitHub Releases 获取。
- 页面在 `apps/web/src/pages`。文案数据集中在 `src/data/site.ts`。
- 没有账号、没有接口、没有内容集合。FAQ、模型名和价格卡都是源码里的常量。
- 桌面端怎么录音和更新，以 Go 实现为准。见 [desktop-shell.md](desktop-shell.md)。这页只记录站点实际写出的句子。

## 权威来源

- 构建配置是 `apps/web/astro.config.mjs`。`output` 为 `static`。`site` 取 `SITE_URL`，缺省 `https://daybee.co`，并去掉末尾斜杠。
- 部署配置是 `apps/web/alchemy.run.ts`，由 Alchemy 管理 Cloudflare Worker、自定义域名和远程部署状态。生产地址是 <https://daybee.co>。
- 可索引路径只有 `indexablePaths` 里的 `/`。
- 产品链接在 `site.github`（`https://github.com/nexmoe/DayBee`）和 `site.releases`（同一仓库的 `/releases`）。

## 运行路径

### 页面

`src/pages/index.astro` 把 Hero、Models、Features、Video、Pricing、Faqs、GetStarted 按顺序放进 `Layout`。没有 `Astro.site` 就抛错。

`404.astro` 使用 `robots` 值 `noindex, follow`，所以 `Layout` 不给它写 canonical。

`Layout.astro` 写标题、描述、Open Graph、Twitter 卡片和图标。默认标题是 `site.title`。OG 图是 `/og.jpg`，尺寸取 `imageWidth`（1200）和 `imageHeight`（630）。JSON-LD 由 `homeJsonLd` 生成，序列化时把 `<` 换成 `<`。

主题色是 `themeColor`（`#ffffff`）和 `themeColorDark`（`#0a0a0a`）。`src/scripts/interactions.ts` 在页内切换 `dark` 类，并把选择写到 `localStorage` 键 `grok-bot-theme`。存储失败被忽略。FAQ 按钮只切换本页的 `aria-expanded` 和面板。

### 生成的文本路由

三个端点都设置 `prerender`（true）：

- `robots.txt.ts` 允许 `/`，附上 `Sitemap`，并写 `Content-Signal: search=yes, ai-input=yes, ai-train=no`。
- `sitemap.xml.ts` 只列出 `indexablePaths`。
- `llms.txt.ts` 输出 `llmsText`：一段产品说明、三个上手步骤、三个模型名和仓库链接。

`site` 缺失时这三个路由返回 500 和文本 `Site URL is not configured.`

### 构建

根脚本 `dev:web` 和 `build:web` 转到 `@daybee/web`。应用自己的脚本是 `astro dev --host 127.0.0.1`、`astro build`、`astro preview --host 127.0.0.1`。`typecheck` 先跑 `astro check`，再按 `apps/web/tsconfig.alchemy.json` 检查部署配置。

Astro 7.3.4，Tailwind 4.3.3。开发工具条在配置里关闭。仓库根的桌面 CI 不构建这个站点。

### 部署

`deploy:web:plan` 和 `deploy:web` 都先构建静态产物，再从 `apps/web` 调用 Alchemy，固定使用 `production` stage。前者只展示计划，后者确认计划后部署；无人值守执行可追加 `--yes`。

Alchemy 2.0.0-beta.83、Effect 4.0.5 和 Bun 平台适配器作为站点开发依赖锁定。默认复用本机 Alchemy 的 `default` Cloudflare profile，可用 `ALCHEMY_PROFILE` 选择其他已登录的 profile；CI 使用 Cloudflare 环境凭据。凭据不写入仓库。

`daybee-web` stack 通过 `Cloudflare.state()` 把状态保存在 Cloudflare 托管的 Worker 和 Durable Object 中，以便不同 checkout 共用。站点资源的逻辑 ID 为 `Website`；生产 Worker 名为 `daybee-web`，其他 stage 由 Alchemy 生成独立名称。`workersDev` 开启稳定地址并关闭版本预览地址。

生产 stage 通过 Worker 的 `domain` 属性绑定 `daybee.co`，由 Cloudflare 管理域名对应的 DNS 记录和边缘证书。其他 stage 不绑定这个域名。原有 <https://daybee-web.nexmoe.workers.dev> 仍可访问同一份静态站点。

部署只上传 `dist` 中的静态文件，不上传请求处理脚本。不存在的路径由 Cloudflare 返回构建出的 `404.html`，状态为 404；HTML 路径采用 `auto-trailing-slash`。兼容日期固定为 `2026-10-18`。仓库忽略 `.alchemy/` 下的本地缓存和日志。

## 约束与失败行为

- `SITE_URL` 必须以 `http://` 或 `https://` 开头，否则配置加载时抛 `SITE_URL must be an absolute http(s) URL`。
- 变更生产主机名时必须同步修改 `apps/web/alchemy.run.ts` 的生产 `domain` 和 `SITE_URL` 的默认值或环境值；`SITE_URL` 只控制生成的搜索元数据。
- 自定义域名的 zone 必须已存在于当前 Cloudflare 账户。注册商的 NS 委派和证书签发完成后，绑定才能在公网通过 HTTPS 访问；Alchemy 不管理注册商的 NS 设置。
- 构建失败时部署脚本不会调用 Alchemy；认证、远程状态或上传失败时部署以非零状态退出。
- 首页文案把系统音频写成 macOS 功能，模型写成 Qwen3 ASR 0.6B、FireRedASR2 AED/CTC、SenseVoice Small（2025）。这些名字与 `protocol` 里的模型常量一致。
- FAQ 写「启动时和每六小时检查签名更新，录音和转写空闲后再安装」。检查间隔与 `updater.go` 一致。安装动作与代码不一致：当前 `InstallUpdate` 由用户触发，并先停止录音，不等待转写结束。见 [desktop-shell.md](desktop-shell.md)。
- Models 区把系统音频的实现写成 ScreenCaptureKit。营销页没有引用桌面采集源码。采集细节不在这页展开。
- `apps/web/src/components/GetStarted.astro` 的 Build from source 和 `apps/web/src/components/Footer.astro` 的 Development 都链到 GitHub 上的 [workspace.md](workspace.md)。
- 页脚 Guides 里的系统音频和回声消除链接到 [audio.md](audio.md)，声纹链接到 [voiceprint.md](voiceprint.md)，麦克风活动链接到 [microphone-activity.md](microphone-activity.md)。
- 站点没有测试文件。`typecheck` 只检查 Astro 页面和 Alchemy 部署配置的类型。

## 验证

在仓库根：

```bash
bun run --cwd apps/web typecheck
bun run build:web
bun run deploy:web:plan
```

预览：

```bash
bun run --cwd apps/web preview
```

链接：`bun run check:docs`

上线：`bun run deploy:web --yes`。成功后 Alchemy 输出站点 URL；再检查首页、静态资源、`robots.txt`、`sitemap.xml`、`llms.txt` 和未知路径的 404 响应。
