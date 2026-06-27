package reel

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/thg/scraper/internal/ai"
)

// scriptEngine turns a grounded brief into a ScriptDraft. It prefers the AI generator
// (grounded on the business profile, per the AI-grounding rule) and degrades honestly to
// a deterministic shot plan when no API key is configured or the call fails — so the
// backend is fully exercisable offline (Postman/CI) without inventing business facts.
//
// mgGet is a getter, not a value, because the server wires the classifier AFTER routes
// are registered (SetUniversalClassifier); capturing it directly would freeze it at nil.
type scriptEngine struct {
	mgGet     func() *ai.MessageGenerator
	claude    *claudeClient // real Anthropic path; nil/unavailable when no key configured
	marketing string        // brand marketing playbook injected into every script prompt
}

// Generate produces a ScriptDraft. Preference order: Claude (Anthropic) → OpenAI →
// deterministic. Any provider error degrades to the next so reel creation never blocks.
func (e *scriptEngine) Generate(ctx context.Context, in ScriptInput) ScriptDraft {
	in.MarketingGuide = e.marketing // ground every provider on the marketing playbook
	if e.claude.available() {
		draft, err := e.claude.generate(ctx, in)
		if err == nil && len(draft.Shots) > 0 {
			return draft
		}
		// Surface the provider failure (e.g. 401 invalid key) instead of degrading silently.
		log.Printf("[reel] Claude script generation failed, degrading: %v", err)
	}
	mg := e.generator()
	if mg != nil && mg.Available() {
		if draft, err := e.generateAI(ctx, mg, in); err == nil && len(draft.Shots) > 0 {
			return draft
		}
	}
	return deterministicDraft(in)
}

func (e *scriptEngine) generator() *ai.MessageGenerator {
	if e.mgGet == nil {
		return nil
	}
	return e.mgGet()
}

func (e *scriptEngine) generateAI(ctx context.Context, mg *ai.MessageGenerator, in ScriptInput) (ScriptDraft, error) {
	prompt := fmt.Sprintf(`Bạn là copywriter video ngắn (reel) cho doanh nghiệp. Viết kịch bản reel ~%d giây.

THÔNG TIN DOANH NGHIỆP (chỉ dùng dữ kiện trong đây, KHÔNG bịa giá/website/cam kết):
%s%s

Ý TƯỞNG / PHONG CÁCH: %s
TỪ KHOÁ: %s

Chia thành 6-8 shot, mỗi shot 4-5 giây. Mỗi shot có: scene (số thứ tự từ 1), kind (broll|product|talking_head),
prompt (mô tả hình ảnh), dur_sec, voiceover (lời thoại tiếng Việt ngắn). Caption kèm hashtag phù hợp.
verify_flags: liệt kê mọi con số/khẳng định cần con người xác minh (rỗng nếu không có).`,
		in.TargetDuration, strings.TrimSpace(in.BusinessBlock), marketingGuideBlock(in.MarketingGuide), in.BriefStyle, strings.Join(in.Keywords, ", "))

	var draft ScriptDraft
	if err := mg.GenerateStructured(ctx, prompt, "reel_script", reelScriptSchema(), &draft); err != nil {
		return ScriptDraft{}, err
	}
	return draft, nil
}

// marketingGuideBlock formats the brand marketing playbook for the script prompt, or "" when
// none is configured. Both the Claude and OpenAI prompt builders share it so the tone/voice
// grounding stays identical across providers (DRY).
func marketingGuideBlock(guide string) string {
	g := strings.TrimSpace(guide)
	if g == "" {
		return ""
	}
	return "\n\nHƯỚNG DẪN THƯƠNG HIỆU & MARKETING (BẮT BUỘC bám theo tone, voice, từ nên/cấm dùng, " +
		"cấu trúc hook–thân–CTA, hashtag chuẩn trong tài liệu dưới đây):\n" + g
}

// reelScriptSchema is the OpenAI strict-mode schema for a ScriptDraft.
func reelScriptSchema() map[string]any {
	shot := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"scene":     map[string]any{"type": "integer"},
			"kind":      map[string]any{"type": "string", "enum": []string{"broll", "product", "talking_head"}},
			"prompt":    map[string]any{"type": "string"},
			"dur_sec":   map[string]any{"type": "integer"},
			"voiceover": map[string]any{"type": "string"},
		},
		"required": []string{"scene", "kind", "prompt", "dur_sec", "voiceover"},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"dialogue":     map[string]any{"type": "string"},
			"caption":      map[string]any{"type": "string"},
			"shots":        map[string]any{"type": "array", "items": shot},
			"verify_flags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []string{"dialogue", "caption", "shots", "verify_flags"},
	}
}

// deterministicDraft builds a grounded-but-templated shot plan with no external call.
// Shot count derives from the target duration (~4.5s/shot, clamped 3..8).
func deterministicDraft(in ScriptInput) ScriptDraft {
	dur := in.TargetDuration
	if dur <= 0 {
		dur = 25
	}
	shotCount := dur / 5
	if shotCount < 3 {
		shotCount = 3
	}
	if shotCount > 8 {
		shotCount = 8
	}
	perShot := dur / shotCount
	if perShot < 3 {
		perShot = 3
	}
	kw := in.Keywords
	shots := make([]Shot, 0, shotCount)
	for i := 0; i < shotCount; i++ {
		kind := "broll"
		if i == 0 {
			kind = "talking_head"
		} else if len(in.Keywords) > 0 && i%2 == 1 {
			kind = "product"
		}
		topic := in.BriefStyle
		if len(kw) > 0 {
			topic = kw[i%len(kw)]
		}
		shots = append(shots, Shot{
			Scene:     i + 1,
			Kind:      kind,
			Prompt:    fmt.Sprintf("Cảnh %d: %s", i+1, topic),
			DurSec:    perShot,
			Voiceover: topic,
		})
	}
	caption := strings.TrimSpace(in.BriefStyle)
	if caption == "" {
		caption = "Reel"
	}
	if len(kw) > 0 {
		caption += " #" + strings.ReplaceAll(strings.TrimSpace(kw[0]), " ", "")
	}
	return ScriptDraft{
		Dialogue:    caption,
		Caption:     caption,
		Shots:       shots,
		VerifyFlags: []string{},
	}
}
