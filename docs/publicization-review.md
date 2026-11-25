# RepoTest 公网化审查与改造依据

审查版本：10a0bc506ca88eb4a319845e3224a3d11a74e0d2。日期：2026-11-25。
新目标来源：用户提供的《RepoTest 公网化改造清单》，7页，SHA-256 90bc76ff23ea86c64744da0af1d83671ce60b51e9e8ced88cb0b2a0d2182eddd；用户要求按清单进行，并确定起步并发1。

## 方向与生产判断

原产品是单管理员自托管的公开Go仓库测试工具：预览精确commit、明确确认、隔离执行、查看报告，LAN配对设备只读。单体Go、SQLite、单worker与内嵌网页符合该首版目标，未发现必须引入账号、Redis或分布式队列的依据。网络代理、资源硬限额、恢复和监控属于必要支撑。

新目标是陌生访客提交公开仓库并获得可转发的只读结构化报告。用户明确要求按公网清单改造，这是授权的范围变化。不能仅移除loopback限制或加反向代理；权限、配额、任务故障语义、日志展示和隔离边界都要随之改变。

当前版本不具备公网生产条件，也不能据164项旧测试通过或七天health观察宣称完整执行产品已可生产。实际部署仍关闭runner/LAN；已有七天观察属于旧版本的预览实例。此前“只剩七天观察”的进度结论只覆盖旧部署清单，作为项目生产判断不充分，现由本审查取代。

## 已核实的问题

- AUD-001 / P1：任务没有持久化并校验执行身份。配置A创建的任务可由B执行，结果仍在A work_key下复用。实际API/SQLite/worker等价fixture复现，未在真实Docker制造故障。位置：internal/domain/task.go:31、cmd/repotest/main.go:65、internal/control/task_worker.go:93、internal/control/sqlite_store.go:522。安排到PUBLIC-3/4，公网开放前关闭。
- AUD-002 / P1：完成写库临时失败后，任务保持running；错误解除后跨三个租约仍跳过，没有注册进程且DELETE=409。真实SQLite trigger故障注入复现。位置：internal/control/task_worker.go:63、159、186。安排到PUBLIC-4，监测必须包含真实任务进度而非只有HTTP存活。
- AUD-003 / P1：取消与Docker清理失败errors.Join后，worker优先按Canceled完成并吞掉清理错误；DELETE可返回204并丢失元数据，资源仍可能存在。真实Worker/SQLite加模拟资源回收失败复现，未操作实际Docker。位置：internal/runner/offline.go:95、225；internal/control/task_worker.go:115；internal/control/process_registry.go:58。安排到PUBLIC-3/4，清理失败必须保留可恢复记录并阻止删除完成。
- AUD-004 / P1产品闭环缺口：解析器跳过Test事件，失败原因没有进入报告；本机日志路由不存在（实际Handler返回404），刷新后也无旧任务重开入口。位置：internal/runner/report.go:22、55；internal/control/handler.go:27；internal/control/web/index.html:34。用例报告/受限诊断属于PUBLIC-1，分享与任务重开属于PUBLIC-2。
- AUD-005 / P2：依赖准备已捕获stderr，但offline.go:68-70错误分支返回空result，日志丢失；确定性配置/输入错误被统一重试并泛化。PUBLIC-1先保存受限输出；PUBLIC-3/4再分类错误与重试。
- AUD-006 / P2：当前公开归档仓库被额外拒绝；兼容范围（根go.mod、CGO关闭、固定本地工具链）未在预览清楚说明。位置：internal/control/http.go:97、150；internal/runner/offline.go:32、109。公开仓库策略须有明确产品理由，限制和错误必须可解释。
- AUD-007 / P2：每任务创建的module proxy Transport没有IdleConnTimeout或CloseIdleConnections；四个实际Transport接本地上游后GC仍保留四条闲置连接。位置：internal/runner/module_proxy.go:53、offline.go:194。PUBLIC-4补生命周期与控制面资源预算。
- AUD-008 / P2：LAN配对接受外源Origin/text/plain，五个坏码请求可耗尽全局尝试。仅做本地handler等价验证，未断言真实浏览器跨源投递。PUBLIC-3按清单移除该旧权限面，而非继续扩配对系统。
- AUD-009 / 生产证据缺口：旧HTTP执行验证使用临时进程/DB，确认后立即取消，未先等待running；不能证明生产服务身份、持久work、活动容器取消、恢复的完整链路。位置：scripts/verify-http-runner.sh:14、42-47。PUBLIC-5/6在专用目标按真实状态验证。

模块导出已有100/512MiB、条数和tmpfs边界，不能误报为“所有导出无界”。rootful Docker共享宿主是已接受的自用风险，不是已发生漏洞；它不能延用为陌生代码公网的开放依据。

复现记录在work/audit-20261125/correctness/_checks、product-alignment及security目录；这些检查通过表示问题成功复现，不表示生产功能通过。原始生产源码与既有测试在审查阶段未修改。

## 改造原则

按用户清单六阶段推进，先形成有诊断价值的报告，后改变权限和提交边界。原URL/ref/commit校验、归档解包限制与资源隔离保留。公网不采用账号体系；公开ID与私有控制令牌分离，API凭据永不进入任务。新沙箱及上线门槛未全部通过时，保持内网，不开放公网。
