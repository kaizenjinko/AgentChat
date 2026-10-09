(function () {
  const state = {
    room: 'general',
    lastId: 0,
    oldestId: 0,
    hasMore: true,
    loadingOlder: false,
    es: null,
    user: null
  };

  const Store = {
    state,
    get(key) {
      return state[key];
    },
    set(key, value) {
      state[key] = value;
      return value;
    }
  };

  window.Store = Store;
})();
