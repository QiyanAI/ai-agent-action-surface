// Connect wizard: auto-detect Claude Code, preview the exact file change,
// then apply (which runs the live enforcement probe first). Technical
// details are always reachable, but the headline language is plain, and the
// measured protection tier is never softened. Nothing is written before the
// user applies.
"use strict";

function currentInputs() {
  return {
    projectDir: document.getElementById("project").value.trim(),
    claudeBin: (document.getElementById("claude-bin").value.trim() || undefined),
  };
}

async function renderDetect() {
  const line = document.getElementById("cc-status");
  clear(line);
  line.append(t("cc_checking"));
  let data;
  try {
    data = await api("GET", "/v1/detect/claude");
  } catch (e) {
    clear(line);
    return;
  }
  clear(line);
  if (data.found) {
    line.append(tf("connect_status_found", { version: data.version, path: data.binary }));
  } else {
    line.append(t("connect_status_missing") + " ");
    line.append(el("a", { href: "https://claude.com/claude-code", target: "_blank", rel: "noreferrer" }, t("cc_install")));
  }
}

// techDetails renders the raw change lines the server plans to write, kept
// available under a disclosure instead of dominating the page.
function techDetails(lines) {
  const det = el("details", { class: "block" });
  det.append(el("summary", {}, t("connect_tech_details")));
  const list = el("ul", {});
  for (const ch of lines) list.append(el("li", { class: "mono" }, String(ch)));
  det.append(list);
  return det;
}

// probeEvidenceLines translates the three live-check results; the raw text
// stays as the evidence value.
function probeEvidenceLines(probe) {
  const names = {
    "deny-blocked": t("connect_evidence_deny"),
    "failure-blocks": t("connect_evidence_failure"),
    "allow-runs": t("connect_evidence_allow"),
  };
  const ul = el("ul", {});
  for (const line of probe.evidence || []) {
    const s = String(line);
    const key = s.split(":")[0];
    const label = names[key] || key;
    ul.append(el("li", {}, label + " — ", el("span", { class: "mono muted" }, s)));
  }
  return ul;
}

function tierBadge(tier) {
  const cls = tier === "enforced" ? "ok" : tier === "blocking-only" ? "warn" : "bad";
  return el("span", { class: "badge " + cls }, tierLabel(tier));
}

async function doPreview() {
  const area = document.getElementById("plan-area");
  const result = document.getElementById("result-area");
  clear(result);
  clear(area);
  const req = currentInputs();
  if (!req.projectDir) return;
  let data;
  try {
    data = await api("POST", "/v1/connect/preview", req);
  } catch (e) {
    area.append(el("div", { class: "notice bad" }, e.message));
    return;
  }
  const det = data.detected || {};
  const plan = data.plan || {};
  const card = el("div", { class: "card" });
  card.append(
    el("div", { class: "spread" },
      el("strong", {}, t("connect_detected") + ": Claude Code " + (det.Version || "?")),
      el("span", { class: "mono muted" }, det.Binary || "")),
  );
  card.append(el("p", {}, t("connect_plan_plain")));
  card.append(el("p", { class: "mono" }, t("connect_settings_file") + ": " + (plan.settingsPath || "")));
  card.append(el("p", { class: "muted" }, t("connect_tier_version") + ": " + tierImpliedLabel(det.TierFromVersion) +
    " · " + t("connect_floor") + ": " + (data.supportFloor || "")));
  card.append(techDetails(plan.changes || plan.Changes || []));
  if (plan.noChanges) {
    card.append(el("p", { class: "badge ok" }, t("connect_already")));
    card.append(el("p", {}, el("a", { class: "go", href: "mission.html?project=" + encodeURIComponent(req.projectDir) }, t("connect_go_mission"))));
  } else {
    card.append(el("p", { class: "muted" }, t("connect_probe_note")));
    const applyBtn = el("button", { class: "primary" }, t("connect_apply"));
    applyBtn.addEventListener("click", async () => {
      applyBtn.disabled = true;
      applyBtn.textContent = t("connect_probe_wait");
      try {
        const out = await api("POST", "/v1/connect/apply", Object.assign({}, req, { allowUnprobed: false }));
        showConnected(out);
      } catch (e) {
        if (e.status === 409) {
          showWeakChoice(req, e.body || {});
        } else {
          showResult(e.message, "bad");
        }
        applyBtn.disabled = false;
        applyBtn.textContent = t("connect_apply");
      }
      refreshInstalled();
    });
    card.append(el("div", { class: "row" }, applyBtn));
  }
  area.append(card);
}

// showConnected renders the measured outcome in plain language, raw probe
// evidence one disclosure away.
function showConnected(out) {
  const result = document.getElementById("result-area");
  clear(result);
  const inst = out.installation || {};
  const card = el("div", { class: "card" });
  card.append(el("p", { class: "badge ok" }, t("connect_installed")));
  card.append(el("div", { class: "row" }, el("span", {}, t("connect_tier") + ":"), tierBadge(inst.tier)));
  card.append(el("p", {}, tierExplain(inst.tier)));
  const det = el("details", { class: "block" });
  det.append(el("summary", {}, t("connect_probe_evidence")));
  det.append(el("p", { class: "muted" }, t("connect_evidence_note")));
  det.append(probeEvidenceLines(out.probe || {}));
  card.append(det);
  card.append(el("p", {}, el("a", { class: "go", href: "mission.html?project=" + encodeURIComponent(inst.projectDir || "") }, t("connect_go_mission"))));
  result.append(card);
}

function tierExplain(tier) {
  if (tier === "enforced") return t("tier_enforced_what");
  if (tier === "blocking-only") return t("tier_blocking_what");
  return t("tier_unsupported_what");
}

// showWeakChoice is the honest path the API always had: install with the
// weaker tier, only after the user explicitly acknowledges it.
function showWeakChoice(req, body) {
  const result = document.getElementById("result-area");
  clear(result);
  const tier = body.tier || "blocking-only";
  const card = el("div", { class: "card" });
  card.append(el("p", { class: "badge warn" }, t("connect_weak_title")));
  card.append(el("p", {}, tf("connect_weak_expl", { tier: tierLabel(tier) })));
  if (body.probe) {
    const det = el("details", { class: "block" });
    det.append(el("summary", {}, t("connect_probe_evidence")));
    det.append(probeEvidenceLines(body.probe));
    card.append(det);
  }
  const retry = el("button", { type: "button" }, t("connect_weak_retry"));
  retry.addEventListener("click", doPreview);
  const confirm = el("input", { type: "checkbox" });
  const confirmLabel = el("label", { class: "row" }, confirm, t("connect_weak_confirm"));
  const weak = el("button", { type: "button", class: "danger" }, t("connect_weak_install"));
  weak.disabled = true;
  confirm.addEventListener("change", () => { weak.disabled = !confirm.checked; });
  weak.addEventListener("click", async () => {
    weak.disabled = true;
    weak.textContent = t("connect_probe_wait");
    try {
      const out = await api("POST", "/v1/connect/apply", Object.assign({}, req, { allowUnprobed: true }));
      showConnected(out);
    } catch (e) {
      showResult(e.message, "bad");
    }
    refreshInstalled();
  });
  card.append(el("div", { class: "row" }, retry));
  card.append(confirmLabel);
  card.append(el("div", { class: "row" }, weak));
  result.append(card);
}

function showResult(text, cls) {
  const result = document.getElementById("result-area");
  clear(result);
  const card = el("div", { class: "card" });
  card.append(el("p", { class: "badge " + cls }, text));
  result.append(card);
}

async function refreshInstalled() {
  const area = document.getElementById("installed");
  clear(area);
  let data;
  try {
    data = await api("GET", "/v1/connect");
  } catch (e) {
    return;
  }
  const list = data.installations || [];
  if (list.length === 0) {
    area.append(el("p", { class: "muted" }, t("connect_none")));
    return;
  }
  const table = el("table", {});
  table.append(el("tr", {},
    el("th", {}, t("connect_project")),
    el("th", {}, t("connect_version")),
    el("th", {}, t("connect_tier")),
    el("th", {}, "")));
  for (const inst of list) {
    const undo = el("button", { class: "danger" }, t("connect_undo"));
    undo.addEventListener("click", async () => {
      undo.disabled = true;
      try {
        await api("DELETE", "/v1/connect?project=" + encodeURIComponent(inst.projectDir));
        showResult(t("connect_undo_done"), "ok");
      } catch (e) {
        showResult(e.message, "bad");
      }
      refreshInstalled();
    });
    table.append(el("tr", {},
      el("td", {}, el("span", { class: "mono" }, inst.projectDir), el("div", { class: "muted small" }, t("connect_measured_note") + (inst.installedAt ? " · " + new Date(inst.installedAt).toLocaleDateString() : ""))),
      el("td", {}, inst.version),
      el("td", {}, tierBadge(inst.tier)),
      el("td", {}, undo)));
  }
  area.append(el("div", { class: "map" }, table));
}

(async function main() {
  init();
  const ok = await ensureSession(document.getElementById("main"));
  if (!ok) return;
  document.getElementById("preview").addEventListener("click", doPreview);
  const projectInput = document.getElementById("project");
  document.getElementById("project-pick").append(pickerButton(projectInput, {
    onSelect: () => { /* preview stays explicit */ },
  }));
  renderDetect();
  refreshInstalled();
})();
