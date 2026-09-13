package main

import (
	"fmt"
	"strings"
)

// Nombre insère une espace fine insécable comme séparateur de milliers.
// « 1270476 » n'est pas lisible ; « 1 270 476 » l'est.
func Nombre(n any) string {
	s := fmt.Sprint(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString("\u202f")
		}
		b.WriteRune(r)
	}
	if neg {
		return "\u2212" + b.String()
	}
	return b.String()
}

var sansAccent = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ã", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "í", "i",
	"ô", "o", "ö", "o", "ó", "o", "õ", "o", "ø", "o",
	"ù", "u", "û", "u", "ü", "u", "ú", "u",
	"ç", "c", "ñ", "n", "ÿ", "y", "æ", "ae", "œ", "oe",
)

// CleTri produit une clé de tri insensible à la casse et aux accents.
// Sans elle, « Les Écologistes » se classe après « Les Républicains », parce
// que « É » vient après « R » dans l'ordre des octets — un tri alphabétique
// faux est plus déroutant qu'une absence de tri.
func CleTri(s string) string {
	return sansAccent.Replace(strings.ToLower(strings.TrimSpace(s)))
}

// Nombre64 : même formatage que Nombre, pour les sommes qui débordent l'int.
func Nombre64(n int64) string { return Nombre(int(n)) }
