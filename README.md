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

缺少配置文件时，`sa2` 默认通过生产站点 `https://sa2.mentorx.org` 的反向代理访问 SA2 API，并在当前用户的配置目录下创建 `sa2-cli/config.json`。之后每次启动都会读取该文件，可用编辑器修改并保存 API 地址。

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

优先级从高到低为 `--api-url`、`SA2_API_URL` 环境变量、配置文件。HTTP 只允许 `localhost` 或环回 IP，其他地址使用 HTTPS。地址可含端口，省略账号凭据、路径、查询和片段。

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

在桌面终端运行时，CLI 会尝试打开浏览器里的独立飞书扫码页。也可以在运行 CLI 的电脑上手动打开终端显示的链接。用飞书 App 扫码后，网页会回到本机 CLI，终端会显示结果。链接五分钟内有效，按 Ctrl+C 可取消。飞书登录沿用网页的员工账号检查。

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
