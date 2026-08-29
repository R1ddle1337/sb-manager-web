(() => {
  const $ = (id) => document.getElementById(id);
  const csrf = () => (document.cookie.split('; ').find((v) => v.startsWith('sbweb_csrf=')) || '').split('=')[1] || '';
  const esc = (value) => String(value == null ? '-' : value).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const json = async (url, options = {}) => { const response = await fetch(url, { credentials: 'same-origin', ...options, headers: { Accept: 'application/json', ...(options.headers || {}) } }); const data = await response.json().catch(() => ({})); if (!response.ok) throw new Error(data.error?.message || `HTTP ${response.status}`); return data; };
  const notice = (message, success = false) => { $('alerts').innerHTML = `<div class="notice ${success ? 'success' : 'error'}">${esc(message)}</div>`; };
  const outputFor = (name) => $(`${name}-output`);
  async function read(name) { try { const data = await json(`/api/v1/${name}`); outputFor(name).textContent = JSON.stringify(data, null, 2); } catch (error) { outputFor(name).textContent = error.message; notice(error.message); } }
  async function action(name, args = {}) { try { const task = await json('/api/v1/servers/local/actions', { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf(), 'Idempotency-Key': `${name}-${Date.now()}` }, body: JSON.stringify({ action: name, args }) }); notice(`任务已创建：${task.id}`, true); } catch (error) { notice(error.message); } }
  document.querySelectorAll('[data-read]').forEach((button) => button.addEventListener('click', () => read(button.dataset.read)));
  document.querySelectorAll('[data-action]').forEach((button) => button.addEventListener('click', () => { if ((button.dataset.action === 'service.restart' && !window.confirm('确认重启 sing-box 及相关服务？')) || (button.dataset.action === 'firewall.ufw-allow' && !window.confirm('确认放行当前所有协议端口？')) || (button.dataset.action === 'firewall.clear-iptables' && !window.confirm('确认清理 sb-manager 添加的 iptables deny 规则？'))) return; action(button.dataset.action); }));
  $('traffic-form').addEventListener('submit', (event) => { event.preventDefault(); const form = new FormData(event.target); const args = {}; for (const [key, value] of form.entries()) if (value !== '') args[key] = value; action('traffic.set', args); });
  $('load-logs').addEventListener('click', async () => { try { const target = $('log-target').value; const result = await json(`/api/v1/logs?target=${encodeURIComponent(target)}&lines=200`); $('logs-output').textContent = result.output || ''; } catch (error) { $('logs-output').textContent = error.message; notice(error.message); } });
  read('health'); read('firewall'); read('traffic');
})();
