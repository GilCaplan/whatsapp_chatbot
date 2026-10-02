package behavior

import (
	"reflect"

	"whatsappdoppel/internal/model"
)

// Settings v4 tweaked three preset numbers so every preset lands exactly on
// the vibe dials (Slow: distracted 25% → 35%; Busy: reactions 5/25 → 0/15;
// Slow: reactions 0/10 → 0/15). A profile or chat that still holds a whole
// pre-v4 Busy/Slow preset keeps its label: it is moved to the new numbers
// once. Anything customised stays as it is (and reads as "custom").

// legacyTweaks are the pre-v4 values of the tweaked fields, per preset and kind.
var legacyTweaks = map[string]map[string]map[string]int{
	PresetBusy: {
		KindDM:    {"reactPercent": 5},
		KindGroup: {"reactPercent": 25},
	},
	PresetSlow: {
		KindDM:    {"reactPercent": 0, "distractedPercent": 25},
		KindGroup: {"reactPercent": 10, "distractedPercent": 25},
	},
}

// LegacyPreset returns a preset as it was before settings v4 (ok false for
// presets that did not change).
func LegacyPreset(id, kind string) (model.BehaviorProfile, bool) {
	tw, ok := legacyTweaks[id]
	if !ok {
		return model.BehaviorProfile{}, false
	}
	d, _ := Preset(id)
	p := d.For(kind)
	for name, v := range tw[normalKind(kind)] {
		*intPtr(&p, name) = v
	}
	return p, true
}

func normalKind(kind string) string {
	if kind == KindGroup {
		return KindGroup
	}
	return KindDM
}

// RelabelLegacyPreset moves a profile labelled Busy/Slow that still equals
// the pre-v4 preset onto the current preset values. Reports whether it changed.
func RelabelLegacyPreset(p *model.BehaviorProfile, kind string) bool {
	old, ok := LegacyPreset(p.Preset, kind)
	if !ok || !Equal(*p, old) {
		return false
	}
	d, _ := Preset(p.Preset)
	label := p.Preset
	*p = d.For(kind)
	p.Preset = label
	return true
}

// RelabelLegacyOverrides does the same for a chat that applied a whole
// pre-v4 Busy/Slow preset: when every tweaked field is overridden with its
// old value, those fields get the new values. Reports whether it changed.
func RelabelLegacyOverrides(o *model.BehaviorOverrides, kind string) bool {
	tw, ok := legacyTweaks[o.Preset]
	if !ok {
		return false
	}
	old := tw[normalKind(kind)]
	ov := reflect.ValueOf(o).Elem()
	for name, v := range old {
		f, _ := fieldByName(name)
		fv := ov.Field(f.oIdx)
		if fv.IsNil() || int(fv.Elem().Int()) != v {
			return false
		}
	}
	d, _ := Preset(o.Preset)
	cur := d.For(kind)
	for name := range old {
		f, _ := fieldByName(name)
		n := *intPtr(&cur, name)
		ov.Field(f.oIdx).Set(reflect.ValueOf(&n))
	}
	return true
}
