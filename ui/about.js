"use strict";
/* LeoLink – "O aplikaci" (click on the logo). Same as in Leonzilla. */

const LL_REPO = "https://github.com/Leontech/LeoLink";
const LL_AUTHOR_WEB = "https://leotech.cz";
// "Report a bug / idea": form on leotech.cz (no GitHub account needed), with the program and version filled in
const llFeedbackUrl = () => `https://leotech.cz/download?fb=leolink&v=${encodeURIComponent(S?.version || "")}&lang=${I18N.lang === "en" ? "en" : "cs"}#leolink`;
const LL_SUPPORT_URL = "https://ko-fi.com/leotechcz";

let leotechLogo = null;
async function leotechSvg() {
  if (leotechLogo === null) {
    try { leotechLogo = await (await fetch("leotech.svg")).text(); } catch { leotechLogo = ""; }
  }
  return leotechLogo;
}

function openUrl(href) {
  api("/api/openurl", { url: href }).catch((e) => toast(e.message, true));
}

// links marked data-url open in the browser of the PC (the window itself is not a browser)
document.addEventListener("click", (e) => {
  const a = e.target.closest("a[data-url]");
  if (!a) return;
  e.preventDefault();
  openUrl(a.href);
});

function dialog(html, cls) {
  const ov = document.createElement("div");
  ov.className = "overlay";
  ov.innerHTML = `<div class="modal ${cls || ""}" role="dialog" aria-modal="true">${html}</div>`;
  document.body.append(ov);
  const close = () => { ov.remove(); document.removeEventListener("keydown", onKey); };
  const onKey = (e) => { if (e.key === "Escape") close(); };
  document.addEventListener("keydown", onKey);
  ov.addEventListener("mousedown", (e) => { if (e.target === ov) close(); });
  ov.querySelectorAll("[data-close]").forEach((b) => (b.onclick = close));
  return ov;
}

async function openAbout() {
  const logo = await leotechSvg();
  const year = Math.max(2026, new Date().getFullYear());
  dialog(`
    <div class="about">
      <button class="plain about-x" data-close title="Zavřít (Esc)">✕</button>
      <div class="about-hero">
        <div class="about-mark"><img src="icon.svg" alt=""></div>
        <div>
          <h3>LeoLink</h3>
          <p class="about-ver" translate="no">${esc(I18N.lang === "en" ? "version" : "verze")} ${esc(S?.version || "")}</p>
          <p class="about-lead">Počítač v Home Assistantu a probuzení připravené pro práci na dálku – s Leonzillou z mobilu.</p>
        </div>
      </div>
      <div class="about-links">
        <a href="${LL_REPO}" data-url><span class="ic">⌥</span><span>Zdrojový kód na GitHubu</span></a>
        <a href="${LL_REPO}/releases/latest" data-url><span class="ic">⬆</span><span>Nejnovější verze</span></a>
        <a href="https://github.com/Leontech/leonzilla" data-url><span class="ic">◆</span><span>Leonzilla – ovládání projektů z PC i z mobilu</span></a>
        <a href="${esc(llFeedbackUrl())}" data-url><span class="ic">?</span><span>Nahlásit chybu / nápad</span></a>
      </div>
      <a class="about-support" href="${LL_SUPPORT_URL}" data-url><span class="cup">☕</span><span><b>Podpořit vývoj</b><small>LeoLink je zdarma. Když se ti hodí, můžeš autorovi koupit kafe.</small></span></a>
      <a class="about-author" href="${LL_AUTHOR_WEB}" data-url title="leotech.cz">
        <span class="about-by">Vytvořil</span>
        <span class="about-logo">${logo || "LEOTECH"}</span>
        <span class="about-web">vývoj hardwaru, firmwaru a softwaru · <u translate="no">leotech.cz</u></span>
      </a>
      <p class="about-copy" translate="no">© ${year} Leoš Vilím · ${I18N.lang === "en" ? "MIT License" : "licence MIT"}</p>
    </div>`, "about-modal");
}
