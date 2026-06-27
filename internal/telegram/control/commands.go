package control

import (
	tgstore "github.com/thg/scraper/internal/store/telegram"
	"github.com/thg/scraper/internal/telegram/render"
)

// IncomingMessage is the channel-neutral, already-extracted Telegram message the webhook hands to
// the domain. The webhook stays thin: parse JSON → fill this → HandleMessage.
type IncomingMessage struct {
	TgUserID  int64
	ChatID    int64
	Username  string
	FirstName string
	Text      string
}

// HandleMessage is the single command dispatcher used by the webhook runtime. It parses the
// command, stamps last_command_at + audits for bound users, enforces the no-execution policy, and
// replies via the injected Sender. Returns an error only on a send failure (logged by the caller).
func (s *Service) HandleMessage(m IncomingMessage) error {
	cmd, arg := ParseCommand(m.Text)

	// /start and /help are valid for unbound users (onboarding) — answer before any binding work.
	if cmd == CmdStart {
		return s.reply(m.ChatID, render.Start())
	}
	if cmd == CmdHelp || cmd == "" || (!IsSupported(cmd) && !IsExecutionCommand(cmd)) {
		return s.reply(m.ChatID, render.Unknown())
	}

	// Outbound-execution attempt → always denied + audited (tenant-scoped if we can resolve it).
	if IsExecutionCommand(cmd) {
		if org := s.resolveOrg(m.TgUserID); org != 0 {
			s.audit(org, 0, m.TgUserID, AuditCommandDenied, cmd, "")
		}
		return s.reply(m.ChatID, render.Denied())
	}

	// /bind is the one control command meaningful while unbound.
	if cmd == CmdBind {
		res := s.Bind(arg, m.TgUserID, m.ChatID, m.Username, m.FirstName)
		if !res.OK {
			return s.reply(m.ChatID, render.BindError())
		}
		return s.reply(m.ChatID, render.BindSuccess(m.FirstName))
	}

	// Remaining commands (/status, /unbind) require an existing binding. Stamp + audit the attempt.
	bindings, bound, _ := s.ActiveBindings(m.TgUserID)
	if bound {
		_ = s.store.UpdateLastCommand(m.TgUserID)
		s.audit(bindings[0].OrgID, 0, m.TgUserID, AuditCommandReceived, cmd, "")
	}
	return s.handleBoundCommand(m, cmd, bindings, bound)
}

// handleBoundCommand answers the binding-aware commands (/status, /unbind) and
// falls back to Unknown. Extracted from HandleMessage's dispatch switch; behavior
// unchanged (an unbound user still gets the unbound variant).
func (s *Service) handleBoundCommand(m IncomingMessage, cmd string, bindings []tgstore.Binding, bound bool) error {
	switch cmd {
	case CmdStatus:
		if !bound {
			return s.reply(m.ChatID, render.StatusUnbound())
		}
		return s.reply(m.ChatID, render.StatusBound(len(bindings)))
	case CmdUnbind:
		if !bound {
			return s.reply(m.ChatID, render.UnbindNone())
		}
		n, _ := s.Unbind(m.TgUserID)
		return s.reply(m.ChatID, render.UnbindDone(int(n)))
	}
	return s.reply(m.ChatID, render.Unknown())
}

// reply sends a DM response via the PLATFORM/dev bot the shared webhook belongs to (DM commands
// arrive on that bot). No-op when no global bot is configured (per-workspace DM webhook is pending).
func (s *Service) reply(chatID int64, text string) error {
	bot := s.globalBot()
	if bot == nil {
		return nil
	}
	return bot.Send(chatID, text)
}
