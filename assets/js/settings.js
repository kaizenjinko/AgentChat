(function () {
  function userInfo() {
    const user = window.Store && window.Store.state ? window.Store.state.user : null;
    if (user) return user;
    return null;
  }

  const Settings = {
    async render() {
      const box = document.getElementById('panel-settings');
      if (!box) return;
      if (window.Shell && window.Shell.setCrumb) window.Shell.setCrumb('Settings');
      let user = userInfo();
      if (!user) {
        try {
          user = await window.Auth.me();
          if (user && window.Store) window.Store.set('user', user);
        } catch (e) {
          user = null;
        }
      }
      const username = user && user.username ? user.username : '—';
      const role = user && user.role ? user.role : 'user';
      const roleLabel = role === 'admin' ? 'Quản trị viên' : 'Người dùng';
      const isAdmin = role === 'admin';
      const initial = window.UI.esc(String(username).charAt(0).toUpperCase());

      box.innerHTML =
        '<div class="panel-head">' +
          '<div><h2>Settings</h2>' +
          '<p class="muted settings-hint">Thông tin tài khoản và bảo mật.</p></div>' +
        '</div>' +
        '<div class="settings-wrap">' +
          '<section class="card settings-card">' +
            '<header class="settings-card-head">' +
              '<span class="avatar avatar-lg">' + initial + '</span>' +
              '<div class="settings-card-meta">' +
                '<span class="settings-name">' + window.UI.esc(username) + '</span>' +
                '<span class="badge ' + (isAdmin ? 'badge-accent' : 'badge-muted') + '">' + roleLabel + '</span>' +
              '</div>' +
            '</header>' +
            '<dl class="settings-list">' +
              '<div class="settings-row"><dt>Tên đăng nhập</dt><dd>' + window.UI.esc(username) + '</dd></div>' +
              '<div class="settings-row"><dt>Vai trò</dt><dd>' + roleLabel + '</dd></div>' +
            '</dl>' +
          '</section>' +
          '<section class="card settings-card">' +
            '<header class="settings-card-head">' +
              '<div class="settings-card-meta">' +
                '<span class="settings-section-title">Đổi mật khẩu</span>' +
                '<span class="muted settings-hint">Tối thiểu 8 ký tự.</span>' +
              '</div>' +
            '</header>' +
            '<form id="pw-form" class="stack">' +
              '<div class="field"><label for="pw-old">Mật khẩu hiện tại</label>' +
                '<input class="input" type="password" id="pw-old" autocomplete="current-password"></div>' +
              '<div class="field"><label for="pw-new">Mật khẩu mới</label>' +
                '<input class="input" type="password" id="pw-new" autocomplete="new-password"></div>' +
              '<div class="field"><label for="pw-confirm">Nhập lại mật khẩu mới</label>' +
                '<input class="input" type="password" id="pw-confirm" autocomplete="new-password"></div>' +
              '<div class="settings-actions">' +
                '<button type="submit" class="btn btn-primary">Đổi mật khẩu</button>' +
              '</div>' +
            '</form>' +
          '</section>' +
        '</div>';

      const form = box.querySelector('#pw-form');
      form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const oldPw = box.querySelector('#pw-old').value;
        const newPw = box.querySelector('#pw-new').value;
        const confirmPw = box.querySelector('#pw-confirm').value;
        if (newPw.length < 8) {
          window.UI.toast('Mật khẩu mới phải có ít nhất 8 ký tự', 'warn');
          return;
        }
        if (newPw !== confirmPw) {
          window.UI.toast('Mật khẩu nhập lại không khớp', 'warn');
          return;
        }
        try {
          await window.Auth.changePassword(oldPw, newPw);
        } catch (err) {
          window.UI.toast(err && err.message ? err.message : 'Đổi mật khẩu thất bại', 'danger');
          return;
        }
        window.UI.toast('Đã đổi mật khẩu', 'ok');
        form.reset();
      });
    }
  };

  window.Settings = Settings;
})();
