(function () {
  const Agents = {
    async render() {
      const box = document.getElementById('panel-agents');
      if (!box) return;
      box.innerHTML = '<div class="empty-dashed">Đang tải…</div>';
      let agents = [];
      let stats = null;
      try {
        const aData = await window.API.get('/api/agents');
        agents = aData && aData.agents ? aData.agents : [];
      } catch (e) {
        box.innerHTML = '<div class="empty-dashed">' + window.UI.esc(e && e.message ? e.message : 'Không tải được agents') + '</div>';
        return;
      }
      try {
        stats = await window.API.get('/api/stats');
      } catch (e) {
        stats = null;
      }
      const totalAgents = agents.length;
      let totalMessages = stats && stats.messages !== undefined ? stats.messages : 0;
      if (!stats && agents.length) {
        totalMessages = agents.reduce((sum, a) => sum + (Number(a.messages) || 0), 0);
      }
      const topAgent = agents.reduce((best, a) => {
        if (!best || (Number(a.messages) || 0) > (Number(best.messages) || 0)) return a;
        return best;
      }, null);
      const topName = topAgent ? (topAgent.name || '—') : '—';
      const topCount = topAgent ? (Number(topAgent.messages) || 0) : 0;
      const lastAt = agents.reduce((latest, a) => {
        const v = a.last_at || '';
        return v > latest ? v : latest;
      }, '');
      if (window.Shell && window.Shell.setCrumb) window.Shell.setCrumb('Agents');
      const head = window.UI.el('div', 'panel-head');
      head.innerHTML = '<h2>Agents</h2>';
      const chips = window.UI.el('div', 'grid-stats');
      chips.innerHTML =
        '<div class="stat-card"><span class="num">' + window.UI.esc(totalAgents) + '</span><span class="label">Tổng agents</span></div>' +
        '<div class="stat-card"><span class="num">' + window.UI.esc(totalMessages) + '</span><span class="label">Tổng tin nhắn</span></div>' +
        '<div class="stat-card"><span class="num is-text" title="' + window.UI.esc(topName) + '">' + window.UI.esc(topName) + '</span><span class="label">Tích cực nhất · ' + window.UI.esc(topCount) + ' tin</span></div>' +
        '<div class="stat-card"><span class="num is-text">' + window.UI.esc(lastAt || '—') + '</span><span class="label">Hoạt động cuối</span></div>';
      box.innerHTML = '';
      box.appendChild(head);
      box.appendChild(chips);
      if (!agents.length) {
        const empty = window.UI.el('div', 'empty-dashed', 'Chưa có agent nào.');
        box.appendChild(empty);
        return;
      }
      const wrap = window.UI.el('div', 'table-wrap');
      const table = window.UI.el('table', 'table');
      table.innerHTML =
        '<thead><tr><th>Agent</th><th>Số tin</th><th>Hoạt động cuối</th></tr></thead><tbody>' +
        agents.map((a) =>
          '<tr><td>' + window.UI.esc(a.name || '') + '</td>' +
          '<td>' + window.UI.esc(a.messages === undefined || a.messages === null ? 0 : a.messages) + '</td>' +
          '<td>' + window.UI.esc(a.last_at || '—') + '</td></tr>'
        ).join('') +
        '</tbody>';
      wrap.appendChild(table);
      box.appendChild(wrap);
    }
  };

  window.Agents = Agents;
})();
