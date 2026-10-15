// Home page: service status strip.
"use strict";

async function refreshStatus() {
  const strip = document.getElementById("status");
  try {
    const health = await api("GET", "/v1/health");
    const approvals = await api("GET", "/v1/approvals?state=pending");
    clear(strip);
    strip.style.display = "";
    const pending = (approvals.approvals || []).length;
    strip.append(
      el("div", { class: "spread" },
        el("span", {},
          el("span", { class: "dot " + (health.status === "ok" ? "done" : "failed") }),
          t("status_title") + ": " + (health.status === "ok" ? t("status_ok") : t("status_degraded")),
        ),
        el("span", { class: "muted mono" }, "v" + health.version),
      ),
    );
    if (health.storeBlocked) {
      strip.append(el("p", { class: "badge bad" }, t("status_blocked_note")));
    }
    if (pending > 0) {
      const link = el("a", { href: "/ui/mission.html" }, t("open_approvals"));
      strip.append(el("div", { class: "row" },
        el("span", { class: "badge warn" }, pending + " " + t("pending_approvals")), link));
    } else {
      strip.append(el("p", { class: "muted" }, t("none_pending")));
    }
  } catch (e) {
    strip.style.display = "none";
  }
}

(async function main() {
  init();
  const ok = await ensureSession(document.getElementById("main"));
  if (!ok) return;
  refreshStatus();
  setInterval(refreshStatus, 3000);
})();
