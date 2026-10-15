// Package playground defines Qiyan's local, fully synthetic demo and
// training scenarios.
//
// Separation from real control is a data-layer fact, not a label: the types
// here are their own (never internal/domain), the scenarios are served as
// static content, the interaction is simulated in the browser, and nothing in
// this package writes to the state store, the decision trail, or the
// coverage registry. A test asserts the package imports nothing that could.
// Demo events therefore cannot masquerade as observed or enforced events —
// there is no code path that could record them (book §17.1: 模拟事件与真实
// 受控事件在底层数据类型中也必须分离).
package playground

// Action is one synthetic action performed by a synthetic agent.
type Action struct {
	Tool      string   `json:"tool"`
	Kind      string   `json:"kind"`
	Target    string   `json:"target"`
	Effect    string   `json:"effect"` // allowed | blocked | awaits-decision
	ReasonsEN []string `json:"reasonsEn,omitempty"`
	ReasonsZH []string `json:"reasonsZh,omitempty"`
}

// Decision is an interactive branch the player (the user) decides.
type Decision struct {
	QuestionEN    string `json:"questionEn"`
	QuestionZH    string `json:"questionZh"`
	ApproveTextEN string `json:"approveTextEn"`
	ApproveTextZH string `json:"approveTextZh"`
	DenyTextEN    string `json:"denyTextEn"`
	DenyTextZH    string `json:"denyTextZh"`
	// Timeout/Cancel are the passive outcomes training scenario B lets the
	// player try: walking away must also be safe (UX-10).
	TimeoutTextEN string `json:"timeoutTextEn,omitempty"`
	TimeoutTextZH string `json:"timeoutTextZh,omitempty"`
	CancelTextEN  string `json:"cancelTextEn,omitempty"`
	CancelTextZH  string `json:"cancelTextZh,omitempty"`
	// FollowupApprove / FollowupDeny are shown after the choice.
	FollowupApprove *Action `json:"followupApprove,omitempty"`
	FollowupDeny    *Action `json:"followupDeny,omitempty"`
}

// Step is one beat of a scenario.
type Step struct {
	RoleEN   string    `json:"roleEn"`
	RoleZH   string    `json:"roleZh"`
	TextEN   string    `json:"textEn"`
	TextZH   string    `json:"textZh"`
	Action   *Action   `json:"action,omitempty"`
	Decision *Decision `json:"decision,omitempty"`
	// RequiresSetting, when set, hides this step unless the player enabled
	// the named setting (training scenario A: widening the allowed range).
	RequiresSetting string `json:"requiresSetting,omitempty"`
}

// Scenario is a complete playable story. All content is bilingual; the UI
// shows the language the reader selected.
type Scenario struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // "demo" | "training"
	TitleEN string `json:"titleEn"`
	TitleZH string `json:"titleZh"`
	IntroEN string `json:"introEn"`
	IntroZH string `json:"introZh"`
	// Setting is an optional toggle the player may flip before/while playing
	// (training scenario A). Label is shown next to the checkbox.
	SettingID     string `json:"settingId,omitempty"`
	SettingEN     string `json:"settingEn,omitempty"`
	SettingZH     string `json:"settingZh,omitempty"`
	SettingNoteEN string `json:"settingNoteEn,omitempty"`
	SettingNoteZH string `json:"settingNoteZh,omitempty"`
	Steps         []Step `json:"steps"`
}

// All returns every scenario this build ships.
func All() []Scenario {
	return []Scenario{Demo(), TrainingScope(), TrainingApproval()}
}

// Demo is the 60-second no-account walkthrough (UX-02): three agents, one
// normal action, one blocked action, one decision the player makes.
func Demo() Scenario {
	ap := func(tool, kind, target, effect string, en, zh []string) *Action {
		return &Action{Tool: tool, Kind: kind, Target: target, Effect: effect, ReasonsEN: en, ReasonsZH: zh}
	}
	return Scenario{
		ID: "demo", Kind: "demo",
		TitleEN: "Meet your AI crew", TitleZH: "认识你的 AI 小队",
		IntroEN: "Three agents, one small job. Everything you see is synthetic — no external AI is called, nothing on your computer changes.",
		IntroZH: "三个 Agent，一件小事。这里的一切都是合成演示——不会调用外部 AI，也不会改动你的电脑。",
		Steps: []Step{
			{RoleEN: "Planner", RoleZH: "策划员",
				TextEN: "Gets the goal: “write hello.txt and a small report”. It drafts the plan and hands it to the Builder.",
				TextZH: "接到目标：“写一个 hello.txt 和一份小报告”。它拟定计划并交给开发员。"},
			{RoleEN: "Builder", RoleZH: "开发员",
				TextEN: "Creates the file inside the workspace the mission allows.",
				TextZH: "在任务允许的工作区里创建文件。",
				Action: ap("Write", "file.create", "workspace/hello.txt", "allowed",
					[]string{"Inside the folders this task may use."},
					[]string{"位于本任务允许使用的目录内。"})},
			{RoleEN: "Builder", RoleZH: "开发员",
				TextEN: "Now it tries to read a file far outside the workspace.",
				TextZH: "接着它试图读取工作区之外很远的一个文件。",
				Action: ap("Read", "file.read", "C:/somewhere-else/notes.txt", "blocked",
					[]string{
						"What happened: it asked for a file outside the task's folders.",
						"Why it matters: that file was never part of what you allowed.",
						"What Qiyan did: stopped the read before it happened.",
						"What you can do: if the task really needs it, widen the mission's folders and run it again.",
					},
					[]string{
						"发生了什么：它请求读取任务目录之外的文件。",
						"为什么需要注意：那个文件从来不在你允许的范围内。",
						"Qiyan 做了什么：在读取发生之前就阻止了它。",
						"你可以做什么：如果任务确实需要，扩大任务的允许目录后重新运行。",
					})},
			{RoleEN: "Builder", RoleZH: "开发员",
				TextEN: "Finally it wants to send the report somewhere external. This one is your call.",
				TextZH: "最后，它想把报告发送到外部。这一次由你来决定。",
				Action: ap("mail.send", "network.access", "https://example.invalid/report", "awaits-decision", nil, nil),
				Decision: &Decision{
					QuestionEN:    "The Builder wants to send the report outside this computer. Approve this one action?",
					QuestionZH:    "开发员想把报告发送到本机之外。要批准这一个操作吗？",
					ApproveTextEN: "Approved once. In the real product the send would now run exactly as shown, and the decision would be recorded.",
					ApproveTextZH: "已批准一次。在真实产品中，这次发送会按所示内容执行一次，并且决定会被记录下来。",
					DenyTextEN:    "Denied. The Builder continues without sending, and says so in its summary.",
					DenyTextZH:    "已拒绝。开发员会在不发送的情况下继续，并在总结中说明。",
					FollowupApprove: ap("mail.send", "network.access", "https://example.invalid/report", "allowed",
						[]string{"You approved exactly this action, once."},
						[]string{"你批准了这一具体操作，仅此一次。"}),
					FollowupDeny: ap("mail.send", "network.access", "https://example.invalid/report", "blocked",
						[]string{"You denied this action for this task."},
						[]string{"你拒绝了本任务中的这一操作。"}),
				}},
			{RoleEN: "Reviewer", RoleZH: "检查员",
				TextEN: "Checks the result against the goal and reports back.",
				TextZH: "对照目标检查结果并汇报。"},
		},
	}
}

// TrainingScope is training scenario A (UX-10): protect project files. The
// player widens the allowed range and watches the same request change
// outcome.
func TrainingScope() Scenario {
	return Scenario{
		ID: "training-scope", Kind: "training",
		TitleEN: "Training: protect the project's files", TitleZH: "安全训练：保护项目文件",
		IntroEN:   "A builder asks for a harmless synthetic file that lives just outside the project folder. Play it once refused; then widen the allowed range and watch the same request pass.",
		IntroZH:   "开发员想访问一个无害的合成文件，它恰好在项目目录之外。先体验一次被拒绝；然后扩大允许范围，看同一个请求如何通过。",
		SettingID: "allowShared", SettingEN: "Allow the shared folder (../shared)", SettingZH: "允许共享目录（../shared）",
		SettingNoteEN: "This is exactly what “adjusting your authorization” means in the real product — you widen the folder list, nothing else changes.",
		SettingNoteZH: "这正是真实产品中“调整授权”的含义——你扩大目录列表，其余一切不变。",
		Steps: []Step{
			{RoleEN: "Builder", RoleZH: "开发员",
				TextEN: "It asks to write report.txt in the project — allowed.",
				TextZH: "它请求在项目内写 report.txt——被允许。",
				Action: &Action{Tool: "Write", Kind: "file.modify", Target: "project/report.txt", Effect: "allowed",
					ReasonsEN: []string{"Inside the project folder you allowed."},
					ReasonsZH: []string{"位于你允许的项目目录内。"}}},
			{RoleEN: "Builder", RoleZH: "开发员",
				TextEN: "Now it wants to write into the shared folder next to the project.",
				TextZH: "现在它想写入项目旁边的共享目录。",
				Action: &Action{Tool: "Write", Kind: "file.modify", Target: "../shared/notes.txt", Effect: "blocked",
					ReasonsEN: []string{"The shared folder is outside the project you allowed.", "Nothing in that folder was touched."},
					ReasonsZH: []string{"共享目录在你允许的项目之外。", "该目录中的任何内容都未被改动。"}}},
			{RoleEN: "Builder", RoleZH: "开发员",
				TextEN:          "With the shared folder allowed, the same request now passes — and only it.",
				TextZH:          "在你允许共享目录后，同一个请求现在会通过——且仅此一处。",
				RequiresSetting: "allowShared",
				Action: &Action{Tool: "Write", Kind: "file.modify", Target: "../shared/notes.txt", Effect: "allowed",
					ReasonsEN: []string{"You added the shared folder to the mission's allowed range."},
					ReasonsZH: []string{"你已把共享目录加入任务的允许范围。"}}},
		},
	}
}

// TrainingApproval is training scenario B: sensitive-operation authorization.
// The player tries deny, approve, timeout and cancel, and sees each outcome.
func TrainingApproval() Scenario {
	mk := func(effect string, en, zh []string) *Action {
		return &Action{Tool: "tmp.cleanup", Kind: "file.delete", Target: "workspace/tmp-old.txt", Effect: effect, ReasonsEN: en, ReasonsZH: zh}
	}
	return Scenario{
		ID: "training-approval", Kind: "training",
		TitleEN: "Training: decide a sensitive request", TitleZH: "安全训练：决定一次敏感请求",
		IntroEN: "Deleting a scratch file cannot be undone, so it waits for you every time. Try each outcome — nothing real is deleted; the file is synthetic.",
		IntroZH: "删除临时文件无法撤销，所以每次都会先来问你。请逐个体验每种结果——不会真正删除任何东西，文件是合成的。",
		Steps: []Step{
			{RoleEN: "Builder", RoleZH: "开发员",
				TextEN: "It asks to delete the synthetic scratch file.",
				TextZH: "它请求删除合成的临时文件。",
				Action: mk("awaits-decision",
					[]string{"Deletion cannot be undone, so Qiyan always asks first."},
					[]string{"删除无法撤销，所以 Qiyan 总会先询问。"}),
				Decision: &Decision{
					QuestionEN:      "Delete workspace/tmp-old.txt?",
					QuestionZH:      "要删除 workspace/tmp-old.txt 吗？",
					ApproveTextEN:   "Approved once: exactly this deletion would run, once, and the record shows your decision.",
					ApproveTextZH:   "已批准一次：只会执行这一次删除，记录中会写明是你的决定。",
					DenyTextEN:      "Denied: the file stays; the agent is told to continue without deleting.",
					DenyTextZH:      "已拒绝：文件保留；Agent 会被告知在不删除的情况下继续。",
					TimeoutTextEN:   "Timed out: nobody answered in time, and the request expired without running. Silence is never a yes.",
					TimeoutTextZH:   "已超时：长时间无人回应，请求自动过期且从未执行。沉默从来不是同意。",
					CancelTextEN:    "Cancelled: the request was withdrawn (for example, the task ended) and can never be approved afterwards.",
					CancelTextZH:    "已取消：请求被撤回（例如任务结束），此后永远不能被批准。",
					FollowupApprove: mk("allowed", []string{"One approved execution — not a standing permission."}, []string{"一次获准的执行——不是长期权限。"}),
					FollowupDeny:    mk("blocked", []string{"Your denial holds for this action in this task."}, []string{"你的拒绝在本任务中对这一操作持续有效。"}),
				}},
			{RoleEN: "Qiyan", RoleZH: "Qiyan",
				TextEN: "If you walk away, the request expires on its own (timeout), or the task is cancelled — either way it never runs by default.",
				TextZH: "如果你离开，请求会自行过期（超时），或随任务取消——无论如何它都不会默认执行。",
				Action: mk("blocked",
					[]string{"Expired and cancelled approvals are recorded as such, and are never replayed."},
					[]string{"过期与已取消的批准会被如实记录，且永远不会被重放。"})},
		},
	}
}
