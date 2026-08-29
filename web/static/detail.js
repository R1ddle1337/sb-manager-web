(() => {
  const $ = (id) => document.getElementById(id);
  const csrf = () => (document.cookie.split('; ').find((v) => v.startsWith('sbweb_csrf=')) || '').split('=')[1] || '';
  const escapeHTML = (value) => String(value == null ? '-' : value).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const json = async (url, options = {}) => {
    const response = await fetch(url, { credentials: 'same-origin', ...options, headers: { Accept: 'application/json', ...(options.headers || {}) } });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error?.message || `HTTP ${response.status}`);
    return data;
  };
  const show = (message, success = false) => { $('alerts').innerHTML = `<div class="notice ${success ? 'success' : 'error'}">${escapeHTML(message)}</div>`; };
  const server = encodeURIComponent(window.SB_SERVER_ID);
  async function loadServer() {
    const [info, status, nodes, caps] = await Promise.all([json(`/api/v1/servers/${server}`), json(`/api/v1/servers/${server}/status`), json(`/api/v1/servers/${server}/nodes`), json(`/api/v1/servers/${server}/capabilities`)]);
    $('server-name').textContent = info.name || info.id;
    $('server-address').textContent = info.address || info.id;
    $('server-state').textContent = info.online ? '在线' : '离线';
    $('server-state').className = info.online ? 'ok' : 'bad';
    $('server-last-seen').textContent = info.last_seen || '-';
    $('server-core').textContent = info.core_version || '-';
    $('server-arch').textContent = info.arch || '-';
    const list = Array.isArray(nodes) ? nodes : (nodes.nodes || []);
    $('server-node-count').textContent = `${list.length} 个`;
    $('capabilities').textContent = JSON.stringify(caps, null, 2);
    $('nodes-list').innerHTML = list.length ? `<table><thead><tr><th>ID</th><th>协议</th><th>端口</th><th>状态</th><th>编辑</th></tr></thead><tbody>${list.map((node) => `<tr><td><code>${escapeHTML(node.id)}</code></td><td>${escapeHTML(node.protocol)}</td><td>${escapeHTML(node.port)}</td><td>${node.enabled ? '<span class="ok">● 启用</span>' : '<span class="bad">● 停用</span>'}</td><td><a href="/servers/${server}/nodes/${encodeURIComponent(node.id)}">编辑 →</a></td></tr>`).join('')}</tbody></table>` : '暂无节点';
    $('nodes-json').href = `/api/v1/servers/${server}/nodes`;
  }
  async function loadNode() {
    const node = await json(`/api/v1/servers/${server}/nodes/${encodeURIComponent(window.SB_NODE_ID)}`);
    const value = node.node || node;
    $('node-title').textContent = value.name || value.id || window.SB_NODE_ID;
    for (const field of ['name', 'port', 'domain']) if ($(field)) $(field).value = value[field] ?? '';
    if ($('address')) $('address').value = value.address ?? value.server_address ?? value.client_address ?? '';
    const metadata = value.metadata || {};
    for (const field of ['remark', 'region', 'purpose', 'line']) if ($(field)) $(field).value = metadata[field] || '';
    if ($('tags')) $('tags').value = Array.isArray(metadata.tags) ? metadata.tags.join(',') : '';
    $('node-output').textContent = JSON.stringify(value, null, 2);
    $('server-link').href = `/servers/${server}`;
    $('share-link').href = `/api/v1/servers/${server}/nodes/${encodeURIComponent(window.SB_NODE_ID)}/share`;
  }
  $('refresh')?.addEventListener('click', () => loadServer().catch((error) => show(error.message)));
  $('node-edit-form')?.addEventListener('submit', async (event) => {
    event.preventDefault();
    const form = new FormData(event.target);
    const args = {};
    for (const [key, value] of form.entries()) if (value !== '') args[key] = key === 'port' ? Number(value) : value;
    const button = event.target.querySelector('button[type="submit"]');
    button.disabled = true;
    try {
      const task = await json(`/api/v1/servers/${server}/nodes/${encodeURIComponent(window.SB_NODE_ID)}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf(), 'Idempotency-Key': `node-set-${window.SB_SERVER_ID}-${window.SB_NODE_ID}-${Date.now()}` }, body: JSON.stringify(args) });
      show(`节点保存任务已创建：${task.id}`, true);
      setTimeout(loadNode, 900);
    } catch (error) { show(error.message); } finally { button.disabled = false; }
  });
  if (window.SB_NODE_ID) loadNode().catch((error) => show(error.message)); else loadServer().catch((error) => show(error.message));
})();
