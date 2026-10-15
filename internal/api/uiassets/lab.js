// Safety Lab: scan a supported input, read the evidence behind every
// finding, preview a suggestion, apply it, and watch a real verification run
// decide whether the change actually holds.
"use strict";

const labState = { lastScan: null, lastApply: null };

function levelBadge(level) {
  const cls = level === "ENFORCED" ? "ok" : level === "UNKNOWN" ? "bad" : "warn";
  return el("span", { class: "badge " + cls }, level);
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
    return;
  }
  labState.lastScan = result;

  const card = el("div", { class: "card" });
  card.append(el("h3", {}, result.inputKind + " — " + result.inputPath));
  card.append(el("p", { class: "muted" }, result.summary));
  card.append(el("p", { class: "muted" },
    "Findings below are read from files only. No code of the scanned project was executed, no external model was called, and no MCP server was contacted."));

  const table = el("table", {});
  table.append(el("tr", {},
    el("th", {}, "Subject"), el("th", {}, "Capability"),
    el("th", {}, "Evidence"), el("th", {}, "Source"), el("th", {}, "Not covered")));
  for (const f of result.findings || []) {
    table.append(el("tr", {},
      el("td", {}, f.subject),
      el("td", { class: "mono" }, f.capability),
      el("td", {}, levelBadge(f.level)),
      el("td", { class: "mono" }, f.sourceRef),
      el("td", { class: "muted" }, f.boundary || "—")));
  }
  card.append(table);
  if (!result.findings || result.findings.length === 0) {
    card.append(el("p", { class: "badge warn" }, "No findings — the checked input is small, not 'safe'."));
  }
  if (result.unsupported && result.unsupported.length) {
    const ul = el("ul", {});
    for (const u of result.unsupported) ul.append(el("li", {}, u));
    card.append(el("p", {}, "Not analysed:"), ul);
  }
  out.append(card);
  renderSuggestions(result);
}

function renderSuggestions(result) {
  const out = document.getElementById("scan-suggestions");
  clear(out);
  if (!result.suggestions || result.suggestions.length === 0) return;
  out.append(el("h2", {}, "Suggested improvements"));
  for (const s of result.suggestions) {
    const card = el("div", { class: "card" });
    card.append(el("h3", {}, s.title));
    card.append(el("p", { class: "muted" }, s.why));
    if (s.effect) card.append(el("p", {}, "Effect: " + s.effect));
    if (s.limitations) card.append(el("p", { class: "muted" }, "Limits: " + s.limitations));
    if (s.kind === "connect-qiyan") {
      card.append(el("a", { class: "go", href: "connect.html" }, "Open the connect wizard"));
    } else if (s.autoApply && s.kind === "add-deny-rule") {
      card.append(el("p", { class: "mono" }, (s.addDenyRules || []).join(", ")));
      const btn = el("button", { class: "primary" }, "Preview and apply");
      btn.addEventListener("click", () => applySuggestion(s, btn));
      card.append(el("div", { class: "row" }, btn));
    } else {
      card.append(el("p", { class: "badge warn" }, "Advisory only — apply by hand."));
    }
    out.append(card);
  }
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
    card.append(el("h3", {}, "Applied and verified"));
    for (const ch of resp.changes || []) card.append(el("p", { class: "mono" }, ch));
    const table = el("table", {});
    table.append(el("tr", {}, el("th", {}, "Rule"), el("th", {}, "Verified on this machine"), el("th", {}, "Evidence")));
    for (const v of resp.verifications || []) {
      table.append(el("tr", {},
        el("td", { class: "mono" }, v.rule),
        el("td", {}, v.verified
          ? el("span", { class: "badge ok" }, "the command was really refused")
          : el("span", { class: "badge warn" }, "not verified")),
        el("td", { class: "muted" }, v.evidence || v.note || "")));
    }
    if ((resp.verifications || []).length === 0) {
      card.append(el("p", { class: "badge warn" }, "Nothing to verify (already present)."));
    }
    card.append(table);
    const undo = el("button", { class: "danger" }, "Undo this change");
    undo.addEventListener("click", async () => {
      undo.disabled = true;
      try {
        await api("POST", "/v1/lab/undo", { projectDir: projectDir, rules: resp.appliedRules });
        card.append(el("p", { class: "badge ok" }, "Undone. Your other settings were left untouched."));
      } catch (e) {
        card.append(el("p", { class: "badge bad" }, e.message));
      }
    });
    card.append(el("div", { class: "row" }, undo));
    out.append(card);
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
    el("option", { value: "claude-project" }, "Claude Code project folder"),
    el("option", { value: "manifest" }, "Qiyan Agent Manifest (.json)"),
    el("option", { value: "mcp-json" }, "MCP server config (.mcp.json)"));
  const path = el("input", { type: "text", id: "scan-path", placeholder: "D:/work/my-project" });
  const go = el("button", { class: "primary" }, "Inspect");
  go.addEventListener("click", () => runScan());
  card.append(
    el("div", { class: "field" }, el("label", {}, "What should be inspected?"), kind),
    el("div", { class: "field" }, el("label", {}, "Path"), path),
    el("div", { class: "row" }, go));
  area.append(card, el("div", { id: "scan-result" }), el("div", { id: "scan-suggestions" }), el("div", { id: "apply-result" }));
})();
