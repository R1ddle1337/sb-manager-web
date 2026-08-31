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
  let alertTimer = null;
  const showError = (message, success = false) => { $('alerts').innerHTML = `<div class="notice ${success ? 'success' : 'error'}">${escapeHTML(message)}</div>`; if (alertTimer) clearTimeout(alertTimer); alertTimer = setTimeout(() => { $('alerts').innerHTML = ''; }, success ? 4500 : 8000); };
  const escapeHTML = (value) => text(value).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const formatTime = (value) => { if (!value) return '-'; const date = new Date(value); return Number.isNaN(date.getTime()) ? text(value) : date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }); };
  const actionNames = { 'health.check': '健康检查', 'backup.create': '创建备份', 'bbr.enable': '开启 BBR', 'bbr.disable': '关闭 BBR', 'hy2-buffer.enable': '优化 HY2 UDP', 'hy2-buffer.disable': '关闭 HY2 优化', 'core.update': '更新核心', 'core.rollback': '回滚核心', 'agent.update': '更新 Agent', 'node.add': '添加节点', 'node.set': '修改节点', 'node.enable': '启用节点', 'node.disable': '停用节点', 'node.delete': '删除节点', 'user.add': '添加节点用户', 'service.restart': '重启服务', 'doctor.repair-safe': '安全修复', 'realm.enable': '启用 Realm', 'realm.disable': '停用 Realm' };
  const taskStates = { pending: ['等待中', 'pending'], running: ['执行中', 'running'], success: ['成功', 'success'], failed: ['失败', 'failed'], canceled: ['已取消', 'canceled'] };
  const skipReasons = { offline: '离线', 'server not found': '服务器不存在', 'already current': '已是目标版本', 'local controller is updated separately': '本机控制端单独更新', 'self-update unsupported; run sudo sb-web update once': '需先手动升级一次' };
  const setState = (id, value, good) => { const el = $(id); el.textContent = value; el.className = good ? 'ok' : 'bad'; };
  let loading = false;
  let allTasks = [];
  let lastServers = [];
  let taskFilter = 'all';
  let refreshTimer = null;
  let serverFilter = '';
  let webUpdateBusy = false;
  let latestAgentVersion = '';
  let canManageAgentUpdates = false;
  let currentSession = null;
  async function loadSession() {
    try {
      currentSession = await json('/api/v1/session');
      $('current-user').textContent = currentSession.username || '用户';
      $('current-role').textContent = currentSession.role || 'viewer';
      $('user-avatar').textContent = String(currentSession.username || 'S').slice(0, 1).toUpperCase();
      document.querySelectorAll('[data-open-server]').forEach((button) => { button.hidden = currentSession.role !== 'admin'; });
      document.querySelectorAll('[data-open-node], [data-action]').forEach((button) => { button.disabled = currentSession.role === 'viewer'; });
      if (currentSession.role === 'viewer') {
        document.querySelectorAll('#batch-form input, #batch-form select, #batch-form button, #add-user-form input, #add-user-form button, #realm-form input, #realm-form button, #cert-form input, #cert-form button').forEach((field) => { field.disabled = true; });
      }
    } catch (_) {
      $('current-user').textContent = '当前用户';
      $('current-role').textContent = '-';
    }
  }
  async function load() {
    if (loading) return;
    loading = true;
    try {
      const results = await Promise.allSettled([
        json('/api/v1/status'), json('/api/v1/nodes'), json('/api/v1/bbr/status'), json('/api/v1/hy2-buffer/status'), json('/api/v1/capabilities'), json('/api/v1/tasks'), json('/api/v1/servers'), json('/api/v1/realm')
      ]);
      const value = (index, fallback) => results[index].status === 'fulfilled' ? results[index].value : fallback;
      const status = value(0, {});
      const nodes = value(1, { nodes: [] });
      const bbr = value(2, {});
      const hy2 = value(3, {});
      const capabilities = value(4, {});
      const tasks = value(5, { tasks: [] });
      const servers = value(6, { servers: [] });
      const realm = value(7, {});
      const services = status.services || status.service || {};
      const serviceOK = services.sing_box?.active ?? services.singbox?.active ?? false;
      setState('service-state', results[0].status === 'rejected' ? '读取失败' : (serviceOK ? '运行中' : '待命/停止'), serviceOK);
      const list = nodes.nodes || nodes;
      $('node-count').textContent = results[1].status === 'rejected' ? '读取失败' : (Array.isArray(list) ? `${list.length} 个` : '-');
      setState('bbr-state', results[2].status === 'rejected' ? '读取失败' : (bbr.enabled ? '已启用' : '未启用'), bbr.enabled === true);
      setState('hy2-state', results[3].status === 'rejected' ? '读取失败' : (hy2.enabled ? '已生效' : '未生效'), hy2.enabled === true);
      $('core-version').textContent = `核心：${capabilities.version || '未知'}`;
      $('status-output').textContent = JSON.stringify(status, null, 2);
      renderRealm(realm);
      syncCapabilityFields(capabilities);
      renderNodes(list);
      allTasks = tasks.tasks || [];
      renderTasks(allTasks);
      lastServers = servers.servers || [];
      renderServers(lastServers);
      const failures = results.filter((result) => result.status === 'rejected');
      if (failures.length) showError(`部分状态读取失败（${failures.length} 项），其余功能仍可继续使用。`);
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
  async function loadWebUpdate() {
    const current = $('web-current-version');
    const status = $('web-update-status');
    const updateButton = $('web-update-button');
    try {
      const info = await json('/api/v1/web/update');
      current.textContent = info.current || '未知';
      latestAgentVersion = info.latest || '';
      canManageAgentUpdates = info.can_update === true;
      $('agent-update-option').hidden = !(canManageAgentUpdates && latestAgentVersion);
      renderServers(lastServers);
      updateButton.hidden = !(info.can_update && info.update_supported && info.update_available);
      if (!info.update_supported) {
        status.textContent = '特权更新服务不可用，请在服务器终端执行 sudo sb-web update。';
      } else if (info.update_available) {
        status.textContent = `发现新版本 ${info.latest}，点击“立即更新”后面板会自动重启。`;
      } else {
        status.textContent = `当前已是最新版本（${info.current}）。`;
      }
    } catch (error) {
      current.textContent = '未知';
      latestAgentVersion = '';
      canManageAgentUpdates = false;
      $('agent-update-option').hidden = true;
      renderServers(lastServers);
      updateButton.hidden = true;
      status.textContent = `检查更新失败：${error.message}`;
    }
  }
  function renderRealm(realm) {
    const value = realm.realm || realm;
    const enabled = value && value.enabled === true;
    setState('realm-state', enabled ? '已启用' : '未启用', enabled);
    $('realm-output').textContent = JSON.stringify(value || {}, null, 2);
    $('realm-disable').disabled = !enabled || currentSession?.role === 'viewer';
  }
  function renderNodes(nodes) {
    if (!Array.isArray(nodes) || !nodes.length) { $('nodes-list').textContent = '暂无节点'; return; }
    const readOnly = currentSession?.role === 'viewer';
    $('nodes-list').innerHTML = `<table><thead><tr><th>ID</th><th>协议</th><th>端口</th><th>状态</th><th>操作</th></tr></thead><tbody>${nodes.map((node) => `<tr><td><a href="/servers/local/nodes/${encodeURIComponent(node.id)}"><code>${escapeHTML(node.id)}</code></a></td><td>${escapeHTML(node.protocol)}</td><td>${escapeHTML(node.port)}</td><td>${node.enabled ? '<span class="status-badge success">启用</span>' : '<span class="status-badge canceled">停用</span>'}</td><td><button class="secondary node-action" data-node-id="${escapeHTML(node.id)}" data-node-action="${node.enabled ? 'disable' : 'enable'}" ${readOnly ? 'disabled' : ''}>${node.enabled ? '停用' : '启用'}</button> <button class="secondary share-action" data-node-id="${escapeHTML(node.id)}" data-qr="0">分享</button> <button class="secondary share-action" data-node-id="${escapeHTML(node.id)}" data-qr="1">QR</button> <button class="secondary delete-action" data-node-id="${escapeHTML(node.id)}" ${readOnly ? 'disabled' : ''}>删除</button></td></tr>`).join('')}</tbody></table>`;
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
    $('tasks-list').innerHTML = `<table><thead><tr><th>创建时间</th><th>目标</th><th>动作</th><th>状态</th><th>次数</th><th>结果摘要</th><th>操作</th></tr></thead><tbody>${filtered.slice(-20).reverse().map((task) => { const state = taskStates[task.status] || [task.status, '']; const summary = task.error || (task.status === 'success' ? '执行完成' : ''); const canManage = currentSession?.role !== 'viewer' && (task.action !== 'agent.update' || currentSession?.role === 'admin'); return `<tr><td>${escapeHTML(formatTime(task.created_at))}</td><td><code>${escapeHTML(task.server_id)}</code></td><td>${escapeHTML(actionNames[task.action] || task.action)}</td><td><span class="status-badge ${state[1]}">${escapeHTML(state[0])}</span></td><td>第 ${Number(task.attempt || 0) + 1} 次</td><td class="result-summary" title="${escapeHTML(summary)}">${escapeHTML(summary)}</td><td><button class="task-detail" data-task-id="${escapeHTML(task.id)}">详情</button>${canManage && (task.status === 'pending' || task.status === 'running') ? ` <button class="task-action" data-task-id="${escapeHTML(task.id)}" data-task-operation="cancel">取消</button>` : ''}${canManage && (task.status === 'failed' || task.status === 'canceled') ? ` <button class="task-action" data-task-id="${escapeHTML(task.id)}" data-task-operation="retry">重试</button>` : ''}</td></tr>`; }).join('')}</tbody></table>`;
    document.querySelectorAll('.task-action').forEach((button) => button.addEventListener('click', () => taskAction(button)));
    document.querySelectorAll('.task-detail').forEach((button) => button.addEventListener('click', () => showTaskDetail(button.dataset.taskId)));
  }
  function renderServers(servers) {
    const filtered = serverFilter ? servers.filter((server) => `${server.id} ${server.name} ${server.address} ${server.region} ${server.agent_version}`.toLowerCase().includes(serverFilter.toLowerCase())) : servers;
    if (!filtered.length) { $('servers-list').textContent = serverFilter ? '没有匹配的服务器' : '暂无服务器'; return; }
    $('servers-list').innerHTML = `<table><thead><tr><th>选择</th><th>名称</th><th>地址</th><th>区域</th><th>状态</th><th>核心</th><th>Agent</th><th>最近心跳</th><th>操作</th></tr></thead><tbody>${filtered.map((server) => {
      const remote = server.id !== 'local';
      const features = Array.isArray(server.agent_features) ? server.agent_features : [];
      const supportsUpdate = features.includes('self_update_v1');
      const outdated = remote && latestAgentVersion && server.agent_version !== latestAgentVersion;
      let updateControl = '-';
      if (remote && !supportsUpdate) updateControl = '<span class="muted">需手动升级一次</span>';
      if (remote && supportsUpdate && !latestAgentVersion) updateControl = '<span class="muted">等待版本检查</span>';
      if (remote && supportsUpdate && latestAgentVersion && !outdated) updateControl = '<span class="ok">已是最新</span>';
      if (remote && supportsUpdate && outdated && canManageAgentUpdates) updateControl = `<button class="agent-update" data-server-id="${escapeHTML(server.id)}" ${server.online ? '' : 'disabled'}>更新至 ${escapeHTML(latestAgentVersion)}</button>`;
      if (remote && supportsUpdate && outdated && !canManageAgentUpdates) updateControl = '<span class="muted">有新版本</span>';
      const selectionDisabled = !server.online || currentSession?.role === 'viewer';
      return `<tr><td><input class="server-select" type="checkbox" value="${escapeHTML(server.id)}" ${selectionDisabled ? 'disabled' : ''}></td><td><a href="/servers/${encodeURIComponent(server.id)}"><strong>${escapeHTML(server.name)}</strong></a></td><td>${escapeHTML(server.address || server.id)}</td><td>${escapeHTML(server.region)}</td><td>${server.online ? '<span class="status-badge success">在线</span>' : '<span class="status-badge failed">离线</span>'}</td><td>${escapeHTML(server.core_version)}</td><td>${escapeHTML(server.agent_version || (remote ? '未知' : '-'))}</td><td>${escapeHTML(formatTime(server.last_seen))}</td><td>${updateControl}</td></tr>`;
    }).join('')}</tbody></table>`;
    document.querySelectorAll('.agent-update').forEach((button) => button.addEventListener('click', () => updateAgent(button)));
    document.querySelectorAll('.server-select').forEach((checkbox) => checkbox.addEventListener('change', updateSelectionCount));
    updateSelectionCount();
  }
  function updateSelectionCount() {
    const selected = document.querySelectorAll('.server-select:checked').length;
    $('server-selection-count').textContent = selected ? `已选择 ${selected} 台服务器` : '尚未选择服务器';
  }
  async function updateAgent(button) {
    const serverID = button.dataset.serverId;
    if (!latestAgentVersion || !window.confirm(`确认将 ${serverID} 的 Agent 更新到 ${latestAgentVersion}？更新后 Agent 会自动重启并重新连接。`)) return;
    button.disabled = true;
    try {
      const task = await json(`/api/v1/servers/${encodeURIComponent(serverID)}/actions`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ action: 'agent.update', args: { version: latestAgentVersion }, idempotency_key: `agent.update-${serverID}-${latestAgentVersion}-${Date.now()}` }) });
      showError(`Agent 更新任务已创建：${task.id}`, true);
      setTimeout(load, 1000);
    } catch (error) { showError(error.message); } finally { button.disabled = false; }
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
      if ($('node-dialog').open) $('node-dialog').close();
      activateWorkspace('nodes');
      showError('节点添加任务已创建。', true);
      setTimeout(load, 900);
    } catch (error) { showError(error.message); }
  });
  document.querySelectorAll('[data-action]').forEach((button) => button.addEventListener('click', () => {
    if (button.dataset.confirm && !window.confirm(button.dataset.confirm)) return;
    action(button.dataset.action, button);
  }));
  $('refresh').addEventListener('click', load);
  $('trend-refresh').addEventListener('click', async () => { try { await json('/api/v1/metrics', { headers: { Accept: 'text/plain' } }); await loadTrend(); } catch (error) { showError(error.message); } });
  $('server-refresh').addEventListener('click', load);
  $('web-update-check').addEventListener('click', () => loadWebUpdate());
  $('web-update-button').addEventListener('click', async () => {
    if (webUpdateBusy || !window.confirm('确认更新 WebUI？服务会短暂重启，配置、数据库和证书会保留。')) return;
    webUpdateBusy = true;
    const button = $('web-update-button');
    const status = $('web-update-status');
    button.disabled = true;
    status.textContent = '正在启动更新，面板即将重启…';
    try {
      await json('/api/v1/web/update', { method: 'POST', headers: { 'X-CSRF-Token': csrf() }, body: '{}' });
      status.textContent = '更新已开始。等待服务重启后请刷新页面。';
      setTimeout(loadWebUpdate, 6000);
    } catch (error) {
      status.textContent = `更新启动失败：${error.message}`;
      showError(error.message);
    } finally {
      webUpdateBusy = false;
      button.disabled = false;
    }
  });
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
  document.querySelectorAll('[data-open-server]').forEach((button) => button.addEventListener('click', createEnrollment));
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
  const nodeDialog = $('node-dialog');
  const closeNodeDialog = () => { if (nodeDialog.open) nodeDialog.close(); };
  document.querySelectorAll('[data-open-node]').forEach((button) => button.addEventListener('click', () => {
    if (typeof nodeDialog.showModal === 'function') nodeDialog.showModal();
    else nodeDialog.setAttribute('open', '');
  }));
  $('node-dialog-close').addEventListener('click', closeNodeDialog);
  $('node-dialog-cancel').addEventListener('click', closeNodeDialog);
  $('batch-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    const selected = [...document.querySelectorAll('.server-select:checked')].map((item) => item.value);
    if (!selected.length) { showError('请至少选择一台在线服务器。'); return; }
    const actionName = new FormData(event.target).get('action');
    const strategy = $('batch-strategy').value;
    const percentage = Number($('batch-percentage').value || 100);
    const args = actionName === 'agent.update' ? { version: latestAgentVersion } : {};
    if (actionName === 'agent.update' && (!canManageAgentUpdates || !latestAgentVersion)) { showError('暂时无法获取可用的 Agent 目标版本。'); return; }
    try {
      const preview = await json('/api/v1/batch/preflight', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ server_ids: selected, action: actionName, args }) });
      const eligible = preview.eligible || [];
      const skipped = preview.skipped || [];
      const skippedText = skipped.length ? `，跳过 ${skipped.length} 台（${skipped.map((item) => item.id + ': ' + (skipReasons[item.reason] || item.reason)).join('；')}）` : '';
      if (!eligible.length || !window.confirm(`预检查：${eligible.length} 台可执行${skippedText}。确认继续？`)) return;
      const result = await json('/api/v1/batch/actions', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ server_ids: selected, action: actionName, args, strategy, percentage }) });
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
  const serverSearch = $('server-search');
  serverSearch.addEventListener('input', () => { serverFilter = serverSearch.value.trim(); renderServers(lastServers); });
  const strategySelect = $('batch-strategy');
  const percentageField = $('batch-percentage-field');
  strategySelect.addEventListener('change', () => { percentageField.hidden = strategySelect.value !== 'percentage'; });
  const syncProtocolFields = () => {
    const protocol = protocolSelect.value;
    document.querySelectorAll('[data-protocols]').forEach((field) => {
      const protocols = field.dataset.protocols.split(',');
      field.hidden = !protocols.includes(protocol);
    });
  };
  protocolSelect.addEventListener('change', syncProtocolFields);
  syncProtocolFields();
  $('task-filter').addEventListener('change', (event) => { taskFilter = event.target.value; renderTasks(allTasks); });
  const drawer = document.createElement('aside');
  drawer.id = 'task-drawer';
  drawer.className = 'task-drawer';
  drawer.hidden = true;
  drawer.innerHTML = '<div class="drawer-header"><strong id="task-drawer-title">任务详情</strong><button id="task-drawer-close" class="icon-button" aria-label="关闭详情">×</button></div><pre id="task-drawer-body" class="output"></pre>';
  document.body.appendChild(drawer);
  $('task-drawer-close').addEventListener('click', () => { drawer.hidden = true; });
  $('refresh-interval').addEventListener('change', (event) => { if (refreshTimer) clearInterval(refreshTimer); const seconds = Number(event.target.value); if (seconds > 0) refreshTimer = setInterval(load, seconds * 1000); });
  const navTitles = { overview: '总览', servers: '服务器', nodes: '节点与用户', tasks: '任务记录', system: '系统维护' };
  const closeSidebar = () => { $('sidebar').classList.remove('open'); $('scrim').hidden = true; $('menu-toggle').setAttribute('aria-expanded', 'false'); };
  function activateWorkspace(name, updateHash = true) {
    if (!navTitles[name]) name = 'overview';
    document.querySelectorAll('[data-workspace-panel]').forEach((panel) => { panel.hidden = panel.dataset.workspacePanel !== name; });
    document.querySelectorAll('[data-nav]').forEach((nav) => nav.classList.toggle('active', nav.dataset.nav === name));
    $('page-title').textContent = navTitles[name];
    if (updateHash) history.replaceState(null, '', `#${name}`);
    closeSidebar();
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }
  $('menu-toggle').addEventListener('click', () => { const open = !$('sidebar').classList.contains('open'); $('sidebar').classList.toggle('open', open); $('scrim').hidden = !open; $('menu-toggle').setAttribute('aria-expanded', String(open)); });
  $('scrim').addEventListener('click', closeSidebar);
  document.querySelectorAll('[data-nav]').forEach((item) => item.addEventListener('click', (event) => { event.preventDefault(); activateWorkspace(item.dataset.nav); }));
  document.querySelectorAll('[data-go-workspace]').forEach((button) => button.addEventListener('click', () => activateWorkspace(button.dataset.goWorkspace)));
  window.addEventListener('hashchange', () => activateWorkspace(location.hash.slice(1), false));
  activateWorkspace(location.hash.slice(1) || 'overview', false);
  if ('EventSource' in window) {
    const stream = new EventSource('/api/v1/events');
    stream.addEventListener('tasks', (event) => { try { const payload = JSON.parse(event.data); allTasks = payload.tasks || []; renderTasks(allTasks); } catch (_) { /* reconnect will retry */ } });
  }
  loadSession().finally(() => {
    loadWebUpdate();
    load();
  });
})();
