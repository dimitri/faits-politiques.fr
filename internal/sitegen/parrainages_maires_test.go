package sitegen

import "testing"

func TestPrettyCandidate(t *testing.T) {
	cas := map[string]string{
		"MÉLENCHON Jean-Luc":    "Jean-Luc Mélenchon",
		"LE PEN Marine":         "Marine Le Pen",
		"DUPONT-AIGNAN Nicolas": "Nicolas Dupont-Aignan",
		"ALLIOT-MARIE Michèle":  "Michèle Alliot-Marie",
		"ARTHAUD Nathalie":      "Nathalie Arthaud",
	}
	for in, want := range cas {
		if got := prettyCandidate(in); got != want {
			t.Errorf("prettyCandidate(%q) = %q, attendu %q", in, got, want)
		}
	}
}
