
# PUBLIC-3 提交与权限模型实施计划

> 在当前工作树内执行；保留既有 PUBLIC-1/2 改动，不合并或开放公网监听。2026-11-27 用户已明确要求提交、推送并提 PR；该授权覆盖本计划先前“不提交、不推送”的阶段性限制。

## 目标

实现 PUBLIC-3 设计中的公开预览/确认、一次性控制令牌、CAPTCHA fail-closed、执行身份校验，以及取消/删除清理失败保护。

## 任务

### 1. 红测：权限与提交边界

- 在 `internal/control/public_handler_test.go` 增加公开 CAPTCHA、预览、确认、token-only cancel/delete、Origin 和重复确认测试。
- 在 `internal/control/sqlite_outbox_test.go` 或新测试增加 token hash 只存哈希、错误 token 拒绝、任务身份持久化测试。
- 在 `internal/control/worker_test.go` 增加身份不匹配不执行、清理错误不转为 cancelled/删除成功测试。
- 先运行针对性测试，记录红测证据。

### 2. 实现公开接口与 token store

- 扩展 `API` 的 CAPTCHA verifier、公开 Origin 配置和 `PublicHandler` 路由。
- 复用现有 preview/confirm 逻辑，公开入口不经过 loopback/LAN pairing；公开 preview 在 GitHub 读取前验证 CAPTCHA。
- SQLite/Memory 保存 token hash；公开确认在同一事务中保存任务与 hash；响应只在首次创建返回原 token。
- 用 `Authorization: Bearer` 和常量时间哈希比较保护取消/删除。

### 3. 实现 AUD-001

- 扩展 `domain.Task` 持久化执行身份。
- 创建/确认时把 repo、commit、Go、runner、config digest 写入任务。
- worker 在 `StartTask` 后、executor 前校验当前身份；不匹配时保存 terminal error 并 ack outbox，不调用 executor。

### 4. 实现 AUD-003

- runner 用可识别的 `CleanupError` 包装容器/网络清理失败。
- process registry 传播执行返回错误给 `CancelAndWait`。
- worker 对 cleanup failure 保存 `cleanup_pending` 错误状态；控制层把取消/删除返回冲突，不删除任务。
- 保持正常取消和正常删除行为不变。

### 5. 验证与报告

- `gofmt`、针对性 Go 测试、`go test -p=1 -count=1 ./...`、`go vet ./...`、`go build ./...`、`git diff --check`。
- 核对 PUBLIC-3 每个 AC、smoke、未做范围和公网 listener 状态。
- 更新 `docs/publicization-stage-report.md` 与 `docs/optimization-design.md`，保留失败轮次、证据和未关闭风险。
- 全量本地验证通过后检查远端 PR；按远端状态更新现有 PR 或创建新 PR，并在 PR 描述中列出验收命令与未做范围。

## 回退与停止条件

任何公开写入口出现绕过 CAPTCHA、token 明文入库/URL、身份不匹配仍执行、清理失败返回成功，立即停止继续扩展，保留红测和失败证据，先修复根因。PUBLIC-4/5/6 不在本计划内。
