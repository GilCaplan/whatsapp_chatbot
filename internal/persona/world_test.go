package persona

import (
	"encoding/json"
	"strings"
	"testing"

	"whatsappdoppel/internal/model"
)

func TestNormalizeWorld(t *testing.T) {
	p := model.Persona{Name: "Leo"}
	if err := Normalize(&p); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p.World)
	if !strings.Contains(string(b), `"routine":[]`) {
		t.Errorf("routine must never be null: %s", b)
	}
	p.World = model.World{Timezone: "Not/AZone"}
	if err := Normalize(&p); err == nil {
		t.Error("unknown zone must be an error")
	}
	p.World = model.World{City: " Tel Aviv ", Timezone: "Asia/Jerusalem", Routine: []model.RoutineBlock{{Label: "gym", Days: []string{"sun", "mon"}, From: "7:00", To: "8:00", Reach: "unreachable"}}}
	if err := Normalize(&p); err != nil {
		t.Fatal(err)
	}
	if p.World.City != "Tel Aviv" || p.World.Routine[0].From != "07:00" || p.World.Routine[0].Days[0] != "mon" {
		t.Errorf("world = %+v", p.World)
	}
	p.World.Routine = []model.RoutineBlock{{Label: "x", From: "07:00", To: "99:00"}}
	if err := Normalize(&p); err == nil {
		t.Error("bad time must be an error")
	}
}
