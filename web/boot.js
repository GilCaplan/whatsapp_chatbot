// Runs synchronously before first paint: apply the cached look and the resolved
// light/dark mode so there is no flash. The source of truth is Settings
// (skin, theme); web/skins.js re-applies it once settings load.
(function () {
  try {
    var s = localStorage.getItem('doppel.skin') || 'glass';
    var t = localStorage.getItem('doppel.theme') || 'system';
    var ONLY = { midnight: 'dark', neon: 'dark', daylight: 'light' }; // keep in sync with web/skins.js
    var m = ONLY[s] || (t === 'light' || t === 'dark' ? t
      : (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'));
    var r = document.documentElement;
    r.setAttribute('data-skin', s);
    r.setAttribute('data-theme', m);
  } catch (e) { /* storage unavailable: the stylesheet's system fallback applies */ }
})();
