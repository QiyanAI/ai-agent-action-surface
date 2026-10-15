// Mission Control page logic: create a mission, start the fleet, follow the
// run live, decide approvals inline, and read the action map straight from
// recorded decisions.
"use strict";

const state = {
  missionId: "",
  tasks: [],
  runState: "",
  eventsPrinted: 0,
};

const ROLE_KEYS = { planner: "fleet_planner", builder: "fleet_builder", reviewer: "fleet_reviewer" };
const ROLE_DESC = { planner: "role_planner_desc", builder: "role_builder_desc", reviewer: "role_reviewer_desc" };

function roleName(role) { return t(ROLE_KEYS[role] || role); }

function taskStateDot(s) {
  switch (s) {
    case "RUNNING": return "running";
    case "WAITING_APPROVAL": return "waiting";
    case "COMPLETED": return "done";
    case "FAILED": return "failed";
    default: return "";
  }
}

async function createMission() {
  const body = {
    goal: document.getElementById("goal").value.trim(),
    mode: document.getElementById("mode").value,
    scope: {
      roots: [document.getElementById("root").value.trim()],
      allowedKinds: ["file.read", "file.create", "file.modify", "file.delete", "process.exec", "session.local"],
      allowedTools: ["Read", "Write", "Edit", "Bash", "Glob", "Grep", "TodoWrite", "TodoRead", "Skill", "TaskCreate", "TaskGet", "TaskList", "TaskUpdate"],
      maxSteps: Number(document.getElementById("steps").value) || 80,
      maxRuntimeSeconds: (Number(document.getElementById("minutes").value) || 60) * 60,
    },
  };
  if (!body.goal || !body.scope.roots[0]) return;
  const m = await api("POST", "/v1/missions", body);
  state.missionId = m.id;
  history.replaceState(null, "", "?mission=" + encodeURIComponent(m.id));
  document.getElementById("start").disabled = false;
  document.getElementById("run-area").style.display = "";
  await refreshMission();
}

async function startRun() {
  const body = {};
  try {
    await api("POST", "/v1/missions/" + state.missionId + "/run", body);
  } catch (e) {
    // a conflict means it is already running — fine to follow it
  }
  startPolling(followRun, 1200);
  startPolling(refreshMission, 2000);
}

async function refreshMission() {
  if (!state.missionId) return;
  const m = await api("GET", "/v1/missions/" + state.missionId);
  state.tasks = m.tasks || [];
  const stateEl = document.getElementById("state");
  stateEl.textContent = (m.mission && m.mission.state) || "";
  stateEl.className = "badge " + (m.mission && m.mission.state === "COMPLETED" ? "ok" : m.mission && m.mission.state === "FAILED" ? "bad" : "warn");
  renderFleet();
  renderResults();
  return true;
}

function renderFleet() {
  const fleet = document.getElementById("fleet");
  clear(fleet);
  const order = ["planner", "builder", "reviewer"];
  for (const role of order) {
    const task = state.tasks.find((t) => t.role === role);
    const box = el("div", { class: "agent" });
    box.append(el("div", { class: "who" }, roleName(role)));
    box.append(el("p", { class: "muted" }, t(ROLE_DESC[role])));
    if (task) {
      box.append(el("div", {},
        el("span", { class: "dot " + taskStateDot(task.state) }),
        el("span", {}, t("task_state") + ": " + task.state)));
      if (task.artifacts && task.artifacts.length) {
        const ul = el("ul", {});
        for (const a of task.artifacts) ul.append(el("li", { class: "mono" }, a.path));
        box.append(ul);
      }
    } else {
      box.append(el("div", {}, el("span", { class: "dot" }), "—"));
    }
    fleet.append(box);
  }
}

async function followRun() {
  if (!state.missionId) return false;
  let run;
  try {
    run = await api("GET", "/v1/missions/" + state.missionId + "/run");
  } catch (e) {
    return false;
  }
  const tl = document.getElementById("timeline");
  const events = run.events || [];
  for (; state.eventsPrinted < events.length; state.eventsPrinted++) {
    const ev = events[state.eventsPrinted];
    if (ev.kind === "task-state") continue; // the cards already show states
    const line = el("div", { class: "ev" },
      el("span", { class: "t" }, fmtTime(ev.at)),
      el("span", {}, "[" + (ev.role ? roleName(ev.role) : "qiyan") + "] " + (ev.text || ev.kind)));
    tl.append(line);
  }
  state.runState = run.state;
  await refreshApprovals();
  await refreshMap();
  if (run.state !== "running") {
    const done = el("div", { class: "ev" }, el("span", { class: "t" }, fmtTime(run.endedAt)),
      el("span", {}, (run.summary || run.state)));
    tl.append(done);
    await refreshMission();
    return false; // stop polling
  }
  return true;
}

async function refreshApprovals() {
  const area = document.getElementById("approvals");
  clear(area);
  let data;
  try {
    data = await api("GET", "/v1/approvals?state=pending");
  } catch (e) {
    return;
  }
  const list = (data.approvals || []).filter((a) => !state.missionId || a.missionId === state.missionId);
  if (list.length === 0) {
    area.append(el("p", { class: "muted" }, t("none_pending")));
    return;
  }
  for (const ap of list) {
    const card = el("div", { class: "approval" });
    const who = roleName(ap.role) + " · " + (ap.agentName || "");
    card.append(el("div", { class: "q" }, who + ": " + ap.action.tool + " → " + (ap.action.path || ap.action.command || ap.action.url || ap.action.kind)));
    for (const line of reasonText(ap.reasons)) {
      card.append(el("p", {}, line));
    }
    card.append(el("p", { class: "muted" }, t("approval_explainer")));
    const yes = el("button", { class: "primary" }, t("approve_once"));
    const no = el("button", { class: "danger" }, t("deny"));
    const decide = async (approve) => {
      yes.disabled = true; no.disabled = true;
      try {
        await api("POST", "/v1/approvals/" + ap.id + "/decision", { approve: approve });
      } catch (e) { /* the poll will show the outcome */ }
      await refreshApprovals();
    };
    yes.addEventListener("click", () => decide(true));
    no.addEventListener("click", () => decide(false));
    card.append(el("div", { class: "row" }, yes, no,
      el("span", { class: "muted mono" }, ap.id)));
    area.append(card);
  }
}

// renderMap builds the action map from recorded decisions: role → tool →
// outcome counts plus the most recent raw rows. Everything shown has a
// decision behind it.
async function refreshMap() {
  if (state.tasks.length === 0) return;
  const map = document.getElementById("map");
  clear(map);
  let all = [];
  for (const task of state.tasks) {
    try {
      const d = await api("GET", "/v1/decisions?task=" + encodeURIComponent(task.id));
      for (const rec of d.decisions || []) {
        rec._role = task.role;
        all.push(rec);
      }
    } catch (e) { /* keep the rest */ }
  }
  if (all.length === 0) {
    map.append(el("p", { class: "muted" }, "—"));
    return;
  }
  const counts = {};
  for (const rec of all) {
    const key = rec._role + "|" + rec.action.tool + "|" + rec.outcome;
    counts[key] = (counts[key] || 0) + 1;
  }
  const keys = Object.keys(counts).sort();
  for (const key of keys) {
    const [role, tool, outcome] = key.split("|");
    map.append(el("div", { class: "edge" },
      el("span", {}, roleName(role)),
      el("span", { class: "arrow" }, "→"),
      el("span", { class: "mono" }, tool),
      el("span", { class: "arrow" }, "→"),
      outcomeBadge(outcome),
      el("span", { class: "muted" }, "×" + counts[key])));
  }
  const recent = all.slice(-8).reverse();
  const table = el("table", {});
  table.append(el("tr", {}, el("th", {}, ""), el("th", {}, "tool"), el("th", {}, "target"), el("th", {}, "outcome")));
  for (const rec of recent) {
    const target = rec.action.path || rec.action.command || rec.action.url || rec.action.kind || "";
    table.append(el("tr", {},
      el("td", {}, roleName(rec._role)),
      el("td", { class: "mono" }, rec.action.tool),
      el("td", { class: "mono" }, String(target).slice(0, 80)),
      el("td", {}, outcomeBadge(rec.outcome))));
  }
  map.append(el("div", { style: "margin-top:12px" }, table));
}

function renderResults() {
  const area = document.getElementById("results");
  clear(area);
  let any = false;
  const list = el("ul", {});
  for (const task of state.tasks) {
    for (const a of task.artifacts || []) {
      any = true;
      list.append(el("li", {}, el("span", { class: "mono" }, a.path)));
    }
  }
  if (any) {
    area.append(el("p", {}, t("artifacts")), list);
  } else {
    area.append(el("p", { class: "muted" }, "—"));
  }
}

(async function main() {
  init();
  const ok = await ensureSession(document.getElementById("main"));
  if (!ok) return;
  document.getElementById("create").addEventListener("click", () => createMission().catch(reportError));
  document.getElementById("start").addEventListener("click", () => startRun().catch(reportError));

  const params = new URLSearchParams(location.search);
  const existing = params.get("mission");
  if (existing) {
    state.missionId = existing;
    document.getElementById("start").disabled = false;
    document.getElementById("run-area").style.display = "";
    document.getElementById("create-card").style.display = "none";
    await refreshMission();
    startPolling(followRun, 1200);
    startPolling(refreshMission, 2000);
  }
})();

function reportError(e) {
  const area = document.getElementById("approvals");
  area.append(el("div", { class: "notice bad" }, e.message));
}
