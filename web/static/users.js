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
    $('users-list').innerHTML = users.length ? `<table><thead><tr><th>用户名</th><th>角色</th><th>创建时间</th><th>操作</th></tr></thead><tbody>${users.map((user) => `<tr><td><code>${esc(user.username)}</code></td><td><select class="role-select" data-user="${esc(user.username)}" ${admin ? '' : 'disabled'}><option value="viewer" ${user.role === 'viewer' ? 'selected' : ''}>viewer</option><option value="operator" ${user.role === 'operator' ? 'selected' : ''}>operator</option><option value="admin" ${user.role === 'admin' ? 'selected' : ''}>admin</option></select></td><td>${esc(user.created)}</td><td>${user.username === session.username || !admin ? '<span class="muted">当前账号</span>' : `<button class="user-delete" data-user="${esc(user.username)}">删除</button>`}</td></tr>`).join('')}</tbody></table>` : '暂无用户';
    $('user-form').hidden = !admin;
    $('database-backup').hidden = !admin;
    if ($('backup-panel')) $('backup-panel').hidden = !admin;
    let tokenPanel = $('token-panel');
    if (!tokenPanel) {
      tokenPanel = document.createElement('section'); tokenPanel.id = 'token-panel'; tokenPanel.className = 'panel'; document.querySelector('.detail-shell').appendChild(tokenPanel);
    }
    tokenPanel.hidden = !admin;
    if (admin) {
      const tokenResult = await json('/api/v1/tokens');
      tokenPanel.innerHTML = `<div class="panel-header"><div><p class="eyebrow">AUTOMATION</p><h2>API Token</h2></div></div><form id="token-form" class="form-grid detail-form"><label>名称<input name="name" placeholder="prometheus" required></label><label>权限<select name="role"><option value="viewer">viewer</option><option value="operator">operator</option><option value="admin">admin</option></select></label><button class="primary-button" type="submit">创建 Token</button></form><div class="table-wrap"><table><thead><tr><th>名称</th><th>角色</th><th>创建时间</th><th>操作</th></tr></thead><tbody>${(tokenResult.tokens || []).map((token) => `<tr><td>${esc(token.name)}</td><td>${esc(token.role)}</td><td>${esc(token.created_at)}</td><td><button class="token-delete" data-token="${esc(token.id)}">撤销</button></td></tr>`).join('')}</tbody></table></div><pre id="token-output" class="output" hidden></pre>`;
      tokenPanel.querySelector('#token-form').addEventListener('submit', async (event) => { event.preventDefault(); try { const value = await json('/api/v1/tokens', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify(Object.fromEntries(new FormData(event.target).entries())) }); const output = tokenPanel.querySelector('#token-output'); output.hidden = false; output.textContent = `请立即保存，此 Token 只显示一次：\n\n${value.token}`; event.target.reset(); await load(); } catch (error) { notice(error.message); } });
      tokenPanel.querySelectorAll('.token-delete').forEach((button) => button.addEventListener('click', async () => { if (!window.confirm('确认撤销该 API Token？')) return; try { await json(`/api/v1/tokens/${encodeURIComponent(button.dataset.token)}`, { method: 'DELETE', headers: { 'X-CSRF-Token': csrf() } }); notice('API Token 已撤销。', true); await load(); } catch (error) { notice(error.message); } }));
    }
    let twoFA = $('twofa-panel');
    if (!twoFA) { twoFA = document.createElement('section'); twoFA.id = 'twofa-panel'; twoFA.className = 'panel'; document.querySelector('.detail-shell').appendChild(twoFA); }
    twoFA.innerHTML = `<div class="panel-header"><div><p class="eyebrow">SECURITY</p><h2>双因素认证</h2></div><span class="muted">${session.totp_enabled ? '已启用' : '未启用'}</span></div>${session.totp_enabled ? '<form id="twofa-disable" class="inline-form"><input name="code" inputmode="numeric" placeholder="验证码或恢复码" required><button class="secondary-button" type="submit">停用 2FA</button></form>' : '<button id="twofa-setup" class="primary-button">生成验证器密钥</button><div id="twofa-verify" hidden><p class="muted">将 URI 添加到验证器后输入当前验证码：</p><pre id="twofa-uri" class="output"></pre><form id="twofa-enable" class="inline-form"><input name="code" inputmode="numeric" placeholder="6 位验证码" required><button class="primary-button" type="submit">启用 2FA</button></form></div>'}`;
    if (!session.totp_enabled) {
      twoFA.querySelector('#twofa-setup').addEventListener('click', async () => { try { const result = await json('/api/v1/2fa/setup', { method: 'POST', headers: { 'X-CSRF-Token': csrf() }, body: '{}' }); twoFA.querySelector('#twofa-uri').textContent = `Secret: ${result.secret}\n\n${result.otpauth_uri}`; twoFA.querySelector('#twofa-verify').hidden = false; } catch (error) { notice(error.message); } });
      twoFA.querySelector('#twofa-enable').addEventListener('submit', async (event) => { event.preventDefault(); try { const result = await json('/api/v1/2fa/enable', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify(Object.fromEntries(new FormData(event.target).entries())) }); notice(`2FA 已启用。请保存恢复码：${result.recovery_codes.join(' ')}`, true); await load(); } catch (error) { notice(error.message); } });
    } else {
      twoFA.querySelector('#twofa-disable').addEventListener('submit', async (event) => { event.preventDefault(); try { await json('/api/v1/2fa/disable', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify(Object.fromEntries(new FormData(event.target).entries())) }); notice('2FA 已停用。', true); await load(); } catch (error) { notice(error.message); } });
    }
    document.querySelectorAll('.role-select').forEach((select) => select.addEventListener('change', () => updateRole(select)));
    document.querySelectorAll('.user-delete').forEach((button) => button.addEventListener('click', () => deleteUser(button)));
  }
  async function updateRole(select) { try { await json(`/api/v1/users/${encodeURIComponent(select.dataset.user)}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify({ role: select.value }) }); notice('角色已更新。', true); } catch (error) { notice(error.message); await load(); } }
  async function deleteUser(button) { if (!window.confirm(`确认删除用户 ${button.dataset.user}？`)) return; try { await json(`/api/v1/users/${encodeURIComponent(button.dataset.user)}`, { method: 'DELETE', headers: { 'X-CSRF-Token': csrf() } }); notice('用户已删除。', true); await load(); } catch (error) { notice(error.message); } }
  $('user-form').addEventListener('submit', async (event) => { event.preventDefault(); const form = new FormData(event.target); try { await json('/api/v1/users', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf() }, body: JSON.stringify(Object.fromEntries(form.entries())) }); event.target.reset(); notice('用户已创建。', true); await load(); } catch (error) { notice(error.message); } });
  $('refresh').addEventListener('click', () => load().catch((error) => notice(error.message)));
  $('database-backup').addEventListener('click', async () => { try { const result = await json('/api/v1/database/backup', { method: 'POST', headers: { 'X-CSRF-Token': csrf() }, body: '{}' }); notice(`数据库备份已创建：${result.path}`, true); } catch (error) { notice(error.message); } });
  const backupPanel = document.createElement('section');
  backupPanel.className = 'panel';
  backupPanel.id = 'backup-panel';
  backupPanel.innerHTML = '<div class="panel-header"><div><p class="eyebrow">BACKUPS</p><h2>sb-manager 备份与恢复</h2></div><button id="backup-refresh" class="secondary-button">刷新</button></div><form id="backup-upload" class="inline-form" enctype="multipart/form-data"><input name="backup" type="file" accept=".gz,.age" required><button class="secondary-button" type="submit">上传备份</button></form><div id="backup-list" class="table-wrap">读取中…</div>';
  document.querySelector('.detail-shell').appendChild(backupPanel);
  async function loadBackups() { try { const result = await json('/api/v1/backups'); backupPanel.querySelector('#backup-list').innerHTML = (result.backups || []).length ? `<table><thead><tr><th>文件</th><th>大小</th><th>时间</th><th>操作</th></tr></thead><tbody>${result.backups.map((backup) => `<tr><td><code>${esc(backup.name)}</code></td><td>${esc(backup.size)}</td><td>${esc(backup.modified_at)}</td><td><button class="backup-restore" data-backup="${esc(backup.name)}">恢复</button></td></tr>`).join('')}</tbody></table>` : '暂无备份'; backupPanel.querySelectorAll('.backup-restore').forEach((button) => button.addEventListener('click', async () => { if (!window.confirm(`恢复 ${button.dataset.backup} 会覆盖当前 sb-manager 状态，确认继续？`)) return; try { await json(`/api/v1/backups/${encodeURIComponent(button.dataset.backup)}/restore`, { method: 'POST', headers: { 'X-CSRF-Token': csrf() }, body: '{}' }); notice('备份恢复完成。', true); } catch (error) { notice(error.message); } })); } catch (error) { notice(error.message); } }
  backupPanel.querySelector('#backup-refresh').addEventListener('click', loadBackups);
  backupPanel.querySelector('#backup-upload').addEventListener('submit', async (event) => { event.preventDefault(); try { await json('/api/v1/backups', { method: 'POST', headers: { 'X-CSRF-Token': csrf() }, body: new FormData(event.target) }); event.target.reset(); notice('备份已上传。', true); await loadBackups(); } catch (error) { notice(error.message); } });
  loadBackups();
  load().catch((error) => notice(error.message));
})();
