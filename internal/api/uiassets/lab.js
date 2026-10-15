// Safety Lab: scan a supported input, read what every finding means in
// plain words (technical evidence one column away), preview a suggestion's
// exact change, apply it, and watch a real verification run decide whether
// the change actually holds. Scanning itself only reads files.
"use strict";

const labState = { lastScan: null, lastApply: null };

function levelBadge(level) {
  const cls = level === "ENFORCED" ? "ok" : level === "UNKNOWN" ? "bad" : "warn";
  const known = t("lab_level_" + level);
  return el("span", { class: "badge " + cls }, known === "lab_level_" + level ? level : known);
}

// plainMeaning answers "what can it do, and why care" in one line, from the
// capability and evidence-level vocabularies only.
function plainMeaning(f) {
  const cap = t("lab_cap_" + f.capability);
  const lvl = t("lab_level_" + f.level);
  return cap + " · " + lvl;
}

function notifyTour(name) {
  if (typeof Tour !== "undefined" && Tour.notify) Tour.notify(name);
}

async function runScan() {
  const kind = document.getElementById("scan-kind").value;
  const path = document.getElementById("scan-path").value.trim();
  const out = document.getElementById("scan-result");
  clear(out);
  if (!path) return;
  let result;
  try {
    result = await api("POST", "/v1/lab/scan", { kind: kind, path: path });
  } catch (e) {
    out.append(el("div", { class: "notice bad" }, e.message));
    out.append(el("p", { class: "muted" }, t("lab_notfound_hint")));
    return;
  }
  labState.lastScan = result;
  rememberProject(path);

  const card = el("div", { class: "card" });
  card.append(el("h3", {}, result.inputKind + " — " + result.inputPath));
  card.append(el("p", { class: "muted" }, result.summary));
  card.append(el("p", { class: "muted" }, t("lab_readonly_note")));

  const table = el("table", {});
  table.append(el("tr", {},
    el("th", {}, t("th_subject")), el("th", {}, t("lab_what_means")),
    el("th", {}, t("th_capability")),
    el("th", {}, t("th_evidence")), el("th", {}, t("th_source")), el("th", {}, t("th_not_covered"))));
  for (const f of result.findings || []) {
    table.append(el("tr", {},
      el("td", {}, f.subject),
      el("td", {}, plainMeaning(f)),
      el("td", { class: "mono" }, f.capability),
      el("td", {}, levelBadge(f.level)),
      el("td", { class: "mono" }, f.sourceRef),
      el("td", { class: "muted" }, f.boundary || "—")));
  }
  card.append(table);
  if (!result.findings || result.findings.length === 0) {
    card.append(el("p", { class: "badge warn" }, t("lab_no_findings")));
  }
  if (result.unsupported && result.unsupported.length) {
    const ul = el("ul", {});
    for (const u of result.unsupported) ul.append(el("li", {}, u));
    card.append(el("p", {}, t("lab_not_analysed")), ul);
  }
  out.append(card);
  renderSuggestions(result);
  notifyTour("scan-done");
}

function renderSuggestions(result) {
  const out = document.getElementById("scan-suggestions");
  clear(out);
  if (!result.suggestions || result.suggestions.length === 0) return;
  out.append(el("h2", {}, t("lab_suggestions")));
  for (const s of result.suggestions) {
    const card = el("div", { class: "card" });
    card.append(el("h3", {}, s.title));
    card.append(el("p", { class: "muted" }, s.why));
    if (s.effect) card.append(el("p", {}, t("lab_effect") + ": " + s.effect));
    if (s.limitations) card.append(el("p", { class: "muted" }, t("lab_limits") + ": " + s.limitations));
    if (s.kind === "connect-qiyan") {
      card.append(el("a", { class: "go", href: "connect.html" }, t("lab_open_wizard")));
    } else if (s.autoApply && s.kind === "add-deny-rule") {
      card.append(el("p", { class: "mono" }, (s.addDenyRules || []).join(", ")));
      const btn = el("button", { class: "primary" }, t("lab_preview"));
      btn.addEventListener("click", () => previewSuggestion(s, card));
      card.append(el("div", { class: "row" }, btn));
    } else {
      card.append(el("p", { class: "badge warn" }, t("lab_advisory")));
    }
    out.append(card);
  }
}

// previewSuggestion shows the exact planned change before anything is
// written, then offers the real apply.
async function previewSuggestion(s, card) {
  const projectDir = document.getElementById("scan-path").value.trim();
  const previewBox = el("div", {});
  card.append(previewBox);
  let resp;
  try {
    resp = await api("POST", "/v1/lab/apply", { projectDir: projectDir, suggestionId: s.id, preview: true });
  } catch (e) {
    previewBox.append(el("div", { class: "notice bad" }, e.message));
    return;
  }
  clear(previewBox);
  previewBox.append(el("p", {}, t("lab_planned_changes") + ":"));
  for (const ch of resp.changes || []) previewBox.append(el("p", { class: "mono" }, ch));
  previewBox.append(el("p", { class: "mono muted" }, t("connect_settings_file") + ": " + (resp.settingsPath || "")));
  const apply = el("button", { class: "primary" }, t("lab_apply_now"));
  const cancel = el("button", {}, t("lab_cancel_preview"));
  cancel.addEventListener("click", () => previewBox.remove());
  apply.addEventListener("click", () => applySuggestion(s, apply));
  previewBox.append(el("div", { class: "row" }, apply, cancel));
}

async function applySuggestion(s, btn) {
  btn.disabled = true;
  const out = document.getElementById("apply-result");
  clear(out);
  const projectDir = document.getElementById("scan-path").value.trim();
  try {
    const resp = await api("POST", "/v1/lab/apply", { projectDir: projectDir, suggestionId: s.id });
    labState.lastApply = resp;
    const card = el("div", { class: "card" });
    card.append(el("h3", {}, t("lab_applied")));
    for (const ch of resp.changes || []) card.append(el("p", { class: "mono" }, ch));
    const table = el("table", {});
    table.append(el("tr", {}, el("th", {}, t("th_rule")), el("th", {}, t("th_verified")), el("th", {}, t("th_evidence"))));
    for (const v of resp.verifications || []) {
      table.append(el("tr", {},
        el("td", { class: "mono" }, v.rule),
        el("td", {}, v.verified
          ? el("span", { class: "badge ok" }, t("lab_verified"))
          : el("span", { class: "badge warn" }, t("lab_not_verified"))),
        el("td", { class: "muted" }, v.evidence || v.note || "")));
    }
    if ((resp.verifications || []).length === 0) {
      card.append(el("p", { class: "badge warn" }, t("lab_nothing_verify")));
    }
    card.append(table);
    const undo = el("button", { class: "danger" }, t("lab_undo"));
    undo.addEventListener("click", async () => {
      undo.disabled = true;
      try {
        await api("POST", "/v1/lab/undo", { projectDir: projectDir, rules: resp.appliedRules });
        card.append(el("p", { class: "badge ok" }, t("lab_undone")));
      } catch (e) {
        card.append(el("p", { class: "badge bad" }, e.message));
      }
    });
    card.append(el("div", { class: "row" }, undo));
    out.append(card);
    notifyTour("apply-done");
  } catch (e) {
    out.append(el("div", { class: "notice bad" }, e.message));
    btn.disabled = false;
  }
}

(async function main() {
  init();
  const ok = await ensureSession(document.getElementById("main"));
  if (!ok) return;

  const area = document.getElementById("lab-area");
  clear(area);
  const card = el("div", { class: "card" });
  const kind = el("select", { id: "scan-kind" },
    el("option", { value: "claude-project" }, t("lab_inputs_claude")),
    el("option", { value: "manifest" }, t("lab_inputs_manifest")),
    el("option", { value: "mcp-json" }, t("lab_inputs_mcp")));
  const pathRow = el("div", { class: "row" });
  const path = el("input", { type: "text", id: "scan-path", placeholder: "D:/work/my-project", class: "flex1" });
  const pickWrap = el("span", { id: "scan-path-pick" });
  pathRow.append(path, pickWrap);
  const go = el("button", { class: "primary", id: "lab-run" }, t("lab_inspect"));
  go.addEventListener("click", () => runScan());
  card.append(
    el("div", { class: "field" }, el("label", {}, t("lab_pick")), kind),
    el("div", { class: "field" }, el("label", {}, t("lab_path")), pathRow),
    el("div", { class: "row" }, go));
  area.append(card, el("div", { id: "scan-result" }), el("div", { id: "scan-suggestions" }), el("div", { id: "apply-result" }));

  // The folder picker only applies to directory scans; file kinds keep a
  // plain path field.
  pickWrap.append(pickerButton(path, {}));
  const syncPicker = () => {
    pickWrap.classList.toggle("hidden", kind.value !== "claude-project");
  };
  kind.addEventListener("change", syncPicker);
  syncPicker();

  if (typeof Tour !== "undefined") Tour.init("lab");
})();
