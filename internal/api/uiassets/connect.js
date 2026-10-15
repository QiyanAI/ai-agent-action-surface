// Connect wizard: preview the exact file change, then apply (which runs the
// live enforcement probe first). Nothing is written before the user applies.
"use strict";

function currentInputs() {
  return {
    projectDir: document.getElementById("project").value.trim(),
    claudeBin: document.getElementById("claude-bin").value.trim() || undefined,
  };
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
    el("p", {}, t("connect_tier_version") + ": " + (det.TierFromVersion || "?") +
       " · " + t("connect_floor") + ": " + (data.supportFloor || "")),
    el("p", { class: "mono" }, t("connect_settings_file") + ": " + (plan.settingsPath || "")),
  );
  const list = el("ul", {});
  for (const ch of plan.changes || plan.Changes || []) {
    list.append(el("li", {}, String(ch)));
  }
  card.append(list);
  if (plan.noChanges) {
    card.append(el("p", { class: "badge ok" }, t("connect_already")));
  } else {
    card.append(el("p", { class: "muted" }, t("connect_probe_note")));
    const applyBtn = el("button", { class: "primary" }, t("connect_apply"));
    applyBtn.addEventListener("click", async () => {
      applyBtn.disabled = true;
      applyBtn.textContent = "…";
      try {
        const out = await api("POST", "/v1/connect/apply", Object.assign({}, req, { allowUnprobed: false }));
        renderInstalled(out);
        showResult(t("connect_installed"), "ok", out);
      } catch (e) {
        showResult(e.message, "bad", e.body);
        applyBtn.disabled = false;
        applyBtn.textContent = t("connect_apply");
      }
      refreshInstalled();
    });
    card.append(el("div", { class: "row" }, applyBtn));
  }
  area.append(card);
}

function showResult(text, cls, extra) {
  const result = document.getElementById("result-area");
  clear(result);
  const card = el("div", { class: "card" });
  card.append(el("p", { class: "badge " + cls }, text));
  if (extra && extra.probe) {
    card.append(el("p", {}, t("connect_tier") + ": " + ((extra.installation && extra.installation.tier) || "?")));
    const ev = el("ul", {});
    for (const line of extra.probe.evidence || []) ev.append(el("li", { class: "mono" }, String(line)));
    card.append(el("p", {}, t("connect_probe_evidence")), ev);
  }
  result.append(card);
}

function renderInstalled(out) {
  if (out && out.installed) showResult(t("connect_installed"), "ok", out);
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
        showResult(t("connect_undo_done"), "ok", null);
      } catch (e) {
        showResult(e.message, "bad", null);
      }
      refreshInstalled();
    });
    table.append(el("tr", {},
      el("td", { class: "mono" }, inst.projectDir),
      el("td", {}, inst.version),
      el("td", {}, el("span", { class: "badge " + (inst.tier === "enforced" ? "ok" : "warn") }, inst.tier)),
      el("td", {}, undo)));
  }
  area.append(el("div", { class: "map" }, table));
}

(async function main() {
  init();
  const ok = await ensureSession(document.getElementById("main"));
  if (!ok) return;
  document.getElementById("preview").addEventListener("click", doPreview);
  refreshInstalled();
})();
