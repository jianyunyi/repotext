# RepoTest 优化与修复设计

当前公网化改造的问题/方向依据见[公网化审查](publicization-review.md)，实施设计见[PUBLIC-1 设计](superpowers/specs/2026-11-25-public-report-s1-design.md)与[PUBLIC-3 设计](superpowers/specs/2026-11-26-public-report-s3-design.md)。PUBLIC-3 已在本地修复并验证 AUD-001 与 AUD-003；AUD-002、PUBLIC-5/6 仍是发布阻断。下方 ISSUE 记录保留原内网项目的历史关闭依据，不表示公网条件已满足。

<!-- 状态块 -->
更新：2026-11-25；状态：进行中；当前维护目录 F:/repotext。未关闭阻断项：ISSUE-010 七天观察未完成；延期风险 ISSUE-004。r22实际安装/固定work/恢复/监控与r23完整Linux回归通过，ISSUE-025/017/013已关闭；待决策：无，用户已批准具体部署。下一步核验2026-12-02T06:36:33Z后七天覆盖与退出回归；runner/LAN关闭。下方旧待授权/准备NOT RUN记述均为历史。

## PUBLIC-3 本地复核增补（2026-11-27）

- **AUD-001 根因与修复**：全局 worker 身份的 repo/commit 为空时，身份比较先无条件规范化空 repo，导致本应只比较 Go、runner 与配置摘要的任务被误判。空字段现跳过对应 repo/commit 比较；缺失任务身份仍 fail closed。正向全局身份与负向版本不匹配测试均通过。
- **AC-303 事务缺口与修复**：原公开确认先提交任务，再单独写控制令牌 hash，写入失败会留下无控制凭据的任务。公开确认现由 SQLite 单事务写入任务、outbox、确认映射和 token hash；注入 token 写入失败的回归证明事务回滚且预览仍可重试。公开响应仍只在首次新建时返回原始 token，重放不返回 token。
- **AUD-003 验证**：cleanup failure 经 worker/process registry 传播为 `cleanup_pending`；公开删除收到冲突且任务保留。控制层与 worker 针对性测试通过。
- **ISSUE-026 根因与修复**：并发 `CancelAndWait` 都可能捕获同一进程句柄，但原 buffered error channel 只能被一个 waiter 消费。现以一次性写入的共享结果和关闭广播通道供所有 waiter 读取；确定性并发测试先复现阻塞，再验证两方都收到同一清理错误。
- **范围边界**：`PublicHandler` 仍独立挂载；主服务默认 listener 保持 `127.0.0.1:8080`，未接入公网 listener。CAPTCHA verifier 缺失时 fail closed，供应商凭据尚未接入；AUD-002、PUBLIC-4/5/6 未在本阶段解决。
- **本地证据**：`go test -p=1 -count=1 ./...`、`go vet ./...`、`go build ./...`、`git diff --check` 退出 0；新增正向身份、事务回滚、重复确认、远程取消/删除及清理失败路由测试。并发 waiter 回归通过；race 检测未运行（`CGO_ENABLED=0` 且无本机 C 编译器）。远端 CI/PR 状态待检查。

## 问题总表
| 问题编号 | 所属阶段/验收 | 级别 | 类型 | 概要 | 状态 | 决策来源 |
|---|---|---|---|---|---|---|
| ISSUE-026 | PUBLIC-3/AC-306 | S2 | 并发错误 | 同一运行任务的并发取消者竞争读取单消费者结果通道，后续等待者会永久阻塞 | 已关闭；共享完成信号并只读错误结果；确定性双等待者回归重复通过（race 检测受本机 CGO 配置限制未运行） | 2026-11-27 独立代码复核；用户继续授权覆盖 |
| ISSUE-025 | S5/AC-009、AC-008工作盘 | S1 | 部署验收缺口 | 通用安装段只mkdir work，未建立持久固定容量挂载及失效启动门禁 | 已关闭；r22预分配/ENOSPC/隔离namespace同一guard拒绝/宿主依赖重挂载及cleanup PASS | 用户明确批准具体2 GiB工作盘与安装方案 |
| ISSUE-024 | S5/AC-009 | S2 | 诊断准确性 | sudo -n true 被误标为安装权限，受限sudoers可能高估 | 已关闭；三场景RED/GREEN、八shell语法、84文件源包与独立复核PASS | 独立复核反馈；用户continue授权覆盖 |
| ISSUE-022 | S5/AC-009 | S2 | 验证工具缺陷 | SSH 提前退出时 CopyTo 异常跳过 stdout/stderr/result 保存 | 已关闭；三场景 RED/GREEN、本轮 r16 完整失败日志与独立复核 | 2026-11-24 用户授权合并并继续 Linux 验收 |
| ISSUE-023 | S5/AC-009 | S2 | 检查材料位置错误 | 本轮解包 .go 副本进入 go ./... 扫描导致首轮编译失败 | 已关闭；改为 _package，go list 5包，Go/vet/build 当前复验通过 | 本轮合并材料必要修正 |
| ISSUE-020 | S5/AC-009 | S2 | 交付缺口 | 旧任务报告引用的统一修复包和最终汇总未生成，原目录未合并 | 已关闭；旧包检查、本轮86基线/快照、24文件精确合并和94文件合并后校验 PASS | 用户要求重新审查并继续完善；2026-11-24明确合并 |
| ISSUE-021 | S5/AC-009 | S1 | 验证门禁缺陷 | 预检漏悬空数据库链接、祖先目录链接，并接受非法端口 | 已关闭；r17 Linux完整S5 fixture拒绝路径/端口全部PASS | 用户继续完善的修复授权覆盖 |
| ISSUE-014 | S3/AC-004 | S1 | 用户流程缺口 | 会话撤销缺生产管理入口 | 已关闭；同字节旧包Windows/Chrome及r17 Linux Go会话回归通过 | 原单用户和设备撤销需求 |
| ISSUE-015 | S3/AC-002/003 | S1 | 用户流程缺口 | LAN 浏览器配对/只读页面缺失 | 已关闭；同字节旧包Windows/Chrome及r17 Linux LAN回归通过 | 原 LAN 只读与配对需求 |
| ISSUE-016 | S3/AC-005 | S1 | 用户流程缺口 | 活动态删除控件与迟到响应隔离缺失 | 已关闭；旧包浏览器及r17 Linux删除/取消/资源清理通过 | 原删除/取消需求 |
| ISSUE-017 | S5/AC-009 | S1 | 文档缺陷 | 安装说明缺二进制/属主/unit 与 reload | 已关闭；r22实际二进制哈希/属主/权限/unit verify及健康启动PASS | 用户明确批准实际安装 |
| ISSUE-018 | S5/AC-009 | S2 | 验证缺口 | CI 多路径 sh -n 仅覆盖首文件 | 已改逐文件检查，本轮七脚本 syntax PASS | 既有 S5 验证要求 |
| ISSUE-019 | S5/AC-009 | S2 | 验证缺口 | Linux 入口缺 S5、传输清单未覆盖全部被验文件 | 已关闭；r17传输83文件全部OK，完整S4/S5入口通过 | 既有 S5 验证要求 |
| ISSUE-011 | S5/AC-009 | S1 | 错误 | 并发备份失败时无条件删除目标，可能删除成功备份 | 已关闭；r17 Linux全量及S5备份竞争/失败清理回归PASS | 继续 S5 的既有修复授权 |
| ISSUE-012 | S5/AC-009 | S1 | 错误 | health URL 的 userinfo 可绕过 loopback 限制；curl --fail 接受 201 | 已关闭；r17 Linux HTTP200/201/userinfo/hostname四场景PASS | 继续 S5 的既有修复授权 |
| ISSUE-013 | S5/AC-009 | S1 | 文档缺陷 | 恢复流程缺少 WAL/SHM、属主和启动前完整性检查 | 已关闭；r22实际完整三件套保留、备份恢复/演练行清理/integrity及健康PASS | 用户明确批准恢复演练 |
| ISSUE-009 | S4/AC-008 | S1 | 错误/环境限制 | Linux 实测暴露 Git PAX 元数据误拒、依赖容器未打开 stdin、锁测试目录权限及目标 `.local` 0775 | 已关闭；r12 Linux 全套、quota 工作盘和 HTTP e2e 通过 | 用户授权继续 Linux 验收及必要修复；外部权限已具体审批 |
| ISSUE-001 | S1/AC-002 | S1 | 风险 | LAN 只读与本机变更授权边界必须由后端强制 | LAN allowlist、loopback-only mutation 与 Host/Origin 检查已实现；loopback CSRF token 与 Host/Origin 已实现并通过本机及 Debian 进程验证；LAN listener 已绑定部署接口 allowlist；关闭依据见 ISSUE-008 的 2026-11-19 Linux 复验 | 用户明确 LAN 只读；实现措施为助手建议 |
| ISSUE-002 | S1/AC-003 | S1 | 错误 | 旧 POST /tasks 曾绕过预览确认 | 当前 POST `/api/v1/tasks` 仅接受一次性持久化 preview token；固定 commit、loopback + CSRF 验证后事务创建 task/outbox。handler、任务/报告页面和目标 Linux HTTP worker 链路已通过 r8 验证；默认启动仍 runner disabled | 用户已授权继续 S4；仍沿用预览后明确确认 |
| ISSUE-003 | S3/AC-002 | S1 | 风险 | cancel 管理路由未验证本机来源 | loopbackOnly + loopback IP Host/Origin 已实现；本机取消成功、LAN 来源拒绝测试通过 | 用户明确本机控制、LAN 只读 |
| ISSUE-004 | S1/AC-008 | S1 | 限制 | 容器共享宿主 Linux 内核；rootful daemon 与其他应用共宿主 | AC-008 r10 已通过（含 ext4 工作盘硬 quota）；风险仍保留，执行不面向陌生代码或公网 | 用户已授权继续 S4；验收门槛来自修订设计 |
| ISSUE-010 | S5/AC-009 | S1 | 验收缺口 | 七天实际观察覆盖尚未完成 | r22安装/恢复/监控与r23回归PASS；观察2026-11-25T06:36:33Z开始，七天未到，不关闭 | 用户明确批准安装/恢复/健康观察并中断后continue |

| ISSUE-005 | S1/AC-009 | S2 | 限制 | Linux 主机可用，但架构、内核、cgroups、容器运行时事实未核验 | 已核验：Debian 13 x86_64、cgroup v2、Docker 29.8.2；部署阶段仍需独立 quota/运维验收 | 用户提供环境事实，实际能力须现场读取 |
| ISSUE-006 | S1/AC-007 | S2 | 风险 | 结果复用键需固定 Go、runner、配置 digest，唯一性和完成窗口需实现 | 已实现并通过 SQLite 并发、版本差异、完成窗口和重启 claim/lease 测试 | 用户明确缓存维度 |
| ISSUE-007 | S3/AC-002/005 | S1 | 缺陷 | loopback 删除路由缺失、双 listener 非原子启动、localhost bind 可绕过来源策略 | 已关闭；本轮及后续 ISSUE-008/S4 集成验证通过 | 用户确认本机操作与 LAN 只读；实现为助手建议 |

## ISSUE-025：持久工作盘与具体部署门禁

- 发现基线：a7fe305；独立 s5_deployment_review 读取通用安装流程，确认其仅创建 /var/lib/repotest/work 普通目录，与 deployment.md 固定容量要求有差距。此为准备阶段历史观察；实际关闭依据见下述r22记录。
- 影响及同类路径：通用 deployment.md 安装段与 deploy/repotest.service 缺持久挂载依赖；check-deployment.sh 只检查绝对路径/链接/权限/runner门禁，不证明磁盘硬容量。prepare-s5-deployment.sh 是临时构建和事实采集，不应为部署或容量兜底。既有 S4 临时2 GiB盘验收不证明新持久盘。
- 方案：2026-11-25具体计划中新建2 GiB ext4镜像、专用mount unit、RequiresMountsFor和mountpoint ExecStartPre；镜像独占创建拒覆盖，fallocate后mkfs -E nodiscard防止打孔，并检查实际blocks×512。工作盘挂载后专用属主0700，nodev/nosuid/noexec；默认runner/LAN关闭。
- 同次准备补有界健康等待，真实恢复用明确synthetic session验证状态，health timer只记录无秘密字段。无新依赖或产品代码改动；预期收益为堵住部署容量/启动与验收缺口，未测量性能收益。2 GiB预分配消耗目标约半数当前可用根盘空间，需明确批准并记录剩余量。
- 复验：当前计划shell语法和独立审阅只能证明准备；实际mounted source/options/permissions、镜像分配、mount失效阻止启动、服务恢复与观察均按计划执行后才可关闭。启用runner前在持久work上重复AC-008压力/HTTP资源清理，不以静态unit检查代替。
- 回退：先停健康timer，再停RepoTest和自有mount；保留镜像/DB/原始日志，不自动删除账户或数据。用户已明确批准该目标安装/挂载/恢复/观察；未自动删除账户或数据，未执行服务回退。
- 当前关闭依据：r22 install/mount实际exit0，镜像大小2147483648、分配2147487744字节，repotest压力写2022703104字节后ENOSPC并清理；私有mount namespace卸载准确work后同一ExecStartPre命令rc32拒绝，宿主挂载仍保留；停自有mount后systemd依赖重挂载/健康PASS。该隔离变体验证不声称真实主服务发生启动失败；在新持久work上启用runner前仍需该变体HTTP执行/资源清理验证。

## ISSUE-024：准备脚本的sudo权限事实

- 独立复核发现scripts/prepare-s5-deployment.sh的INSTALL_PRIVILEGE标签高估sudo -n true含义；sudoers可以仅允许true，而拒绝useradd/install/runuser/systemctl。原脚本没有实际安装，但会影响下一步具体权限判断。
- 修复方案：仅标记SUDO_PROBE=root_uid/true_noninteractive/unavailable；采集sudo -n -l的实际规则或失败输出，明确INSTALL_COMMAND_PERMISSIONS=UNVERIFIED。UID0也仅作为事实，不能外推宿主权限/namespace或systemd能力；后续按目标规则核实具体安装操作。不改变安装标准或实际服务。
- 排查范围：prepare脚本权限块、deployment.md准备说明、stage-report当前记录；其他生产代码不使用此标签。验证使用本地假sudo政策（仅true允许、政策拒绝、UID0）检查实际标记，不访问真实SSH/sudo或目标。
- RED证据：work/s5-deployment-preparation-20261124/sudo-red.log，三场景原脚本FAIL；预期新标记和受限规则输出。修复后重跑三场景、八个shell语法、源包字节与独立复核；完整目标准备仍须实际运行。
- 回退：本阶段新增prepare脚本可以移除，原文档恢复95文件基线对应.snapshot；未部署、未恢复实际数据库。
- 关闭依据：主代理三场景GREEN，独立merge_driver_review另行运行无真实sudo的三场景回归及bash -n，均通过；重算r18全部84清单字节和归档SHA一致（dac0f34d956fc954384da7e9c9c6580f45892d9029f44ad3f2ed558a0bd14afc）。此关闭只针对权限标记准确性，不将目标准备或实际安装权限改为PASS。

## ISSUE-022：Linux 验收驱动保留传输失败证据

- 发现与复现：原目录合并后运行 `scripts/invoke-linux-acceptance.ps1 -EvidenceName s5-linux-20261124-r15`，CopyTo 报 pipe ended；只有源包，没有 stdout.log/stderr.log/result.json。独立 SSH 与文件读取都证明配置访问 Permission denied。
- 根因：驱动虽已异步读取子进程输出，CopyTo 异常却在等待进程和保存结果前终止脚本。搜索 scripts 中 CopyTo/ReadToEndAsync，只有此入口；不修改 SSH 配置、认证参数或远端脚本。
- 方案与取舍：捕获传输错误，关闭 stdin 后等待子进程，保存真实 stdout/stderr、进程退出码与传输错误；若子进程 exit 0 但传输失败，整个驱动仍 exit 1。直接修复现有流程；外部 wrapper 只能补本次记录，不能修复日后同类失败。
- 验收：`python scripts/test-linux-acceptance-driver.py` 使用本地真实子进程验证提前失败、成功传输字节 SHA-256、提前 exit 0 三条路径；不连 SSH，不计 Linux PASS。RED 证据 work/merge-20261124/driver-red.log。修复后使用新证据名重跑真实入口并记录 SSH 阻塞。
- 回退：精确包版本在 work/merge-20261124/_package/changes/scripts/invoke-linux-acceptance.ps1；本轮原目录快照在 before/。没有服务安装或实际数据库变更。
- 决策来源：本次继续 Linux 验收所需的诊断修复，用户此前与当前连续修复授权覆盖；不改变验收标准或部署范围。
- 关闭依据：driver-green.log 三场景 PASS；r16 实际 exit 255，保存原始 stderr（SSH 配置 Permission denied）与 transfer_error；独立 merge_driver_review 重算包/清单并核对源码与证据，未发现高、中严重问题。此关闭仅针对日志工具，不关闭 Linux 验收。

## ISSUE-023：合并检查材料的 Go 发现范围

- 本轮首轮 Go/vet/build 发现 work/merge-20261124/package/changes/internal/control 不完整副本编译失败；根因是解包目录含正常 .go，Go ./... 将其纳入扫描。实现本身未变更。
- 方案：核对两个绝对路径属于本轮 work 目录后，将 package 移为 Go 忽略的 _package；快照仍全部 .snapshot，并修正 merge.py 与交接引用。没有删测试、改生产逻辑或降低标准。
- 验证：go-packages.log 恰为5个根项目包；go-test-current.jsonl 为156 PASS/6 SKIP/0 FAIL，vet/build 当前结果均exit0。首轮 FAIL 日志保留为历史，见阶段报告。同类路径审查范围是 work/merge-20261124/_package、before 和当前 go list；远端源包只包含 cmd/internal/scripts/deploy/go.mod/go.sum，与此材料无关。
- 关闭依据：上述实际复验与独立复核；不要把恢复材料重新放到 Go 可发现目录。修复该材料位置计一轮返工。

## ISSUE-021：部署预检路径与端口边界

- 发现版本：本轮 94 文件会话基线，work/continuation-20261124/session-baseline.json；相关代码 scripts/check-deployment.sh。
- 根因：`test -e` 对悬空链接返回 false，原数据库条件跳过链接拒绝；只对末级目录使用 `test -L`，漏掉祖先链接；loopback 地址使用 glob 而没有检查端口。端口本身仍被服务启动拒绝，因此该项单独是 S2 契约缺口；路径拒绝不完整按 S1 处理。
- 影响与同类路径：数据库文件、数据库父目录和 work 目录均经过部署预检；生产 main.go 的 loopback listener 检查保留。审查脚本的路径检查调用、部署手册与 verify-s5-operations.sh，不把预检当作运行时无竞态的文件访问保证。
- 方案：对数据库/work 的每个路径组件拒绝链接，涵盖尾部斜杠；显式拒绝悬空数据库链接；检查 loopback 端口合法；保留原 0700/0600 私有权限要求并修正错误提示。
- 验证：先在现有临时 S5 fixture 增加悬空 db、祖先目录链接及非法端口检查；本机能运行的分支实际复验，目标 Linux 不能运行时记录 BLOCKED/NOT RUN，不能将静态审查写成 PASS。累积 Go/浏览器检查按受影响范围复用，本轮未改服务源码。
- 取舍与代价：复用 POSIX shell/stat，无依赖；不新增发布范围。特权操作者在检查后替换路径的竞态不由此预检解决。
- 回退：恢复 work/continuation-20261124/before/scripts/ 中两个 .snapshot 文件；原 F:/repotext 未改。
- 决策来源：用户明确继续完善与审查后的必要修复；Linux 与实际部署边界不变。
- 实际检查：work/continuation-20261124/delivery-checks.json；端口八场景中五个非法值在原隔离代码块均 exit 0、新块均 exit 1；三个有效值仍 exit 0。两个实际 Windows junction 路径被新完整预检以 symlink 原因拒绝。完整 Linux fixture 未运行，原始目标链接分支 RED/GREEN 仍欠证据；不把 junction 结果外推为 Linux PASS。
- 独立复核：readiness_review 最初只读发现根因，随后实施两脚本，故该 Agent 不是修复后的独立审查者。主代理未参与脚本实施，已逐项核对并实际复跑上述边界及最终源包哈希。
- 上述“Linux待验”为合并前历史；r17的S5_PREFLIGHT_SYMLINK/ANCESTOR/TRAILING/DB_DANGLING/PORT等标记全部通过，verified-summary.json核验83精确源文件与目标OK记录，完成完整Linuxfixture关闭本问题。

## ISSUE-020：中断的统一交付恢复

- 原因与证据：临时副本代码及运行日志仍在，但 work/delivery-progress-20261123 和 final-summary.json 不存在，旧 Task3 四项未勾选。
- 方案：保存完整持久副本，核对原 F:/repotext 的全部 86 基线文件与新增集合，生成精确改动文件/哈希/补丁、真实 diff 和交接说明；独立验证 ZIP CRC、路径、逐字节哈希以及只读补丁检查。
- 本轮基线：94 文件 .snapshot 和 session-baseline.json；区分本轮改动与此前累计修复，缓存/生成包/数据库/截图不计源码 diff。
- 验证与恢复：最终 package-result.json 为交付检查结果。原目录保持原状；未来合并前再验基线并备份待替换文件，恢复步骤见 README-HANDOFF.md。
- 上述是 C: 包制作时的历史观察；本轮已在 F: 完成原目录合并、快照、逐字节复验与本机回归，当前依据 work/merge-20261124/summary.json，恢复步骤见该目录 README-HANDOFF.md。

## ISSUE-009：2026-11-21 Linux 实测修复方案
- 复现：全套 Linux 测试中 4 个测试失败；基础 CPU/PID/OOM/只读/断网探针已通过。原始证据见 `work/s4-linux-20261121-r3/`。
- 已确认根因：GitHub 原始 tarball 首项为 `pax_global_header` / type `g`；依赖容器创建参数缺少 `-i`；Go testing 子目录未显式 chmod 0700；目标 `/data/home/.local` 为用户所有的 0775，生产检查按设计拒绝组可写路径。
- 最小方案：只接纳无文件语义的 Git 全局 `comment` PAX 记录，其他全局记录拒绝；依赖容器打开 stdin；锁测试创建私有夹具。目标目录仅在具体审批后从 0775 调整为 0755，不放宽生产校验。
- 同类路径：复查 `extractTarGz` 的路径/链接/PAX 回归、依赖与离线容器 stdin 参数、`RunnerServiceLockFile` 和 `AcquireRunnerWorkLock`。离线测试容器原已包含 `-i`。
- 验证：已补充 PAX 注释头与恶意全局路径回归；r8 重跑 Linux 全套测试/vet/build、资源 smoke、固定 commit 离线执行、HTTP 执行/报告/取消和清理。持久磁盘 quota 仍单独 NOT RUN。
- 回退：本轮修改前文件副本保存在 `work/s4-linux-20261121-before/`；远端 `.local` 权限原为 0775，当前已按具体授权调整为 0755；恢复为 0775 后 runner 应重新 fail closed。执行配置保持默认关闭。
- 另有验收工具问题：Windows .NET SSH stdin 在压缩包前添加 UTF-8 BOM，远端 `od` 实测为 `ef bb bf 1f 8b`。驱动改用无 BOM 的 Console.InputEncoding，r3 已成功传输。Windows .NET Framework 不支持 StandardInputEncoding 的尝试未到达目标机。

## ISSUE-010：S5 部署与运维交付
当前进展（2026-11-25）：r20准备exit0/stderr0，84源文件及六产物校验；r21 Linux备份/健康/预检/静态unit夹具通过，Windows test/vet/build通过。SSH沙箱直接调用仍拒绝配置，但提权现已正常；目标事实不再阻塞。具体计划见superpowers/plans/2026-11-25-s5-target-deployment.md，独立复核补充ISSUE-025；实际安装/恢复/观察待批准。以下2026-11-24及r12内容保留为历史，不表示本轮部署完成。
- 2026-11-24 r17完整目标Linux回归已通过，164测试PASS/0SKIP/0FAIL，资源/HTTP/quota/备份/health/preflight均通过；详见 work/s5-linux-20261124-r17/verified-summary.json。用户在代理沙箱外运行，代理内SSH配置读取仍受管拒绝。接下来复用deployment.md的临时构建与既定安装布局，先收集PID1/systemd、权限、已有账户/文件及8080占用事实，构建两二进制并记录SHA；保存私有临时准备目录，不写服务/账户/实际数据库。已有部署存在时不能直接套用新安装步骤。后续部署方案须以目标事实与精确二进制为依据，7天观察尚未开始。此准备工作由用户继续P5授权覆盖，实际部署仍按既定具体目标/动作边界确认。
- 范围：systemd 服务模板、默认关闭执行的环境模板、SQLite 一致性备份/恢复说明、healthz 监控探针、Windows/Linux CI 和 r10 全流程 smoke。
- 约束：只生成和 dry-run，不部署、不启用 LAN、不修改目标机现有服务；rootful Docker socket 风险继续显式记录。
- 退出标准：部署 preflight 拒绝缺失/可写路径和未固定镜像；备份可重放并验证 SQLite 完整性；healthz 失败有非零退出；Windows 构建与 Linux r10 smoke 可重跑；回滚步骤明确。
- 本轮证据：目标机 preflight/health shell dry-run 通过；备份聚焦测试和 Windows/Linux Go 回归通过；Linux r12 smoke 通过。未执行 systemd 安装、服务停止/恢复演练或 7 天观察。

## 2026-11-23 · 项目进度重新复核与修复方案

决策来源：用户明确要求旧任务不可续接时重新审查项目进度并继续完善后续阶段。沿用现有需求与连续修复授权，不扩大部署、LAN 暴露或陌生仓库执行范围。完整方案和退出标准见 superpowers/specs/2026-11-23-progress-repair-design.md；任务见 superpowers/plans/2026-11-23-progress-repair.md。

- ISSUE-014，S1 错误，AC-004：Pairing.Revoke/SQLiteStore.RevokeSession 只有测试调用，生产管理入口缺失。真实临时服务本机 GET /api/v1/sessions 返回 404。补本机受 loopbackOnly/CSRF 保护的活动会话列表和按 SHA-256 句柄撤销，复用 sessions 表，验证指定撤销后旧 cookie 401、另一 cookie 仍有效。回退本轮 endpoint/UI 文件，不删除会话数据。
- ISSUE-015，S1 错误，AC-002/003：LAN GET / 真实进程返回 404，浏览器不能配对与查看报告。补同源配对/按 ID 只读页面，不传本机 CSRF，不暴露任务和会话 mutation。验证配对成功/失败、401、Host 与 405、HTML 转义。回退新页面及 GET / 路由，保留现有 API。
- ISSUE-016，S1 错误，AC-005：浏览器 watchTask 在活动态提前 return，真实运行态渲染没有删除控件；底层活动删除已有历史 Linux PASS。为活动态暴露现有 DELETE 接口并以视图版本/计时器阻止迟到轮询覆盖删除结果。复验完成、失败重试、删除中迟到回复；目标资源回收仍要求 Linux smoke。回退 UI 增量，不改底层取消协议。
- ISSUE-017，S1 文档缺陷，AC-009：安装缺 backup binary、目录 repotest 属主、unit 安装/daemon-reload。补完整 build/hash 与经单独授权后的 install 步骤，专用账户使用真实 /var/lib/repotest home。独立复核路径、用户、权限和恢复调用；实际安装/恢复仍 NOT RUN。
- ISSUE-018，S2 验证缺口，AC-009：sh -n scripts/*.sh 仅检查第一个脚本，已用有效首脚本与第二个 if then 脚本复现旧命令 exit 0。CI 改逐脚本检查并验证第二脚本失败阻断。回退 CI 单行，不能将旧覆盖度当全脚本 PASS。
- ISSUE-019，S2 验证缺口，AC-009：完整 Linux 入口只跑 S4；传输与源哈希未覆盖部署文件/新 health 回归。接入仅用临时 fixture 的 S5 检查入口，覆盖 backup/health/preflight；传输并在 CRLF 规范化前核对脚本/deploy/Go 源哈希。目标执行未完成时保持 BLOCKED；回退验收脚本和传输驱动增量。

同类路径排查：现有本机 preview/confirm/cancel/delete 全部复用 loopbackOnly；LAN 任务/报告/日志读取已有会话检查；sessions 只有现有 token_hash/expires_at/revoked 表，不新增认证模型。前端所有任务状态进入 watchTask，报告由 loadReport 追加，需同时处理两条迟到响应路径。CI 与手动多文件 sh -n 写法同时校正，历史执行结果保留日期。

恢复快照：work/progress-repair-20261123/before/*.snapshot 与 baseline.json，原目录 F:/repotext 保持不变。新修复另行打包，旧 reviewed ZIP 固定不变。

## ISSUE-001：LAN 只读与本机变更授权
- 发现依据：用户选择 LAN 可访问、设备配对、LAN 只读；现骨架 `internal/control/http.go` 的 submit/cancel 路由不区分来源和授权。
- 影响：同网段配对查看设备若能发 POST，可提交并运行代码或取消任务，违反本机主动授权边界。
- 根因：接口层没有分离读 capability 与 mutation capability。
- 方案：区分 loopback 管理 listener 与 LAN read listener；服务层对每个 mutation 强制本机能力与 CSRF/Origin 验证；配对 cookie 只授予读取。防 DNS rebinding，禁止 LAN 会话调用任何副作用 endpoint。
- 替代：单 listener + 路由中间件判来源；复杂度较低但容易受 Host/Origin 混淆，暂不推荐。
- 验证：LAN 已绑定会话对 POST submit/confirm/cancel/delete 全得拒绝；loopback 合法 CSRF 可成功；伪造 Host、Origin、DNS rebinding 请求拒绝。
- 回退：关闭 LAN listener，仅 loopback 可用。
- 决策来源：LAN 只读是用户决定；双 listener/CSRF 技术实现为助手建议，S1 待确认。

## ISSUE-002：预览和授权阶段缺失
- 发现依据：`internal/control/http.go` 现有 POST `/api/v1/tasks` 直接 Create queued；没有 GitHub metadata/ref resolve/preview，也没有确认 commit API。
- 影响：无法展示 SHA 供用户确认，当前 POST 与要求冲突。
- 根因：初始骨架先于完整需求决策建立。
- 方案：改成 POST preview (无任务创建)、GET preview、POST confirm (仅 loopback 且一次性 preview nonce)；执行器未注册时 confirm 返回明确 `runner_disabled`，无模拟任务。
- 验证：非法 URL/ref、私有仓库、重定向/限流分支；合法 ref 固定 SHA；preview 不写 task；重复/过期 nonce 拒绝。
- 回退：保留预览 UI，关闭确认路由。
- 决策来源：用户明确预览后确认和 B（不造模拟任务）；API 形态为助手建议，S1待确认。

## ISSUE-003：取消无认证
- 发现依据：`internal/control/http.go` POST `/api/v1/tasks/{id}/cancel` 只检查任务存在与状态。
- 影响：任何可访问 API 的 LAN 客户端都能取消。
- 方案：S3 引入本机 mutation capability、CSRF 和状态 CAS；LAN handler 无该路由。删除运行任务等待进程回收后再删数据。
- 验证：LAN 取消/删除拒绝；本机无效 CSRF 拒绝；完成与取消竞态仅单一终态；删除后报告日志不可读取。
- 回退：暂时禁用取消/删除 endpoint。
- 决策来源：用户确认仅本机可删除；取消沿用“本机主动控制”的已确认边界。

## ISSUE-004：本机执行器不能提供隔离
- 发现依据：`internal/runner/process.go` 使用 `exec.CommandContext` 执行任意指定程序；不是容器隔离，无 Unix 进程组 kill、cgroup 或网络隔离。
- 影响：恶意仓库可访问当前用户权限、LAN 和文件；不得用于陌生仓库或公网服务。
- 方案：保持执行器未接入 Web；S4 使用 Linux 容器执行器，固定参数、无凭据、两阶段网络、硬限制；目标环境验收不过不启用。
- 验证：Linux 恶意联网、超时子孙进程、PID/内存/磁盘限制、工作区逃逸、租约恢复。
- 回退：全局 `RUNNER_ENABLED=false`，仅允许预览/报告历史。
- 决策来源：用户先做页面流程、执行器后续；Linux 主机可用。

## 历史
此前设计初稿 `docs/DESIGN-REVIEW.md` 是 PDF 的审查提纲。本文以用户随后确认的本地 Web/LAN 只读/preview-confirm 流程为当前权威基线；后续若有冲突，以 `project-design.md` 的决策记录和本问题总表为准。


## ISSUE-007：loopback 删除能力与双 listener 原子启动缺少实现/验证（本轮已修复）
- 级别/类型：S1 / 缺陷与验证缺口；关联 R-002、R-005、AC-002、AC-005。
- 证据：`internal/control/handler.go` 未注册删除路由；`cmd/repotest/main.go` 先启动 LAN serve goroutine，再调用 loopback `ListenAndServe`，listener 绑定并非原子事务；截至本轮 process smoke 仅覆盖配对读取、报告/日志读取及 LAN POST 拒绝。
- 方案：先以测试固定 loopback 删除终态任务、拒绝活动态任务、LAN 删除拒绝；管理请求验证 loopback RemoteAddr 与本机 Host/Origin。重构启动顺序为先绑定 loopback 与可选 LAN 两个 socket，任何 bind 失败先关闭已绑定 socket，两个 bind 都成功后再 serve。进程级测试占用 LAN 地址并确认服务退出且 loopback 地址未被监听。
- 替代：仅在 handler 层假设“只从 loopback mount”且不测启动失败；风险是未来 listener 接线或错误返回产生暴露窗口，因此不采用。
- 预期收益：本机删除与 LAN read-only 边界可复核；启动失败不留下半启动 listener。代价：启动器需显式持有/关闭两个 listener。
- 回退：删除本机 DELETE 路由并禁用 LAN listener；可还原 main 的既有启动逻辑，但不得宣称原子启动验收通过。
- 验证：`go test ./...`、`go vet ./...`、`go build ./...`；进程级失败启动场景；远端双 listener/配对读取 smoke。
- 决策来源：此前用户明确授权删除/取消只由本机发起、LAN 只读及逐阶段实现；活动态删除维持拒绝为执行器未接入时的安全限制。
| ISSUE-007 | S3/AC-002/005 | S1 | 缺陷 | loopback 删除路由缺失、双 listener 非原子启动、localhost bind 可绕过来源策略 | 本轮已实现并通过验证；独立复核问题已关闭；不代表整体 AC-002/005 通过 | 用户确认本机操作与 LAN 只读；具体实现是助手建议 |

### ISSUE-007 当前结果（2026-11-18）
- 状态：已实现并通过本轮检查，独立复核关闭其 S1 子项。
- 实现：`cmd/repotest/main.go` 仅允许显式 loopback IP 管理 bind；`bindListeners` 在启动 HTTP Serve 前先绑定全部 listener，第二个 bind 失败时关闭第一个。`internal/control/local_request.go` 要求 loopback 来源及 IP Host/Origin。管理端仅删除终态任务；LAN handler 不提供删除/取消/预览 mutation。
- 测试：终态任务删除后报告/日志级联不可读；活动任务删除返回 409；LAN 仍拒绝删除；loopback cancel 成功、非 loopback cancel 拒绝；Process 启动时 LAN bind 失败、loopback bind 失败及 SQLite 初始化失败均不留下任何 listener。Debian 13 x86_64 远端 `go test ./...` 与进程级 HTTP 检查通过。
- 独立复核：复核指出 `localhost` bind/Host 风险；修复为显式 loopback IP 并加测试后，复核确认该项关闭。范围限于本轮改动，不代表 S3 全面安全验收。
- 未完成：设计要求的 CSRF token 已实现并完成本机及 Debian 进程级验证；活动任务在执行器未接入前只能 409，不能满足取消和确认进程回收；AC-007 幂等 cache key/outbox 未实现。

## ISSUE-008：活动任务回收、幂等 outbox 与 LAN Host allowlist
- 级别/类型：S1 / 验收缺口；关联 AC-002、AC-005、AC-007。实现已完成，等待 Linux 实机复验与 S3 阶段验收。
- 发现依据：Linux ProcessRunner 缺少进程组终止和任务生命周期登记；SQLite `tasks` 缺少唯一复用键/outbox；LAN handler 未按实际 bind 接口限制 Host。
- 实施顺序：① Linux ProcessRunner 进程组取消 + 任务进程 registry，删除运行任务先取消、等待进程树退出再 CAS 终态并级联删除；无可等待的活任务进程则拒绝删除。② SQLite 事务加入确定性 work key 唯一约束和 outbox row，重复并发请求返回相同任务，不重复入队；key 包含规范化 repo、40 位 SHA、Go 版本、runner 版本、配置摘要。③ LAN Host allowlist 从已 bind 的具体 IP 或通配 listener 启动时的接口地址集生成，handler 每次请求校验，Host 包含端口时只比较规范化 host；接口变更需要重启。
- 实现：Linux runner 以独立进程组执行并在取消时杀死整个组；task process registry 提供取消并等待退出，HTTP 删除只在等待完成并确认任务已终止后级联删除。SQLite 在同一事务写 task 与 outbox，唯一 work key 由服务端从规范化仓库、完整 SHA、Go/runner 版本和配置摘要生成；claim lease、重试、ack 可跨重启。LAN Host 集合从已绑定 listener 地址推导，通配地址使用启动时具体接口 IP 集合；未知 hostname 拒绝，接口变化要求重启。
- 验收：Linux 启动父/子进程，取消后进程组停止并由 `Wait` 回收父进程；HTTP 删除等待退出再删，超时不删；并发相同 key 只创建一个 task/outbox 且调用方拿到同一 task；配置版本改变生成不同 key；SQLite 重启后 outbox 可恢复 claim；非 allowlist Host 拒绝，允许 IP 可访问。
- 历史风险与限制（2026-11-19，S4 接入前）：当时 ProcessRunner 不是沙箱，worker 尚未接入。当前 Docker 执行器和 TaskWorker 已在 S4 接入，资源/恢复见 r10/r14；仍不承诺外部 exactly-once，部署接口 IP 变化仍需重启，rootful Docker 共宿主/共享内核风险仍归 ISSUE-004。
- 回退：保持活动任务 DELETE 返回 409；禁用任务复用/outbox 与 LAN listener，回退到 loopback-only 模式。数据库迁移前备份副本，迁移失败拒绝启动且不重置数据。
- 决策来源：用户已明确授权“活动任务停止与进程回收、幂等键和 outbox、LAN Host allowlist 与部署接口绑定”；具体方案按既有 7 天保留、10 分钟复用和 LAN 只读决策执行。

## 2026-11-23 · S5 收尾修复方案

恢复来源为 2026-11-21 保存对话及 r14。原目录 F:/repotext 只读，修复副本位于本会话系统临时目录。r14 exit 0，全部 Go 源文件哈希与当前原目录一致；不将历史测试等同于新修复的 Linux 复验。

- ISSUE-011：`validateDestination` 的检查到 `VACUUM INTO` 创建之间存在竞争，错误清理无文件所有权。使用标准库 `O_CREATE|O_EXCL` 以 0600 独占创建空输出，SQLite 在该空文件中备份；仅清理本次独占取得的文件。替代方案是临时文件加原子发布，但维护成本更高。验证并发调用仅一个成功、成功备份不被删除及完整性；回退恢复旧实现并禁止并发备份。
- ISSUE-012：curl 的 URI authority 支持 userinfo，旧前缀 glob 未固定真实 host。按 authority 精确拆出 loopback 和纯数字端口，再要求 curl 输出的状态码为 200。复用 shell/curl，不加依赖。保留实际 loopback HTTP 200/201、userinfo/hostname 在请求前拒绝的可运行检查。回退禁用探针并由管理员手动检查。
- ISSUE-013：保留停止服务后的主库及 WAL/SHM，在同目录备份其快照，再安装已验证备份，明确 0600 和 repotest 属主。提供先 integrity check、后启动的步骤；实际目标服务恢复不在本轮授权范围，保持 NOT RUN。回退依据保留的原数据库集合。
- 排查范围：备份 CLI/全部 backupDatabase 调用与测试、健康 shell/运维说明/CI 调用、systemd 用户和停止策略。S4 runner 不受这些变更影响；Linux runner、systemd 安装和长期观察未因本轮被豁免。
- 决策来源：用户继续已授权 S5 工作及例行修复；保留既有 AC-009 基线。

### 修复结果与证据

ISSUE-011 使用校验后执行函数作为确定性竞争检查边界，无生产时序钩子。旧逻辑丢失竞争调用的备份，修复后独占创建及失败输出清理检查通过。ISSUE-012 的实际 HTTP 检查先暴露 201/userinfo 错误，修复后四项通过。Windows 全套 test/vet/build exit 0；独立复核未发现本次变更中的 S0/S1。ISSUE-013 的说明修正通过静态复核，实际恢复仍为 NOT RUN。证据位于 work/verification，历史 r14 位于 work/s4-linux-20261121-r14。

### 2026-11-23 · 进度复核修复结果

ISSUE-014/015/016 已在副本通过新增五项集成覆盖（RED 为四项失败、一项已有边界通过；GREEN 为五项通过）与真实 Chrome 八组检查。设备句柄不具 bearer 能力，指定撤销后旧会话读取 401；LAN 页面无管理 CSRF，任务及会话 mutation 拒绝；活动删除调用现有接口，迟到 poll 有浏览器实测，report/cancel 的版本保护独立静态复核通过。

ISSUE-017 安装说明经独立复核，实际安装仍 NOT RUN。ISSUE-018 新命令能拒绝第二个坏脚本，旧命令 exit 0，新命令 exit 1；七脚本逐一语法通过。ISSUE-019 包/清单集合与逐字节哈希通过，CRLF 原字节检查成功、转换后检查失败；目标 S5 权限 fixture 和完整新回归仍 BLOCKED，不能以 Windows/Git Bash 结果代替。

全部本轮 Go test/vet/build、格式与包发现通过；两个 Linux/amd64 命令仅交叉编译通过，未在目标运行。S5 规格由 continuation_review 复核，S3 规格同代理复核，S3 安全/代码质量由未参与该实现的 s5_readiness_review 复核；范围内未发现剩余 S0/S1 或必须修复的 S2。证据见 work/progress-repair-20261123，恢复按阶段前 .snapshot 文件，不改变原 F:/repotext。

## ISSUE-020：PUBLIC-2 不可用分享页
- 级别/类型：S2，错误；关联 PUBLIC-2 页面验收。状态：已关闭（2026-11-26，红绿回归与全量本地验证通过）。
- 证据：`GET /r/{id}` 对不存在任务复用 API JSON 错误响应；新增页面回归测试失败，返回 `404 application/json`，浏览器分享链接没有解释文案。API 本身仍应维持 JSON 契约。
- 根因：`publicPage` 将非过期错误直接委托 `writePublicReportError`，未区分 HTML 页面与 JSON API 的表示层。
- 方案：页面路由对缺失/格式错误 ID 返回通用 HTML 404，对存储不可用返回不泄漏内部详情的 HTML 503；复用过期说明版式，不加载数据脚本。API 路由与错误码保持不变。过期/不可用页面不提供重提交按钮，公开提交依赖 PUBLIC-3，留在其阶段实现。
- 替代方案：把详情留给前端 JSON bootstrap；但当服务端无法读取报告时，页面本身仍只显示空白/脚本加载状态，故不采用。
- 风险与回退：只影响 `/r/{id}` 错误呈现；可回退 handler/template 增量及测试，不触及 API、数据库、监听器或 runner。
- 验证：红绿测试覆盖已过期 410 HTML、物理缺失 404 HTML、错误页不加载公开数据脚本；重跑 `internal/control`、全量 test/vet/build 与 diff 检查。
- 决策来源：PUBLIC-2 已批准的只读范围与连续执行授权；重提交能力未获本阶段范围授权，推迟 PUBLIC-3。
