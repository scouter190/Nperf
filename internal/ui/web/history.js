'use strict';
// Panel "Historial": lista, filtros, selección, registro en solo lectura, borrado y exportación.
// Se carga después de app.js y reutiliza sus funciones (el, clear, reportView, chartBlock...).
// Todo dato del disco o de la red se inserta como texto (textContent), nunca como HTML.

const HIST_KINDS = [['', 'Todos'], ['test', 'TEST!'], ['ping', 'Ping'], ['trace', 'Traceroute'], ['mtu', 'MTU'], ['ports', 'Puertos'], ['internet', 'Internet'], ['speed', 'Velocidad (M-Lab)']];
const HIST_KIND_NAMES = { test: 'TEST!', ping: 'Ping', trace: 'Traceroute', mtu: 'MTU', ports: 'Puertos', internet: 'Internet', speed: 'Velocidad (M-Lab)' };
const HIST_STATUS = { done: 'terminado', cancelled: 'cancelado', error: 'error' };

let histRecords = [];
let histSkipped = 0;
let histSelected = {};      // id -> true
let histOpen = null;        // registro abierto (solo lectura) o null
let histLoading = false, histReload = false;
let histSig = '';

function histShown() { return !!(window.NPerfShell && window.NPerfShell.state.view === 'history'); }

function histErr(e) { return String(e && e.message ? e.message : e); }

function histMsg(text, isErr) {
  const m = $('hist-msg');
  m.textContent = text || '';
  m.className = isErr ? 'err-text' : 'muted';
}

// ---- filtros ----
function dayStart(v) {
  const p = /^(\d{4})-(\d{2})-(\d{2})$/.exec(v);
  return p ? new Date(Number(p[1]), Number(p[2]) - 1, Number(p[3]), 0, 0, 0, 0) : null;
}
function histFilter() {
  const f = { host: $('hist-host').value.trim(), kind: $('hist-kind').value, since: '', until: '' };
  const a = dayStart($('hist-since').value), b = dayStart($('hist-until').value);
  if (a) f.since = a.toISOString();
  if (b) { b.setHours(23, 59, 59, 0); f.until = b.toISOString(); }
  return f;
}

// ---- texto de la lista ----
function histWhen(iso) {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return String(iso || '-');
  return d.toLocaleString('es', { dateStyle: 'short', timeStyle: 'medium' });
}
function histDest(r) {
  const d = r.dest;
  if (!d) return '-';
  return (d.alias || '') + (d.alias && d.ip ? ' (' + d.ip + ')' : (d.ip || '')) || '-';
}
function mbpsOf(d) { return d && d.status === 'measured' && typeof d.throughput_mbps === 'number' ? d.throughput_mbps : null; }
function histSummary(r) {
  if (r.status === 'error') return 'Error: ' + (r.error || 'sin detalle');
  if (r.status === 'cancelled') return 'Cancelado';
  const x = r.result || {};
  switch (r.kind) {
    case 'test': {
      const out = [];
      [['tcp', 'TCP'], ['udp', 'UDP']].forEach(function (p) {
        const f = x[p[0]] && mbpsOf(x[p[0]].fwd), v = x[p[0]] && mbpsOf(x[p[0]].rev);
        if (f !== null) out.push(p[1] + ' ida ' + fmt(f, 1) + ' Mbps');
        if (v !== null) out.push(p[1] + ' vuelta ' + fmt(v, 1) + ' Mbps');
      });
      if (x.rtt && x.rtt.status === 'measured') out.push('RTT ' + fmt(x.rtt.avg_ms, 1) + ' ms');
      return out.join(' · ') || 'Sin datos medidos';
    }
    case 'ping': return 'Pérdida ' + fmt(x.loss_pct, 1) + ' %' + (x.rtt_status === 'ok' ? ' · media ' + fmtMs(x.avg_ms) : '');
    case 'trace': return (x.reached ? 'Destino alcanzado' : 'Destino no alcanzado') + ' · ' + (x.hops || []).length + ' saltos';
    case 'mtu': return x.status === 'ok' ? 'MTU ' + x.mtu_bytes + ' bytes' : 'MTU no disponible';
    case 'ports': {
      const ps = x.ports || [];
      return ps.filter(function (p) { return p.state === 'open'; }).length + ' abiertos de ' + ps.length;
    }
    case 'internet': return (x.verdict && x.verdict.message) || 'Sin veredicto';
    case 'speed': {
      const out = [];
      if (x.download && typeof x.download.mbps === 'number') out.push('↓ ' + fmt(x.download.mbps, 1) + ' Mbps');
      if (x.upload && typeof x.upload.mbps === 'number') out.push('↑ ' + fmt(x.upload.mbps, 1) + ' Mbps');
      return out.join(' · ') || 'Sin datos medidos';
    }
  }
  return '-';
}

// ---- lista ----
function histSelectedIds() { return histRecords.map(function (r) { return r.id; }).filter(function (id) { return histSelected[id]; }); }

function histUpdateButtons() {
  const n = histSelectedIds().length, total = histRecords.length;
  $('hist-count').textContent = total + ' registro' + (total === 1 ? '' : 's') + ' · ' + n + ' seleccionado' + (n === 1 ? '' : 's');
  $('hist-del-sel').disabled = n === 0;
  $('hist-del-all').disabled = total === 0;
  $('hist-export-sel').disabled = n === 0;
  $('hist-export-sel').textContent = 'Exportar seleccionados (' + n + ')';
  const all = $('hist-all');
  all.checked = total > 0 && n === total;
  all.indeterminate = n > 0 && n < total;
  all.disabled = total === 0;
}

function histRow(r) {
  const tr = el('tr', r.status === 'error' ? 'hist-err' : '');
  const c0 = el('td', 'hist-chk');
  const cb = document.createElement('input'); cb.type = 'checkbox'; cb.checked = !!histSelected[r.id];
  cb.ariaLabel = 'Seleccionar el registro del ' + histWhen(r.time);
  cb.addEventListener('change', function () { if (cb.checked) histSelected[r.id] = true; else delete histSelected[r.id]; histUpdateButtons(); });
  c0.appendChild(cb); tr.appendChild(c0);
  tr.appendChild(el('td', '', histWhen(r.time)));
  const k = el('td'); const kw = el('span', 'hist-kind');
  kw.appendChild(el('span', 'hist-kind-name', HIST_KIND_NAMES[r.kind] || r.kind));
  kw.appendChild(el('span', 'hist-badge ' + (HIST_STATUS[r.status] ? r.status : 'error'), HIST_STATUS[r.status] || r.status));
  k.appendChild(kw);
  tr.appendChild(k);
  tr.appendChild(el('td', '', histDest(r)));
  tr.appendChild(el('td', 'hist-res', histSummary(r)));
  const c5 = el('td');
  const ob = el('button', '', 'Abrir'); ob.type = 'button'; ob.title = 'Abrir el registro del ' + histWhen(r.time) + ' en solo lectura';
  ob.addEventListener('click', function () { histOpenRecord(r.id); });
  c5.appendChild(ob); tr.appendChild(c5);
  return tr;
}

function histRender() {
  const body = $('hist-rows'); clear(body);
  histRecords.forEach(function (r) { body.appendChild(histRow(r)); });
  $('hist-empty').hidden = histRecords.length > 0;
  $('hist-table').hidden = histRecords.length === 0;
  histUpdateButtons();
}

function histLoad() {
  if (histLoading) { histReload = true; return Promise.resolve(); }
  histLoading = true;
  return Promise.resolve().then(function () { return nperf.history(histFilter()); })
    .then(function (v) {
      histRecords = (v && v.records) || [];
      histSkipped = (v && v.skipped) || 0;
      const keep = {};
      histRecords.forEach(function (r) { if (histSelected[r.id]) keep[r.id] = true; });
      histSelected = keep;
      histMsg(histSkipped ? histSkipped + ' archivo' + (histSkipped === 1 ? '' : 's') + ' del historial no se pudieron leer y se omiten.' : '', false);
      histRender();
    }, function (e) { histMsg('No se pudo leer el historial: ' + histErr(e), true); })
    .then(function () { histLoading = false; if (histReload) { histReload = false; return histLoad(); } });
}

// ---- borrado y exportación (modales de NPerfModals) ----
function histModals() { return window.NPerfModals; }
function histAfterChange(r) {
  histMsg('', false);
  return histLoad().then(function () {
    if (window.NPerfSidebar) window.NPerfSidebar.refreshHistCount();
    return poll();
  });
}
function histAskDelete(scope) {
  const ids = histSelectedIds();
  const n = scope === 'all' ? histRecords.length : ids.length;
  if (!n) return;
  histModals().openConfirmDelete({
    scope: scope, count: n,
    run: function () { return scope === 'all' ? nperf.deleteAllHistory() : nperf.deleteHistory(ids); },
    onDone: function () { if (scope === 'all') histSelected = {}; else ids.forEach(function (id) { delete histSelected[id]; }); histAfterChange(); }
  });
}
function histExport(ids) {
  if (!ids.length) return;
  histModals().openExport({ ids: ids, run: function (i, format, o) { return nperf.exportHistory(i, format, o); } });
}

// ---- registro abierto (solo lectura) ----
function histDirections(r) {
  const a = (r.source && r.source.alias) || 'Este equipo', b = (r.dest && r.dest.alias) || 'destino';
  return [{ value: 'both', label: a + ' ⇄ ' + b }, { value: 'fwd', label: a + ' → ' + b }, { value: 'rev', label: b + ' → ' + a }];
}
function histSynthetic(r) {
  const d = r.dest || {};
  return {
    directions: histDirections(r),
    self: { alias: (r.source && r.source.alias) || 'Este equipo' },
    peers: [{ id: 'dest', alias: d.alias || 'destino', ips: d.ip ? [d.ip] : [] }], no_agent: [], selected_id: 'dest',
    test: { report: r.result, diagnosis: r.diagnosis || null }
  };
}
function histEndpoint(label, e) {
  if (!e) return null;
  const parts = [];
  if (e.alias) parts.push(e.alias);
  if (e.ip) parts.push(e.ip);
  if (e.link) parts.push(KINDS[e.link] || e.link);
  if (e.ssid) parts.push(e.ssid);
  if (e.nominal_mbps) parts.push('nominal ' + Math.round(e.nominal_mbps) + ' Mbps');
  return label + ': ' + (parts.join(' · ') || '-');
}
function histRecordBody(r, box, charts) {
  const detailed = $('hist-detailed').checked;
  const x = r.result;
  if (r.status === 'error') box.appendChild(el('p', 'err-text', 'Error: ' + (r.error || 'sin detalle')));
  else if (r.status === 'cancelled' && !x) box.appendChild(el('p', 'muted', 'Cancelado.'));
  if (r.kind === 'test') {
    if (x) box.appendChild(reportView(histSynthetic(r), detailed));
    const ch = r.chart;
    if (ch && ch.phases && ch.phases.length) {
      charts.appendChild(el('h2', '', 'Gráficas del TEST!'));
      if (ch.cancelled || r.status === 'cancelled') charts.appendChild(el('p', 'muted chart-cancelled', 'Cancelado: se muestran las muestras recibidas antes de cancelar.'));
      ch.phases.forEach(function (p) { charts.appendChild(chartBlock(p, ch.cancelled)); });
    }
    return;
  }
  if (!x) return;
  if (r.diagnosis && r.diagnosis.messages && r.diagnosis.messages.length) box.appendChild(combinedView(r.diagnosis));
  if (r.kind === 'ping') box.appendChild(pingView(x, detailed));
  else if (r.kind === 'trace') box.appendChild(traceView(x));
  else if (r.kind === 'mtu') box.appendChild(mtuView(x));
  else if (r.kind === 'ports') box.appendChild(portsView(x));
  else if (r.kind === 'internet') box.appendChild(internetView(x, r.diagnosis));
  else if (r.kind === 'speed') box.appendChild(speedView(x));
}

function histRenderRecord() {
  const r = histOpen;
  if (!r) return;
  const box = $('hist-record'), charts = $('hist-record-charts');
  clear(box); clear(charts);
  box.appendChild(el('h2', '', (HIST_KIND_NAMES[r.kind] || r.kind) + ' · ' + histWhen(r.time)));
  box.appendChild(el('p', 'status-line', 'Estado: ' + (HIST_STATUS[r.status] || r.status) + (r.version ? ' · NPerf ' + r.version : '')));
  [histEndpoint('Origen', r.source), histEndpoint('Destino', r.dest)].forEach(function (t) { if (t) box.appendChild(el('p', 'muted', t)); });
  histRecordBody(r, box, charts);
  if (r.notes && r.notes.length) box.appendChild(listOf('notes', r.notes));
}

function histShowList() {
  histOpen = null;
  $('hist-record-view').hidden = true;
  $('hist-list-view').hidden = false;
  histLoad();
}
function histOpenRecord(id) {
  Promise.resolve().then(function () { return nperf.historyRecord(id); })
    .then(function (r) {
      histOpen = r;
      $('hist-detailed').checked = window.NPerfCore.ui.isDetailedMode;
      $('hist-list-view').hidden = true;
      $('hist-record-view').hidden = false;
      histRenderRecord();
      $('hist-back').focus();
    }, function (e) { histMsg('No se pudo abrir el registro: ' + histErr(e), true); });
}

// ---- panel ----
// La vista la muestra NPerfShell (cabecera); al entrar se reinicia a la lista y se recarga.
function histEnter() {
  histOpen = null; $('hist-record-view').hidden = true; $('hist-list-view').hidden = false; histLoad();
}

// Llamada desde render() de app.js con cada estado.
function renderHistoryState(s) {
  const w = $('history-warning');
  w.hidden = !s.history_warning;
  w.textContent = s.history_warning ? 'Aviso: ' + s.history_warning + '.' : '';
  $('hist-disabled').hidden = !!s.history_enabled;
  // Si termina un TEST! o diagnóstico con el panel abierto en la lista, se recarga.
  const sg = JSON.stringify([s.test && s.test.status, s.probe && s.probe.status, s.history_warning]);
  if (sg !== histSig) {
    histSig = sg;
    if (histShown() && !histOpen) histLoad();
  }
}

document.addEventListener('DOMContentLoaded', function () {
  const kind = $('hist-kind');
  HIST_KINDS.forEach(function (k) { const o = el('option', '', k[1]); o.value = k[0]; kind.appendChild(o); });
  document.addEventListener('nperf:view', function () { if (histShown()) histEnter(); });
  $('hist-filters').addEventListener('submit', function (e) { e.preventDefault(); histLoad(); });
  $('hist-reset').addEventListener('click', function () {
    $('hist-host').value = ''; $('hist-kind').value = ''; $('hist-since').value = ''; $('hist-until').value = '';
    histLoad();
  });
  $('hist-all').addEventListener('change', function () {
    histSelected = {};
    if ($('hist-all').checked) histRecords.forEach(function (r) { histSelected[r.id] = true; });
    histRender();
  });
  $('hist-del-sel').addEventListener('click', function () { histAskDelete('selected'); });
  $('hist-del-all').addEventListener('click', function () { histAskDelete('all'); });
  $('hist-export-sel').addEventListener('click', function () { histExport(histSelectedIds()); });
  $('hist-export-one').addEventListener('click', function () { if (histOpen) histExport([histOpen.id]); });
  $('hist-enable-link').addEventListener('click', function () { window.NPerfShell.openDrawer('settings'); });
  $('hist-back').addEventListener('click', histShowList);
  $('hist-detailed').addEventListener('change', histRenderRecord);
});
