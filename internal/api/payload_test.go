package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

func TestTaskPayloadCarriesArtifacts(t *testing.T) {
	task := domain.Task{
		ID: "task_1", MissionID: "mission_1", Role: domain.RoleBuilder,
		Title: "Execute the plan", State: domain.TaskCompleted,
		Artifacts: []domain.Artifact{{Path: "D:/work/report.md", Description: "file.create"}},
	}
	p := taskPayload(task)
	if len(p.Artifacts) != 1 || p.Artifacts[0].Path != "D:/work/report.md" {
		t.Fatalf("artifacts = %+v", p.Artifacts)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"path":"D:/work/report.md"`) {
		t.Fatalf("wire shape missing the artifact path: %s", raw)
	}
	// A task without artifacts keeps the field out of the wire entirely.
	raw2, err := json.Marshal(taskPayload(domain.Task{ID: "t2"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw2), "artifacts") {
		t.Fatalf("empty artifacts must be omitted: %s", raw2)
	}
}
