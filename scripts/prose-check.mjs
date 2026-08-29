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
 * 18. Neither can see whether a rewritten sentence still claims what the code
 * does. Nothing can; that is what docs-tech/i18n-review.md is for.
 */
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const MAX = 30;
const AVG = 18;
const ROOT = new URL('..', import.meta.url).pathname;
const CHANGELOG = join(ROOT, 'docs/_docs/changelog.md');

function walk(p, out = []) {
  if (statSync(p).isDirectory()) {
    for (const e of readdirSync(p).sort()) walk(join(p, e), out);
  } else if (p.endsWith('.md') && p !== CHANGELOG) {
    out.push(p);
  }
  return out;
}

// Paragraphs, with the line each one starts on — a rewrite needs somewhere to
// go, and a sentence has no line of its own once it wraps.
function paragraphs(md) {
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
    if (liquid) { if (line.includes('%}')) liquid = false; return; }
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

// Inline code is one word whatever it holds: `EASYWALL_WEB_TRUSTED_PROXIES` is
// one thing a reader takes in, not three, and a rule that punished naming a key
// would be a rule against being specific.
const clean = t => t
  .replace(/`[^`]*`/g, 'CODE')
  .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
  .replace(/\{\{[^}]*\}\}/g, 'LINK')
  .replace(/^[-*+]\s+|^\d+\.\s+/, '');

// An abbreviation does not end a sentence. `e.g.` inside a parenthesis is the
// one that would otherwise split every list of examples in two. The trailing
// period is dropped rather than restored: this string is only ever counted and
// printed.
//
// `No` was on this list and is deliberately not: matched case-insensitively it
// swallows a sentence-final "no." — a real word this corpus's prose uses, and
// one the rewrite is likely to use more, not less. The abbreviation "No. 5"
// that the entry was protecting against does not occur here. The rest of the
// list was checked for the same shape of mistake (a common freestanding word
// masquerading as an abbreviation): none of `e.g`, `i.e`, `etc`, `vs`, `Dr`,
// `Mr` occur anywhere in the current corpus, so none has caused a wrong split
// yet — but see the report for `etc`, which is a closer call.
const split = t => t
  .replace(/\b(e\.g|i\.e|etc|vs|Dr|Mr)\./gi, '$1')
  .split(/(?<=[.!?])["')\]]*\s+(?=[A-Z(`"'—])/)
  .map(s => s.trim())
  .filter(Boolean);

const targets = process.argv.slice(2).filter(a => !a.startsWith('--'));
const files = (targets.length ? targets : [join(ROOT, 'docs/_docs')])
  .flatMap(t => walk(t));

let allWords = [], breaches = 0;

for (const file of files) {
  const rows = [];
  for (const p of paragraphs(readFileSync(file, 'utf8'))) {
    for (const s of split(clean(p.text))) {
      const n = s.split(/\s+/).filter(Boolean).length;
      allWords.push(n);
      if (n > MAX) rows.push({ line: p.line, n, s });
    }
  }
  if (!rows.length) continue;
  breaches += rows.length;
  console.log(`\n${relative(ROOT, file)}`);
  for (const r of rows) {
    console.log(`  :${r.line}  ${r.n} words  ${r.s.slice(0, 96)}${r.s.length > 96 ? '...' : ''}`);
  }
}

const avg = allWords.length
  ? allWords.reduce((a, b) => a + b, 0) / allWords.length : 0;

console.log(`\n${files.length} pages · ${allWords.length} prose sentences · ` +
            `average ${avg.toFixed(1)} words · ${breaches} over ${MAX}`);

if (breaches || avg >= AVG) {
  if (avg >= AVG) console.error(`average is ${avg.toFixed(1)}, want under ${AVG}`);
  process.exit(1);
}
