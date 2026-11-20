'use strict';
// Lista de equipos (sidebar) y cabecera conectada. Todo dato de la red se inserta como texto.
// Usa utilidades de app.js (el, clear, poll, nperf) y NPerfCore/NPerfShell.
(function () {
  const core = window.NPerfCore;
  const $ = function (id) { return document.getElementById(id); };
  const GATEWAY_ID = 'gateway';
  const KIND_TITLES = { eth: 'Cable (Ethernet)', wifi: 'WiFi', other: 'Otro tipo de enlace', unknown: 'Tipo de enlace desconocido' };
  const KIND_SHORT = { eth: 'Cable', wifi: 'Wi-Fi', other: 'Otro' };
  const TAGS = { available: 'ok', busy: 'warn', queued: 'warn' };
  const SERVER_PORT = 5300; // puerto fijo del servidor (decisiones cerradas)

  function linkLine(d) {
    const parts = [];
    if (d.link_speed && d.link_speed !== 'no disponible') parts.push(d.link_speed);
    if (d.ssid) parts.push(d.ssid);
    return parts.join(' · ');
  }

  // peerRow construye una fila. selectable: se puede elegir como destino; noAgent: sin NPerf.
  function peerRow(d, selectable, selected, noAgent) {
    const gw = noAgent && d.id === GATEWAY_ID;
    const off = d.status === 'unreachable';
    const row = el('div', 'peer' + (selectable ? '' : ' self') + (selected ? ' sel' : '') + (off ? ' off' : '') + (gw ? ' gw' : '') + (noAgent && !gw ? ' manual' : ''));
    const kind = noAgent ? 'noagent' : (KIND_TITLES[d.link] ? d.link : 'unknown');
    const r1 = el('div', 'peer-r1');
    const ico = el('span', 'ico ' + kind); ico.title = noAgent ? (gw ? 'Puerta de enlace (sin NPerf)' : 'Equipo sin NPerf') : KIND_TITLES[kind];
    r1.appendChild(ico);
    const name = el('span', 'peer-name', d.alias); name.title = d.alias;
    r1.appendChild(name);
    r1.appendChild(el('span', 'tag ' + (TAGS[d.status] || ''), d.status_label));
    row.appendChild(r1);
    const r2 = el('div', 'peer-r2');
    r2.appendChild(el('span', 'mono', (d.ips || []).join(', ')));
    const ln = noAgent ? '' : linkLine(d);
    if (ln) r2.appendChild(el('span', 'peer-link mono', ln));
    row.appendChild(r2);
    if (noAgent && !gw) {
      const rm = el('button', 'btn sm rm', 'Quitar'); rm.type = 'button';
      rm.title = 'Quitar ' + d.alias + ' de la lista';
      rm.addEventListener('click', function (e) {
        e.stopPropagation();
        core.busy(rm, Promise.resolve().then(function () { return nperf.removeHost(d.id); }))
          .then(poll, function (x) { core.pushToast(x, 'error'); });
      });
      rm.addEventListener('keydown', function (e) { e.stopPropagation(); });
      row.appendChild(rm);
    }
    if (selectable) {
      row.role = 'button';
      row.ariaDisabled = off ? 'true' : 'false';
      row.ariaPressed = selected ? 'true' : 'false';
      row.tabIndex = off ? -1 : 0;
      if (!off) {
        const pick = function () { act(nperf.selectPeer(d.id)); };
        row.addEventListener('click', pick);
        row.addEventListener('keydown', function (e) {
          if (e.target === row && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); pick(); }
        });
      }
    }
    row.title = d.alias + ' - ' + (d.ips || []).join(', ');
    return row;
  }

  function fill(id, rows) {
    const c = $(id); clear(c);
    rows.forEach(function (r) { c.appendChild(r); });
  }

  function renderList(s) {
    core.renderIf('sb-self', s.self, function (d) { fill('sb-self', [peerRow(d, false, false, false)]); });
    core.renderIf('sb-peers', [s.peers, s.selected_id], function () {
      fill('sb-peers', (s.peers || []).map(function (p) { return peerRow(p, true, p.id === s.selected_id, false); }));
    });
    const na = s.no_agent || [];
    core.renderIf('sb-noagent', [na, s.selected_id], function () {
      fill('sb-noagent', na.map(function (p) { return peerRow(p, true, p.id === s.selected_id, true); }));
    });
    const empty = !(s.peers && s.peers.length);
    $('sb-empty').hidden = !empty;
    core.renderIf('sb-hint', s.firewall_hint || '', function (h) {
      $('sb-empty-hint').textContent = h || 'Comprueba que NPerf esté en marcha en el otro equipo y que el cortafuegos permita los puertos 5300 y 5301.';
    });
  }

  // ---- añadir equipo sin NPerf ----
  function setAddOpen(open) {
    core.ui.isAddHostOpen = open;
    $('host-form').hidden = !open;
    $('host-open').hidden = open;
    if (open) $('host-target').focus(); else $('host-open').focus();
  }

  function submitHost(e) {
    e.preventDefault();
    const err = $('host-error'); err.hidden = true;
    core.busy($('host-add'), Promise.resolve().then(function () { return nperf.addHost($('host-target').value, $('host-alias').value); }))
      .then(function () {
        $('host-target').value = ''; $('host-alias').value = '';
        setAddOpen(false);
        return poll();
      }, function (x) { err.textContent = String(x && x.message ? x.message : x); err.hidden = false; });
  }

  // ---- cabecera ----
  let histBusy = false;
  function refreshHistCount() {
    if (histBusy) return;
    histBusy = true;
    Promise.resolve().then(function () { return nperf.history({}); })
      .then(function (v) { setHistCount(((v && v.records) || []).length); }, function () {})
      .then(function () { histBusy = false; });
  }
  function setHistCount(n) { $('hist-badge').textContent = '(' + n + ')'; }

  function networkLabel(self) {
    const parts = [];
    if (KIND_SHORT[self.link]) parts.push(KIND_SHORT[self.link]);
    if (self.link_speed && self.link_speed !== 'no disponible') parts.push(self.link_speed);
    return parts.length ? 'Red · ' + parts.join(' ') : 'Red';
  }

  function renderHeader(s) {
    core.renderIf('hdr-server', s.config_error || '', function (err) { window.NPerfShell.setServer(SERVER_PORT, err); });
    core.renderIf('hdr-net', [s.self.link, s.self.link_speed], function () { $('hdr-net').textContent = networkLabel(s.self); });
    // El contador solo se recalcula al empezar/terminar un TEST!, un diagnóstico o al cambiar el ajuste.
    core.renderIf('hdr-hist', [s.test && s.test.status, s.probe && s.probe.status, s.history_enabled, s.history_warning || ''], refreshHistCount);
  }

  function render(s) { renderList(s); renderHeader(s); }

  function init() {
    $('host-open').addEventListener('click', function () { setAddOpen(true); });
    $('host-cancel').addEventListener('click', function () { $('host-error').hidden = true; setAddOpen(false); });
    $('host-form').addEventListener('submit', submitHost);
    $('host-form').addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { e.stopPropagation(); setAddOpen(false); }
    });
    $('config-open').addEventListener('click', function () { window.NPerfShell.openDrawer('settings'); });
    document.addEventListener('nperf:view', function () { core.ui.activeView = window.NPerfShell.state.view; });
    document.addEventListener('nperf:drawer', function () { core.ui.activeDrawer = window.NPerfShell.state.drawer; });
  }

  window.NPerfSidebar = { render: render, setHistCount: setHistCount, refreshHistCount: refreshHistCount, peerRow: peerRow };
  init();
})();
