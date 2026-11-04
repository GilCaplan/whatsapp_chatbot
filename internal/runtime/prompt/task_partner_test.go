package prompt

import (
	"testing"

	"whatsappdoppel/internal/logic/task"
)

func TestPartnerStylesMatchTask(t *testing.T) {
	if len(PartnerStyles) != len(task.PartnerStyles) {
		t.Fatal("prompt.PartnerStyles and task.PartnerStyles must list the same styles")
	}
	for _, s := range task.PartnerStyles {
		if _, ok := PartnerStyles[s]; !ok {
			t.Fatalf("style %q has no wording", s)
		}
	}
}
