// Developer page: endpoint index and live coverage table.
"use strict";

const ENDPOINTS = [
  ["POST", "/v1/agents", "Register an agent (operator). The returned token is shown once."],
  ["GET", "/v1/agents", "List registered agents (operator)."],
  ["POST", "/v1/missions", "Create a mission with its authorized scope (operator)."],
  ["GET", "/v1/missions", "List missions (operator)."],
  ["GET", "/v1/missions/{id}", "Read one mission and its tasks (operator; an agent sees missions it holds a task in)."],
  ["POST", "/v1/missions/{id}/tasks", "Create a task under a mission (operator)."],
  ["GET", "/v1/tasks/{id}", "Read one task (operator; an agent sees its own)."],
  ["POST", "/v1/tasks/{id}/cancel", "Cancel a task (operator)."],
  ["POST", "/v1/sessions", "Bind a runtime session to a task (operator); the task starts."],
  ["POST", "/v1/authorize", "Ask before acting (agent token). Returns ALLOW / DENY / REQUIRE_APPROVAL / UNKNOWN."],
  ["GET", "/v1/approvals?state=pending", "List pending approvals (operator)."],
  ["GET", "/v1/approvals/{id}", "Read one approval, including its state (agent sees its own)."],
  ["POST", "/v1/approvals/{id}/decision", "Approve or deny one pending action (local user only)."],
  ["POST", "/v1/events", "Report what actually happened (agent)."],
  ["GET", "/v1/decisions?task=", "Audited decision trail (operator/cookie session)."],
  ["GET", "/v1/coverage", "Which execution paths are actually enforced (operator/cookie session)."],
  ["GET", "/v1/health", "Liveness and store state (no auth; no data)."],
];

function renderEndpoints() {
  const area = document.getElementById("endpoints");
  clear(area);
  const table = el("table", {});
  table.append(el("tr", {}, el("th", {}, t("th_method")), el("th", {}, t("th_path")), el("th", {}, t("th_purpose"))));
  for (const [m, p, d] of ENDPOINTS) {
    table.append(el("tr", {},
      el("td", {}, el("span", { class: "badge" }, m)),
      el("td", { class: "mono" }, p),
      el("td", {}, d)));
  }
  area.append(table);
}

async function renderCoverage() {
  const area = document.getElementById("dev-coverage");
  clear(area);
  let data;
  try {
    data = await api("GET", "/v1/coverage");
  } catch (e) {
    return;
  }
  const rows = data.coverage || [];
  if (rows.length === 0) {
    area.append(el("p", { class: "muted" }, t("dev_coverage_empty")));
    return;
  }
  const table = el("table", {});
  table.append(el("tr", {}, el("th", {}, t("th_path")), el("th", {}, t("th_state")), el("th", {}, t("th_basis"))));
  for (const r of rows) {
    const cls = r.state === "ENFORCED" ? "ok" : r.state === "OBSERVED" ? "warn" : "bad";
    table.append(el("tr", {},
      el("td", { class: "mono" }, r.path),
      el("td", {}, el("span", { class: "badge " + cls }, r.state)),
      el("td", { class: "muted" }, r.basis)));
  }
  area.append(table);
}

(async function main() {
  init();
  const ok = await ensureSession(document.getElementById("main"));
  if (!ok) return;
  renderEndpoints();
  renderCoverage();
  if (typeof Tour !== "undefined") Tour.init("dev");
})();
