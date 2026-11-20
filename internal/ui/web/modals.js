'use strict';
// Modales: genérico (foco atrapado, Escape por la pila de capas), exportación y confirmación de borrado.
// El DOM se construye con createElement/textContent; los datos externos nunca se interpretan como HTML.
(function () {
  var FORMATS = [
    ['csv', 'CSV para Excel (ES)'], ['csv-std', 'CSV estándar'], ['json', 'JSON'], ['html', 'HTML autocontenido']
  ];
  var FOCUSABLE = { BUTTON: 1, INPUT: 1, SELECT: 1, TEXTAREA: 1, A: 1 };
  var current = null;

  function shell() { return window.NPerfShell; }
  function core() { return window.NPerfCore; }

  function mk(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined && text !== null) e.textContent = text;
    return e;
  }

  function errText(e) { return String(e && e.message ? e.message : e); }

  function toast(msg, kind) {
    var c = core();
    if (c && c.pushToast) c.pushToast(msg, kind);
  }

  // Controles enfocables dentro de un nodo (recorrido propio; sin querySelectorAll).
  function focusables(node, out) {
    out = out || [];
    var kids = node.children || [];
    for (var i = 0; i < kids.length; i++) {
      var k = kids[i];
      if (k.hidden) continue;
      if (FOCUSABLE[String(k.tagName || k.tag || '').toUpperCase()] && !k.disabled && k.type !== 'hidden') out.push(k);
      focusables(k, out);
    }
    return out;
  }

  // open abre el modal genérico. Solo hay uno a la vez; devuelve null si ya hay uno abierto.
  // opts: title, content (nodos), links (nodos del pie izquierdo), actions [{label, kind, onClick, id}],
  // initialFocus (nodo), blocking, onClose.
  function open(opts) {
    if (current) return null;
    var prev = document.activeElement;
    var back = mk('div', 'mdl-back');
    var dlg = mk('div', 'mdl');
    var title = mk('h2', '', opts.title || '');
    title.id = 'mdl-title';
    dlg.role = 'dialog';
    dlg.ariaModal = 'true';
    dlg.ariaLabel = opts.title || '';
    dlg.appendChild(title);
    (opts.content || []).forEach(function (n) { dlg.appendChild(n); });
    var foot = mk('div', 'foot');
    var links = mk('div', 'links');
    (opts.links || []).forEach(function (n) { links.appendChild(n); });
    var acts = mk('div', 'acts');
    var buttons = {};
    var h = { el: dlg, back: back, buttons: buttons, closed: false };
    (opts.actions || []).forEach(function (a) {
      var b = mk('button', 'btn' + (a.kind ? ' ' + a.kind : ''), a.label);
      b.type = 'button';
      if (a.id) buttons[a.id] = b;
      b.addEventListener('click', function () { if (a.onClick) a.onClick(h, b); });
      acts.appendChild(b);
    });
    foot.appendChild(links);
    foot.appendChild(acts);
    dlg.appendChild(foot);
    back.appendChild(dlg);

    h.close = function () {
      if (h.closed) return;
      h.closed = true;
      current = null;
      var s = shell();
      if (s) s.layers.shut('modal');
      var c = core();
      if (c) c.ui.activeModal = null;
      if (back.parentNode) back.parentNode.removeChild(back);
      if (opts.onClose) opts.onClose();
      if (prev && prev.focus) prev.focus();
    };
    h.setBlocking = function (b) { var s = shell(); if (s) s.layers.setBlocking('modal', b); };
    h.focusables = function () { return focusables(dlg); };

    back.addEventListener('keydown', function (e) {
      if (e.key !== 'Tab') return;
      var f = focusables(dlg);
      if (!f.length) { e.preventDefault(); return; }
      var act = document.activeElement, first = f[0], last = f[f.length - 1];
      if (f.indexOf(act) < 0) { e.preventDefault(); first.focus(); }
      else if (e.shiftKey && act === first) { e.preventDefault(); last.focus(); }
      else if (!e.shiftKey && act === last) { e.preventDefault(); first.focus(); }
    });

    current = h;
    var c = core();
    if (c) c.ui.activeModal = opts.name || 'modal';
    var s = shell();
    if (s) s.layers.open('modal', h.close, !!opts.blocking);
    document.body.appendChild(back);
    var f0 = opts.initialFocus || focusables(dlg)[0];
    if (f0 && f0.focus) f0.focus();
    return h;
  }

  // ---- exportación ----
  function defaultExport(ids, format, o) { return window.nperf ? window.nperf.exportHistory(ids, format, o) : Promise.reject(new Error('puente no disponible')); }

  // openExport: opts { ids, run(ids, format, o) -> Promise, onDone(result) }.
  function openExport(opts) {
    var ids = (opts && opts.ids) || [];
    if (!ids.length) return null;
    var run = opts.run || defaultExport;
    var exporting = false;
    var sel = mk('select', 'inp');
    FORMATS.forEach(function (f) { var o = mk('option', '', f[1]); o.value = f[0]; sel.appendChild(o); });
    sel.value = 'csv';
    var client = mk('input', 'inp'); client.type = 'text'; client.maxLength = 120;
    var tech = mk('input', 'inp'); tech.type = 'text'; tech.maxLength = 120;
    var notes = mk('textarea', 'inp'); notes.maxLength = 1000;
    var inter = mk('input'); inter.type = 'checkbox';
    function field(label, ctl) { var l = mk('label', 'fld'); l.appendChild(mk('span', '', label)); l.appendChild(ctl); return l; }
    var interLbl = mk('label', 'fld inline');
    interLbl.appendChild(inter);
    interLbl.appendChild(mk('span', '', 'Incluir intervalos (segundo CSV)'));
    var msg = mk('p', 'fnote');
    msg.hidden = true;
    msg.role = 'alert';

    function sync() {
      var csv = sel.value === 'csv' || sel.value === 'csv-std';
      inter.disabled = !csv;
      if (!csv) inter.checked = false;
    }
    sel.addEventListener('change', sync);
    sync();

    var h = open({
      name: 'export',
      title: ids.length === 1 ? 'Exportar registro' : 'Exportar ' + ids.length + ' registros',
      content: [field('Formato', sel), field('Cliente (opcional)', client), field('Técnico (opcional)', tech),
        field('Notas (opcional)', notes), interLbl, msg],
      initialFocus: sel,
      actions: [
        { id: 'cancel', label: 'Cancelar', onClick: function (m) { if (!exporting) m.close(); } },
        {
          id: 'go', label: 'Exportar...', kind: 'primary',
          onClick: function (m, btn) {
            if (exporting) return;
            exporting = true;
            msg.hidden = true;
            m.setBlocking(true);
            m.buttons.cancel.disabled = true;
            var o = { client: client.value.trim(), technician: tech.value.trim(), notes: notes.value.trim(), intervals: !inter.disabled && !!inter.checked };
            var p = Promise.resolve().then(function () { return run(ids, sel.value, o); });
            var c = core();
            (c ? c.busy(btn, p, 'Exportando...') : p).then(function (r) {
              exporting = false;
              if (r === 'cancelado') toast('Exportación cancelada');
              else toast(Array.isArray(r) && r.length ? 'Exportado en: ' + r.join(' y ') : 'Exportación terminada');
              m.close();
              if (opts.onDone) opts.onDone(r);
            }, function (e) {
              exporting = false;
              m.setBlocking(false);
              m.buttons.cancel.disabled = false;
              msg.textContent = 'No se pudo exportar: ' + errText(e);
              msg.hidden = false;
            });
          }
        }
      ]
    });
    if (h) { h.fields = { format: sel, client: client, technician: tech, notes: notes, intervals: inter, message: msg }; }
    return h;
  }

  // ---- confirmación de borrado ----
  // confirmText describe el alcance exacto. scope: 'selected' | 'all'.
  function confirmText(scope, n) {
    var s = n === 1 ? '' : 's';
    if (scope === 'all') return '¿Borrar todo el historial (' + n + ' registro' + s + ')? Esta acción no se puede deshacer.';
    if (n === 1) return '¿Borrar el registro seleccionado? Esta acción no se puede deshacer.';
    return '¿Borrar los ' + n + ' registros seleccionados? Esta acción no se puede deshacer.';
  }

  // openConfirmDelete: opts { scope, count, run() -> Promise, onDone(result) }.
  function openConfirmDelete(opts) {
    var n = (opts && opts.count) | 0;
    if (n < 1) return null;
    var scope = opts.scope === 'all' ? 'all' : 'selected';
    var deleting = false;
    var msg = mk('p', 'fnote');
    msg.hidden = true;
    msg.role = 'alert';
    var h = open({
      name: 'confirm-delete',
      title: scope === 'all' ? 'Borrar todo el historial' : 'Borrar registros',
      content: [mk('p', '', confirmText(scope, n)), msg],
      actions: [
        { id: 'cancel', label: 'Cancelar', onClick: function (m) { if (!deleting) m.close(); } },
        {
          id: 'go', label: scope === 'all' ? 'Borrar todo' : 'Borrar', kind: 'danger',
          onClick: function (m, btn) {
            if (deleting) return;
            deleting = true;
            msg.hidden = true;
            m.setBlocking(true);
            m.buttons.cancel.disabled = true;
            var p = Promise.resolve().then(function () { return opts.run(); });
            var c = core();
            (c ? c.busy(btn, p, 'Borrando...') : p).then(function (r) {
              deleting = false;
              toast('Registros eliminados');
              m.close();
              if (opts.onDone) opts.onDone(r);
            }, function (e) {
              deleting = false;
              m.setBlocking(false);
              m.buttons.cancel.disabled = false;
              msg.textContent = 'No se pudo borrar: ' + errText(e);
              msg.hidden = false;
            });
          }
        }
      ]
    });
    if (h) h.buttons.cancel.focus();
    return h;
  }

  window.NPerfModals = {
    render: function (s) { void s; },
    open: open, openExport: openExport, openConfirmDelete: openConfirmDelete,
    isOpen: function () { return !!current; }, confirmText: confirmText
  };
})();
