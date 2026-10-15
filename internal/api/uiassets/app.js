// Shared console helpers.
//
// Rendering contract: every piece of data on these pages is rendered with
// textContent (via el()/setText), never innerHTML. Agent names, paths and
// reasons are attacker-influenced strings; the CSP forbids inline script, and
// this file's helpers keep data out of markup entirely. Do not add any
// innerHTML-with-data call here or in page scripts.
"use strict";

// sessionState holds the CSRF token once signed in.
const sessionState = { csrf: sessionStorage.getItem("qiyan-csrf") || "" };

// api() wraps fetch for the versioned control API: same-origin, session
// cookie, and the CSRF header on state-changing methods.
async function api(method, path, body) {
  const opts = {
    method: method,
    credentials: "same-origin",
    headers: { "Accept": "application/json" },
  };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  if (sessionState.csrf && method !== "GET") {
    opts.headers["X-Qiyan-CSRF"] = sessionState.csrf;
  }
  const resp = await fetch(path, opts);
  let data = null;
  try { data = await resp.json(); } catch (e) { data = null; }
  if (!resp.ok) {
    const err = new Error((data && data.error && data.error.message) || ("HTTP " + resp.status));
    err.status = resp.status;
    err.body = data;
    throw err;
  }
  return data;
}

// bootstrapSession validates the ?k= one-time link (or a pasted operator
// token) and mints the UI session cookie plus CSRF token.
async function bootstrapSession(k) {
  const data = await api("POST", "/v1/ui/session", { token: k });
  sessionState.csrf = data.csrf;
  sessionStorage.setItem("qiyan-csrf", data.csrf);
  return true;
}

// ensureSession: reads ?k= from the URL once, then verifies the session works.
async function ensureSession(mainEl) {
  const params = new URLSearchParams(location.search);
  const k = params.get("k");
  if (k) {
    try {
      await bootstrapSession(k);
      history.replaceState(null, "", location.pathname);
    } catch (e) {
      renderBootstrap(mainEl, e.message);
      return false;
    }
  }
  try {
    await api("GET", "/v1/health");
  } catch (e) {
    // No session yet: show the sign-in card.
    renderBootstrap(mainEl, "");
    return false;
  }
  if (!sessionState.csrf) {
    // Cookie exists but this tab has no CSRF token (e.g. reloaded session
    // storage): re-bootstrap with a pasted token to regain mutations.
  }
  return true;
}

// renderBootstrap shows the one-time sign-in card (English + Chinese).
function renderBootstrap(mainEl, errText) {
  clear(mainEl);
  const card = el("div", { class: "card" });
  card.append(
    el("h2", {}, t("bootstrap_needed")),
    el("p", { class: "muted" }, t("bootstrap_help")),
  );
  if (errText) {
    card.append(el("p", { class: "badge bad" }, errText));
  }
  const input = el("input", { type: "text", placeholder: "operator token" });
  const btn = el("button", { class: "primary" }, t("bootstrap_button"));
  btn.addEventListener("click", async () => {
    try {
      await bootstrapSession(input.value.trim());
      location.reload();
    } catch (e) {
      errText = e.message;
      renderBootstrap(mainEl, errText);
    }
  });
  card.append(el("div", { class: "row" }, input, btn));
  mainEl.append(card);
}

// el() builds an element; children may be strings (rendered as text) or
// nodes. No attribute value ever becomes markup.
function el(tag, attrs, ...children) {
  const node = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (k === "class") node.className = v;
      else if (k.startsWith("on") && typeof v === "function") node.addEventListener(k.slice(2), v);
      else node.setAttribute(k, String(v));
    }
  }
  for (const c of children.flat()) {
    if (c === null || c === undefined) continue;
    node.append(typeof c === "string" ? document.createTextNode(c) : c);
  }
  return node;
}

function clear(node) { while (node.firstChild) node.removeChild(node.firstChild); }

function fmtTime(iso) {
  if (!iso) return "";
  const d = new Date(iso);
  return d.toLocaleTimeString();
}

// Top navigation, shared by every page.
function renderTop(active) {
  const nav = document.getElementById("topnav");
  if (!nav) return;
  clear(nav);
  const links = [
    ["/ui/", "nav_home"],
    ["/ui/mission.html", "nav_team"],
    ["/ui/lab.html", "nav_lab"],
    ["/ui/dev.html", "nav_dev"],
    ["/ui/demo.html", "nav_demo"],
  ];
  for (const [href, key] of links) {
    const a = el("a", { href: href }, t(key));
    if (active === key) a.className = "active";
    nav.append(a);
  }
}

// reasonLine renders the structured reasons in the current language. The
// facts come from reason codes and parameters; wording is local.
const REASON_TEXT = {
  en: {
    path_outside_scope: "It wanted to touch {path}, outside the folders this task may use.",
    tool_not_in_scope: "This kind of action is not part of what you allowed for this task.",
    write_by_readonly: "This role is read-only and cannot make changes.",
    delete_operation: "Deleting {path} cannot be undone, so it needs your decision first.",
    credential_reference: "It wanted to use a credential; credentials always need your approval.",
    external_network: "It wanted to reach the network; outbound access needs your approval.",
    external_state_change: "It wanted to change something outside this computer.",
    unrecognized_command: "This action could not be safely interpreted, so it needs your review.",
    sensitive_command: "This involves a powerful program ({program}), so it needs your approval.",
    mode_companion: "You are in accompany mode, where changes and programs wait for you.",
    budget_exhausted: "The task has used up its budget.",
    task_cancelled: "The task was cancelled.",
    user_denied: "You denied this action earlier.",
    approval_expired: "The approval expired.",
    action_changed: "What would run differs from what was approved.",
    identity_unknown: "The identity behind this action could not be confirmed.",
    delegation_escalation: "This hand-off would give more authority than the task allows.",
    control_unavailable: "The control service could not decide.",
    no_external_effect: "Session bookkeeping only; no effect outside the agent.",
    user_approved: "You approved this action.",
  },
  zh: {
    path_outside_scope: "它想访问 {path}，位于本任务允许的目录之外。",
    tool_not_in_scope: "此类操作不在你为本任务授权的范围内。",
    write_by_readonly: "该角色为只读，不能执行修改。",
    delete_operation: "删除 {path} 无法撤销，需要先经过你的确认。",
    credential_reference: "它想使用凭据；凭据操作必须经你批准。",
    external_network: "它想访问网络；对外访问必须经你批准。",
    external_state_change: "它想更改本机之外的对象。",
    unrecognized_command: "无法可靠解释该操作，需要你审查。",
    sensitive_command: "该操作涉及高风险程序（{program}），需要你的批准。",
    mode_companion: "当前为陪同模式，修改和程序都会等待你确认。",
    budget_exhausted: "任务的预算已用完。",
    task_cancelled: "任务已被取消。",
    user_denied: "你之前拒绝过该操作。",
    approval_expired: "批准已过期。",
    action_changed: "将执行的内容与已批准的不一致。",
    identity_unknown: "无法确认该操作背后的身份。",
    delegation_escalation: "该交接会让接收方获得超出任务允许的权限。",
    control_unavailable: "控制服务无法作出决策。",
    no_external_effect: "仅为会话内部记录，不影响外部。",
    user_approved: "你已批准该操作。",
  },
};

function reasonText(reasons) {
  const lang = currentLang();
  const table = REASON_TEXT[lang] || REASON_TEXT.en;
  const lines = [];
  for (const r of reasons || []) {
    let text = table[r.code] || r.code;
    for (const [k, v] of Object.entries(r.params || {})) {
      text = text.split("{" + k + "}").join(v);
    }
    // Any placeholder without a parameter collapses cleanly.
    text = text.replace(/\{[^}]*\}/g, "").replace(/\s{2,}/g, " ").trim();
    lines.push(text);
  }
  return lines;
}

function outcomeBadge(outcome) {
  const cls = outcome === "ALLOW" ? "ok" : outcome === "DENY" ? "bad" : outcome === "REQUIRE_APPROVAL" ? "warn" : "";
  return el("span", { class: "badge " + cls }, outcome);
}

// startPolling calls fn now and then every intervalMs until it returns false.
function startPolling(fn, intervalMs) {
  let stopped = false;
  async function tick() {
    if (stopped) return;
    let keep = true;
    try { keep = await fn(); } catch (e) { /* transient; keep polling */ }
    if (!stopped && keep !== false) setTimeout(tick, intervalMs);
  }
  tick();
  return () => { stopped = true; };
}

function init() {
  applyI18N();
  renderTop(document.body.getAttribute("data-page"));
  initLangToggle();
}
