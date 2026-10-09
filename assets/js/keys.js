(function () {
  let bannerKey = '';

  function extractPlain(data) {
    if (!data) return '';
    if (typeof data === 'string') return data;
    if (data.key && typeof data.key === 'string') return data.key;
    if (data.plain && typeof data.plain === 'string') return data.plain;
    if (data.api_key && typeof data.api_key === 'string') return data.api_key;
    if (data.key && typeof data.key === 'object') {
      if (typeof data.key.plain === 'string') return data.key.plain;
      if (typeof data.key.api_key === 'string') return data.key.api_key;
      if (typeof data.key.key === 'string') return data.key.key;
    }
    return '';
  }

  function statusBadge(status) {
    const s = status === undefined || status === null ? 'active' : String(status);
    let cls = 'badge-ok';
    if (s === 'disabled' || s === 'revoked') cls = 'badge-danger';
    else if (s !== 'active') cls = 'badge-muted';
    return '<span class="badge ' + cls + '">' + window.UI.esc(s) + '</span>';
  }

  function banner(plain) {
    bannerKey = plain;
    const head = document.createElement('div');
    head.className = 'panel-head';
    head.innerHTML = '<h2>API Keys</h2>';
    const card = document.createElement('div');
    card.className = 'card';
    card.style.background = 'var(--accent-soft)';
    card.style.marginBottom = 'var(--s4)';
    card.innerHTML =
      '<div class="field"><label>Key mới (chỉ hiển thị 1 lần — hãy sao chép ngay)</label>' +
      '<div class="row"><code id="key-plain" class="input" style="font-family:ui-monospace,monospace;word-break:break-all">' + window.UI.esc(plain) + '</code>' +
      '<button class="btn btn-primary" id="key-copy">Copy</button></div></div>';
    return { head, card };
  }

  const Keys = {
    async render() {
      const box = document.getElementById('panel-keys');
      if (!box) return;
      if (window.Shell && window.Shell.setCrumb) window.Shell.setCrumb('API Keys');
      box.innerHTML = '<div class="empty-dashed">Đang tải…</div>';
      const head = document.createElement('div');
      head.className = 'panel-head';
      head.innerHTML = '<h2>API Keys</h2><button class="btn btn-primary" id="key-create">Tạo key</button>';
      let keys = [];
      try {
        const data = await window.API.get('/api/keys');
        keys = data && data.keys ? data.keys : [];
      } catch (e) {
        box.innerHTML = '';
        box.appendChild(head);
        box.appendChild(window.UI.el('div', 'empty-dashed', window.UI.esc(e && e.message ? e.message : 'Không tải được keys')));
        head.querySelector('#key-create').addEventListener('click', () => Keys.openCreate());
        return;
      }
      box.innerHTML = '';
      box.appendChild(head);
      if (bannerKey) {
        const b = banner(bannerKey);
        box.appendChild(b.card);
        const copyBtn = box.querySelector('#key-copy');
        if (copyBtn) {
          copyBtn.addEventListener('click', () => {
            Keys.copy(bannerKey);
          });
        }
      }
      if (!keys.length) {
        box.appendChild(window.UI.el('div', 'empty-dashed', 'Chưa có API key nào.'));
      } else {
        const wrap = document.createElement('div');
        wrap.className = 'table-wrap';
        const table = document.createElement('table');
        table.className = 'table';
        table.innerHTML =
          '<thead><tr><th>Prefix</th><th>Tên</th><th>Trạng thái</th><th>Dùng cuối</th><th></th></tr></thead><tbody>' +
          keys.map((k) =>
            '<tr><td><code>' + window.UI.esc(k.prefix || '') + '…</code></td>' +
            '<td>' + window.UI.esc(k.name || '') + '</td>' +
            '<td>' + statusBadge(k.status) + '</td>' +
            '<td>' + window.UI.esc(k.last_used_at || '—') + '</td>' +
            '<td style="text-align:right"><button class="btn btn-danger btn-sm" data-revoke="' + window.UI.esc(k.id) + '">Revoke</button></td></tr>'
          ).join('') +
          '</tbody>';
        wrap.appendChild(table);
        box.appendChild(wrap);
        box.querySelectorAll('[data-revoke]').forEach((btn) => {
          btn.addEventListener('click', () => Keys.revoke(btn.dataset.revoke));
        });
      }
      head.querySelector('#key-create').addEventListener('click', () => Keys.openCreate());
    },

    openCreate() {
      const backdrop = document.createElement('div');
      backdrop.className = 'modal-backdrop';
      const modal = document.createElement('form');
      modal.className = 'modal';
      modal.innerHTML =
        '<div class="modal-head"><span>Tạo API key</span>' +
        '<button type="button" class="btn-icon" id="key-cancel-x" aria-label="Đóng">✕</button></div>' +
        '<div class="field" style="margin-bottom:var(--s4)"><label>Tên key</label>' +
        '<input class="input" id="key-name" placeholder="ví dụ: agent-ci" required></div>' +
        '<div class="field"><label>Agent (tùy chọn)</label>' +
        '<input class="input" id="key-agent" placeholder="ví dụ: worker-1"></div>' +
        '<div class="modal-actions"><button type="button" class="btn btn-ghost" id="key-cancel">Hủy</button>' +
        '<button type="submit" class="btn btn-primary">Tạo</button></div>';
      backdrop.appendChild(modal);
      document.body.appendChild(backdrop);
      const close = () => backdrop.remove();
      modal.querySelector('#key-cancel').addEventListener('click', close);
      modal.querySelector('#key-cancel-x').addEventListener('click', close);
      backdrop.addEventListener('click', (e) => {
        if (e.target === backdrop) close();
      });
      modal.addEventListener('submit', async (e) => {
        e.preventDefault();
        const name = modal.querySelector('#key-name').value.trim();
        const agentName = modal.querySelector('#key-agent').value.trim();
        if (!name) {
          window.UI.toast('Nhập tên key', 'warn');
          return;
        }
        try {
          const data = await window.API.post('/api/keys', { name, agent_name: agentName });
          close();
          bannerKey = extractPlain(data);
          window.UI.toast('Đã tạo API key', 'ok');
          await Keys.render();
        } catch (err) {
          window.UI.toast(err && err.message ? err.message : 'Tạo key thất bại', 'danger');
        }
      });
    },

    copy(text) {
      const value = text || bannerKey;
      if (!value) return;
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(value).then(
          () => window.UI.toast('Đã copy key', 'ok'),
          () => window.UI.toast('Không copy được, hãy copy thủ công', 'warn')
        );
      } else {
        window.UI.toast('Hãy copy thủ công', 'warn');
      }
    },

    async revoke(id) {
      if (!confirm('Thu hồi API key này? Hành động không thể hoàn tác.')) return;
      try {
        await window.API.del('/api/keys/' + encodeURIComponent(id));
      } catch (e) {
        window.UI.toast(e && e.message ? e.message : 'Thu hồi thất bại', 'danger');
        return;
      }
      window.UI.toast('Đã thu hồi key', 'ok');
      bannerKey = '';
      await Keys.render();
    }
  };

  window.Keys = Keys;
})();
