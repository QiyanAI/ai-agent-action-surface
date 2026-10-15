// The interactive demo and training player. All content is synthetic and
// served as inert scenario data; the play happens entirely in this page.
// Nothing here calls the mission, decision or approval APIs — playing a
// scenario cannot create a real event, and the page says so.
"use strict";

const play = {
  scenarios: [],
  scenario: null,
  step: 0,
  startedAt: null,
  timerId: null,
  settings: {},
  revealed: [],   // step indices already shown (setting toggle may add more)
  stats: null,
};

function langText(en, zh) {
  return currentLang() === "zh" ? zh : en;
}

function effectBadge(effect) {
  if (effect === "allowed") return el("span", { class: "badge ok" }, t("demo_action_allowed"));
  if (effect === "blocked") return el("span", { class: "badge bad" }, t("demo_action_blocked"));
  return el("span", { class: "badge warn" }, t("demo_action_awaiting"));
}

function startTimer() {
  if (play.timerId) return;
  play.startedAt = Date.now();
  play.timerId = setInterval(() => {
    const el2 = document.getElementById("demo-timer");
    if (el2) el2.textContent = t("demo_elapsed") + ": " + ((Date.now() - play.startedAt) / 1000).toFixed(0) + "s";
  }, 250);
}

function stopTimer() {
  if (play.timerId) clearInterval(play.timerId);
  play.timerId = null;
}

function scenarioVisibleSteps() {
  return play.scenario.steps.map((st, i) => ({ st: st, i: i }))
    .filter((x) => !x.st.requiresSetting || play.settings[x.st.requiresSetting]);
}

async function pickScenario(id) {
  play.scenario = play.scenarios.find((s) => s.id === id) || play.scenarios[0];
  play.step = 0;
  play.startedAt = null;
  play.settings = {};
  stopTimer();
  play.stats = { steps: 0, allowed: 0, blocked: 0, approved: 0, denied: 0 };
  renderPlayer();
}

function renderPlayer() {
  const area = document.getElementById("demo-area");
  clear(area);
  const sc = play.scenario;

  const picker = el("select", {},
    ...play.scenarios.map((s) => el("option", { value: s.id, selected: s.id === sc.id ? "selected" : null }, langText(s.titleEn, s.titleZh))));
  picker.addEventListener("change", () => pickScenario(picker.value));

  const head = el("div", { class: "card" });
  head.append(
    el("div", { class: "row" }, el("span", { class: "badge demo" }, sc.kind === "demo" ? "DEMO" : "TRAINING"),
      picker, el("span", { id: "demo-timer", class: "muted mono" }, "")),
    el("h3", {}, langText(sc.titleEn, sc.titleZh)),
    el("p", { class: "muted" }, langText(sc.introEn, sc.introZh)),
    el("p", { class: "muted" }, t("demo_no_external")));

  const stepsBox = el("div", { class: "timeline", id: "demo-steps" });
  head.append(stepsBox);
  area.append(head);

  if (sc.settingId) {
    const toggle = el("input", { type: "checkbox" });
    toggle.checked = !!play.settings[sc.settingId];
    toggle.addEventListener("change", () => {
      play.settings[sc.settingId] = toggle.checked;
      renderPlayer();
    });
    head.append(el("div", { class: "row" }, toggle,
      el("span", {}, langText(sc.settingEn, sc.settingZh))));
    head.append(el("p", { class: "muted" }, langText(sc.settingNoteEn, sc.settingNoteZh)));
  }

  const visible = scenarioVisibleSteps();
  const shown = visible.slice(0, play.step);
  for (const x of shown) {
    stepsBox.append(renderStep(x.st));
  }

  const controls = el("div", { class: "row" });
  if (play.step < visible.length) {
    const next = el("button", { class: "primary" }, t("demo_next"));
    next.addEventListener("click", () => {
      startTimer();
      play.step++;
      renderPlayer();
      const last = document.getElementById("demo-steps").lastElementChild;
      if (last) last.scrollIntoView({ behavior: "smooth", block: "nearest" });
    });
    controls.append(next);
  } else {
    stopTimer();
    const seconds = play.startedAt ? ((Date.now() - play.startedAt) / 1000).toFixed(0) : "0";
    head.append(el("p", {}, t("demo_finished").replace("{n}", String(seconds))));
    const again = el("button", {}, t("demo_restart"));
    again.addEventListener("click", () => pickScenario(sc.id));
    controls.append(again);
    const toMission = el("a", { class: "go", href: "mission.html" }, t("demo_cta_mission"));
    const toLab = el("a", { class: "go", href: "lab.html" }, t("demo_cta_lab"));
    controls.append(toMission, toLab);
    head.append(controls);

    // Share card (demo kind): counts only.
    const cardBtn = el("button", {}, t("demo_create_card"));
    const cardBox = el("div", { id: "share-card-box" });
    cardBtn.addEventListener("click", () => {
      showShareCard(cardBox, buildShareStats("demo",
        [t("fleet_planner"), t("fleet_builder"), t("fleet_reviewer")], play.stats));
      cardBtn.disabled = true;
    });
    head.append(el("div", { class: "row" }, cardBtn), cardBox);
    return;
  }
  head.append(controls);
}

function renderStep(st) {
  const wrap = el("div", {});
  wrap.append(el("div", { class: "ev" },
    el("span", { class: "t" }, langText(st.roleEn, st.roleZh)),
    el("span", {}, langText(st.textEn, st.textZh))));
  if (st.action) {
    play.stats.steps++;
    if (st.action.effect === "allowed") play.stats.allowed++;
    if (st.action.effect === "blocked") play.stats.blocked++;
    const card = el("div", { class: "card", style: "margin:6px 0 10px 24px" });
    card.append(el("div", { class: "spread" },
      el("span", { class: "mono" }, st.action.tool + " → " + st.action.target),
      effectBadge(st.action.effect)));
    const reasons = currentLang() === "zh" ? st.action.reasonsZh : st.action.reasonsEn;
    for (const r of reasons || []) card.append(el("p", { class: "muted" }, r));
    wrap.append(card);

    if (st.decision) {
      const dec = st.decision;
      const box = el("div", { class: "approval", style: "margin-left:24px" });
      box.append(el("div", { class: "q" }, langText(dec.questionEn, dec.questionZh)));
      const row = el("div", { class: "row" });
      const mk = (label, text, followup, count) => {
        const b = el("button", { class: label === t("approve_once") ? "primary" : "" }, label);
        b.addEventListener("click", () => {
          if (count) count();
          box.append(el("p", {}, text));
          if (followup) {
            play.stats.steps++;
            followup.effect === "allowed" ? play.stats.allowed++ : play.stats.blocked++;
            box.append(el("div", { class: "ev" }, el("span", { class: "mono" }, followup.tool + " → " + followup.target),
              effectBadge(followup.effect)));
          }
        });
        return b;
      };
      row.append(mk(t("approve_once"), langText(dec.approveTextEn, dec.approveTextZh), dec.followupApprove, () => play.stats.approved++));
      row.append(mk(t("deny"), langText(dec.denyTextEn, dec.denyTextZh), dec.followupDeny, () => play.stats.denied++));
      if (dec.timeoutTextEn) {
        row.append(mk(t("demo_timeout"), langText(dec.timeoutTextEn, dec.timeoutTextZh), null, null));
      }
      if (dec.cancelTextEn) {
        row.append(mk(t("demo_cancel"), langText(dec.cancelTextEn, dec.cancelTextZh), null, null));
      }
      box.append(row);
      wrap.append(box);
    }
  }
  if (st.requiresSetting) {
    wrap.append(el("p", { class: "muted" }, t("demo_setting_changed")));
  }
  return wrap;
}

(async function main() {
  init();
  const ok = await ensureSession(document.getElementById("main"));
  if (!ok) return;
  const area = document.getElementById("demo-area");
  let data;
  try {
    data = await api("GET", "/v1/playground/scenarios");
  } catch (e) {
    area.append(el("div", { class: "notice bad" }, e.message));
    return;
  }
  play.scenarios = data.scenarios || [];
  if (play.scenarios.length === 0) {
    area.append(el("div", { class: "notice bad" }, "no scenarios available"));
    return;
  }
  area.append(el("p", { class: "muted" }, data.note || ""));
  await pickScenario("demo");
})();
