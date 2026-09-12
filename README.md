# 日志中枢 · Log Agent

这是一个用 Go 标准库提供服务端、HTML/CSS/JavaScript 提供交互界面的 Dozzle 日志管理面板。

## 启动

需要 Go 1.22 或更高版本：

```bash
go run .
```

浏览器打开 <http://localhost:8099>。

## 当前能力

- 统一查看多个 Dozzle 节点，支持节点切换、日志级别筛选和关键词搜索
- 日志实时流采用 Server-Sent Events（SSE）推送
- 支持暂停 / 继续接收、清空过滤条件和 CSV 导出
- 支持新增 Dozzle 节点，配置通过 Go API 保存到当前进程内存
- 支持敏感信息脱敏、结构化字段提取、健康检查过滤等加工开关
- 前端资源通过 `embed` 打包进 Go 服务，单个项目即可运行

服务端会连接真实 Dozzle v10 实例：读取实例配置和容器事件，拉取容器历史日志，并通过 Dozzle SSE 接收实时日志。添加地址可以填写 Dozzle 根地址，也可以填写具体容器页面地址（例如 `/container/<container-id>`）。配置数据库后，节点信息会在服务重启后自动恢复。

## 节点数据库配置

节点信息可以持久化到 MySQL 的 `log_agent.vps_info` 表。启动服务前设置以下环境变量：

```bash
export DOZZLE_DB_HOST=数据库地址
export DOZZLE_DB_PORT=3306
export DOZZLE_DB_USER=数据库用户
export DOZZLE_DB_PASSWORD=数据库密码
export DOZZLE_DB_NAME=log_agent
```

Windows PowerShell 请使用 `$env:` 设置环境变量，且必须在同一个窗口中启动服务：

```powershell
$env:DOZZLE_DB_HOST = "数据库地址"
$env:DOZZLE_DB_PORT = "3306"
$env:DOZZLE_DB_USER = "数据库用户"
$env:DOZZLE_DB_PASSWORD = "数据库密码"
$env:DOZZLE_DB_NAME = "log_agent"
go run .
```

启动日志中应看到数据库连接成功；如果看到 `memory-only node storage`，说明环境变量没有传给当前服务进程。

服务启动时会读取 `vps_info` 表中的 `name`、`address`、`style` 字段；新增、修改和解绑节点时也会同步写入数据库。未配置数据库环境变量时，服务仍可启动，但节点配置只保存在内存中。

日志缓存默认最多保存 100,000 条。可通过 `LOG_AGENT_MAX_STORED_LOGS` 调整，取值范围为 1～1,000,000；缓存越大，占用的内存越多。
