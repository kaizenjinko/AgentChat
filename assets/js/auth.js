(function () {
  const Auth = {
    async login(username, password) {
      const data = await window.API.post('/api/auth/login', {
        username,
        password
      });
      return data && data.user ? data.user : data;
    },
    async logout() {
      try {
        await window.API.post('/api/auth/logout');
      } catch (e) {}
      window.location.href = '/login.html';
    },
    async me() {
      const data = await window.API.get('/api/auth/me');
      return data && data.user ? data.user : data;
    },
    async changePassword(oldPw, newPw) {
      return window.API.post('/api/auth/change-password', {
        old_password: oldPw,
        new_password: newPw
      });
    },
    async requireAuth() {
      try {
        return await Auth.me();
      } catch (e) {
        if (e && e.status === 401) {
          window.location.href = '/login.html';
        }
        throw e;
      }
    }
  };

  window.Auth = Auth;
})();
