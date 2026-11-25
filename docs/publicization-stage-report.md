# RepoTest 公网化阶段报告

模式：Full；沟通方式：连续执行。PUBLIC-1 报告层：通过；下一阶段：PUBLIC-2只读分享。
基线：10a0bc5；分支：feat/public-report-s1。用户已要求按公网化PDF进行，并确认起步并发1。
未关闭发布阻断：AUD-001执行身份错配、AUD-002完成写库后卡住、AUD-003取消清理失败；PUBLIC-5专用沙箱与PUBLIC-6全部上线门槛。
待补事实：域名、专用机器/云厂商/预算、后续验证码凭据；报告与本地只读分享实现不依赖这些。PUBLIC-1检查点已完成，进入PUBLIC-2；当前仍未开放公网。

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

验收汇总：6个PR-AC全部PASS，FAIL/BLOCKED/NOT RUN均为0。独立后端、UI、语料的spec→quality审查闭合；复审记录见work/public-report-s1/*-review.md。未把13个第三方测试失败误写为本项目验收失败或通过的第三方测试。

检查点与恢复：基线10a0bc506ca88eb4a319845e3224a3d11a74e0d2；功能分支feat/public-report-s1。docs/evidence/public-report-s1/source.sha256记录186个变化的代码/测试/捕获Git暂存字节SHA256，清单SHA256为f50a921a478420ee1724a8a1fc04e79ddb1c37c9c9b929ab003e33777fa0a68b。需要恢复时使用基线独立检出；本阶段没有部署或迁移现有运行服务，不需回滚生产数据。恢复方案已记录，未执行回滚。

未做：PUBLIC-2只读分享/CSP/OpenGraph/过期重提；PUBLIC-3/4权限、队列及AUD-001/002/003修复；专用沙箱和生产开放。没有新增第三方依赖或修改生产监听/runner开关。

### 后端检查与修复证据

- 规格复审PASS：真实build_failed归属一次、无选中测试no_tests/null、Windows相对位置；末轮提前TestMain崩溃保留诊断/null覆盖率，正常断言失败仍100%，执行但未覆盖仍0%。
- 质量复审PASS：work/public-report-s1/backend-quality-review.md；24/24真实捕获回归、runner/domain测试、vet、原独立审查复现、gofmt。此结论仅覆盖后端，未替代界面、真实仓库和生产验收。
- 测试参考修正：超时捕获虽有coverprofile，但覆盖文本位于Test输出内，没有包级覆盖摘要；期望应为null，不能信任该文本来让测试通过。原raw与manifest保持不变。
- 可信度边界：覆盖率和用例是执行工具报告的数据，测试代码可打印或伪造运行时输出；定位只验证实际仓库文件与固定提交链接，并非测试代码真实性证明。提前执行/缺终态/截断必须显式保留，不能声称对任意恶意TestMain进行了真实性认证。
