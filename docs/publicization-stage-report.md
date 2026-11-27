# RepoTest 公网化阶段报告

模式：Full；沟通方式：连续执行。PUBLIC-1报告层：通过（Windows Go 1.23.12本机全量验证及GitHub Actions运行45777149693的Linux/Windows job均通过）；PUBLIC-2只读分享：本地验收通过；PUBLIC-3：本地验收通过，正在检查并更新 PR #1；公网监听仍关闭。
基线：10a0bc5；分支：feat/public-report-s1。用户已要求按公网化PDF进行，并确认起步并发1。
未关闭发布阻断：AUD-002完成写库后卡住；PUBLIC-5专用沙箱与PUBLIC-6全部上线门槛。AUD-001与AUD-003已在 PUBLIC-3 本地修复并通过回归，远端 PR 检查待完成。
待补事实：域名、专用机器/云厂商/预算、后续验证码凭据；报告与本地只读分享实现不依赖这些。旧失败运行45624252912（929adaa）已由修复提交1c2c5ef及成功运行45777149693取代。2026-11-27用户明确授权提交、推送和提 PR，不包括合并或开启公网。公网监听继续关闭。

## 2026-11-26 · PUBLIC-2 只读分享

- **结论与状态**：通过（仅PUBLIC-2本地验收）。PUBLIC-1已通过本机与远端Linux/Windows CI；既有AUD/PUBLIC-5/6仍阻止公网发布；当前未开放公网。
- **需要你决定的事**：无新增决定。连续执行授权覆盖PUBLIC-2；公开提交与过期后重提依赖PUBLIC-3，本阶段不加入提交按钮。
- **做了什么**：新增独立匿名只读`PublicHandler`、任务报告白名单投影、诊断裁剪、7天过期判定、模板分享页与资源白名单；过期、缺失/清除、存储故障分别返回安全HTML说明。复用现有安全DOM报告renderer，不改本机/LAN handler。
- **优化手段与代价**：独立路由表与投影隔离写接口和完整日志；无新增依赖、表结构或监听器。代价是页面与JSON API各自处理错误表示，已用测试保持契约一致。
- **没做什么**：未接入`main`监听器、未开放公网；未实现PUBLIC-3提交/令牌/验证码、PUBLIC-4队列限额、PUBLIC-5沙箱或PUBLIC-6上线。
- **告警与代码错误**：ISSUE-020（S2，页面表示缺陷）已按RED-GREEN修复；404/503页面不加载数据脚本、不泄漏存储详情。旧PUBLIC-1 Windows失败已由1c2c5ef及运行45777149693取代，本轮保留历史证据但不计为当前失败。
- **验收结果**：PUBLIC-2六组检查PASS、FAIL 0：匿名读取与路由隔离；字段白名单/诊断裁剪；运行态与7天TTL；过期/缺失/存储故障页面；CSP/缓存/转义/静态资源；本机/LAN回归。`go test -p=1 -count=1 ./...`、`go vet ./...`、`go build ./...`及`git diff --check`均退出0。
- **阶段度量**：PUBLIC-2新增9个实现/测试/资产/设计计划文件，合计+808/-0行；优化记录+10行（不含本报告和既有PUBLIC-1改动）。ISSUE-020修复1轮，新增阻塞0。明细见PUBLIC-2设计与计划文档。
- **下一步**：按清单进入PUBLIC-3设计；既有AUD-001/002/003及PUBLIC-5/6仍为发布阻断。

## 2026-11-27 · PUBLIC-3 公开提交与权限

- **结论与状态**：本地阶段验收通过；正在检查远端 PR/CI 后推送更新。独立公开 handler 尚未接入运行 listener，公网仍关闭。
- **需要你决定的事**：无。用户明确要求提交、推送并提 PR；该授权不包括合并 PR、配置生产 CAPTCHA 凭据或开放公网。
- **做了什么**：公开预览先验证 CAPTCHA；公开确认固定预览 commit，首次创建返回独立控制令牌；SQLite 同事务持久化任务、outbox、确认映射及令牌 SHA-256；Bearer 令牌用于远程取消/删除；任务持久化执行身份，worker 校验身份；清理失败保留任务并返回 `cleanup_pending`。
- **优化手段与代价**：哈希比较使用常量时间路径，控制令牌不进入公开 task projection；事务故障注入证明确认完整回滚。CAPTCHA 仅提供 verifier 接口，未配置时 fail closed，供应商凭据未接入。
- **没做什么**：未实现 PUBLIC-4 队列/限流、PUBLIC-5 专用沙箱重评审或 PUBLIC-6 TLS/封禁/告警/恢复上线门槛；未改变默认 loopback listener。
- **告警与代码错误**：复核发现并修复空 repo/commit 的 worker 全局身份误判、公开确认任务/token hash 非原子写入；独立审查另发现 ISSUE-026（S2，并发取消 waiter 会卡住），已按红测修复为广播式共享结果。首轮 PR Linux CI 又暴露普通进程退出错误被误传播为旧删除 API 500，现仅传播 `cleanup_pending` 并补红绿回归；本地已定向验证，Linux 专属场景等待 CI 复验。本轮不宣称完成 AUD-002。
- **验收结果**：AC-301~307 本地功能测试通过；当前修复后的全量测试、`vet`、构建须重新运行。首个 PR 头 `1eb9a19` 的远端 Windows CI 通过、Linux CI 失败于进程组删除回归；修复推送后须以新 CI 结论更新本报告。
- **阶段度量**：本次 PR 暂存 30 个文件，`+1895/-31` 行（含 PUBLIC-2/3 实现、测试和阶段文档）；跟进复核发现三个需要补闭的实现缺口，均已补回归并通过针对性测试；阻塞 0。并发 race 工具因 `CGO_ENABLED=0` 未运行。
- **下一步**：重跑修复后的全仓验证，提交并推送旧删除 API 错误传播修复，等待 PR #1 新 Linux/Windows checks；保持公网 listener 关闭且不合并。

## 2026-11-26 · PUBLIC-1 Windows 修复复验

- **结论与状态**：通过。修复提交`1c2c5ef`已推送；Go 1.23.12 Windows本机全量复验及远端Linux/Windows CI运行`45777149693`均通过。PR #1已创建，当前状态查询暂不可用；公网未开放。
- **失败证据**：GitHub Actions运行`45624252912`，提交`929adaa97e138252997e6c425834efd508a826f4`，Windows job失败、Linux job通过。Windows runner上仓库源码位置全部变为`nil`，包含常规相对路径、panic与build失败位置；短路径/大小写别名经`EvalSymlinks`规范化后与原路径比较不相等是触发点。
- **修复**：`indexReportSource`先对根目录每级祖先做`Lstat`，拒绝符号链接/异常重解析目录，再将Windows路径规范化到`EvalSymlinks`结果后索引，保留非Windows下的严格根路径比较。只涉及报告源码映射与针对性测试。
- **本机证据**：Go `1.23.12 windows/amd64` 下`go test -p=1 -count=1 ./...`、`go vet ./...`、`go build ./...`均退出0；短路径、大小写别名、junction拒绝专项通过。符号链接祖先测试因当前Windows账号缺少创建权限而SKIP；`git diff --check`通过。
- **远端验收**：GitHub Actions运行`45777149693`针对提交`1c2c5eff09237056c37020c3ecab1836e9a6ae6c`，Linux与Windows jobs均成功。符号链接祖先专项因当前Windows账号缺少创建权限而SKIP，不影响Windows alias/junction专项和全仓CI结果；公网监听仍关闭。

## 2026-11-25 · 审查与范围变更

旧首版方向基本正确，但不是公网产品。164项旧回归/实际安装与七天health观察只能证明所覆盖的旧路径，不能证明执行器关闭实例的完整测试业务已具备生产条件。本次独立审查核实三条P1故障及报告/诊断/任务重开等缺口，详见publicization-review.md。

用户提供7页《RepoTest 公网化改造清单》后，目标变为陌生访客提交公开仓库并分享结构化报告。采用清单报告→分享→权限→限流队列→沙箱→上线顺序。并发起步1；不引入账号、Redis或多语言。第6/7节门槛未全部通过时保持内网。

原运行实例仍是10a0bc5对应的预览服务，七天观察属于旧版本证据，不借用旧状态标记公网阶段通过。报告第一阶段的源码、回归和验收分别记录；真实输出、真实仓库和故障注入分开标注。

## PUBLIC-1 当前实施检查点

后端用例/子测试、叶子摘要、包级覆盖率、仓库内源码位置、输出与元数据预算、依赖诊断保留已实现。独立spec发现的test-build变体归属、no-selected-tests覆盖率、Windows安全相对路径已修复并复审PASS；独立quality发现的提前TestMain崩溃误报覆盖率已修复，规格与质量复审均PASS。管理员DOM报告、受保护日志入口及真实状态筛选已实现；深层子测试手机溢出修复的规格和质量复查均PASS。

已捕获24份真正由Go1.26.3工具生成的owned_fixture输出，源码/raw哈希、独立coverprofile统计与有界缺终态检查通过；已接入解析器并全部PASS。包括2,724,636字节完整大输出；原始字节不改写，消费端截断另行标记。语料保存在internal/runner/testdata/corpus-capture与corpus-source。这些受控fixture不计为10个真实GitHub仓库。另保留独立审查者实际产生的TestMain提前崩溃捕获，raw SHA256为1d7f5b54da0b1b2f8cd07c0116892cf0a1eca46fc4d0c532afd88a5341ed4be6。

10个公开仓库的固定commit/根go.mod/体积元数据已核验，清单internal/runner/testdata/public-repositories.json；10份真实报告均已生成并通过重放及独立spec/quality核对。UUID缺少显式Go指令是合法默认，未伪造版本或更换仓库。首轮SSH被远端关闭，仅留下两个完成日志，保留为未完成尝试；第二轮在私有临时后台目录按任务并发1逐个执行。testify首轮依赖上游502未计为通过，仅补跑该固定提交；失败尝试另存，不覆盖原证据。镜像通过InspectGoImage实际确认Go1.27.1。

真实仓库测试结果为8个退出0、2个退出1；31个包，叶子用例通过2226、失败13、跳过3。gjson的TestJSONString失败位于固定提交gjson_test.go:2560；testify保留12个失败叶子。报告验证通过意味着这些结果被正确表示，不表示第三方仓库测试全部成功。所有源位置均以固定GitHub文件API响应、Git blob SHA1、源码SHA256和14条行号独立核验。保存位置为internal/runner/testdata/public-corpus；raw/report哈希、选择记录、原502尝试和源码核验一起保留，标准runner测试无需网络即可重放。

### 第一阶段检查记录

- PR-AC-01/02/03/04：PASS。用例/终态、包级覆盖率、源码映射、输出边界及依赖诊断有实际回归；24份捕获全部PASS。
- PR-AC-05：PASS，独立spec和quality最终复审均通过。桌面1440、手机390下检查真实gjson/testify报告及明确标注的恶意输入、日志权限、预览/确认/取消/删除/设备流程；深层33个用例保留完整名称/状态，手机scrollWidth390、最窄明细264px。
- PR-AC-06：PASS。10个不同固定提交的原始执行报告、哈希、来源、叶子统计、包级终态/覆盖率和失败行号通过独立核对。
- 本地smoke：go test -json -p=1 -count=1 ./...退出0；go vet ./...退出0；go build ./...退出0。日志为work/public-report-s1/final-tests.jsonl、final-vet.log、final-build.log。
- Linux/container smoke：PASS，work/public-report-s1-linux-smoke-r1/result.json退出0；实际离线容器（含取消/资源清理）、当前报告与控制层、cmd测试及全局vet/build通过，结束时没有本次runner容器或网络。未启动或更改被观察的实际服务。
- Git捕获字节检查：162个testdata文件暂存内容与工作文件逐字节相同。gitattributes禁用捕获数据的换行转换并明确允许CRLF行尾；没有改写raw来满足格式检查。证据work/public-report-s1/git-index-artifact-check.json。

### 修复与度量

返工7轮：后端真实build/no-selected-tests/Windows相对路径；超时黄金参考；提前TestMain覆盖率；UI实际持久化状态/取消日志/规范化URL；语料独立位置与缺终态核验；手机深层用例；Git保留CRLF后的检查属性。首次TDD RED不算返工。阻塞事件2次：SSH中断、依赖上游502，均已解除；旧公网发布门槛和待补部署事实另列，不计为本阶段新阻塞。

diff实测（10a0bc5到本阶段暂存，git diff --cached --numstat）：实现16文件，+898/-86；测试8文件，+1432/-0；文档含哈希清单8文件，+375/-0；捕获/来源162文件，+30958/-0，单列不冒充实现代码。排除ignored work日志、截图、源码包与二进制；详见work/public-report-s1/diff-metrics.json与diff-numstat.txt。

当前汇总：本地与受控Linux的6个PR-AC均PASS，独立后端、UI、语料spec→quality闭合；复审记录见work/public-report-s1/*-review.md。旧运行45624252912在提交929adaa上Linux PASS、Windows FAIL；修复提交1c2c5ef对应运行45777149693的Linux/Windows jobs均PASS。未把13个第三方测试失败误写为本项目验收失败或通过的第三方测试。

检查点与恢复：基线10a0bc506ca88eb4a319845e3224a3d11a74e0d2；功能分支feat/public-report-s1。docs/evidence/public-report-s1/source.sha256记录186个变化的代码/测试/捕获Git暂存字节SHA256，清单SHA256为f50a921a478420ee1724a8a1fc04e79ddb1c37c9c9b929ab003e33777fa0a68b。需要恢复时使用基线独立检出；本阶段没有部署或迁移现有运行服务，不需回滚生产数据。恢复方案已记录，未执行回滚。

历史记录（截至2026-11-25）：当时未做PUBLIC-2只读分享/CSP/OpenGraph；2026-11-26已完成PUBLIC-2本地验收，但未开放公网。过期重提、PUBLIC-3/4权限与队列、AUD-001/002/003修复、专用沙箱和生产开放仍未完成。没有新增第三方依赖或修改生产监听/runner开关。

### 后端检查与修复证据

- 规格复审PASS：真实build_failed归属一次、无选中测试no_tests/null、Windows相对位置；末轮提前TestMain崩溃保留诊断/null覆盖率，正常断言失败仍100%，执行但未覆盖仍0%。
- 质量复审PASS：work/public-report-s1/backend-quality-review.md；24/24真实捕获回归、runner/domain测试、vet、原独立审查复现、gofmt。此结论仅覆盖后端，未替代界面、真实仓库和生产验收。
- 测试参考修正：超时捕获虽有coverprofile，但覆盖文本位于Test输出内，没有包级覆盖摘要；期望应为null，不能信任该文本来让测试通过。原raw与manifest保持不变。
- 可信度边界：覆盖率和用例是执行工具报告的数据，测试代码可打印或伪造运行时输出；定位只验证实际仓库文件与固定提交链接，并非测试代码真实性证明。提前执行/缺终态/截断必须显式保留，不能声称对任意恶意TestMain进行了真实性认证。
- **下一步**：按清单进入PUBLIC-3设计；既有AUD-001/002/003及PUBLIC-5/6仍为发布阻断.
- **下一步**：按清单进入PUBLIC-3设计；既有AUD-001/002/003及PUBLIC-5/6仍为发布阻断。
