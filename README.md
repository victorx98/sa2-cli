# sa2-cli

`sa2` 为 Agent 提供 SuperAcademy 的命令行入口。认证、授权和令牌轮换由 `sa2-api` 处理。

## 安装与运行

从发行页下载对应 macOS、Windows 或 Linux 的归档，核对 `checksums.txt` 后解压。也可以用 Go 1.27.4 从源码构建。在 macOS 上使用原生构建环境和 `CGO_ENABLED=1`，链接系统 Security framework。

发行归档支持 Linux、macOS 和 Windows 的 amd64（x86-64）版本。

```sh
go build -trimpath -o sa2 ./cmd/sa2
./sa2 --help
```

Windows 源码构建使用 `go build -trimpath -o sa2.exe ./cmd/sa2`。

## API 地址配置

普通命令默认通过生产站点 `https://sa2.mentorx.org` 的反向代理访问 `/api/...`。首次运行普通命令时，CLI 会在当前用户的配置目录下创建 `sa2-cli/config.json`，之后可编辑该文件修改默认 API 地址。

| 系统 | 用户配置目录 |
| --- | --- |
| Linux | 设置了 `XDG_CONFIG_HOME` 时使用该目录，否则使用用户主目录下的 `.config` |
| macOS | 用户主目录下的 `Library/Application Support` |
| Windows | `%APPDATA%` |

```json
{
  "SA2_API_URL": "https://sa2.mentorx.org"
}
```

优先级从高到低为 `--api-url`、`SA2_API_URL` 环境变量、配置文件。指定 `--api-url` 时跳过配置文件。查看帮助或版本也无需读取配置。HTTP 只允许 `localhost` 或环回 IP，其他地址使用 HTTPS。地址可含端口，省略账号凭据、路径、查询和片段。

本地开发时将全局选项 `--dev` 放在子命令前，例如 `sa2 --dev whoami`。它直连 `http://localhost:8094` 的 sa2-api，跳过 `SA2_API_URL` 和用户配置文件。`--dev` 与 `--api-url` 互斥。登录和后续命令使用相同的地址，凭据按地址隔离。

## 命令示例

### 邮箱登录

```sh
sa2 login --email 'you@example.com'
```

密码在终端提示时输入，输入内容会隐藏。自动化可从宿主秘密管理器提供的文件读取密码，`SA2_PASSWORD_FILE` 填写该文件的路径。

```sh
sa2 login --email 'you@example.com' --password-stdin < "$SA2_PASSWORD_FILE"
```

### 飞书授权及扫码登录

```sh
sa2 login --feishu
```

飞书登录需要 HTTPS。CLI 打开同一域名上由 sa2-ui 提供的 `/auth/cli` 扫码页，收到授权码后向 sa2-api 的 `/api/auth/feishu/login` 换取会话。也可以在运行 CLI 的电脑上手动打开终端显示的链接。链接五分钟内有效，按 Ctrl+C 可取消。飞书登录沿用网页的员工账号检查。本地 HTTP 开发请使用 `sa2 --dev login --email EMAIL`。

### 查看当前身份

```sh
sa2 whoami
```

每次执行都会尝试续期，成功后更新刷新令牌并返回用户 ID、邮箱和角色。续期结果不明时需要重新登录。

```json
{"data":{"id":"8b93fc3c-c385-43fd-bf90-bfe24a6c7faa","email":"you@example.com","roles":["student"]}}
```

### 清除本地会话

```sh
sa2 logout
```

清除当前 API 地址对应的本地凭据。重复执行也会返回 `{"data":{"loggedOut":true}}`。

### 临时指定 API 地址

将示例域名替换成实际环境的 HTTPS 地址。同一环境的登录、身份查询和退出使用同一个地址。

```sh
sa2 login --feishu --api-url https://sa2-staging.example.com
sa2 whoami --api-url https://sa2-staging.example.com
sa2 logout --api-url https://sa2-staging.example.com
```

### 查看帮助和版本

```sh
sa2 --help
sa2 login --help
sa2 --version
```

业务命令的 stdout 是单行 JSON。成功读取 `data`，失败读取 `error.code` 和 `error.message`。退出码为 `0` 成功、`1` 运行失败、`2` 参数错误、`3` 认证被拒或需登录、`130` 取消。交互提示来自 stderr。

## 系统凭据库

| 系统 | 运行条件 |
| --- | --- |
| macOS | 当前用户的默认 Keychain 已解锁，且允许当前二进制访问其凭据项。 |
| Windows | 当前运行身份可访问 Credential Manager。通用凭据以本机范围持久化。 |
| Linux | 当前用户可访问已解锁的、持久加密的默认 Secret Service 集合。CLI 通过当前用户的 Unix session bus 连接。 |

刷新令牌只保存在系统凭据库。密码和访问令牌只由 CLI 在进程内存中处理。应用状态目录只保存空锁文件。凭据库未配置、锁定或拒绝访问时，命令返回 `CREDENTIAL_STORE_UNAVAILABLE`。Linux SSH、容器和 CI 宿主需要预配独立 OS 账户、该账户的 Unix session bus 及持久加密的默认集合。通过 `DBUS_SESSION_BUS_ADDRESS` 指定非标准的 `unix:path=` 地址。

Go 版使用独立凭据命名空间。切换前在旧版对使用过的各 origin 执行 `logout`，再在 Go 版重新登录。回滚时用对应版本重新登录。

## Agent Skill

将 [agent-cli-usage](./skills/agent-cli-usage/SKILL.md) 所在目录按 Agent 宿主支持的方式注册，再为宿主提供已安装的 `sa2` 和对应的 OS 凭据库。Skill 说明输入模式、结果解析和失败恢复。

### 在本地 Codex 中使用

Codex 可以通过终端直接运行 `sa2`。先在 Codex 中输入“运行 `sa2 --version` 并告诉我版本号”，确认它能从 PATH 找到已安装的 CLI。

要让 Codex 根据自然语言任务自动选择这个 Skill，在 macOS 或 Linux 上从本仓库根目录执行一次：

```sh
mkdir -p "$HOME/.agents/skills"
ln -s "$PWD/skills/agent-cli-usage" "$HOME/.agents/skills/agent-cli-usage"
ln -s "$PWD/skills/sa2-book-session" "$HOME/.agents/skills/sa2-book-session"
ln -s "$PWD/skills/sa2-manage-session" "$HOME/.agents/skills/sa2-manage-session"
ln -s "$PWD/skills/sa2-manage-monthly-report" "$HOME/.agents/skills/sa2-manage-monthly-report"
```

在 Codex 中输入 `/skills` 查看列表，也可以显式调用：

> $agent-cli-usage 请运行 `sa2 --help`，告诉我支持哪些登录方式。

登录后，可以直接说“查看我在本地 SA2 环境 `http://localhost:8094` 的身份和角色”。对应命令为 `sa2 --dev whoami`。登录时使用同一个 API 地址，并确保 Codex 能访问登录用户的系统凭据库。`whoami` 会轮换令牌，验证安装时使用 `--version` 或 `--help` 即可。

## 开发

从本仓库执行以下检查。集成测试启动本地 HTTPS 模拟 API。

```sh
go test ./...
go vet ./...
go run ./tools/contract -check
```

Linux 原生凭据测试使用隔离的 D-Bus 和 GNOME Keyring。安装这两个测试依赖后，运行 `bash scripts/test-linux-keyring.sh`。CI 在 macOS 和 Windows 上也运行各自的原生凭据测试。

主分支 CI 默认校验仓库内的 OpenAPI 快照。可配置 `SA2_API_READ_TOKEN` 核对私有 API 仓库的固定提交。 [OpenAPI 契约与凭据配置](./contracts/README.md)

## 手动发布

发布前从仓库根目录运行 `go run ./tools/contract -runtime https://sa2.mentorx.org`（本地改用 `http://localhost:8094`）。检查只读取公开的 `/api/docs-json`，逐项比较 CLI 使用的操作、参数与关联数据结构，同时报告全部缺失和差异。源码快照仍用 `go run ./tools/contract -check` 验证。

1. 确认目标提交的 CI 已通过，在预发布验证目标宿主的认证链路和峰值续期负载，再由维护者创建发行 tag。
2. 在 Linux、macOS 和 Windows 的 amd64 环境中检出同一 tag。从仓库根目录用 Bash 执行以下命令，Windows 可使用 Git Bash。

   ```bash
   set -e
   sa2_version=$(git describe --tags --exact-match)
   sa2_os=$(go env GOOS)
   test "$(go env GOARCH)" = amd64
   sa2_binary=sa2
   sa2_cgo=0
   case "$sa2_os" in
     darwin) sa2_cgo=1 ;;
     windows) sa2_binary=sa2.exe ;;
     linux) ;;
     *) exit 1 ;;
   esac
   mkdir -p dist
   CGO_ENABLED="$sa2_cgo" go build -trimpath -ldflags "-X main.version=$sa2_version" -o "dist/$sa2_binary" ./cmd/sa2
   test "$(./dist/"$sa2_binary" --version | tr -d '\r')" = "$sa2_version"
   ./dist/"$sa2_binary" --help
   tar -czf "dist/sa2_${sa2_version}_${sa2_os}-amd64.tar.gz" -C dist "$sa2_binary"
   ```

3. 将三个归档汇总到同一 tag 的 `dist/`，从仓库根目录生成 Skill 归档和校验和。

   ```bash
   set -e
   sa2_version=$(git describe --tags --exact-match)
   git archive --format=zip --output=dist/agent-cli-usage.zip HEAD:skills
   cd dist
   sha256sum "sa2_${sa2_version}_"{linux,darwin,windows}-amd64.tar.gz agent-cli-usage.zip > checksums.txt
   ```

   macOS 可将 `sha256sum` 替换为 `shasum -a 256`。在 GitHub Releases 手动选择该 tag，上传三个二进制归档、`agent-cli-usage.zip` 和 `checksums.txt`，核对附件后发布。Actions 只运行 CI 检查。

实施与验收条件见 [Go 安全重构方案](./docs/plans/active/2026-11-19-go-migration.md)。


## 一对一约课

使用 `sa2 sessions --help` 查看参数。登录、预览和确认使用同一个 API 地址。生产环境默认访问 `https://sa2.mentorx.org`，由 nginx 将 `/api` 转发到 sa2-api。

本地 API 按监听端口确定预约地址，开发环境的 `.env` 无需配置 `SESSION_BOOKING_ORIGIN`。工作区使用 8094 端口，登录和预约时都加 `--dev`。如果 API 使用其他端口，例如 8080，则两步都使用 `sa2 --api-url http://localhost:8080`。以下示例中的大写 ID 为占位符。

```sh
sa2 sessions find --kind student --text '学生姓名'
sa2 sessions find --kind mentor --text '导师姓名'
sa2 sessions options --student-id S --mentor-id M --contract-id C
sa2 sessions slots --student-id S --mentor-id M --from 2026-12-01 --to 2026-12-08 --timezone America/New_York --duration 90
sa2 sessions plan create --student-id S --mentor-id M --contract-id C --type-id T --entitlement External --start 2026-12-02T09:00:00-05:00 --timezone America/New_York --duration 90 --title '简历辅导' --out booking.plan
sa2 sessions create --plan booking.plan --confirm
sa2 sessions wait --request-id K --timeout 60s
```

每次预览须为 `--out` 指定新文件名。预览被阻止时命令以非零状态退出，并报告阻止原因。成功的预览返回权益来源、日历及会议影响。确认前核对时区、备注可见性和权益。改期和取消同样需要生成计划。计划文件仅当前系统用户可读，绑定账号、origin、请求标识及有效期。业务提交超时后用原 requestId 执行 `sa2 sessions operation`，外部结果需要人工核对时保留原标识。

Agent 指引见 [新预约](skills/sa2-book-session/SKILL.md) 和 [查看、改期与取消](skills/sa2-manage-session/SKILL.md)。

## 月报查询与里程碑提取

需要具有学生查询、月报读取权限的登录账号。以下 ID 为占位符。

```sh
sa2 sessions find --kind student --text '学生姓名'
sa2 reports monthly get --student-id S --period 2026-11
sa2 reports monthly latest --student-id S --section milestones
sa2 reports monthly latest --student-id S --through 2026-11 --lookback 24 --section milestones
```

`latest` 从当前本地月份（或 `--through`）向前查找月份最近的已保存月报，默认含截止月回查 24 个月，可设为 1–120 个月；不按编辑时间排序，也不查询截止月份之后的月报。返回 `data.report` 和 `data.search`（from、through、checkedMonths）。`get` 返回 `data`。`--section milestones` 仅输出标识、月份、更新时间和里程碑数组。

未生成月份的默认模板不作为已有月报返回；指定月份或搜索范围内没有记录时返回 `REPORT_NOT_FOUND`。认证、权限或连接失败会立即停止，不作为无月报处理。读取命令不生成或刷新月报。单项更新见下文。

Agent 指引见 [月报查询](skills/sa2-manage-monthly-report/SKILL.md)。例如：“找到 francetest 学生本月及之前最近的已有月报，提取里程碑内容。”匹配多名学生时先确定目标。

### 精准更新单个里程碑

此功能需要目标环境部署单项 PATCH 接口。用 `schema` 查看随 CLI 打包的 OpenAPI 请求结构，无需登录：

```sh
sa2 reports monthly milestone schema
sa2 reports monthly get --student-id S --period 2026-06 --section milestones
sa2 reports monthly milestone update --student-id S --period 2026-06 --report-id R --key first_counselor_strategy_meeting --expected-updated-at 2026-08-05T23:18:49.843Z --notes-file notes.html
```

ID、key、时间为占位示例，必须使用 GET 返回的实际值。最后一条命令默认只读预览；在用户已授权具体变更后，原命令增加 `--confirm` 提交一次并回读核验。省略 `--status` 保留状态；省略备注选项保留备注；`--clear-notes` 明确清空，与 `--notes-file` 互斥。至少指定状态或备注。备注文件为 UTF-8 富文本 HTML（或不含标记的文本），上限 100000 个 UTF-16 单元；已有读取接口会修剪首尾空白。

状态只支持 `not_started`、`done`、`on_track`、`needs_attention`、`at_risk`、`critical`，不接受 `—`。“系统默认”等含糊指令需由用户明确。只按已有且唯一的 key 更新，不改名称、排序，不创建或删除里程碑，不替换整个数组。

版本或原数组在读取后有变化时返回 `STALE_REPORT`，需重新核对；不自动改版本重试。`MILESTONE_API_NOT_DEPLOYED` 表示需先部署 API，不回退旧 PUT。`UPDATE_OUTCOME_UNKNOWN` 表示可能已写入，先回读；`UPDATE_VERIFICATION_FAILED` 表示需要核对最终状态，不盲目重放。成功返回 `submitted=true`、`verified=true`、before/after 及回读版本。原网页整段 PUT 的并发行为未改变。
