#!/usr/bin/env node
/**
 * Measures the prose in docs/_docs, so that "sentences over 30 words" means the
 * same thing to two people on two days. The spec that produced this branch and
 * the plan that implements it counted 42 and 63 with the same intention and
 * different filters; this file is the tiebreak, not the authority on style.
 *
 * Only docs/_docs is walked — docs/ also holds assets and layouts that are not
 * prose, and docs-tech/ is never published, so it is out of scope for a rewrite
 * that is about the site a stranger reads.
 *
 *   npm run check:prose                                every page; exits 1 on a breach
 *   node scripts/prose-check.mjs docs/_docs/features   one subtree, listed
 *
 * What is deliberately NOT prose: front matter, fenced code, table rows, lines
 * of raw HTML, Liquid lines, and inline code — which counts as one word. Those
 * carry the numbers, keys, paths and flags the rewrite preserves byte for byte,
 * and measuring them reports a nine-row limits table as one enormous sentence.
 *
 * docs/_docs/changelog.md is excluded by exact path, not by pattern. It is
 * rendered from CHANGELOG.md by scripts/render-changelog.mjs and its bullets are
 * a historical record that this branch does not rewrite — roughly 214 of them
 * run past 30 words and 184 past 40, so a walk that included it could never
 * pass no matter how well the rest of the site is rewritten. A pattern-based
 * exclusion would risk silently swallowing some future page that genuinely
 * belongs in the count; an exact path cannot.
 *
 * The two rules are the spec's: no prose sentence over 30 words, average under
 * 18 (over-40 is also reported, for context, but does not gate). Neither rule
 * can see whether a rewritten sentence still claims what the code does.
 * Nothing can; that is what docs-tech/i18n-review.md is for.
 *
 * A trailing advisory section lists over-30-word table cells and alt="" image
 * descriptions — real prose this corpus definition excludes from the gated
 * count on purpose, but worth seeing. It never affects exit status.
 */
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';

const MAX = 30;
const AVG = 18;
const ROOT = new URL('..', import.meta.url).pathname;
const CHANGELOG = resolve(ROOT, 'docs/_docs/changelog.md');

function walk(p, out = []) {
  // Targets arrive both absolute (the default docs/_docs walk) and relative
  // (a path typed on the command line, or passed straight through by a task
  // that runs this per batch) — resolve() before comparing, or the exclusion
  // above only ever fires for the one form it happened to be spelled in.
  if (statSync(p).isDirectory()) {
    for (const e of readdirSync(p).sort()) walk(join(p, e), out);
  } else if (p.endsWith('.md') && resolve(p) !== CHANGELOG) {
    out.push(p);
  }
  return out;
}

// Inline code is one word whatever it holds: `EASYWALL_WEB_TRUSTED_PROXIES` is
// one thing a reader takes in, not three, and a rule that punished naming a key
// would be a rule against being specific.
const clean = t => t
  .replace(/`[^`]*`/g, 'CODE')
  .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
  .replace(/\{\{[^}]*\}\}/g, 'LINK')
  .replace(/^[-*+]\s+|^\d+\.\s+/, '');

const wordCount = t => t.split(/\s+/).filter(Boolean).length;

// Paragraphs, with the line each one starts on — a rewrite needs somewhere to
// go, and a sentence has no line of its own once it wraps. `adv` collects the
// two things this corpus definition deliberately does not gate on (table
// cells, alt="" text) so they can be reported without moving the target.
function paragraphs(md, adv = { cells: [], alts: [] }) {
  const lines = md.split('\n');
  const out = [];
  let buf = [], start = 0, fenced = false, front = false, liquid = false;

  const flush = () => {
    if (buf.length) out.push({ line: start, text: buf.join(' ') });
    buf = [];
  };

  lines.forEach((raw, i) => {
    // A blockquote marker is not a word: without stripping it, a wrapped
    // callout like "> event, which htmx does not emit" counts its `>` as one
    // of the sentence's words, and a blockquoted table or HTML line would
    // dodge the checks below that key on the first character.
    const line = raw.trim().replace(/^(>\s?)+/, '');

    if (i === 0 && line === '---') { front = true; return; }
    if (front) { if (line === '---') front = false; return; }
    if (/^```/.test(line)) { fenced = !fenced; flush(); return; }
    if (fenced) return;

    // A Liquid tag that wraps onto more than one line — every {% include
    // themed-figure.html %} in this corpus does, with its `alt="..."` sentence
    // on a continuation line that itself starts with neither `{%` nor `<`.
    // Without tracking the open tag, that continuation line reads as prose.
    // The alt text itself is advisory, not gated: a screen-reader user meets
    // it the way a sighted reader meets a caption, but the rewrite's target
    // is prose word counts, not this one.
    if (liquid) {
      const m = line.match(/alt="([^"]*)"/);
      if (m) {
        const text = clean(m[1]);
        const n = wordCount(text);
        if (n > MAX) adv.alts.push({ line: i + 1, n, text });
      }
      if (line.includes('%}')) liquid = false;
      return;
    }
    if (/^\{%/.test(line) && !line.includes('%}')) { liquid = true; flush(); return; }

    // A raw-HTML line: `<figure class="docs-shot">` and `</figure>` carry no
    // words of their own and are structure. `<figcaption>...</figcaption>`
    // does — a caption is a sentence a reader sees under a screenshot, just
    // wrapped in tags with nothing else on the line — so strip the tags and
    // measure what is left instead of losing every caption in the corpus.
    if (/^</.test(line)) {
      const text = line.replace(/<[^>]*>/g, '').trim();
      if (!text) { flush(); return; }
      if (!buf.length) start = i + 1;
      buf.push(text);
      return;
    }

    // A table row is structure, not a gated sentence — but its cells are
    // advisory, same reasoning as alt text: two paragraphs in two cells is a
    // paragraph you scroll sideways on a phone, and that is worth seeing even
    // though this checker does not fail on it.
    if (/^\|/.test(line)) {
      for (const cell of line.split('|').slice(1, -1)) {
        const text = clean(cell.trim());
        const n = wordCount(text);
        if (n > MAX) adv.cells.push({ line: i + 1, n, text });
      }
    }

    // A table row, a Liquid line, a heading, a horizontal rule: structure,
    // not prose.
    if (!line || /^\|/.test(line) || /^\{%/.test(line) ||
        /^#{1,6}\s/.test(line) || /^-{3,}$/.test(line)) {
      flush();
      return;
    }

    // A list item is prose and is measured, but each item is its own unit —
    // three bullets are not one sixty-word sentence.
    if (/^[-*+]\s|^\d+\.\s/.test(line)) flush();

    if (!buf.length) start = i + 1;
    buf.push(line);
  });

  flush();
  return out;
}

// An abbreviation does not end a sentence. `e.g.` inside a parenthesis is the
// one that would otherwise split every list of examples in two. The trailing
// period is dropped rather than restored: this string is only ever counted and
// printed.
//
// `No` and `etc` were on this list and are deliberately not, on identical
// evidence: matched case-insensitively, `No` swallows a sentence-final "no."
// and `etc` would swallow one ending in "etc." — both real words this
// corpus's prose can end a sentence with, and neither abbreviation ("No. 5",
// "and so on, etc.," continuing the same sentence) occurs here today. Same
// zero-occurrence argument that removed `No`, so the same conclusion for
// `etc`. `e.g`, `i.e`, `vs`, `Dr`, `Mr` stay: none of those is ever
// sentence-final in practice — they introduce what follows, or (`Dr`/`Mr`)
// precede the name that follows them.
const split = t => t
  .replace(/\b(e\.g|i\.e|vs|Dr|Mr)\./gi, '$1')
  // A bold or italic run can close right where a sentence ends — "**Fixed in
  // v2.4.0.** htmx was..." — so `*` closes the trailing-marker class the same
  // as a quote or bracket, and opens the next one the same as a capital
  // letter would. A sentence can also open on a digit ("2.13 also takes...")
  // or on `easywall` — the product's name is lowercase by design, not a
  // typo a capital-letter rule should be forgiving of.
  .split(/(?<=[.!?])["')\]*]*\s+(?=[A-Z0-9(`"'—*]|easywall\b)/)
  .map(s => s.trim())
  .filter(Boolean);

const targets = process.argv.slice(2).filter(a => !a.startsWith('--'));
const files = (targets.length ? targets : [join(ROOT, 'docs/_docs')])
  .flatMap(t => walk(t));

let allWords = [], breaches = 0;
let advCells = [], advAlts = [];

for (const file of files) {
  const rows = [];
  const adv = { cells: [], alts: [] };
  for (const p of paragraphs(readFileSync(file, 'utf8'), adv)) {
    for (const s of split(clean(p.text))) {
      const n = wordCount(s);
      allWords.push(n);
      if (n > MAX) rows.push({ line: p.line, n, s });
    }
  }
  if (rows.length) {
    breaches += rows.length;
    console.log(`\n${relative(ROOT, file)}`);
    for (const r of rows) {
      console.log(`  :${r.line}  ${r.n} words  ${r.s.slice(0, 96)}${r.s.length > 96 ? '...' : ''}`);
    }
  }
  for (const c of adv.cells) advCells.push({ file, ...c });
  for (const a of adv.alts) advAlts.push({ file, ...a });
}

const avg = allWords.length
  ? allWords.reduce((a, b) => a + b, 0) / allWords.length : 0;
const over40 = allWords.filter(n => n > 40).length;

console.log(`\n${files.length} pages · ${allWords.length} prose sentences · ` +
            `average ${avg.toFixed(1)} words · ${breaches} over ${MAX} · ${over40} over 40`);

// Advisory only: table cells and alt="" text are not gated sentences and do
// not affect exit status, but both carry real prose Phase 3 should be able
// to see — a table is for parallel short facts, and a long cell is a
// paragraph you scroll sideways on a phone.
if (advCells.length || advAlts.length) {
  console.log(`\n--- advisory (not gated, does not affect exit status) ---`);
  if (advCells.length) {
    console.log(`\n${advCells.length} table cell(s) over ${MAX} words:`);
    for (const c of advCells) {
      console.log(`  ${relative(ROOT, c.file)}:${c.line}  ${c.n} words  ${c.text.slice(0, 96)}${c.text.length > 96 ? '...' : ''}`);
    }
  }
  if (advAlts.length) {
    console.log(`\n${advAlts.length} alt="" description(s) over ${MAX} words:`);
    for (const a of advAlts) {
      console.log(`  ${relative(ROOT, a.file)}:${a.line}  ${a.n} words  ${a.text.slice(0, 96)}${a.text.length > 96 ? '...' : ''}`);
    }
  }
}

if (breaches || avg >= AVG) {
  if (avg >= AVG) console.error(`average is ${avg.toFixed(1)}, want under ${AVG}`);
  process.exit(1);
}
