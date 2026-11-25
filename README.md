# RepoTest

RepoTest 是预览公开 GitHub 仓库、确认精确 commit 后生成 Go 测试报告的实验性服务。当前仍处于部署验收前阶段，默认只监听本机且关闭执行器。

本机管理员可预览、确认任务（执行能力启用时）、查看报告、取消或删除任务，并列出和撤销已配对设备。LAN 页面提供一次性配对和按任务 ID 查看状态、报告与日志；LAN 设备只能读取。

2026-11-24 已完成修复合并、本机回归和目标 Linux 全套验收。Linux r17 为 164 PASS、0 SKIP、0 FAIL，包含隔离、硬 quota、HTTP 执行/取消/清理及 S5 备份、健康探针和部署预检。部署准备入口已补齐目标事实与二进制哈希采集；实际 systemd 安装、恢复演练和 7 天观察尚未完成，当前进度以 [阶段报告](docs/stage-report.md) 为准。运行日志、恢复快照、数据库、缓存和验收归档保存在本地 `work/`，不纳入 Git。

## 本地开发检查

```powershell
go test -p=1 -count=1 ./...
go vet ./...
go build ./...
```

本机服务使用 `go run ./cmd/repotest`，默认地址为 `127.0.0.1:8080`。仓库执行还要求目标 Linux、固定本地镜像和 AC-008 门控；启动时配对码不会授予设备执行权限。

## 部署准备

运行 `scripts/invoke-linux-acceptance.ps1 -PrepareOnly -EvidenceName <新证据目录>` 只生成传输包与哈希清单。实际目标验证运行完整 Linux 入口，包含 S4 与新增 S5 检查。安装、备份/恢复和监控步骤见 [部署手册](docs/deployment.md)。

## 安全状态

当前修复已在既有 Linux 目标通过容器资源、断网、取消与硬 quota 回归；这不代表实际服务部署和长期观察已经完成。rootful Docker 共宿主和共享内核风险保留，公网及陌生用户提交不在当前范围。当前信任边界见 [设计文档](docs/project-design.md)；[原始设计复核](docs/DESIGN-REVIEW.md) 保留为历史。
