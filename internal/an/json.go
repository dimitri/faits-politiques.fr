package an

import (
	"encoding/json"
	"strings"
)

// L'open data de l'Assemblée nationale sérialise un élément unique comme un
// objet et plusieurs éléments comme un tableau, pour le même champ. Ce
// comportement vient de la conversion XML -> JSON et concerne mandats, groupes,
// votants, organes. Toute lecture passe donc par asSlice.
func asSlice(raw json.RawMessage) []json.RawMessage {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil
	}
	if s[0] == '[' {
		var out []json.RawMessage
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil
		}
		// Les tableaux de l'AN contiennent parfois des éléments nuls.
		kept := out[:0]
		for _, e := range out {
			if strings.TrimSpace(string(e)) != "null" {
				kept = append(kept, e)
			}
		}
		return kept
	}
	return []json.RawMessage{raw}
}

// Un champ typé côté XML est sérialisé en objet {"@xsi:type":…,"#text":…}
// plutôt qu'en chaîne. C'est le cas de acteur.uid, de organeRef et de
// acteurRef, alors que organe.uid reste une chaîne simple. Toute lecture de
// valeur scalaire passe donc par str.
func str(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if s[0] == '"' {
		var v string
		if err := json.Unmarshal(raw, &v); err == nil {
			return v
		}
		return ""
	}
	if s[0] == '{' {
		var box struct {
			Text string `json:"#text"`
		}
		if err := json.Unmarshal(raw, &box); err == nil {
			return box.Text
		}
		return ""
	}
	return strings.Trim(s, `"`)
}

// nullable transforme la chaîne vide en NULL SQL : une date absente ne doit pas
// devenir une date vide.
func nullable(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

var slugRepl = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ã", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "í", "i",
	"ô", "o", "ö", "o", "ó", "o", "õ", "o", "ø", "o",
	"ù", "u", "û", "u", "ü", "u", "ú", "u",
	"ç", "c", "ñ", "n", "ÿ", "y", "æ", "ae", "œ", "oe", "ß", "ss",
	"'", "-", "’", "-", " ", "-", ".", "", "\"", "",
)

func slugify(parts ...string) string {
	s := strings.ToLower(strings.Join(parts, "-"))
	s = slugRepl.Replace(s)
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
