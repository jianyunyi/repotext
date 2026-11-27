# RepoTest PUBLIC-3 提交与权限模型设计

<!-- 状态块 -->
模式：Full；沟通方式：连续执行
版本：1；更新：2026-11-27；当前阶段：PUBLIC-3；状态：进行中
未关闭阻断项：AUD-002（不属于本阶段）；延期告警/风险：CAPTCHA 供应商凭据、PUBLIC-4/5/6
待决策：无；下一步：完成全量复验、远端 PR 检查与推送

## 需求理解

PUBLIC-3 把现有“本机/局域网身份授权”替换为“创建者持有控制令牌”的公开提交模型。匿名访客可以提交公开 GitHub 仓库、先预览并确认固定 commit；创建成功后获得任务 ID 和一次性展示的控制令牌。任务 ID 可进入分享链接，控制令牌只由提交者浏览器保存，不进入 URL、报告或日志。

范围：公开预览、CAPTCHA 验证边界、公开确认、令牌控制的取消/删除、任务执行身份持久化与校验、清理失败的可恢复记录；保留 URL/ref/commit 校验、仓库大小限制、归档解包校验和现有报告投影。

不做：PUBLIC-4 限流/队列容量/并发与总时长收紧；PUBLIC-5 沙箱重评审；PUBLIC-6 TLS/封禁/告警/重启恢复上线门槛；公网监听；账号体系；GitHub 私有仓库凭据注入容器；CAPTCHA 具体供应商接入。现有本机/LAN 兼容入口暂不在本阶段拆除，PUBLIC-3 的公开入口不依赖它们。

约束：当前服务仍只绑定 loopback；实现必须可在现有 Go/SQLite 代码和测试中验证，不以开启公网作为验收前提。旧任务没有执行身份时，在配置了身份校验的 worker 上安全失败，不继续执行。

## 需求与验收基线（进入实施前冻结）

| 需求编号 | 用户需求 | 交付物 | 阶段 | 验收编号 | 验证操作与预期结果 | smoke |
|---|---|---|---|---|---|---|
| R-301 | 公开访客可完成预览、确认并得到任务凭据 | `PublicHandler` 公开 API、设计/测试 | PUBLIC-3 | AC-301 | 带有效 CAPTCHA 的公开预览可复用现有 URL/ref/commit、仓库大小和归档校验；公开确认返回不可预测任务 ID 和控制令牌 | 是 |
| R-302 | CAPTCHA 未配置时不得绕过人机验证 | CAPTCHA verifier 接口及错误契约 | PUBLIC-3 | AC-302 | verifier 缺失时公开提交在调用 GitHub 前返回 `503 captcha_unavailable`；空/无效令牌返回非成功且不创建预览/任务 | 是 |
| R-303 | 任务 ID 与控制令牌分离，令牌只展示一次并以哈希持久化 | SQLite/Memory token store、Bearer 校验 | PUBLIC-3 | AC-303 | 响应含一次性原始令牌；数据库/存储只见 SHA-256 哈希；重复确认不再返回令牌；错误 token 不能取消/删除；正确令牌可用且比较路径使用常量时间比较 | 是 |
| R-304 | 公开任务控制不依赖 loopback、LAN 配对或本机身份 | 公开取消/删除路由与 Origin 边界 | PUBLIC-3 | AC-304 | 非 loopback 请求带正确 Bearer 令牌可取消/删除；缺令牌、错误令牌、未允许 Origin 拒绝；令牌不出现在 URL | 是 |
| R-305 | 修复 AUD-001：任务持久化并校验执行身份 | `domain.Task` 执行身份、worker 校验 | PUBLIC-3 | AC-305 | 创建任务保存 repo、commit、Go、runner、config digest；worker 当前身份任一字段不匹配时不调用 executor，任务保留为 `error/execution_identity_mismatch` | 是 |
| R-306 | 修复 AUD-003：清理失败不得虚报取消/删除成功 | cleanup error、process registry 结果传播、状态保留 | PUBLIC-3 | AC-306 | 取消引起的资源清理失败会保留任务和错误码 `cleanup_pending`，取消不返回 204；删除不会继续删除元数据；无清理错误时现有取消/删除流程保持通过 | 是 |
| R-307 | 不回退已有输入与报告安全边界 | 既有验证与回归 | PUBLIC-3 | AC-307 | PUBLIC-1/2 相关 Go 测试、报告投影和 `git diff --check` 通过；不挂载公网 listener | 是 |

验收基线在本文件进入实施时冻结；后续变更必须追加决策记录，不通过降低标准或删除测试掩盖失败。

## 方案

### 公开接口

- `POST /api/v1/public/previews`：请求为 `repo_url`、可选 `ref`、`captcha_token`。先执行 CAPTCHA，再调用现有预览校验和 GitHub 元数据读取；返回现有 `Preview` 结构及一次性 `preview_id`。
- `POST /api/v1/public/tasks`：请求为 `preview_id`。确认固定 commit，返回 `task`、`reused`；仅首次创建返回 `control_token`。控制令牌使用 32 字节 CSPRNG，采用 URL-safe base64 展示。
- `POST /api/v1/public/tasks/{id}/cancel`、`DELETE /api/v1/public/tasks/{id}`：只接受 `Authorization: Bearer <control_token>`，令牌不接受 query/path/body。可选 `PublicOrigins` 配置对浏览器 `Origin` 做精确白名单检查；无 Origin 的非浏览器调用仍必须持有 Bearer 令牌。
- 公开报告继续使用 PUBLIC-2 的 `/r/{id}` 与 `/api/v1/public/reports/{id}`，不返回控制令牌、内部错误详情或完整日志。

### 数据与身份

- SQLite 新增 `task_tokens(task_id PRIMARY KEY, token_hash BLOB)`，只保存 SHA-256 哈希并由任务外键级联删除；确认任务与 token hash 在同一事务中写入。
- `domain.Task` 增加可持久化的执行身份：规范化仓库 URL、固定 commit、Go 版本、runner 版本和配置摘要。任务创建时写入；worker 启动前将持久身份与当前配置逐字段比较。
- 任务 ID 继续使用 16 字节随机值；不把 token 放入任务 JSON、报告、日志或分享 URL。

### 失败路径

- CAPTCHA verifier 为 nil 时 fail closed，返回 `503 captcha_unavailable`；verifier 失败返回 `403 captcha_failed`，不访问 GitHub、不写预览。
- 错误/缺失/过期控制令牌统一拒绝，不泄露任务是否存在；存储读取后使用 `subtle.ConstantTimeCompare` 比较哈希。
- 执行身份不匹配时任务终态为 `error`，保留记录供诊断，不调用 executor。
- runner 清理错误通过 `CleanupError` 传播到 worker；worker 保存 `cleanup_pending` 记录并返回控制层冲突，控制层不把取消或删除报告为成功。任务保留，待后续恢复/运维流程处理。

## 有意简化与上限

| 位置/简化及理由 | 适用上限 | 升级触发条件 | 升级路径 |
|---|---|---|---|
| CAPTCHA 只抽象 verifier，不在本阶段绑定供应商 | 单实例、供应商凭据未确定 | 选定供应商并需要真实端到端验证 | 增加服务端 verifier、密钥配置与供应商 contract test；未配置仍 fail closed |
| 公开 Origin 仅做可选白名单，主要依靠不可读的 Bearer header 抗 CSRF | 当前 API/单实例 | 需要浏览器跨站策略、CORS 或多域部署 | 配置精确公域名白名单并补浏览器集成测试；不把 token 放入 Cookie/URL |
| `cleanup_pending` 先保留为错误终态，不新增独立 cleanup 队列 | 当前单 worker、PUBLIC-4 未实施 | 需要自动重试清理或多实例 worker | 引入持久化 cleanup outbox/状态机，先完成 PUBLIC-4/5 设计 |
| 保留本机/LAN 兼容 handler，公开入口独立挂载 | 当前公网 listener 未开启 | 切换生产入口或删除旧兼容面 | 在 PUBLIC-6 发布变更中移除 pairing/session 路由并更新运维回退 |

## 阶段计划

| 阶段 | 目标与任务 | 输入/依赖 | 产物 | 退出标准 | 高风险(需独立复核) | 状态 |
|---|---|---|---|---|---|---|
| P3-A | 写红测试：token/CAPTCHA/公开路由/身份不匹配/清理失败 | PUBLIC-2 handler、SQLite store、worker | 针对性回归测试 | AC-301~306 的关键负向/正向路径有覆盖 | 是 | 通过（本地） |
| P3-B | 实现公开预览确认与令牌存储、Origin 边界 | P3-A | API、SQLite/Memory token store | AC-301~304 通过；任务与 token hash 同事务 | 是 | 通过（本地） |
| P3-C | 实现身份持久化校验和清理错误传播 | P3-A、runner cleanup | domain、worker、registry、runner | AC-305~306 通过；全局身份不误拒任务 | 是 | 通过（本地） |
| P3-D | 回归、文档和阶段验收 | P3-B/C | 阶段报告、优化记录、测试证据 | AC-307 与 smoke 通过；公网仍关闭 | 是 | 进行中（本地验证通过，远端 CI 待检查） |

## 交付与回退

检查点：保留当前 `feat/public-report-s1` 工作树所有未提交 PUBLIC-1/2 改动；新增文件、diff、测试日志和阶段报告作为证据。不得提交、推送、合并或开启公网 listener，除非用户另行要求。

## 决策记录

- 2026-11-27：用户明确要求“提PR推送”，覆盖此前暂不提交/推送的操作限制；授权范围仅为提交当前改造、推送当前功能分支并创建或更新 PR，不包括合并 PR、开放公网 listener 或声明生产公网可用。

回退：删除 PUBLIC-3 公开路由/令牌表/身份校验/cleanup error 增量并恢复对应测试；保留 PUBLIC-1/2 报告改动。数据库新表无数据时可保留，若需回退 schema 则先备份 SQLite 后在维护窗口删除。

## 决策与变更记录

| 日期 | 变更/决策 | 来源 | 原因 | 影响需求和阶段 |
|---|---|---|---|---|
| 2026-11-26 | PUBLIC-3 采用独立 `/api/v1/public/*` 写入口；不打开公网 listener | 用户已确认范围与连续执行授权 | 隔离旧 loopback/LAN 权限面，便于本地验收和回退 | R-301~307 |
| 2026-11-26 | CAPTCHA 未配置时 fail closed；供应商接入延期 | 用户已确认待定供应商 | 公网提交不能因缺配置而绕过人机验证 | R-302 |
| 2026-11-26 | 清理失败保留 `error/cleanup_pending` 任务而不删除 | AUD-003 审查结论 | 不虚报取消成功并保留运维恢复证据 | R-306 |
| 2026-11-27 | 并发取消 waiter 共享完成结果，避免单消费者通道导致永久等待 | 独立代码复核 ISSUE-026 | 多个持有效 token 的控制请求可并发到达，所有请求均须结束并观察同一 cleanup 结果 | R-306 |

## 历史

PUBLIC-2 只读分享设计见 `docs/superpowers/specs/2026-11-25-public-report-s2-design.md`；PUBLIC-2 未开放公网监听。
