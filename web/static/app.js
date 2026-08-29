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
  const showError = (message) => { $('alerts').innerHTML = `<div class="notice">${escapeHTML(message)}</div>`; };
  const escapeHTML = (value) => text(value).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const setState = (id, value, good) => { const el = $(id); el.textContent = value; el.className = good ? 'ok' : 'bad'; };
  async function load() {
    $('alerts').innerHTML = '';
    try {
      const [status, nodes, bbr, hy2, capabilities, tasks, servers] = await Promise.all([
        json('/api/v1/status'), json('/api/v1/nodes'), json('/api/v1/bbr/status'), json('/api/v1/hy2-buffer/status'), json('/api/v1/capabilities'), json('/api/v1/tasks'), json('/api/v1/servers')
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
      renderNodes(list);
      renderTasks(tasks.tasks || []);
      renderServers(servers.servers || []);
    } catch (error) { showError(error.message); }
  }
  function renderNodes(nodes) {
    if (!Array.isArray(nodes) || !nodes.length) { $('nodes').textContent = '暂无节点'; return; }
    $('nodes').innerHTML = `<table><thead><tr><th>ID</th><th>协议</th><th>端口</th><th>状态</th></tr></thead><tbody>${nodes.map((node) => `<tr><td>${escapeHTML(node.id)}</td><td>${escapeHTML(node.protocol)}</td><td>${escapeHTML(node.port)}</td><td>${node.enabled ? '<span class="ok">启用</span>' : '停用'}</td></tr>`).join('')}</tbody></table>`;
  }
  function renderTasks(tasks) {
    if (!tasks.length) { $('tasks').textContent = '暂无任务'; return; }
    $('tasks').innerHTML = `<table><thead><tr><th>时间</th><th>服务器</th><th>操作</th><th>状态</th><th>错误</th></tr></thead><tbody>${tasks.slice(-20).reverse().map((task) => `<tr><td>${escapeHTML(task.created_at)}</td><td>${escapeHTML(task.server_id)}</td><td>${escapeHTML(task.action)}</td><td>${escapeHTML(task.status)}</td><td>${escapeHTML(task.error || '')}</td></tr>`).join('')}</tbody></table>`;
  }
  function renderServers(servers) {
    if (!servers.length) { $('servers').textContent = '暂无服务器'; return; }
    $('servers').innerHTML = `<table><thead><tr><th>名称</th><th>地址</th><th>区域</th><th>状态</th><th>核心</th><th>最近心跳</th></tr></thead><tbody>${servers.map((server) => `<tr><td>${escapeHTML(server.name)}</td><td>${escapeHTML(server.address || server.id)}</td><td>${escapeHTML(server.region)}</td><td>${server.online ? '<span class="ok">在线</span>' : '<span class="bad">离线</span>'}</td><td>${escapeHTML(server.core_version)}</td><td>${escapeHTML(server.last_seen)}</td></tr>`).join('')}</tbody></table>`;
  }
  async function action(name, button) {
    button.disabled = true;
    try {
      const task = await json('/api/v1/servers/local/actions', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ action: name, idempotency_key: `${name}-${Date.now()}` }) });
      showError(`任务已创建：${task.id}`);
      setTimeout(load, 700);
      setTimeout(load, 2500);
    } catch (error) { showError(error.message); } finally { button.disabled = false; }
  }
  document.querySelectorAll('[data-action]').forEach((button) => button.addEventListener('click', () => action(button.dataset.action, button)));
  $('refresh').addEventListener('click', load);
  $('add-server').addEventListener('click', async () => {
    try {
      const enrollment = await json('/api/v1/enrollment', { method: 'POST', headers: { 'X-CSRF-Token': csrf() }, body: '{}' });
      const output = $('join-command');
      output.hidden = false;
      output.textContent = `有效期至：${enrollment.expires_at}\n\n${enrollment.join_command}`;
    } catch (error) { showError(error.message); }
  });
  load();
})();
