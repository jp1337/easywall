// Renders every diagram in docs/_diagrams/*.mmd to a committed SVG, one per theme.
//
// Why pre-render rather than ship mermaid.js: the runtime bundle is 3.5 MB to draw
// a handful of boxes, the docs site deliberately makes no third-party requests, and
// an SVG cannot shift the layout after paint. A diagram is a few kB and needs no
// JavaScript at all.
//
// Diagrams are named files rather than fenced blocks inside pages, so a page can
// reference one by a stable name, several pages can share one, and the source of a
// picture is reviewable on its own in a diff.
//
//   npm run build:diagrams     render everything
//   npm run check:diagrams     fail if a diagram is missing or out of date (CI)
//
// Reference one from Markdown with the snippet in docs/_diagrams/README.md.

import { createHash } from 'node:crypto';
import { readFile, writeFile, mkdir, readdir } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import path from 'node:path';
import { chromium } from 'playwright-core';

const ROOT = path.resolve(import.meta.dirname, '..');
const SRC_DIR = path.join(ROOT, 'docs', '_diagrams');
const OUT_DIR = path.join(ROOT, 'docs', 'assets', 'diagrams');
const MERMAID = path.join(ROOT, 'node_modules', 'mermaid', 'dist', 'mermaid.min.js');
const CHECK = process.argv.includes('--check');

// Both themes, because the docs site follows the reader's. Values are the tokens
// from DESIGN.md; keep them in step with web/src/docs.css.
const THEMES = {
  dark: {
    background: 'transparent',
    primaryColor: '#181b22',
    primaryTextColor: '#f1f3f6',
    primaryBorderColor: '#5e636e',
    lineColor: '#7d8593',
    secondaryColor: '#111318',
    tertiaryColor: '#0a0b0f',
    noteBkgColor: '#181b22',
    noteTextColor: '#a2aab8',
    noteBorderColor: '#252932',
    actorBkg: '#181b22',
    actorBorder: '#5e636e',
    actorTextColor: '#f1f3f6',
    signalColor: '#a2aab8',
    signalTextColor: '#a2aab8',
    labelBoxBkgColor: '#181b22',
    labelBoxBorderColor: '#5e636e',
    labelTextColor: '#f1f3f6',
    fontFamily: 'Inter, system-ui, sans-serif',
    fontSize: '17px',
  },
  light: {
    background: 'transparent',
    primaryColor: '#f4f6f9',
    primaryTextColor: '#0f1116',
    primaryBorderColor: '#868a93',
    lineColor: '#5a6270',
    secondaryColor: '#ffffff',
    tertiaryColor: '#fcfcfd',
    noteBkgColor: '#f4f6f9',
    noteTextColor: '#5a6270',
    noteBorderColor: '#e4e8ee',
    actorBkg: '#f4f6f9',
    actorBorder: '#868a93',
    actorTextColor: '#0f1116',
    signalColor: '#5a6270',
    signalTextColor: '#5a6270',
    labelBoxBkgColor: '#f4f6f9',
    labelBoxBorderColor: '#868a93',
    labelTextColor: '#0f1116',
    fontFamily: 'Inter, system-ui, sans-serif',
    fontSize: '17px',
  },
};

// The renderer is part of the input. mermaid 11.16.0 -> 11.16.1 moved the bezier
// control points on every rounded container, so four committed SVGs no longer
// matched what the pinned version produced — and --check, hashing the .mmd alone,
// reported them current. Fold the version in so an upgrade is a re-render.
const MERMAID_VERSION = JSON.parse(
  await readFile(new URL('../node_modules/mermaid/package.json', import.meta.url), 'utf8'),
).version;

const digest = (s) =>
  createHash('sha256').update(`${MERMAID_VERSION}\n${s.trim()}`).digest('hex').slice(0, 16);

// Written into the SVG so --check can tell "not rendered yet" from "rendered
// from an older version of the source, or by an older mermaid".
const STAMP = 'data-source-digest';

async function sources() {
  if (!existsSync(SRC_DIR)) return [];
  const names = (await readdir(SRC_DIR)).filter((n) => n.endsWith('.mmd')).sort();
  return Promise.all(
    names.map(async (name) => ({
      name: name.replace(/\.mmd$/, ''),
      source: await readFile(path.join(SRC_DIR, name), 'utf8'),
    })),
  );
}

async function check(diagrams) {
  const problems = [];
  for (const d of diagrams) {
    for (const theme of Object.keys(THEMES)) {
      const file = path.join(OUT_DIR, `${d.name}-${theme}.svg`);
      if (!existsSync(file)) {
        problems.push(`${d.name}-${theme}.svg is missing`);
        continue;
      }
      const svg = await readFile(file, 'utf8');
      const m = svg.match(new RegExp(`${STAMP}="([a-f0-9]+)"`));
      if (!m || m[1] !== digest(d.source)) {
        problems.push(
          `${d.name}-${theme}.svg is stale — ${d.name}.mmd changed, ` +
            `or it was rendered by a mermaid other than ${MERMAID_VERSION}`,
        );
      }
    }
  }
  if (problems.length) {
    console.error('Diagrams out of date — run: npm run build:diagrams\n');
    for (const p of problems) console.error('  ' + p);
    process.exit(1);
  }
  console.log(`${diagrams.length} diagram(s), all current`);
}

// Looks for a Chromium the machine already has, newest first.
async function findChromium() {
  if (process.env.PLAYWRIGHT_CHROMIUM) return process.env.PLAYWRIGHT_CHROMIUM;
  const cache = path.join(process.env.HOME || '', '.cache', 'ms-playwright');
  if (existsSync(cache)) {
    const dirs = (await readdir(cache)).filter((d) => d.startsWith('chromium-')).sort().reverse();
    for (const d of dirs) {
      for (const rel of [['chrome-linux64', 'chrome'], ['chrome-linux', 'chrome']]) {
        const exe = path.join(cache, d, ...rel);
        if (existsSync(exe)) return exe;
      }
    }
  }
  for (const exe of ['/usr/bin/chromium', '/usr/bin/chromium-browser', '/usr/bin/google-chrome']) {
    if (existsSync(exe)) return exe;
  }
  throw new Error('no Chromium found — set PLAYWRIGHT_CHROMIUM to one');
}

async function main() {
  const diagrams = await sources();
  if (diagrams.length === 0) {
    console.log(`no .mmd files in ${path.relative(ROOT, SRC_DIR)}`);
    return;
  }
  if (CHECK) return check(diagrams);

  await mkdir(OUT_DIR, { recursive: true });
  const lib = await readFile(MERMAID, 'utf8');

  // playwright-core rather than playwright: it ships no browser of its own, so a
  // contributor who already has Chromium does not download a second one.
  const browser = await chromium.launch({ executablePath: await findChromium() });
  const page = await browser.newPage();
  await page.setContent('<!doctype html><body></body>');
  await page.addScriptTag({ content: lib });

  for (const d of diagrams) {
    for (const [theme, vars] of Object.entries(THEMES)) {
      let svg = await page.evaluate(
        async ({ source, vars, id }) => {
          window.mermaid.initialize({
            startOnLoad: false,
            theme: 'base',
            themeVariables: vars,
            // Global, not just per-diagram-type. With HTML labels mermaid puts a
            // <foreignObject> full of HTML into the SVG, and an unclosed <br>
            // makes the file invalid XML. Inline in a page that is survivable; as
            // an <img> source it is not — SVG in an <img> is parsed strictly, so
            // one <br> silently kills the whole picture.
            htmlLabels: false,
            // useMaxWidth would emit width="100%", which leaves the SVG with no
            // intrinsic size. That is fine inline, but as an <img> source the
            // browser cannot size it: naturalWidth is 0 and nothing renders.
            // Concrete pixels here; the stylesheet scales it to the column.
            flowchart: { curve: 'basis', htmlLabels: false, useMaxWidth: false },
            sequence: { useMaxWidth: false, actorMargin: 60 },
            state: { useMaxWidth: false },
            securityLevel: 'strict',
          });
          const { svg } = await window.mermaid.render(id, source);
          return svg;
        },
        { source: d.source, vars, id: `d-${d.name}-${theme}` },
      );
      svg = svg.replace('<svg ', `<svg ${STAMP}="${digest(d.source)}" `);
      // An <img> needs an intrinsic aspect ratio to reserve space before the file
      // arrives. Mermaid gives width and height but not always both, so derive
      // whatever is missing from the viewBox.
      const vb = svg.match(/viewBox="([-\d.]+) ([-\d.]+) ([\d.]+) ([\d.]+)"/);
      if (vb) {
        const w = Math.ceil(parseFloat(vb[3]));
        const h = Math.ceil(parseFloat(vb[4]));
        svg = svg.replace(/<svg([^>]*)>/, (m0, attrs) => {
          const cleaned = attrs
            .replace(/\swidth="[^"]*"/, '')
            .replace(/\sheight="[^"]*"/, '')
            .replace(/\sstyle="[^"]*"/, '');
          return `<svg${cleaned} width="${w}" height="${h}">`;
        });
      }
      // An SVG used as an <img> source is parsed as strict XML. Anything that
      // does not parse fails silently — the browser reports naturalWidth 0 and
      // shows the alt text. Catch it here rather than in a screenshot.
      const bad = await page.evaluate((markup) => {
        const doc = new DOMParser().parseFromString(markup, 'image/svg+xml');
        const err = doc.querySelector('parsererror');
        return err ? err.textContent.slice(0, 200) : null;
      }, svg);
      if (bad) {
        throw new Error(`${d.name}-${theme}.svg is not valid XML and would not render:\n${bad}`);
      }

      await writeFile(path.join(OUT_DIR, `${d.name}-${theme}.svg`), svg, 'utf8');
    }
    console.log(`  ${d.name}`);
  }
  await browser.close();
  console.log(`rendered ${diagrams.length} diagram(s) x ${Object.keys(THEMES).length} themes`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
