(() => {
  const root = document.querySelector('#public-report');
  if (!root) return;

  const taskId = root.dataset.taskId;
  const status = document.querySelector('#public-status');
  const mount = document.querySelector('#report-mount');
  const labels = {
    queued: '任务排队中',
    running: '任务运行中',
    succeeded: '测试完成',
    failed: '测试失败',
    timeout: '测试超时',
    cancelled: '任务已取消',
    error: '执行错误'
  };

  async function load() {
    try {
      const response = await fetch(`/api/v1/public/reports/${encodeURIComponent(taskId)}`, {
        cache: 'no-store',
        credentials: 'omit',
        headers: {Accept: 'application/json'}
      });
      if (response.status === 410) {
        status.textContent = '报告已过期，数据已清除。';
        return;
      }
      if (!response.ok) {
        status.textContent = response.status === 404 ? '报告不存在或已不可用。' : '暂时无法读取报告。';
        return;
      }

      const data = await response.json();
      status.textContent = labels[data.task?.status] || '报告状态未知';
      if (data.report) {
        window.RepoTestReport.render(mount, data.report);
      } else {
        mount.textContent = '报告尚未生成。';
      }
    } catch {
      status.textContent = '暂时无法连接报告服务。';
    }
  }

  load();
})();
