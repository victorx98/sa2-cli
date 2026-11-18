# sa2-cli

`sa2-cli` 为 Agent 提供访问 SuperAcademy 的命令行入口。Agent 通过 `sa2` 调用已开放的能力，`sa2-api` 负责身份验证、数据访问授权和业务规则。

项目处于设计阶段，当前仓库只有目录骨架。以下技术栈和包职责是后续实现的约定。

## 技术栈

- Node.js 22.12+、TypeScript 和 ESM，使用 `tsc` 构建。
- npm workspaces 管理仓库内的包，Commander 处理命令行参数和帮助。
- Node 原生 `fetch` 发送 HTTP 请求，`openapi-typescript` 生成 API 类型。
- `node:test` 配合本地 mock HTTP 服务，验证 CLI 的输出和退出码。

## 目录结构

```text
sa2-cli/
├── packages/
│   ├── core/       # 能力定义、输入校验、执行逻辑和 API 客户端
│   ├── cli/        # 命令注册、帮助、终端输出和退出码
│   └── sdk/        # 面向 TypeScript 调用方的公开接口
├── contracts/      # 从 sa2-api 同步的 OpenAPI 契约快照
├── skills/         # Agent 使用说明及参考资料
└── tests/          # 跨包集成测试和 CLI 进程测试
```

`cli` 和 `sdk` 都依赖 `core`。CLI 将命令行参数转换成结构化输入，SDK 接收 TypeScript 对象，两者共享能力校验和执行逻辑。CLI 的命令结果以 JSON 写入 `stdout`，诊断信息写入 `stderr`，执行失败返回非零退出码。

`sa2-cli`、`sa2-api` 和 `sa2-ui` 各自维护在独立 Git 仓库中。CLI 使用带有来源提交记录的 OpenAPI 快照，生成的类型放在 `packages/core/src/api/generated/`。同步后，单独检出 `sa2-cli` 即可构建。
