/* easywall app.js */
'use strict';

// Translations for the text this file builds itself. base.html inlines them as
// window.easywallStrings; the auth pages do not, and do not need to — none of
// the code that calls str() runs there. Falling back to the key keeps a missing
// blob visible rather than blank.
const str = (key) => (window.easywallStrings && window.easywallStrings[key]) || key;

/* ── Theme ──────────────────────────────────────────────────────────────── */
const normalizeTheme = (t) => {
  if (t === 'dark' || t === 'light') return 'easywall-' + t;
  if (t === 'easywall-dark' || t === 'easywall-light') return t;
  return null;
};

const getTheme = () =>
  normalizeTheme(localStorage.getItem('theme')) ||
  (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'easywall-dark' : 'easywall-light');

// The switch track is drawn from data-theme in CSS, so it needs no help to look
// right — but aria-checked has to be written by hand or the control announces no
// state at all. Null-guarded because applyTheme also runs before the DOM exists.
const syncThemeSwitch = (t) => {
  const btn = document.getElementById('theme-toggle-btn');
  if (btn) btn.setAttribute('aria-checked', String(t === 'easywall-light'));
};

const applyTheme = (t) => {
  document.documentElement.setAttribute('data-theme', t);
  localStorage.setItem('theme', t);
  syncThemeSwitch(t);
};

// Apply immediately to prevent flash
applyTheme(getTheme());

const toggleTheme = () => applyTheme(getTheme() === 'easywall-dark' ? 'easywall-light' : 'easywall-dark');

/* ── Mobile sidebar ──────────────────────────────────────────────────────── */
document.addEventListener('DOMContentLoaded', () => {
  const themeBtn = document.getElementById('theme-toggle-btn');
  if (themeBtn) themeBtn.addEventListener('click', toggleTheme);
  syncThemeSwitch(getTheme());

  const sidebar = document.getElementById('sidebar');
  const overlay = document.getElementById('overlay');
  const menuBtn = document.getElementById('menu-btn');

  if (menuBtn && sidebar && overlay) {
    menuBtn.addEventListener('click', () => {
      sidebar.classList.toggle('open');
      overlay.classList.toggle('visible');
    });
    overlay.addEventListener('click', () => {
      sidebar.classList.remove('open');
      overlay.classList.remove('visible');
    });
  }

  /* ── Auto-dismiss flash messages ─────────────────────────────────────── */
  const flash = document.querySelector('.alert[data-auto-dismiss]');
  if (flash) setTimeout(() => flash.remove(), 4000);

  /* ── Port rule editor ───────────────────────────────────────────────── */
  initRuleEditor();

  /* ── Forwarding rule editor ─────────────────────────────────────────── */
  initForwardingEditor();

  /* ── Apply status polling ───────────────────────────────────────────── */
  initApplyStatus();

  /* ── HTMX toast feedback (HX-Trigger: easywall:saved / easywall:error) ─ */
  initHtmxToast();

  /* ── Live entry counters for the list editors ───────────────────────── */
  initListCounter('iplist-input', 'iplist-count', str('count_entry_one'), str('count_entry_many'));
  initListCounter('custom-input', 'custom-count', str('count_rule_one'), str('count_rule_many'));
});

/* ── List editor counter ──────────────────────────────────────────────────
   Counts the same way the server does — blank lines and comments are not
   entries — so the number does not jump when the page reloads. */
function initListCounter(inputId, outId, one, many) {
  const input = document.getElementById(inputId);
  const out   = document.getElementById(outId);
  if (!input || !out) return;

  const update = () => {
    const n = input.value
      .split('\n')
      .map(l => l.trim())
      .filter(l => l !== '' && !l.startsWith('#'))
      .length;
    out.textContent = `${n} ${n === 1 ? one : many}`;
  };

  input.addEventListener('input', update);
  update();
}

/* ── HTMX toast ───────────────────────────────────────────────────────────
   Listens for custom events that the server fires via the HX-Trigger
   response header. Shows a small auto-dismissing alert in the toast
   container at the bottom-right of the page. */
function initHtmxToast() {
  const container = document.getElementById('toast-container');
  if (!container) return;

  // Keys match the flashKey values the server sends. The text comes from the
  // locale so a toast is in the same language as the page that raised it.
  const messages = {
    saved:                    { text: str('saved'), kind: 'success' },
    options_saved:            { text: str('options_saved'), kind: 'success' },
    settings_saved:           { text: str('settings_saved'), kind: 'success' },
    system_saved:             { text: str('system_saved'), kind: 'success' },
    save_error:               { text: str('save_error'), kind: 'error' },
    system_invalid_duration:  { text: str('system_invalid_duration'), kind: 'warning' },
  };

  const show = (key, kind) => {
    const msg = messages[key] || { text: key, kind: kind || 'info' };
    const k = msg.kind || kind || 'info';
    // Map to the alert variants the stylesheet actually defines. There is no
    // informational variant by design — only firewall state carries colour.
    const variant = { success: 'alert-ok', error: 'alert-crit', warning: 'alert-warn' }[k] || '';
    const el = document.createElement('div');
    el.setAttribute('role', 'alert');
    el.className = `alert ${variant} toast-item`.replace(/\s+/g, ' ').trim();
    el.innerHTML = `<span>${esc(msg.text)}</span>`;
    container.appendChild(el);
    // Auto-dismiss after 2.5 seconds. The fade lives in CSS (.is-leaving) —
    // setting .style.* here would violate style-src 'self'.
    setTimeout(() => {
      el.classList.add('is-leaving');
      setTimeout(() => el.remove(), 300);
    }, 2500);
  };

  // We parse the HX-Trigger header manually in htmx:afterRequest. HTMX's
  // own auto-dispatch of HX-Trigger custom events with namespaced names
  // (containing ":") is unreliable — by the time we'd attach a listener
  // on a colon-namespaced event, the event may have already fired or
  // the dispatch context may differ from document.body. Reading the
  // header directly is deterministic and one-line simpler.
  document.body.addEventListener('htmx:afterRequest', e => {
    const trigger = e.detail?.xhr?.getResponseHeader('HX-Trigger');
    if (!trigger) return;
    let parsed;
    try { parsed = JSON.parse(trigger); } catch { return; }
    if (parsed['easywall:saved']) show(parsed['easywall:saved'], 'success');
    if (parsed['easywall:error']) show(parsed['easywall:error'], 'error');
  });
}

/* ── Port / Custom rule editor ───────────────────────────────────────────── */
function initRuleEditor() {
  const tbody   = document.getElementById('rules-tbody');
  const hidden  = document.getElementById('rules-json');
  const addBtn  = document.getElementById('add-rule-btn');
  const ruleType = document.getElementById('rule-type')?.value;

  if (!tbody || !hidden) return;

  const isSimple = tbody.dataset.simple === 'true'; // blacklist/whitelist use simple text

  // Column headings, in order, taken from the rendered table so client-built
  // rows carry the same labels the server used — in whatever language.
  const headLabels = () =>
    [...(tbody.closest('table')?.querySelectorAll('thead th') || [])].map(th => th.textContent.trim());

  const syncHidden = () => {
    if (isSimple) return; // handled differently
    const rows = [...tbody.querySelectorAll('tr[data-idx]')];
    const rules = rows.map(tr => {
      const port = tr.querySelector('.f-port')?.value.trim() ?? '';
      const desc = tr.querySelector('.f-desc')?.value.trim() ?? '';
      const ssh  = tr.querySelector('.f-ssh')?.checked ?? false;
      return { port, description: desc, ssh };
    }).filter(r => r.port);
    hidden.value = JSON.stringify(rules);
  };

  tbody.addEventListener('input', syncHidden);
  tbody.addEventListener('change', syncHidden);

  tbody.addEventListener('click', e => {
    const del = e.target.closest('.del-rule');
    if (del) {
      del.closest('tr').remove();
      syncHidden();
      updateRuleCount();
    }
  });

  if (addBtn) {
    addBtn.addEventListener('click', () => {
      const idx = tbody.querySelectorAll('tr').length;
      const tr = document.createElement('tr');
      tr.dataset.idx = idx;
      tr.innerHTML = ruleRowHTML(idx, { port: '', description: '', ssh: false }, headLabels());
      tbody.appendChild(tr);
      tr.querySelector('.f-port')?.focus();
      syncHidden();
      updateRuleCount();
    });
  }

  // Initial sync
  syncHidden();
  initRuleFilter();
}

/* ── Row filter ───────────────────────────────────────────────────────────
   A port table can hold fifty entries; scrolling to find one is the wrong
   interaction. Filtering happens in the browser because the rows are already
   here — a round trip would be slower and would discard unsaved edits. */
function updateRuleCount() {
  const tbody = document.getElementById('rules-tbody');
  const out   = document.getElementById('rules-count');
  if (!tbody || !out) return;
  const all     = [...tbody.querySelectorAll('tr[data-idx]')];
  const visible = all.filter(tr => !tr.hidden);
  const noun = (n) => str(n === 1 ? 'count_rule_one' : 'count_rule_many');
  // "3 of 8 rules" reorders in other languages ("3 von 8 Regeln"), so the whole
  // phrase is one message with slots rather than a concatenation.
  out.textContent = visible.length === all.length
    ? `${all.length} ${noun(all.length)}`
    : str('count_filtered')
        .replace('{shown}', visible.length)
        .replace('{total}', all.length)
        .replace('{noun}', noun(all.length));
}

function initRuleFilter() {
  const input   = document.getElementById('rules-filter');
  const tbody   = document.getElementById('rules-tbody');
  const noMatch = document.getElementById('rules-no-match');
  if (!input || !tbody) return;

  input.addEventListener('input', () => {
    const q = input.value.trim().toLowerCase();
    let shown = 0;
    tbody.querySelectorAll('tr[data-idx]').forEach(tr => {
      const port = tr.querySelector('.f-port')?.value.toLowerCase() ?? '';
      const desc = tr.querySelector('.f-desc')?.value.toLowerCase() ?? '';
      const hit  = !q || port.includes(q) || desc.includes(q);
      tr.toggleAttribute('hidden', !hit);
      if (hit) shown++;
    });
    if (noMatch) noMatch.toggleAttribute('hidden', shown > 0 || !q);
    updateRuleCount();
  });
}

// Must stay identical to the server-rendered row in ports.html — same column
// order, same classes, same data-label values, or a row added in the browser
// looks nothing like the rest of the table (and loses its labels on mobile).
// The labels are read out of the table header rather than written here, so a
// row added client-side is labelled in the interface's own language.
function ruleRowHTML(idx, r, labels) {
  const L = labels || [];
  return `
    <td data-label="${esc(L[0] ?? '')}"><input class="f-port input-cell input-cell-data" type="text" value="${esc(r.port)}"
         placeholder="${esc(str('ports_port_hint'))}" aria-label="${esc(L[0] ?? '')}"></td>
    <td data-label="${esc(L[1] ?? '')}">
      <input class="f-ssh checkbox" type="checkbox" ${r.ssh ? 'checked' : ''} aria-label="${esc(L[1] ?? '')}">
    </td>
    <td class="cell-wide" data-label="${esc(L[2] ?? '')}"><input class="f-desc input-cell" type="text" value="${esc(r.description)}"
         placeholder="${esc(str('ports_desc_hint'))}" aria-label="${esc(L[2] ?? '')}"></td>
    <td>
      <button type="button" class="btn-icon btn-icon-danger del-rule row-action" title="${esc(str('action_remove_rule'))}">
        <svg class="size-4" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd"
          d="M9 2a1 1 0 00-.894.553L7.382 4H4a1 1 0 000 2v10a2 2 0 002 2h8a2 2 0 002-2V6a1 1 0 100-2h-3.382l-.724-1.447A1 1 0 0011 2H9zM7 8a1 1 0 012 0v6a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v6a1 1 0 102 0V8a1 1 0 00-1-1z"
          clip-rule="evenodd"/></svg>
      </button>
    </td>`;
}

/* ── Forwarding editor ────────────────────────────────────────────────────── */
function initForwardingEditor() {
  const tbody  = document.getElementById('fwd-tbody');
  const hidden = document.getElementById('fwd-json');
  const addBtn = document.getElementById('add-fwd-btn');

  if (!tbody || !hidden) return;

  const headLabels = () =>
    [...(tbody.closest('table')?.querySelectorAll('thead th') || [])].map(th => th.textContent.trim());

  const syncHidden = () => {
    const rows = [...tbody.querySelectorAll('tr[data-idx]')];
    const rules = rows.map(tr => ({
      protocol:    tr.querySelector('.f-proto')?.value ?? 'tcp',
      source_port: parseInt(tr.querySelector('.f-src')?.value ?? '0', 10),
      dest_port:   parseInt(tr.querySelector('.f-dst')?.value ?? '0', 10),
    })).filter(r => r.source_port > 0 && r.dest_port > 0);
    hidden.value = JSON.stringify(rules);
  };

  tbody.addEventListener('input',  syncHidden);
  tbody.addEventListener('change', syncHidden);

  tbody.addEventListener('click', e => {
    const del = e.target.closest('.del-rule');
    if (del) { del.closest('tr').remove(); syncHidden(); }
  });

  if (addBtn) {
    addBtn.addEventListener('click', () => {
      const idx = tbody.querySelectorAll('tr').length;
      const tr  = document.createElement('tr');
      tr.dataset.idx = idx;
      tr.innerHTML   = fwdRowHTML(idx, { protocol: 'tcp', source_port: '', dest_port: '' }, headLabels());
      tbody.appendChild(tr);
      tr.querySelector('.f-src')?.focus();
      syncHidden();
    });
  }

  syncHidden();
}

// Mirrors the server-rendered row in forwarding.html, labels included.
function fwdRowHTML(idx, r, labels) {
  const L = labels || [];
  const tcpSel = r.protocol === 'tcp' ? 'selected' : '';
  const udpSel = r.protocol === 'udp' ? 'selected' : '';
  return `
    <td data-label="${esc(L[0] ?? '')}">
      <select class="f-proto input-cell input-cell-data" aria-label="${esc(L[0] ?? '')}">
        <option value="tcp" ${tcpSel}>tcp</option>
        <option value="udp" ${udpSel}>udp</option>
      </select>
    </td>
    <td data-label="${esc(L[1] ?? '')}"><input class="f-src input-cell input-cell-data" type="number" min="1" max="65535" value="${esc(String(r.source_port))}"
         placeholder="${esc(str('port_range_hint'))}" aria-label="${esc(L[1] ?? '')}"></td>
    <td class="cell-flow" data-label="${esc(L[2] ?? '')}">
      <span class="flow-arrow" aria-hidden="true">&rarr;</span>
      <input class="f-dst input-cell input-cell-data" type="number" min="1" max="65535" value="${esc(String(r.dest_port))}"
         placeholder="${esc(str('port_range_hint'))}" aria-label="${esc(L[2] ?? '')}"></td>
    <td>
      <button type="button" class="btn-icon btn-icon-danger del-rule row-action" title="${esc(str('action_remove_rule'))}">
        <svg class="size-4" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd"
          d="M9 2a1 1 0 00-.894.553L7.382 4H4a1 1 0 000 2v10a2 2 0 002 2h8a2 2 0 002-2V6a1 1 0 100-2h-3.382l-.724-1.447A1 1 0 0011 2H9zM7 8a1 1 0 012 0v6a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v6a1 1 0 102 0V8a1 1 0 00-1-1z"
          clip-rule="evenodd"/></svg>
      </button>
    </td>`;
}

/* ── Apply status polling ─────────────────────────────────────────────────── */
function initApplyStatus() {
  const statusEl = document.getElementById('apply-status');
  if (!statusEl) return;

  let timer = null;

  // Same four words the server renders into apply.html, from the same keys —
  // the polled update used to say "Rolled Back" where the page said "Rolled back".
  const statusLabels = {
    idle:        { text: str('state_idle'),         cls: 'inactive' },
    pending:     { text: str('state_pending'),      cls: 'pending'  },
    accepted:    { text: str('state_accepted'),     cls: 'active'   },
    rolled_back: { text: str('state_rolled_back'),  cls: 'error'    },
  };

  const render = (data) => {
    const s = statusLabels[data.acceptance] || statusLabels.idle;
    statusEl.innerHTML = `<span class="status-dot ${s.cls}">${s.text}</span>`;

    const confirmBtn = document.getElementById('confirm-btn');
    const startBtn   = document.getElementById('start-btn');

    // toggleAttribute('hidden') instead of .style.display: an inline style
    // would violate style-src 'self'.
    const pending = data.acceptance === 'pending';
    if (confirmBtn) confirmBtn.toggleAttribute('hidden', !pending);
    if (startBtn)   startBtn.toggleAttribute('hidden', pending);

    // rolled_back is terminal — stop polling and show the error banner.
    // accepted is NOT terminal: Reset() returns the state to idle within milliseconds,
    // so keep polling and let the UI transition naturally to idle.
    if (data.acceptance === 'rolled_back') {
      if (timer) { clearInterval(timer); timer = null; }
      statusEl.insertAdjacentHTML('afterend',
        `<div role="alert" class="alert alert-crit mt-3"><span>${esc(str('apply_rolled_back_toast'))}</span></div>`);
    }
  };

  const poll = () => {
    fetch('/apply/status')
      .then(r => r.json())
      .then(render)
      .catch(() => {}); // silent on network error
  };

  poll();
  timer = setInterval(poll, 2000);
}

/* ── Utilities ────────────────────────────────────────────────────────────── */
function esc(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}
