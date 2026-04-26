## ADDED Requirements

### Requirement: Web-based visa/service research crawl
The system SHALL implement `VisaResearchHandler` that handles `job.Type = "visa_research"`. It SHALL accept only `web_url` source types and extract structured data fields from each page: `title`, `deadline_date`, `fee_amount`, `document_list`, `office_address`, `contact_info`. Fields absent from a page SHALL be `null` (not an error).

#### Scenario: Visa page structured extraction
- **WHEN** `VisaResearchHandler.Handle` navigates to a Vietnamese embassy or government visa page
- **THEN** the handler extracts `title`, `fee_amount` (e.g., "850.000 VNĐ"), `deadline_date` if present, and `document_list` as a string array; missing fields are null

#### Scenario: General service page extraction
- **WHEN** the source is a private visa agency page
- **THEN** the handler extracts `title`, `contact_info` (phone, email), and `office_address` where present; partial extraction is valid

### Requirement: Restricted filter pipeline for web pages
`VisaResearchHandler` SHALL apply only `KeywordRelevance` and `QualityThreshold` stages from `FilterEngine`. It SHALL NOT apply `EngagementGate` (web pages have no reactions/comments), `LanguageFilter`, or `IntentMatchScorer`.

#### Scenario: No engagement filter on visa pages
- **WHEN** a visa page is evaluated and has no engagement data
- **THEN** the `EngagementGate` is not called; the item is not failed for missing engagement metrics

### Requirement: Summary insight always produced
`VisaResearchHandler` SHALL always call `InsightPipeline.Run(records, ["summary"])` after crawl completion, regardless of `task.Output.Insights`. Additional requested insights are appended.

#### Scenario: Summary always in output
- **WHEN** `VisaResearchHandler.Handle` completes with 5 pages crawled
- **THEN** `OutputDataset.insights.summary` contains a Vietnamese text summary of the visa requirements found

### Requirement: No Facebook-specific crawl logic
`VisaResearchHandler` SHALL NOT use any Facebook-specific page navigation, authentication tokens, or Facebook DOM selectors. It navigates public URLs only via the browser container's unauthenticated session.

#### Scenario: Visa handler uses public URL navigation only
- **WHEN** `VisaResearchHandler.Handle` executes
- **THEN** it navigates directly to each `source.url` without logging into any account; no Facebook session cookies are used
