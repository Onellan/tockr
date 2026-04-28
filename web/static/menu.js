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
        var hasSelectableOption = false;
        Array.from(select.options).forEach(function (option) {
          if (!option.value) {
            option.hidden = false;
            option.disabled = false;
            return;
          }
          var optionValue = option.getAttribute("data-" + optionAttr);
      var matchesParent = optionValue === parentValue;
      if (!matchesParent && optionValue && optionValue.indexOf(",") !== -1) {
      matchesParent = optionValue.split(",").map(function (value) {
        return value.trim();
      }).indexOf(parentValue) !== -1;
      }
      var visible = !parentValue || !optionValue || matchesParent;
          option.hidden = !visible;
          option.disabled = !visible;
          if (visible) {
            hasSelectableOption = true;
          }
        });
        if (selectedOption && selectedOption.disabled) {
          select.value = "";
        }
        if (!hasSelectableOption) {
          // Prevent stale hidden values from being submitted for parent selections
          // that have no available child options.
          select.value = "";
          select.disabled = true;
          select.setAttribute("aria-disabled", "true");
        } else {
          select.disabled = false;
          select.removeAttribute("aria-disabled");
        }
      }

      parent.addEventListener("change", refreshOptions);
      refreshOptions();
    });
  }

  setupDependentSelectors();

  function setupTimesheetEntryModes() {
    document.querySelectorAll("[data-entry-mode]").forEach(function (root) {
      var form = root.closest("form");
      if (!form) return;
      var radios = Array.from(root.querySelectorAll('input[name="entry_mode"]'));
      var panels = Array.from(form.querySelectorAll("[data-entry-mode-panel]"));
      if (!radios.length || !panels.length) return;

      function selectedMode() {
        var checked = radios.find(function (radio) {
          return radio.checked;
        });
        return checked ? checked.value : "manual";
      }

      function refresh() {
        var mode = selectedMode();
        panels.forEach(function (panel) {
          var active = panel.getAttribute("data-entry-mode-panel") === mode;
          panel.hidden = !active;
          panel.querySelectorAll("input, select, textarea").forEach(function (input) {
            input.disabled = !active;
            if (input.hasAttribute("data-required")) {
              input.required = active;
            }
          });
        });
      }

      radios.forEach(function (radio) {
        radio.addEventListener("change", refresh);
      });
      refresh();
    });
  }

  setupTimesheetEntryModes();

  function setupSMTPPortSuggestion() {
    var defaultsByEncryption = {
      none: "25",
      starttls: "587",
      ssl_tls: "465"
    };

    document.querySelectorAll("[data-smtp-encryption-select]").forEach(function (select) {
      var form = select.closest("form") || document;
      var portInput = form.querySelector("[data-smtp-port-input]");
      var helpText = form.querySelector("[data-smtp-port-help]");
      if (!portInput) return;

      var userEditedPort = false;
      var updatingPortProgrammatically = false;

      function encryptionKey() {
        return String(select.value || "").trim().toLowerCase();
      }

      function suggestedPort() {
        return defaultsByEncryption[encryptionKey()] || "";
      }

      function setHelpText(port) {
        if (!helpText) return;
        if (!port) {
          helpText.textContent = "Enter provider port manually to match the selected encryption type.";
          return;
        }
        helpText.textContent = "Suggested default for " + String(select.options[select.selectedIndex].text || select.value) + ": " + port + ". Existing value is preserved unless you edit it.";
      }

      function refreshSuggestion() {
        var suggested = suggestedPort();
        setHelpText(suggested);
        if (!suggested) return;
        if (userEditedPort && portInput.value.trim() !== "") return;
        updatingPortProgrammatically = true;
        portInput.value = suggested;
        updatingPortProgrammatically = false;
      }

      portInput.addEventListener("input", function () {
        if (updatingPortProgrammatically) return;
        userEditedPort = portInput.value.trim() !== "";
      });

      select.addEventListener("change", refreshSuggestion);
      refreshSuggestion();
    });
  }

  setupSMTPPortSuggestion();

  function setupMobileNav() {
    var shell = document.querySelector("[data-app-shell]");
    var sidebar = document.getElementById("app-sidebar");
    var toggle = document.querySelector("[data-mobile-nav-toggle]");
    var closeButtons = Array.from(document.querySelectorAll("[data-mobile-nav-close]"));
    var backdrop = document.querySelector(".mobile-nav-backdrop");
    if (!shell || !sidebar || !toggle) return;

    function setOpen(open) {
      shell.classList.toggle("nav-open", open);
      document.body.classList.toggle("nav-open", open);
      toggle.setAttribute("aria-expanded", open ? "true" : "false");
      if (backdrop) backdrop.hidden = !open;
      if (open) {
        var firstLink = sidebar.querySelector("a, button");
        if (firstLink) firstLink.focus();
      } else {
        toggle.focus();
      }
    }

    toggle.addEventListener("click", function () {
      setOpen(toggle.getAttribute("aria-expanded") !== "true");
    });

    closeButtons.forEach(function (button) {
      button.addEventListener("click", function () {
        setOpen(false);
      });
    });

    sidebar.addEventListener("click", function (event) {
      if (event.target.closest("a") && window.matchMedia("(max-width: 920px)").matches) {
        setOpen(false);
      }
    });

    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && shell.classList.contains("nav-open")) {
        setOpen(false);
      }
    });
  }

  setupMobileNav();

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
    var closeDetailsButton = event.target.closest("[data-close-details]");
    if (closeDetailsButton) {
      var details = closeDetailsButton.closest("details");
      if (details) details.removeAttribute("open");
      return;
    }

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

  document.addEventListener("submit", function (event) {
    var form = event.target;
    if (!(form instanceof HTMLFormElement)) return;
    var message = form.getAttribute("data-confirm");
    if (!message) return;
    if (!window.confirm(message)) event.preventDefault();
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
