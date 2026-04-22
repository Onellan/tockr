(function () {
  var navStorageKey = "tockr.collapsedNavGroups";
  var defaultCollapsedGroups = ["manage", "analyze", "admin"];

  function readNavState() {
    try {
      var value = localStorage.getItem(navStorageKey);
      if (!value) return { collapsed: [], expanded: [] };
      var state = JSON.parse(value);
      if (Array.isArray(state)) {
        return { collapsed: state, expanded: [] };
      }
      return {
        collapsed: Array.isArray(state.collapsed) ? state.collapsed : [],
        expanded: Array.isArray(state.expanded) ? state.expanded : []
      };
    } catch (error) {
      return { collapsed: [], expanded: [] };
    }
  }

  function writeNavState(state) {
    try {
      localStorage.setItem(navStorageKey, JSON.stringify(state));
    } catch (error) {
      return;
    }
  }

  function setSectionCollapsed(section, collapsed) {
    var trigger = section.querySelector("[data-nav-toggle]");
    var items = section.querySelector(".nav-section-items");
    if (!trigger || !items) return;
    section.classList.toggle("collapsed", collapsed);
    items.hidden = collapsed;
    trigger.setAttribute("aria-expanded", collapsed ? "false" : "true");
  }

  function setupNavSections() {
    var state = readNavState();
    document.querySelectorAll("[data-nav-section]").forEach(function (section) {
      var group = section.getAttribute("data-nav-group");
      var hasActiveLink = Boolean(section.querySelector(".nav-link.active"));
      var defaultCollapsed = defaultCollapsedGroups.indexOf(group) !== -1;
      var explicitlyExpanded = state.expanded.indexOf(group) !== -1;
      var explicitlyCollapsed = state.collapsed.indexOf(group) !== -1;
      setSectionCollapsed(section, !hasActiveLink && (explicitlyCollapsed || (defaultCollapsed && !explicitlyExpanded)));
    });
  }

  document.addEventListener("click", function (event) {
    var trigger = event.target.closest("[data-nav-toggle]");
    if (!trigger) return;
    var section = trigger.closest("[data-nav-section]");
    if (!section) return;
    var group = section.getAttribute("data-nav-group");
    var collapsed = trigger.getAttribute("aria-expanded") === "true";
    var state = readNavState();
    state.collapsed = state.collapsed.filter(function (item) {
      return item !== group;
    });
    state.expanded = state.expanded.filter(function (item) {
      return item !== group;
    });
    if (collapsed) state.collapsed.push(group);
    else state.expanded.push(group);
    writeNavState(state);
    setSectionCollapsed(section, collapsed);
  });

  setupNavSections();

  function setupDependentSelectors() {
    document.querySelectorAll("select[data-filter-parent]").forEach(function (select) {
      var form = select.closest("form") || document;
      var parentName = select.getAttribute("data-filter-parent");
      var optionAttr = select.getAttribute("data-filter-attr");
      var parent = form.querySelector('[name="' + parentName + '"]');
      if (!parent || !optionAttr) return;

      function refreshOptions() {
        var parentValue = parent.value;
        var selectedOption = select.options[select.selectedIndex];
        Array.from(select.options).forEach(function (option) {
          if (!option.value) {
            option.hidden = false;
            option.disabled = false;
            return;
          }
          var optionValue = option.getAttribute("data-" + optionAttr);
          var visible = !parentValue || !optionValue || optionValue === parentValue;
          option.hidden = !visible;
          option.disabled = !visible;
        });
        if (selectedOption && selectedOption.disabled) {
          select.value = "";
        }
      }

      parent.addEventListener("change", refreshOptions);
      refreshOptions();
    });
  }

  setupDependentSelectors();

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
