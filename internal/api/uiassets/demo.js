// Demo page. Implemented in the playground phase of this same build; until
// then it states plainly that it is not available rather than faking it.
"use strict";

(async function main() {
  init();
  const ok = await ensureSession(document.getElementById("main"));
  if (!ok) return;
  const area = document.getElementById("demo-area");
  area.append(el("div", { class: "notice warn" }, "Not available in this build yet — the interactive demo arrives with the playground phase of this same delivery."));
})();
