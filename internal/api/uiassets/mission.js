// Mission Control page logic: create a mission, start the fleet, follow the
// run live, decide approvals inline, stop a run, and read results straight
// from recorded decisions. Everything shown is real recorded state; when the
// live run record is unavailable the page says so instead of guessing.
"use strict";

const state = {
  missionId: "",
  tasks: [],
  runState: "",
  runLive: false,
  lastRun: null,
  lastDecisions: null,
  eventsPrinted: 0,
};

const ROLE_KEYS = { planner: "fleet_planner", builder: "fleet_builder", reviewer: "fleet_reviewer" };
const ROLE_DESC = { planner: "role_planner_desc", builder: "role_builder_desc", reviewer: "role_reviewer_desc" };

function roleName(role) { return t(ROLE_KEYS[role] || role); }

function missionStateText(s) { return t("state_mission_" + (s || "UNKNOWN")); }
function taskStateText(s) { return t("state_task_" + (s || "UNKNOWN")); }

function missionBadgeClass(s) {
  if (s === "COMPLETED") return "badge ok";
  if (s === "FAILED") return "badge bad";
  return "badge warn";
}

function taskStateDot(s) {
  switch (s) {
    case "RUNNING": return "running";
    case "WAITING_APPROVAL": return "waiting";
    case "COMPLETED": return "done";
    case "FAILED": return "failed";
    case "CANCELLED": return "failed";
    default: return "";
  }
}

function notifyTour(name) {
  if (typeof Tour !== "undefined" && Tour.notify) Tour.notify(name);
}

function createMsg(text, cls) {
  const box = document.getElementById("create-msg");
  clear(box);
  if (text) box.append(el("div", { class: "notice " + (cls || "warn") }, text));
}

// renderDetect shows whether Claude Code is available on this machine; the
// tier wording is a version hint only — the probe in the wizard is the real
// measurement.
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
    line.append(tf("cc_found_plain", { version: data.version, tier: tierLabel(data.tierFromVersion) }));
  } else {
    line.append(t("cc_not_found") + " ");
    const a = el("a", { href: "https://claude.com/claude-code", target: "_blank", rel: "noreferrer" }, t("cc_install"));
    line.append(a);
  }
  return data;
}

function renderSamples() {
  const box = document.getElementById("goal-samples");
  clear(box);
  box.append(el("span", { class: "muted" }, t("samples_label")));
  for (const key of ["sample_report", "sample_organize", "sample_review"]) {
    const chip = el("button", { type: "button", class: "chip" }, t(key));
    chip.addEventListener("click", () => {
      document.getElementById("goal").value = t(key);
      notifyTour("goal-filled");
    });
    box.append(chip);
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
  if (!body.goal || !body.scope.roots[0]) {
    createMsg(t("mission_need_input"));
    return;
  }
  createMsg("");
  const m = await api("POST", "/v1/missions", body);
  state.missionId = m.id;
  rememberProject(body.scope.roots[0]);
  history.replaceState(null, "", "?mission=" + encodeURIComponent(m.id));
  document.getElementById("start").disabled = false;
  document.getElementById("run-area").style.display = "";
  await refreshMission();
  notifyTour("mission-created");
}

async function startRun() {
  document.getElementById("start").disabled = true;
  let started = true;
  try {
    await api("POST", "/v1/missions/" + state.missionId + "/run", {});
  } catch (e) {
    if (e.status === 409) {
      // already running — fine to follow it
    } else {
      started = false;
      createMsg(e.message, "bad");
    }
  }
  if (!started) {
    document.getElementById("start").disabled = false;
    return;
  }
  createMsg("");
  state.runLive = true;
  document.getElementById("stop").style.display = "";
  notifyTour("run-started");
  startPolling(followRun, 1200);
  startPolling(refreshMission, 2000);
}

async function stopRun() {
  if (!window.confirm(t("mission_stop_confirm"))) return;
  const btn = document.getElementById("stop");
  btn.disabled = true;
  try {
    await api("POST", "/v1/missions/" + state.missionId + "/run/cancel", {});
  } catch (e) {
    createMsg(e.message, "bad");
  }
}

function showStopButton(show) {
  document.getElementById("stop").style.display = show ? "" : "none";
}

async function refreshMission() {
  if (!state.missionId) return;
  const m = await api("GET", "/v1/missions/" + state.missionId);
  state.tasks = m.tasks || [];
  const mstate = (m.mission && m.mission.state) || "";
  const stateEl = document.getElementById("state");
  stateEl.textContent = missionStateText(mstate);
  stateEl.className = missionBadgeClass(mstate);
  renderFleet();
  if (mstate === "COMPLETED" || mstate === "FAILED" || mstate === "CANCELLED") {
    showStopButton(false);
    await renderSummary();
  }
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
        el("span", {}, t("task_state") + ": " + taskStateText(task.state))));
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
    if (state.runLive && state.runState === "running") {
      // The registry is process-lifetime only; say so instead of going quiet.
      const tl = document.getElementById("timeline");
      tl.append(el("div", { class: "notice warn" }, t("run_lost_notice")));
    }
    state.runLive = false;
    showStopButton(false);
    await refreshMission();
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
    if (ev.kind === "approval-pending") notifyTour("approval-shown");
  }
  state.runState = run.state;
  state.lastRun = run;
  await refreshApprovals();
  await refreshMap();
  if (run.state !== "running") {
    state.runLive = false;
    showStopButton(false);
    const done = el("div", { class: "ev" }, el("span", { class: "t" }, fmtTime(run.endedAt)),
      el("span", {}, (run.summary || run.state)));
    tl.append(done);
    await refreshMission();
    notifyTour("run-ended");
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

// outcomeText localizes a decision outcome, splitting "stopped by Qiyan"
// from "denied by you" by the recorded reason — they are different facts.
function outcomeText(rec) {
  if (rec.outcome === "DENY") {
    const denied = (rec.reasons || []).some((r) => r.code === "user_denied");
    return denied ? t("outcome_denied_by_user") : t("outcome_blocked");
  }
  if (rec.outcome === "ALLOW") return t("outcome_allow");
  if (rec.outcome === "REQUIRE_APPROVAL") return t("outcome_waiting");
  return t("outcome_unknown");
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
  state.lastDecisions = all;
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
      el("span", { class: "badge " + (outcome === "ALLOW" ? "ok" : outcome === "DENY" ? "bad" : "warn") }, t("outcome_" + (outcome === "REQUIRE_APPROVAL" ? "waiting" : outcome === "ALLOW" ? "allow" : outcome === "DENY" ? "blocked" : "unknown"))),
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
      el("td", {}, el("span", { class: "badge " + (rec.outcome === "ALLOW" ? "ok" : rec.outcome === "DENY" ? "bad" : "warn") }, outcomeText(rec)))));
  }
  map.append(el("div", { style: "margin-top:12px" }, table));
}

// coverageForRoot finds the recorded probe summary row for this folder, if
// any. Coverage rows carry the project scope in their path (see the connect
// API); no row means "not measured", which the UI states plainly.
async function coverageForRoot(root) {
  if (!root) return null;
  const scope = "(" + root.replace(/\\/g, "/").replace(/\/+$/, "").toLowerCase() + ")";
  try {
    const data = await api("GET", "/v1/coverage");
    for (const row of data.coverage || []) {
      const p = String(row.path || "").toLowerCase();
      if (p.endsWith(scope) && p.includes("#summary")) return row;
    }
  } catch (e) { /* treated as not measured */ }
  return null;
}

function coverageTierText(stateName) {
  if (stateName === "ENFORCED") return t("tier_enforced");
  if (stateName === "OBSERVED") return t("tier_blocking");
  return t("tier_unsupported");
}

// renderSummary is the outcome-first view: what happened, what was produced,
// what was decided, and how protected this folder is — real records only.
async function renderSummary() {
  const area = document.getElementById("results");
  clear(area);
  const run = state.lastRun;
  const missionState = document.getElementById("state").textContent;

  const card = el("div", { class: "card summary" });
  card.append(el("div", { class: "row spread" },
    el("h3", {}, t("summary_title")),
    el("span", { class: "badge demo" }, t("summary_real"))));
  if (run) {
    const start = run.startedAt ? new Date(run.startedAt) : null;
    const end = run.endedAt ? new Date(run.endedAt) : null;
    if (start && end) {
      const secs = Math.max(0, Math.round((end - start) / 1000));
      card.append(el("p", { class: "muted" }, t("summary_duration") + ": " + secs + "s"));
    }
    if (run.summary) card.append(el("p", {}, run.summary));
  }

  // Files produced — recorded per task from allowed decisions.
  const list = el("ul", {});
  let fileCount = 0;
  for (const task of state.tasks) {
    for (const a of task.artifacts || []) {
      fileCount++;
      list.append(el("li", {}, el("span", { class: "mono" }, a.path)));
    }
  }
  if (fileCount > 0) {
    card.append(el("p", {}, t("artifacts") + ":"), list);
  }

  // Decision counts from the recorded trail.
  if (state.lastDecisions) {
    const c = { total: state.lastDecisions.length, allowed: 0, blocked: 0, approved: 0, denied: 0 };
    for (const rec of state.lastDecisions) {
      if (rec.outcome === "ALLOW") c.allowed++;
      if (rec.outcome === "DENY") c.blocked++;
      for (const r of rec.reasons || []) {
        if (r.code === "user_approved") c.approved++;
        if (r.code === "user_denied") c.denied++;
      }
    }
    card.append(el("p", {}, tf("summary_counts", c)));
  }

  // Protection fact for the working folder, from measured coverage rows.
  const rootInput = document.getElementById("root");
  const root = (rootInput && rootInput.value.trim()) || (state.tasks[0] && "") || "";
  const cov = await coverageForRoot(root);
  if (cov) {
    card.append(el("p", {}, tf("summary_coverage", {
      tier: coverageTierText(cov.state),
      date: cov.checkedAt ? new Date(cov.checkedAt).toLocaleDateString() : "",
    })));
  } else {
    card.append(el("p", { class: "muted" }, t("summary_coverage_none")));
  }
  area.append(card);

  // Share card after a completed mission: counts only, from the recorded
  // decisions already shown on this page.
  if (missionState === t("state_mission_COMPLETED") && state.lastDecisions) {
    const counts = { steps: state.lastDecisions.length, allowed: 0, blocked: 0, approved: 0, denied: 0, files: fileCount, decisions: state.lastDecisions.length };
    for (const rec of state.lastDecisions) {
      if (rec.outcome === "ALLOW") counts.allowed++;
      if (rec.outcome === "DENY") counts.blocked++;
      for (const r of rec.reasons || []) {
        if (r.code === "user_approved") counts.approved++;
        if (r.code === "user_denied") counts.denied++;
      }
    }
    const btn = el("button", {}, t("demo_create_card"));
    const box = el("div", {});
    btn.addEventListener("click", () => {
      showShareCard(box, buildShareStats("mission",
        [t("fleet_planner"), t("fleet_builder"), t("fleet_reviewer")], counts));
      btn.disabled = true;
    });
    area.append(el("div", { class: "row" }, btn), box);
  }

  // A fresh start is one click away once the run is over.
  const again = el("a", { class: "go", href: "mission.html" }, t("run_again"));
  area.append(el("p", {}, again));
}

(async function main() {
  init();
  const ok = await ensureSession(document.getElementById("main"));
  if (!ok) return;
  document.getElementById("create").addEventListener("click", () => createMission().catch(reportError));
  document.getElementById("start").addEventListener("click", () => startRun().catch(reportError));
  document.getElementById("stop").addEventListener("click", () => stopRun().catch(reportError));

  // Project picker: connected projects, recents, folder browsing, or a fresh
  // test workspace. Choosing only fills the field — the mission create step
  // is still the explicit confirmation of the scope.
  const rootInput = document.getElementById("root");
  document.getElementById("root-pick").append(pickerButton(rootInput, { onSelect: () => notifyTour("project-chosen") }));

  renderSamples();
  renderDetect();

  const params = new URLSearchParams(location.search);
  const project = params.get("project");
  if (project) {
    rootInput.value = project;
    rememberProject(project);
  }
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
  if (typeof Tour !== "undefined") Tour.init("mission");
})();

function reportError(e) {
  createMsg(e.message, "bad");
}
