// Maqueta (spec 008 T5): solo cambia escenarios con hidden/disabled/classList. Sin HTML dinámico.
(function () {
  'use strict';
  var S = window.NPerfShell;
  var d = document;
  // Escenarios: vista, drawer, modal, popover, modo detallado, acordeón de inactivas.
  var ESC = {
    'resultado': { t: 'Panel · resultado de TEST!' },
    'detallado': { t: 'Panel · resultado con modo detallado', detail: true },
    'parcial': { t: 'Panel · TEST! cancelado (parcial, "—")' },
    'curso': { t: 'Panel · TEST! en curso' },
    'cola': { t: 'Panel · en cola' },
    'diagnostico': { t: 'Panel · diagnóstico en curso' },
    'ping': { t: 'Panel · resultado de Ping' },
    'internet': { t: 'Panel · resultado de Internet' },
    'velocidad': { t: 'Panel · resultado de velocidad' },
    'vacio': { t: 'Panel · sin equipos (vacío)' },
    'anadir': { t: 'Panel · añadir equipo' },
    'sinhist': { t: 'Panel · historial desactivado (Exportar bloqueado)' },
    'popover': { t: 'Panel · popover ⓘ', popover: true },
    'mlab': { t: 'Modal M-Lab', modal: 'mlab' },
    'historial': { t: 'Historial', view: 'history' },
    'historial-registro': { t: 'Historial · registro (solo lectura)', view: 'history' },
    'historial-desactivado': { t: 'Historial desactivado', view: 'history' },
    'exportar': { t: 'Modal exportar', view: 'history', modal: 'exportar' },
    'borrar': { t: 'Modal borrar', view: 'history', modal: 'borrar' },
    'borrar-todo': { t: 'Modal borrar todo', view: 'history', modal: 'borrar-todo' },
    'red': { t: 'Drawer Red', drawer: 'adapters' },
    'red-inactivas': { t: 'Drawer Red (inactivas abiertas)', drawer: 'adapters', acc: true },
    'ajustes': { t: 'Drawer Ajustes (guardado)', drawer: 'settings' },
    'ajustes-error': { t: 'Drawer Ajustes (error al guardar)', drawer: 'settings' },
    'error': { t: 'Servidor con error (config_error)' }
  };
  var cur = 'resultado';
  var modalOpen = null;

  function inList(list, name) { return list.split(' ').indexOf(name) >= 0; }

  function applyEsc() {
    d.querySelectorAll('[data-esc]').forEach(function (el) {
      var toks = el.getAttribute('data-esc').split(' ');
      var neg = toks.some(function (t) { return t.charAt(0) === '!'; });
      var on;
      if (neg) on = !toks.some(function (t) { return t === '!' + cur; });
      else on = toks.indexOf(cur) >= 0;
      el.hidden = !on;
    });
    d.querySelectorAll('[data-dis]').forEach(function (el) {
      el.disabled = inList(el.getAttribute('data-dis'), cur);
    });
  }

  function closeModal() {
    modalOpen = null;
    S.layers.shut('modal');
    d.getElementById('mdl-back').hidden = true;
    ['mlab', 'exportar', 'borrar', 'borrar-todo'].forEach(function (n) { d.getElementById('mdl-' + n).hidden = true; });
  }
  function openModal(name) {
    closeModal();
    modalOpen = name;
    d.getElementById('mdl-back').hidden = false;
    d.getElementById('mdl-' + name).hidden = false;
    S.layers.open('modal', closeModal);
    if (name === 'mlab') d.getElementById('mlab-cancel').focus();
  }

  function setDetail(on) {
    var sw = d.getElementById('sw-detail');
    sw.ariaChecked = on ? 'true' : 'false';
    d.getElementById('detail').hidden = !on;
  }

  function go(name) {
    var e = ESC[name];
    if (!e) return;
    cur = name;
    S.closePopover();
    S.closeDrawer();
    closeModal();
    var slot = d.getElementById(name === 'historial-registro' ? 'slot-hist' : 'slot-panel');
    var res = d.getElementById('res-test');
    if (res.parentNode !== slot) slot.appendChild(res);
    S.setView(e.view || 'panel');
    S.setServer(5300, name === 'error');
    applyEsc();
    setDetail(!!e.detail);
    d.getElementById('acc-inactive').open = !!e.acc;
    if (e.drawer) {
      d.getElementById('drawer-title').textContent = e.drawer === 'adapters' ? 'Tarjetas de red' : 'Ajustes';
      S.openDrawer(e.drawer);
    }
    if (e.popover) S.togglePopover(d.getElementById('btn-info'), d.getElementById('pop-info'));
    if (e.modal) openModal(e.modal);
    var sel = d.getElementById('m-esc');
    if (sel.value !== name) sel.value = name;
    if (location.hash.slice(1).split('&')[0] !== name) history.replaceState(null, '', '#' + name + (d.body.classList.contains('captura') ? '&c' : ''));
  }

  function init() {
    var q = location.search + location.hash;
    if (/captura|&c\b/.test(q)) d.body.classList.add('captura');
    var sel = d.getElementById('m-esc');
    Object.keys(ESC).forEach(function (k) {
      var o = d.createElement('option');
      o.value = k;
      o.textContent = ESC[k].t;
      sel.appendChild(o);
    });
    sel.addEventListener('change', function () { go(sel.value); });
    d.addEventListener('click', function (e) {
      var t = e.target.closest ? e.target.closest('[data-go],[data-modal],[data-close],#btn-speed,#btn-export,#btn-info,#hdr-adapters,#hdr-settings,#nav-panel,#nav-history,#sw-detail') : null;
      if (!t) return;
      if (t.hasAttribute('data-go')) { go(t.getAttribute('data-go')); return; }
      if (t.hasAttribute('data-modal')) { openModal(t.getAttribute('data-modal')); return; }
      if (t.hasAttribute('data-close')) { closeModal(); return; }
      if (t.id === 'btn-speed') { openModal('mlab'); return; }
      if (t.id === 'btn-export') { openModal('exportar'); return; }
      if (t.id === 'btn-info') { S.togglePopover(t, d.getElementById('pop-info')); return; }
      if (t.id === 'sw-detail') { setDetail(t.ariaChecked !== 'true'); return; }
      if (t.id === 'nav-panel' && cur.indexOf('historial') === 0) go('resultado');
      if (t.id === 'nav-history' && cur.indexOf('historial') !== 0) go('historial');
    });
    d.getElementById('mdl-back').addEventListener('click', function (e) { if (e.target === e.currentTarget) closeModal(); });
    var h = location.hash.slice(1).split('&')[0];
    go(ESC[h] ? h : 'resultado');
  }
  init();
})();
