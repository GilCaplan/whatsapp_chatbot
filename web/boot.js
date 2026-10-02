// Runs synchronously before first paint: apply the cached theme override so
// there is no light→dark flash. The real source of truth is Settings.theme.
(function () {
  try {
    var t = localStorage.getItem('doppel.theme');
    if (t === 'light' || t === 'dark') document.documentElement.setAttribute('data-theme', t);
  } catch (e) { /* storage unavailable */ }
})();
