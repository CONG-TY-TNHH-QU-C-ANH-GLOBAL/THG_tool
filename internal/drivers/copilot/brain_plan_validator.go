package copilot

import (
	"errors"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/fburl"
	"github.com/thg/scraper/internal/textutil"
)

func validateBrainPlan(plan *BrainPlanResponse) error {
	if plan == nil {
		return errors.New("missing plan")
	}
	domain := strings.ToLower(strings.TrimSpace(plan.DomainScope))
	if domain != "facebook" && domain != "out_of_scope" {
		return fmt.Errorf("invalid domain_scope %q", plan.DomainScope)
	}
	decision := strings.ToLower(strings.TrimSpace(plan.Decision))
	switch decision {
	case "execute", "ask_user", "chat", "refuse":
	default:
		return fmt.Errorf("invalid decision %q", plan.Decision)
	}
	if plan.Confidence < 0 || plan.Confidence > 1 {
		return fmt.Errorf("confidence out of range: %v", plan.Confidence)
	}
	for _, action := range plan.Actions {
		if err := validateBrainAction(action); err != nil {
			return err
		}
	}
	return nil
}

func validateBrainAction(action BrainAction) error {
	tool := strings.TrimSpace(action.Tool)
	if !allowedBrainTool(tool) {
		return fmt.Errorf("tool %q is not allowed", action.Tool)
	}
	switch tool {
	case "scrape_group":
		if !isFacebookURL(argStringFromMap(action.Args, "url")) {
			return errors.New("scrape_group requires a concrete Facebook url")
		}
	case "scrape_comments":
		if !isFacebookURL(argStringFromMap(action.Args, "post_url")) {
			return errors.New("scrape_comments requires a concrete Facebook post_url")
		}
	case "search_groups":
		query := strings.TrimSpace(argStringFromMap(action.Args, "query"))
		if query == "" || tooBroadBrainQuery(query) {
			return errors.New("search_groups requires a specific query")
		}
	case "add_group":
		if !isFacebookURL(argStringFromMap(action.Args, "url")) {
			return errors.New("add_group requires a Facebook url")
		}
	case "auto_comment":
		if !isFacebookURL(argStringFromMap(action.Args, "post_url")) {
			return errors.New("auto_comment requires a Facebook post_url")
		}
	case "auto_inbox":
		if !isFacebookURL(argStringFromMap(action.Args, "target_url")) {
			return errors.New("auto_inbox requires a Facebook target_url")
		}
	case "create_job_post":
		if strings.TrimSpace(textutil.FirstNonEmpty(argStringFromMap(action.Args, "content"), argStringFromMap(action.Args, "description"), argStringFromMap(action.Args, "title"))) == "" {
			return errors.New("create_job_post requires title, description, or content")
		}
	case "scan_fanpage_inbox":
		if !isFacebookURL(argStringFromMap(action.Args, "page_url")) {
			return errors.New("scan_fanpage_inbox requires a concrete Facebook page_url")
		}
	case "care_fanpage":
		if !isFacebookURL(argStringFromMap(action.Args, "page_url")) || strings.TrimSpace(argStringFromMap(action.Args, "action")) == "" {
			return errors.New("care_fanpage requires page_url and action")
		}
	case "post_to_profile":
		if strings.TrimSpace(argStringFromMap(action.Args, "content")) == "" {
			return errors.New("post_to_profile requires content")
		}
	case "set_context":
		if strings.TrimSpace(argStringFromMap(action.Args, "key")) == "" || strings.TrimSpace(argStringFromMap(action.Args, "value")) == "" {
			return errors.New("set_context requires key and value")
		}
	case "describe_business":
		if strings.TrimSpace(argStringFromMap(action.Args, "description")) == "" {
			return errors.New("describe_business requires description")
		}
	}
	return nil
}

func allowedBrainTool(name string) bool {
	return brainAllowedTools()[strings.TrimSpace(name)]
}

// isFacebookURL delegates to the host-anchored fburl source of truth so brain
// action validation rejects lookalike hosts (facebook.com.evil.com) that the old
// substring check accepted.
func isFacebookURL(raw string) bool {
	return fburl.IsFacebookURL(raw)
}

func tooBroadBrainQuery(query string) bool {
	folded := foldVietnameseForMatch(query)
	parts := strings.Fields(folded)
	if len(parts) < 2 {
		switch strings.TrimSpace(folded) {
		case "facebook", "fb", "group", "groups", "lead", "leads", "khach", "khach hang":
			return true
		}
	}
	return false
}
