package ai

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

func productionAgentTools() []map[string]any {
	allowed := map[string]bool{
		"set_context":       true,
		"describe_business": true,
		"get_stats":         true,
		"add_group":         true,
		"scrape_group":      true,
		"scrape_comments":   true,
		"classify_leads":    true,
		"search_groups":     true,
		"auto_comment":      true,
		"comment_all_leads": true,
		"auto_inbox":        true,
		"inbox_all_leads":   true,
		"create_job_post":   true,
	}
	out := make([]map[string]any, 0, len(allowed))
	for _, tool := range agentTools {
		fn, _ := tool["function"].(map[string]any)
		name, _ := fn["name"].(string)
		if allowed[name] {
			out = append(out, tool)
		}
	}
	return out
}

var agentTools = []map[string]any{
	{
		"type": "function",
		"function": map[string]any{
			"name":        "scrape_group",
			"description": "Crawl a concrete Facebook group or post URL through the authenticated workspace browser session.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":        map[string]string{"type": "string", "description": "Facebook group or post URL"},
					"account_id": map[string]string{"type": "integer", "description": "Workspace Facebook account ID. Empty means auto-pick a ready account."},
					"max_items":  map[string]string{"type": "integer", "description": "Maximum posts/items to collect when the user gives a number."},
				},
				"required": []string{"url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "scrape_comments",
			"description": "Read existing comments from one Facebook post for lead analysis. Do not use this to publish a new comment.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"post_url":   map[string]string{"type": "string", "description": "Facebook post URL"},
					"account_id": map[string]string{"type": "integer", "description": "Workspace Facebook account ID"},
					"max_items":  map[string]string{"type": "integer", "description": "Maximum comments/items to collect when the user gives a number."},
				},
				"required": []string{"post_url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "search_groups",
			"description": "Discover suitable Facebook sources when the user describes a target audience but does not provide a source URL.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":      map[string]string{"type": "string", "description": "Search query derived from prompt and business context"},
					"account_id": map[string]string{"type": "integer", "description": "Workspace Facebook account ID. Empty means auto-pick a ready account."},
					"max_items":  map[string]string{"type": "integer", "description": "Maximum sources/items to collect when the user gives a number."},
				},
				"required": []string{"query"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "auto_comment",
			"description": "Queue a comment for one concrete Facebook post. Default is draft unless auto mode is explicit.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"post_url":    map[string]string{"type": "string", "description": "Target post URL"},
					"account_id":  map[string]string{"type": "integer", "description": "Workspace Facebook account ID"},
					"context":     map[string]string{"type": "string", "description": "Post context if available"},
					"target_name": map[string]string{"type": "string", "description": "Author name if available"},
				},
				"required": []string{"post_url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "comment_all_leads",
			"description": "Queue comments for qualified leads with dedup, cooldown, and approval guardrails.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"template":     map[string]string{"type": "string", "description": "Optional user-provided comment template"},
					"score_filter": map[string]string{"type": "string", "description": "hot, warm, cold, or all"},
					"account_id":   map[string]string{"type": "integer", "description": "Workspace Facebook account ID"},
					"max_items":    map[string]string{"type": "integer", "description": "Maximum number of leads to comment when the user gives a number — e.g. set 1 to test a single comment. Omit for the default."},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "auto_inbox",
			"description": "Queue an inbox message for one concrete lead. Default is draft unless auto mode is explicit.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target_url":  map[string]string{"type": "string", "description": "Profile or Messenger target URL"},
					"account_id":  map[string]string{"type": "integer", "description": "Workspace Facebook account ID"},
					"context":     map[string]string{"type": "string", "description": "Lead context"},
					"target_name": map[string]string{"type": "string", "description": "Lead name if available"},
				},
				"required": []string{"target_url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "inbox_all_leads",
			"description": "Queue inbox outreach for qualified leads with conversation, dedup, cooldown, and approval guardrails.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"score_filter": map[string]string{"type": "string", "description": "hot, warm, cold, or all"},
					"account_id":   map[string]string{"type": "integer", "description": "Workspace Facebook account ID"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "create_job_post",
			"description": "Queue a Facebook post/group post draft from the user request and business context.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":       map[string]string{"type": "string", "description": "Post title or topic"},
					"description": map[string]string{"type": "string", "description": "Post brief"},
					"content":     map[string]string{"type": "string", "description": "Full content if provided"},
					"group_url":   map[string]string{"type": "string", "description": "Target group URL if specified"},
					"account_id":  map[string]string{"type": "integer", "description": "Workspace Facebook account ID"},
				},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "describe_business",
			"description": "Store org-scoped business context: brand, services, target customers, tone, reject rules, and approval policy.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"description": map[string]string{"type": "string", "description": "Free-form business/workspace description"},
				},
				"required": []string{"description"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "set_context",
			"description": "Store org-scoped configuration such as business_profile, private_files_summary, or data_sources_summary. NOT for approval / outbound policy — outbound_mode and auto_comment_mode are admin-only and the server will reject this call for those keys.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key":   map[string]string{"type": "string", "description": "business_profile, private_files_summary, data_sources_summary"},
					"value": map[string]string{"type": "string", "description": "Value to store"},
				},
				"required": []string{"key", "value"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "get_stats",
			"description": "Read workspace stats.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "add_group",
			"description": "Register a Facebook source for the current organization.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":  map[string]string{"type": "string", "description": "Facebook source URL"},
					"name": map[string]string{"type": "string", "description": "Source name"},
				},
				"required": []string{"url", "name"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "classify_leads",
			"description": "Confirm that classification is handled inline by prompt-scoped crawl results and current business context.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
}
