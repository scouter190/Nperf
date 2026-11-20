'use strict';
// Diagnósticos hacia internet (latencia y velocidad con M-Lab) y ajustes de historial y destinos.
// Se carga después de app.js y reutiliza sus funciones. Todo dato se inserta como texto.

// Textos fijos del aviso de M-Lab: deben coincidir con internal/ndt7/notice.go (lo comprueba un test).
const SPEED_TITLE = 'Prueba de velocidad con M-Lab';
const SPEED_IP = 'Esta prueba envía tráfico a un servidor de M-Lab (Measurement Lab). M-Lab publica la dirección IP pública de quien mide, la fecha y hora y los metadatos de la prueba como datos abiertos (CC0), y los conserva de forma indefinida.';
const SPEED_DATA = 'La prueba descarga y sube datos durante unos 20 s: puede consumir cientos de MB en conexiones rápidas; tenlo en cuenta en conexiones móviles o con tarifa por datos.';
const SPEED_LIMIT = 'Solo se hace cuando tú lo pides; el límite es de 40 pruebas al día por usuario de Windows en este equipo.';
const SPEED_ATTRIBUTION = 'Velocidad medida con NDT7 de M-Lab (Measurement Lab), https://www.measurementlab.net/';
const SPEED_DAILY_LIMIT = 40;

let inetFocusBack = null;

function inetErr(e) { return String(e && e.message ? e.message : e); }
function inetWhen(iso) {
  const d = new Date(iso);
  return isNaN(d.getTime()) ? String(iso || '-') : d.toLocaleString('es', { dateStyle: 'short', timeStyle: 'medium' });
}
function statOk(st) { return st && st.rtt_status === 'ok' && typeof st.avg_ms === 'number'; }

// ---- vistas de resultado (también las usa el historial) ----
function internetView(x, combined) {
  const box = el('div');
  if (!x) return box;
  if (combined && combined.messages && combined.messages.length) box.appendChild(combinedView(combined));
  else if (x.verdict) {
    box.appendChild(el('p', 'status-line inet-kind', x.verdict.message || 'Sin veredicto'));
    if (Array.isArray(x.verdict.notes) && x.verdict.notes.length) box.appendChild(listOf('notes warn-notes', x.verdict.notes));
  }
  const t = el('table');
  const hr = el('tr');
  ['Destino', 'RTT medio', 'Jitter', 'Pérdida', 'TCP 443'].forEach(function (c) { hr.appendChild(el('th', '', c)); });
  t.appendChild(hr);
  const row = function (name, st, tcp) {
    const tr = el('tr');
    cell(tr, name);
    if (!st || (st.received === 0)) {
      const c = el('td', 'muted', 'sin respuesta' + (st && st.sent ? ' (enviados ' + st.sent + ')' : '')); c.colSpan = 3; tr.appendChild(c);
    } else {
      cell(tr, statOk(st) ? fmtMs(st.avg_ms) : 'no disponible', 'num');
      cell(tr, st.jitter_status === 'ok' ? fmtMs(st.jitter_ms) : 'no disponible', 'num');
      cell(tr, fmt(st.loss_pct, 1) + ' %', 'num');
    }
    cell(tr, tcp === true ? 'responde' : (tcp === false ? 'no responde' : '-'));
    t.appendChild(tr);
  };
  if (x.gateway && x.gateway.ip) row('Puerta de enlace (' + x.gateway.ip + ')', x.gateway, null);
  else box.appendChild(el('p', 'muted', 'Puerta de enlace no detectada.'));
  (x.targets || []).forEach(function (g) { row(g.ip, g, g.tcp443); });
  box.appendChild(t);
  if (x.trace && Array.isArray(x.trace.hops)) {
    box.appendChild(el('h3', '', 'Ruta hacia ' + x.trace.target + (x.trace.reached ? ' (alcanzado)' : ' (no alcanzado)')));
    const tt = el('table', 'trace');
    const h2 = el('tr');
    ['Salto', 'Equipo', 'RTT medio', 'Pérdida'].forEach(function (c) { h2.appendChild(el('th', '', c)); });
    tt.appendChild(h2);
    x.trace.hops.forEach(function (h) {
      const tr = el('tr', h.from ? '' : 'silent');
      cell(tr, h.ttl, 'num'); cell(tr, h.from || '*');
      cell(tr, statOk(h) ? fmtMs(h.avg_ms) : '*', 'num'); cell(tr, fmt(h.loss_pct, 0) + ' %', 'num');
      tt.appendChild(tr);
    });
    box.appendChild(tt);
  }
  if (Array.isArray(x.trace_notes) && x.trace_notes.length) box.appendChild(listOf('notes', x.trace_notes));
  return box;
}

function speedDirRow(t, name, d, status, reason) {
  const tr = el('tr');
  cell(tr, name);
  if (!d) {
    const c = el('td', 'muted', 'no disponible' + (reason ? ': ' + reason : '')); c.colSpan = 3; tr.appendChild(c);
    t.appendChild(tr);
    return;
  }
  cell(tr, fmt(d.mbps, 1) + ' Mbps', 'num');
  cell(tr, typeof d.min_rtt_ms === 'number' ? fmtMs(d.min_rtt_ms) : '-', 'num');
  const inc = status === 'incomplete' || d.incomplete === true;
  cell(tr, inc ? 'incompleta' + (reason || d.reason ? ': ' + (reason || d.reason) : '') : 'completa', inc ? 'warn-cell' : '');
  t.appendChild(tr);
}

function speedView(x) {
  const box = el('div');
  if (!x) return box;
  const srv = x.server;
  const loc = srv ? [srv.city, srv.country].filter(function (v) { return v; }).join(', ') : '';
  box.appendChild(kvTable([
    ['Servidor', srv ? srv.machine + (loc ? ' (' + loc + ')' : '') : 'no disponible'],
    ['Hora', x.time ? inetWhen(x.time) : '-'],
    ['Pruebas hoy', typeof x.tests_today === 'number' ? x.tests_today + ' de ' + SPEED_DAILY_LIMIT : '-']
  ]));
  const t = el('table');
  const hr = el('tr');
  ['Sentido', 'Velocidad', 'RTT mínimo', 'Estado'].forEach(function (c) { hr.appendChild(el('th', '', c)); });
  t.appendChild(hr);
  speedDirRow(t, 'Descarga', x.download, x.download_status, x.download_reason);
  speedDirRow(t, 'Subida', x.upload, x.upload_status, x.upload_reason);
  box.appendChild(t);
  [x.download, x.upload].forEach(function (d) { if (d && Array.isArray(d.notes) && d.notes.length) box.appendChild(listOf('notes', d.notes)); });
  box.appendChild(el('p', 'muted speed-attr', x.attribution || SPEED_ATTRIBUTION));
  return box;
}

// ---- consentimiento de la prueba de velocidad ----
function openSpeedDialog() {
  inetFocusBack = document.activeElement;
  $('speed-title').textContent = SPEED_TITLE;
  $('speed-ip').textContent = SPEED_IP;
  $('speed-data').textContent = SPEED_DATA;
  $('speed-limit').textContent = SPEED_LIMIT;
  $('speed-dialog').hidden = false;
  $('speed-cancel').focus();
}
function closeSpeedDialog() {
  $('speed-dialog').hidden = true;
  if (inetFocusBack && inetFocusBack.focus) inetFocusBack.focus();
  inetFocusBack = null;
}
// Único punto desde el que se llama a startSpeed: el botón "Aceptar y medir" del diálogo.
function acceptSpeed() {
  closeSpeedDialog();
  showProbe = true; delete sig.detail;
  act(nperf.startSpeed(true));
}
function speedDialogKeys(e) {
  if (e.key === 'Escape') { e.preventDefault(); closeSpeedDialog(); return; }
  if (e.key !== 'Tab') return;
  const bs = Array.prototype.slice.call($('speed-dialog').querySelectorAll('button'));
  const i = bs.indexOf(document.activeElement);
  const n = e.shiftKey ? (i <= 0 ? bs.length - 1 : i - 1) : (i === bs.length - 1 ? 0 : i + 1);
  e.preventDefault();
  bs[n].focus();
}

// ---- barra de pruebas a internet y ajustes (desde render de app.js) ----
const INET_BTNS = [['internet', 'probe-internet', 'Internet'], ['speed', 'probe-speed', 'Velocidad a internet']];

function renderInternetState(s) {
  const p = s.probe || { running: false, status: 'idle' };
  INET_BTNS.forEach(function (x) {
    const b = $(x[1]);
    const own = p.running && p.kind === x[0];
    b.textContent = own ? (p.status === 'cancelling' ? 'Cancelando…' : 'Cancelar') : x[2];
    b.classList.toggle('danger', own);
    b.disabled = own ? p.status === 'cancelling' : !s.can_probe;
    b.title = own || s.can_probe ? '' : s.probe_blocked_reason;
  });
  $('set-history').checked = !!s.history_enabled;
  if (changed('targets', s.internet_targets)) {
    const ul = $('targets'); clear(ul);
    (s.internet_targets || []).forEach(function (n) {
      const li = el('li'); li.appendChild(el('span', 'mono', n));
      const b = el('button', '', 'Quitar'); b.type = 'button'; b.title = 'Quitar el destino ' + n;
      b.addEventListener('click', function () { setTargets((last.internet_targets || []).filter(function (v) { return v !== n; })); });
      li.appendChild(b); ul.appendChild(li);
    });
    $('target-add').disabled = (s.internet_targets || []).length >= 4;
  }
}

function setTargetsError(msg) {
  const e = $('targets-error');
  e.hidden = !msg; e.textContent = msg || '';
}
function setTargets(list) {
  setTargetsError('');
  return Promise.resolve().then(function () { return nperf.setInternetTargets(list); })
    .then(function () { delete sig.targets; return poll(); }, function (e) { setTargetsError(inetErr(e)); });
}

document.addEventListener('DOMContentLoaded', function () {
  $('probe-internet').addEventListener('click', function () {
    if (last && last.probe && last.probe.running && last.probe.kind === 'internet') { act(nperf.cancelProbe()); return; }
    showProbe = true; delete sig.detail;
    act(nperf.startInternet());
  });
  $('probe-speed').addEventListener('click', function () {
    if (last && last.probe && last.probe.running && last.probe.kind === 'speed') { act(nperf.cancelProbe()); return; }
    openSpeedDialog();
  });
  $('speed-accept').addEventListener('click', acceptSpeed);
  $('speed-cancel').addEventListener('click', closeSpeedDialog);
  $('speed-dialog').addEventListener('keydown', speedDialogKeys);
  $('speed-policy-mlab').addEventListener('click', function () { Promise.resolve().then(function () { return nperf.openPrivacy('mlab'); }).then(null, toast); });
  $('speed-policy-nperf').addEventListener('click', function () { Promise.resolve().then(function () { return nperf.openPrivacy('nperf'); }).then(null, toast); });
  $('target-add').addEventListener('click', function () {
    const v = $('target').value.trim();
    if (!v) { setTargetsError('Indique una dirección IPv4 pública.'); return; }
    const cur = (last && last.internet_targets) || [];
    if (cur.indexOf(v) >= 0) { setTargetsError('Ese destino ya está en la lista.'); return; }
    setTargets(cur.concat([v])).then(function () { if ($('targets-error').hidden) $('target').value = ''; });
  });
  $('target').addEventListener('keydown', function (e) { if (e.key === 'Enter') { e.preventDefault(); $('target-add').click(); } });
  $('targets-default').addEventListener('click', function () { setTargets([]); });
  $('set-history').addEventListener('change', function () {
    const on = $('set-history').checked, m = $('set-history-msg');
    m.hidden = true;
    Promise.resolve().then(function () { return nperf.setHistoryEnabled(on); })
      .then(poll, function (e) { m.textContent = inetErr(e); m.hidden = false; return poll(); });
  });
});
