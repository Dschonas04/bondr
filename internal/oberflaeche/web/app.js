"use strict";

// Der Schlüssel kommt beim Öffnen in der Adresse (#k=…) und bleibt für diesen Tab.
(function () {
  const m = location.hash.match(/k=([0-9a-f]+)/);
  if (m) {
    try { sessionStorage.setItem("bondr", m[1]); } catch (e) {}
    window.bondrSchluessel = m[1];
    history.replaceState(null, "", location.pathname);
  } else {
    try { window.bondrSchluessel = sessionStorage.getItem("bondr") || ""; } catch (e) { window.bondrSchluessel = ""; }
  }
})();

const $ = (id) => document.getElementById(id);
const farben = ["#2f6fdf", "#e08a2c", "#2aa36b", "#a05ad8", "#d94c6a"];
const symbole = { Kabel: "🔌", WLAN: "📶", Handy: "📱", Sonstiges: "🌐" };

async function api(pfad, daten) {
  const opt = { method: daten === undefined ? "GET" : "POST", headers: { "X-Bondr": window.bondrSchluessel } };
  if (daten !== undefined) {
    opt.headers["Content-Type"] = "application/json";
    opt.body = JSON.stringify(daten);
  }
  const r = await fetch(pfad, opt);
  if (!r.ok) throw new Error((await r.text()).trim() || r.statusText);
  return r.status === 204 ? null : r.json();
}

function zahl(x, stellen = 1) {
  return x.toLocaleString("de-DE", { minimumFractionDigits: stellen, maximumFractionDigits: stellen });
}

function groesse(b) {
  if (b >= 1e9) return zahl(b / 1e9, 2) + " GB";
  if (b >= 1e6) return zahl(b / 1e6, 1) + " MB";
  return zahl(b / 1e3, 0) + " KB";
}

function el(tag, klasse, text) {
  const e = document.createElement(tag);
  if (klasse) e.className = klasse;
  if (text !== undefined) e.textContent = text;
  return e;
}

const farbeJeWeg = {};
function farbe(name) {
  if (!(name in farbeJeWeg)) farbeJeWeg[name] = farben[Object.keys(farbeJeWeg).length % farben.length];
  return farbeJeWeg[name];
}

function zeigeWege(wege) {
  const box = $("wege");
  box.replaceChildren();
  $("keinweg").hidden = wege.length > 0;
  const max = Math.max(1, ...wege.map((w) => w.mbit_spitze), ...wege.map((w) => w.mbit_rein));
  for (const w of wege) {
    const zeile = el("div", "weg" + (w.aktiv ? "" : " aus"));
    zeile.append(el("div", "symbol", symbole[w.art] || "🌐"));
    const mitte = el("div");
    const name = el("div", "name");
    const punkt = el("span", "punkt" + (w.gestoert ? " stoer" : ""));
    name.append(punkt, document.createTextNode(w.name));
    farbe(w.name);
    mitte.append(name);
    const meta = [w.art, w.ip, w.offen + (w.offen === 1 ? " Verbindung" : " Verbindungen"), groesse(w.gb_rein * 1e9) + " empfangen"];
    if (w.mbit_spitze > 0.5) meta.push("Spitze " + zahl(w.mbit_spitze, 0) + " Mbit/s");
    mitte.append(el("div", "meta", meta.join(" · ")));
    if (w.gestoert) mitte.append(el("div", "meta fehler", "gestört: " + (w.fehler || "keine Verbindung")));
    zeile.append(mitte);
    const rate = el("div", "rate");
    const b = el("b", "", zahl(w.mbit_rein));
    rate.append(b, document.createTextNode(" Mbit/s"), el("small", "", "↑ " + zahl(w.mbit_raus) + " Mbit/s"));
    zeile.append(rate);
    const sch = el("label", "schalter");
    sch.title = w.aktiv ? "Weg nicht mehr benutzen" : "Weg benutzen";
    const cb = el("input");
    cb.type = "checkbox";
    cb.checked = w.aktiv;
    cb.setAttribute("aria-label", w.name + " benutzen");
    cb.addEventListener("change", () => api("/api/weg", { name: w.name, an: cb.checked }).then(holen));
    sch.append(cb, el("span"));
    zeile.append(sch);
    const balken = el("div", "balken");
    const i = el("i");
    i.style.width = Math.min(100, (w.mbit_rein / max) * 100) + "%";
    balken.append(i);
    zeile.append(balken);
    box.append(zeile);
  }
}

function zeigeDownloads(liste) {
  const box = $("downloads");
  box.replaceChildren();
  for (const d of liste) {
    const z = el("div", "dl");
    const kopf = el("div", "kopf");
    const name = (d.datei || d.url).split(/[\\/]/).pop();
    kopf.append(el("div", "datei", name));
    let txt = groesse(d.geladen) + (d.groesse > 0 ? " von " + groesse(d.groesse) : "");
    txt += " · " + zahl(d.mbit, 0) + " Mbit/s";
    if (d.status !== "läuft") txt += " · " + d.status;
    kopf.append(el("div", "zahlen", txt));
    z.append(kopf);
    const ant = el("div", "anteile");
    const gesamt = d.groesse > 0 ? d.groesse : Math.max(1, d.geladen);
    const legende = el("div", "legende");
    for (const w of (d.wege || []).sort((a, b) => b.bytes - a.bytes)) {
      const i = el("i");
      i.style.width = (w.bytes / gesamt) * 100 + "%";
      i.style.background = farbe(w.name);
      ant.append(i);
      const l = el("span");
      const q = el("b");
      q.style.background = farbe(w.name);
      l.append(q, document.createTextNode(w.name + " " + zahl(d.geladen ? (w.bytes / d.geladen) * 100 : 0, 0) + " %"));
      legende.append(l);
    }
    z.append(ant, legende);
    if (d.fehler) z.append(el("p", "fehler klein", d.fehler));
    const knoepfe = el("div", "legende");
    if (d.status === "läuft") {
      const k = el("button", "link", "abbrechen");
      k.onclick = () => api("/api/abbrechen?id=" + d.id, {}).then(holen);
      knoepfe.append(k);
    } else if (d.status === "fertig") {
      const k = el("button", "link", "im Ordner zeigen");
      k.onclick = () => api("/api/zeigen?id=" + d.id, {});
      knoepfe.append(k);
    }
    z.append(knoepfe);
    box.append(z);
  }
}

let beschaeftigt = false;
async function holen() {
  if (beschaeftigt) return;
  beschaeftigt = true;
  try {
    const s = await api("/api/stand");
    $("summe").textContent = zahl(s.mbit_rein);
    $("proxyadr").textContent = s.proxy;
    $("ordner").textContent = s.ordner;
    $("version").textContent = "Bondr " + s.version;
    const sp = $("sysproxy");
    if (document.activeElement !== sp) sp.checked = s.systemproxy;
    zeigeWege(s.wege || []);
    zeigeDownloads(s.downloads || []);
  } catch (e) {
    $("summe").textContent = "?";
  } finally {
    beschaeftigt = false;
  }
}

$("sysproxy").addEventListener("change", (e) => {
  api("/api/systemproxy", { an: e.target.checked }).catch((f) => alert(f.message)).then(holen);
});

$("ladeform").addEventListener("submit", (e) => {
  e.preventDefault();
  $("ladefehler").hidden = true;
  api("/api/laden", { url: $("url").value })
    .then(() => { $("url").value = ""; holen(); })
    .catch((f) => { $("ladefehler").textContent = f.message; $("ladefehler").hidden = false; });
});

$("ordnerneu").addEventListener("click", () => {
  const neu = prompt("Ordner für Downloads", $("ordner").textContent);
  if (neu) api("/api/ordner", { ordner: neu }).then(holen);
});

$("beenden").addEventListener("click", () => {
  if (!confirm("Bondr beenden? Der Windows-Proxy wird dabei abgeschaltet.")) return;
  api("/api/beenden", {}).finally(() => { document.body.innerHTML = "<main><section><h2>Bondr</h2><p>Beendet. Der Windows-Proxy ist wieder aus.</p></section></main>"; });
});

holen();
setInterval(holen, 1000);
