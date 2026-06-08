-- Seed company identity for the grounded comment generator (PR-3, Omnichannel
-- Sales Copilot track). These four keys populate ai.BusinessProfile.{Name,
-- Website, OfficialContact, PrimaryCTA}, which ai.ResolveCompanyIdentity projects
-- into models.CompanyIdentity. The comment generator uses them for brand trust;
-- the contact-policy guard (ai.ScreenCommentContacts) rejects any website / email
-- / phone NOT grounded here — so a wrong/empty value means "no contact cited",
-- never a fabricated one.
--
-- 1. Replace <ORG_ID> with the org id (run: SELECT id,name FROM orgs;).
-- 2. FILL IN the website + official contact (leave a value EMPTY to cite none —
--    the generator will fall back to a brand-name + inbox CTA with no URL/contact).
-- 3. Run on the SQLite prod DB (data/scraper.db) with write access.
--
-- Idempotent upsert (user_context.key is unique).
INSERT INTO user_context (key, value, updated_at) VALUES
  ('org:<ORG_ID>:business_name',    'THG Fulfill', CURRENT_TIMESTAMP),
  ('org:<ORG_ID>:business_website', 'https://FILL-IN-OFFICIAL-WEBSITE', CURRENT_TIMESTAMP),
  ('org:<ORG_ID>:official_contact', 'FILL-IN (Telegram/Zalo/email, e.g. t.me/thgfulfill)', CURRENT_TIMESTAMP),
  ('org:<ORG_ID>:primary_cta',      'inbox để khảo sát sản phẩm và gửi phương án fulfillment/sourcing phù hợp', CURRENT_TIMESTAMP)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP;

-- Verify:
-- SELECT key, value FROM user_context WHERE key LIKE 'org:<ORG_ID>:%'
--   AND key IN ('org:<ORG_ID>:business_name','org:<ORG_ID>:business_website',
--               'org:<ORG_ID>:official_contact','org:<ORG_ID>:primary_cta');
