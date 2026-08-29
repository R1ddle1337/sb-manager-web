(() => {
  const $ = (id) => document.getElementById(id);
  const csrf = () => (document.cookie.split('; ').find((v) => v.startsWith('sbweb_csrf=')) || '').split('=')[1] || '';
  const esc = (value) => String(value == null ? '-' : value).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const json = async (url, options = {}) => { const response = await fetch(url, { credentials: 'same-origin', ...options, headers: { Accept: 'application/json', ...(options.headers || {}) } }); const data = await response.json().catch(() => ({})); if (!response.ok) throw new Error(data.error?.message || `HTTP ${response.status}`); return data; };
  const notice = (message, success = false) => { $('alerts').innerHTML = `<div class="notice ${success ? 'success' : 'error'}">${esc(message)}</div>`; };
  async function load() {
    const [result, session] = await Promise.all([json('/api/v1/users'), json('/api/v1/session')]);
    const users = result.users || [];
    const admin = session.role === 'admin';
    $('users-list').innerHTML = users.length ? `<table><thead><tr><th>用户名</th><th>角色</th><th>创建时间</th><th>操作</th></tr></thead><tbody>${users.map((user) => `<tr><td><code>${esc(user.username)}</code></td><td><select class="role-select" data-user="${esc(user.username)}" ${admin ? '' : 'disabled'}><option value="viewer" ${user.role === 'viewer' ? 'selected' : ''}>viewer</option><option value="operator" ${user.role === 'operator' ? 'selected' : ''}>operator</option><option value="admin" ${user.role === 'admin' ? 'selected' : ''}>admin</option></select></td><td>${esc(user.created)}</td><td>${user.username === 'admin' || !admin ? '<span class="muted">受保护</span>' : `<button class="user-delete" data-user="${esc(user.username)}">删除</button>`}</td></tr>`).join('')}</tbody></table>` : '暂无用户';
    $('user-form').hidden = !admin;
    $('database-backup').hidden = !admin;
    document.querySelectorAll('.role-select').forEach((select) => select.addEventListener('change', () => updateRole(select)));
    document.querySelectorAll('.user-delete').forEach((button) => button.addEventListener('click', () => deleteUser(button)));
  }
  async function updateRole(select) { try { await json(`/api/v1/users/${encodeURIComponent(select.dataset.user)}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ role: select.value }) }); notice('角色已更新。', true); } catch (error) { notice(error.message); await load(); } }
  async function deleteUser(button) { if (!window.confirm(`确认删除用户 ${button.dataset.user}？`)) return; try { await json(`/api/v1/users/${encodeURIComponent(button.dataset.user)}`, { method: 'DELETE', headers: { 'X-CSRF-Token': csrf() } }); notice('用户已删除。', true); await load(); } catch (error) { notice(error.message); } }
  $('user-form').addEventListener('submit', async (event) => { event.preventDefault(); const form = new FormData(event.target); try { await json('/api/v1/users', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify(Object.fromEntries(form.entries())) }); event.target.reset(); notice('用户已创建。', true); await load(); } catch (error) { notice(error.message); } });
  $('refresh').addEventListener('click', () => load().catch((error) => notice(error.message)));
  $('database-backup').addEventListener('click', async () => { try { const result = await json('/api/v1/database/backup', { method: 'POST', headers: { 'X-CSRF-Token': csrf() }, body: '{}' }); notice(`数据库备份已创建：${result.path}`, true); } catch (error) { notice(error.message); } });
  load().catch((error) => notice(error.message));
})();
