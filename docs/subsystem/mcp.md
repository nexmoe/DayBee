# MCP 文稿读取

DayBee 内置本地 stdio MCP 服务，让 MCP 客户端读取应用已经保存的转写文稿。

## 职责与边界

- `apps/desktop/main.go` 在收到 `--mcp` 参数时调用 `desktop.RunMCP`，不启动窗口、托盘、录音引擎或单实例锁。桌面应用关闭时也能读取已有文稿。
- `apps/desktop/internal/desktop/mcp.go` 解析当前应用的数据目录和录音目录，生成客户端配置；读取过程不创建或修复设置文件。
- `apps/desktop/internal/transcriptmcp/server.go` 用官方 Go MCP SDK（`v1.8.0`）注册三个只读工具、文稿索引资源和文稿资源模板；`protocolVersion`（`2026-07-28`）是唯一支持的协议版本。
- `apps/desktop/internal/transcriptmcp/catalog.go` 负责全文筛选、按实际录音时间排序、分页、摘要和文稿 ID。
- `apps/desktop/src/renderer/src/components/mcp-settings.tsx` 在「通用」页显示和复制连接配置。配置中的命令是当前运行的 DayBee 可执行文件，参数为 `--mcp`；设置了 `DAYBEE_STORE_DIR` 时保留该隔离目录，并把路径规范化为本机的绝对路径。
- MCP 不启动或停止录音，不补转写，不修改文稿、设置或模型。原始音频不作为 MCP 资源返回。

## 权威来源

- 沿用 `recorder.History` 读取当前输出目录、默认 DayBee 文稿目录和非隔离运行的旧默认录音目录。文件契约见 [recording.md](recording.md)。
- 每次请求重新读取 `desktop-settings.json` 的 `settings.recording.outputDir` 并扫描文件；切换输出目录、录音期间完成的新转写均无需重启 MCP 服务。没有第二份文稿数据库或缓存。
- 文稿全文来自历史的 `TextPreview`，该字段实际保留全文；缺少正文但存在转写片段时使用片段正文。纯音频记录不进入文稿列表。
- ID 是规范化音频路径去掉扩展名后的 SHA-256。WAV 收尾为同路径 FLAC 后 ID 不变。文稿资源 URI 为 `daybee://transcripts/{id}`。工具只查已发现的 ID，不接受任意文件路径。

## 运行路径

在 DayBee 的「通用 → MCP · AI 连接」复制配置，合并到支持 stdio 的 MCP 客户端配置中。例如 macOS 安装位置：

```json
{
  "mcpServers": {
    "daybee": {
      "command": "/Applications/DayBee.app/Contents/MacOS/DayBee",
      "args": ["--mcp"]
    }
  }
}
```

Windows 使用安装目录中的 `DayBee.exe`；应用界面生成实际路径。测试、自定义数据目录可通过连接配置的 `env.DAYBEE_STORE_DIR` 选择对应数据。

客户端启动命令后，SDK 在 stdin/stdout 上处理 MCP 工具和资源请求。采用 [MCP `2026-07-28`](https://modelcontextprotocol.io/specification/2026-07-28) 的无状态模式；客户端可先调用 `server/discover` 获取支持的版本、能力和使用说明，也可直接调用工具或资源。每个请求的 `params._meta` 必须包含 `io.modelcontextprotocol/protocolVersion` 和 `io.modelcontextprotocol/clientCapabilities`，建议同时提供 `io.modelcontextprotocol/clientInfo`；每个结果通过 `_meta.io.modelcontextprotocol/serverInfo` 标识 DayBee。

服务发现的 `supportedVersions` 只返回 `2026-07-28`。旧版 `initialize` 请求返回 `-32022`，错误消息说明所需的协议版本，错误数据列出唯一支持的版本；请求不会进入旧版会话。stdout 仅承载 JSON-RPC 消息，诊断写入 stderr；stdin 关闭后进程退出。

| 工具 / 资源 | 输入与返回 |
| --- | --- |
| `list_transcripts` | 可选 `date`、`source`、`offset`、`limit`。返回 `items`、`total` 和可选 `nextOffset`，最新文稿在前。每项包含 ID、资源 URI、录音时间、来源、会话、字符数和摘要。 |
| `search_transcripts` | 必填 `query`，其余参数同列表。按不区分大小写的字面文本搜索全部正文，摘要围绕命中位置。 |
| `read_transcript` | 必填列表或搜索返回的 `id`。返回完整 `text`、录音时间、来源、音频路径、会话、设备及带说话人和起止时间的 `chunks`。不截断全文。 |
| `daybee://transcripts` | `resources/read` 返回默认第一页文稿索引；更多结果通过列表工具分页。 |
| `daybee://transcripts/{id}` | 资源模板；`resources/read` 返回与全文工具相同的完整 JSON 文稿。 |

典型调用顺序：用 `search_transcripts({"query":"项目会议"})` 找到 ID，再用 `read_transcript({"id":"返回的 ID"})` 阅读全文。

## 约束与失败行为

- `limit` 缺省或为 0 时取 20，上限 100；负数、超限及负 `offset` 返回工具错误。`nextOffset` 缺失代表没有下一页。
- `date` 是有效的 `YYYY-MM-DD`，按录音开始时间在本机时区中的日期筛选。`source` 只接受 `microphone` 或 `system`。空查询、未知 ID 和非法参数返回错误。
- 设置文件不存在时使用默认输出目录，不写入默认设置。设置损坏或不可读时返回明确错误，不把它覆盖为默认配置。
- 历史文件扫描的损坏文件跳过规则沿用 `recorder.History`。请求取消会在读取前后检查。
- 新版协议逐请求验证元数据；缺少必填字段返回 `-32602`，不支持的版本返回 `-32022` 并列出支持的版本，未知文稿资源返回 `-32602`。之前请求的元数据不会代替后续请求中的必填字段。
- 新版协议中服务发现、工具和资源列表、资源正文均返回 `cacheScope: "private"` 和 `ttlMs: 0`；结果仅供当前用户的客户端缓存且立即过期，后续读取重新扫描文稿以获取增量转写和输出目录变化。
- 每个客户端持有自己的 stdio 进程；不监听网络端口，不依赖桌面应用处于打开状态。三个工具均声明 `readOnlyHint`，文稿正文是数据，不作为服务指令执行。

## 验证

仓库根执行：

```bash
bun run --cwd apps/desktop test:go
bun run build
bun run --cwd apps/desktop test:mcp
bun run --cwd apps/desktop test:e2e -- mcp.spec.ts
bun run typecheck
bun run check:docs
```

协议测试覆盖 `2026-07-28` 无初始化调用、逐请求元数据校验、版本错误与重试信息、私有缓存策略，以及四个旧协议版本的初始化和逐请求版本拒绝；同时验证工具和资源发现、完整正文、说话人时间戳、字面搜索、分页、日期和来源筛选、音频收尾后 ID 稳定性及非法路径拒绝。原生 stdio 验收用 `2026-07-28` 连接真实可执行文件，在隔离文稿目录验证旧版初始化拒绝、增量转写和输出目录切换；不会读取用户真实录音。
