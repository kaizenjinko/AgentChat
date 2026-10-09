(function () {
  const SPARK = {
    users: [3, 5, 4, 7, 6, 9, 11, 10, 12],
    rooms: [1, 2, 2, 3, 4, 4, 5, 6, 7],
    messages: [12, 18, 15, 24, 21, 30, 28, 36, 42],
    active_keys: [2, 2, 3, 3, 4, 5, 5, 6, 6]
  };

  function esc(s) {
    return String(s === undefined || s === null ? '' : s).replace(/[&<>"']/g, (c) => ({
      '&': '&amp;',
      '<': '&lt;',
      '>': '&gt;',
      '"': '&quot;',
      "'": '&#39;'
    }[c]));
  }

  function el(tag, cls, html) {
    const node = document.createElement(tag);
    if (cls) node.className = cls;
    if (html !== undefined && html !== null) node.innerHTML = html;
    return node;
  }

  function show(panelId, label) {
    window.Shell.show(panelId);
    if (label) window.Shell.setCrumb('Dashboard / ' + label, 'crumb');
  }

  function toast(msg, type) {
    const variant = type ? ' toast-' + type : '';
    const node = el('div', 'toast' + variant, esc(msg));
    node.style.position = 'fixed';
    node.style.right = '24px';
    node.style.bottom = '24px';
    node.style.zIndex = '60';
    node.style.maxWidth = '340px';
    document.body.appendChild(node);
    window.setTimeout(() => node.remove(), 3000);
    return node;
  }

  function fmtDate(v) {
    if (!v) return '—';
    return String(v);
  }

  function sparkline(values) {
    const w = 120;
    const h = 32;
    const min = Math.min.apply(null, values);
    const max = Math.max.apply(null, values);
    const span = max - min || 1;
    const step = values.length > 1 ? w / (values.length - 1) : w;
    const pts = values.map((v, i) => {
      const x = i * step;
      const y = h - ((v - min) / span) * (h - 4) - 2;
      return x.toFixed(1) + ',' + y.toFixed(1);
    }).join(' ');
    return '<svg class="sparkline" viewBox="0 0 ' + w + ' ' + h + '" preserveAspectRatio="none" aria-hidden="true">' +
      '<polyline points="' + pts + '" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>' +
      '</svg>';
  }

  function renderStats(stats) {
    const box = document.getElementById('stats');
    if (!box) return;
    const items = [
      { key: 'users', label: 'Users', value: stats.users },
      { key: 'rooms', label: 'Rooms', value: stats.rooms },
      { key: 'messages', label: 'Messages', value: stats.messages },
      { key: 'active_keys', label: 'Active keys', value: stats.active_keys }
    ];
    box.innerHTML = items.map((it) =>
      '<div class="stat-card">' +
        '<div class="num">' + esc(it.value == null ? 0 : it.value) + '</div>' +
        '<div class="label">' + esc(it.label) + '</div>' +
        sparkline(SPARK[it.key] || [0]) +
      '</div>'
    ).join('');
  }

  function roleBadge(role) {
    const cls = role === 'admin' ? 'badge-accent' : 'badge-muted';
    return '<span class="badge ' + cls + '">' + esc(role) + '</span>';
  }

  function statusBadge(status) {
    const cls = status === 'active' ? 'badge-ok' : 'badge-danger';
    return '<span class="badge ' + cls + '">' + esc(status) + '</span>';
  }

  function renderUsers(users) {
    const body = document.getElementById('users-body');
    if (!body) return;
    if (!users || !users.length) {
      body.innerHTML = '<tr><td colspan="9">Chưa có user</td></tr>';
      return;
    }
    body.innerHTML = users.map((u) => {
      const toggleLabel = u.status === 'active' ? 'Disable' : 'Enable';
      const toggleAction = u.status === 'active' ? 'disable' : 'enable';
      return '<tr>' +
        '<td>' + esc(u.username) + '</td>' +
        '<td>' + roleBadge(u.role) + '</td>' +
        '<td>' + statusBadge(u.status) + '</td>' +
        '<td class="num">' + esc(u.room_count) + '</td>' +
        '<td class="num">' + esc(u.msg_count) + '</td>' +
        '<td class="num">' + esc(u.key_count) + '</td>' +
        '<td class="num">' + esc(u.agent_count) + '</td>' +
        '<td>' + esc(fmtDate(u.last_login_at)) + '</td>' +
        '<td><div class="table-actions">' +
          '<button class="btn btn-sm" data-action="' + toggleAction + '" data-id="' + esc(u.id) + '" data-username="' + esc(u.username) + '">' + toggleLabel + '</button>' +
          '<button class="btn btn-sm" data-action="set-password" data-id="' + esc(u.id) + '" data-username="' + esc(u.username) + '">Reset password</button>' +
          '<button class="btn btn-sm btn-danger" data-action="delete" data-id="' + esc(u.id) + '" data-username="' + esc(u.username) + '">Delete</button>' +
        '</div></td>' +
      '</tr>';
    }).join('');
  }

  function renderAgents(agents) {
    const body = document.getElementById('agents-body');
    if (!body) return;
    if (!agents || !agents.length) {
      body.innerHTML = '<tr><td colspan="4">Chưa có agent</td></tr>';
      return;
    }
    body.innerHTML = agents.map((a) =>
      '<tr>' +
        '<td>' + esc(a.name) + '</td>' +
        '<td>' + esc(a.username) + '</td>' +
        '<td class="num">' + esc(a.messages) + '</td>' +
        '<td>' + esc(fmtDate(a.last_at)) + '</td>' +
      '</tr>'
    ).join('');
  }

  function renderRooms(rooms) {
    const body = document.getElementById('rooms-body');
    if (!body) return;
    if (!rooms || !rooms.length) {
      body.innerHTML = '<tr><td colspan="5">Chưa có room</td></tr>';
      return;
    }
    body.innerHTML = rooms.map((r) =>
      '<tr>' +
        '<td>' + esc(r.room) + '</td>' +
        '<td>' + esc(r.username) + '</td>' +
        '<td class="num">' + esc(r.count) + '</td>' +
        '<td class="num">' + esc(r.last_id) + '</td>' +
        '<td>' + esc(fmtDate(r.last_at)) + '</td>' +
      '</tr>'
    ).join('');
  }

  function openModal(title, fieldsHtml, onSubmit) {
    const backdrop = el('div', 'modal-backdrop');
    const modal = el('div', 'modal');
    modal.innerHTML =
      '<div class="modal-head"><span>' + esc(title) + '</span>' +
        '<button class="btn-icon" data-close aria-label="Đóng" type="button">' +
          '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>' +
        '</button>' +
      '</div>' +
      '<form class="modal-form">' + fieldsHtml +
        '<div class="modal-actions">' +
          '<button type="button" class="btn" data-close>Huỷ</button>' +
          '<button type="submit" class="btn btn-primary">Lưu</button>' +
        '</div>' +
      '</form>';
    backdrop.appendChild(modal);
    document.body.appendChild(backdrop);
    const close = () => backdrop.remove();
    modal.querySelectorAll('[data-close]').forEach((b) => b.addEventListener('click', close));
    backdrop.addEventListener('click', (e) => {
      if (e.target === backdrop) close();
    });
    const form = modal.querySelector('form');
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const submit = form.querySelector('button[type="submit"]');
      const data = {};
      new FormData(form).forEach((v, k) => { data[k] = v; });
      submit.disabled = true;
      try {
        await onSubmit(data, close);
      } finally {
        submit.disabled = false;
      }
    });
    const first = form.querySelector('input, select');
    if (first) first.focus();
    return { close, modal };
  }

  async function loadStats() {
    try {
      const data = await window.API.get('/api/admin/stats');
      const stats = data && data.stats ? data.stats : data;
      renderStats(stats || {});
    } catch (e) {
      if (e && e.status !== 401) toast(e.message || 'Không tải được thống kê', 'danger');
    }
  }

  async function loadUsers() {
    try {
      const data = await window.API.get('/api/admin/users');
      renderUsers((data && data.users) || []);
    } catch (e) {
      if (e && e.status !== 401) toast(e.message || 'Không tải được users', 'danger');
    }
  }

  async function loadAgents() {
    try {
      const data = await window.API.get('/api/admin/agents');
      renderAgents((data && data.agents) || []);
    } catch (e) {
      if (e && e.status !== 401) toast(e.message || 'Không tải được agents', 'danger');
    }
  }

  async function loadRooms() {
    try {
      const data = await window.API.get('/api/admin/rooms');
      renderRooms((data && data.rooms) || []);
    } catch (e) {
      if (e && e.status !== 401) toast(e.message || 'Không tải được rooms', 'danger');
    }
  }

  function createUser() {
    const fields =
      '<div class="field"><label for="cu-username">Username</label>' +
        '<input class="input" id="cu-username" name="username" type="text" required /></div>' +
      '<div class="field"><label for="cu-password">Password</label>' +
        '<input class="input" id="cu-password" name="password" type="password" autocomplete="new-password" required /></div>' +
      '<div class="field"><label for="cu-role">Role</label>' +
        '<select class="input" id="cu-role" name="role">' +
          '<option value="user">user</option>' +
          '<option value="admin">admin</option>' +
        '</select></div>';
    openModal('Tạo user', fields, async (data, close) => {
      try {
        await window.API.post('/api/admin/users', {
          username: data.username,
          password: data.password,
          role: data.role
        });
        close();
        toast('Đã tạo user ' + data.username, 'ok');
        loadUsers();
        loadStats();
      } catch (e) {
        if (e && e.status === 409) {
          toast('username đã tồn tại', 'danger');
        } else {
          toast((e && e.message) || 'Tạo user thất bại', 'danger');
        }
      }
    });
  }

  async function userAction(id, action, username) {
    if (action === 'disable' || action === 'enable') {
      try {
        await window.API.patch('/api/admin/users/' + encodeURIComponent(id), { action });
        toast('Đã ' + action + ' ' + username, 'ok');
        loadUsers();
        loadStats();
      } catch (e) {
        toast((e && e.message) || 'Thao tác thất bại', 'danger');
      }
      return;
    }

    if (action === 'set-password') {
      const pw = window.prompt('Mật khẩu mới cho ' + username + ':');
      if (pw === null) return;
      if (!pw) {
        toast('Mật khẩu không được rỗng', 'danger');
        return;
      }
      try {
        await window.API.patch('/api/admin/users/' + encodeURIComponent(id), {
          action: 'set-password',
          password: pw
        });
        toast('Đã đổi mật khẩu cho ' + username, 'ok');
      } catch (e) {
        toast((e && e.message) || 'Đổi mật khẩu thất bại', 'danger');
      }
      return;
    }

    if (action === 'delete') {
      if (!window.confirm('Xoá user "' + username + '"? Toàn bộ dữ liệu liên quan sẽ bị xoá.')) return;
      try {
        await window.API.del('/api/admin/users/' + encodeURIComponent(id));
        toast('Đã xoá ' + username, 'ok');
        loadUsers();
        loadStats();
      } catch (e) {
        toast((e && e.message) || 'Xoá user thất bại', 'danger');
      }
    }
  }

  const NAV_LABELS = {
    users: 'Users',
    agents: 'Agents',
    rooms: 'Rooms'
  };

  function wireUsersTable() {
    const body = document.getElementById('users-body');
    if (!body || body.dataset.wired) return;
    body.dataset.wired = '1';
    body.addEventListener('click', (e) => {
      const btn = e.target.closest('button[data-action]');
      if (!btn) return;
      userAction(btn.dataset.id, btn.dataset.action, btn.dataset.username);
    });
  }

  function showTab(tab) {
    const panel = document.getElementById('panel-dashboard');
    if (!panel) return;
    panel.querySelectorAll('.tab').forEach((t) => t.classList.toggle('active', t.dataset.tab === tab));
    panel.querySelectorAll('.dash-tab').forEach((p) => { p.hidden = p.dataset.tab !== tab; });
    if (NAV_LABELS[tab]) window.Shell.setCrumb('Dashboard / ' + NAV_LABELS[tab], 'crumb');
    if (tab === 'agents') loadAgents();
    if (tab === 'rooms') loadRooms();
  }

  function wireTabs() {
    const tabs = document.getElementById('dash-tabs');
    if (!tabs || tabs.dataset.wired) return;
    tabs.dataset.wired = '1';
    tabs.addEventListener('click', (e) => {
      const tab = e.target.closest('.tab');
      if (!tab) return;
      showTab(tab.dataset.tab);
    });
  }

  const Admin = {
    render: function () {
      const panel = document.getElementById('panel-dashboard');
      if (!panel) return;
      window.Shell.setCrumb('Dashboard', 'crumb');
      wireTabs();
      wireUsersTable();
      const createBtn = document.getElementById('create-user-btn');
      if (createBtn && !createBtn.dataset.wired) {
        createBtn.dataset.wired = '1';
        createBtn.addEventListener('click', createUser);
      }
      showTab('users');
      loadStats();
      loadUsers();
    },

    loadStats: loadStats,
    loadUsers: loadUsers,
    loadAgents: loadAgents,
    loadRooms: loadRooms,
    toast: toast,
    show: show
  };

  window.Admin = Admin;
})();
