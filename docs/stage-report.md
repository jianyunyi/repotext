# RepoTest 阶段报告

当前工作已按用户公网化清单转入PUBLIC-1报告层，并发起步1；新阶段以[公网化阶段报告](publicization-stage-report.md)为准。以下保留原内网实例的实际部署与观察记录，其通过项不作为公网版本生产资格。

<!-- 状态块 -->
当前阶段：S5 部署与运维；状态：进行中；2026-11-25 r22实际安装/工作盘/恢复/监控及r23完整Linux回归通过，七天观察进行中
未关闭阻断项：ISSUE-010 七天覆盖未完成；ISSUE-025/017/013已关闭；延期风险：ISSUE-004；待决策：无，用户已批准具体部署并在中断后continue。下一步：2026-12-02 14:36:33 +08:00后检查完整样本/缺失/故障及退出回归；runner/LAN关闭。实际主机重启和平台生命周期持久性未测，不外推。

## 2026-11-25 · S5 实际安装、恢复与观察启动

### 结论与状态

实际部署、固定work容量、恢复演练和监控启动通过，S5整体进行中，七天尚未到。用户“批准”覆盖instabox-test-env上的具体新安装/2 GiB盘/恢复/监控；后续continue保留原授权。Full/连续执行；基线a7fe305加本轮开始的6个文档.snapshot，位置work/s5-install-20261125-r22/baseline。产品Go/前端字节未修改；新增7个部署配置取回目标实际字节并核对SHA，保存在deploy/及scripts/export-health.sh。独立s5_execution_review复核执行脚本及运行证据；s5_monitor_prepare单独实现监控并运行本机检查，最终复核记录见同目录。

### 需要你决定的事

无。安装/演练/健康观察授权已满足，未重复索取。七天观察窗口从2026-11-25T06:36:33Z开始，候选结束2026-12-02T06:36:33Z（北京时间两日均14:36:33）；达到时间不自动等于覆盖通过，须计算实际采样、缺失和故障。未启用runner/LAN或执行主机重启。

### 已完成、优化与代价、未完成

- 预检固定六产物SHA、root/systemd/docker、路径与端口；root私有副本二次哈希后安装两个二进制、默认环境、专用账户/home及unit，健康启动。main active、NRestarts0，home/work0700、DB/image/env0600。root私有证据/var/backups/repotest-deployment.bMqjzCah。
- 2 GiB镜像大小2147483648字节，实际分配2147487744；nodiscard格式化、nodev/nosuid/noexec。真实repotest用户请求写2415919104字节，写到2022703104时ENOSPC并只删除自己压力文件；有效容量小于镜像总大小。私有mount namespace卸载准确work后同一guard rc32，宿主挂载仍在；停自有mount后service依赖恢复挂载且健康。额外临时systemd fixture的ExecStartPre status32、主touch未执行并清理/reset自身单元，未使真实主服务启动失败。
- 停服无repotest进程，baseline备份，种子不可用synthetic session hash与原表计数；备份后fixture writer修改revoked1并无checkpoint退出，实际DB/WAL/SHM三件套全部保留到/var/lib/repotest/restore-before.AFxt6ttl；恢复验revoked0、清理种子、表计数原后全相等、完整性ok、属主/模式/preflight后健康启动。该新服务原计数全零，不虚构真实task/report数据恢复。
- 安装五分钟health timer、小时root私有JSON导出timer/helper。source限定health unit，失败仍记录state/df；导出flock防重叠、成功有效JSON后推进相邻窗口checkpoint，按__CURSOR去重，未公开主服务配对码日志。首条完整样本已保存，06:49:27Z最终核对3条HEALTH_PASS/0FAIL、2个archives/33个unique entries，LastTrigger06:45Z证明真实定时执行，主服务异常重启0。
- Windows test/vet/build/gofmt通过；完整Linux r23为164 PASS/0 SKIP/0 FAIL、stderr0/transfer_error null，包含S4资源/断网/硬quota/HTTP成功取消/最终清理与S5备份/health/preflight。

复用现有Go、SQLite、systemd、ext4与已装flock/Python，无新增第三方依赖。增加root盘约2 GiB预分配及4个observer units/helper维护成本；小时归档将原批准的手工日窗口自动化，降低采样被轮转前未保存的机会，未测量性能收益。所有数据库、源包、原始日志和归档保留在私有work/或目标备份目录，不入Git。七天覆盖、主机重启、平台生命周期持久性、默认关闭部署的runner执行稳定性未完成/未验；新持久work上的runner HTTP变体须在将来启用前验证。

### 告警与代码错误

本轮安装、恢复和回归未发现新增产品代码错误。install/monitor stderr只有systemd为本次unit创建启用链接的信息；实际unit verify为空日志。预期ENOSPC、guard32及拒绝端口失败均为通过的失败路径检查，不计产品FAIL。中断前安装和mount已完成，续接先只读确认健康/未执行restore，未重跑安装或覆写旧资源。ISSUE-025/017/013运行证据已关闭；ISSUE-010七天覆盖未关闭，ISSUE-004风险延期。

### 验收、证据和度量

当轮分组结果由work/s5-install-20261125-r22/verified-summary.json列出；安装/挂载/恢复/monitor/final各自保留脚本SHA、stdout/stderr、exit/remote_exit/transfer_error。Windows与Linux分别有fresh logs；r23源包在归一化前84文件逐字节核验，Go/S4/S5通过。新增7配置另以目标实际SHA核验，其中export脚本和4个Python片段通过语法、本机cursor去重、导出失败/invalid/busy时checkpoint保持检查，且真实systemd运行/首样本归档通过。既有Go未变；不把7天未来结果或静态阅读计为运行PASS。

14 PASS、0 FAIL、0 BLOCKED、1 NOT RUN（七天覆盖）。实现7文件、测试0文件、文档6文件；增删行与基线/终点SHA按真实diff写在diff-stat.json，包含新增配置，不含work检查片段、日志、二进制或缓存。实现返工0轮，文档返工1轮：独立复核修正README/deployment/plan旧状态；脚本首次成功，无产品修复；额外fixture门禁验证是补齐同项集成证据，不删除或放松标准。阻塞0次：沿用已存SSH权限规则，用户主动中断后从真实检查点恢复，不作为授权/环境阻塞。

检查点保留用户指定GitHub上的版本、代码/文档SHA、每项实际日志和root恢复集；回退只停止本次health/export observers、RepoTest和自有mount并保留全部数据，不自动删账户/盘/备份。恢复失败按已保存完整三件套回退后再验integrity/ownership/preflight，实际本轮未发生失败回退。

### 沟通与后续

当前三文档和README统一为已批准、已安装/恢复/监控、七天进行中；问题权威字段在optimization-design.md总表，准备阶段的待批准/NOT RUN是下方历史。新operator配置已按目标字节保存，源码/文档与部署入口对应，产品默认关闭边界一致。持续观察只记录default-off服务；七天窗口结束后需重算真实样本/缺失/故障并重跑退出smoke再决定S5是否通过。

## 历史：2026-11-25 · S5 目标部署准备与方案复核

### 结论与状态

部署准备已实现并在实际目标验证；S5整体待沟通，未实际安装。Git基线a7fe305a33fe554b5975f2c86e62e440fecf1faa，已按用户请求推送Github；本轮仅文档差异，工作区文件哈希及度量保存到work/s5-deploy-preparation-20261125-r20。Full/连续执行；独立s5_deployment_review检查实际r20日志、unit/env/备份源码和最终具体计划。完整Linux累积回归仍引用r17历史证据，本轮新增r21 S5夹具不代替实际服务验收。

### 需要你决定的事

批准在现有instabox-test-env上按superpowers/plans/2026-11-25-s5-target-deployment.md进行首次安装：创建专用账户/目录，安装核验二进制，建立2 GiB预分配ext4工作镜像及mount依赖，启动loopback/default-off服务，执行只触及该新服务数据库的备份恢复演练，启用五分钟health timer开始观察。当前约4.04 GiB根盘可用；镜像预分配后约余2 GiB。该容量是基于S4实测的推荐方案，用户尚未批准。无需重新批准既有SSH读取/临时构建。目标卷在托管平台的生命周期持久性须向平台设置核对；未从df推断永久保留。

最终额外只读核对：/data/home位于独立/data盘(/dev/vdb)，可用16,509,224 KiB约15.7 GiB；/var/lib位于根盘(/dev/vda)。完整Linux smoke的第二个2 GiB临时quota镜像不争用部署剩余根盘空间，执行前仍须重查。新增probe/mount/drop-in/health unit路径不存在；该事实解决独立复核提出的容量关联疑问，不等于实际持久挂载验收。

### 做了什么、优化与代价、未完成

执行r20准备、核验84文件传输与六产物哈希；目标systemd running、sudo有效UID0、无既有repotest账户/服务/部署路径，8080无监听。准备产物位于/tmp/repotest-s5-prepared.oKH4ntJ9，计划锚定日志中的精确SHA。重跑Windows test/vet/build与目标r21 S5备份/health/路径门禁。更新设计/问题总表/运维入口及Git现状，修正旧顶层阶段状态。

复用现有Go/SQLite、probe、systemd和已装挂载工具；只补具体安装计划，无新依赖或产品逻辑。预期收益为确保工作容量、挂载失效保护和恢复证据，未测量性能提升；成本是2 GiB预分配与新mount/health units维护。实际安装、持久mount/失效验证、恢复和七天观察NOT RUN；未启用runner/LAN。新持久work盘不能沿用临时盘的AC-008容量结论，runner启用前需重跑该盘压力与HTTP回归。

### 告警与代码错误

- ISSUE-025，S1部署验收缺口：通用安装只mkdir work。独立复核提出mount依赖/失效门禁；二次审阅发现mkfs默认discard可能使已分配镜像打孔，计划改-E nodiscard并检查blocks。方案已准备，实际验收前不关闭。
- ISSUE-010实际部署/恢复/观察缺口和ISSUE-004共享内核/rootful风险仍保留。本轮Go及S5夹具检查未发现新增产品代码错误。
- r19未提权SSH配置拒绝，随后r20提权解除。附加只读探测先遭Windows参数转义错误，再遭stdin BOM/CRLF异常，结果不作为完整PASS；保留target-facts.log和target-facts-verified.stderr.log为历史，后用简单无复杂引号SSH命令成功核实UID0、工具路径和磁盘，最终事实汇总记录其实际stdout。未修改SSH配置或用替代凭据绕过管理限制。

### 验收与检查证据

当轮结果以work/s5-deploy-preparation-20261125-r20/verified-summary.json为准。检查对象：a7fe305产品源码与工作区文档；r20/r21均在归一化CRLF前验证84源文件。准备（target/artifact）、Windows tests/vet/build、Linux S5 backup/health/preflight/static-unit、计划shell语法、独立复核和文档一致性分别列当前结果。实际install、persistent-mount failure、restore、seven-day observation均NOT RUN。历史探测失败不算当前产品测试FAIL，仍保留原证据。最小逻辑证据映射：备份独占/竞态/完整性→AC-009/r21六Go测试；health200/201/userinfo/hostname→AC-009/r21四场景；链接/权限/端口/default-off→AC-009/r21预检场景；mount/start/restore新增方案→ISSUE-025/010，目前只有准备和审查、运行NOT RUN。Windows和Linux变体分别报告，静态unit不证明服务安装。

最终计数为12 PASS、0 FAIL、0 BLOCKED、4 NOT RUN（包括实际运行、哈希/语法及独立文档复核分组，复核PASS不作为安装PASS）。度量：6个文档文件、+277/-11行；实现/测试文件0，返工2轮，阻塞事件1次且已解除。来源为实际git diff与新增计划diff；独立代理最终另行重算84源哈希、核对六产物锚定及状态块，未发现剩余须修正项。平台卷生命周期持久性未核实，保留为安装前事实项。

### 阶段度量、检查点与后续

真实git diff及新增计划逐行计数分类保存到work/s5-deploy-preparation-20261125-r20/diff-stat.json，完整patch和终点文件SHA保存同目录；基线为a7fe305，终点为工作区文件SHA。本轮不改实现或测试文件；文档返工2轮（第一轮补nodiscard/已有probe拒覆盖及timer范围，第二轮补相邻完整日志导出窗口与成功后推进checkpoint）；补充探测的转义/编码输送修正单列操作尝试，未改产品。r19沙箱权限为一个阻塞事件，r20解除；具体部署等待批准是现有ISSUE-010授权关卡，未新算技术阻塞。排除work内归档、二进制、日志、缓存和检查副本。

全文一致性：当前三文档状态统一为r20准备通过、实际安装待批准，ISSUE-010/025未关闭、ISSUE-004延期；Git现状与S3/r17退出状态已更新。2026-11-24及更早SSH拒绝/准备未运行保留为历史，由本节取代；不再据旧记录重复索要读取授权。恢复文档差异仅取回本轮精确文件，未执行目标服务回退。取得具体授权后按计划安装，成功安装/演练后才能开始真实七天观察。

## 历史：2026-11-24 · Linux r17通过与部署准备

用户在自己的PowerShell运行完整入口，r17于22:39 +08:00完成，exit_code=remote_exit_code=0、transfer_error=null、stderr0字节。主代理重新核对83文件manifest、tar及当前字节与目标逐项OK，并核对全套标记；Linux全量164 PASS/0 SKIP/0 FAIL，S4 CPU/PID/内存/只读/断网、硬quota、HTTP成功与取消、最终容器/网络清理均PASS；S5备份、health四场景及私有目录/链接/端口fixture全部PASS。目标证据 work/s5-linux-20261124-r17/verified-summary.json；原r16配置拒绝只作历史，不再将Linux回归标为阻塞。

已验证关闭ISSUE-021、011、012、014–016、019的目标回归欠账；具体权威状态见optimization-design.md问题总表。实际systemd安装、服务恢复和七天观察仍NOT RUN，ISSUE-010未关闭，ISSUE-004延期。没有新代码错误；本轮发现范围为83精确源文件、完整目标日志及准备脚本静态检查，未重新运行Chrome。

后续按既有deployment.md方案，将临时二进制构建和必要目标事实收集封装为scripts/prepare-s5-deployment.sh；仅读取PID1/systemd状态、UID/sudo探测与允许规则、账户/组、已有路径和8080监听，保存两个Go二进制及原unit/env/probes到私有mktemp目录并输出SHA。该脚本不安装、改账户、启动服务或触碰实际数据库；S5_PREPARATION_PASS仅指事实/产物准备成功，不表示systemd可用或安装通过。语法和源包可以本机核对，实际准备运行保持NOT RUN；先获得目标事实，再完成可审查的安装/升级与恢复方案。当前代理内读取SSH配置仍受管拒绝，用户授权已明确；外部PowerShell入口是有效的协作路径，不重复索取读取授权。

模式Full/连续执行；本次95文件基线和.snapshot在work/s5-deployment-preparation-20261124，阶段度量与状态见其中summary.json及diff-stat.json。排除work临时检查/快照/日志、二进制、缓存和数据库。独立复核发现ISSUE-024（S2 sudo权限事实误标），原脚本对仅允许true的sudo政策输出INSTALL_PRIVILEGE；已改为SUDO_PROBE、实际sudo -n -l规则及INSTALL_COMMAND_PERMISSIONS=UNVERIFIED。sudo-red.log三FAIL、sudo-green.log三PASS；修复后八shell syntax与PowerShell AST通过，84文件r18准备包重生成。独立merge_driver_review另行运行三场景/语法并重算包哈希，未发现新增重要问题，ISSUE-024已关闭；其未运行SSH或实际安装。当前阶段检查7 PASS/4 NOT RUN，不把准备和静态检查外推为已部署。此次“修改→复验”计返工1轮；既有代理SSH阻塞尚未解除，本次不新增阻塞事件。当前变更只有准备脚本和文档，不修改已验Go/前端/沙箱逻辑；回退仅恢复本阶段快照中的改动文件并移除新增prepare脚本，不执行服务回退。目标验收修复关闭不等于整个P5通过。全文旧状态在各历史段保留并明确由r17取代。

## 历史：2026-11-24 · 原目录合并、本机回归与 Linux r16

### 结论与状态

原目录合并和本机回归通过；S5 仍 BLOCKED。旧 ZIP SHA-256 为 e822f8717774640dd50d606fbe18919b0fed844fc2a43507abbcd384b8a84030，86 基线文件与全部24改动核验通过，8新增目标无冲突；保存86快照后复制精确字节，合并后94文件校验通过。随后修复驱动传输异常的诊断。独立复核 merge_driver_review 只读重算归档/清单、检查源码和现有 RED/GREEN/r16 日志，未发现高、中严重问题；该复核未重跑会写临时文件的测试或运行SSH。

### 需要你决定的事

没有新的产品取舍或合并许可需求。当前会话读 C:/Users/asus/.ssh/config 被拒，阻止目标回归；需恢复既有访问能力后继续。实际部署、恢复与观察仍按 deployment.md 在目标验收后准备并单独授权。本次“继续 Linux 验收”不豁免目标检查。

### 已完成、优化与代价

完成 AC-009 合并前核验、快照、精确合并、本机 Go/health/脚本检查，补 ISSUE-022 三条本地子进程回归。驱动捕获 CopyTo 异常并保存真正 stdout/stderr/退出码和 transfer_error，提前exit0且传输失败仍exit1；复用原流程和Python标准库，无新依赖，没有测量性能收益。ISSUE-023 修正检查副本路径，避免混入 Go 测试范围。交付和恢复依据 work/merge-20261124/README-HANDOFF.md、summary.json、diff-stat.json、source-current.sha256；历史旧包保留在 _package。

### 未完成与未做事项

目标Linux尚未执行；S4隔离/硬quota/HTTP取消清理与S5完整Linux fixture保持 BLOCKED。systemd实际安装、服务备份恢复演练、7天观察 NOT RUN。未变更SSH配置/密钥/安全选项，未部署、启用runner或LAN；本轮未重跑Chrome，UI/后端字节与已验旧包一致，旧浏览器证据仅作历史引用。

### 告警与代码错误

- ISSUE-022，S2验证工具缺陷：r15旧驱动在pipe ended后缺失败日志；三场景RED/GREEN及r16真实失败记录证明修复，已关闭。
- ISSUE-023，S2检查材料位置错误：本轮首轮Go/vet/build错误仅来自解包副本被扫描；改为_package并复跑通过，已关闭。保留历史首轮日志，未删测试或改产品逻辑。
- 环境限制：r16 exit255，stderr为SSH配置 Permission denied；同路径Python只读打开也PermissionError，WSL枚举E_ACCESSDENIED。本会话不具备所需访问能力；一个尚未解除的SSH阻塞事件。
- ISSUE-021完整Linux验证和ISSUE-010实际部署欠账仍未关闭；ISSUE-004共享内核/rootful共宿主风险延期。本轮适用代码检查未发现新增未关闭错误。

### 验收与证据

- AC-009 合并：preflight-result.json、merge-result.json，86基线/24改动/8新增/94合并后字节 PASS；未改变原冻结标准。
- smoke AC-001–009 的本机适用 Go 检查：go test -p=1 -count=1 -json ./...，go-test-current.jsonl，156 PASS/6 SKIP/0 FAIL；4测试包PASS，1无测试包。六个SKIP涉及Linux权限/锁/Docker等，仍是SKIP。
- Go vet/build、gofmt：build-current-result.json与gofmt-result.json全部exit0；go-packages.log恰5包，恢复材料未增加发现范围。
- AC-009 health四场景：HTTP200成功、HTTP201失败、userinfo与hostname拒绝且不发送请求，health.log全部PASS；七脚本syntax与PowerShell AST均通过。
- ISSUE-022：driver-red.log保留失败基线，driver-green.log三场景PASS，包括成功传输tar SHA和提前exit0失败；这是Windows驱动逻辑验证，不是目标回归。
- 目标r16：result.json/stderr.log为exit255与配置拒绝，BLOCKED；83源文件tar/manifest集合和精确字节另核验PASS，source包验证不替代运行。
- 当前检查组12 PASS、1 BLOCKED、3 NOT RUN；逐项状态由summary.json记录，不将历史首轮FAIL计入当前结果，也不将静态审查计为运行PASS。

### 阶段度量与恢复

基线为before的86快照、终点为source-current.sha256；快照文本差异共25文件：实现10文件+194/-26、测试7文件+531/-0、文档8文件+523/-24，详见diff-stat.json与diff-summary.json，包含新增源码和更新文档。首次24文件合并不算返工；因ISSUE-022失败补修驱动和ISSUE-023材料扫描FAIL后修正各计1轮，共2轮；SSH持续阻塞计1事件，原目录写入阻塞已解除。排除work恢复材料/证据、缓存、二进制、数据库；本阶段没有Git仓库，使用哈希和真实快照文本差异，无commit/push。

完整回退只恢复清单对应86快照并移除8包内新增及本轮新增回归文件；步骤见本轮交接。材料不等于已演练服务恢复。全文一致性已核对当前目录、合并事实、问题权威状态、证据与下一步；下方旧“原目录未改/C:维护”等文字均为合并前历史，由本条取代。


## 历史：2026-11-24 · 合并前进度复核、部署预检修复与统一交付

### 结论与状态

完成原有方案 Task3 的中断交付，持久副本位于 C:/Users/asus/source/repotest-20261124。S1/S2 及 S3/S4 历史证据保留；S3 用户流程已通过本轮 Windows/Chrome 检查，S5 仍缺目标新修复回归与实际安装/恢复/观察。原目录 86 文件基线一致，未写入；不将修复包等同于已合并或已部署。

### 需要你决定的事

实现没有新产品取舍。原目录 F:/repotext 不在本会话可写范围，既有 SSH 命令在打开配置时返回 Permission denied；需恢复这两个环境访问条件后继续。实际部署/恢复仍按 deployment.md 在目标回归后另行确认。现有 continue 授权用于继续修复与验证，未扩大部署对象或范围。

### 做了什么、优化手段与代价、没做什么

恢复旧副本全部源码和有效历史证据，重新核对任务及验收基线；重跑 Go、健康探针和 Chrome 交互，完成精确改动文件、ZIP、原始/改动哈希、补丁和合并/回退说明。ISSUE-020 的旧统一包未生成事实已记录，没有把旧报告中的预期产物当证据。

独立复核发现 ISSUE-021：部署预检漏悬空数据库链接、祖先/尾斜杠链接，端口 glob 接受非法输入。补逐路径组件链接检查、0–65535 numeric 端口，并为现有 Linux S5 fixture 增加十条拒绝路径及一条兼容路径。复用 shell/stat/awk 和已有测试，无依赖、数据库表或新服务；代价是部署预检多做祖先 lstat 和维护这些边界检查，未测量性能收益。

未合并原目录、未重跑完整目标 Linux、未安装/启动 systemd、未恢复实际服务、未开始 7 天观察。LAN/runner 实际部署默认关闭。Chrome 检查仅在 loopback 上运行明确标注的合成测试夹具，没有执行仓库代码。

### 告警、代码错误与独立复核

- ISSUE-020（S2 交付缺口）：统一包与最终汇总旧路径不存在；本轮生成并以真实基线、补丁、ZIP CRC/路径/字节哈希检查关闭。
- ISSUE-021（路径 S1、端口 S2）：已修改并通过本机适用边界；完整 Linux fixture、悬空文件链接仍待目标证据，验收未关闭。详见 optimization-design.md、work/continuation-20261124/preflight-review.md 和 delivery-checks.json。
- ISSUE-011–019：恢复副本中已有修复并完成本轮适用检查；目标相关检查仍待验，不撤销原 S4 历史证据。
- ISSUE-004：rootful Docker 与其他应用共宿主、共享 Linux 内核的延期风险保留。
- 本轮在所审查 S3 会话/LAN/UI 和 S5 交付/预检变更范围未发现剩余待改 S0/S1；这不代表全项目或目标部署没有缺陷。

readiness_review 独立只读复核 S3/S5 并发现预检问题，随后实施两脚本，因此不是该修复的独立审查者。主代理未参与脚本实施，独立核对补丁与真实重跑边界；目标运行、宿主与长期观察仍未独立验收。打包由 finish_package 实施，主代理核对源码集合、哈希和实际补丁/ZIP结果。

### 当前验收结果与证据

分组 PASS 14、FAIL 0、BLOCKED 2、NOT RUN 3，详见 work/continuation-20261124/summary.json。分组不与单个 Go 测试或历史检查混算。

- Go 全套：156 单项 PASS、6 SKIP、0 FAIL，4 个有测试包 PASS；另一个 domain 包无测试。命令 go test -p=1 -count=1 -json ./...，日志 go-test.jsonl、go-counts.json；vet/build exit 0，gofmt 无输出。
- 健康 HTTP 四场景 PASS；七脚本逐一语法与 PowerShell AST PASS。日志 health.log、shell-syntax.log、powershell-ast.json。语法检查不是 Linux 权限检查。
- Chrome 八组 PASS：配对/错误码、只读与转义、指定撤销后 401、实际 queued 删除、running 409 保留与重读、迟到 poll/report/cancel、无 JS 异常。browser.log、browser-result.json；已查看 390px LAN 与桌面设备管理截图。
- 端口隔离代码块八场景 PASS：旧块接受五个非法值，新块均 exit 1，三个合法值 exit 0；两个真实 Windows junction 的完整预检按 symlink 原因拒绝。delivery-checks.json 说明边界，不外推为 Linux 文件系统 PASS。
- PrepareOnly 源包的 82 文件清单、归档集合与精确字节/哈希 PASS；先核对字节再 CRLF 转换的顺序已检查。验收包在 acceptance-package；统一修复 ZIP/原目录 86 基线/补丁各自 PASS，结果在 package-result.json。
- BLOCKED 2：原目录合并、目标完整新回归。后者包括完整 S5 新 fixture、S4 Docker/HTTP/quota/取消清理及 Linux 专属检查；6 个 Windows SKIP 不补算 PASS。
- NOT RUN 3：实际 systemd 安装、实际服务恢复演练、7 天观察。WSL 也被访问限制拒绝，不能充当既定 Linux 目标。

### 阶段度量、检查点与恢复

本轮基线为会话开始 94 文件的 SHA-256/.snapshot；累计基线是原 F 86 文件清单。本轮实测 8 文件 +151/-18（实现 1 文件 +22/-3，测试 1 文件 +16/-0，文档 6 文件 +113/-15）；累计 24 文件 +1110/-48。由真实文本 SequenceMatcher 计算，取数与分类在 work/delivery-progress-20261124/session-diff-stat.json、session-diff-summary.json、diff-stat.json、diff-summary.json；不把旧修复规模算成本轮实施。排除 work 生成证据、缓存、二进制、数据库、截图与 ZIP。

本轮返工 0：首次补齐及首次修复不计返工，无因失败复验再修改的轮次；历史阶段度量仍保留原口径。持续阻塞事件 2（原目录可写范围、SSH 配置访问），均未解除，不因多次汇报重复计数。

检查点以 changed-files.sha256 固定精确变更文件，archive.sha256 是 ZIP 外部哈希，源包另有 source.sha256。恢复使用 before/*.snapshot；未来合并先核对全基线、检查新目标不存在，再备份所有被替换文件并复制清单列出的文件。复制保留验证过的精确字节；补丁应用使用 --ignore-whitespace 处理原 CRLF。原目录未改，尚无原目录回退动作；材料不代替实际服务恢复演练。

全文一致性：三文档当前状态块、README、任务勾选及部署验收边界已同步。旧 2026-11-23 记录改为历史，并明确当时统一包未生成；当前路径和结果以本节为准。

## 历史 · 2026-11-23 · 项目进度重新审查与后续完善

以下保留当时已完成代码/检查的记录。其引用的 final-summary.json 和 work/delivery-progress-20261123 当时未生成，相关统一包验证不能据此认定完成；由 2026-11-24 当前交付与检查记录取代。

### 结论与状态

已按用户要求重新审查 S1–S5，补齐本轮六项实际缺口。S1/S2 历史验收、S3 的 TTL/去重/worker 及 S4 隔离历史证据保留；S3 的完整用户流程不能仅由旧接口测试替代，本轮已补齐并通过副本 Windows/Chrome 检查。S5 仍阻塞于原目录合并、目标 Linux 新代码及运维验收，没有创建新 S6 来绕过缺口。

当前工作在系统临时目录中的续接副本，原 F:/repotext 未修改。本报告顶部与本节是当前结论；同日早先续接记录及此前检查点均为历史。

### 需要用户决定的事

当前实现没有新的技术取舍需要确认。原目录写入和既有 SSH 访问仍依赖环境恢复；用户授权不改变当前受管会话的访问边界。具体部署/服务启动/实际恢复仍需单独明确授权，应在目标新修复回归完成后依 deployment.md 推进。7 天观察仍未开始。

### 做了什么、优化手段与代价、没做什么

新增本机活动设备列表与逐会话撤销；新增 LAN 配对、状态、报告和日志只读页面；运行态也能触发现有删除 API，并使用视图版本/计时器处理迟到 poll/report/cancel 响应。复用 SQLite 会话哈希、loopbackOnly 与嵌入式 HTML，不增加表、依赖或前端构建链。代价是一个本机会话查询/撤销入口及两页内的状态处理，未声称性能提升。

S5 补齐两份二进制、专用账户 home/属主、unit/env/reload 安装说明；CI 改为逐脚本语法检查；新增仅用临时 fixture 的 S5 运维验证入口并接入完整 smoke。PrepareOnly 生成包而不连接 SSH，传输清单覆盖 Go、HTML、scripts、deploy、mod/sum，并在目标转换 CRLF 前核对字节哈希。

未合并原目录、未执行新目标 Linux 回归、未安装或启动 systemd、未做实际服务恢复或 7 天观察。实际 LAN/runner 仍未开启；浏览器检查中的两个监听只绑定 127.0.0.1，所有任务和输出是标明用途的测试夹具，没有执行仓库代码。

### 告警与代码错误

- ISSUE-014（S1，AC-004）：撤销仅有内部方法，真实旧接口 404；本机管理入口及指定撤销检查已补齐。
- ISSUE-015（S1，AC-002/003）：LAN 首页真实 404，浏览器配对/只读流程缺失；新页面、401/405/Host 及转义检查通过。
- ISSUE-016（S1，AC-005）：运行态未显示删除按钮；现在调用现有接口，真实排队删除、运行态 409 保留/重读和三类迟到响应均通过浏览器检查。实际 Linux 活进程回收仍需新 smoke。
- ISSUE-017（S1 文档）：安装缺备份程序、属主、unit/reload；说明已补齐并独立复核，实际安装/恢复仍 NOT RUN。
- ISSUE-018（S2 验证）：CI 多路径 sh -n 漏后续文件；旧测试 exit 0，新逐脚本命令拒绝第二个坏脚本 exit 1。
- ISSUE-019（S2 验证）：完整目标入口未含 S5，哈希未覆盖被验脚本与部署文件；新入口和全文件清单已补齐，目标权限 fixture 仍待验。
- ISSUE-004 rootful Docker 共宿主/共享内核风险继续延期；Windows 五个既有 Go 检查 SKIP 不补算 Linux PASS。

S5 规格由 continuation_review 复核，主代理核对代码与包；S3 规格同代理复核，安全与代码质量由未参与 S3 实现的 s5_readiness_review 独立复核。变更范围内未发现剩余 S0/S1 或必须修复的 S2，静态结论不代表目标机或长期稳定性。

### 验收结果与证据

本轮运行证据在 work/progress-repair-20261123；最终分组计数、文件清单和包验证结果见该目录 final-summary.json 与 work/delivery-progress-20261123。FAIL 0；BLOCKED 2 为合并与完整目标新回归；NOT RUN 3 为 systemd 安装、实际恢复、7 天观察。只读审查不纳入运行 PASS 计数。

- Windows Go：`go test -p=1 -count=1 -v ./...`、vet、build 均 exit 0；gofmt 无输出，go list 为原五包。日志 final-test/vet/build.log、final-go-results.json、final-go-packages.log。
- 新增五项集成覆盖：RED 为四项失败、一项原有边界通过，GREEN 五项通过，见 s3-red/green.log；不是五项都先失败。
- 真实 Chrome 八组 PASS：配对/错误码、只读与转义、撤销后 401、queued 实际删除、running 无登记时 409 和重读、迟到 poll/report/cancel 隔离、无 JS 异常。见 browser-regression.py、browser-result.json；桌面/390px 手机截图已查看，未横向溢出。
- 七脚本逐个 syntax、PowerShell AST、health 四场景、CI 二脚本红/绿、实际 SHA 清单消费者与 CRLF 顺序检查通过，见 s5-*.log 与 ci-shell-*.log。S5 入口在 Windows 正确拒绝 Linux-only 验证，不把拒绝当 Linux fixture PASS。
- 两个命令 Linux/amd64、CGO=0 交叉编译 exit 0，见 linux-cross-build-results.json；目标运行 NOT RUN。
- 新传输包的集合/字节哈希验证、新统一 ZIP 的安全路径/CRC/改动文件哈希及原基线/只读补丁检查结果，按实际输出写入 final-summary.json 与新包日志。旧修复 ZIP SHA 保持不变。

### 阶段度量、检查点与恢复

阶段前 87 文件的 SHA-256 与逐文件 .snapshot 在 work/progress-repair-20261123/baseline.json 和 before/；不会参与 Go 包发现或传输。与本轮基线、原 F 基线分别计算文本 diff，按实现/测试/文档分类，实际文件数和增删行见 diff-stat.json、diff-summary.json；排除缓存、二进制、日志、截图、包与恢复副本。无 Git 仓库，不创建 commit。

返工和环境事件详细口径见最终 summary：首次实现不计返工，失败检查/审查反馈后的修复与复验才计；原有 F/SSH 阻塞未解除不重复计新事件。WSL 和界面引擎不可用作为能力限制记录，前者不能代替既定目标，后者已按现有页面上下文推进。

当前代码版本由新包 changed-files.sha256 固定；原目录完整基线由 baseline.sha256 固定，ZIP 校验值在外部 archive.sha256，避免自包含哈希。原目录未改，无需回退；将来合并前再次核对基线及新文件不存在，备份所有待替换原文件后逐项复制。实际恢复演练仍不是本轮副本回退或备份材料的替代。

## 历史 · 2026-11-23 · 首次续接修复与交接

### 结论与状态

S5 当前阻塞：临时副本已修复三项复核问题并通过本地检查；原项目合并、Linux 新修复复验、实际部署/恢复和 7 天观察尚未完成。本状态只适用于续接副本，原 F:/repotext 未修改。

### 需要用户决定的事

当前会话无法读取 SSH 配置，且原目录不在可写范围。将 F:/repotext 设为可写项目并允许既有 SSH 连接后，可按基线哈希合并修复包并继续目标验收。实际 systemd 安装、服务启动和恢复演练仍需明确授权，目标和步骤见 deployment.md。

### 做了什么、优化及代价、没做什么

恢复 r14 退出码 0 的收尾证据并核对源哈希。修复并发备份清理、健康探针 URI authority/HTTP200 校验及恢复说明；复用 Go 标准库、shell、curl 和 Python 标准库，不加依赖。代价是备份增加一次独占空文件创建、保留一个校验后执行的测试边界，CI 增加实际健康 HTTP 回归。

未合并到原目录、未做新 Linux runner 或 systemd 验收、未部署、未开启 LAN/runner、未进行长期观察。

### 告警与代码错误

- ISSUE-011（S1）：确定性检查复现旧逻辑删除其他调用完成的备份；副本本地修复通过，Linux 待验。
- ISSUE-012（S1）：旧脚本接受 userinfo 和 201；实际 HTTP 先 FAIL，修复后 PASS。
- ISSUE-013（S1 文档缺陷）：恢复缺少 WAL/SHM 与属主；说明修正并独立复核，实际服务恢复归 ISSUE-010，仍 NOT RUN。
- ISSUE-004：延期风险保留，rootful Docker 共宿主及共享 Linux 内核。
- 本次修复经未参与实施的代理独立静态复核，未发现变更范围内剩余 S0/S1；静态复核不替代目标机测试。

### 当前验收结果（只统计本节本轮检查）

- PASS 7：Windows 全套 Go test、vet、build；gofmt 无输出；逐脚本 shell syntax；实际 health HTTP 四项边界检查（作为一组）；原目录只读补丁检查（显式忽略 CRLF 上下文空白）。
- FAIL 0；BLOCKED 2：新 Linux SSH 验收、原目录合并；NOT RUN 3：systemd 安装、目标服务恢复演练、7 天观察。
- Windows backup 路径测试有 1 个既有 SKIP，未把该 Linux 路径检查计为 PASS；S4 Docker/GitHub 测试在本地无镜像/目标条件时跳过，当前不补算 Linux PASS。
- 历史 r14：GO_TEST_PASS、GO_VET_PASS、GO_BUILD_PASS、QUOTA_PRESSURE_PASS、HTTP succeeded/cancelled、RUNNER_RESOURCES_CLEAN；exit 0。原 Go 源码与 r14 清单一致，新修复另待 Linux 复验。
- 命令、退出码和日志：work/verification/go-results.json、test.log、health.log、shell-syntax.log，以及 tdd-review.md。

### 阶段度量与恢复

本轮 diff：10 文件，+320/-20 行。以复制前原目录哈希为基线；排除缓存、临时文件、日志、历史归档和生成包，详细 numstat 口径在 work/delivery/diff-stat.json。无 Git 仓库，不创建 commit。

返工 1 轮：health 首次修复仍接受 userinfo，第二次完整 authority 校验后通过；备份首次实现和无改动重跑不计返工。历史缺失度量不按零计。本轮独立环境阻塞事件 2：SSH 配置读取被拒；普通用户目录新建副本被拒（系统临时目录替代已解除）。原 F 目录不在可写范围是本轮合并限制，未试图写入。

恢复：原 F:/repotext 完整保留。修复包含 baseline.sha256、changes.patch、改动文件和验证日志；合并前校验基线，再备份原改动文件。无需还原原目录来撤销本轮；删除或弃用修复副本不会影响原项目。

全文一致性检查：三个当前状态块、问题总表、计划与部署说明一致；历史条目保留日期，当前结论以本节和问题总表为准。实际部署/恢复/观察均未写成通过。

## 历史 · 2026-11-21 · S4 本机实现检查点

### 结论与状态
S4 本机实现已接入；本节原始检查点保留为历史。目标机复验见下一节，执行开关仍关闭。

以下验收表记录 SSH 恢复前的历史状态；当前结论以“目标机复验 r12”、S4 收尾和 S5 节及顶部状态块为准。

### 已完成
- 依赖获取使用固定 GitHub commit 归档和现有安全解包；依赖容器仅连接 Docker internal 网络，只允许访问主机上的模块代理。代理限制 Go module/sumdb 路径、method、响应/总字节数，拒绝二次编码路径，目标域名固定并将 DNS 结果校验为公网 IP 后直连锁定 IP；仅允许 ZIP 请求一次跳转至固定 Google Storage 对象路径。测试容器使用 `--network=none`、只读 rootfs、非 root UID、无 capabilities、内存/CPU/PID 上限及大小受限 tmpfs。
- 增加 SQLite 一次性 preview nonce、事务消费 nonce 与创建 task/outbox；worker claim/lease 消费 outbox，attempt-fenced 更新任务阶段与终态，并持久化报告/日志。服务启动恢复 interrupted task，取消通过 process registry 等待 worker 退出。默认仍无执行入口；执行还需要显式验收门控、Linux 和本地固定镜像。
- 本机 UI 提供明确 commit 确认、任务状态、取消、报告读取和删除。LAN handler 仍不挂载 task mutation。
- 加入 OS 账户 home 下私有 `~/.local/state/repotest` Linux advisory lock，并将 Docker cleanup label 限定到当前 OS UID；锁路径不受 `HOME`/`XDG_CACHE_HOME` 环境覆盖影响，状态目录校验失败时启动关闭。Linux 实测仍待执行。

### 验收与证据
| 检查 | 对象/命令 | 结果 |
|---|---|---|
| 全套单测 | Windows Go 1.26.3 `go test -p=1 ./... -count=1`，GOMAXPROCS=2 | PASS，所有 package 通过 |
| 静态检查 | `go vet ./...` | PASS |
| 构建 | `go build ./...` | PASS |
| 格式与 UI 脚本 | `gofmt -l` 无输出；Node `vm.Script` 解析嵌入脚本 | PASS |
| Linux Docker runner 测试 | `TestOfflineGoInDocker`, `TestPinnedGitHubCommitRunsOffline` | NOT RUN：本机 Windows；需目标 Linux 与 `REPOTEST_TEST_IMAGE` |
| 登录密钥指纹 | `ssh-keygen -lf` 检查用户指定的私钥文件 | PASS：ED25519，`SHA256:3ViieSxNieZx68+nmZ031rMPtvb0n54h/eZkXJ9Zk4E`，与用户提供值一致；该值是登录密钥指纹 |
| 服务端主机公钥 | `ssh-keyscan -T 8 -p 2222 ssh.instabox.sh` | BLOCKED：当前运行网络层返回 `Permission denied`，未取得服务端 host key |
| SSH 连接复查 | 显式指定主机、2222 端口、专用用户与原密钥路径，关闭别名配置并使用 BatchMode | BLOCKED：`ssh: connect to host ssh.instabox.sh port 2222: Permission denied`，尚未到达认证阶段；原私钥 ACL 另有 Authenticated Users/Users 访问权限，OpenSSH 提示权限过宽。未修改私钥、ACL 或用户 SSH 配置 |
| Linux 端到端 | 目标机容器/代理/HTTP/CPU/PID/内存/磁盘/取消/重启 | NOT RUN：未建立 SSH 连接 |

### 尚未完成与风险
AC-008 仍有未关闭 S1/限制：目标机实际 runner、代理拒绝绕过与出站隔离、压力限制、持久化磁盘硬配额、取消/重启残留清理、端到端报告页面均未实测。tmpfs 容量和应用层归档/模块缓存字节上限不是目标主机文件系统 quota 的替代验收。rootful Docker daemon 仍与其他应用共用；执行仍关闭。

### 度量与检查点
本目录无 Git 且没有完整阶段前文件快照，阶段 diff 文件数/增删行数、返工轮次与阻塞计数为 NOT RECORDED。未把无 Git 的历史修改误算为本轮变更。当前源文件 SHA-256 检查点见 [`work/s4-code-checkpoint.sha256`](../work/s4-code-checkpoint.sha256)；恢复需要按该文件重新校验后再从外部备份恢复，本报告不声称已创建完整回滚副本。独立只读复核发现的启动清理并发竞态已通过固定 per-UID 锁及按 UID 隔离 Docker label 处理；目标 Linux 上锁行为仍未实测。

## 2026-11-21 · S4 目标机复验 r8

### 结论与状态
S4 目标机核心 runner 验收通过，阶段状态为待补证。执行开关继续关闭，因为目标文件系统的持久磁盘硬 quota 尚未实测；tmpfs 和应用层字节上限不能替代该证据。

### 验收与证据
| 检查 | 对象/命令 | 实际结果 | 状态 |
|---|---|---|---|
| 主机事实 | SSH `instabox-test-env`；Debian 13、Linux 6.18.54、x86_64、Go 1.27.1、Docker 29.8.2、cgroup v2/systemd | 与既有目标记录一致；主机公钥 ED25519 `SHA256:0XsbK2JfDp3PfQYJBUefUg9MncflVj/cBmAlrLK+ZM0` | PASS |
| 容器隔离与资源 | `scripts/verify-sandbox-profile.sh` | `--network none`、只读 rootfs、非 root、capabilities 全丢弃；CPU `50000/100000`、memory `134217728`、PIDs `32`；CPU throttle、PID reject、OOMKilled、16 MiB tmpfs 均实测 | PASS |
| 依赖代理与来源 | `TestPinnedGitHubCommitRunsOffline` | 固定 commit `b4617d0b9670ad14039b2739167fd35a60f557c5` 通过 GitHub 归档和受限模块代理进入 offline runner；`github.com/gorilla/mux` passed | PASS |
| 断网执行/超时清理 | `TestOfflineGoInDocker` | 仅 loopback、只读 rootfs、cgroup 上限；超时后容器无残留 | PASS |
| 实际 HTTP 执行链路 | `scripts/verify-http-runner.sh`，门控 `REPOTEST_RUNNER_ENABLED=1` + `REPOTEST_AC008_ACCEPTED=1` | CSRF 预览、固定 commit 确认、worker、报告读取成功；任务 `daa6445446c45eb174ae556f5dc4454d` succeeded | PASS |
| 实际取消 | 同一 HTTP 脚本第二任务 | loopback cancel 返回 204，任务 `5ef8a871e8dc05232cf9dc4c11dc316a` 终态 cancelled | PASS |
| 重启/残留清理 | Linux runner 单测、最终 Docker label 扫描 | `CleanupInterrupted`/scratch marker 测试通过；最终 runner containers/networks 为空 | PASS |
| 持久磁盘硬 quota | SSH 只读盘点：`/` 为 ext4、`/tmp` 为 tmpfs；目标无 `quota` 工具，未配置独立 quota 验收 | 未执行；不能由 tmpfs 结果推断 | NOT RUN |
| 代码回归 | Linux r8 `go test -p=1 -count=1 ./...`、`go vet ./...`、`go build ./...`；Windows 同三命令 | 全部 exit 0；r8 归档 SHA-256 `0FE3427ADFBAA33F9F3E5843E63BB265B90E1102F6C3B2C41AC1C4CBC9DD8324` | PASS |

### 修复与问题
ISSUE-009 已关闭：GitHub 全局 PAX comment 兼容与危险 metadata 拒绝、依赖容器 stdin/模块 tar 导出、Linux 锁目录测试夹具和目标 `.local` 权限均已修复/核验。Windows SSH 二进制传输 BOM 也已修复；r8 为最终可重跑证据。ISSUE-004 保留为延期风险：rootful daemon 共宿主、容器共享宿主内核；未对陌生代码或公网启用执行。

### 度量与恢复
本目录无 Git，完整阶段 diff 文件数/增删行数仍 NOT RECORDED；本轮返工 4 轮（PAX/stdin/锁权限、模块导出、传输驱动、HTTP e2e），阻塞事件 1（目标 `.local` 目录组写权限，已获具体授权并调整为 0755）。r8 源码归档、日志、结果和哈希位于 [`work/s4-linux-20261121-r8`](../work/s4-linux-20261121-r8)；恢复副本位于 [`work/s4-linux-20261121-before`](../work/s4-linux-20261121-before)，其中 `.go.snapshot` 文件不参与构建。

## 2026-11-21 · S4 收尾 r10

S4/AC-008 已通过。r10 在目标机使用临时 2 GiB ext4 loopback 工作盘：写入 2.25 GiB 被拒绝；同一工作盘上的真实 HTTP runner 成功任务 `88d4f8c80ccc88558c368b356aa9655f` 与取消任务 `4ce06c2641fdf56c65499117608da9c6` 均通过，任务结束后工作盘仅保留 ext4 `lost+found`。Linux 全套 `go test -p=1 -count=1 ./...`、`go vet ./...`、`go build ./...` 和最终 Docker/网络清理均 PASS。r10 证据位于 [`work/s4-linux-20261121-r10`](../work/s4-linux-20261121-r10)；执行仍默认关闭，S5 部署前不启用。

## 2026-11-21 · S5 运维产物与 dry-run

### 结论与状态
S5 产物已实现，当前待补实际部署验收。未安装 systemd unit、未启用服务、未开放 LAN、未启用 runner，也未开始七天稳定性观察。

### 产物与检查
| 检查 | 对象/操作 | 实际结果 | 状态 |
|---|---|---|---|
| 部署 preflight | `scripts/check-deployment.sh`；目标机 `sh -n`、私有路径成功、非 loopback/0750 失败分支 | `PREFLIGHT_REMOTE_PASS`；默认输出 `RUNNER_DISABLED=1` 和 `DEPLOYMENT_PREFLIGHT_PASS` | PASS |
| systemd/env 契约 | `deploy/repotest.service`, `deploy/repotest.env.example` | dedicated user、ProtectSystem、NoNewPrivileges、私有 work/db、loopback、runner gates 默认 0 | PASS（静态） |
| SQLite backup | `cmd/repotest-backup`; Windows 聚焦测试与 Linux r12 全套测试 | `VACUUM INTO`、完整性检查、拒绝覆盖；聚焦测试 PASS | PASS |
| health probe | `scripts/check-health.sh`；目标机 `sh -n` 和 `127.0.0.1:1` 失败路径 | `HEALTH_PROBE_SYNTAX_AND_FAILURE_PASS` | PASS |
| CI 契约 | `.github/workflows/ci.yml` | Windows/Ubuntu test/vet/build；Ubuntu shell syntax；runner gates 0；无 deploy/push | PASS（静态） |
| S4 累积 smoke | Linux r12 `go test -p=1 -count=1 ./...`, `go vet ./...`, `go build ./...`, quota/HTTP runner | exit 0；quota 压力、HTTP succeeded/cancelled、清理 PASS；归档 SHA-256 `24ABD06336F417BA6E779A4AF250DDE6B7F1C65A9F9408DA6936F29DF66DB8AD` | PASS |
| 实际部署/恢复/长期观察 | systemd 安装、备份恢复到目标服务、7 天观察 | 未执行；用户尚未授权部署 | NOT RUN |

### 变更与回退
新增 `deploy/`, `cmd/repotest-backup/`, `scripts/check-deployment.sh`, `scripts/check-health.sh`, `.github/workflows/ci.yml` 和 `docs/deployment.md`。回退为删除未安装的 S5 产物或恢复本阶段文件副本；没有执行 systemd、数据库迁移、对外发送或 LAN 变更。


## 2026-11-18 · S3 控制面检查点 · loopback 管理与 listener 启动边界

### 结论与状态
本检查点的管理来源隔离、终态任务删除和 listener 失败关闭均已实现并验证；S3 整体仍进行中，未达到阶段退出标准。独立复核发现的 localhost 名称解析风险已修复并二次复核关闭。

### 需要你决定的事
确认本检查点结果后，继续完成 S3 剩余验收（CSRF token、执行器接入前活动任务限制与后续回收机制、幂等键/outbox/故障恢复）。本检查点不等于 S3 验收通过。

### 已完成与代价
`cmd/repotest/main.go` 只接受 loopback IP 管理 bind，先绑定两个 listener 再开始 Serve；失败关闭已绑定 socket。管理 preview/cancel/delete 检查 loopback RemoteAddr、Host、Origin。删除只允许终态任务，活动任务返回 409，避免在没有执行器回收能力时误删。代价是用户需经 `127.0.0.1` 或 `[::1]` 访问管理页面，`localhost` Host 不接受。

### 告警、错误与未完成
ISSUE-007 已关闭（本轮实现并经独立复核）；ISSUE-001 仍部分未关闭：未实现独立 CSRF token，LAN Host 的允许集合尚未绑定部署接口地址。AC-005 活动任务停止回收未实现，因为执行器未接入。AC-007 幂等缓存、唯一约束/outbox 未实现。执行器安全风险 ISSUE-004 仍禁止用于不可信代码。

### 验收证据
| 项目 | 操作 | 实际结果 | 状态 |
|---|---|---|---|
| loopback 删除 | Windows `go test ./internal/control -run 'TestLoopbackAdminDelete|TestLANHandlerRejectsTaskDeletion' -count=1` | 终态删除 204，task/report/log 级联不可读；活动态 409；LAN 删除 405 | PASS |
| mutation 来源 | `TestLoopbackCancelRejectsLANAndAcceptsMatchingLocalOrigin` 与 delete Host/Origin 测试 | 非 loopback/错误 Host/Origin 403；正确 loopback cancel 成功 | PASS |
| listener bind 单元 | `go test ./cmd/repotest -run TestBindListeners -count=1` | 第二个 bind 失败后第一个 socket 可重新绑定；显式 loopback 地址限制通过 | PASS |
| 进程失败关闭 | Debian 13 x86_64 `go test ./cmd/repotest -run 'TestProcess(Migration|Startup)' -count=1 -v`（经全套测试执行） | LAN bind 失败、loopback bind 失败、SQLite 初始化失败均退出；另一 listener 未开放 | PASS |
| 远端全套与构建 | Debian 13 x86_64 `go test ./...`、`go build ./cmd/repotest` | 所有 package 测试通过；服务构建成功 | PASS |
| 远端 HTTP 双 listener | SSH stdin 字节流执行 staging 包 | 未配对报告 401；pair 200；report/log 200；LAN POST/DELETE 405；loopback health 200；loopback delete 204 | PASS |
| 本地静态检查 | Windows `go test ./... -count=1`; `go vet ./...`; `go build ./...` | exit 0 | PASS |
| 独立安全复核 | 修改前后复核 `bindListeners`/Host 校验 | 发现 localhost DNS/hosts 风险；改为显式 IP 后复核关闭该项；范围仅本轮改动 | PASS |

### 产物与恢复
源码：`cmd/repotest/main.go`、`cmd/repotest/listeners_test.go`、`cmd/repotest/listeners_process_test.go`、`internal/control/local_request.go`、`internal/control/handler.go`、`internal/control/http.go`、`internal/control/store.go`、`internal/control/management_delete_test.go`、既有管理/预览相关测试及阶段文档。完整源码和验证脚本打包为合并压缩包，经 SSH stdin 传输；详细 SHA-256 在 staging 清单中更新。无 Git 仓库，当前未提交；恢复方式为从修改前文件副本逐项还原（本轮没有保存完整阶段前快照）。

### 阶段度量
无 Git 且未保存完整阶段前文本快照，diff 行数/文件数记为 NOT RECORDED；返工轮次与阻塞次数无可靠阶段基线，记为 NOT RECORDED。前述失败命令保留在对话证据中，不算当前验收失败。
## 2026-11-18 · S2 · 真实仓库预览（修复后）

### 结论
S2 实现和验收证据通过，等待用户阶段验收。独立复核：未独立复核。检查点：源码由工作区传入临时远端目录；运行结束已清理。没有 Git 仓库，采用本报告末尾 SHA-256；未修改 SSH 配置/key，无触碰端口 8787 服务。

### 需要你决定的事
验收本阶段，并确认是否进入 S3（配对认证、LAN 只读访问、SQLite 持久化及清理）。

### 已完成
真实 GitHub 预览 API 和嵌入式首页；预览不创建任务/执行代码；ref 解析默认分支、branch、tag、40 位 SHA；annotated tag 解引用限制最多 5 层；目标架构采用 runtime GOOS/GOARCH。默认 loopback 监听，S3 前拒绝 LAN bind。

### 优化手段与代价
基于 Debian 13 x86_64 实机结果修正此前 arm64 硬编码；tag fallback 经分支/tag类型显式处理；SHA 查询 404/422 映射为 ref not found。代价：预览可能产生多个 GitHub API 请求，受匿名 API 速率限制。

### 未完成与未做事项
尚无确认提交 API、任务持久化、报告页、配对、TTL 清理和执行器。当前内存 store 与旧 cancel route 仍在代码中，但预览页面不调用；loopback 默认且拒绝 LAN 绑定。任务/API 完整安全收敛安排 S3。

### 告警与代码错误
| 编号 | 类型 | 级别 | 证据与影响 | 处理状态 |
|---|---|---|---|---|
| ISSUE-001 | 风险 | S1 | LAN 只读鉴权尚未实现 | S3 前拒绝 LAN bind，延期 |
| ISSUE-002 | 错误 | S1 | 原 POST tasks 直建任务 | 预览页仅调用 preview；旧 handler 后续移除，延至 S3 |
| ISSUE-003 | 风险 | S1 | cancel route 目前无认证 | loopback only，S3 修复 |
| ISSUE-004 | 限制 | S1 | ProcessRunner 不是沙箱 | Web 未调用；S4 前不得运行不可信代码 |
| ISSUE-005 | 限制 | S2 | 目标 Debian 13 x86_64，与 PDF 预期 ARM64 不同 | 已通过 runtime 检测修正预览架构标识；容器能力留到 S4 实测 |
| ISSUE-006 | 风险 | S2 | cache key/唯一约束未实现 | S3/S4 |

检查范围：预览、URL 校验、runtime 目标字段、服务监听默认和启动拒绝；没有做全面安全审计。

### 验收与检查证据
| 项目 | 操作 | 预期 | 实际 | 状态 |
|---|---|---|---|---|
| 单元/构建 | Windows `go test ./...`, `go vet ./...`, `go build ./...` | 全通过 | exit 0 | PASS |
| 单元/构建 | Debian 13 x86_64 同三命令 | 全通过 | exit 0 | PASS |
| 默认分支 | 目标机预览 `gorilla/mux` | 返回 SHA 和 linux/amd64 | `db9d1d0073d27a0a2d9a8c1bc52aa0af4374d265` | PASS |
| tag | 目标机预览 `v1.8.1` | 固定为 commit SHA | `b4617d0b9670ad14039b2739167fd35a60f557c5` | PASS |
| 40位 SHA | 目标机有效 SHA/不存在 SHA | 校验仓库归属，错误清晰 | 有效 SHA 返回相同值；随机 SHA `repo_or_ref_not_found` | PASS |
| 输入校验 | 非 github.com URL | invalid_url，不向上游请求 | 本地 transport 测试 PASS | PASS |
| 首页与无假运行 | `GET /` 和 preview-only form | HTTP 200，无执行授权或任务副作用 | 首页测试 PASS；preview 路由未调用 Store；远端任务 GET 404 | PASS |
| LAN 边界 | 非 loopback bind | S3 配对完成前拒绝 | main 启动 guard；未作独立进程级测试 | NOT RUN |

### 阶段度量
无 Git 基线；改动文件计数实测约 13 个，增删行数 N/A（未保存阶段前哈希快照）。返工轮次 4（测试夹具、ref API 形状、文件拼接损坏、SHA错误映射）；阻塞事件 1（Windows 二进制管道被文本化，改用 .NET 字节流解除）。

### 沟通与后续
目标机 Debian 13 x86_64、4核、7.8GiB；Go 1.27.1、Git、curl、Docker 可用。实际目标架构已写入 project-design。S2 后仍不开放 LAN；下一阶段必须先做认证边界。
全文一致性检查：本报告、设计和问题记录统一以 S2 通过、待用户验收为当前状态。历史失败留在沟通记录中但不计当前失败项。

### 文件哈希
<!-- 更新前最终文件哈希 -->

## 2026-11-19 · S3 CSRF 接线复验

loopback 服务进程启动时生成 256-bit 随机 token；loopback 首页在 `no-store` HTML meta 中提供 token；预览页面通过 `X-CSRF-Token` 发送。预览、取消、删除均要求正确 token、loopback RemoteAddr 和 IP Host/Origin。LAN handler 不挂载 mutation 路由。

Windows 已运行 `go test ./... -count=1`、`go vet ./...`、`go build ./...`，均 exit 0。Debian 13 x86_64 使用合并 staging 包经 SSH 标准输入传输并执行：`go test ./...` 全通过；进程级结果为页面 token 取得成功、缺失 token `403`、伪造 token/Host `403`、正确 token 进入 preview handler 并按非法 URL 返回 `400`；配对报告/日志仍为 `200`，LAN 写/删 `405`，loopback health `200`、删除 `204`。远端 SSH 命令 exit 0。

staging 合并包仅含 `source.tar.gz` 与 `verify.sh`，本次传输前 SHA-256 为 `7040FB40D7EB47A3E68A6D4F099938CFC1F6729A51C2D9639C35F50673AA4ECF`。本轮未改执行器或幂等/outbox；S3 整体仍未通过，活动任务回收、幂等/outbox 及 LAN Host allowlist 绑定仍未完成。

## 2026-11-19 · ISSUE-008 实施与 Linux 复验

- 活动进程：Linux runner 使用独立进程组；取消触发组级 SIGKILL；registry 提供 CancelAndWait；删除运行任务等待退出后再落终态和删除。单测覆盖真实 shell 子进程，另有 HTTP delete 集成测试。非 Linux 使用 Go 默认 CommandContext 取消机制。
- 幂等/outbox：SQLite 事务内创建任务和 outbox；唯一部分索引约束 work key；ExecutionIdentity 字段不可由其他包直接构造，key 在 store 中生成并校验 task 仓库/commit 一致；并发重复返回同一任务。测试覆盖版本差异、过期终态、并发唯一、重启 claim/lease/retry/ack、身份不匹配不落库。
- LAN Host：生产 main 在绑定 listener 后从真实 listener 地址生成 allowlist；指定 IP 使用绑定 IP，通配地址枚举启动时接口 IP；handler 要求 Host 是 allowlist 中的 IP literal。Linux 进程级验证允许绑定 IP Host 返回 200，伪造 hostname 返回 421。
- 本机证据：Windows `go test ./... -count=1`、`go vet ./...`、`go build ./...` 均通过。Debian 13 x86_64 合并包验证全套 `go test -count=1 ./...`、`go vet ./...`、`go build ./...` exit 0；进程级 HTTP 检查 report/log 200、LAN write/delete 405、未允许 Host 421、允许 Host 200、loopback health 200 和活动任务删除 204。SSH 命令 exit 0。
- 传输证据：合并压缩包仅含 `source.tar.gz` 与 `verify.sh`，经 SSH 字节流传输并完整执行；远端测试目录及压缩包已由退出 trap 清理。最终包 SHA-256 保存在工作区 staging 旁的 `manifest.txt`，避免归档自包含哈希造成递归变化。
- 限制：Web 任务执行器/消费者仍未接入；outbox 有持久化和 claim 原语但没有 dispatcher/consumer。进程组无法约束主动 `setsid()` 脱离的恶意后代，需后续 cgroup/PID namespace 隔离；因此 ISSUE-004 仍保持禁用不可信代码执行。
- 独立安全复核：复核未发现本轮范围内的可利用问题；要求保留两项明确边界：进程组之外的 daemonized/`setsid()` 后代需要 cgroup/PID namespace，outbox 原语目前没有消费者。两项已列为后续限制，不宣称已解决。

## 2026-11-19 · S3 验收与 S4 sandbox profile 检查点

- S3 ISSUE-008 检查点已由用户确认。Windows 全套 Go 检查和 Debian 13 x86_64 合并包进程验证见上节，均通过。
- 用户确认使用当前 `instabox-test-env` 做 S4 验收。SSH 环境：Linux `6.18.54-instabox` x86_64，cgroup v2，Docker client/server 29.8.2，systemd cgroup driver；Docker daemon rootful，`/var/run/docker.sock` 为 `root:docker`。既有其他应用容器/网络继续视为不可信邻接服务，探针无宿主目录挂载、无端口、`--network none`，且 `--rm` 清理；未连接/修改 8787 服务。
- 可重复探针：`scripts/verify-sandbox-profile.sh`；通过 SSH stdin 执行，exit 0。输出 `CPU_MAX=50000 100000`、`MEMORY_MAX=134217728`、`PIDS_MAX=32`、capabilities 全丢弃、只读 rootfs、仅 loopback、16 MiB tmpfs 超限被拒；第二个容器 64 MiB 内存限制下写 96 MiB tmpfs，Docker inspect 同时报告 `ExitCode=137` 与 `OOMKilled=true`，随后删除容器。
- 独立安全复核：确认不挂载宿主路径、不发布端口、不连接网络、不改既有容器；指出 137 本身不足以归因 OOM。修复为检查 Docker `OOMKilled=true`，复测通过。第二次只读复核通过清理与状态检查逻辑。
- 尚未验证：CPU/PID 的实际压力行为；tmpfs 是内存盘，不等于持久磁盘 quota；没有 RepoTest runner image、源代码获取、模块代理、离线 go test 或端到端取消测试。AC-008 整体仍 NOT RUN/未通过，执行开关保持关闭。
