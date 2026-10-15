// Safety Lab page. Implemented in the safety-lab phase of this same build;
// until then the page states plainly that it is not available rather than
// pretending to be.
"use strict";

(async function main() {
  init();
  const ok = await ensureSession(document.getElementById("main"));
  if (!ok) return;
  const area = document.getElementById("lab-area");
  area.append(el("div", { class: "notice warn" }, "Not available in this build yet — the Safety Lab arrives with the next phase of this same delivery."));
})();
