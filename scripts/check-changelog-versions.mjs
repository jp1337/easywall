#!/usr/bin/env node
/**
 * A second opinion on which versions CHANGELOG.md contains.
 *
 * render-changelog.mjs is both the renderer and the checker: one regex decides
 * what gets written and whether what was written is right, so a heading
 * neither of them parses is invisible to both. Measured at b723422 —
 * CHANGELOG.md:108 read "## [2.15.1] —## [2.15.1] — 2026-09-07", the page
 * carried 32 <details> sections rather than 34, and check:changelog reported
 * it current. 2.15.1 had no section and no compare link on the site.
 *
 * This deliberately shares no code with the renderer. Three sources are
 * compared, and they are independent of each other:
 *
 *   1. every version that appears in a `## [x.y.z]` heading, found by counting
 *      bracketed tokens rather than by matching the renderer's line shape
 *   2. every version with a link definition at the bottom of the file
 *   3. every <details> section in the generated page
 *
 * Any disagreement is a version that is in the file and not on the site, or on
 * the site twice, or missing a compare link. Importing the renderer's HEADER
 * regex would put this back to one source of truth, which is the defect.
 */
import { readFileSync } from 'node:fs';

const changelog = readFileSync(new URL('../CHANGELOG.md', import.meta.url), 'utf8');
const page = readFileSync(new URL('../docs/_docs/changelog.md', import.meta.url), 'utf8');

const fail = [];

// 1. Versions in headings. A heading line is any line starting "## ", and every
//    bracketed token on it is a version claim — which is what catches a line
//    holding two of them.
const headingVersions = [];
for (const line of changelog.split('\n')) {
  if (!line.startsWith('## ')) continue;
  const tokens = [...line.matchAll(/\[([^\]]+)\]/g)].map(m => m[1]);
  if (tokens.length === 0) {
    fail.push(`a "## " heading carries no [version]: ${line.trim()}`);
    continue;
  }
  if (tokens.length > 1) {
    fail.push(`one heading line carries ${tokens.length} versions (${tokens.join(', ')}) — ` +
      `two headings ran together, and the renderer's regex reads only the first: ${line.trim()}`);
  }
  headingVersions.push(...tokens);
}

// 2. Link definitions.
const linked = new Set(
  [...changelog.matchAll(/^\[([^\]]+)\]:\s*https?:\S+/gm)].map(m => m[1]),
);

// 3. Sections on the generated page.
//
// The renderer's own section marker, not every <details>: CHANGELOG.md
// contains one <details> in prose that the renderer passes through, so
// counting the tag gives 36 against 35 headings on an unmodified tree.
// Measured 2026-09-11: `grep -c '<details' docs/_docs/changelog.md` = 36,
// `grep -c '^## \[' CHANGELOG.md` = 35, `grep -c '<details' CHANGELOG.md` = 1.
// Confirmed against render-changelog.mjs's own output: `<summary>` is opened
// by the renderer and immediately followed by `<strong>` from summary() — the
// two concatenate with no separator, so `<summary><strong>` is the literal
// string the renderer emits once per section, never as part of the one
// hand-written <details> in CHANGELOG.md's prose.
const sections = (page.match(/<summary><strong>/g) || []).length;

// CHANGELOG.md has no `## [Unreleased]` section on this tree (confirmed by
// grep before writing this), so the special case that would exempt it from
// needing a link definition is dropped rather than kept as a branch nothing
// reaches.
const seen = new Set();
for (const v of headingVersions) {
  if (seen.has(v)) fail.push(`version ${v} appears in two headings`);
  seen.add(v);
  if (!linked.has(v)) {
    fail.push(`version ${v} has a heading and no link definition — no compare link on the site`);
  }
}

if (sections !== headingVersions.length) {
  fail.push(`CHANGELOG.md has ${headingVersions.length} version headings and the generated page ` +
    `has ${sections} <details> sections. A heading the renderer did not parse is a version with ` +
    `no section of its own, which is what 2.15.1 got.`);
}

if (fail.length) {
  console.error('check:changelog-versions failed:');
  for (const f of fail) console.error(`  - ${f}`);
  console.error('\nThis check does not share a parser with render-changelog.mjs, deliberately.');
  process.exit(1);
}
console.log(`check:changelog-versions: ${headingVersions.length} versions, ` +
  `${sections} sections, ${linked.size} link definitions — agreed`);
