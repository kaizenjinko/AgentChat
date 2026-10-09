(function () {
  async function request(method, url, body) {
    const opts = {
      method,
      credentials: 'same-origin',
      headers: {}
    };
    if (body !== undefined) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    let res;
    try {
      res = await fetch(url, opts);
    } catch (e) {
      const err = new Error('network error');
      err.status = 0;
      throw err;
    }
    let data = null;
    const text = await res.text();
    if (text) {
      try {
        data = JSON.parse(text);
      } catch (e) {
        data = null;
      }
    }
    if (!res.ok) {
      if (res.status === 401 && !url.includes('/api/auth/login')) {
        window.location.href = '/login.html';
      }
      let msg = 'request failed';
      if (data && typeof data.error === 'string') msg = data.error;
      const err = new Error(msg);
      err.status = res.status;
      err.data = data;
      throw err;
    }
    return data;
  }

  const API = {
    get(url) {
      return request('GET', url);
    },
    post(url, body) {
      return request('POST', url, body === undefined ? {} : body);
    },
    patch(url, body) {
      return request('PATCH', url, body === undefined ? {} : body);
    },
    del(url) {
      return request('DELETE', url);
    }
  };

  window.API = API;
})();
