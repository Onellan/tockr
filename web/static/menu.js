(function () {
  function dropdowns() {
    return Array.from(document.querySelectorAll("[data-dropdown]"));
  }

  function parts(root) {
    return {
      trigger: root.querySelector("[data-dropdown-trigger]"),
      menu: root.querySelector("[data-dropdown-menu]")
    };
  }

  function close(root, restoreFocus) {
    var p = parts(root);
    if (!p.trigger || !p.menu) return;
    p.menu.hidden = true;
    root.classList.remove("open");
    p.trigger.setAttribute("aria-expanded", "false");
    if (restoreFocus) p.trigger.focus();
  }

  function closeAll(except) {
    dropdowns().forEach(function (root) {
      if (root !== except) close(root, false);
    });
  }

  function open(root, focusFirst) {
    var p = parts(root);
    if (!p.trigger || !p.menu) return;
    closeAll(root);
    p.menu.hidden = false;
    root.classList.add("open");
    p.trigger.setAttribute("aria-expanded", "true");
    if (focusFirst) {
      var item = p.menu.querySelector('a, button, [tabindex]:not([tabindex="-1"])');
      if (item) item.focus();
    }
  }

  document.addEventListener("click", function (event) {
    var trigger = event.target.closest("[data-dropdown-trigger]");
    if (trigger) {
      var root = trigger.closest("[data-dropdown]");
      var p = parts(root);
      if (!root || !p.menu) return;
      if (p.menu.hidden) open(root, false);
      else close(root, false);
      return;
    }

    if (!event.target.closest("[data-dropdown]")) closeAll(null);
  });

  document.addEventListener("keydown", function (event) {
    var root = event.target.closest("[data-dropdown]");
    if (event.key === "Escape") {
      dropdowns().forEach(function (item) {
        if (item.classList.contains("open")) close(item, item === root);
      });
    }
    if (event.key === "ArrowDown" && event.target.matches("[data-dropdown-trigger]")) {
      event.preventDefault();
      open(event.target.closest("[data-dropdown]"), true);
    }
  });
})();
