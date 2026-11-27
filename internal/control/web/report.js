/* Shared report view. All runtime fields are untrusted text. */
window.RepoTestReport = (() => {
  const labels = {passed: '通过', failed: '失败', build_failed: '构建失败', skipped: '跳过', incomplete: '未结束', no_tests: '未发现测试'};
  const rank = status => ({failed: 0, build_failed: 0, incomplete: 1, skipped: 2, passed: 3, no_tests: 4}[status] ?? 5);
  const text = value => value == null ? '' : String(value);
  const node = (tag, content, className) => {
    const element = document.createElement(tag);
    if (content != null) element.textContent = text(content);
    if (className) element.className = className;
    return element;
  };
  const count = value => Number.isSafeInteger(value) && value >= 0 ? value : '未提供';
  const elapsed = value => Number.isFinite(value) && value >= 0 ? `${value} ms` : '耗时未提供';

  function sourceLink(report, location) {
    const repo = text(report.repo_url);
    const commit = text(report.commit);
    const path = text(location.path);
    if (!/^https:\/\/github\.com\/[A-Za-z0-9][A-Za-z0-9-]*\/[A-Za-z0-9_.-]+$/.test(repo) ||
        !/^[a-fA-F0-9]{40}$/.test(commit) || !Number.isSafeInteger(location.line) || location.line <= 0 ||
        !path || /[\\:%?#\x00-\x1f\x7f]/.test(path) || path.split('/').some(part => !part || part === '.' || part === '..')) return null;
    try {
      const repository = new URL(repo);
      if (repository.href !== repo) return null;
      const candidate = new URL(text(location.github_url));
      const prefix = repository.pathname + '/blob/' + commit + '/';
      if (candidate.origin !== 'https://github.com' || candidate.username || candidate.password ||
          candidate.search || candidate.hash !== '#L' + location.line || candidate.href !== location.github_url ||
          !candidate.pathname.startsWith(prefix)) return null;
      const encodedPath = candidate.pathname.slice(prefix.length);
      if (encodedPath.split('/').length !== path.split('/').length ||
          encodedPath.split('/').map(decodeURIComponent).join('/') !== path) return null;
      return candidate.href;
    } catch { return null; }
  }

  function appendLocation(target, report, location) {
    if (!location || typeof location !== 'object') return;
    const label = `${text(location.path)}:${text(location.line)}`;
    const href = sourceLink(report, location);
    const element = node(href ? 'a' : 'span', label, 'report-location');
    if (href) {
      element.href = href;
      element.target = '_blank';
      element.rel = 'noopener noreferrer';
      element.title = '查看此 commit 的源码位置；诊断来自仓库代码';
    }
    target.append(element);
  }

  function appendOutput(target, output, truncated, label) {
    if (!output && !truncated) return;
    const section = node('div', null, 'report-diagnostic');
    section.append(node('p', label + (truncated ? ' · 输出已截断' : ''), 'report-output-label'));
    if (output) {
      const pre = node('pre', output, 'report-output');
      pre.tabIndex = 0;
      pre.setAttribute('aria-label', label);
      section.append(pre);
    }
    target.append(section);
  }

  function appendTests(target, report, cases) {
    if (!Array.isArray(cases) || !cases.length) return;
    const tests = cases.filter(test => test && typeof test === 'object');
    const byName = new Map(tests.map(test => [text(test.name), test]));
    const children = new Map();
    tests.forEach(test => {
      const parent = byName.get(text(test.parent));
      if (parent && parent !== test) {
        if (!children.has(parent)) children.set(parent, []);
        children.get(parent).push(test);
      }
    });
    const ordered = list => [...list].sort((a, b) => rank(a.status) - rank(b.status) || text(a.name).localeCompare(text(b.name)));
    const rendered = new Set();
    function append(test, container, depth) {
      // Bound visual indentation; the flat fallback preserves deeper full names.
      if (rendered.has(test) || depth > 4) return;
      rendered.add(test);
      const details = node('details', null, 'report-test');
      details.open = rank(test.status) <= 1;
      const summary = node('summary');
      summary.append(node('span', test.name, 'report-name'), node('span', labels[test.status] || text(test.status), 'report-status'), node('span', elapsed(test.elapsed_ms), 'report-time'));
      details.append(summary);
      appendLocation(details, report, test.failure_location);
      appendOutput(details, test.output, test.output_truncated, '测试诊断');
      if (children.has(test)) ordered(children.get(test)).forEach(child => append(child, details, depth + 1));
      if (details.querySelector('details[open]')) details.open = true;
      container.append(details);
    }
    ordered(tests.filter(test => !byName.has(text(test.parent)) || byName.get(text(test.parent)) === test)).forEach(test => append(test, target, 0));
    // Keep malformed, cyclic, or excessively deep parent data readable as flat cases.
    ordered(tests).forEach(test => append(test, target, 0));
  }

  function render(target, data) {
    const report = data && typeof data === 'object' ? data : {};
    target.replaceChildren();
    target.classList.add('report');
    target.append(node('h2', '测试报告'));
    const metadata = [report.repo_url, report.commit, report.go_version, report.goos, report.goarch].filter(value => value != null && value !== '').map(text);
    if (metadata.length) target.append(node('p', metadata.join(' · '), 'report-meta'));
    target.append(node('p', '报告与诊断来自仓库中的不可信代码，不构成真实性证明。覆盖率仅展示包级结果，未计算总覆盖率。', 'report-trust'));
    if (report.summary && typeof report.summary === 'object') {
      const s = report.summary;
      target.append(node('p', `叶子测试：通过 ${count(s.passed)} · 失败 ${count(s.failed)} · 跳过 ${count(s.skipped)} · 未结束 ${count(s.incomplete)}；包级失败 ${count(s.package_failures)}`, 'report-summary'));
      if (s.dropped_cases) target.append(node('p', `用例数量达到上限，丢弃 ${count(s.dropped_cases)} 个用例；这些用例未计为通过。`, 'report-warning'));
    } else target.append(node('p', '叶子测试汇总未提供（旧版或包级报告）。', 'report-summary'));
    if (report.no_tests) target.append(node('p', '未发现测试；这不表示存在测试并通过。', 'report-warning'));
    if (report.incomplete) target.append(node('p', '报告不完整：存在未结束用例或采集限制，请结合诊断查看。', 'report-warning'));
    if (report.truncated) target.append(node('p', '报告已截断，部分输出或用例未保留。', 'report-warning'));
    appendOutput(target, report.output, report.output_truncated, '运行诊断');

    const controls = node('div', null, 'report-controls');
    const filter = node('input');
    filter.type = 'search';
    filter.placeholder = '输入包名';
    const status = node('select');
    [['', '全部状态'], ['failed', '失败（含构建失败）'], ['incomplete', '未结束'], ['passed', '通过'], ['skipped', '跳过'], ['no_tests', '无测试']].forEach(([value, title]) => { const option = node('option', title); option.value = value; status.append(option); });
    const sort = node('select');
    [['failure', '失败优先'], ['name', '包名'], ['elapsed', '耗时（从长到短）']].forEach(([value, title]) => { const option = node('option', title); option.value = value; sort.append(option); });
    [['包名筛选', filter], ['包状态', status], ['包排序', sort]].forEach(([title, control]) => { const label = node('label', title); label.append(control); controls.append(label); });
    target.append(controls);
    const resultCount = node('p', null, 'report-count');
    resultCount.setAttribute('aria-live', 'polite');
    const packages = node('div', null, 'report-packages');
    target.append(resultCount, packages);
    const all = Array.isArray(report.packages) ? report.packages.filter(pkg => pkg && typeof pkg === 'object') : [];
    function refresh() {
      packages.replaceChildren();
      const visible = all.filter(pkg => text(pkg.import_path).toLocaleLowerCase().includes(filter.value.toLocaleLowerCase()) && (!status.value || pkg.status === status.value || (status.value === 'failed' && pkg.status === 'build_failed')));
      visible.sort((a, b) => (sort.value === 'failure' ? rank(a.status) - rank(b.status) : sort.value === 'elapsed' ? (Number(b.elapsed_ms) || 0) - (Number(a.elapsed_ms) || 0) : 0) || text(a.import_path).localeCompare(text(b.import_path)));
      resultCount.textContent = `显示 ${visible.length} / ${all.length} 个包`;
      if (!visible.length) packages.append(node('p', all.length ? '没有匹配的包，请调整筛选条件。' : '未提供包结果；请查看运行诊断或读取原始日志。', 'report-empty'));
      visible.forEach(pkg => {
        const details = node('details', null, 'report-package');
        details.dataset.status = text(pkg.status);
        details.open = rank(pkg.status) <= 1;
        const summary = node('summary');
        summary.append(node('span', pkg.import_path, 'report-name'), node('span', labels[pkg.status] || text(pkg.status), 'report-status'), node('span', elapsed(pkg.elapsed_ms), 'report-time'));
        details.append(summary);
        const coverage = pkg.status === 'no_tests' ? '未发现测试 · 覆盖率不适用' : typeof pkg.coverage_pct === 'number' && Number.isFinite(pkg.coverage_pct) && pkg.coverage_pct >= 0 && pkg.coverage_pct <= 100 ? `覆盖率 ${pkg.coverage_pct}%` : '覆盖率未提供';
        details.append(node('p', coverage, 'report-coverage'));
        appendLocation(details, report, pkg.failure_location);
        appendTests(details, report, pkg.tests);
        appendOutput(details, pkg.output, pkg.output_truncated, '包诊断');
        packages.append(details);
      });
    }
    filter.addEventListener('input', refresh);
    status.addEventListener('change', refresh);
    sort.addEventListener('change', refresh);
    refresh();
  }
  return {render};
})();
