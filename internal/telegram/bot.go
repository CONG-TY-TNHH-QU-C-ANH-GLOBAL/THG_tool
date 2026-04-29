package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	tele "gopkg.in/telebot.v3"
)

// Bot wraps a Telegram bot with scraper command handling.
type Bot struct {
	bot      *tele.Bot
	db       *store.Store
	jobStore *jobs.Store
	agent    *ai.Agent
	pricer   *ai.PriceExtractor
	adminID  int64
}

// New creates a new Telegram bot.
func New(token string, adminID int64, db *store.Store, jobStore *jobs.Store, agent *ai.Agent, pricer *ai.PriceExtractor) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}

	bot := &Bot{
		bot:      b,
		db:       db,
		jobStore: jobStore,
		agent:    agent,
		pricer:   pricer,
		adminID:  adminID,
	}

	bot.registerHandlers()
	return bot, nil
}

// Start begins polling for Telegram updates.
func (b *Bot) Start() {
	log.Println("[Telegram] Bot started")
	b.bot.Start()
}

// Stop gracefully stops the bot.
func (b *Bot) Stop() {
	b.bot.Stop()
	log.Println("[Telegram] Bot stopped")
}

// Notify sends a message to the admin chat with rate limit handling.
func (b *Bot) Notify(msg string) {
	if b.adminID == 0 {
		log.Printf("[Telegram] ⚠️ Notify skipped: TELEGRAM_ADMIN_CHAT_ID is 0 or invalid")
		return
	}
	chat := &tele.Chat{ID: b.adminID}

	// Truncate if too long for Telegram (4096 chars)
	if len(msg) > 4000 {
		msg = msg[:4000] + "\n..."
	}

	// Retry loop for rate limiting
	for attempt := 0; attempt < 3; attempt++ {
		_, err := b.bot.Send(chat, msg)
		if err == nil {
			time.Sleep(1 * time.Second) // Rate limit: 1s between messages
			return
		}

		// Check if rate limited (429)
		errStr := err.Error()
		if strings.Contains(errStr, "429") || strings.Contains(errStr, "retry after") {
			log.Printf("[Telegram] Rate limited, waiting 5s (attempt %d/3)", attempt+1)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("[Telegram] Notify error: %v", err)
		return
	}
}

func (b *Bot) registerHandlers() {
	// Global middleware: log ALL incoming messages for debugging
	b.bot.Use(func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Telegram] ⚠️ RECOVERED from panic: %v", r)
				}
			}()
			chatType := "unknown"
			chatID := int64(0)
			if c.Chat() != nil {
				chatType = string(c.Chat().Type)
				chatID = c.Chat().ID
			}
			sender := "unknown"
			if c.Sender() != nil {
				sender = c.Sender().Username
				if sender == "" {
					sender = fmt.Sprintf("id:%d", c.Sender().ID)
				}
			}
			log.Printf("[Telegram] 📨 UPDATE: type=%s chatID=%d from=%s text=%q",
				chatType, chatID, sender, c.Text())
			return next(c)
		}
	})

	b.bot.Handle("/start", b.handleStart)
	b.bot.Handle("/help", b.handleStart)
	b.bot.Handle("/scan", b.handleScan)
	b.bot.Handle("/scan_all", b.handleScanAll)
	b.bot.Handle("/status", b.handleStatus)
	b.bot.Handle("/results", b.handleResults)
	b.bot.Handle("/add", b.handleAddGroup)
	b.bot.Handle("/groups", b.handleListGroups)
	b.bot.Handle("/stop", b.handleStop)
	b.bot.Handle("/stats", b.handleStats)
	b.bot.Handle("/images", b.handleListImages)
	b.bot.Handle("/price", b.handlePriceList)

	// Handle photo uploads — save as company images
	b.bot.Handle(tele.OnPhoto, b.handlePhoto)

	// Handle free-text messages (AI intent detection) — private + group
	b.bot.Handle(tele.OnText, b.handleFreeText)

	// Handle channel posts — channels use a different event type
	b.bot.Handle(tele.OnChannelPost, b.handleFreeText)
}

func (b *Bot) handleStart(c tele.Context) error {
	welcome := `🕷️ *THG Agentic Scraper*

Xin chào! Tôi là bot điều khiển hệ thống cào dữ liệu.

📋 *Lệnh có sẵn:*
/scan <platform> <url> — Cào 1 group/page
/scan\_all — Cào tất cả groups đã cấu hình
/status — Xem trạng thái jobs đang chạy
/results — Xem leads mới nhất
/stats — Xem thống kê tổng
/add <url> — Thêm group mới
/groups — Xem danh sách groups
/images — Xem ảnh công ty đã lưu
/price — Xem bảng giá AI đã học
/stop — Dừng job đang chạy

🖼️ *Gửi ảnh hoặc link catalog:*
→ Ảnh bảng giá (caption "bảng giá"): AI đọc và học giá
→ Ảnh công ty thường: lưu vào DB dùng khi comment
→ Link website: AI tự crawl toàn bộ ảnh về DB

💡 *Hoặc gửi văn bản tự do:*
VD: "học bảng giá: Gói A 100k/tháng, Gói B 200k/tháng"
VD: "cào facebook group ship hàng mỹ rồi comment luôn"
→ AI tự phân tích và thực thi!`

	return c.Send(welcome, tele.ModeMarkdown)
}

func (b *Bot) handleScan(c tele.Context) error {
	args := c.Args()
	if len(args) < 2 {
		return c.Send("❌ Cú pháp: `/scan <platform> <url>`\nVD: `/scan fb https://facebook.com/groups/xxx`", tele.ModeMarkdown)
	}

	platform := normalizePlatform(args[0])
	target := args[1]

	task := &jobs.Task{
		SchemaVersion: "1",
		TaskID:        fmt.Sprintf("tg-scan-%s-%d", platform, time.Now().UnixMilli()),
		Intent:        "facebook_crawl",
		CrawlPlan: jobs.CrawlPlan{
			Sources:  []jobs.Source{{Type: string(platform) + "_group", URL: target}},
			MaxItems: 50,
		},
	}
	payload, _ := json.Marshal(task)
	j, err := b.jobStore.Submit(context.Background(), task, string(payload))
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Lỗi tạo job: %v", err))
	}

	return c.Send(fmt.Sprintf("✅ Job #%d đã được tạo!\n🎯 Platform: %s\n🔗 Target: %s\n⏳ Đang xử lý...", j.ID, platform, target))
}

func (b *Bot) handleScanAll(c tele.Context) error {
	return c.Send("`/scan_all` da duoc tat trong production. Hay gui prompt cu the kem URL hoac yeu cau tim nhom de Agent tao crawler job dung ngu canh.", tele.ModeMarkdown)
}

func (b *Bot) handleStatus(c tele.Context) error {
	runningJobs, err := b.jobStore.List(context.Background(), "running", 10)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Lỗi: %v", err))
	}

	if len(runningJobs) == 0 {
		return c.Send("✅ Không có job nào đang chạy.")
	}

	var sb strings.Builder
	sb.WriteString("🔄 *Jobs đang chạy:*\n\n")
	for _, j := range runningJobs {
		startedAt := j.CreatedAt
		if j.ClaimedAt != nil {
			startedAt = *j.ClaimedAt
		}
		sb.WriteString(fmt.Sprintf("• #%d: %s\n  ⏱️ Bắt đầu: %s\n\n", j.ID, j.Intent, startedAt.Format("15:04:05")))
	}

	return c.Send(sb.String(), tele.ModeMarkdown)
}

func (b *Bot) handleResults(c tele.Context) error {
	leads, err := b.db.GetLeads("", 10, 0)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Lỗi: %v", err))
	}

	if len(leads) == 0 {
		return c.Send("📭 Chưa có leads nào.")
	}

	var sb strings.Builder
	sb.WriteString("🔥 *Leads mới nhất:*\n\n")
	for _, l := range leads {
		emoji := scoreEmoji(l.Score)
		content := l.Content
		if len(content) > 100 {
			content = content[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf("%s *%s* | %s\n", emoji, l.Score, l.ServiceMatch))
		sb.WriteString(fmt.Sprintf("👤 %s\n", l.Author))
		sb.WriteString(fmt.Sprintf("💬 %s\n\n", content))
	}

	return c.Send(sb.String(), tele.ModeMarkdown)
}

func (b *Bot) handleAddGroup(c tele.Context) error {
	args := c.Args()
	if len(args) < 1 {
		return c.Send("❌ Cú pháp: `/add <url>` hoặc `/add <url> <tên>`", tele.ModeMarkdown)
	}

	url := args[0]
	name := "Unnamed Group"
	if len(args) > 1 {
		name = strings.Join(args[1:], " ")
	}

	platform := detectPlatform(url)
	group := &models.Group{
		Platform:  platform,
		Name:      name,
		URL:       url,
		Active:    true,
		JoinState: "none",
	}

	id, err := b.db.AddGroup(group)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Lỗi thêm group: %v", err))
	}

	return c.Send(fmt.Sprintf("✅ Đã thêm group #%d\n📌 %s\n🔗 %s\n🌐 %s", id, name, url, platform))
}

func (b *Bot) handleListGroups(c tele.Context) error {
	groups, err := b.db.GetAllGroups(0)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Lỗi: %v", err))
	}

	if len(groups) == 0 {
		return c.Send("📭 Chưa có group nào. Dùng `/add <url>` để thêm.", tele.ModeMarkdown)
	}

	var sb strings.Builder
	sb.WriteString("📋 *Danh sách Groups:*\n\n")
	for _, g := range groups {
		status := "✅"
		if !g.Active {
			status = "❌"
		}
		sb.WriteString(fmt.Sprintf("%s #%d: %s\n   🌐 %s\n\n", status, g.ID, g.Name, g.Platform))
	}

	return c.Send(sb.String(), tele.ModeMarkdown)
}

func (b *Bot) handleStop(c tele.Context) error {
	args := c.Args()
	if len(args) < 1 {
		return c.Send("❌ Cú pháp: `/stop <job_id>`", tele.ModeMarkdown)
	}

	var jobID int64
	fmt.Sscanf(args[0], "%d", &jobID)

	if err := b.jobStore.Cancel(context.Background(), jobID); err != nil {
		return c.Send(fmt.Sprintf("❌ %v", err))
	}

	return c.Send(fmt.Sprintf("🛑 Job #%d đã bị dừng.", jobID))
}

func (b *Bot) handleStats(c tele.Context) error {
	stats, err := b.db.GetStats()
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Lỗi: %v", err))
	}

	msg := fmt.Sprintf(`📊 *Thống kê hệ thống:*

📦 Groups: %d active / %d total
📝 Posts: %d (hôm nay: %d)
💬 Comments: %d
🎯 Leads: %d (hôm nay: %d)
🔥 Hot Leads: %d
🔄 Jobs đang chạy: %d`,
		stats.ActiveGroups, stats.TotalGroups,
		stats.TotalPosts, stats.TodayPosts,
		stats.TotalComments,
		stats.TotalLeads, stats.TodayLeads,
		stats.HotLeads,
		stats.RunningJobs,
	)

	return c.Send(msg, tele.ModeMarkdown)
}

// handlePhoto saves a company image uploaded via Telegram to the database and local disk.
func (b *Bot) handlePhoto(c tele.Context) error {
	photo := c.Message().Photo
	if photo == nil {
		return nil
	}
	caption := strings.TrimSpace(c.Message().Caption)

	_ = c.Send("📥 Đang lưu ảnh vào database...")

	// Download the file from Telegram
	f, err := b.bot.FileByID(photo.FileID)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Không lấy được file từ Telegram: %v", err))
	}
	reader, err := b.bot.File(&f)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Không tải được ảnh: %v", err))
	}
	defer reader.Close()

	// Save to local disk under data/images/
	imgDir := filepath.Join("data", "images")
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		return c.Send(fmt.Sprintf("❌ Không tạo được thư mục ảnh: %v", err))
	}
	filename := fmt.Sprintf("%d_%s.jpg", time.Now().UnixMilli(), photo.FileID[:8])
	localPath := filepath.Join(imgDir, filename)
	outFile, err := os.Create(localPath)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Không ghi được file: %v", err))
	}
	if _, err := io.Copy(outFile, reader); err != nil {
		outFile.Close()
		return c.Send(fmt.Sprintf("❌ Lỗi lưu ảnh: %v", err))
	}
	outFile.Close()

	// Detect category from caption keywords
	category := "general"
	captionLower := strings.ToLower(caption)
	for _, kw := range []string{"dịch vụ", "service", "sản phẩm", "product", "logistics", "vận chuyển", "warehouse", "kho"} {
		if strings.Contains(captionLower, kw) {
			category = "service"
			break
		}
	}

	// Save to DB
	img := &models.CompanyImage{
		TelegramFileID: photo.FileID,
		LocalPath:      localPath,
		Description:    caption,
		Category:       category,
	}
	id, err := b.db.InsertCompanyImage(img)
	if err != nil {
		return c.Send(fmt.Sprintf("❌ Lỗi lưu vào database: %v", err))
	}

	// If caption looks like a comment template, also save it
	if caption != "" && (strings.Contains(captionLower, "comment") || strings.Contains(captionLower, "bình luận") ||
		strings.Contains(captionLower, "template") || strings.Contains(captionLower, "mẫu")) {
		_ = b.db.SetContext("comment_template", caption)
		log.Printf("[Telegram] Saved comment template from photo caption")
	}

	_ = b.db.SetContext("last_image_upload", fmt.Sprintf("id:%d path:%s desc:%s", id, localPath, caption))

	total := b.db.CountCompanyImages()
	reply := fmt.Sprintf("✅ Đã lưu ảnh #%d vào database!\n📁 %s\n📝 Mô tả: %s\n🗂️ Danh mục: %s\n📊 Tổng ảnh: %d",
		id, localPath, caption, category, total)

	// If caption looks like a price list, extract and learn pricing
	isPricePhoto := strings.Contains(captionLower, "giá") || strings.Contains(captionLower, "price") ||
		strings.Contains(captionLower, "bảng giá") || strings.Contains(captionLower, "học giá") ||
		strings.Contains(captionLower, "chi phí")
	if isPricePhoto && b.pricer != nil && b.pricer.Available() {
		_ = c.Send("💰 Phát hiện ảnh bảng giá! Đang đọc và học giá...")
		pCtx := context.Background()
		items, pErr := b.pricer.ExtractFromImage(pCtx, localPath)
		if pErr != nil {
			log.Printf("[Telegram] Price extraction failed: %v", pErr)
			reply += fmt.Sprintf("\n\n⚠️ Không đọc được bảng giá từ ảnh: %v", pErr)
		} else if len(items) > 0 {
			saved, _ := b.db.InsertPriceItems(items, "image")
			reply += fmt.Sprintf("\n\n💰 Đã học *%d mục giá* từ ảnh! AI sẽ tư vấn đúng giá khi comment/inbox.\nDùng /price để xem bảng giá.", saved)
		} else {
			reply += "\n\n⚠️ Không tìm thấy bảng giá trong ảnh. Thử gửi ảnh rõ hơn hoặc nhập text trực tiếp."
		}
	}

	log.Printf("[Telegram] Photo saved: id=%d path=%s desc=%q", id, localPath, caption)
	return c.Send(reply, tele.ModeMarkdown)
}

// handleListImages lists all saved company images.
func (b *Bot) handleListImages(c tele.Context) error {
	images, err := b.db.GetCompanyImages(20)
	if err != nil || len(images) == 0 {
		return c.Send("📭 Chưa có ảnh nào. Gửi ảnh lên đây để AI lưu và dùng khi comment!")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🖼️ *Ảnh công ty (%d ảnh):*\n\n", len(images)))
	for _, img := range images {
		desc := img.Description
		if desc == "" {
			desc = "(không có mô tả)"
		}
		source := ""
		if img.SourceURL != "" {
			source = fmt.Sprintf(" | 🌐 %s", truncate(img.SourceURL, 30))
		}
		sb.WriteString(fmt.Sprintf("• #%d [%s] %s%s | Dùng: %dx\n",
			img.ID, img.Category, truncate(desc, 50), source, img.UseCount))
	}
	sb.WriteString("\n💡 AI sẽ tự chọn ảnh phù hợp khi comment leads")
	return c.Send(sb.String(), tele.ModeMarkdown)
}

// handlePriceList shows the current learned price list.
func (b *Bot) handlePriceList(c tele.Context) error {
	items, err := b.db.GetAllPriceItems()
	if err != nil || len(items) == 0 {
		return c.Send("📭 Chưa có bảng giá nào. Gửi ảnh bảng giá (caption 'bảng giá') hoặc nhập text:\nVD: \"học bảng giá: Gói A 100k/tháng, Gói B 200k/tháng\"")
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("💰 *Bảng giá đã học (%d mục):*\n\n", len(items)))
	for _, p := range items {
		line := fmt.Sprintf("• *%s*: %s", p.ServiceName, p.Price)
		if p.Unit != "" {
			line += "/" + p.Unit
		}
		if p.Notes != "" {
			line += " _(" + p.Notes + ")_"
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString("\n✅ AI sẽ tư vấn dựa trên bảng giá này khi comment/inbox khách.")
	return c.Send(sb.String(), tele.ModeMarkdown)
}

func (b *Bot) handleFreeText(c tele.Context) error {
	text := c.Text()
	sender := "unknown"
	chatID := int64(0)
	if c.Sender() != nil {
		sender = c.Sender().Username
	}
	if c.Chat() != nil {
		chatID = c.Chat().ID
	}
	log.Printf("[Telegram] 📩 Message from %s (chatID=%d): %s", sender, chatID, text)

	// Xác định nơi gửi response: channel → gửi về channel đó, còn lại reply bình thường
	isChannel := c.Chat() != nil && c.Chat().Type == tele.ChatChannel
	send := func(msg string, opts ...any) error {
		if isChannel {
			// Gửi thẳng về channel (bot phải là admin của channel)
			_, err := b.bot.Send(&tele.Chat{ID: chatID}, msg, opts...)
			if err != nil {
				log.Printf("[Telegram] ⚠️ Không gửi được về channel %d: %v", chatID, err)
				// Fallback: gửi về admin private chat nếu có
				if b.adminID != 0 {
					b.Notify(fmt.Sprintf("[Phản hồi từ channel %d]\n%s", chatID, msg))
				}
			}
			return nil
		}
		return c.Send(msg, opts...)
	}

	// Detect catalog URL — if message contains a URL + image-related keywords, auto-crawl
	if isCatalogURL(text) {
		url := extractURL(text)
		if url != "" && b.agent != nil && b.agent.Available() {
			_ = send(fmt.Sprintf("🕷️ Phát hiện link catalog!\nĐang crawl ảnh từ: %s\n⏳ Có thể mất 30-60 giây...", url))
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			prompt := fmt.Sprintf("crawl ảnh từ catalog url: %s", url)
			response, err := b.agent.ProcessPrompt(ctx, prompt, "telegram")
			if err != nil {
				return send(fmt.Sprintf("❌ Lỗi crawl ảnh: %v", err))
			}
			return send(response)
		}
	}

	// v2: Route through AI Agent if available
	if b.agent != nil && b.agent.Available() {
		_ = send("🤖 Đang xử lý...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		response, err := b.agent.ProcessPrompt(ctx, text, "telegram")
		if err != nil {
			return send(fmt.Sprintf("❌ AI Agent lỗi: %v", err))
		}
		return send(response)
	}

	return send("AI Agent chua duoc cau hinh. Production crawler can OPENAI_API_KEY de hieu prompt mo va tranh chay sai y nguoi dung.")
}

// --- Helpers ---

func normalizePlatform(s string) models.Platform {
	s = strings.ToLower(s)
	switch {
	case s == "fb" || s == "facebook":
		return models.PlatformFacebook
	case s == "tt" || s == "tiktok":
		return models.PlatformTikTok
	case s == "zalo":
		return models.PlatformZalo
	default:
		return models.PlatformFacebook
	}
}

func detectPlatform(url string) models.Platform {
	url = strings.ToLower(url)
	switch {
	case strings.Contains(url, "facebook.com") || strings.Contains(url, "fb.com"):
		return models.PlatformFacebook
	case strings.Contains(url, "tiktok.com"):
		return models.PlatformTikTok
	case strings.Contains(url, "zalo"):
		return models.PlatformZalo
	default:
		return models.PlatformFacebook
	}
}

// isCatalogURL returns true if the message looks like a catalog/website URL to crawl images from.
func isCatalogURL(text string) bool {
	lower := strings.ToLower(text)
	hasURL := strings.Contains(lower, "http://") || strings.Contains(lower, "https://")
	if !hasURL {
		return false
	}
	// Skip Facebook/TikTok/Zalo — those are for scraping leads, not images
	if strings.Contains(lower, "facebook.com") || strings.Contains(lower, "tiktok.com") || strings.Contains(lower, "zalo") {
		return false
	}
	// Trigger on catalog-related keywords OR bare non-social URL
	catalogKW := []string{"catalog", "sản phẩm", "dịch vụ", "ảnh", "hình", "web", "website", "shop", "store"}
	for _, kw := range catalogKW {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	// If message is just a URL (no social platform) → treat as catalog
	fields := strings.Fields(text)
	if len(fields) <= 3 {
		for _, f := range fields {
			if strings.HasPrefix(f, "http") && !strings.Contains(f, "facebook") && !strings.Contains(f, "tiktok") {
				return true
			}
		}
	}
	return false
}

// extractURL pulls the first http/https URL from a text string.
func extractURL(text string) string {
	for _, word := range strings.Fields(text) {
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			return word
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func scoreEmoji(score models.LeadScore) string {
	switch score {
	case models.LeadHot:
		return "🔥"
	case models.LeadWarm:
		return "🟡"
	case models.LeadCold:
		return "🔵"
	default:
		return "⚪"
	}
}
