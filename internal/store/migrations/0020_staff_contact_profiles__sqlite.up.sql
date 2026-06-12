-- Staff/sales contact profiles (SaaS UX Hardening PR-5).
-- Company identity (org:{id}:* keys) stays the brand/service truth;
-- the CONTACT line in generated comments comes from the assigned
-- salesperson's own profile, falling back to the workspace default
-- contact only when allowed (org:{id}:allow_company_contact_fallback).
CREATE TABLE IF NOT EXISTS staff_contact_profiles (
  user_id INTEGER PRIMARY KEY,
  org_id INTEGER NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  role_title TEXT NOT NULL DEFAULT '',
  telegram TEXT NOT NULL DEFAULT '',
  zalo TEXT NOT NULL DEFAULT '',
  phone TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL DEFAULT '',
  preferred_cta TEXT NOT NULL DEFAULT '',
  signature_text TEXT NOT NULL DEFAULT '',
  visibility TEXT NOT NULL DEFAULT 'team',
  active INTEGER NOT NULL DEFAULT 1,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_staff_contact_org ON staff_contact_profiles(org_id);
