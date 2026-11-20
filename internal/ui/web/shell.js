// Shell de la interfaz: cabecera, vistas Panel/Historial, drawers, popover y pila de Escape.
// Solo propiedades del DOM, classList y hidden (sin HTML dinámico).
(function (root) {
  'use strict';

  // Prioridad de las capas: popover (1) > modal (2) > drawer (3).
  var ORDER = ['popover', 'modal', 'drawer'];

  // createLayers lleva la pila de capas abiertas. Es independiente del DOM para poder probarla.
  function createLayers() {
    var open = {};
    return {
      open: function (kind, close, blocking) {
        open[kind] = { close: close, blocking: !!blocking };
      },
      shut: function (kind) { delete open[kind]; },
      setBlocking: function (kind, b) { if (open[kind]) open[kind].blocking = !!b; },
      has: function (kind) { return !!open[kind]; },
      // escape cierra la capa superior; devuelve true si lo consumió.
      escape: function () {
        for (var i = 0; i < ORDER.length; i++) {
          var l = open[ORDER[i]];
          if (!l) continue;
          if (l.blocking) return true; // p. ej. exportación en curso: Escape no cierra nada.
          delete open[ORDER[i]];
          l.close();
          return true;
        }
        return false;
      }
    };
  }

  var layers = createLayers();
  var st = { view: 'panel', drawer: null, popover: null };

  function byId(id) { return root.document ? root.document.getElementById(id) : null; }

  var TITLES = { adapters: 'Tarjetas de red', settings: 'Ajustes' };

  // fire avisa a las vistas (nperf:view, nperf:drawer) sin acoplarlas al shell.
  function fire(name) {
    var d = root.document;
    if (d && typeof root.CustomEvent === 'function') d.dispatchEvent(new root.CustomEvent(name));
  }

  function setView(v) {
    if (v !== 'panel' && v !== 'history') return;
    st.view = v;
    ['panel', 'history'].forEach(function (name) {
      var sec = byId('view-' + name), tab = byId('nav-' + name);
      if (sec) sec.hidden = name !== v;
      if (tab) {
        tab.classList.toggle('active', name === v);
        tab.ariaCurrent = name === v ? 'page' : 'false';
      }
    });
    fire('nperf:view');
  }

  // openDrawer reutiliza el mismo fondo: al cambiar de drawer solo se alternan los paneles.
  function openDrawer(name) {
    var drawer = byId('drawer'), back = byId('backdrop');
    if (!drawer || !back) return;
    st.drawer = name;
    ['adapters', 'settings'].forEach(function (n) {
      var pane = byId('drawer-' + n), btn = byId('hdr-' + n);
      if (pane) pane.hidden = n !== name;
      if (btn) {
        btn.classList.toggle('active', n === name);
        btn.ariaExpanded = n === name ? 'true' : 'false';
      }
    });
    var title = byId('drawer-title');
    if (title) title.textContent = TITLES[name] || '';
    drawer.classList.add('open');
    back.classList.add('open');
    layers.open('drawer', closeDrawer);
    fire('nperf:drawer');
  }

  function closeDrawer() {
    var drawer = byId('drawer'), back = byId('backdrop');
    st.drawer = null;
    layers.shut('drawer');
    if (drawer) drawer.classList.remove('open');
    if (back) back.classList.remove('open');
    ['adapters', 'settings'].forEach(function (n) {
      var btn = byId('hdr-' + n);
      if (btn) { btn.classList.remove('active'); btn.ariaExpanded = 'false'; }
    });
    fire('nperf:drawer');
  }

  function toggleDrawer(name) {
    if (st.drawer === name) closeDrawer(); else openDrawer(name);
  }

  function closePopover() {
    var p = st.popover;
    st.popover = null;
    layers.shut('popover');
    if (p) {
      p.pop.hidden = true;
      if (p.btn) p.btn.ariaExpanded = 'false';
    }
  }

  // togglePopover alterna el popover pop anclado al botón btn.
  function togglePopover(btn, pop) {
    if (st.popover && st.popover.pop === pop) { closePopover(); return; }
    closePopover();
    pop.hidden = false;
    if (btn) btn.ariaExpanded = 'true';
    st.popover = { btn: btn, pop: pop };
    layers.open('popover', closePopover);
  }

  // Estado del servidor: solo Activo o con error (config_error).
  function setServer(port, configError) {
    var pill = byId('server-pill'), txt = byId('server-text');
    if (!pill || !txt) return;
    var bad = !!configError;
    pill.classList.toggle('err', bad);
    pill.classList.toggle('ok', !bad);
    txt.textContent = bad ? 'Servidor con error' : 'Servidor :' + port + ' · Activo';
  }

  function onKeydown(e) {
    if (e.key === 'Escape' && layers.escape()) e.preventDefault();
  }

  function onClickOutside(e) {
    var p = st.popover;
    if (p && !p.pop.contains(e.target) && !(p.btn && p.btn.contains(e.target))) closePopover();
  }

  function on(id, fn) {
    var el = byId(id);
    if (el) el.addEventListener('click', fn);
  }

  function init() {
    var d = root.document;
    if (!d) return;
    d.addEventListener('keydown', onKeydown);
    d.addEventListener('click', onClickOutside);
    on('nav-panel', function () { setView('panel'); });
    on('nav-history', function () { setView('history'); });
    on('hdr-adapters', function () { toggleDrawer('adapters'); });
    on('hdr-settings', function () { toggleDrawer('settings'); });
    on('server-pill', function () {
      var pill = byId('server-pill');
      if (pill && pill.classList.contains('err')) openDrawer('settings');
    });
    on('btn-info', function () {
      var pop = byId('pop-info');
      if (pop) togglePopover(byId('btn-info'), pop);
    });
    on('backdrop', closeDrawer);
    on('drawer-close', closeDrawer);
    setView(st.view);
  }

  var api = {
    createLayers: createLayers, layers: layers, state: st, init: init,
    setView: setView, openDrawer: openDrawer, closeDrawer: closeDrawer, toggleDrawer: toggleDrawer,
    togglePopover: togglePopover, closePopover: closePopover, setServer: setServer
  };
  root.NPerfShell = api;
  if (root.document && root.document.getElementById('app-header')) init();
})(typeof window !== 'undefined' ? window : this);
