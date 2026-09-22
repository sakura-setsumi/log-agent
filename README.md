# Log Agent

> A focused, self-hosted dashboard for aggregating, searching, processing, and analyzing logs from multiple [Dozzle](https://dozzle.dev/) instances.

**English (default)** · [中文 / Chinese](#中文)

## Overview

Log Agent is a single-binary Go service with an embedded HTML/CSS/JavaScript dashboard. It connects to one or more Dozzle v10 instances, combines their live streams, and keeps a bounded in-memory history for fast investigation.

Highlights:

- Multi-node Dozzle log stream with container selection, level filters, full-range search, CSV export, and pause/resume.
- Time-window history loading with automatic older-page loading after the per-container browse limit is reached.
- Processing pipeline for sensitive-data masking, structured-field extraction, and health-check filtering.
- AI log analysis with selectable context, image/text attachments, model profiles, and browser-local conversation history.
- Local node/model storage for localhost use, with browser-local storage for remote visitors.
- No database required; frontend assets are embedded into the Go executable.

## Quick start

Requires Go 1.22 or newer for development:

```bash
go run .
```

Open <http://localhost:8099>.

For a stable installation, build and run the executable instead of `go run .`:

```powershell
.\scripts\build.ps1
.\dist\dozzle-ops.exe
```

The service listens on `:8099` by default. Set `LOG_AGENT_PORT` or `PORT` to use another port.

The latest Windows amd64 executable is available in the [Releases](https://github.com/sakura-setsumi/log-agent/releases) page.

## Configuration

The application stores node connections and AI providers as JSON files in a `data` directory beside the executable:

```text
dozzle-ops.exe
data/
├─ nodes.json
└─ models.json
```

When the dashboard is opened through localhost, this local directory is used. When it is opened from another machine, that visitor's browser `localStorage` is used instead. This keeps remote visitors' connections and API keys isolated from the host machine.

Useful environment variables:

| Variable | Purpose |
| --- | --- |
| `LOG_AGENT_CONFIG_DIR` | Override the data directory. |
| `LOG_AGENT_SETTINGS_FILE` | Choose the runtime settings JSON file. |
| `LOG_AGENT_ADMIN_TOKEN` | Preconfigure the administrator token. |
| `LOG_AGENT_ENVIRONMENT` | Set the displayed runtime environment. |
| `LOG_AGENT_PORT` / `PORT` | Change the listening port. |
| `LOG_AGENT_MAX_STORED_LOGS` | Set the global in-memory cache limit (1–1,000,000). |

Example:

```powershell
$env:LOG_AGENT_CONFIG_DIR = 'D:\log-agent-config'
$env:LOG_AGENT_PORT = '9099'
.\log-agent.exe
```

`models.json` contains API keys in plain text. Keep it outside public repositories and backups that are shared with others.

## Development and tests

Run the complete test suite:

```bash
go test ./...
```

The end-to-end tests start an isolated temporary service and use a local Chrome or Edge session. They do not use port 8099 or real Dozzle nodes.

Build the Windows executable with tests:

```powershell
.\scripts\build.ps1
```

Build quickly without tests:

```powershell
.\scripts\build.ps1 -SkipTests
```

Build output is written to `dist/`; local binaries and caches are intentionally ignored by Git.

## Architecture

- `main.go` — HTTP API, Dozzle connectors, SSE ingestion, history paging, cache policy, and embedded frontend.
- `app.js` — dashboard state, node/container selection, log stream, filtering, paging, and export.
- `assistant-ui.js` — AI assistant conversations, context, attachments, and session history.
- `bootstrap.js` — application initialization after frontend modules load.
- `styles.css` — dashboard styling and responsive layout.

The backend connects to real Dozzle v10 APIs for container metadata, historical logs, and SSE streams. A Dozzle root URL or a specific `/container/<container-id>` URL can be used when adding a node.

## Security notes

- Dozzle endpoints and AI provider URLs are user-configured; use HTTPS and network access controls in production.
- Do not commit `data/models.json`, exported configuration files, private keys, or administrator tokens.
- The service does not modify Nginx or host system configuration.

## License

No license file is currently included. Contact the repository owner before redistributing the project.

<a id="中文"></a>

<details>
<summary>中文说明（点击展开）</summary>

## 项目简介

Log Agent 是一个面向 Dozzle 的自托管日志聚合、检索、加工和分析面板。服务端使用 Go 编写，前端 HTML/CSS/JavaScript 会嵌入最终可执行文件。

主要能力：

- 聚合多个 Dozzle 节点，支持容器选择、级别筛选、完整时间范围搜索、CSV 导出和暂停/继续接收。
- 按时间窗口拉取历史日志；单容器浏览缓存达到上限后，滚动到顶部会自动加载更早日志。
- 提供敏感信息脱敏、结构化字段提取和健康检查过滤等日志加工规则。
- 支持 AI 日志分析、上下文选择、图片/文本附件、模型配置和浏览器本地会话历史。
- 本机访问时使用可执行文件旁的本地 JSON 配置；远程访问时使用访客浏览器的 localStorage。
- 无需数据库，前端资源直接嵌入 Go 可执行文件。

## 快速启动

开发环境需要 Go 1.22 或更高版本：

```bash
go run .
```

浏览器访问 <http://localhost:8099>。正式运行建议先构建可执行文件：

```powershell
.\scripts\build.ps1
.\dist\dozzle-ops.exe
```

默认监听 `:8099`，可通过 `LOG_AGENT_PORT` 或 `PORT` 修改端口。Windows amd64 最新可执行文件位于 [Releases](https://github.com/sakura-setsumi/log-agent/releases)。

## 配置和安全

节点和 AI 供应商默认保存在可执行文件旁的 `data/nodes.json` 与 `data/models.json`。可使用 `LOG_AGENT_CONFIG_DIR`、`LOG_AGENT_SETTINGS_FILE`、`LOG_AGENT_ADMIN_TOKEN`、`LOG_AGENT_ENVIRONMENT`、`LOG_AGENT_PORT` 和 `LOG_AGENT_MAX_STORED_LOGS` 覆盖默认设置。

`models.json` 会以明文保存 API key，请勿提交到公开仓库，也不要共享导出的敏感配置、私钥或管理员 token。服务不会修改 Nginx 或主机系统配置。

## 开发测试

```bash
go test ./...
```

使用 `scripts/build.ps1` 构建 Windows 程序；使用 `scripts/build.ps1 -SkipTests` 可跳过测试。构建输出在 `dist/`，本地生成物会被 Git 忽略。

## 代码结构

- `main.go`：HTTP API、Dozzle 连接、SSE 日志接收、历史分页、缓存策略和嵌入式前端。
- `app.js`：页面状态、节点/容器选择、日志流、筛选、分页和导出。
- `assistant-ui.js`：AI 助手会话、上下文、附件和历史记录。
- `bootstrap.js`：前端模块加载完成后的初始化。
- `styles.css`：面板样式和响应式布局。

</details>
