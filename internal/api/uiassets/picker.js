// Project picker (R03-01): one shared way to choose the folder Qiyan works
// in, used by Mission Control, Safety Lab and the connect wizard.
//
// Sources of choices:
//   - connected projects, from GET /v1/connect (server-side truth);
//   - recently used projects, remembered in this browser only;
//   - a folder browser backed by GET /v1/fs/list (directories only, the
//     server never lists files and refuses its own state directory);
//   - "create a test workspace" backed by POST /v1/fs/mkdir (one folder).
//
// Choosing a project never grants anything: it only fills the path field.
// The mission scope confirmation, the connect preview and the lab apply
// remain the explicit, server-validated steps.
"use strict";

const RECENT_KEY = "qiyan-recent-projects";
const SANDBOX_DEFAULT = "Qiyan-Sandbox";

function recentProjects() {
  try {
    const raw = localStorage.getItem(RECENT_KEY);
    const list = raw ? JSON.parse(raw) : [];
    return Array.isArray(list) ? list.filter((p) => typeof p === "string") : [];
  } catch (e) {
    return [];
  }
}

function rememberProject(path) {
  if (!path) return;
  const list = recentProjects().filter((p) => p.toLowerCase() !== path.toLowerCase());
  list.unshift(path);
  try { localStorage.setItem(RECENT_KEY, JSON.stringify(list.slice(0, 8))); } catch (e) { /* private mode */ }
}

// pickerButton returns the "Choose a project" button for a form row. It
// toggles the panel below the input it belongs to.
function pickerButton(inputEl, opts) {
  opts = opts || {};
  const btn = el("button", { type: "button" }, t("picker_choose"));
  btn.addEventListener("click", () => togglePicker(inputEl, opts));
  return btn;
}

function togglePicker(inputEl, opts) {
  const host = inputEl.closest(".field") || inputEl.parentElement;
  const open = host.querySelector(".picker-panel");
  if (open) {
    open.remove();
    return;
  }
  const panel = el("div", { class: "picker-panel card" });
  host.append(panel);
  renderPickerPanel(panel, inputEl, opts, { browse: "" });
}

function selectProject(panel, inputEl, opts, path) {
  inputEl.value = path;
  rememberProject(path);
  panel.remove();
  inputEl.dispatchEvent(new Event("change", { bubbles: true }));
  if (opts.onSelect) opts.onSelect(path);
}

async function connectedInstallations() {
  try {
    const data = await api("GET", "/v1/connect");
    return (data && data.installations) || [];
  } catch (e) {
    return [];
  }
}

// tierImpliedLabel describes what a VERSION STRING suggests. It must never
// be phrased as a measurement - only a completed probe yields the measured
// labels (tierLabel).
function tierImpliedLabel(tier) {
  if (tier === "enforced") return t("tier_implied_enforced");
  if (tier === "blocking-only") return t("tier_implied_blocking");
  return t("tier_implied_unsupported");
}

function tierLabel(tier) {
  if (tier === "enforced") return t("tier_enforced");
  if (tier === "blocking-only") return t("tier_blocking");
  return t("tier_unsupported");
}

async function renderPickerPanel(panel, inputEl, opts, state) {
  // A newer render supersedes this one when the user clicks faster than the
  // API answers; stale renders must not append into the fresh panel.
  state.renderSeq = (state.renderSeq || 0) + 1;
  const seq = state.renderSeq;
  clear(panel);
  const installs = await connectedInstallations();
  if (seq !== state.renderSeq) return;
  const recents = recentProjects();
  const connectedPaths = new Set(installs.map((i) => (i.projectDir || "").toLowerCase()));

  // Connected projects (server-side truth).
  if (installs.length > 0) {
    panel.append(el("div", { class: "picker-section" }, t("picker_connected")));
    for (const inst of installs) {
      const row = el("button", { type: "button", class: "picker-row" });
      row.append(
        el("span", { class: "mono picker-path" }, inst.projectDir || ""),
        el("span", { class: "badge " + (inst.tier === "enforced" ? "ok" : "warn") }, tierLabel(inst.tier)),
      );
      row.addEventListener("click", () => selectProject(panel, inputEl, opts, inst.projectDir));
      panel.append(row);
    }
  }

  // Recently used (this browser only).
  const fresh = recents.filter((p) => !connectedPaths.has(p.toLowerCase()));
  if (fresh.length > 0) {
    panel.append(el("div", { class: "picker-section" }, t("picker_recent")));
    for (const p of fresh) {
      const row = el("button", { type: "button", class: "picker-row" });
      row.append(el("span", { class: "mono picker-path" }, p));
      row.addEventListener("click", () => selectProject(panel, inputEl, opts, p));
      panel.append(row);
    }
  }

  // First use: explain how to get a safe workspace.
  if (installs.length === 0 && recents.length === 0) {
    panel.append(el("p", { class: "muted" }, t("picker_none")));
  }

  // A project that does not exist yet: create one test folder.
  const newSection = el("div", { class: "picker-section" });
  newSection.append(el("div", {}, t("picker_new_section")));
  const nameInput = el("input", { type: "text", value: state.browse ? SANDBOX_DEFAULT : "" });
  const createBtn = el("button", { type: "button" }, t("picker_sandbox_create"));
  const createMsg = el("div", { class: "muted" });
  createBtn.addEventListener("click", async () => {
    const parent = (state.browse || "").trim();
    const name = nameInput.value.trim();
    if (!parent || !name) {
      createMsg.textContent = t("picker_sandbox_need");
      return;
    }
    const path = parent.replace(/\/+$/, "") + "/" + name;
    try {
      const data = await api("POST", "/v1/fs/mkdir", { path: path });
      selectProject(panel, inputEl, opts, data.created);
    } catch (e) {
      createMsg.textContent = t("picker_create_failed") + ": " + e.message;
    }
  });
  if (!state.browse) {
    newSection.append(el("p", { class: "muted" }, t("picker_sandbox_hint")));
  } else {
    newSection.append(el("div", { class: "row" }, el("span", { class: "muted" }, t("picker_in") ), el("span", { class: "mono" }, state.browse), el("span", { class: "muted" }, t("picker_name")), nameInput, createBtn));
    newSection.append(createMsg);
  }
  panel.append(newSection);

  // Folder browser.
  const browseSection = el("div", { class: "picker-section" });
  browseSection.append(el("div", {}, t("picker_browse")));
  if (state.browse === "") {
    const places = state.places || [];
    if (!state.places) {
      try {
        const data = await api("GET", "/v1/fs/list");
        state.places = (data && data.places) || [];
      } catch (e) {
        state.places = [];
      }
      if (seq !== state.renderSeq) return;
    }
    for (const p of state.places) {
      const row = el("button", { type: "button", class: "picker-row" });
      row.append(el("span", { class: "picker-path mono" }, p.kind === "drive" ? p.path : t("picker_place_" + p.kind) + " — " + p.path));
      row.addEventListener("click", () => {
        state.browse = p.path;
        renderPickerPanel(panel, inputEl, opts, state);
      });
      browseSection.append(row);
    }
  } else {
    const crumb = el("div", { class: "row picker-crumb" });
    const upBtn = el("button", { type: "button" }, "↑ " + t("picker_up"));
    upBtn.addEventListener("click", () => {
      let p = state.browse.replace(/\/+$/, "");
      const idx = p.lastIndexOf("/");
      p = idx > 0 ? p.slice(0, idx) : p.slice(0, idx + 1);
      if (/^[A-Za-z]:$/.test(p)) p += "/"; // drive root keeps its slash
      state.browse = p;
      renderPickerPanel(panel, inputEl, opts, state);
    });
    const useBtn = el("button", { type: "button", class: "primary" }, t("picker_use_this"));
    useBtn.addEventListener("click", () => selectProject(panel, inputEl, opts, state.browse));
    crumb.append(upBtn, useBtn);
    browseSection.append(crumb);
    browseSection.append(el("div", { class: "mono picker-path" }, state.browse));
    const listBox = el("div", { class: "picker-list" });
    browseSection.append(listBox);
    listBox.append(el("div", { class: "muted" }, t("picker_loading")));
    try {
      const data = await api("GET", "/v1/fs/list?path=" + encodeURIComponent(state.browse));
      if (seq !== state.renderSeq) return;
      if (data.path && data.path !== state.browse) {
        // Adopt the server's canonical spelling (resolved path, drive-root
        // slash) before anything can be selected with the local one.
        state.browse = data.path;
        renderPickerPanel(panel, inputEl, opts, state);
        return;
      }
      clear(listBox);
      for (const e of data.entries || []) {
        const row = el("button", { type: "button", class: "picker-row" });
        row.append(el("span", { class: "picker-path" }, "📁 " + e.name));
        row.addEventListener("click", () => {
          state.browse = e.path;
          renderPickerPanel(panel, inputEl, opts, state);
        });
        listBox.append(row);
      }
      if (!data.entries || data.entries.length === 0) {
        listBox.append(el("div", { class: "muted" }, t("picker_empty_folder")));
      }
      if (data.truncated) {
        listBox.append(el("div", { class: "muted" }, t("picker_truncated")));
      }
    } catch (err) {
      clear(listBox);
      listBox.append(el("div", { class: "badge bad" }, t("picker_load_failed") + ": " + err.message));
    }
  }
  panel.append(browseSection);
}
