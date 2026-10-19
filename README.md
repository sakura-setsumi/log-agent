# 栈桥

> A focused, self-hosted dashboard for aggregating, searching, processing, and analyzing logs from multiple [Dozzle](https://dozzle.dev/) instances.

**English (default)** · [中文 / Chinese](#中文)

## Overview

栈桥 is a single-binary Go service with an embedded HTML/CSS/JavaScript dashboard. It connects to one or more Dozzle v10 instances, combines their live streams, and keeps a bounded in-memory history for fast investigation.

Highlights:

- Multi-node Dozzle log stream with container selection, level filters, full-range search, CSV export, and pause/resume.
- Time-window history loading with automatic older-page loading after the per-container browse limit is reached.
- Processing pipeline for sensitive-data masking, structured-field extraction, and health-check filtering.
- AI log analysis with selectable context, image/text attachments, model profiles, and browser-local conversation history.
- Command Groups page for ordered, reusable server command sets, and a Servers page that is an SSH terminal (xterm.js) with an SFTP file browser, in the style of FinalShell. Select or drop whole folders to upload them with their directory structure.
- Database workspace for MySQL, PostgreSQL and SQLite: connection/schema tree, paged editable tables, a visual table/column/index designer, and one SQL query across multiple databases with combined results and source labels.
- Local node/model storage for localhost use, with browser-local storage for remote visitors.
- No database required; frontend assets are embedded into the Go executable.

## Quick start

Requires Go 1.26.4 or newer for development:

```bash
go run .
```

This is a Windows desktop app: it opens its own window (Microsoft Edge WebView2, preinstalled on Windows 11) and does not listen on any network port or open in a browser. For a stable installation, build and run the executable instead of `go run .`:

```powershell
.\scripts\build.ps1
.\dist\栈桥.exe
```

The window serves its own frontend and talks to the Go handlers in-process, so nothing else on the machine (or network) can reach the dashboard. Only one instance runs at a time; starting a second one brings the first window forward. Links to external sites open in the system browser. Diagnostics go to `log-agent.log` in the data directory, and `LOG_AGENT_DEBUG=1` enables the WebView2 developer tools.

The first start creates a desktop shortcut named **栈桥**. Moving the executable repoints the shortcut on its next start; a deleted shortcut is not recreated. Set `LOG_AGENT_NO_SHORTCUT=1` to skip it.

The latest Windows amd64 executable is available in the [Releases](https://github.com/sakura-setsumi/log-agent/releases) page.

## Configuration

The application stores node connections, AI providers and server-command flows as JSON files in a `data` directory beside the executable:

```text
栈桥.exe
data/
├─ nodes.json
├─ models.json
├─ commands.json
├─ command-logs.json
└─ databases.json
```

When the dashboard is opened through localhost, this local directory is used. When it is opened from another machine, that visitor's browser `localStorage` is used instead. This keeps remote visitors' connections and API keys isolated from the host machine.

Useful environment variables:

| Variable | Purpose |
| --- | --- |
| `LOG_AGENT_CONFIG_DIR` | Override the data directory. |
| `LOG_AGENT_SETTINGS_FILE` | Choose the runtime settings JSON file. |
| `LOG_AGENT_ADMIN_TOKEN` | Preconfigure the administrator token. |
| `LOG_AGENT_ENVIRONMENT` | Set the displayed runtime environment. |
| `LOG_AGENT_DEBUG` | Set to `1` to enable the WebView2 developer tools and context menu. |
| `LOG_AGENT_MAX_STORED_LOGS` | Set the global in-memory cache limit (1–1,000,000). |
| `LOG_AGENT_NO_SHORTCUT` | Set to `1` to skip creating the Windows desktop shortcut. |

Example:

```powershell
$env:LOG_AGENT_CONFIG_DIR = 'D:\log-agent-config'
.\栈桥.exe
```

`models.json` contains API keys in plain text, `commands.json` contains SSH passwords for servers that use password login, and `databases.json` contains database connection credentials. Keep these files outside public repositories and backups that are shared with others.

## Database workspace

Create a connection from the Database menu or tab. SQLite accepts an absolute file path and creates the file if it does not exist. Expand connections to browse databases, schemas, tables and views. Tables with primary keys support double-click editing, inserting rows and deleting selected rows; Save changes commits the batch in a transaction and rejects conflicting updates. Views and existing rows in tables without primary keys are read-only.

For multi-database execution, choose databases from the target dropdown above the SQL editor, enter one SQL statement and click Run SQL (Ctrl+Enter). The same SQL executes separately on each selected database, including SELECT, INSERT, UPDATE, DELETE and DDL. Result sets are combined by column name, missing columns become NULL, and every row includes its connection, database and engine. Direct single-table SELECT results that include every primary-key column support double-click cell editing. Enter or leaving the cell saves to that row's source database; the cell menu can set NULL. Joins, aggregates, computed expressions, generated columns and incomplete keys remain read-only. Updates check the original values and restore the displayed value on failure. Statements without result columns show a per-database execution report with status, affected rows, duration and errors; reports can also be exported as CSV. Affected counts that a driver does not provide are marked as not reported. SQL must be supported by each target; there are no joins between connections or automatic dialect translations. Up to 16 targets run with concurrency 4; each source is bounded by a 60-second timeout, an 8 MiB result limit and the selected row limit (up to 2,500). Selecting multiple statements in one action is rejected.

The left tree browses connections and table structures without selection checkboxes. SQL targets are chosen in a separate tree dropdown grouped by connection and database. Select a connection to check all its databases, or check databases individually; partly selected connections show a mixed checkbox state. Search preserves selections hidden by the filter. The limit is 16 databases, and a group that exceeds it is rejected without changing the selection. Each SQL tab retains its targets. Clearing the dropdown leaves zero targets even when browsing a table.

SQL editor execution permits changes on one or several databases. Each destination executes independently using its database's autocommit behavior: failure on one database does not roll back successful changes elsewhere. Check the individual reports before retrying a partially successful write. Cancellation does not undo completed changes, and in-flight outcomes may need verification. Explicit transaction-control statements (BEGIN/COMMIT/ROLLBACK/savepoints) are rejected because SQL executions do not hold a persistent connection between actions. The legacy API query mode remains database-enforced read-only. Structure changes offer a SQL preview before applying; PostgreSQL and SQLite plans use transactions, while MySQL DDL can auto-commit. Primary key changes and advanced constraints can be written in SQL.

The workspace has SQL tabs above the editor and table/result tabs below it. Use + or New SQL tab to create an independent editor; SQL text, cursor/undo state, query targets and results are retained when switching tabs. Queries can run in the background and their results stay with the originating SQL tab. Opening a table reuses its tab; each table retains its data/structure view, paging, selection and pending row edits. Closing a table with pending edits asks before discarding them. Tabs are kept for the current application session.

On a blank editor line, a template inserts a new statement; on an occupied line, it replaces only the current statement. Existing statements keep their own table and column pickers. The template button opens a floating panel above the workspace, without reducing the SQL editor width or limiting the panel to the editor height. Click a select/filter/sort/count/distinct/group/insert/update/delete template to fill the current SQL tab. Underlined table names and column expressions can be clicked directly inside the text to choose a schema source, table or columns. Filter operators are also selectable, including NULL checks, comparisons, LIKE, IN and BETWEEN; edit their arguments directly in the SQL text. Long SQL lines wrap automatically. Multi-column choices and repeated GROUP BY fields stay aligned. All generated content remains editable SQL. Filling a template does not execute it or change checked query targets.

The editor recognizes queries, writes and DDL separated by semicolons (the last statement can omit one). Single-database and multi-database execution icons appear on the right of each statement's final line. The single-database icon and Execute statement (one DB) use the Single-database target selector and preserve checked targets; the multi-database icon uses checked databases and requires at least two. Typing never executes SQL. Ctrl+Enter and Run SQL execute selected text, or the statement at the cursor when nothing is selected, on checked databases. Statement recognition follows the chosen single-database target's dialect and respects quoted semicolons, comments and nested queries.

Data edits use one transaction for each Save changes batch on the current table/database: successful changes commit together and a failed statement or conflicting update rolls the batch back. MySQL tables must use a transactional engine such as InnoDB for this guarantee. There is no manually held editing transaction or distributed transaction spanning connections.

The editor and grids use CodeMirror 6 and Tabulator; schema inspection and migration planning use Atlas, with go-sql-driver/mysql, pgx and modernc SQLite drivers. All browser assets are embedded and work offline. To rebuild the pinned browser dependencies (Node.js and pnpm required):

```bash
pnpm --dir scripts/database-ui install --frozen-lockfile --ignore-scripts
pnpm --dir scripts/database-ui test
node scripts/database-ui/build.mjs
```

## Development and tests

Run the complete test suite:

```bash
go test ./...
```

The end-to-end tests start an isolated temporary service and use a local Chrome or Edge session. They do not use real Dozzle nodes.

Build the Windows executable with tests:

```powershell
.\scripts\build.ps1
```

Build quickly without tests:

```powershell
.\scripts\build.ps1 -SkipTests
```

The approved logo source is `assets/branding/zhanqiao-logo-approved.png`. Run `python scripts/export-brand.py` with Pillow installed to export the header logo, favicon and Windows icon. The executable's icon and version info come from `app-icon.ico`, compiled into the committed `rsrc_windows_*.syso` files that `go build` links automatically. After changing the icon, regenerate them:

```powershell
go install github.com/tc-hib/go-winres@latest
go-winres simply --icon app-icon.ico --arch amd64,386,arm64 --manifest none --product-name "栈桥" --file-description "栈桥" --product-version 1.0.0 --file-version 1.0.0 --original-filename 栈桥.exe
```

Build output is written to `dist/`; local binaries and caches are intentionally ignored by Git.

## Architecture

- `main.go` — HTTP API, Dozzle connectors, SSE ingestion, history paging, cache policy, and embedded frontend.
- `app.js` — dashboard state, node/container selection, log stream, filtering, paging, and export.
- `assistant-ui.js` — AI assistant conversations, context, attachments, and session history.
- `term_api.go` / `terminal.js` — the Servers page: SSH shell sessions and SFTP over one connection, and the xterm.js front end. xterm.js is vendored in `third_party/xterm/` (MIT).
- `database.go` / `database_schema.go` / `database.js` / `database.css` — database connections, multi-target query merging, transactional row changes and the schema designer. Browser libraries and their licenses are vendored in `third_party/database/`.
- `bootstrap.js` — application initialization after frontend modules load.
- `styles.css` — dashboard styling and responsive layout.

The backend connects to real Dozzle v10 APIs for container metadata, historical logs, and SSE streams. A Dozzle root URL or a specific `/container/<container-id>` URL can be used when adding a node.

## Security notes

- Dozzle endpoints and AI provider URLs are user-configured; use HTTPS and network access controls in production.
- Do not commit `data/models.json`, `data/commands.json`, exported configuration files, private keys, or administrator tokens.
- The service does not modify Nginx or host system configuration.

## License

No license file is currently included. Contact the repository owner before redistributing the project.

<a id="中文"></a>

<details>
<summary>中文说明（点击展开）</summary>

## 项目简介

栈桥是一个面向 Dozzle 的自托管日志聚合、检索、加工和分析面板。服务端使用 Go 编写，前端 HTML/CSS/JavaScript 会嵌入最终可执行文件。

主要能力：

- 聚合多个 Dozzle 节点，支持容器选择、级别筛选、完整时间范围搜索、CSV 导出和暂停/继续接收。
- 按时间窗口拉取历史日志；单容器浏览缓存达到上限后，滚动到顶部会自动加载更早日志。
- 提供敏感信息脱敏、结构化字段提取和健康检查过滤等日志加工规则。
- 支持 AI 日志分析、上下文选择、图片/文本附件、模型配置和浏览器本地会话历史。
- 「指令集组」页编排可复用的服务器指令集；「服务器」页是类似 FinalShell 的 SSH 终端（xterm.js），下方带 SFTP 文件管理（浏览、上传、下载、新建、重命名、删除）和指令集快捷发送，支持选择或拖入整个文件夹上传并保留目录结构。
- 「数据库」页支持 MySQL、PostgreSQL、SQLite，提供连接和结构树、数据查看与编辑、可视化建表及字段和索引管理；支持一条 SQL 在多个库执行并合并结果。
- 本机访问时使用可执行文件旁的本地 JSON 配置；远程访问时使用访客浏览器的 localStorage。
- 无需数据库，前端资源直接嵌入 Go 可执行文件。

## 快速启动

开发环境需要 Go 1.26.4 或更高版本：

```bash
go run .
```

这是 Windows 桌面应用：程序打开独立窗口（基于 Microsoft Edge WebView2，Windows 11 自带），不监听任何网络端口，也不再通过浏览器访问。请先构建可执行文件：

```powershell
.\scripts\build.ps1
.\dist\栈桥.exe
```

窗口内的前端资源由程序自身提供，接口在进程内直接调用，不占用端口，本机其他程序和局域网都无法访问。同一时间只运行一个实例，再次启动会把已有窗口切到前台；外部网站链接用系统浏览器打开；运行日志写入数据目录下的 `log-agent.log`，设置 `LOG_AGENT_DEBUG=1` 可开启 WebView2 开发者工具。首次启动会在桌面创建名为「栈桥」的快捷方式。移动 exe 后再启动一次会更新快捷方式；手动删除后不会再自动创建。设置 `LOG_AGENT_NO_SHORTCUT=1` 可关闭此功能。Windows amd64 最新可执行文件位于 [Releases](https://github.com/sakura-setsumi/log-agent/releases)。

## 配置和安全

节点和 AI 供应商默认保存在可执行文件旁的 `data/nodes.json` 与 `data/models.json`，指令集组页的指令集、指令集组、服务器库和收藏保存在 `data/commands.json`，每次运行的指令与服务器返回日志保存在 `data/command-logs.json`（保留最近 200 次运行）（本机访问时只读写该文件，不再使用浏览器 localStorage；其他机器访问时仍保存在各自浏览器中。旧版本留在浏览器里的指令集会在首次启动时自动迁移进该文件）。可使用 `LOG_AGENT_CONFIG_DIR`、`LOG_AGENT_SETTINGS_FILE`、`LOG_AGENT_ADMIN_TOKEN`、`LOG_AGENT_ENVIRONMENT`、`LOG_AGENT_DEBUG`、`LOG_AGENT_MAX_STORED_LOGS` 和 `LOG_AGENT_NO_SHORTCUT` 覆盖默认设置。

数据库连接保存在 `data/databases.json`。`models.json` 会以明文保存 API key，`commands.json` 会以明文保存使用密码登录的服务器密码，`databases.json` 会保存数据库连接凭据，请勿提交到公开仓库，也不要共享导出的敏感配置、私钥或管理员 token。服务不会修改 Nginx 或主机系统配置。

## 数据库与多库查询

在「数据库」菜单或页签中新增连接，展开左侧结构树可浏览库、Schema、表和视图。SQLite 使用绝对文件路径，文件不存在时会创建。有主键的表可双击单元格编辑、新增行、选择行删除，点击「保存修改」后以事务提交；遇到并发修改会回滚。视图和无主键表的已有记录只读。

在 SQL 区上方的目标数据库下拉框中多选数据库，输入一条语句后点击「执行 SQL」或按 Ctrl+Enter。同一条 SQL 分别在各库执行，支持查询、INSERT、UPDATE、DELETE 和 DDL。有返回列的结果按列名对齐合并，缺失列填 NULL，并保留连接、数据库和引擎来源；无返回列时显示逐库执行报告，包括状态、影响行数、耗时和错误，也可导出 CSV。驱动没有提供影响行数时明确显示「未报告」。各库需支持相同语法及表结构；不进行连接之间的 JOIN 或 SQL 方言转换。最多选择 16 个库、并发 4 个，每库超时 60 秒、结果上限 8 MiB，行数上限可选至 2,500；单次选择多条 SQL 执行会被拒绝。

左侧结构树用于浏览连接和表结构，不显示复选框。SQL 目标在独立的树形多选下拉框中选择，按「连接 → 数据库」分组；可勾选整个连接或逐库选择，部分选中时连接显示半选状态。搜索不会丢失被筛选隐藏的选择。最多选择 16 个库，连接整组选中若超过限制则保持原选择。每个 SQL 页签各自保存目标库；清空下拉框后保持 0 个目标，不会自动回退到左侧正在浏览的表。

SQL 编辑器允许单库、多库修改，各目标按数据库自身的自动提交行为独立执行；某库失败不会回滚其他库已经成功的修改。部分成功后应逐库检查再重试；取消也不会撤销已完成的修改，仍在执行的库需核实最终结果。由于各次操作之间没有持续占用的连接，不支持通过 BEGIN/COMMIT/ROLLBACK 或保存点手动控制跨次事务，会明确报错。旧 API 的 query 模式仍强制只读。结构页支持可视化建表、添加/修改/重命名/删除字段、新增/删除索引、重命名/删除表，先预览 SQL 再应用。PostgreSQL 和 SQLite 使用事务，MySQL DDL 可能自动提交。主键变更和高级约束可在 SQL 编辑器中操作。

工作区上方是 SQL 页签，下方是表与查询结果页签。点击「＋」或「新建 SQL 页签」可创建独立编辑区；切换时保留各自的 SQL 文本、光标与撤销状态、目标库和结果。查询可以在后台运行，结果只写回发起查询的页签。打开同一个表会复用其页签；每张表独立保留数据/结构视图、分页、选中行和未保存修改。关闭有未保存修改的表时会先确认。页签在本次应用会话中保留。

模板按钮打开覆盖工作区的独立浮层，不挤占 SQL 文本区宽度，也不受文本区高度限制。点击查询、条件、排序、统计、去重、分组或增删改模板后，内容填入当前 SQL 页签。在文本中点击带下划线的表名、字段表达式即可选择结构来源、表和字段；条件运算符也支持选择，包括 NULL 判断、比较、LIKE、IN、BETWEEN 等，对应的值可直接在 SQL 文本中编辑。长 SQL 自动换行显示。多列选择和 GROUP BY 中重复的字段会同步更新。生成的内容仍是可直接编辑的 SQL，填入模板不会自动执行，也不会改变已勾选的查询目标。

编辑器会识别以分号分隔的查询、增删改和 DDL 语句（最后一条可不写分号），并在每段语句末行右侧显示单库、多库执行图标。单库图标和「执行语句（单库）」使用「单库目标」下拉框所选的库，并保留多库勾选；多库图标使用目标数据库下拉框中勾选的库，至少需要勾选两个。输入文字不会自动执行。Ctrl+Enter 和「执行 SQL」优先执行选中的文本，否则执行光标所在语句，目标为下拉框中勾选的库。识别采用「单库目标」对应的 SQL 方言，处理字符串中的分号、注释和嵌套语句。

数据编辑以每次「保存修改」为一个事务，范围为当前库、当前表的本批操作；成功时统一提交，任一操作失败或检测到并发修改时整批回滚。MySQL 表需使用 InnoDB 等事务型引擎。当前没有手动保持编辑事务或跨连接的分布式事务功能。

采用 CodeMirror 6、Tabulator、Atlas 以及成熟的 MySQL、pgx、modernc SQLite 驱动，浏览器依赖随程序离线打包。依赖版本和构建脚本位于 `scripts/database-ui/`，许可证位于 `third_party/database/LICENSES.txt`。

## 开发测试

```bash
go test ./...
```

使用 `scripts/build.ps1` 构建 Windows 程序；使用 `scripts/build.ps1 -SkipTests` 可跳过测试。构建输出在 `dist/`，本地生成物会被 Git 忽略。

## 代码结构

- `main.go`：HTTP API、Dozzle 连接、SSE 日志接收、历史分页、缓存策略和嵌入式前端。
- `app.js`：页面状态、节点/容器选择、日志流、筛选、分页和导出。
- `assistant-ui.js`：AI 助手会话、上下文、附件和历史记录。
- `term_api.go`、`terminal.js`：「服务器」页——同一条连接上的 SSH shell 与 SFTP，以及基于 xterm.js 的前端；xterm.js 随仓库放在 `third_party/xterm/`（MIT）。
- `bootstrap.js`：前端模块加载完成后的初始化。
- `styles.css`：面板样式和响应式布局。

</details>
