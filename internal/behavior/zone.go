package behavior

import "whatsappdoppel/internal/model"

// FollowsPersona reports whether active hours are set in the persona's time
// zone (Availability.Timezone == model.PersonaZoneMarker).
func FollowsPersona(av model.Availability) bool { return av.Timezone == model.PersonaZoneMarker }

// WithZone substitutes the persona's time zone (World.Timezone; "" = this
// Mac) for the "persona" marker in active hours. Every caller of Available /
// NextOpen that knows the persona applies it first; without it the marker
// falls back to this Mac's zone (Location).
func WithZone(eff Effective, personaTZ string) Effective {
	if FollowsPersona(eff.Profile.Availability) {
		eff.Profile.Availability.Timezone = personaTZ
	}
	return eff
}
