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
  async function load() {
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
      renderNodes(list);
      renderTasks(tasks.tasks || []);
      renderServers(servers.servers || []);
    } catch (error) { showError(error.message); }
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
    $('nodes-list').innerHTML = `<table><thead><tr><th>ID</th><th>协议</th><th>端口</th><th>状态</th><th>操作</th></tr></thead><tbody>${nodes.map((node) => `<tr><td><a href="/servers/local/nodes/${encodeURIComponent(node.id)}"><code>${escapeHTML(node.id)}</code></a></td><td>${escapeHTML(node.protocol)}</td><td>${escapeHTML(node.port)}</td><td>${node.enabled ? '<span class="ok">● 启用</span>' : '<span class="bad">● 停用</span>'}</td><td><button class="secondary node-action" data-node-id="${escapeHTML(node.id)}" data-node-action="${node.enabled ? 'disable' : 'enable'}">${node.enabled ? '停用' : '启用'}</button> <button class="secondary share-action" data-node-id="${escapeHTML(node.id)}">分享</button> <button class="secondary delete-action" data-node-id="${escapeHTML(node.id)}">删除</button></td></tr>`).join('')}</tbody></table>`;
    document.querySelectorAll('.node-action').forEach((button) => button.addEventListener('click', () => nodeAction(button)));
    document.querySelectorAll('.share-action').forEach((button) => button.addEventListener('click', () => showShare(button.dataset.nodeId)));
    document.querySelectorAll('.delete-action').forEach((button) => button.addEventListener('click', () => { if (window.confirm(`确认删除节点 ${button.dataset.nodeId}？`)) nodeActionWith(button, 'delete'); }));
  }
  function renderTasks(tasks) {
    if (!tasks.length) { $('tasks-list').textContent = '暂无任务'; return; }
    $('tasks-list').innerHTML = `<table><thead><tr><th>时间</th><th>服务器</th><th>操作</th><th>状态</th><th>尝试</th><th>错误</th><th>操作</th></tr></thead><tbody>${tasks.slice(-20).reverse().map((task) => `<tr><td>${escapeHTML(task.created_at)}</td><td>${escapeHTML(task.server_id)}</td><td><code>${escapeHTML(task.action)}</code></td><td>${escapeHTML(task.status)}</td><td>${escapeHTML(task.attempt || 0)}</td><td>${escapeHTML(task.error || '')}</td><td>${task.status === 'pending' || task.status === 'running' ? `<button class="task-action" data-task-id="${escapeHTML(task.id)}" data-task-operation="cancel">取消</button>` : ''}${task.status === 'failed' || task.status === 'canceled' ? ` <button class="task-action" data-task-id="${escapeHTML(task.id)}" data-task-operation="retry">重试</button>` : ''}</td></tr>`).join('')}</tbody></table>`;
    document.querySelectorAll('.task-action').forEach((button) => button.addEventListener('click', () => taskAction(button)));
  }
  function renderServers(servers) {
    if (!servers.length) { $('servers-list').textContent = '暂无服务器'; return; }
    $('servers-list').innerHTML = `<table><thead><tr><th>选择</th><th>名称</th><th>地址</th><th>区域</th><th>状态</th><th>核心</th><th>最近心跳</th></tr></thead><tbody>${servers.map((server) => `<tr><td><input class="server-select" type="checkbox" value="${escapeHTML(server.id)}" ${server.online ? '' : 'disabled'}></td><td><a href="/servers/${encodeURIComponent(server.id)}"><strong>${escapeHTML(server.name)}</strong></a></td><td>${escapeHTML(server.address || server.id)}</td><td>${escapeHTML(server.region)}</td><td>${server.online ? '<span class="ok">● 在线</span>' : '<span class="bad">● 离线</span>'}</td><td>${escapeHTML(server.core_version)}</td><td>${escapeHTML(server.last_seen)}</td></tr>`).join('')}</tbody></table>`;
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
  async function showShare(nodeID) {
    try {
      const result = await json(`/api/v1/servers/local/nodes/${encodeURIComponent(nodeID)}/share`);
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
  $('add-server').addEventListener('click', async () => {
    try {
      const enrollment = await json('/api/v1/enrollment', { method: 'POST', headers: { 'X-CSRF-Token': csrf() }, body: '{}' });
      const output = $('join-command');
      output.hidden = false;
      output.textContent = `有效期至：${enrollment.expires_at}\n\n${enrollment.join_command}`;
    } catch (error) { showError(error.message); }
  });
  $('batch-form').addEventListener('submit', async (event) => {
    event.preventDefault();
    const selected = [...document.querySelectorAll('.server-select:checked')].map((item) => item.value);
    if (!selected.length) { showError('请至少选择一台在线服务器。'); return; }
    const actionName = new FormData(event.target).get('action');
    if (!window.confirm(`确认在 ${selected.length} 台服务器执行 ${actionName}？`)) return;
    try {
      const strategy = $('batch-strategy').value;
      const percentage = Number($('batch-percentage').value || 100);
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
  const navTitles = { overview: '总览', servers: '服务器', nodes: '节点与用户', 'quick-actions': '快捷操作', realm: 'Hysteria Realm', certificates: '证书', tasks: '任务与审计' };
  const closeSidebar = () => { $('sidebar').classList.remove('open'); $('scrim').hidden = true; $('menu-toggle').setAttribute('aria-expanded', 'false'); };
  $('menu-toggle').addEventListener('click', () => { const open = !$('sidebar').classList.contains('open'); $('sidebar').classList.toggle('open', open); $('scrim').hidden = !open; $('menu-toggle').setAttribute('aria-expanded', String(open)); });
  $('scrim').addEventListener('click', closeSidebar);
  document.querySelectorAll('[data-nav]').forEach((item) => item.addEventListener('click', () => { document.querySelectorAll('[data-nav]').forEach((nav) => nav.classList.toggle('active', nav === item)); $('page-title').textContent = navTitles[item.dataset.nav] || '总览'; closeSidebar(); }));
  load();
})();
