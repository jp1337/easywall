/* easywall app.js */
'use strict';

/* ── Theme ──────────────────────────────────────────────────────────────── */
const normalizeTheme = (t) => {
  if (t === 'dark' || t === 'light') return 'easywall-' + t;
  if (t === 'easywall-dark' || t === 'easywall-light') return t;
  return null;
};

const getTheme = () =>
  normalizeTheme(localStorage.getItem('theme')) ||
  (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'easywall-dark' : 'easywall-light');

const applyTheme = (t) => {
  document.documentElement.setAttribute('data-theme', t);
  localStorage.setItem('theme', t);
};

// Apply immediately to prevent flash
applyTheme(getTheme());

const toggleTheme = () => applyTheme(getTheme() === 'easywall-dark' ? 'easywall-light' : 'easywall-dark');

/* ── Mobile sidebar ──────────────────────────────────────────────────────── */
document.addEventListener('DOMContentLoaded', () => {
  const themeBtn = document.getElementById('theme-toggle-btn');
  if (themeBtn) themeBtn.addEventListener('click', toggleTheme);

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

  /* ── Options icon sync ──────────────────────────────────────────────── */
  initOptionIcons();
});

/* ── Port / Custom rule editor ───────────────────────────────────────────── */
function initRuleEditor() {
  const tbody   = document.getElementById('rules-tbody');
  const hidden  = document.getElementById('rules-json');
  const addBtn  = document.getElementById('add-rule-btn');
  const ruleType = document.getElementById('rule-type')?.value;

  if (!tbody || !hidden) return;

  const isSimple = tbody.dataset.simple === 'true'; // blacklist/whitelist use simple text

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
    }
  });

  if (addBtn) {
    addBtn.addEventListener('click', () => {
      const idx = tbody.querySelectorAll('tr').length;
      const tr = document.createElement('tr');
      tr.dataset.idx = idx;
      tr.innerHTML = ruleRowHTML(idx, { port: '', description: '', ssh: false });
      tbody.appendChild(tr);
      tr.querySelector('.f-port')?.focus();
      syncHidden();
    });
  }

  // Initial sync
  syncHidden();
}

function ruleRowHTML(idx, r) {
  return `
    <td><input class="f-port w-port" type="text" value="${esc(r.port)}"
         placeholder="80 or 8000:9000"></td>
    <td><input class="f-desc w-desc" type="text" value="${esc(r.description)}"
         placeholder="Description"></td>
    <td class="td-center">
      <input class="f-ssh" type="checkbox" ${r.ssh ? 'checked' : ''}>
    </td>
    <td class="td-actions">
      <button type="button" class="btn btn-ghost btn-sm del-rule" title="Remove">
        <svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd"
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
      tr.innerHTML   = fwdRowHTML(idx, { protocol: 'tcp', source_port: '', dest_port: '' });
      tbody.appendChild(tr);
      tr.querySelector('.f-src')?.focus();
      syncHidden();
    });
  }

  syncHidden();
}

function fwdRowHTML(idx, r) {
  const tcpSel = r.protocol === 'tcp' ? 'selected' : '';
  const udpSel = r.protocol === 'udp' ? 'selected' : '';
  return `
    <td>
      <select class="f-proto f-proto-select">
        <option value="tcp" ${tcpSel}>tcp</option>
        <option value="udp" ${udpSel}>udp</option>
      </select>
    </td>
    <td><input class="f-src w-port-num" type="number" min="1" max="65535" value="${esc(String(r.source_port))}"
         placeholder="1–65535"></td>
    <td><input class="f-dst w-port-num" type="number" min="1" max="65535" value="${esc(String(r.dest_port))}"
         placeholder="1–65535"></td>
    <td class="td-actions">
      <button type="button" class="btn btn-ghost btn-sm del-rule" title="Remove">
        <svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd"
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

  const statusLabels = {
    idle:        { text: 'Idle',         cls: 'inactive' },
    pending:     { text: 'Pending',      cls: 'pending'  },
    accepted:    { text: 'Accepted',     cls: 'active'   },
    rolled_back: { text: 'Rolled Back',  cls: 'error'    },
  };

  const render = (data) => {
    const s = statusLabels[data.acceptance] || statusLabels.idle;
    statusEl.innerHTML = `<span class="status-dot ${s.cls}">${s.text}</span>`;

    const confirmBtn = document.getElementById('confirm-btn');
    const startBtn   = document.getElementById('start-btn');

    if (data.acceptance === 'pending') {
      if (confirmBtn) confirmBtn.style.display = '';
      if (startBtn)   startBtn.style.display = 'none';
    } else {
      if (confirmBtn) confirmBtn.style.display = 'none';
      if (startBtn)   startBtn.style.display = '';
    }

    // rolled_back is terminal — stop polling and show the error banner.
    // accepted is NOT terminal: Reset() returns the state to idle within milliseconds,
    // so keep polling and let the UI transition naturally to idle.
    if (data.acceptance === 'rolled_back') {
      if (timer) { clearInterval(timer); timer = null; }
      statusEl.insertAdjacentHTML('afterend',
        '<div class="alert alert-error mt-3">Rules were rolled back — acceptance timeout.</div>');
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

/* ── Options / Settings icon sync ────────────────────────────────────────── */
function initOptionIcons() {
  document.querySelectorAll('.opt-block .option-item').forEach(item => {
    const toggle = item.querySelector('.opt-toggle input');
    const icon   = item.querySelector('.option-icon');
    if (!toggle || !icon) return;
    const update = () => {
      icon.classList.toggle('on',  toggle.checked);
      icon.classList.toggle('off', !toggle.checked);
    };
    toggle.addEventListener('change', update);
  });
}

/* ── Utilities ────────────────────────────────────────────────────────────── */
function esc(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}
