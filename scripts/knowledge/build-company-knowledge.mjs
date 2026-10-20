import { readFile, mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const root = new URL('../../docs/business/company-knowledge/', import.meta.url);
const readJSON = async name => JSON.parse(await readFile(new URL(name, root), 'utf8'));
const [pack, registry, questions, examples] = await Promise.all(['knowledge.json', 'sources.json', 'open-questions.json', 'response-examples.json'].map(readJSON));
const fail = message => { throw new Error(message); };
const unique = (items, name) => {
  const ids = items.map(item => item.id);
  if (new Set(ids).size !== ids.length) fail(`Duplicate ${name} id`);
  if (ids.some(id => !/^[a-z][a-z0-9-]*$/.test(id))) fail(`Invalid ${name} id`);
  return new Set(ids);
};
const date = value => /^\d{4}-\d{2}-\d{2}$/.test(value) && !Number.isNaN(Date.parse(value));
const sourceIDs = unique(registry.sources, 'source');
const topicIDs = unique(pack.topics, 'topic');
const recordIDs = unique(pack.records, 'record');
unique(questions.items, 'question');
unique(examples.cases, 'example');
const types = new Set(['faq', 'shipping_policy', 'sales_playbook', 'pricing_rule', 'banned_claim']);
const evidence = new Set(['public_source_checked', 'task_owner_confirmed', 'implementation_checked', 'legacy_source_checked', 'proposed_guidance', 'proposed_example', 'needs_confirmation']);
const bySource = new Map(registry.sources.map(source => [source.id, source]));
const references = (item, label) => {
  if (!Array.isArray(item.source_ids) || !item.source_ids.length || item.source_ids.some(id => !sourceIDs.has(id))) fail(`Invalid sources for ${label}`);
};
if ([pack, registry, questions, examples].some(item => item.schema_version !== 1)) fail('Unsupported schema version');
if (examples.status !== 'proposed' || !date(examples.updated_on)) fail('Examples must remain proposed');
for (const item of examples.cases) {
  for (const key of ['input_text', 'expected_intent', 'why_this_works']) if (!item[key]?.trim()) fail(`Incomplete example ${item.id}`);
  for (const key of ['known_facts', 'ask_only_if_missing', 'avoid', 'knowledge_ids']) if (!Array.isArray(item[key]) || item[key].some(text => typeof text !== 'string' || !text.trim())) fail(`Invalid ${key} in ${item.id}`);
  if (!item.knowledge_ids.length || item.knowledge_ids.some(id => !recordIDs.has(id))) fail(`Missing knowledge in ${item.id}`);
  if (!(item.draft_reply === null || typeof item.draft_reply === 'string' && item.draft_reply.trim())) fail(`Invalid draft ${item.id}`);
}
if (pack.review_state !== 'pending' || pack.runtime_state !== 'not_loaded') fail('This draft builder does not publish or approve knowledge');
if (!date(pack.updated_on) || !date(registry.checked_on) || !date(questions.updated_on)) fail('Invalid pack date');
for (const source of registry.sources) {
  if (!date(source.checked_on) || !source.title || !source.coverage) fail(`Incomplete source ${source.id}`);
  if (source.url) {
    const url = new URL(source.url);
    if (url.protocol !== 'https:' || url.username || url.password || !(url.hostname === 'thgfulfill.com' || url.hostname.endsWith('.thgfulfill.com'))) fail(`Non-company public source ${source.id}`);
  }
  if (['repository_snapshot', 'legacy_training_snapshot'].includes(source.kind) && (!source.revision || !source.paths?.length)) fail(`Missing revision for ${source.id}`);
  if (source.sections_reviewed && (!Array.isArray(source.sections_reviewed) || source.sections_reviewed.some(text => typeof text !== 'string' || !text.trim()))) fail(`Invalid reviewed sections ${source.id}`);
}
for (const record of pack.records) {
  references(record, record.id);
  if (!topicIDs.has(record.topic) || !types.has(record.asset_type) || !evidence.has(record.verification)) fail(`Invalid record taxonomy ${record.id}`);
  if (!['public_reference', 'sales_internal'].includes(record.audience)) fail(`Invalid audience ${record.id}`);
  for (const key of ['facts', 'guidance', 'ask_when_missing', 'avoid']) {
    if (!Array.isArray(record[key]) || record[key].some(text => typeof text !== 'string' || !text.trim())) fail(`Invalid ${key} for ${record.id}`);
  }
  for (const key of ['applies_when', 'required_inputs', 'steps', 'outputs', 'search_terms']) {
    if (record[key] !== undefined && (!Array.isArray(record[key]) || record[key].some(text => typeof text !== 'string' || !text.trim()))) fail(`Invalid ${key} for ${record.id}`);
  }
  if (record.related_ids && (!Array.isArray(record.related_ids) || new Set(record.related_ids).size !== record.related_ids.length || record.related_ids.some(id => id === record.id || !recordIDs.has(id)))) fail(`Invalid related records ${record.id}`);
  if (!record.title || !record.guidance.length || !record.avoid.length) fail(`Incomplete guidance ${record.id}`);
  if (record.verification === 'public_source_checked' && !record.source_ids.some(id => bySource.get(id).kind === 'official_public_page')) fail(`No checked public source for ${record.id}`);
  if (record.verification === 'task_owner_confirmed' && !record.source_ids.includes('owner-task')) fail(`No owner decision for ${record.id}`);
  if (record.verification === 'implementation_checked' && !record.source_ids.some(id => bySource.get(id).kind === 'repository_snapshot')) fail(`No implementation source for ${record.id}`);
  if (record.verification === 'legacy_source_checked' && (record.audience !== 'sales_internal' || !record.source_ids.some(id => bySource.get(id).kind === 'legacy_training_snapshot'))) fail(`Unscoped legacy snapshot ${record.id}`);
  if (['proposed_guidance', 'proposed_example', 'needs_confirmation'].includes(record.verification) && record.facts.length) fail(`Unverified company facts in ${record.id}`);
  if (!['proposed_guidance', 'proposed_example', 'needs_confirmation'].includes(record.verification) && !record.facts.length) fail(`Missing factual basis for ${record.id}`);
}
for (const topic of pack.topics) if (!pack.records.some(record => record.topic === topic.id)) fail(`Empty topic ${topic.id}`);
for (const question of questions.items) {
  references(question, question.id);
  for (const key of ['owner_role', 'question', 'reason', 'until_confirmed']) if (!question[key]) fail(`Incomplete question ${question.id}`);
  if (!['before_operational_claims', 'content_completion'].includes(question.priority)) fail(`Invalid priority ${question.id}`);
  if (question.evidence_notes && (!Array.isArray(question.evidence_notes) || question.evidence_notes.some(text => typeof text !== 'string' || !text.trim()))) fail(`Invalid evidence notes ${question.id}`);
  if (question.affected_record_ids && (!Array.isArray(question.affected_record_ids) || question.affected_record_ids.some(id => !recordIDs.has(id)))) fail(`Invalid affected records ${question.id}`);
  if (question.handling_state !== undefined || question.interim_guidance_ids !== undefined) {
    if (question.handling_state !== 'interim_guidance_defined' || !Array.isArray(question.interim_guidance_ids) || !question.interim_guidance_ids.length || question.interim_guidance_ids.some(id => !pack.records.some(record => record.id === id && record.verification === 'proposed_guidance' && record.audience === 'sales_internal'))) fail(`Invalid interim handling ${question.id}`);
  }
}

const labels = {
  public_source_checked: 'Đã đối chiếu nguồn công khai; chưa phải duyệt cam kết cho mọi đơn',
  task_owner_confirmed: 'Người dùng/leader đã chốt trong phạm vi task',
  implementation_checked: 'Đã đối chiếu code; không xác nhận năng lực kinh doanh hay production',
  legacy_source_checked: 'Đã đọc snapshot Training trong repo; chưa xác nhận chính sách hiện hành',
  proposed_guidance: 'Hướng dẫn đề xuất, chờ owner duyệt',
  proposed_example: 'Tình huống tổng hợp, chưa được duyệt làm câu trả lời chuẩn',
  needs_confirmation: 'Chưa đủ căn cứ xác nhận điều khoản'
};
const metadata = title => `---\ndoc_type: business\nstatus: draft\nowner: platform\nlast_reviewed: ${pack.updated_on}\nrelated_pr_or_issue: Add Taobao sourcing to replies\n---\n\n# ${title}\n\n> Tài liệu sinh từ JSON cùng thư mục bằng scripts/knowledge/build-company-knowledge.mjs. Sửa nguồn JSON rồi sinh lại.\n\n`;
const sourceLink = id => {
  const source = bySource.get(id);
  return source.url ? `[${source.title}](${source.url})` : `${source.title} (${id}${source.revision ? `, ${source.repository}@${source.revision}` : ''})`;
};
const bullets = (label, lines) => lines.length ? `**${label}**\n\n${lines.map(text => `- ${text}`).join('\n')}\n\n` : '';
const questionInterim = question => question.handling_state === 'interim_guidance_defined' ?
  `**Cách tiếp tục:** Đã có hướng dẫn tạm thời theo ủy quyền của người dùng; việc biên soạn và kiểm thử tiếp tục. Điều khoản kinh doanh vẫn chưa được xác nhận; trạng thái này không duyệt asset hay bật production.\n\n${bullets('Hướng dẫn tạm thời', question.interim_guidance_ids.map(id => { const record = pack.records.find(entry => entry.id === id); return `[${record.title}](topics/${record.topic}.md#${id})`; }))}` : '';
const renderRecord = record => `## ${record.title}\n\nMã: \`${record.id}\` · Loại: \`${record.asset_type}\` · ${record.audience === 'sales_internal' ? 'Nội bộ sale' : 'Tham chiếu công khai'}\n\nTrạng thái: ${labels[record.verification]}. Mọi mục vẫn chờ duyệt trước khi nạp runtime.\n\n${bullets('Từ khóa tra cứu; không mở rộng phạm vi dịch vụ', record.search_terms || [])}${bullets('Dữ kiện có nguồn', record.facts)}${bullets('Áp dụng khi', record.applies_when || [])}${bullets('Đầu vào cần cho nghiệp vụ; không phải hỏi tất cả ngay', record.required_inputs || [])}${bullets('Cách tư vấn / xử lý đề xuất', record.guidance)}${bullets('Các bước tham chiếu; cần xác nhận SOP hiện hành', record.steps || [])}${bullets('Đầu ra cần có', record.outputs || [])}${bullets('Chỉ hỏi khi thiếu hoặc cần xác nhận', record.ask_when_missing)}${bullets('Điều cần tránh', record.avoid)}${bullets('Mục liên quan', (record.related_ids || []).map(id => { const item = pack.records.find(entry => entry.id === id); return `[${item.title}](${item.topic}.md#${item.id})`; }))}Nguồn: ${record.source_ids.map(sourceLink).join('; ')}.\n\n`;
const outputs = new Map();
const counts = Object.fromEntries([...evidence].map(key => [key, pack.records.filter(record => record.verification === key).length]));
for (const topic of pack.topics) {
  const records = pack.records.filter(record => record.topic === topic.id);
  outputs.set(`topics/${topic.id}.md`, metadata(topic.title) + records.map(record => `<a id="${record.id}"></a>\n\n` + renderRecord(record)).join(''));
}
outputs.set('INDEX.md', metadata('Kho tri thức công ty THGFulfill — bản nền tảng') +
  `Bộ này có **${pack.records.length} mục / ${pack.topics.length} nhóm**, ${registry.sources.length} nguồn và ${questions.items.length} câu hỏi còn mở. Chưa nạp, duyệt hay kích hoạt trên production.\n\n` +
  `Đọc [hướng dẫn sử dụng](README.md), rồi chọn nhóm cần thiết. Trạng thái đối chiếu nguồn khác trạng thái được phép sử dụng. Các hướng dẫn và ví dụ đề xuất chưa phải SOP được công ty duyệt.\n\n` +
  pack.topics.map(topic => `- [${topic.title}](topics/${topic.id}.md) — ${pack.records.filter(record => record.topic === topic.id).length} mục`).join('\n') +
  `\n\n## Mức độ xác nhận\n\n` + [...evidence].map(key => `- ${labels[key]}: ${counts[key]} mục.`).join('\n') +
  `\n\nXem [phạm vi đã có và còn thiếu](COVERAGE.md), [cách viết lời đáp](REPLY_STYLE.md), [${examples.cases.length} ví dụ đề xuất](REPLY_EXAMPLES.md), [nguồn và giới hạn](SOURCES.md), [thông tin cần xác nhận](OPEN_QUESTIONS.md), [cách đưa vào hệ thống](INTEGRATION.md).\n`);
outputs.set('SOURCES.md', metadata('Nguồn, thời điểm đối chiếu và giới hạn') + registry.sources.map(source =>
  `## ${source.id} — ${source.title}\n\nLoại: \`${source.kind}\` · Ngày đối chiếu: ${source.checked_on}.\n\n${source.url ? `[Mở nguồn](${source.url})` : `Repo/trao đổi: ${source.repository || source.kind}${source.revision ? ` @ ${source.revision}` : ''}`}\n\n${source.coverage}\n\n${source.paths ? source.paths.map(path => `- \`${path}\``).join('\n') + '\n\n' : ''}${bullets('Các phần đã đọc chọn lọc', source.sections_reviewed || [])}`).join(''));
outputs.set('REPLY_EXAMPLES.md', metadata('Câu trả lời mẫu để góp ý và đánh giá') +
  'Các đầu vào dưới đây là tình huống tổng hợp, không phải lead production hoặc câu trả lời đã được công ty duyệt. Mô tả ảnh là giả định cho bài kiểm tra, không chứng minh hệ thống đã đọc ảnh. Lời đáp chỉ là một cách viết phù hợp; đánh giá ý nghĩa và căn cứ, không so khớp từng chữ.\n\n' +
  examples.cases.map(item => `## ${item.id}\n\n**Bài đăng giả định:** ${item.input_text}\n\n${item.image_description ? `**Ảnh giả định:** ${item.image_description}\n\n` : ''}**Nhu cầu cần hiểu:** ${item.expected_intent}\n\n${bullets('Đã biết từ đầu vào', item.known_facts)}${bullets('Chỉ hỏi phần còn thiếu cần cho bước tiếp', item.ask_only_if_missing)}**Lời đáp đề xuất:** ${item.draft_reply ?? 'Không tạo lời chào dịch vụ/sourcing tự động; bài ngoài phạm vi nhu cầu đang xử lý.'}\n\n**Vì sao:** ${item.why_this_works}\n\n${bullets('Tránh', item.avoid)}Tri thức tham chiếu: ${item.knowledge_ids.map(id => { const record = pack.records.find(entry => entry.id === id); return `[${record.title}](topics/${record.topic}.md#${id})`; }).join('; ')}.\n\n`).join(''));
outputs.set('OPEN_QUESTIONS.md', metadata('Thông tin cần bổ sung hoặc xác nhận') +
  'Có thể tiếp tục biên soạn và kiểm thử với dữ liệu đã có. Các mục dưới đây chỉ chặn phát ngôn/cam kết tương ứng, không chặn toàn bộ công việc. Owner là vai trò đề xuất, chưa phân công một cá nhân.\n\n' +
  questions.items.map(item => `## ${item.id}\n\n${questionInterim(item)}**Ưu tiên:** ${item.priority === 'before_operational_claims' ? 'Xác nhận trước khi dùng cho cam kết vận hành tương ứng' : 'Bổ sung để hoàn thiện nội dung'}.\n\n**Cần xác nhận:** ${item.question}\n\n**Đầu mối phù hợp:** ${item.owner_role}.\n\n**Vì sao:** ${item.reason}\n\n${bullets('Bằng chứng và điểm chưa thống nhất', item.evidence_notes || [])}${bullets('Mục bị ảnh hưởng', (item.affected_record_ids || []).map(id => { const record = pack.records.find(entry => entry.id === id); return `[${record.title}](topics/${record.topic}.md#${id})`; }))}**Trong lúc chưa có:** ${item.until_confirmed}\n\nNguồn: ${item.source_ids.map(sourceLink).join('; ')}.\n\n`).join(''));

const check = process.argv.includes('--check');
for (const [name, rendered] of outputs) {
  const content = rendered.trimEnd() + '\n';
  const target = new URL(name, root);
  if (check) {
    let existing;
    try { existing = await readFile(target, 'utf8'); } catch { fail(`Missing generated file ${name}`); }
    if (existing !== content) fail(`Stale generated file ${name}`);
  } else {
    await mkdir(new URL('./', target), { recursive: true });
    await writeFile(target, content, 'utf8');
  }
}
console.log(JSON.stringify({ mode: check ? 'checked' : 'generated', root: fileURLToPath(root), records: pack.records.length, topics: pack.topics.length, sources: registry.sources.length, examples: examples.cases.length, open_questions: questions.items.length, verification: counts, runtime: pack.runtime_state }, null, 2));
