package main

import (
	"fmt"
	"strings"
	"time"
	"unicode"
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

// Decimal formate un nombre à décimales à la française : virgule décimale, et
// espace fine insécable pour les milliers. « 1234.5 » devient « 1 234,5 ».
//
// Ce n'est pas un détail de goût. Dans une page française, « 3.8 ‰ » se lit
// comme trois mille huit cents pour mille par quelqu'un qui parcourt vite.
func Decimal(v float64, dec int) string {
	s := fmt.Sprintf("%.*f", dec, v)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	ent, frac, _ := strings.Cut(s, ".")
	out := Nombre(ent)
	if frac != "" {
		out += "," + frac
	}
	if neg {
		return "−" + out
	}
	return out
}

// eurHab : un montant par habitant. Les collectivités s'y comparent à l'euro
// près, mais un rond de plus serait une fausse précision — la source arrondit
// déjà. Une valeur absente n'est pas zéro, et s'affiche comme absente.
func eurHab(v float64) string {
	if v == 0 {
		return "—"
	}
	return Nombre(int(v+0.5)) + " €"
}

// Montant : des milliards quand il y en a, des millions sinon. « 0,0 Md€ »
// pour l'épargne brute d'un département est une fausse précision qui ressemble
// à zéro ; « 35 M€ » dit la même chose et se lit.
func Montant(v float64) string {
	if v >= 1e9 || v <= -1e9 {
		return Decimal(v/1e9, 1) + " Md€"
	}
	if v >= 1e6 || v <= -1e6 {
		return Decimal(v/1e6, 0) + " M€"
	}
	return Nombre(int(v)) + " €"
}

// Octets : une taille de fichier ou de table, en unités binaires (Ko = 1024
// octets) — la convention déjà suivie par pg_size_pretty, pour ne pas
// afficher un nombre qui ne correspondrait à aucune des deux mesures.
func Octets(n int64) string {
	const unite = 1024
	unites := [...]string{"Ko", "Mo", "Go", "To"}
	if n < unite {
		return Nombre(int(n)) + " octets"
	}
	div, exp := int64(unite), 0
	for v := n / unite; v >= unite && exp < len(unites)-1; v /= unite {
		div *= unite
		exp++
	}
	return Decimal(float64(n)/float64(div), 1) + " " + unites[exp]
}

var moisLong = [...]string{"", "janvier", "février", "mars", "avril", "mai", "juin",
	"juillet", "août", "septembre", "octobre", "novembre", "décembre"}

// dateFr : « 13 septembre 2026 à 23:24 ». Le format Go de référence rend les
// mois en anglais, et « 13 September 2026 » dans un pied de page français est
// le genre de détail qui fait douter du reste.
func dateFr(t time.Time) string {
	return fmt.Sprintf("%d %s %d à %02d:%02d", t.Day(), moisLong[int(t.Month())],
		t.Year(), t.Hour(), t.Minute())
}

// dateJourFr : « 5 juillet 1962 », sans heure — pour les dates historiques.
func dateJourFr(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), moisLong[int(t.Month())], t.Year())
}

// NomPropre : « Emmanuel MACRON » devient « Emmanuel Macron ».
//
// Le Journal officiel et les proclamations du Conseil constitutionnel écrivent
// le patronyme en capitales ; les fiches, en casse ordinaire. Juxtaposés dans
// un même tableau, « Emmanuel MACRON » en 2022 et « Emmanuel Macron » en 2017
// laissent croire à deux personnes. La correction est d'AFFICHAGE seulement :
// la base garde la graphie de la source, qui est aussi celle qui permet de
// séparer le prénom du nom.
//
// Seuls les mots entièrement en capitales sont touchés — « de », « Le » ou
// « McCain » restent tels quels — et chaque segment d'un nom composé l'est
// séparément : « LE PEN » → « Le Pen », « GALLIARD-MINIER » → « Galliard-Minier ».
func NomPropre(s string) string {
	mots := strings.Fields(s)
	for i, m := range mots {
		mots[i] = casseMot(m)
	}
	return strings.Join(mots, " ")
}

func casseMot(m string) string {
	r := []rune(m)
	lettres, majuscules := 0, 0
	for _, c := range r {
		if unicode.IsLetter(c) {
			lettres++
			if unicode.IsUpper(c) {
				majuscules++
			}
		}
	}
	if lettres < 2 || lettres != majuscules {
		return m
	}
	debut := true
	for i, c := range r {
		if !unicode.IsLetter(c) {
			debut = c == '-' || c == '\'' || c == '’'
			continue
		}
		if debut {
			r[i] = unicode.ToUpper(c)
		} else {
			r[i] = unicode.ToLower(c)
		}
		debut = false
	}
	return string(r)
}
