## ADDED Requirements

### Requirement: General-purpose intent-based web crawl
The system SHALL implement `WebCrawlHandler` that handles `job.Type = "web_crawl"`. It SHALL accept `web_url` source types and extract generic structured data: `title`, `description`, `contact_info`, `price_signals` (numeric values and currency markers found in text), `source_url`. It is the default handler for POD/dropshipping discovery, supplier research, and open-web intent tasks.

#### Scenario: POD customer discovery crawl
- **WHEN** `WebCrawlHandler.Handle` processes a task with `intent="web_crawl"` and sources pointing to Facebook marketplace and forum pages
- **THEN** the handler extracts `title`, `description`, `price_signals`, `contact_info` from each page and returns structured records

#### Scenario: Generic supplier research
- **WHEN** sources are B2B directories or e-commerce listing pages
- **THEN** the handler extracts product titles, pricing text, and contact info; missing fields are null

### Requirement: Restricted filter pipeline for open web
`WebCrawlHandler` SHALL apply `KeywordRelevance`, `SpamDetector`, and `IntentMatchScorer` stages only. It SHALL NOT apply `EngagementGate` or `LanguageFilter` (open-web content is multilingual and has no social engagement metrics).

#### Scenario: No engagement or language filtering
- **WHEN** a web page is evaluated with no engagement data and content in multiple languages
- **THEN** only keyword relevance, spam check, and intent match are evaluated

### Requirement: Public URL navigation only
`WebCrawlHandler` SHALL NOT use any authenticated browser session. If the browser container has an active Facebook session, the handler SHALL NOT leverage it for `web_url` sources — navigation is via a new unauthenticated tab or the existing session browsing to public URLs.

#### Scenario: No session credentials used for web crawl
- **WHEN** `WebCrawlHandler.Handle` navigates to a public marketplace URL
- **THEN** the page is loaded without any Facebook-specific auth header, cookie injection, or login step
