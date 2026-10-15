// Share card: a small, pretty, sanitized local image the user may choose to
// save after a demo or a completed mission.
//
// Privacy is structural, not a promise: the card's text comes only from this
// file's fixed label table and from integers; no path, prompt, file name,
// agent name, reason text or transcript content is ever passed in. The SVG
// is generated in the browser, previewed first, and saved only when the user
// clicks save — there is no upload path and no tracking pixel.
"use strict";

const SHARE_LABEL = {
  en: {
    brand: "Qiyan",
    demoTitle: "My synthetic demo run",
    missionTitle: "Mission report card",
    demoLine: "DEMO — synthetic data, no external AI was called",
    missionLine: "A real run on this computer under Qiyan's control",
    roles: "AI crew",
    steps: "actions shown",
    allowed: "actions allowed",
    blocked: "actions stopped",
    approved: "approved by me",
    denied: "denied by me",
    files: "files produced",
    decisions: "control decisions recorded",
    coverage: "Coverage: see the Coverage table in the console for exactly what is enforced — this card claims nothing more.",
    savedNote: "Saved as a file on this computer. Nothing is shared automatically.",
  },
  zh: {
    brand: "Qiyan",
    demoTitle: "我的演示运行",
    missionTitle: "任务成果卡",
    demoLine: "演示——合成数据，未调用外部 AI",
    missionLine: "在这台电脑上，由 Qiyan 控制的一次真实运行",
    roles: "AI 小队",
    steps: "展示的操作",
    allowed: "被允许的操作",
    blocked: "被阻止的操作",
    approved: "我批准的操作",
    denied: "我拒绝的操作",
    files: "产生的文件",
    decisions: "记录的控制决策",
    coverage: "覆盖范围：以控制台中的 Coverage 表为准——这张卡不宣称更多。",
    savedNote: "已保存为本机文件。不会自动分享任何内容。",
  },
};

// ShareStats carries ONLY integers and a fixed role list. Callers must not
// add strings here; a new field means re-earning the sanitization argument.
function buildShareStats(kind, roles, counts) {
  return {
    kind: kind, // "demo" | "mission"
    roles: roles.slice(0, 3),
    steps: counts.steps | 0,
    allowed: counts.allowed | 0,
    blocked: counts.blocked | 0,
    approved: counts.approved | 0,
    denied: counts.denied | 0,
    files: counts.files | 0,
    decisions: counts.decisions | 0,
  };
}

function shareCardSVG(stats) {
  const L = SHARE_LABEL[currentLang()] || SHARE_LABEL.en;
  const rows = [
    [L.roles, stats.roles.join(" · ")],
    [L.steps, String(stats.steps)],
    [L.allowed, String(stats.allowed)],
    [L.blocked, String(stats.blocked)],
    [L.approved + " / " + L.denied, stats.approved + " / " + stats.denied],
  ];
  if (stats.kind === "mission") {
    rows.push([L.files, String(stats.files)]);
    rows.push([L.decisions, String(stats.decisions)]);
  }
  const height = 150 + rows.length * 34 + 44;
  const esc = (s) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  let body = "";
  rows.forEach((r, i) => {
    const y = 150 + i * 34;
    body += `<text x="40" y="${y}" font-size="16" fill="#5b6675">${esc(r[0])}</text>` +
            `<text x="560" y="${y}" font-size="16" fill="#1c2430" text-anchor="end" font-weight="600">${esc(r[1])}</text>`;
  });
  const title = stats.kind === "demo" ? L.demoTitle : L.missionTitle;
  const line = stats.kind === "demo" ? L.demoLine : L.missionLine;
  const noteY = 150 + rows.length * 34 + 18;
  const svg =
`<svg xmlns="http://www.w3.org/2000/svg" width="600" height="${height}" viewBox="0 0 600 ${height}" font-family="system-ui, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif">
  <rect width="600" height="${height}" fill="#f6f7f9"/>
  <rect x="0" y="0" width="600" height="110" fill="#2f6fed"/>
  <text x="40" y="54" font-size="26" fill="#ffffff" font-weight="700">${esc(L.brand)} — ${esc(title)}</text>
  <text x="40" y="84" font-size="14" fill="#dbe7ff">${esc(line)}</text>
  ${body}
  <text x="40" y="${noteY}" font-size="12" fill="#5b6675">${esc(L.coverage)}</text>
</svg>`;
  return svg;
}

// showShareCard renders the preview and the save button into container.
function showShareCard(container, stats) {
  clear(container);
  const svg = shareCardSVG(stats);
  const blob = new Blob([svg], { type: "image/svg+xml" });
  const url = URL.createObjectURL(blob);
  const preview = el("img", { src: url, alt: "share card preview", class: "share-preview" });
  const save = el("button", { class: "primary" }, t("share_save"));
  save.addEventListener("click", () => {
    const a = document.createElement("a");
    a.href = url;
    a.download = "qiyan-card.svg";
    a.click();
  });
  container.append(
    el("div", { class: "card" },
      el("p", { class: "muted" }, t("share_preview_note")),
      preview,
      el("div", { class: "row" }, save),
      el("p", { class: "muted" }, SHARE_LABEL[currentLang()].savedNote)));
}
