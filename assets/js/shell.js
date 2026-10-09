(function () {
  const ICONS = {
    chat: '<path d="M21 12a8 8 0 0 1-11.5 7.2L3 21l1.8-6.5A8 8 0 1 1 21 12z"/>',
    agents: '<circle cx="12" cy="8" r="4"/><path d="M5 21a7 7 0 0 1 14 0"/>',
    keys: '<circle cx="8" cy="15" r="3"/><path d="M10.5 12.5 20 3m-3 0h3v3"/>',
    settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-2.9 1.2 2 2 0 1 1-4 0 1.7 1.7 0 0 0-2.9-1.2l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1A1.7 1.7 0 0 0 4.6 15a2 2 0 1 1 0-4 1.7 1.7 0 0 0 1.2-2.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1A1.7 1.7 0 0 0 11.5 4.6a2 2 0 1 1 4 0 1.7 1.7 0 0 0 2.9 1.2l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0 1.2 2.9 2 2 0 1 1 0 4 1.7 1.7 0 0 0-1.2 1.9z"/>',
    dashboard: '<path d="M3 3v18h18"/><path d="M7 14l3-3 3 3 4-5"/>',
    users: '<circle cx="9" cy="8" r="3.5"/><path d="M3 20a6 6 0 0 1 12 0"/><path d="M16 5.5a3.5 3.5 0 0 1 0 7"/><path d="M18 20a6 6 0 0 0-3-5.2"/>',
    rooms: '<path d="M21 12a8 8 0 0 1-11.5 7.2L3 21l1.8-6.5A8 8 0 1 1 21 12z"/>',
    back: '<path d="M15 18l-6-6 6-6"/>',
    menu: '<path d="M4 6h16M4 12h16M4 18h16"/>',
    theme: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>',
    logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="M16 17l5-5-5-5"/><path d="M21 12H9"/>',
    chevron: '<path d="M6 9l6 6 6-6"/>'
  };

  function svg(name, size) {
    const s = size || 20;
    return '<svg viewBox="0 0 24 24" width="' + s + '" height="' + s +
      '" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
      (ICONS[name] || '') + '</svg>';
  }

  function navItem(item) {
    const icon = item.icon ? svg(item.icon, item.iconSize || 20) : '';
    const cls = 'nav-item' + (item.active ? ' active' : '');
    const attrs = item.panel && !item.href
      ? 'href="#' + item.panel + '" data-panel="' + item.panel + '"'
      : 'href="' + (item.href || '#') + '"';
    const id = item.id ? ' id="' + item.id + '"' : '';
    const hidden = item.hidden ? ' hidden' : '';
    return '<a class="' + cls + '"' + id + ' ' + attrs + hidden + '>' +
      icon + '<span class="nav-label">' + item.label + '</span></a>';
  }

  const Shell = {
    svg: svg,

    mount: function (cfg) {
      cfg = cfg || {};
      const card = document.querySelector('.app-card');
      if (!card) return null;

      const nav = (cfg.nav || []).map(navItem).join('');
      const roomsBlock = cfg.rooms
        ? '<div id="sidebar-rooms">' +
            '<div class="rooms-head">' +
              '<span class="sidebar-label">Rooms</span>' +
              '<button class="btn-icon" id="new-room" title="Tạo room" aria-label="Tạo room">' +
                '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>' +
              '</button></div>' +
            '<div id="rooms" class="scroll"></div>' +
          '</div>'
        : '';

      const profileMenu = cfg.profileMenu
        ? '<div class="profile-menu" id="profile-menu" hidden>' +
            '<a class="nav-item" id="profile-settings" href="#settings">Cài đặt</a>' +
            '<a class="nav-item" id="logout-btn" href="#logout">Đăng xuất</a>' +
          '</div>'
        : '';

      const profileAction = cfg.profileMenu
        ? svg('chevron', 16)
        : '<button class="btn-icon" id="logout-btn" title="Đăng xuất" aria-label="Đăng xuất">' + svg('logout', 18) + '</button>';

      const wordmarkAction = cfg.collapse
        ? '<span class="spacer"></span>' +
          '<button class="btn-icon" id="collapse-btn" title="Thu gọn" aria-label="Thu gọn sidebar">' + svg('menu', 18) + '</button>'
        : '';

      const search = cfg.search
        ? '<div class="search-pill">' +
            '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/></svg>' +
            '<input type="text" placeholder="Tìm kiếm…" aria-label="Tìm kiếm" /></div>'
        : '';

      const sidebar =
        '<aside class="sidebar">' +
          '<div class="wordmark"><span>' + (cfg.app || 'AgentChat') + '</span>' + wordmarkAction + '</div>' +
          search +
          '<nav class="nav">' + nav + '</nav>' +
          roomsBlock +
          profileMenu +
          '<div class="profile-row" id="profile-row" role="button" tabindex="0">' +
            '<span class="avatar" id="profile-avatar">?</span>' +
            '<span class="profile-meta">' +
              '<span class="name" id="profile-name">…</span>' +
              '<span class="sub"><span class="badge badge-muted" id="profile-badge">user</span></span>' +
            '</span>' +
            profileAction +
          '</div>' +
        '</aside>' +
        '<div class="sidebar-backdrop" id="sidebar-backdrop"></div>';

      const header = cfg.header || {};
      const crumb = header.crumb !== undefined ? header.crumb : (cfg.app || 'AgentChat');
      const crumbId = header.crumbId || 'crumb';
      const topbar =
        '<header class="topbar">' +
          '<button class="btn-icon menu-btn" id="menu-btn" title="Menu" aria-label="Mở menu">' + svg('menu', 20) + '</button>' +
          '<span class="wordmark" id="' + crumbId + '">' + crumb + '</span>' +
          '<span class="spacer"></span>' +
          (header.conn ? '<span id="conn" class="conn">connecting…</span>' : '') +
          '<button class="theme-toggle" id="theme-toggle" title="Chế độ sáng/tối" aria-label="Chế độ sáng/tối">' + svg('theme', 17) + '</button>' +
        '</header>';

      card.insertAdjacentHTML('afterbegin', sidebar);
      const main = card.querySelector('.main');
      if (main) {
        main.insertAdjacentHTML('afterbegin', topbar);
      } else {
        card.insertAdjacentHTML('beforeend', topbar);
      }
      Shell.wire(cfg);
      return { card: card };
    },

    wire: function (cfg) {
      const card = document.querySelector('.app-card');
      const open = (on) => { if (card) card.classList.toggle('sidebar-open', !!on); };

      const menuBtn = document.getElementById('menu-btn');
      if (menuBtn) menuBtn.addEventListener('click', () => open(!card.classList.contains('sidebar-open')));
      const backdrop = document.getElementById('sidebar-backdrop');
      if (backdrop) backdrop.addEventListener('click', () => open(false));

      const toggle = document.getElementById('theme-toggle');
      if (toggle) toggle.addEventListener('click', () => Shell.toggleTheme());

      document.querySelectorAll('.nav-item[data-panel]').forEach((n) => {
        if (!n.dataset.panel) return;
        n.addEventListener('click', (e) => {
          e.preventDefault();
          const panel = n.dataset.panel;
          Shell.show(panel);
          open(false);
          document.dispatchEvent(new CustomEvent('shell:nav', { detail: { panel } }));
        });
      });

      const profileRow = document.getElementById('profile-row');
      if (profileRow && cfg.profileMenu) {
        profileRow.addEventListener('click', () => {
          const menu = document.getElementById('profile-menu');
          if (menu) menu.hidden = !menu.hidden;
        });
      }
      const settingsBtn = document.getElementById('profile-settings');
      if (settingsBtn) settingsBtn.addEventListener('click', (e) => {
        e.preventDefault();
        const menu = document.getElementById('profile-menu');
        if (menu) menu.hidden = true;
        Shell.show('settings');
        document.dispatchEvent(new CustomEvent('shell:nav', { detail: { panel: 'settings' } }));
      });
      const logoutBtn = document.getElementById('logout-btn');
      if (logoutBtn) logoutBtn.addEventListener('click', (e) => {
        e.preventDefault();
        window.Auth.logout();
      });
      const collapse = document.getElementById('collapse-btn');
      if (collapse) collapse.addEventListener('click', () => {
        const sb = document.querySelector('.sidebar');
        if (sb) sb.classList.toggle('sidebar-collapsed');
      });
    },

    toggleTheme: function () {
      const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
      document.documentElement.dataset.theme = next;
      try { localStorage.setItem('theme', next); } catch (e) {}
      return next;
    },

    renderProfile: function (user) {
      if (!user) return;
      const nameEl = document.getElementById('profile-name');
      const badgeEl = document.getElementById('profile-badge');
      const avatarEl = document.getElementById('profile-avatar');
      const initial = (user.username || '?').charAt(0).toUpperCase();
      if (nameEl) nameEl.textContent = user.username || '';
      if (badgeEl) {
        badgeEl.textContent = user.role === 'admin' ? 'admin' : 'user';
        badgeEl.className = 'badge ' + (user.role === 'admin' ? 'badge-accent' : 'badge-muted');
      }
      if (avatarEl) avatarEl.textContent = initial;
    },

    setCrumb: function (text, id) {
      const el = (id && document.getElementById(id)) || document.querySelector('.topbar .wordmark');
      if (el) el.textContent = text;
    },

    show: function (panelId) {
      document.querySelectorAll('.panel').forEach((p) => {
        p.hidden = p.dataset.panel !== panelId;
      });
      document.querySelectorAll('.nav-item[data-panel]').forEach((n) => {
        n.classList.toggle('active', n.dataset.panel === panelId);
      });
      const rooms = document.getElementById('sidebar-rooms');
      if (rooms) rooms.hidden = panelId !== 'chat';
    }
  };

  window.Shell = Shell;
})();
