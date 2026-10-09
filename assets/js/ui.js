(function () {
  const UI = {
    el(tag, cls, html) {
      const node = document.createElement(tag);
      if (cls) node.className = cls;
      if (html !== undefined && html !== null) node.innerHTML = html;
      return node;
    },

    sidebarOpen(on) {
      const card = document.querySelector('.app-card');
      if (card) card.classList.toggle('sidebar-open', !!on);
    },

    esc(s) {
      return String(s === undefined || s === null ? '' : s).replace(/[&<>"']/g, (c) => ({
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;'
      }[c]));
    },

    initTheme() {
      let theme;
      try {
        theme = localStorage.getItem('theme');
      } catch (e) {
        theme = null;
      }
      if (theme !== 'light' && theme !== 'dark') {
        theme = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
      }
      document.documentElement.dataset.theme = theme;
      return theme;
    },

    toggleTheme() {
      return window.Shell.toggleTheme();
    },

    toast(msg, type) {
      const variant = type ? ' toast-' + type : '';
      const node = UI.el('div', 'toast' + variant, UI.esc(msg));
      document.body.appendChild(node);
      window.setTimeout(() => {
        node.remove();
      }, 3000);
      return node;
    },

    show(panelId) {
      return window.Shell.show(panelId);
    },

    logout() {
      return window.Auth.logout();
    },

    renderProfile(user) {
      return window.Shell.renderProfile(user);
    },

    async init() {
      UI.initTheme();
      let user = null;
      try {
        user = await window.Auth.requireAuth();
      } catch (e) {
        return null;
      }
      if (user) {
        window.Store.set('user', user);
        window.Shell.renderProfile(user);
      }
      return user;
    }
  };

  window.UI = UI;
})();
