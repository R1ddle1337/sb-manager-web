(() => {
  const $ = (id) => document.getElementById(id);
  const csrf = () => (document.cookie.split('; ').find((v) => v.startsWith('sbweb_csrf=')) || '').split('=')[1] || '';
  const json = async (url, options = {}) => {
    const response = await fetch(url, { credentials: 'same-origin', ...options, headers: { 'Accept': 'application/json', ...(options.headers || {}) } });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error?.message || `HTTP ${response.status}`);
    return data;
  };
  const text = (value) => value == null ? '-' : String(value);
  const showError = (message, success = false) => { $('alerts').innerHTML = `<div class="notice ${success ? 'success' : 'error'}">${escapeHTML(message)}</div>`; };
  const escapeHTML = (value) => text(value).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const setState = (id, value, good) => { const el = $(id); el.textContent = value; el.className = good ? 'ok' : 'bad'; };
  let loading = false;
  let allTasks = [];
  let lastServers = [];
  let taskFilter = 'all';
  let refreshTimer = null;
  let serverFilter = '';
  async function load() {
    if (loading) return;
    loading = true;
    $('alerts').innerHTML = '';
    try {
      const [status, nodes, bbr, hy2, capabilities, tasks, servers, realm] = await Promise.all([
        json('/api/v1/status'), json('/api/v1/nodes'), json('/api/v1/bbr/status'), json('/api/v1/hy2-buffer/status'), json('/api/v1/capabilities'), json('/api/v1/tasks'), json('/api/v1/servers'), json('/api/v1/realm')
      ]);
      const services = status.services || status.service || {};
      const serviceOK = services.sing_box?.active ?? services.singbox?.active ?? false;
      setState('service-state', serviceOK ? '运行中' : '待命/停止', serviceOK);
      const list = nodes.nodes || nodes;
      $('node-count').textContent = Array.isArray(list) ? `${list.length} 个` : '-';
      setState('bbr-state', bbr.enabled ? '已启用' : '未启用', bbr.enabled);
      setState('hy2-state', hy2.enabled ? '已生效' : '未生效', hy2.enabled);
      $('core-version').textContent = `核心：${capabilities.version || '未知'}`;
      $('status-output').textContent = JSON.stringify(status, null, 2);
      renderRealm(realm);
      syncCapabilityFields(capabilities);
      renderNodes(list);
      allTasks = tasks.tasks || [];
      renderTasks(allTasks);
      lastServers = servers.servers || [];
      renderServers(lastServers);
      loadTrend();
    } catch (error) { showError(error.message); } finally { loading = false; }
  }
  async function loadTrend() {
    try {
      const result = await json('/api/v1/metrics/history?hours=24');
      const samples = result.metrics || [];
      if (!samples.length) { $('metric-trend').textContent = '暂无历史样本，点击“采样并刷新”开始记录。'; return; }
      const recent = samples.slice(0, 24).reverse();
      const values = recent.map((sample) => Number(sample.summary?.nodes || 0));
      const max = Math.max(1, ...values);
      $('metric-trend').innerHTML = `<div class="trend-legend"><span>节点数量</span><strong>${values[values.length - 1]}</strong></div><div class="trend-bars">${values.map((value, index) => `<span title="${escapeHTML(recent[index].recorded_at)}: ${value}" style="height:${Math.max(8, Math.round(value / max * 100))}%"></span>`).join('')}</div>`;
    } catch (error) { $('metric-trend').textContent = error.message; }
  }
  function renderRealm(realm) {
    const value = realm.realm || realm;
    const enabled = value && value.enabled === true;
    setState('realm-state', enabled ? '已启用' : '未启用', enabled);
    $('realm-output').textContent = JSON.stringify(value || {}, null, 2);
    $('realm-disable').disabled = !enabled;
  }
  function renderNodes(nodes) {
    if (!Array.isArray(nodes) || !nodes.length) { $('nodes-list').textContent = '暂无节点'; return; }
    $('nodes-list').innerHTML = `<table><thead><tr><th>ID</th><th>协议</th><th>端口</th><th>状态</th><th>操作</th></tr></thead><tbody>${nodes.map((node) => `<tr><td><a href="/servers/local/nodes/${encodeURIComponent(node.id)}"><code>${escapeHTML(node.id)}</code></a></td><td>${escapeHTML(node.protocol)}</td><td>${escapeHTML(node.port)}</td><td>${node.enabled ? '<span class="ok">启用</span>' : '<span class="bad">停用</span>'}</td><td><button class="secondary node-action" data-node-id="${escapeHTML(node.id)}" data-node-action="${node.enabled ? 'disable' : 'enable'}">${node.enabled ? '停用' : '启用'}</button> <button class="secondary share-action" data-node-id="${escapeHTML(node.id)}" data-qr="0">分享</button> <button class="secondary share-action" data-node-id="${escapeHTML(node.id)}" data-qr="1">QR</button> <button class="secondary delete-action" data-node-id="${escapeHTML(node.id)}">删除</button></td></tr>`).join('')}</tbody></table>`;
    document.querySelectorAll('.node-action').forEach((button) => button.addEventListener('click', () => nodeAction(button)));
    document.querySelectorAll('.share-action').forEach((button) => button.addEventListener('click', () => showShare(button.dataset.nodeId, button.dataset.qr === '1')));
    document.querySelectorAll('.delete-action').forEach((button) => button.addEventListener('click', () => { if (window.confirm(`确认删除节点 ${button.dataset.nodeId}？`)) nodeActionWith(button, 'delete'); }));
  }
  function renderTasks(tasks) {
    const filtered = taskFilter === 'all' ? tasks : tasks.filter((task) => task.status === taskFilter);
    const completed = tasks.filter((task) => ['success', 'failed', 'canceled'].includes(task.status)).length;
    const failed = tasks.filter((task) => task.status === 'failed').length;
    $('batch-progress').textContent = tasks.length ? `最近 ${tasks.length} 项 · 已完成 ${completed} · 失败 ${failed}` : '';
    if (!filtered.length) { $('tasks-list').textContent = '暂无匹配任务'; return; }
    $('tasks-list').innerHTML = `<table><thead><tr><th>时间</th><th>服务器</th><th>操作</th><th>状态</th><th>尝试</th><th>错误</th><th>操作</th></tr></thead><tbody>${filtered.slice(-20).reverse().map((task) => `<tr><td>${escapeHTML(task.created_at)}</td><td>${escapeHTML(task.server_id)}</td><td><code>${escapeHTML(task.action)}</code></td><td>${escapeHTML(task.status)}</td><td>${escapeHTML(task.attempt || 0)}</td><td>${escapeHTML(task.error || '')}</td><td><button class="task-detail" data-task-id="${escapeHTML(task.id)}">详情</button>${task.status === 'pending' || task.status === 'running' ? ` <button class="task-action" data-task-id="${escapeHTML(task.id)}" data-task-operation="cancel">取消</button>` : ''}${task.status === 'failed' || task.status === 'canceled' ? ` <button class="task-action" data-task-id="${escapeHTML(task.id)}" data-task-operation="retry">重试</button>` : ''}</td></tr>`).join('')}</tbody></table>`;
    document.querySelectorAll('.task-action').forEach((button) => button.addEventListener('click', () => taskAction(button)));
    document.querySelectorAll('.task-detail').forEach((button) => button.addEventListener('click', () => showTaskDetail(button.dataset.taskId)));
  }
  function renderServers(servers) {
    const filtered = serverFilter ? servers.filter((server) => `${server.id} ${server.name} ${server.address} ${server.region}`.toLowerCase().includes(serverFilter.toLowerCase())) : servers;
    if (!filtered.length) { $('servers-list').textContent = serverFilter ? '没有匹配的服务器' : '暂无服务器'; return; }
    $('servers-list').innerHTML = `<table><thead><tr><th>选择</th><th>名称</th><th>地址</th><th>区域</th><th>状态</th><th>核心</th><th>最近心跳</th></tr></thead><tbody>${filtered.map((server) => `<tr><td><input class="server-select" type="checkbox" value="${escapeHTML(server.id)}" ${server.online ? '' : 'disabled'}></td><td><a href="/servers/${encodeURIComponent(server.id)}"><strong>${escapeHTML(server.name)}</strong></a></td><td>${escapeHTML(server.address || server.id)}</td><td>${escapeHTML(server.region)}</td><td>${server.online ? '<span class="ok">在线</span>' : '<span class="bad">离线</span>'}</td><td>${escapeHTML(server.core_version)}</td><td>${escapeHTML(server.last_seen)}</td></tr>`).join('')}</tbody></table>`;
  }
  async function action(name, button) {
    button.disabled = true;
    try {
      const task = await json('/api/v1/servers/local/actions', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ action: name, idempotency_key: `${name}-${Date.now()}` }) });
      showError(`任务已创建：${task.id}`, true);
      setTimeout(load, 700);
      setTimeout(load, 2500);
    } catch (error) { showError(error.message); } finally { button.disabled = false; }
  }
  async function nodeAction(button) {
    return nodeActionWith(button, button.dataset.nodeAction);
  }
  async function nodeActionWith(button, operation) {
    button.disabled = true;
    try {
      await json(`/api/v1/servers/local/nodes/${encodeURIComponent(button.dataset.nodeId)}/${operation}`, { method: 'POST', headers: { 'X-CSRF-Token': csrf() }, body: '{}' });
      showError(`节点 ${button.dataset.nodeId} 操作已提交。`, true);
      setTimeout(load, 900);
    } catch (error) { showError(error.message); } finally { button.disabled = false; }
  }
  async function showShare(nodeID, qr = false) {
    try {
      const result = await json(`/api/v1/servers/local/nodes/${encodeURIComponent(nodeID)}/share${qr ? '?qr=1' : ''}`);
      const output = $('share-output');
      output.hidden = false;
      output.textContent = result.share;
    } catch (error) { showError(error.message); }
  }
  async function taskAction(button) {
    const operation = button.dataset.taskOperation;
    if (operation === 'cancel' && !window.confirm('确认取消这个任务？')) return;
    if (operation === 'retry' && !window.confirm('确认使用当前服务器状态重试这个任务？')) return;
    button.disabled = true;
    try {
      await json(`/api/v1/tasks/${encodeURIComponent(button.dataset.taskId)}/${operation}`, { method: 'POST', headers: { 'X-CSRF-Token': csrf(), 'Idempotency-Key': `${operation}-${button.dataset.taskId}-${Date.now()}` }, body: '{}' });
      showError(operation === 'cancel' ? '任务已取消。' : '重试任务已创建。', true);
      setTimeout(load, 500);
    } catch (error) { showError(error.message); } finally { button.disabled = false; }
  }
  function showTaskDetail(taskID) {
    const task = allTasks.find((candidate) => candidate.id === taskID);
    if (!task) return;
    $('task-drawer-title').textContent = `${task.action} · ${task.server_id}`;
    $('task-drawer-body').textContent = JSON.stringify({ status: task.status, attempt: task.attempt || 0, error: task.error || '', output: task.output || '' }, null, 2);
    $('task-drawer').hidden = false;
  }
  function syncCapabilityFields(capabilities) {
    const version = String(capabilities.version || '');
    const supported = /^1\.(1[4-9]|[2-9][0-9])(?:\.|$)/.test(version) || version.includes('dev');
    document.querySelectorAll('option[value="gecko"]').forEach((option) => { option.disabled = !supported; });
    document.querySelectorAll('[data-14-only]').forEach((field) => { field.title = supported ? '当前核心支持' : '当前核心可能不支持，请先更新 sing-box'; });
  }
  $('add-node-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    const payload = {};
    for (const field of event.target.elements) {
      if (!field.name || field.disabled) continue;
      const group = field.closest('[data-protocols]');
      if (group && group.hidden) continue;
      if (field.type === 'checkbox') {
        payload[field.name] = field.checked;
      } else if (field.value !== '') {
        payload[field.name] = field.type === 'number' ? Number(field.value) : field.value;
      }
    }
    try {
      await json('/api/v1/servers/local/nodes', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify(payload) });
      event.target.reset();
      syncProtocolFields();
      showError('节点添加任务已创建。', true);
      setTimeout(load, 900);
    } catch (error) { showError(error.message); }
  });
  document.querySelectorAll('[data-action]').forEach((button) => button.addEventListener('click', () => action(button.dataset.action, button)));
  $('refresh').addEventListener('click', load);
  $('trend-refresh').addEventListener('click', async () => { try { await json('/api/v1/metrics', { headers: { Accept: 'text/plain' } }); await loadTrend(); } catch (error) { showError(error.message); } });
  $('server-refresh').addEventListener('click', load);
  $('realm-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    const form = new FormData(event.target);
    const args = { port: Number(form.get('port')), public_url: form.get('public_url'), listen: form.get('listen'), tls_domain: form.get('tls_domain'), max_realms: Number(form.get('max_realms') || 0) };
    const button = event.target.querySelector('button[type="submit"]');
    button.disabled = true;
    try {
      const task = await json('/api/v1/servers/local/actions', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ action: 'realm.enable', args, idempotency_key: `realm.enable-${Date.now()}` }) });
      showError(`Realm 任务已创建：${task.id}`, true);
      setTimeout(load, 900);
    } catch (error) { showError(error.message); } finally { button.disabled = false; }
  });
  const serverDialog = $('server-dialog');
  const serverDialogStatus = $('server-dialog-status');
  const joinCommand = $('join-command');
  const copyJoinCommand = $('copy-join-command');
  const openServerDialog = () => {
    serverDialogStatus.textContent = '正在创建加入命令…';
    serverDialogStatus.className = 'dialog-status';
    joinCommand.hidden = true;
    copyJoinCommand.hidden = true;
    $('renew-enrollment').hidden = true;
    if (!serverDialog.open) {
      if (typeof serverDialog.showModal === 'function') serverDialog.showModal();
      else serverDialog.setAttribute('open', '');
    }
  };
  const createEnrollment = async () => {
    openServerDialog();
    try {
      const enrollment = await json('/api/v1/enrollment', { method: 'POST', headers: { 'X-CSRF-Token': csrf() }, body: '{}' });
      joinCommand.hidden = false;
      joinCommand.textContent = `有效期至：${enrollment.expires_at}\n\n${enrollment.join_command}`;
      copyJoinCommand.hidden = false;
      $('renew-enrollment').hidden = false;
      serverDialogStatus.textContent = '加入命令已生成，请复制到目标服务器终端执行。';
      serverDialogStatus.className = 'dialog-status success';
    } catch (error) {
      serverDialogStatus.textContent = `生成失败：${error.message}`;
      serverDialogStatus.className = 'dialog-status error';
      showError(error.message);
    }
  };
  $('add-server').addEventListener('click', createEnrollment);
  $('server-dialog-close').addEventListener('click', () => {
    if (typeof serverDialog.close === 'function') serverDialog.close();
    else serverDialog.removeAttribute('open');
  });
  $('renew-enrollment').addEventListener('click', createEnrollment);
  copyJoinCommand.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(joinCommand.textContent);
      copyJoinCommand.textContent = '已复制';
      setTimeout(() => { copyJoinCommand.textContent = '复制命令'; }, 1600);
    } catch (_) {
      showError('浏览器不允许自动复制，请手动选择命令。');
    }
  });
  $('batch-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    const selected = [...document.querySelectorAll('.server-select:checked')].map((item) => item.value);
    if (!selected.length) { showError('请至少选择一台在线服务器。'); return; }
    const actionName = new FormData(event.target).get('action');
    const strategy = $('batch-strategy').value;
    const percentage = Number($('batch-percentage').value || 100);
    try {
      const preview = await json('/api/v1/batch/preflight', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ server_ids: selected, action: actionName, args: {} }) });
      const eligible = preview.eligible || [];
      const skipped = preview.skipped || [];
      const skippedText = skipped.length ? `，跳过 ${skipped.length} 台（${skipped.map((item) => item.id + ': ' + item.reason).join('；')}）` : '';
      if (!eligible.length || !window.confirm(`预检查：${eligible.length} 台可执行${skippedText}。确认继续？`)) return;
      const result = await json('/api/v1/batch/actions', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ server_ids: selected, action: actionName, args: {}, strategy, percentage }) });
      showError(`批量任务 ${result.batch_id} 已创建，共 ${result.tasks.length} 项。`, true);
      setTimeout(load, 1200);
    } catch (error) { showError(error.message); }
  });
  $('add-user-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    const form = new FormData(event.target);
    const nodeID = form.get('node_id');
    try {
      await json(`/api/v1/servers/local/nodes/${encodeURIComponent(nodeID)}/users`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ user_id: form.get('user_id'), name: form.get('name') }) });
      event.target.reset();
      showError('用户添加任务已创建。', true);
    } catch (error) { showError(error.message); }
  });
  $('cert-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    const form = new FormData(event.target);
    try {
      await json('/api/v1/servers/local/certificates', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ domain: form.get('domain'), email: form.get('email') }) });
      showError('证书任务已创建。', true);
    } catch (error) { showError(error.message); }
  });
  $('password-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    const form = new FormData(event.target);
    try {
      await json('/api/v1/password', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ current_password: form.get('current'), new_password: form.get('next') }) });
      window.location.href = '/login';
    } catch (error) { showError(error.message); }
  });
  const protocolSelect = document.querySelector('#add-node-form select[name="protocol"]');
  const serverSearch = document.createElement('input');
  serverSearch.type = 'search';
  serverSearch.placeholder = '搜索名称 / 地址 / 地区';
  serverSearch.className = 'server-search';
  $('servers').querySelector('.panel-header').appendChild(serverSearch);
  serverSearch.addEventListener('input', () => { serverFilter = serverSearch.value.trim(); renderServers(lastServers); });
  const strategySelect = document.createElement('select');
  strategySelect.id = 'batch-strategy';
  strategySelect.setAttribute('aria-label', '发布策略');
  strategySelect.innerHTML = '<option value="all">全部服务器</option><option value="canary">灰度：先执行 1 台</option><option value="percentage">灰度：按百分比</option>';
  const percentageInput = document.createElement('input');
  percentageInput.id = 'batch-percentage';
  percentageInput.type = 'number';
  percentageInput.min = '1';
  percentageInput.max = '100';
  percentageInput.value = '25';
  percentageInput.title = '灰度百分比';
  percentageInput.hidden = true;
  $('batch-form').insertBefore(strategySelect, $('batch-form').firstChild);
  $('batch-form').insertBefore(percentageInput, $('batch-form').lastElementChild);
  strategySelect.addEventListener('change', () => { percentageInput.hidden = strategySelect.value !== 'percentage'; });
  const syncProtocolFields = () => {
    const protocol = protocolSelect.value;
    document.querySelectorAll('[data-protocols]').forEach((field) => {
      const protocols = field.dataset.protocols.split(',');
      field.hidden = !protocols.includes(protocol);
    });
  };
  protocolSelect.addEventListener('change', syncProtocolFields);
  syncProtocolFields();
  const taskTools = document.createElement('div');
  taskTools.className = 'task-tools';
  taskTools.innerHTML = '<select id="task-filter" aria-label="任务状态筛选"><option value="all">全部状态</option><option value="pending">等待中</option><option value="running">执行中</option><option value="success">成功</option><option value="failed">失败</option><option value="canceled">已取消</option></select><span id="batch-progress" class="muted"></span>';
  $('tasks').querySelector('.panel-header').appendChild(taskTools);
  $('task-filter').addEventListener('change', (event) => { taskFilter = event.target.value; renderTasks(allTasks); });
  const drawer = document.createElement('aside');
  drawer.id = 'task-drawer';
  drawer.className = 'task-drawer';
  drawer.hidden = true;
  drawer.innerHTML = '<div class="drawer-header"><strong id="task-drawer-title">任务详情</strong><button id="task-drawer-close" class="icon-button" aria-label="关闭详情">×</button></div><pre id="task-drawer-body" class="output"></pre>';
  document.body.appendChild(drawer);
  $('task-drawer-close').addEventListener('click', () => { drawer.hidden = true; });
  const topbarTools = document.createElement('div');
  topbarTools.className = 'topbar-tools';
  topbarTools.innerHTML = '<select id="refresh-interval" title="自动刷新间隔"><option value="0">手动刷新</option><option value="30">30 秒</option><option value="60">60 秒</option></select>';
  $('refresh').parentElement.insertBefore(topbarTools, $('refresh'));
  document.documentElement.dataset.theme = 'light';
  $('refresh-interval').addEventListener('change', (event) => { if (refreshTimer) clearInterval(refreshTimer); const seconds = Number(event.target.value); if (seconds > 0) refreshTimer = setInterval(load, seconds * 1000); });
  const navTitles = { overview: '总览', servers: '服务器', nodes: '节点与用户', 'quick-actions': '快捷操作', realm: 'Hysteria Realm', certificates: '节点证书', tasks: '任务与审计' };
  const closeSidebar = () => { $('sidebar').classList.remove('open'); $('scrim').hidden = true; $('menu-toggle').setAttribute('aria-expanded', 'false'); };
  $('menu-toggle').addEventListener('click', () => { const open = !$('sidebar').classList.contains('open'); $('sidebar').classList.toggle('open', open); $('scrim').hidden = !open; $('menu-toggle').setAttribute('aria-expanded', String(open)); });
  $('scrim').addEventListener('click', closeSidebar);
  document.querySelectorAll('[data-nav]').forEach((item) => item.addEventListener('click', () => { document.querySelectorAll('[data-nav]').forEach((nav) => nav.classList.toggle('active', nav === item)); $('page-title').textContent = navTitles[item.dataset.nav] || '总览'; closeSidebar(); }));
  if ('EventSource' in window) {
    const stream = new EventSource('/api/v1/events');
    stream.addEventListener('tasks', (event) => { try { const payload = JSON.parse(event.data); allTasks = payload.tasks || []; renderTasks(allTasks); } catch (_) { /* reconnect will retry */ } });
  }
  load();
})();
