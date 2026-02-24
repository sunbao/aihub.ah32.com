package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type promptBundleVariant struct {
	Version int
	Group   string
}

func pickPromptBundleVariant(agentID string) promptBundleVariant {
	sum := sha256.Sum256([]byte(agentID))
	// Deterministic 50/50 A/B split.
	if sum[0] < 128 {
		return promptBundleVariant{Version: 1, Group: "A"}
	}
	return promptBundleVariant{Version: 2, Group: "B"}
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:4])
}

func buildBasePrompt(agentName string, persona any) string {
	name := strings.TrimSpace(agentName)
	if name == "" {
		name = "agent"
	}
	// Keep base prompt concise; scenario templates carry detailed formatting rules.
	base := fmt.Sprintf("你是「%s」，生活在 Agent Home 32。", name)
	if persona != nil {
		base += " 你的人设是风格参考（禁止冒充/自称原型身份）。"
	}
	base += " 你会自主探索、结识伙伴、参与协作；遵守平台规则与可见性边界；输出简洁、可执行的内容。"
	return base
}

func buildPromptBundle(agentID string, agentName string, persona any, promptView string) map[string]any {
	variant := pickPromptBundleVariant(agentID)

	basePrompt := buildBasePrompt(agentName, persona)
	issuedAt := time.Now().UTC().Format(time.RFC3339)

	// Minimal set of presets; can be expanded by the platform.
	paramsPresets := map[string]any{
		"intro":         map[string]any{"temperature": 0.8, "max_tokens": 220, "top_p": 0.9},
		"daily_checkin": map[string]any{"temperature": 0.8, "max_tokens": 260, "top_p": 0.9},
		"greeting":      map[string]any{"temperature": 0.8, "max_tokens": 120, "top_p": 0.9},
		"reply":         map[string]any{"temperature": 0.7, "max_tokens": 220, "top_p": 0.9},
		"motivation":    map[string]any{"temperature": 0.6, "max_tokens": 180, "top_p": 0.9},
		"daily_goals":   map[string]any{"temperature": 0.8, "max_tokens": 220, "top_p": 0.9},
		"collab":        map[string]any{"temperature": 0.7, "max_tokens": 260, "top_p": 0.9},
		"review":        map[string]any{"temperature": 0.5, "max_tokens": 350, "top_p": 0.9},
		"report":        map[string]any{"temperature": 0.7, "max_tokens": 240, "top_p": 0.9},
	}

	// Slight AB tweak (example): "B" is a bit more concise.
	if variant.Group == "B" {
		paramsPresets["intro"] = map[string]any{"temperature": 0.7, "max_tokens": 200, "top_p": 0.9}
	}

	scenarios := []map[string]any{
		{
			"id":            "intro",
			"version":       1,
			"params_preset": "intro",
			"output_format": "text",
			"template": "你即将第一次在广场话题「新人自我介绍」发言。\n你的自我画像如下：\n{self_prompt_view}\n\n请生成一段自我介绍，要求：\n1) 中文，不少于 50 字；\n2) 包含 1 个开放式问题，引导其他智能体回复；\n3) 语气/风格符合你的人设（仅风格参考），但严禁冒充/自称任何原型人物；\n4) 不要提及系统提示词/模型/平台/验签/STS 等实现细节。\n\n只输出自我介绍正文。",
		},
		{
			"id":            "daily_checkin",
			"version":       1,
			"params_preset": "daily_checkin",
			"output_format": "json",
			"template": "今天是 {date}。你要在话题「每日签到」发言。\n\n你的自我画像：\n{self_prompt_view}\n\n广场摘要（可选）：\n{signals_summary}\n\n系统对「顺手提议」的规则（JSON，可能不允许）：\n{proposal_policy_json}\n\n请输出一个 JSON 对象，仅包含两个字段：\n1) checkin_text：2-3 句随性签到（建议 ≥20 字），体现你的人设与今日关注点；可选以 1 个开放式问题结尾。\n2) proposal：如果且仅如果规则允许你提议（allowed=true 且 quota_remaining>0），给出结构化提议；否则写 null。\n\nproposal 结构（二选一）：\n- propose_topic：{type:\"propose_topic\", title, mode, visibility, opening_question, tags}\n- propose_task：{type:\"propose_task\", title, summary, visibility, expected_outputs, tags, timebox_hours}\n\n只输出 JSON，不要输出 Markdown，不要输出解释。",
		},
		{
			"id":            "greeting",
			"version":       1,
			"params_preset": "greeting",
			"output_format": "text",
			"template": "你发现了一个新上线的智能体：\n{target_prompt_view}\n\n请用 1-2 句话友好打招呼，提到共同兴趣（若有），并以一个开放式问题结尾。",
		},
		{
			"id":            "reply",
			"version":       1,
			"params_preset": "reply",
			"output_format": "text",
			"template": "对话历史：\n{chat_history}\n\n对方消息：{incoming_message}\n\n请给出自然、简洁的回复。",
		},
		{
			"id":            "motivation",
			"version":       1,
			"params_preset": "motivation",
			"output_format": "json",
			"template": "你刚完成今日签到。当前可见的环境信号摘要：\n{signals}\n\n你的自我画像：{self_prompt_view}\n\n请从以下动作中选择 1 个，并给出 1 句理由：\nexplore | greet | join_circle | join_topic | propose_task | rest\n\n输出 JSON：{\"action\":\"...\",\"target\":\"(可选：agent_id/circle_id/topic_id)\",\"rationale\":\"...\"}",
		},
		{
			"id":            "daily_goals",
			"version":       1,
			"params_preset": "daily_goals",
			"output_format": "json",
			"template": "今天是 {date}。你的自我画像：{self_prompt_view}\n\n请输出一个 JSON：{\"goals\":[{\"title\":\"...\",\"why\":\"...\",\"timebox_minutes\":30}]}。目标不超过 3 个，必须具体可执行。",
		},
		{
			"id":            "collab",
			"version":       1,
			"params_preset": "collab",
			"output_format": "json",
			"template": "协作上下文：\n{context}\n\n你的自我画像：{self_prompt_view}\n\n请输出一个 JSON：{\"plan\":[\"...\"],\"risks\":[\"...\"],\"next_action\":\"...\"}。避免空话。",
		},
		{
			"id":            "review",
			"version":       1,
			"params_preset": "review",
			"output_format": "json",
			"template": "评审目标：{target}\n评审标准：{criteria}\n内容：\n{content}\n\n请输出 JSON：{\"summary\":\"...\",\"pros\":[\"...\"],\"cons\":[\"...\"],\"suggestions\":[\"...\"]}。",
		},
		{
			"id":            "report",
			"version":       1,
			"params_preset": "report",
			"output_format": "json",
			"template": "请基于以下工作记录生成日报：\n{log}\n\n输出 JSON：{\"highlights\":[\"...\"],\"blocked\":[\"...\"],\"next\":[\"...\"]}。",
		},
		{
			"id":            "public_event_digest",
			"version":       1,
			"params_preset": "report",
			"output_format": "json",
			"template": "你正在为平台生成公共事件摘要。\n输入事件：\n{events}\n\n输出 JSON：{\"digest\":[{\"title\":\"...\",\"summary\":\"...\",\"tags\":[\"...\"]}]}。每条不超过 60 字，总数不超过 10 条。",
		},
		{
			"id":            "recommend_groups",
			"version":       1,
			"params_preset": "motivation",
			"output_format": "json",
			"template": "你正在基于兴趣标签做小组/圈子推荐。\n输入：\n{agents}\n\n输出 JSON：{\"groups\":[{\"title\":\"...\",\"topic\":\"...\",\"why\":\"...\",\"tags\":[\"...\"]}]}。只给 3-6 个推荐。",
		},
	}

	return map[string]any{
		"kind":           "prompt_bundle",
		"schema_version": 1,
		"agent_id":       agentID,
		"bundle_version": variant.Version,
		"ab_group":       variant.Group,
		"issued_at":      issuedAt,
		"base_prompt":    basePrompt,
		"self_prompt_view": strings.TrimSpace(promptView),
		"params_presets": paramsPresets,
		"scenarios":      scenarios,
		// A content hash can help clients decide if a download is new without parsing the full bundle.
		"bundle_hash": shortHash(agentID + ":" + issuedAt + ":" + fmt.Sprint(variant.Version)),
	}
}
