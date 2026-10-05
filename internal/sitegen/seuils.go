package sitegen

import (
	"encoding/csv"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Un décompte sans son seuil produit un contresens. « 197 pour · 0 contre →
// rejeté » n'est lisible que si la page dit qu'il en fallait 289 : le seuil est
// un FAIT constitutionnel, pas une interprétation.
//
// Il vit dans data/ parce qu'il relève d'un choix — quel seuil s'applique à
// quel type de scrutin, et sur quelle base — et que tout choix de ce site doit
// être une diff relisible, soumise à relecture contradictoire.
type Seuil struct {
	TypeVote, Regle, Note, Source, Consultee string
	Base, Voix                               int
}

func loadSeuils(path string) (map[string]Seuil, error) {
	f, err := os.Open(path)
	if err != nil {
		// Un seuil manquant n'est pas une erreur de construction : la page
		// retombe sur la barre proportionnelle. Une absence s'affiche.
		if os.IsNotExist(err) {
			return map[string]Seuil{}, nil
		}
		return nil, err
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	out := map[string]Seuil{}
	for i, r := range rows {
		if i == 0 || len(r) < 7 {
			continue
		}
		base, _ := strconv.Atoi(strings.TrimSpace(r[1]))
		voix, _ := strconv.Atoi(strings.TrimSpace(r[2]))
		if base <= 0 || voix <= 0 {
			continue
		}
		out[strings.TrimSpace(r[0])] = Seuil{
			TypeVote: strings.TrimSpace(r[0]), Base: base, Voix: voix,
			Regle: r[3], Note: r[4], Source: r[5], Consultee: r[6],
		}
	}
	return out, nil
}

// coupures : les amorces de la liste des signataires. Couper avant elles est
// une opération mécanique et réversible — le libellé officiel reste affiché
// intégralement juste en dessous.
var coupures = []string{" par M. ", " par Mme ", " par MM. ", " par Mmes "}

// TitreCourt rend le libellé source lisible comme un titre SANS le réécrire.
// Deux opérations seulement : couper avant les signataires, et mettre la
// première lettre en capitale. Aucun mot ajouté, aucun mot remplacé.
//
// Le libellé brut commence par une minuscule et court parfois sur neuf lignes :
// fidèle, mais illisible en h1, surtout à 390 px.
func TitreCourt(objet string) (string, bool) {
	t := strings.TrimSpace(objet)
	orig := t
	t = strings.TrimRight(t, ".")

	for _, c := range coupures {
		if i := strings.Index(t, c); i > 40 {
			t = t[:i]
			break
		}
	}
	if utf8.RuneCountInString(t) > 120 {
		r := []rune(t)[:120]
		if j := strings.LastIndex(string(r), " "); j > 60 {
			t = string(r)[:j]
		} else {
			t = string(r)
		}
		// Une parenthèse ouverte et jamais refermée se lit comme un défaut :
		// on coupe avant elle plutôt que de laisser « (première… ».
		if strings.Count(t, "(") > strings.Count(t, ")") {
			if k := strings.LastIndex(t, "("); k > 60 {
				t = t[:k]
			}
		}
		t = strings.TrimRight(t, " ,;:(") + "…"
	}
	if r, n := utf8.DecodeRuneInString(t); n > 0 {
		t = string(unicode.ToUpper(r)) + t[n:]
	}
	return t, t != orig
}

// ResultatLong accorde le résultat avec l'objet du vote. Une motion est
// féminine ; un projet de loi ne l'est pas. Le résultat reste en ENCRE :
// « adopté » n'est pas une position de vote, et le colorer en vert reviendrait
// à dire qu'adopter est bien.
func ResultatLong(resultat, typeVote string) string {
	if resultat == "" {
		return "Résultat non publié"
	}
	feminin := strings.HasPrefix(typeVote, "motion")
	switch resultat {
	case "adopté":
		if feminin {
			return "Adoptée"
		}
		return "Adopté"
	case "rejeté":
		if feminin {
			return "Rejetée"
		}
		return "Rejeté"
	}
	if r, n := utf8.DecodeRuneInString(resultat); n > 0 {
		return string(unicode.ToUpper(r)) + resultat[n:]
	}
	return resultat
}
