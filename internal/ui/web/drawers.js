'use strict';
// Drawers de Tarjetas de red (T12) y Ajustes (T13).
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

  // ---- Ajustes (T13) ----
  // La clave es solo escritura: nunca se guarda en un nodo, banner, toast ni registro.
  const MAX_TARGETS = 4;
  const IPV4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/;
  const CIDR = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}\/(3[0-2]|[12]?\d)$/;
  const RESTART_TEXT = 'Guardado · servidor reiniciado';

  function createSettings(core, nperf) {
    const root = mk('div', 'settings-pane');
    let last = null;
    const banner = mk('div', 'alert');
    banner.hidden = true;
    banner.role = 'status';
    root.appendChild(banner);

    function card(title) {
      const c = mk('div', 'surf set');
      const h = mk('h3', '', title);
      c.appendChild(h);
      root.appendChild(c);
      return { el: c, h: h };
    }
    function button(text, cls) { const b = mk('button', cls || 'btn', text); b.type = 'button'; return b; }
    function input(cls, label, type) {
      const i = mk('input', cls); i.type = type || 'text'; i.ariaLabel = label;
      return i;
    }
    function inlineError() { const e = mk('span', 'err-text'); e.hidden = true; e.role = 'alert'; return e; }
    function setErr(e, msg) { e.hidden = !msg; e.textContent = msg || ''; }

    function showBanner(kind, text) { banner.className = 'alert ' + kind; banner.hidden = false; banner.textContent = text; }
    // restart: solo alias, clave y subredes reinician el servidor.
    function run(btn, busyText, restart, call) {
      showBanner('info', restart ? 'Aplicando y reiniciando el servidor…' : 'Guardando…');
      return core.busy(btn, Promise.resolve().then(call), busyText).then(function () {
        showBanner('ok', restart ? RESTART_TEXT : 'Guardado');
        return true;
      }, function (e) {
        showBanner('err', 'No se pudo guardar: ' + errText(e));
        return false;
      });
    }

    // Alias
    const aliasCard = card('Alias');
    const aliasRow = mk('div', 'hrow'), aliasIn = input('inp', 'Alias'), aliasBtn = button('Guardar');
    aliasIn.maxLength = 64;
    aliasRow.appendChild(aliasIn); aliasRow.appendChild(aliasBtn); aliasCard.el.appendChild(aliasRow);
    function aliasRefresh() {
      const v = String(aliasIn.value || '').trim();
      aliasBtn.disabled = !v || (last !== null && v === last.alias) || core.ui.pendingActions.has(aliasBtn);
    }
    aliasIn.addEventListener('input', aliasRefresh);
    aliasBtn.addEventListener('click', function () {
      const v = String(aliasIn.value).trim();
      if (!v) return;
      run(aliasBtn, 'Guardando…', true, function () { return nperf.setAlias(v); }).then(aliasRefresh);
    });

    // Clave
    const keyCard = card('Clave compartida ');
    const keyBadge = mk('span', 'tag', 'No configurada');
    keyCard.h.appendChild(keyBadge);
    keyCard.el.appendChild(mk('p', '', 'Solo escritura; nunca se muestra.'));
    const keyRow = mk('div', 'hrow'), keyIn = input('inp', 'Nueva clave', 'password'), keyBtn = button('Guardar');
    keyIn.placeholder = 'Nueva clave'; keyIn.autocomplete = 'new-password';
    keyRow.appendChild(keyIn); keyRow.appendChild(keyBtn); keyCard.el.appendChild(keyRow);
    function keyRefresh() { keyBtn.disabled = !String(keyIn.value) || core.ui.pendingActions.has(keyBtn); }
    keyIn.addEventListener('input', keyRefresh);
    keyBtn.addEventListener('click', function () {
      const k = String(keyIn.value);
      if (!k) return;
      keyIn.value = '';   // vaciado antes de enviar: el campo nunca conserva la clave
      keyRefresh();
      run(keyBtn, 'Guardando…', true, function () { return nperf.setKey(k); }).then(keyRefresh);
    });

    // Historial
    const histCard = card('Historial local');
    const histLbl = mk('label', 'chk'), histChk = mk('input'); histChk.type = 'checkbox';
    histLbl.appendChild(histChk); histLbl.appendChild(mk('span', '', ' Guardar resultados en el historial'));
    histCard.el.appendChild(histLbl);
    histCard.el.appendChild(mk('p', '', 'Se conservan 90 días o 50 MB como máximo.'));
    let histBusy = false;
    histChk.addEventListener('change', function () {
      const on = !!histChk.checked;
      histBusy = true; histChk.disabled = true;
      run(null, '', false, function () { return nperf.setHistoryEnabled(on); }).then(function (ok) {
        histBusy = false; histChk.disabled = false;
        if (!ok && last) histChk.checked = !!last.history_enabled;
      });
    });

    // Destinos de latencia
    const tCard = card('Destinos de latencia a internet ');
    tCard.h.appendChild(mk('span', 'muted2', '(máx. 4)'));
    const tList = mk('ul', 'li');
    const tRow = mk('div', 'hrow'), tIn = input('inp mono', 'Nuevo destino de latencia'), tAdd = button('Añadir');
    tIn.placeholder = 'IPv4 pública, p. ej. 9.9.9.9'; tIn.maxLength = 64;
    const tDef = button('Restaurar valores por defecto', 'btn sm'), tErr = inlineError();
    tRow.appendChild(tIn); tRow.appendChild(tAdd);
    [tList, tRow, tDef, tErr].forEach(function (n) { tCard.el.appendChild(n); });
    function targets() { return (last && last.internet_targets) || []; }
    function tRefresh() { tAdd.disabled = targets().length >= MAX_TARGETS || core.ui.pendingActions.has(tAdd); }
    function setTargets(btn, list) {
      setErr(tErr, '');
      return run(btn, '…', false, function () { return nperf.setInternetTargets(list); }).then(function (ok) { tRefresh(); return ok; });
    }
    tAdd.addEventListener('click', function () {
      const v = String(tIn.value).trim(), cur = targets();
      if (!IPV4.test(v)) { setErr(tErr, 'Indique una dirección IPv4 válida (n.n.n.n).'); return; }
      if (cur.indexOf(v) >= 0) { setErr(tErr, 'Ese destino ya está en la lista.'); return; }
      if (cur.length >= MAX_TARGETS) { setErr(tErr, 'Máximo ' + MAX_TARGETS + ' destinos.'); return; }
      setTargets(tAdd, cur.concat([v])).then(function (ok) { if (ok) tIn.value = ''; });
    });
    tIn.addEventListener('keydown', function (e) { if (e.key === 'Enter') { e.preventDefault(); tAdd.click(); } });
    tDef.addEventListener('click', function () { setTargets(tDef, []); });

    // Subredes
    const sCard = card('Subredes extra');
    const sList = mk('ul', 'li');
    const sRow = mk('div', 'hrow'), sIn = input('inp mono', 'Subred'), sAdd = button('Añadir');
    sIn.placeholder = '192.168.2.0/24';
    const sErr = inlineError();
    sRow.appendChild(sIn); sRow.appendChild(sAdd);
    [sList, sRow, sErr].forEach(function (n) { sCard.el.appendChild(n); });
    sAdd.addEventListener('click', function () {
      const v = String(sIn.value).trim(), cur = (last && last.extra_subnets) || [];
      setErr(sErr, '');
      if (!CIDR.test(v)) { setErr(sErr, 'Indique una subred en formato 192.168.2.0/24.'); return; }
      if (cur.indexOf(v) >= 0) { setErr(sErr, 'Esa subred ya está en la lista.'); return; }
      run(sAdd, 'Añadiendo…', true, function () { return nperf.addSubnet(v); }).then(function (ok) { if (ok) sIn.value = ''; });
    });
    sIn.addEventListener('keydown', function (e) { if (e.key === 'Enter') { e.preventDefault(); sAdd.click(); } });

    function listItems(ul, items, cls, onRemove, restart) {
      emptyNode(ul);
      if (!items.length) { ul.appendChild(mk('li', 'muted2', 'Ninguno')); return; }
      items.forEach(function (n) {
        const li = mk('li'), b = button('Quitar', 'btn sm');
        li.appendChild(mk('span', cls, n));
        b.title = 'Quitar ' + n;
        b.addEventListener('click', function () { onRemove(b, n); });
        li.appendChild(b); ul.appendChild(li);
      });
    }

    function render(s) {
      if (!s) return;
      last = s;
      keyBadge.className = 'tag' + (s.has_key ? ' ok' : '');
      keyBadge.textContent = s.has_key ? 'Configurada' : 'No configurada';
      core.setValueIfIdle(aliasIn, s.alias);
      aliasRefresh(); keyRefresh();
      if (!histBusy) histChk.checked = !!s.history_enabled;
      core.renderIf('drawers.targets', s.internet_targets || [], function (list) {
        listItems(tList, list, 'mono', function (b, n) {
          setTargets(b, targets().filter(function (v) { return v !== n; }));
        });
      });
      tRefresh();
      core.renderIf('drawers.subnets', s.extra_subnets || [], function (list) {
        listItems(sList, list, 'mono', function (b, n) {
          run(b, '…', true, function () { return nperf.removeSubnet(n); });
        });
      });
    }
    return { root: root, render: render, banner: banner };
  }

  function errText(e) { return String(e && e.message ? e.message : e); }

  let settings = null;
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
    const pane = byId('drawer-settings');
    if (pane) {
      settings = createSettings(core, window.nperf);
      emptyNode(pane);
      pane.appendChild(settings.root);
    }
    poller.load(); // carga inicial; después solo con el drawer abierto
  }

  window.NPerfDrawers = { render: function (s) { if (settings) settings.render(s); }, wifiStandard: wifiStandard, createPoller: createPoller, createSettings: createSettings };
  if (typeof document !== 'undefined' && document.addEventListener) document.addEventListener('DOMContentLoaded', init);
})();
