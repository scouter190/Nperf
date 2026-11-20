'use strict';
// Drawers de Tarjetas de red (T12) y Ajustes (T13 lo completa).
// Todo dato de la red se inserta como texto. BSSID y MAC solo se muestran aquí.
(function () {
  const ADAPTERS_MS = 5000, MONITOR_MS = 1000;
  const KIND_TITLES = { eth: 'Cable (Ethernet)', wifi: 'WiFi', other: 'Otro tipo de enlace', unknown: 'Tipo de enlace desconocido' };

  // "802.11ax (WiFi 6)" / "802.11ac" -> "Wi-Fi 6" / "Wi-Fi 5". Desconocido: se muestra tal cual.
  const GENERATIONS = { be: '7', ax: '6', ac: '5', n: '4' };
  function wifiStandard(s) {
    if (!s) return '';
    const str = String(s);
    const m = /802\.11(be|ax|ac|n)\b/i.exec(str);
    if (!m) return str;
    const gen = GENERATIONS[m[1].toLowerCase()];
    return 'Wi-Fi ' + gen + (gen === '6' && /6\s*E/i.test(str) ? 'E' : '');
  }

  // Sondeo solo con el drawer abierto y la ventana visible. deps se inyecta para poder probarlo sin DOM.
  function createPoller(deps) {
    let adTimer = null, moTimer = null, adBusy = false, moBusy = false, moOn = false;
    function adapters() {
      if (adBusy) return;
      adBusy = true;
      Promise.resolve().then(function () { return deps.nperf.adapters(true); })
        .then(function (v) { deps.onAdapters(v); }, function (e) { deps.onAdaptersError(e); })
        .then(function () { adBusy = false; });
    }
    function monitor() {
      if (moBusy || !deps.active()) return;
      moBusy = true; moOn = true;
      Promise.resolve().then(function () { return deps.nperf.monitor(true); })
        .then(function (v) {
          if (deps.active()) { deps.onMonitor(v); return; }
          // Se cerró con la petición en vuelo: su respuesta puede llegar tras monitor(false); se apaga de nuevo.
          moOn = false;
          return Promise.resolve().then(function () { return deps.nperf.monitor(false); }).then(null, function () {});
        }, function (e) { deps.onMonitorError(e); })
        .then(function () { moBusy = false; });
    }
    function stop() {
      if (adTimer !== null) { deps.clearInterval(adTimer); adTimer = null; }
      if (moTimer !== null) { deps.clearInterval(moTimer); moTimer = null; }
      if (moOn) {
        moOn = false;
        Promise.resolve().then(function () { return deps.nperf.monitor(false); }).then(null, function () {});
      }
    }
    // sync: llamar al abrir/cerrar el drawer y al cambiar la visibilidad de la ventana.
    function sync() {
      if (!deps.active()) { stop(); return; }
      if (adTimer === null) { adapters(); adTimer = deps.setInterval(adapters, ADAPTERS_MS); }
      if (moTimer === null) { monitor(); moTimer = deps.setInterval(monitor, MONITOR_MS); }
    }
    return { sync: sync, stop: stop, load: adapters, isRunning: function () { return adTimer !== null; } };
  }

  // ---- DOM ----
  function mk(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined && text !== null) e.textContent = String(text);
    return e;
  }
  function emptyNode(n) { while (n.firstChild) n.removeChild(n.firstChild); }
  function byId(id) { return document.getElementById(id); }

  const live = {};   // nombre de adaptador -> {in_bps, out_bps} del último punto del monitor
  const openData = {};

  function fmtRate(bps) {
    const n = Number(bps);
    if (!isFinite(n) || bps === null || bps === undefined) return '—';
    const mb = n / 1e6;
    return (mb >= 100 ? mb.toFixed(0) : mb.toFixed(1)).replace('.', ',') + ' Mbps';
  }
  function liveText(name) {
    const p = live[name];
    return p ? fmtRate(p.out_bps) + ' / ' + fmtRate(p.in_bps) : '—';
  }
  function updateLive() {
    const nodes = document.querySelectorAll('[data-live]');
    for (let i = 0; i < nodes.length; i++) nodes[i].textContent = liveText(nodes[i].getAttribute('data-live'));
  }

  function row(dl, k, v, extra) {
    if (v === undefined || v === null || v === '') return;
    const d = mk('div');
    d.appendChild(mk('dt', '', k));
    const dd = mk('dd', '', v);
    if (extra) extra(dd);
    d.appendChild(dd);
    dl.appendChild(d);
  }

  function icon(a) {
    const kind = KIND_TITLES[a.kind] ? a.kind : 'unknown';
    const i = mk('span', 'ico ' + kind); i.title = a.kind_label || KIND_TITLES[kind];
    return i;
  }

  function activeCard(a) {
    const w = a.wifi;
    const card = mk('div', 'surf nic act');
    const h = mk('div', 'nic-h');
    h.appendChild(icon(a));
    h.appendChild(mk('span', 'n', a.name));
    h.appendChild(mk('span', 'tag ok', 'Activa'));
    card.appendChild(h);
    const dl = mk('dl', 'kvg');
    if (w && w.available) {
      row(dl, 'SSID', w.ssid);
      row(dl, 'Banda / canal', [w.band, w.channel].filter(Boolean).join(' · '));
      row(dl, 'Señal', [w.signal, w.rssi].filter(Boolean).join(' · '));
      row(dl, 'Estándar', wifiStandard(w.standard));
    } else if (w) {
      row(dl, 'Wi-Fi', w.reason || 'no disponible');
    }
    row(dl, 'Tasa ↑ / ↓ (en vivo)', liveText(a.name), function (dd) { dd.dataset.live = a.name; });
    const tx = (w && w.available && w.tx_rate) || a.tx_rate, rx = (w && w.available && w.rx_rate) || a.rx_rate;
    row(dl, 'Enlace ↑ / ↓', tx || rx ? (tx || '—') + ' / ' + (rx || '—') : a.speed);
    row(dl, 'IPv4', (a.ipv4 || []).join(', '));
    row(dl, 'Adaptador', a.description);
    if (w && w.available) row(dl, 'BSSID', w.bssid);
    row(dl, 'MAC', a.mac);
    card.appendChild(dl);
    return card;
  }

  function inactiveCard(a) {
    const card = mk('div', 'surf nic dim');
    const h = mk('div', 'nic-h');
    h.appendChild(icon(a));
    h.appendChild(mk('span', 'n', a.name));
    if (a.virtual === true) {
      const t = mk('span', 'tag accent', 'virtual');
      t.title = 'Adaptador virtual (VPN, máquina virtual...): su velocidad es nominal';
      h.appendChild(t);
    } else h.appendChild(mk('span', 'tag', 'desconectada'));
    card.appendChild(h);
    const info = [a.connected ? a.speed : '', (a.ipv4 || []).join(', '), a.description].filter(Boolean).join(' · ');
    if (info) card.appendChild(mk('span', 'muted2 mono', info));
    if (a.mac) card.appendChild(mk('span', 'muted2 mono', 'MAC ' + a.mac));
    return card;
  }

  function renderAdapters(v, core) {
    const n = byId('location-notice');
    if (n) {
      n.hidden = !v.location_denied;
      byId('location-text').textContent = v.location_notice || '';
    }
    const c = byId('adapters');
    emptyNode(c);
    const all = v.adapters || [];
    if (!all.length) { c.appendChild(mk('p', 'muted', 'No se detectaron tarjetas de red.')); return; }
    const act = all.filter(function (a) { return a.connected && a.virtual !== true; });
    const off = all.filter(function (a) { return !(a.connected && a.virtual !== true); });
    if (!act.length) c.appendChild(mk('p', 'muted', 'No hay interfaces físicas activas.'));
    act.forEach(function (a) { c.appendChild(activeCard(a)); });
    if (off.length) {
      const det = mk('details', 'acc'); det.id = 'acc-inactive';
      det.open = !!core.ui.isInactiveAdaptersOpen;
      det.addEventListener('toggle', function () { core.ui.isInactiveAdaptersOpen = det.open; });
      det.appendChild(mk('summary', '', 'Ver ' + off.length + (off.length === 1 ? ' interfaz inactiva o virtual' : ' interfaces inactivas o virtuales')));
      off.forEach(function (a) { det.appendChild(inactiveCard(a)); });
      c.appendChild(det);
    }
    updateLive();
  }

  function monitorPoints(a, key, latest) {
    const out = [];
    (a.points || []).forEach(function (p, i) {
      const x = (p.at_ms - latest) / 1000;
      if (p.gap && i > 0) out.push([x - 0.001, null]);
      out.push([x, p.gap ? null : p[key] / 1e6]);
    });
    return out;
  }

  function renderMonitor(v) {
    const msg = byId('monitor-msg');
    msg.hidden = !!v.available;
    msg.textContent = v.available ? '' : (v.message || 'el monitor de tarjetas no está disponible');
    (v.adapters || []).forEach(function (a) {
      const pts = a.points || [];
      if (pts.length) live[a.name] = pts[pts.length - 1];
    });
    updateLive();
    const core = window.NPerfCore;
    core.renderIf('drawers.monitor', v, function () {
      const c = byId('monitor');
      emptyNode(c);
      let latest = 0;
      (v.adapters || []).forEach(function (a) { (a.points || []).forEach(function (p) { if (p.at_ms > latest) latest = p.at_ms; }); });
      (v.adapters || []).forEach(function (a) {
        const box = mk('div', 'mon-adapter');
        const hd = mk('div', 'card-head');
        hd.appendChild(icon(a));
        hd.appendChild(mk('span', 'name', a.name));
        hd.appendChild(mk('span', 'card-speed', a.status === 'up' ? 'conectada' : 'desconectada'));
        box.appendChild(hd);
        if (!a.points || !a.points.length) box.appendChild(mk('p', 'muted', 'Sin datos todavía.'));
        else {
          const cb = mk('div', 'chart-box');
          box.appendChild(cb);
          const key = 'mon:' + a.guid;
          const w = window.NPerfChart.render(cb, { title: 'Tráfico de ' + a.name, xLabel: 'Hace', xUnit: 's', yUnit: 'Mbps', decimals: 2, width: 300, height: 190,
            series: [{ name: 'Entrada', points: monitorPoints(a, 'in_bps', latest) }, { name: 'Salida', points: monitorPoints(a, 'out_bps', latest) }] });
          const det = w && w.querySelector && w.querySelector('.chart-data');
          if (det) { det.open = !!openData[key]; det.addEventListener('toggle', function () { openData[key] = det.open; }); }
        }
        c.appendChild(box);
      });
      if (!(v.adapters || []).length && v.available) c.appendChild(mk('p', 'muted', 'No hay tarjetas físicas que monitorizar.'));
    });
  }

  function errText(e) { return String(e && e.message ? e.message : e); }

  function init() {
    const core = window.NPerfCore, shell = window.NPerfShell;
    const poller = createPoller({
      nperf: window.nperf,
      active: function () { return !!shell && shell.state.drawer === 'adapters' && !document.hidden; },
      setInterval: function (f, ms) { return setInterval(f, ms); },
      clearInterval: function (t) { clearInterval(t); },
      onAdapters: function (v) { core.renderIf('drawers.adapters', v, function () { renderAdapters(v, core); }); },
      onAdaptersError: function (e) { emptyNode(byId('adapters')); byId('adapters').appendChild(mk('p', 'err-text', errText(e))); core.resetSig('drawers.adapters'); },
      onMonitor: renderMonitor,
      onMonitorError: function (e) { const m = byId('monitor-msg'); m.hidden = false; m.textContent = errText(e); core.resetSig('drawers.monitor'); }
    });
    document.addEventListener('nperf:drawer', poller.sync);
    document.addEventListener('visibilitychange', poller.sync);
    window.addEventListener('pagehide', poller.stop);
    byId('location-open').addEventListener('click', function () {
      Promise.resolve(window.nperf.openLocationSettings()).then(null, function (e) { core.pushToast(errText(e), 'error'); });
    });
    poller.load(); // carga inicial; después solo con el drawer abierto
  }

  window.NPerfDrawers = { render: function (s) { void s; }, wifiStandard: wifiStandard, createPoller: createPoller };
  if (typeof document !== 'undefined' && document.addEventListener) document.addEventListener('DOMContentLoaded', init);
})();
