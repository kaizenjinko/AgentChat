(function () {
  const state = window.Store.state;
  const PAGE_SIZE = 10;
  let markedReady = false;

  function initMarked() {
    if (markedReady || !window.marked) return;
    markedReady = true;
    const katexExt = window.markedKatex;
    if (katexExt && window.katex) {
      window.marked.use(katexExt({ throwOnError: false, output: 'html' }));
    }
    window.marked.setOptions({ gfm: true, breaks: true });
  }

  function wrapTables(html) {
    return html
      .replace(/<table>/g, '<div class="md-table-scroll"><table>')
      .replace(/<\/table>/g, '</table></div>');
  }

  function linkifyMentions(html) {
    const re = /@([A-Za-z0-9_][A-Za-z0-9_-]*)/g;
    const parts = html.split(/(<[^>]+>)/);
    return parts.map((part) => {
      if (part.charCodeAt(0) === 60) return part;
      return part.replace(re, '<span class="mention">@$1</span>');
    }).join('');
  }

  const CTRL_ESCAPE = {
    '\x07': 'a',
    '\t': 't',
    '\x0b': 'v',
    '\x0c': 'f'
  };

  function repairMath(md) {
    if (!/[\x07\t\x0b\x0c]/.test(md)) return md;
    return md.replace(/[\x07\t\x0b\x0c](?=[a-zA-Z{])/g, (ch) => '\\' + CTRL_ESCAPE[ch]);
  }

  function renderMarkdown(md) {
    if (!window.marked || !window.DOMPurify) {
      return window.UI.esc(md).replace(/\n/g, '<br>');
    }
    initMarked();
    const src = repairMath(md || '');
    let html = window.marked.parse(src);
    html = DOMPurify.sanitize(html);
    html = wrapTables(html);
    return linkifyMentions(html);
  }

  function msgEl(m) {
    const el = document.createElement('div');
    el.className = 'msg';
    el.dataset.id = m.id;
    const reply = m.reply_to ? '<div class="reply-to">↳ reply #' + window.UI.esc(m.reply_to) + '</div>' : '';
    el.innerHTML = `
    <div class="meta">
      <span class="agent">${window.UI.esc(m.agent)}</span>
      <span>#${window.UI.esc(m.id)}</span>
      <span>${window.UI.esc(m.created_at || '')}</span>
      ${m.mentions && m.mentions.length ? '· ' + m.mentions.map((x) => '@' + window.UI.esc(x)).join(' ') : ''}
    </div>
    ${reply}
    <div class="body">${renderMarkdown(m.content)}</div>
  `;
    return el;
  }

  function append(m) {
    const box = document.getElementById('messages');
    const empty = box.querySelector('.empty-dashed');
    if (empty) empty.remove();
    box.appendChild(msgEl(m));
    const nearBottom = box.scrollHeight - box.scrollTop - box.clientHeight < 200;
    if (nearBottom) box.scrollTop = box.scrollHeight;
  }

  function clearMessages(showEmpty = true) {
    const box = document.getElementById('messages');
    box.innerHTML = showEmpty
      ? '<div class="empty-dashed">Chưa có tin nhắn trong room này.</div>'
      : '';
  }

  function prepend(m) {
    const box = document.getElementById('messages');
    const empty = box.querySelector('.empty-dashed');
    if (empty) empty.remove();
    box.insertBefore(msgEl(m), box.firstChild);
  }

  async function loadMessages() {
    const data = await window.API.get(
      '/api/messages?room=' + encodeURIComponent(state.room) + '&limit=' + PAGE_SIZE + '&order=desc'
    );
    const box = document.getElementById('messages');
    box.innerHTML = '';
    const messages = data && data.messages ? data.messages : [];
    if (!messages.length) {
      clearMessages();
      state.hasMore = false;
      return;
    }
    for (let i = messages.length - 1; i >= 0; i--) {
      box.appendChild(msgEl(messages[i]));
    }
    state.lastId = messages[0].id;
    state.oldestId = messages[messages.length - 1].id;
    state.hasMore = messages.length === PAGE_SIZE;
    box.scrollTop = box.scrollHeight;
  }

  async function loadOlder() {
    if (state.loadingOlder || !state.hasMore || !state.oldestId) return;
    state.loadingOlder = true;
    const box = document.getElementById('messages');
    if (box.querySelector('.load-older')) {
      state.loadingOlder = false;
      return;
    }
    const hint = document.createElement('div');
    hint.className = 'load-older';
    hint.textContent = 'Đang tải tin nhắn cũ…';
    box.insertBefore(hint, box.firstChild);
    const prevHeight = box.scrollHeight;
    const prevTop = box.scrollTop;
    const room = state.room;
    try {
      const data = await window.API.get(
        '/api/messages?room=' + encodeURIComponent(room) +
        '&until=' + state.oldestId + '&limit=' + PAGE_SIZE + '&order=desc'
      );
      if (room !== state.room) return;
      const messages = data && data.messages ? data.messages : [];
      if (!messages.length) {
        state.hasMore = false;
        return;
      }
      const frag = document.createDocumentFragment();
      for (let i = messages.length - 1; i >= 0; i--) {
        frag.appendChild(msgEl(messages[i]));
      }
      box.insertBefore(frag, hint);
      state.oldestId = messages[messages.length - 1].id;
      state.hasMore = messages.length === PAGE_SIZE;
      box.scrollTop = prevTop + (box.scrollHeight - prevHeight);
    } finally {
      hint.remove();
      state.loadingOlder = false;
    }
  }

  function bindScroll() {
    const box = document.getElementById('messages');
    if (!box || box.dataset.scrollBound) return;
    box.dataset.scrollBound = '1';
    box.addEventListener('scroll', () => {
      if (box.scrollTop < 80) loadOlder();
    });
  }

  function subscribe() {
    if (state.es) state.es.close();
    const es = new EventSource('/api/stream?room=' + encodeURIComponent(state.room));
    state.es = es;
    const conn = document.getElementById('conn');
    es.addEventListener('hello', () => {
      if (conn) {
        conn.textContent = 'live';
        conn.classList.add('live');
      }
    });
    es.addEventListener('message', (ev) => {
      const m = JSON.parse(ev.data);
      if (m.room !== state.room) return;
      if (document.querySelector('.msg[data-id="' + m.id + '"]')) return;
      if (m.id <= state.lastId) return;
      state.lastId = m.id;
      append(m);
      loadRooms();
    });
    es.addEventListener('room-deleted', (ev) => {
      const payload = JSON.parse(ev.data);
      if (payload.room === state.room) {
        state.room = 'general';
        state.lastId = 0;
        state.oldestId = 0;
        state.hasMore = true;
        const roomName = document.getElementById('roomName');
        if (roomName) roomName.textContent = '#general';
        clearMessages();
        const box = document.getElementById('messages');
        box.innerHTML = '<div class="empty-dashed">Room đã bị xóa.</div>';
      }
      loadRooms();
    });
    es.onerror = () => {
      if (conn) {
        conn.textContent = 'reconnecting…';
        conn.classList.remove('live');
      }
    };
  }

  async function loadRooms() {
    const data = await window.API.get('/api/rooms');
    const box = document.getElementById('rooms');
    if (!box) return;
    const rooms = data && data.rooms ? data.rooms : [];
    if (!rooms.length) {
      box.innerHTML = '<div class="empty-dashed">Chưa có room.</div>';
      return;
    }
    box.innerHTML = '';
    for (const r of rooms) {
      const div = document.createElement('div');
      div.className = 'room nav-item' + (r.room === state.room ? ' active' : '');
      const left = document.createElement('span');
      left.textContent = '#' + r.room;
      const right = document.createElement('span');
      right.className = 'right';
      const count = document.createElement('span');
      count.className = 'badge badge-muted';
      count.textContent = r.count;
      const del = document.createElement('button');
      del.className = 'btn-icon';
      del.title = 'Xóa room';
      del.textContent = '✕';
      del.onclick = (e) => {
        e.stopPropagation();
        deleteRoom(r.room);
      };
      right.append(count, del);
      div.append(left, right);
      div.onclick = () => selectRoom(r.room);
      box.appendChild(div);
    }
  }

  async function deleteRoom(room) {
    if (!confirm('Xóa room #' + room + ' và toàn bộ tin nhắn?')) return;
    try {
      await window.API.del('/api/rooms/' + encodeURIComponent(room));
    } catch (e) {
      window.UI.toast(e && e.message ? e.message : 'Xóa room thất bại', 'danger');
      return;
    }
    if (room === state.room) {
      state.room = 'general';
      state.lastId = 0;
      state.oldestId = 0;
      state.hasMore = true;
      const roomName = document.getElementById('roomName');
      if (roomName) roomName.textContent = '#general';
      clearMessages();
      await loadMessages();
      subscribe();
    }
    loadRooms();
  }

  function selectRoom(room) {
    state.room = room;
    state.lastId = 0;
    state.oldestId = 0;
    state.hasMore = true;
    state.loadingOlder = false;
    const roomName = document.getElementById('roomName');
    if (roomName) roomName.textContent = '#' + room;
    clearMessages();
    loadMessages();
    subscribe();
    loadRooms();
  }

  function bindComposer() {
    const form = document.getElementById('composer');
    if (!form) return;
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const input = document.getElementById('input');
      if (!input) return;
      const content = input.value.trim();
      if (!content) return;
      input.value = '';
      try {
        await window.API.post('/api/messages', { agent: 'user', room: state.room, content });
      } catch (err) {
        window.UI.toast(err && err.message ? err.message : 'Gửi tin thất bại', 'danger');
      }
    });
  }

  function bindNewRoom() {
    const btn = document.getElementById('new-room');
    if (!btn) return;
    btn.addEventListener('click', () => createRoom());
  }

  function createRoom() {
    const backdrop = document.createElement('div');
    backdrop.className = 'modal-backdrop';
    const modal = document.createElement('form');
    modal.className = 'modal';
    modal.innerHTML =
      '<div class="modal-head"><span>Tạo room mới</span>' +
      '<button type="button" class="btn-icon" id="room-cancel-x" aria-label="Đóng">✕</button></div>' +
      '<div class="field" style="margin-bottom:var(--s4)"><label>Tên room</label>' +
      '<input class="input" id="room-name" placeholder="ví dụ: task-123" autocomplete="off" required></div>' +
      '<div class="modal-actions"><button type="button" class="btn btn-ghost" id="room-cancel">Hủy</button>' +
      '<button type="submit" class="btn btn-primary">Tạo</button></div>';
    backdrop.appendChild(modal);
    document.body.appendChild(backdrop);
    const close = () => backdrop.remove();
    const input = modal.querySelector('#room-name');
    input.focus();
    modal.querySelector('#room-cancel').addEventListener('click', close);
    modal.querySelector('#room-cancel-x').addEventListener('click', close);
    backdrop.addEventListener('click', (e) => {
      if (e.target === backdrop) close();
    });
    modal.addEventListener('submit', async (e) => {
      e.preventDefault();
      const room = input.value.trim();
      if (!room) {
        window.UI.toast('Tên room không được rỗng', 'warn');
        return;
      }
      if (!/^[A-Za-z0-9._-]{1,64}$/.test(room)) {
        window.UI.toast('Tên room chỉ gồm chữ, số, . _ - (tối đa 64)', 'warn');
        return;
      }
      try {
        await window.API.post('/api/rooms', { room });
        window.UI.toast('Đã tạo room #' + room, 'ok');
      } catch (err) {
        if (err && err.status === 409) {
          window.UI.toast('Room #' + room + ' đã tồn tại', 'warn');
          input.focus();
        } else {
          window.UI.toast(err && err.message ? err.message : 'Tạo room thất bại', 'danger');
        }
        return;
      }
      close();
      await loadRooms();
      selectRoom(room);
    });
  }

  const Chat = {
    initMarked,
    renderMarkdown,
    msgEl,
    append,
    clearMessages,
    loadMessages,
    loadOlder,
    subscribe,
    loadRooms,
    deleteRoom,
    selectRoom,
    createRoom,
    start() {
      bindComposer();
      bindScroll();
      bindNewRoom();
      loadRooms();
      selectRoom('general');
    }
  };

  window.Chat = Chat;
})();
