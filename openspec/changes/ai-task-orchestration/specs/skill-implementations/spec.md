## ADDED Requirements

### Requirement: `scrape_group` skill
The system SHALL implement a `scrape_group` skill that navigates to a Facebook group URL, scrolls to collect posts up to `max_posts` (default 50), and returns a `[]Post` slice. Each `Post` SHALL include `url`, `author_name`, `author_profile_url`, `content`, `timestamp`, and `group_url`. The skill SHALL deduplicate against existing `posts` table rows by `dedup_hash` before writing.

#### Scenario: Successful group scrape
- **WHEN** `scrape_group.Run(ctx, accountID, {"url":"https://facebook.com/groups/abc"})` is called with a running workspace
- **THEN** the skill navigates to the group, scrolls and collects posts, writes new posts to the `posts` table, and returns `SkillResult{Summary:"Đã cào N bài viết", Data:[]Post{...}}`

#### Scenario: Scrape with custom post limit
- **WHEN** `scrape_group.Run` is called with `params={"url":"...","max_posts":20}`
- **THEN** the skill stops after collecting 20 posts regardless of remaining scroll depth

#### Scenario: No new posts found (all duplicates)
- **WHEN** all scraped posts already exist in the `posts` table by `dedup_hash`
- **THEN** the skill returns `SkillResult{Summary:"Không có bài viết mới", Data:[]Post{}}` and writes nothing to the DB

### Requirement: `post_comment` skill
The system SHALL implement a `post_comment` skill that navigates to a Facebook post URL and submits a comment. The `message` parameter is required. An optional `image_path` parameter specifies a file under `data/images/` to attach; the system SHALL reject absolute paths or paths outside `data/images/`.

#### Scenario: Successful comment posted
- **WHEN** `post_comment.Run(ctx, accountID, {"url":"https://fb.com/post/123","message":"Bạn quan tâm không?"})` is called
- **THEN** the skill navigates to the post, types the message into the comment box, and submits; returns `SkillResult{Summary:"Đã đăng bình luận"}`

#### Scenario: Image attachment from allowed directory
- **WHEN** `post_comment.Run` is called with `{"image_path":"data/images/product.jpg"}`
- **THEN** the skill attaches the image file to the comment before submitting

#### Scenario: Image path outside `data/images/` rejected
- **WHEN** `post_comment.Run` is called with `{"image_path":"/etc/passwd"}`
- **THEN** the skill returns an error before any browser action; no comment is posted

### Requirement: `send_inbox` skill
The system SHALL implement a `send_inbox` skill that opens a Facebook Messenger conversation with a target user profile URL and sends a message. The `profile_url` and `message` parameters are required.

#### Scenario: Successful DM sent
- **WHEN** `send_inbox.Run(ctx, accountID, {"profile_url":"https://fb.com/user123","message":"Xin chào!"})` is called
- **THEN** the skill navigates to the profile, opens the Message button, types and sends the message; returns `SkillResult{Summary:"Đã gửi tin nhắn tới user123"}`

### Requirement: `check_notifications` skill
The system SHALL implement a `check_notifications` skill that opens the Facebook notifications panel and returns a list of unread notifications (up to `max_items`, default 20). Each notification SHALL include `text`, `url`, and `timestamp`.

#### Scenario: Notifications retrieved
- **WHEN** `check_notifications.Run(ctx, accountID, {})` is called
- **THEN** the skill opens the notifications panel, reads up to 20 notifications, and returns `SkillResult{Summary:"Có N thông báo mới", Data:[]Notification{...}}`

### Requirement: `get_profile_info` skill
The system SHALL implement a `get_profile_info` skill that navigates to a Facebook profile URL and extracts public information: `name`, `bio`, `location`, `work`, `follower_count`. The `profile_url` parameter is required.

#### Scenario: Profile info returned
- **WHEN** `get_profile_info.Run(ctx, accountID, {"profile_url":"https://fb.com/someone"})` is called
- **THEN** the skill returns `SkillResult{Summary:"Đã lấy thông tin profile", Data:ProfileInfo{...}}`

### Requirement: `comment_hot_leads` skill
The system SHALL implement a `comment_hot_leads` skill that queries the `leads` table for all leads with `score="hot"` and `status="pending_comment"` for the caller's org, generates a comment for each using the existing `internal/ai` comment generator, and calls `post_comment.Run` for each. The `max_comments` parameter (default 10) caps the batch.

#### Scenario: Hot leads commented in batch
- **WHEN** `comment_hot_leads.Run(ctx, accountID, {})` is called and 5 hot leads with `status="pending_comment"` exist
- **THEN** the skill generates and posts a comment for each lead, updates each lead's `status` to `"commented"`, and returns `SkillResult{Summary:"Đã bình luận 5 leads nóng"}`

#### Scenario: No hot leads — graceful no-op
- **WHEN** `comment_hot_leads.Run` is called and no leads have `score="hot" AND status="pending_comment"`
- **THEN** the skill returns `SkillResult{Summary:"Không có leads nóng cần bình luận"}` without making any browser calls

### Requirement: Inter-action delay
All skills SHALL wait `SKILL_ACTION_DELAY_MS` (default 1500ms) between consecutive browser interactions (clicks, form submissions, navigations) to reduce the risk of Facebook rate-limiting.

#### Scenario: Delay applied between actions
- **WHEN** `post_comment.Run` types a message and then clicks Submit
- **THEN** at least `SKILL_ACTION_DELAY_MS` elapses between the type action and the click action

### Requirement: Selector healing on failure
When a skill's `chromedp` selector fails to find an element (timeout or `"no such element"`), the skill SHALL call `internal/ai.SelectorHealer.Heal(ctx, action, platform)` to retrieve a corrected selector and retry once. If the healed selector also fails, the skill returns an error.

#### Scenario: Broken selector healed and retried
- **WHEN** a `chromedp.Click("#comment-box")` times out
- **THEN** the skill calls `SelectorHealer.Heal("comment_input", "facebook")`, receives a new selector, retries the click once with the healed selector, and continues if the retry succeeds

#### Scenario: Double failure after healing
- **WHEN** both the original and healed selectors fail
- **THEN** the skill returns an error with both selector strings logged for diagnostics
