# SA2 API 契约

`operations.json` 声明 CLI 消费的操作。`source.json` 记录来源仓库、完整提交 SHA、文档路径和 Git blob SHA。`openapi.cli.json` 是从该来源提取的操作及递归引用组件。

从 `sa2-cli` 运行本地检查。

```sh
go run ./tools/contract -check
```

同步时先更新 `source.json` 的提交和 blob，然后导出该提交中的契约。以下命令使用仓库内的 API checkout 和临时文件。

```sh
mkdir -p .cache
api_commit=$(jq -r .commit contracts/source.json)
api_document=$(jq -r .path contracts/source.json)
git -C ../sa2-api show "$api_commit:$api_document" > .cache/api-openapi.json
go run ./tools/contract -source .cache/api-openapi.json
go run ./tools/contract -check -source .cache/api-openapi.json
```

PR 检查子集形态并运行测试。主分支 CI 默认校验仓库内的快照，日志会提示此次检查只覆盖快照。手动发布前核对目标提交的 CI 结果。更新上游版本时审查生成的差异并复验请求、响应和错误映射。

要额外核对固定 API 提交的 Git blob 和契约子集，可在 `sa2-cli` 的 GitHub Actions 仓库 Secret 中配置 `SA2_API_READ_TOKEN`。凭据只需对私有 `victorx98/sa2-api` 有 `Contents: Read` 权限。配置后，读取上游或核对契约失败会使 CI 失败。
