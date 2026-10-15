package main

import (
	"sort"
	"strings"
)

// Localized rendering of engine reason codes.
//
// The engine speaks in stable codes (path_outside_scope, sensitive_command,
// …); this table turns them into the plain-language sentences a person reads.
// Both languages describe the same fact — neither softens or overstates what
// the other says (UX-06, UX-13). Placeholders are {name} and are substituted
// from the reason's structured parameters, so wording varies per language
// while facts never do.

type localizedText struct {
	en string
	zh string
}

var reasonTexts = map[string]localizedText{
	"path_outside_scope": {
		en: "It wanted to touch {path}, which is outside the folders this task is allowed to use.",
		zh: "它想访问 {path}，该位置不在本任务允许使用的目录范围内。",
	},
	"tool_not_in_scope": {
		en: "This kind of action ({kind}{tool}) is not part of what you allowed for this task.",
		zh: "此类操作（{kind}{tool}）不在你为本任务授权的范围内。",
	},
	"write_by_readonly": {
		en: "This agent is read-only, so it cannot make changes ({kind}).",
		zh: "该角色为只读，不能执行修改类操作（{kind}）。",
	},
	"delete_operation": {
		en: "Deleting {path} cannot be undone, so it always needs your decision first.",
		zh: "删除 {path} 无法撤销，因此必须先经过你的确认。",
	},
	"credential_reference": {
		en: "It wanted to use a credential ({credential}); credentials always need your explicit approval.",
		zh: "它想使用凭据（{credential}）；凭据操作必须经你明确批准。",
	},
	"external_network": {
		en: "It wanted to reach {target} over the network; outbound access needs your explicit approval.",
		zh: "它想通过网络访问 {target}；对外访问必须经你明确批准。",
	},
	"external_state_change": {
		en: "It wanted to change something outside this computer ({target}); that needs your explicit approval.",
		zh: "它想更改本机之外的对象（{target}）；这需要你明确批准。",
	},
	"unrecognized_command": {
		en: "The command or tool ({program}{kind}) could not be safely interpreted, so it is not allowed to run without your review.",
		zh: "无法可靠解释该命令或工具（{program}{kind}），未经你审查不会执行。",
	},
	"sensitive_command": {
		en: "Shell commands always wait for your approval ({program}).",
		zh: "Shell 命令一律等待你的批准（{program}）。",
	},
	"mode_companion": {
		en: "You are in accompany mode, where changes wait for your confirmation.",
		zh: "当前为陪同模式，修改类操作会等待你的确认。",
	},
	"budget_exhausted": {
		en: "This task has used up its allowed budget ({budget}).",
		zh: "本任务的授权额度（{budget}）已用完。",
	},
	"task_cancelled": {
		en: "This task was cancelled, so no further actions will run.",
		zh: "任务已取消，不再执行新的操作。",
	},
	"user_denied": {
		en: "You already denied this exact action for this task.",
		zh: "你已拒绝过该任务中的这一操作。",
	},
	"approval_expired": {
		en: "The previous approval expired; a fresh decision is needed.",
		zh: "先前的批准已过期，需要重新确认。",
	},
	"approval_cancelled": {
		en: "This request was cancelled and can no longer be approved.",
		zh: "该请求已被取消，不能再被批准。",
	},
	"authorization_replayed": {
		en: "This approval was already used once; each approval covers exactly one execution.",
		zh: "该批准已使用过一次；每次批准只对应一次执行。",
	},
	"action_changed": {
		en: "What is about to run differs from what was approved, so the old approval does not apply.",
		zh: "即将执行的内容与已批准的内容不一致，原批准不适用。",
	},
	"identity_unknown": {
		en: "The identity behind this action could not be confirmed.",
		zh: "无法确认该操作背后的身份。",
	},
	"delegation_escalation": {
		en: "This hand-off would give another agent more authority than the task allows, so it was stopped.",
		zh: "该交接会让另一个 Agent 获得超出任务允许的权限，已被阻止。",
	},
	"control_unavailable": {
		en: "The control service could not be reached or could not decide.",
		zh: "控制服务无法访问或无法作出决策。",
	},
	"no_external_effect": {
		en: "Session bookkeeping only — no effect outside the agent; not a protected action.",
		zh: "仅为会话内部记录，不影响外部；不属于受保护操作。",
	},
	"path_not_covered": {
		en: "This action runs on a path Qiyan cannot enforce, so it is not covered by protection.",
		zh: "该操作位于 Qiyan 无法强制控制的路径上，不在保护范围内。",
	},
	"user_approved": {
		en: "You approved this exact action.",
		zh: "你已批准这一具体操作。",
	},
}

// renderReasons renders up to three reasons as bilingual lines. Unknown codes
// render as themselves rather than being dropped: an unexplained refusal must
// still be visible.
func renderReasons(reasons []struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params"`
}) string {
	if len(reasons) == 0 {
		return "  (no further detail was provided)\n  （未提供更多细节）"
	}
	var lines []string
	seen := map[string]bool{}
	for _, r := range reasons {
		if seen[r.Code] || len(lines) >= 6 {
			continue
		}
		seen[r.Code] = true
		if t, ok := reasonTexts[r.Code]; ok {
			lines = append(lines, "  "+substitute(t.en, r.Params), "  "+substitute(t.zh, r.Params))
		} else {
			lines = append(lines, "  ("+r.Code+")")
		}
	}
	return strings.Join(lines, "\n")
}

// substitute replaces {name} placeholders from params, then removes any
// placeholder left without a parameter (including now-empty brackets) so a
// sentence never shows a dangling brace.
func substitute(text string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		text = strings.ReplaceAll(text, "{"+k+"}", params[k])
	}
	for {
		i := strings.IndexByte(text, '{')
		j := strings.IndexByte(text, '}')
		if i < 0 || j < i {
			break
		}
		text = text[:i] + text[j+1:]
	}
	text = strings.ReplaceAll(text, "()", "")
	text = strings.ReplaceAll(text, "（）", "")
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	return strings.TrimSpace(text)
}
