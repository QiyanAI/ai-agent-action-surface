// Guided tours for the three core entries (R03-04): Mission, Safety Lab and
// the developer entry. A tour highlights the real control for each step and,
// where the step is a real action, advances when that action actually
// happens. It never performs approvals or sensitive actions for the user,
// never creates fake tasks, and can always be reopened from the Help entry.
"use strict";

const Tour = (() => {
  let steps = [];
  let idx = 0;
  let card = null;
  let hl = null;
  let page = "";
  let fired = {};

  const TOURS = {
    mission: [
      { sel: "#goal", key: "tour_mission_1" },
      { sel: "#root", key: "tour_mission_2", event: "project-chosen" },
      { sel: "#mode", key: "tour_mission_3" },
      { sel: "#create", key: "tour_mission_4", event: "mission-created" },
      { sel: "#start", key: "tour_mission_5", event: "run-started" },
      { sel: "#fleet", key: "tour_mission_6" },
      { sel: "#approvals", key: "tour_mission_7" },
    ],
    lab: [
      { sel: "#lab-area", key: "tour_lab_1" },
      { sel: "#lab-run", key: "tour_lab_2", event: "scan-done" },
      { sel: "#scan-result", key: "tour_lab_3" },
      { sel: "#scan-result", key: "tour_lab_4" },
      { sel: "#scan-suggestions", key: "tour_lab_5" },
      { sel: "#apply-result", key: "tour_lab_6", event: "apply-done" },
    ],
    dev: [
      { sel: "#dev-explain", key: "tour_dev_1" },
      { sel: "#dev-refhost", key: "tour_dev_2" },
      { sel: "#dev-setup", key: "tour_dev_3" },
      { sel: "#dev-run", key: "tour_dev_4" },
      { sel: "#dev-coverage", key: "tour_dev_5" },
    ],
  };

  function flagKey(name) { return "qiyan-tour-" + name; }

  function clearHighlight() {
    if (hl) { hl.classList.remove("tour-hl"); hl = null; }
  }

  function close() {
    if (card) { card.remove(); card = null; }
    clearHighlight();
    steps = [];
    idx = 0;
  }

  function anchor(sel) {
    const node = document.querySelector(sel);
    if (!node) return null;
    // Only highlight controls that are actually visible right now; a hidden
    // control is still described in the step text.
    if (node.offsetParent === null && sel !== "body") return null;
    return node;
  }

  function render() {
    if (!card) return;
    clear(card);
    const step = steps[idx];
    const target = anchor(step.sel);
    clearHighlight();
    if (target) {
      target.classList.add("tour-hl");
      hl = target;
      target.scrollIntoView({ block: "center", behavior: "smooth" });
    }
    card.append(el("div", { class: "muted" }, tf("tour_step_of", { n: idx + 1, total: steps.length })));
    card.append(el("div", { class: "tour-text" }, t(step.key)));
    if (step.event && !fired[step.event]) {
      const hint = el("div", { class: "muted tour-wait" }, t("tour_wait"));
      card.append(hint);
    }
    const back = el("button", { type: "button" }, t("tour_back"));
    back.disabled = idx === 0;
    back.addEventListener("click", () => { idx--; render(); });
    const next = el("button", { type: "button", class: "primary" },
      idx === steps.length - 1 ? t("tour_finish") : t("tour_next"));
    if (step.event && !fired[step.event]) {
      // The step advances by doing the real action; Next stays available so
      // skipping is always possible.
    }
    next.addEventListener("click", () => {
      if (idx === steps.length - 1) {
        try { localStorage.setItem(flagKey(page), "done"); } catch (e) { /* private mode */ }
        close();
        return;
      }
      idx++;
      render();
    });
    const exit = el("button", { type: "button" }, t("tour_exit"));
    exit.addEventListener("click", () => {
      try { localStorage.setItem(flagKey(page), "dismissed"); } catch (e) { /* private mode */ }
      close();
    });
    card.append(el("div", { class: "row" }, back, next, exit));
  }

  function run() {
    if (!TOURS[page]) return;
    steps = TOURS[page];
    idx = 0;
    fired = {};
    if (card) card.remove();
    card = el("div", { class: "tour-card card" });
    document.body.append(card);
    render();
  }

  // init shows a first-use prompt (once per page) unless a tour is already
  // requested in the URL.
  function init(name) {
    page = name;
    const params = new URLSearchParams(location.search);
    if (params.get("tour") === "1" && TOURS[page]) {
      params.delete("tour");
      const qs = params.toString();
      history.replaceState(null, "", location.pathname + (qs ? "?" + qs : ""));
      run();
      return;
    }
    let seen = "";
    try { seen = localStorage.getItem(flagKey(page)) || ""; } catch (e) { /* private mode */ }
    if (seen || !TOURS[page]) return;
    card = el("div", { class: "tour-card card" });
    card.append(el("div", {}, t("tour_first_prompt")));
    const yes = el("button", { type: "button", class: "primary" }, t("tour_start"));
    yes.addEventListener("click", run);
    const no = el("button", { type: "button" }, t("tour_no_thanks"));
    no.addEventListener("click", () => {
      try { localStorage.setItem(flagKey(page), "dismissed"); } catch (e) { /* private mode */ }
      close();
    });
    card.append(el("div", { class: "row" }, yes, no));
    document.body.append(card);
  }

  // notify is called by page scripts when a real action happened.
  function notify(name) {
    fired[name] = true;
    if (card && steps[idx] && steps[idx].event === name) render();
  }

  return { init, run, notify, close };
})();

// openHelp is the always-available entry to the three tours.
function openHelp() {
  const existing = document.getElementById("help-panel");
  if (existing) { existing.remove(); return; }
  const panel = el("div", { class: "card", id: "help-panel" });
  panel.append(el("h3", {}, t("help_title")));
  const entries = [
    ["mission", "/ui/mission.html", "help_mission"],
    ["lab", "/ui/lab.html", "help_lab"],
    ["dev", "/ui/dev.html", "help_dev"],
  ];
  const here = document.body.getAttribute("data-page");
  for (const [name, href, key] of entries) {
    const pageMap = { mission: "nav_team", lab: "nav_lab", dev: "nav_dev" };
    if (pageMap[name] === here) {
      const btn = el("button", { type: "button" }, t(key));
      btn.addEventListener("click", () => { panel.remove(); Tour.run(); });
      panel.append(el("div", { class: "row" }, btn));
    } else {
      panel.append(el("div", { class: "row" }, el("a", { class: "go", href: href + "?tour=1" }, t(key))));
    }
  }
  panel.append(el("p", { class: "muted" }, t("dev_endpoints_note")));
  const closeBtn = el("button", { type: "button" }, t("tour_exit"));
  closeBtn.addEventListener("click", () => panel.remove());
  panel.append(closeBtn);
  document.body.append(panel);
}
