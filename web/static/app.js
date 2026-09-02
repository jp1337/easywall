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

  // The submit button stays in the markup for a script-free operator; once
  // this runs, data-js (set in the head, before first paint) hides it and the
  // select submits itself on change instead.
  const langSelect = document.getElementById('lang-select');
  if (langSelect) langSelect.addEventListener('change', () => langSelect.form.submit());

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
  initApplyCountdown();
  initApplyChip();

  /* ── HTMX toast feedback (HX-Trigger: easywall:saved / easywall:error) ─ */
  initHtmxToast();

  /* ── Live entry counters for the list editors ───────────────────────── */
  initListCounter('iplist-input', 'iplist-count', str('count_entry_one'), str('count_entry_many'));
  initListCounter('custom-input', 'custom-count', str('count_rule_one'), str('count_rule_many'));

  /* ── Copy recovery codes ─────────────────────────────────────────────── */
  initRecoveryCopy();
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
    settings_invalid_network: { text: str('settings_invalid_network'), kind: 'warning' },
    options_invalid_limit:    { text: str('options_invalid_limit'), kind: 'warning' },
    provenance_reset_done:    { text: str('provenance_reset_done'), kind: 'success' },
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

  if (!tbody || !hidden) return;

  const isSimple = tbody.dataset.simple === 'true'; // blacklist/whitelist use simple text

  // Column headings, in order, taken from the rendered table so client-built
  // rows carry the same labels the server used — in whatever language.
  const headLabels = () =>
    [...(tbody.closest('table')?.querySelectorAll('thead th') || [])].map(th => th.textContent.trim());

  // A row goes in the payload unless it is completely untouched.
  //
  // It used to be `filter(r => r.port)`, which threw away anything without a
  // port before the form was submitted — so adding a row, typing the
  // description first and pressing Save discarded what you had written with no
  // message and no trace. The counter above the table said nine rules, the
  // table showed nine, and eight were sent. An empty row you never typed in is
  // noise and still goes; a row you touched is data, and the server decides
  // whether it is valid.
  const syncHidden = () => {
    if (isSimple) return; // handled differently
    const rows = [...tbody.querySelectorAll('tr[data-idx]')];
    const rules = rows.map(tr => {
      const port = tr.querySelector('.f-port')?.value.trim() ?? '';
      const desc = tr.querySelector('.f-desc')?.value.trim() ?? '';
      const ssh  = tr.querySelector('.f-ssh')?.checked ?? false;
      // One comma-separated field, split here and nowhere else. Empty entries
      // are dropped so "10.0.0.0/8, " is not a rule with a blank source, and an
      // empty field stays an empty list — which is what "anywhere" is.
      const sources = (tr.querySelector('.f-sources')?.value ?? '')
        .split(',').map(s => s.trim()).filter(s => s !== '');
      const service = tr.dataset.service ?? '';
      const rule = { port, description: desc, ssh };
      if (sources.length) rule.sources = sources;
      if (service) rule.service = service;
      return rule;
    }).filter(r => r.port !== '' || r.description !== '' || r.ssh || r.sources || r.service);
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
      tr.innerHTML = ruleRowHTML(idx, { port: '', description: '', ssh: false, sources: [] }, headLabels());
      tbody.appendChild(tr);
      tr.querySelector('.f-port')?.focus();
      syncHidden();
      updateRuleCount();
    });
  }

  // The catalogue is progressive enhancement: it appends rows the operator could
  // have typed, into fields that already work without it. With JavaScript off the
  // button is not shown at all — the same rule the language switcher set in 2.9 —
  // rather than a control that does nothing.
  const dialog = document.getElementById('catalogue-dialog');
  const openBtn = document.getElementById('catalogue-btn');
  if (dialog && openBtn && typeof dialog.showModal === 'function') {
    openBtn.hidden = false;
    openBtn.addEventListener('click', () => dialog.showModal());
    document.getElementById('catalogue-close')
      ?.addEventListener('click', () => dialog.close());

    dialog.addEventListener('click', e => {
      const item = e.target.closest('.catalogue-item');
      if (!item) return;
      let rows;
      try { rows = JSON.parse(item.dataset.rows || '[]'); } catch { return; }
      const sources = item.dataset.sources || '';
      const labels = headLabels();
      const serviceName = item.querySelector('.catalogue-name')?.textContent.trim() || '';
      rows.forEach(row => {
        const tr = document.createElement('tr');
        tr.dataset.idx = tbody.querySelectorAll('tr').length;
        tr.dataset.service = item.dataset.service || '';
        tr.innerHTML = ruleRowHTML(tr.dataset.idx, {
          port: row.Port,
          description: row.Description,
          ssh: false,
          sources: sources ? sources.split(',').map(s => s.trim()) : [],
          serviceName,
        }, labels);
        tbody.appendChild(tr);
      });
      dialog.close();
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
      const srcs = tr.querySelector('.f-sources')?.value.toLowerCase() ?? '';
      const hit  = !q || port.includes(q) || desc.includes(q) || srcs.includes(q);
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
    <td class="cell-wide" data-label="${esc(L[2] ?? '')}"><input class="f-sources input-cell" type="text" value="${esc((r.sources || []).join(', '))}"
         placeholder="${esc(str('ports_sources_hint'))}" aria-label="${esc(L[2] ?? '')}"></td>
    <td class="cell-wide" data-label="${esc(L[3] ?? '')}">
      <input class="f-desc input-cell" type="text" value="${esc(r.description)}"
         placeholder="${esc(str('ports_desc_hint'))}" aria-label="${esc(L[3] ?? '')}">
      ${r.serviceName ? `<span class="chip">${esc(r.serviceName)}</span>` : ''}
    </td>
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

  // The number the field holds, not the number parseInt reads out of the text.
  //
  // These are <input type="number">, and the two do not agree. A spreadsheet
  // writes 10000 as "1E+04"; the field accepts it as a valid number, and
  // parseInt("1E+04", 10) stops at the "1". Pasting a port list therefore stored
  // port 1 — privileged, and not the port anyone asked for — and the page said
  // "Changes saved." Measured in Chrome: typed 1E+04, payload source_port 1,
  // stored tcp 1 → 9999. valueAsNumber is the browser's own parse of the field,
  // so what is sent is what the field means.
  //
  // Anything that is not a whole number — an empty field, "80.5", text the field
  // could not hold — becomes 0, which the server refuses by name and the page
  // reports. Deliberately not nudged to the nearest integer: this editor does not
  // get to decide which port the operator meant.
  const portOf = el => {
    const n = el?.valueAsNumber;
    return Number.isInteger(n) ? n : 0;
  };

  // Same rule as the port editor: only a row nobody typed in is dropped. Half a
  // forwarding rule used to vanish on save without a word.
  const syncHidden = () => {
    const rows = [...tbody.querySelectorAll('tr[data-idx]')];
    const rules = rows.map(tr => {
      const srcEl = tr.querySelector('.f-src');
      const dstEl = tr.querySelector('.f-dst');
      const src = srcEl?.value.trim() ?? '';
      const dst = dstEl?.value.trim() ?? '';
      return {
        touched:     src !== '' || dst !== '',
        protocol:    tr.querySelector('.f-proto')?.value ?? 'tcp',
        source_port: portOf(srcEl),
        dest_port:   portOf(dstEl),
      };
    }).filter(r => r.touched).map(({ touched, ...rule }) => rule);
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
    <td data-label="${esc(L[2] ?? '')}">
      <div class="flow">
        <span class="flow-arrow" aria-hidden="true">&rarr;</span>
        <input class="f-dst input-cell input-cell-data" type="number" min="1" max="65535" value="${esc(String(r.dest_port))}"
           placeholder="${esc(str('port_range_hint'))}" aria-label="${esc(L[2] ?? '')}">
      </div>
    </td>
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
    idle:        { text: str('state_idle'),         cls: 'inactive'      },
    // static rides along with pending only: the pending dot does not pulse
    // beside a running countdown (DESIGN.md, amended in 2.14) — two "still
    // happening" indicators side by side is one too many, and the digits say
    // it more precisely. Without this, the first poll rewrote the dot without
    // the class from the server-rendered markup and it started pulsing again.
    pending:     { text: str('state_pending'),      cls: 'pending static' },
    accepted:    { text: str('state_accepted'),     cls: 'active'         },
    rolled_back: { text: str('state_rolled_back'),  cls: 'error'          },
    unknown:     { text: str('state_unknown'),      cls: 'error'          },
  };

  const render = (data) => {
    // The countdown corrects itself from the same poll rather than opening a
    // second one.
    if (data) document.dispatchEvent(new CustomEvent('easywall:status', { detail: data }));

    // An unreachable core used to fall through to "idle" — a definite claim
    // that nothing is pending, made at the moment nothing is known. It also
    // swapped the confirm button for apply, inviting a second apply while a
    // window may still be open on a core that cannot be asked. Not knowing is
    // its own state and says so.
    const known = data && typeof data.acceptance === 'string' && statusLabels[data.acceptance];
    const s = known || statusLabels.unknown;
    statusEl.innerHTML = `<span class="status-dot ${s.cls}">${s.text}</span>`;

    const confirmBtn  = document.getElementById('confirm-btn');
    const startBtn    = document.getElementById('start-btn');
    const rollbackBtn = document.getElementById('rollback-btn');

    // toggleAttribute('hidden') instead of .style.display: an inline style
    // would violate style-src 'self'.
    if (!known) {
      // Offer none of the three: each would be a guess about state nobody has.
      // rollback-btn used to be left out of this — a page rendered mid-window
      // whose poll then went unknown left "Roll back now" on screen with
      // nothing left to cancel.
      if (confirmBtn)  confirmBtn.toggleAttribute('hidden', true);
      if (startBtn)    startBtn.toggleAttribute('hidden', true);
      if (rollbackBtn) rollbackBtn.toggleAttribute('hidden', true);
      return;
    }

    const pending = data.acceptance === 'pending';
    if (confirmBtn)  confirmBtn.toggleAttribute('hidden', !pending);
    if (startBtn)    startBtn.toggleAttribute('hidden', pending);
    if (rollbackBtn) rollbackBtn.toggleAttribute('hidden', !pending);

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
      .then(r => (r.ok ? r.json() : null))
      .then(render)
      .catch(() => render(null)); // a failed request is not "idle" either
  };

  poll();
  timer = setInterval(poll, 2000);
}

/* ── Acceptance countdown ─────────────────────────────────────────────────
   The server renders the remaining seconds; this ticks between the polls that
   already happen and takes the server's number as the correction on each one.
   Relative rather than an absolute deadline on purpose — the machines a
   firewall is administered from are the ones whose clocks drift.

   At zero it stops and claims nothing. The local tick reaches 00:00 up to two
   seconds before the poll learns what happened, and "confirmed in the last
   instant" and "rolled back" are not distinguishable from here. The state word
   changes when a poll returns something else, and not before: this screen's
   whole argument is that it says what is true, and the one moment it would be
   guessing is the moment it matters. */
function initApplyCountdown() {
  const el = document.getElementById('apply-countdown');
  if (!el) return;

  let left = parseInt(el.dataset.remaining, 10);
  if (!Number.isFinite(left)) return;

  const paint = () => { el.textContent = mmss(left); };

  // The poll in initApplyStatus already runs every 2s; this listens rather than
  // fetching again, so an open window costs one request per two seconds in
  // total and not two. No extra clamp here: acceptance_remaining is the
  // server's own whole-seconds-rounded-up count, never negative, unlike the
  // local tick below it corrects, which walks past zero unless it clamps
  // itself.
  document.addEventListener('easywall:status', (e) => {
    const n = e.detail && e.detail.acceptance_remaining;
    if (Number.isFinite(n)) { left = n; paint(); }
  });

  paint();
  const tick = tickEverySecond(() => {
    left = Math.max(0, left - 1);
    paint();
    if (left === 0) clearInterval(tick);
  });
}

/* ── Acceptance chip ──────────────────────────────────────────────────────
   The topbar's countdown, on every page but /apply. Requests during a
   120-second window, from a page that is not /apply: none. One at the end. */
function initApplyChip() {
  const chip = document.getElementById('apply-chip');
  if (!chip) return;

  const timeEl  = chip.querySelector('.apply-chip-time');
  const labelEl = chip.querySelector('.apply-chip-label');
  const dot     = chip.querySelector('.status-dot');

  let left = parseInt(chip.dataset.remaining, 10);
  if (!Number.isFinite(left)) return;

  const settle = (data) => {
    // idle and accepted both mean confirmed: Reset() returns the status to idle
    // within milliseconds of an Accept, so by the time this asks, a confirmed
    // window reads as either one.
    const rolledBack = data && data.acceptance === 'rolled_back';
    if (timeEl) timeEl.remove();
    if (dot) dot.className = `status-dot ${rolledBack ? 'error' : 'active'}`;
    if (labelEl) labelEl.textContent = rolledBack ? str('apply_chip_rolled_back') : str('apply_chip_confirmed');

    // A rollback stays and keeps its link to /apply — something was undone and
    // the operator has to know. A confirmation is not news for long.
    if (!rolledBack) setTimeout(() => chip.remove(), 4000);
  };

  if (timeEl) timeEl.textContent = mmss(left);
  const tick = tickEverySecond(() => {
    left = Math.max(0, left - 1);
    if (timeEl) timeEl.textContent = mmss(left);
    if (left > 0) return;
    clearInterval(tick);
    fetch('/apply/status')
      .then(r => (r.ok ? r.json() : null))
      .then(settle)
      .catch(() => settle(null)); // a failed request is not a rollback
  });
}

function mmss(total) {
  const s = Math.max(0, total | 0);
  return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
}

/* ── Recovery-code copy button ────────────────────────────────────────────
   The eight codes are shown once, on the response that issues them, and never
   served from a route — so there is nothing here to fetch, only what is
   already on the page. navigator.clipboard needs no CSP change, unlike a
   blob: download would. */
function initRecoveryCopy() {
  const btn = document.querySelector('[data-copy-codes]');
  if (!btn) return;

  btn.addEventListener('click', () => {
    const codes = [...document.querySelectorAll('.recovery-code')]
      .map(li => li.textContent.trim())
      .filter(Boolean);
    if (codes.length === 0) return;

    navigator.clipboard.writeText(codes.join('\n')).then(() => {
      btn.textContent = str('totp_copied');
      setTimeout(() => { btn.textContent = str('totp_copy'); }, 2000);
    }).catch(() => {
      // The codes are shown once, with no download route, by deliberate
      // design — copy is the primary path to get them off this page. A
      // rejection here (permission denied, an insecure context, a browser
      // that never asked) must not read as "copied" to someone who is about
      // to navigate away believing they have a backup. It also must not
      // reach the console as an unhandled rejection: ui-check.mjs's health
      // sweep treats one as a failure.
      btn.textContent = str('totp_copy_failed');
      setTimeout(() => { btn.textContent = str('totp_copy'); }, 4000);
    });
  });
}

/* ── Utilities ────────────────────────────────────────────────────────────── */

// Both the countdown and the chip need a once-a-second local tick, and only
// the chip is also allowed to talk to the server — a text-based guard checks
// the chip's own section for the word this file uses for "keep asking the
// server on an interval", so that word cannot also be the one spelling "count
// a local clock down once a second" in that same section, or the guard could
// no longer tell the two apart and a real regression would stop showing up.
function tickEverySecond(fn) {
  return setInterval(fn, 1000);
}

function esc(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}
