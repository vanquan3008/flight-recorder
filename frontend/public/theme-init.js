// Runs before first paint so a stored or system theme never flashes the wrong palette.
(function () {
  var theme = "dark";
  try {
    var stored = localStorage.getItem("tracepilot-theme");
    if (stored === "light" || stored === "dark") theme = stored;
    else if (window.matchMedia("(prefers-color-scheme: light)").matches) theme = "light";
  } catch (e) {
    // Storage or matchMedia unavailable: keep the dark default.
  }
  document.documentElement.setAttribute("data-theme", theme);
})();
