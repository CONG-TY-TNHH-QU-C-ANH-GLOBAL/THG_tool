import { readFile, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { resolve, join } from 'node:path';

// Read only the registered HTML snapshots, never credentials or live data.
const argument = process.argv.indexOf('--training-root');
if (argument < 0 || !process.argv[argument + 1]) throw new Error('--training-root is required');
const trainingRoot = resolve(process.argv[argument + 1]);
const packRoot = new URL('../../docs/business/company-knowledge/', import.meta.url);
const registry = JSON.parse(await readFile(new URL('sources.json', packRoot), 'utf8'));
const sha = data => createHash('sha256').update(data).digest('hex');
const normalizedHash = data => sha(data.toString('utf8').replaceAll('\r\n', '\n'));
// Resolve a fixed system installation, never an executable from the caller's PATH.
const gitBinary = process.platform === 'win32' ? String.raw`C:\Program Files\Git\cmd\git.exe` : '/usr/bin/git';
const git = args => execFileSync(gitBinary, ['-C', trainingRoot, ...args], { maxBuffer: 8 * 1024 * 1024 });
const revision = git(['rev-parse', 'HEAD']).toString('utf8').trim();
const clean = text => text.replaceAll(/<[^<>]*>/g, ' ').replaceAll('&amp;', '&').replaceAll('&nbsp;', ' ').replaceAll('&quot;', '"').replaceAll('&#39;', "'");
const normalize = text => clean(text).normalize('NFD').replaceAll(/\p{M}/gu, '').replaceAll('đ', 'd').toLowerCase().replaceAll(/[^a-z0-9]+/g, ' ').trim();
const entries = [];
for (const source of registry.sources.filter(item => item.kind === 'legacy_training_snapshot')) {
  if (source.repository !== 'THG_Training' || source.revision !== revision) throw new Error(`Snapshot mismatch: ${source.id}`);
  for (const path of source.paths) {
    if (!/^[a-z0-9.-]+\.html$/i.test(path)) throw new Error(`Unexpected snapshot path: ${path}`);
    const bytes = await readFile(join(trainingRoot, path));
    const committed = git(['show', `${revision}:${path}`]);
    if (normalizedHash(bytes) !== normalizedHash(committed)) throw new Error(`Uncommitted source content: ${path}`);
    const headings = [...bytes.toString('utf8').matchAll(/<h[1-6]\b[^>]*>([\s\S]*?)<\/h[1-6]>/gi)].map(match => normalize(match[1]));
    const sections = source.sections_reviewed || [];
    for (const section of sections) {
      const needle = normalize(section);
      if (!headings.some(heading => heading.includes(needle))) throw new Error(`Section title not found: ${source.id}: ${section}`);
    }
    entries.push({ source_id: source.id, path, working_file_sha256: sha(bytes), lf_normalized_sha256: normalizedHash(bytes), reviewed_sections: sections,
      scope: sections.length ? 'Selected section titles found; content interpretation remains a manual review.' : 'Selected reading described in sources.json; no exact section inventory recorded.' });
  }
}
const report = { schema_version: 1, checked_on: registry.checked_on, repository: 'THG_Training', revision,
  scope: 'Local snapshot provenance only; no live publication, business approval or full-document review implied.',
  files: entries };
await writeFile(new URL('TRAINING_SOURCE_AUDIT.json', packRoot), JSON.stringify(report, null, 2) + '\n', 'utf8');
console.log(JSON.stringify({ sources_checked: entries.length, section_titles_checked: entries.reduce((sum, item) => sum + item.reviewed_sections.length, 0), revision }));
