-- Telegram webhook runtime (spec: specs/TELEGRAM_BOT_RUNTIME.md). The bot must send replies +
-- notifications to a bound user's private chat, so the binding needs the Telegram chat_id (for a
-- 1:1 chat this equals telegram_user_id, but storing it explicitly is forward-compatible).
ALTER TABLE telegram_bindings ADD COLUMN chat_id INTEGER NOT NULL DEFAULT 0;
