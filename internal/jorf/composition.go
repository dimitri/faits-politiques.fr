package jorf

import (
	"regexp"
	"strings"
)

// Lecture des décrets de composition du Gouvernement.
//
// La prose est réglée, parce que c'est du droit. Quatre formes couvrent tout ce
// qui a été observé de 1990 à 2026 :
//
//	Sont nommés ministres : M. Laurent NUNEZ, ministre de l'intérieur ; …
//	Mme Catherine PÉGARD est nommée ministre de la culture.
//	Il est mis fin aux fonctions de : Mme Rachida DATI, ministre de la culture ; …
//	Sont nommés ministres délégués … : - auprès du ministre de l'intérieur : M. X ; …
//
// DEUX VARIATIONS DE MISE EN PAGE, qu'il a fallu découvrir :
//
//  1. l'en-tête de section est tantôt suivi des noms sur la même ligne, tantôt
//     seul sur la sienne, les noms venant ensuite. La lecture est donc un
//     automate à état — la fonction en cours vaut jusqu'au prochain en-tête —
//     et non un découpage ligne à ligne, qui perdait la fonction de tout le
//     monde dès que la source retournait à la ligne ;
//  2. le patronyme est en CAPITALES jusqu'en 2022 environ, puis en casse
//     ordinaire : « M. Gérard COLLOMB » en 2017, « M. Jean-Louis Thiériot » en
//     2024. Une règle fondée sur les seules capitales lisait le premier et pas
//     le second. La découpe prénom / nom essaie donc les capitales d'abord, et
//     retombe sur « le dernier mot est le patronyme » quand il n'y en a pas.

const (
	FonctionPremierMinistre = "PREMIER_MINISTRE"
	FonctionMinistreEtat    = "MINISTRE_ETAT"
	FonctionMinistre        = "MINISTRE"
	FonctionMinistreDelegue = "MINISTRE_DELEGUE"
	FonctionSecretaireEtat  = "SECRETAIRE_ETAT"
	FonctionHautCommissaire = "HAUT_COMMISSAIRE"
)

const (
	SensNomination = "NOMINATION"
	SensCessation  = "CESSATION"
)

type Membre struct {
	Rang         int
	Sens         string
	Fonction     string
	Civilite     string
	Prenom       string
	Nom          string
	Rattachement string // « ministre de l'intérieur », pour un délégué
	Portefeuille string // « ministre de la culture », « chargée de l'autonomie »
}

var (
	// Le nom court de la civilité jusqu'au premier séparateur. Il n'est PAS
	// découpé ici : la découpe prénom / nom demande de regarder la casse, ce
	// qu'une expression régulière ne sait pas faire proprement quand la
	// convention change d'une année à l'autre.
	reCivilite = regexp.MustCompile(`(M\.|Mme|MM\.|Mmes)\s+([^,;:.]{2,90}?)(?:\s+est\s+nomm|\s*[,;:]|\s*\.|$)`)

	// Les en-têtes de section. L'ordre compte : « ministres délégués » doit
	// être essayé avant « ministres », faute de quoi tout le monde est ministre.
	reSectionMinistresDelegues = regexp.MustCompile(`(?i)sont nomm[ée]s?\s+ministres?\s+d[ée]l[ée]gu`)
	reSectionSecretaires       = regexp.MustCompile(`(?i)sont nomm[ée]s?\s+secr[ée]taires?\s+d['’]?\s?[EÉée]tat`)
	reSectionMinistresEtat     = regexp.MustCompile(`(?i)sont nomm[ée]s?\s+ministres?\s+d['’]?\s?[EÉé]tat`)
	reSectionMinistres         = regexp.MustCompile(`(?i)sont nomm[ée]s?\s+ministres`)
	reSectionHautCommissaire   = regexp.MustCompile(`(?i)sont nomm[ée]s?\s+hauts?[- ]commissaires?`)
	reCessation                = regexp.MustCompile(`(?i)il est mis fin aux fonctions`)

	// « - auprès du ministre de l'intérieur : » — le rattachement d'un délégué.
	// Il vaut jusqu'au rattachement suivant, y compris par-dessus un retour à
	// la ligne.
	//
	// Le motif n'est PAS ancré en début de ligne : la base complète met un
	// rattachement par ligne, mais les livraisons quotidiennes en alignent
	// plusieurs sur la même, séparés par des tirets. Ancré, il ne voyait que le
	// premier et rattachait tous les délégués suivants au mauvais ministre.
	reRattachement = regexp.MustCompile(
		`(?i)[-–—]?\s*aupr[èe]s\s+(?:du|de la|de l['’]|des|de)\s*([^:;]{2,200}?)\s*:`)

	reBlancsComp = regexp.MustCompile(`\s+`)

	// Les particules qui appartiennent au patronyme et le précèdent.
	particules = map[string]bool{
		"de": true, "du": true, "des": true, "le": true, "la": true, "les": true,
		"van": true, "von": true, "di": true, "da": true, "dos": true, "el": true,
	}
)

// LireComposition rend les membres cités par un décret, dans l'ordre du texte —
// qui est l'ordre protocolaire.
func LireComposition(contenu string) []Membre {
	var out []Membre
	rang := 0
	fonction, sens, rattachement := "", SensNomination, ""

	for _, ligne := range strings.Split(contenu, "\n") {
		ligne = strings.TrimSpace(ligne)
		if ligne == "" {
			continue
		}

		// Un en-tête de section change la fonction en cours, et remet le
		// rattachement à zéro : un nouveau bloc ne rattache plus au précédent.
		if f, s, reste, ok := entete(ligne); ok {
			fonction, sens, rattachement = f, s, ""
			ligne = reste
			if strings.TrimSpace(ligne) == "" {
				continue
			}
		}

		// La ligne est découpée aux rattachements : chaque segment hérite du
		// rattachement qui l'ouvre, et le premier garde celui de la ligne
		// précédente.
		for _, seg := range segmenter(ligne, &rattachement) {
			for _, bloc := range strings.Split(seg.texte, ";") {
				m := reCivilite.FindStringSubmatchIndex(bloc)
				if m == nil {
					continue
				}
				prenom, nom := decouperNom(bloc[m[4]:m[5]])
				if nom == "" {
					continue
				}
				reste := strings.TrimLeft(strings.TrimSpace(bloc[m[5]:]), " ,;:.")
				reste = strings.TrimPrefix(reste, "est nommé ")
				reste = strings.TrimPrefix(reste, "est nommée ")

				// La fonction vient de la section en cours ; à défaut, du
				// libellé qui suit le nom — c'est le cas des formes
				// individuelles.
				f := fonction
				if g := fonctionDePortefeuille(reste); g != "" && (f == "" || g == FonctionPremierMinistre) {
					f = g
				}
				if f == "" {
					continue // ni section ni portefeuille : pas une nomination
				}
				rang++
				out = append(out, Membre{
					Rang: rang, Sens: sens, Fonction: f,
					Civilite:     bloc[m[2]:m[3]],
					Prenom:       prenom,
					Nom:          nom,
					Rattachement: seg.rattachement,
					Portefeuille: normaliser(strings.TrimRight(reste, " .;,")),
				})
			}
		}
	}
	return out
}

type segment struct{ rattachement, texte string }

// segmenter découpe une ligne aux rattachements qu'elle contient. Le pointeur
// porte le rattachement courant d'une ligne à l'autre : la base complète ouvre
// un rattachement sur une ligne et donne les noms sur les suivantes.
func segmenter(ligne string, courant *string) []segment {
	ms := reRattachement.FindAllStringSubmatchIndex(ligne, -1)
	if len(ms) == 0 {
		return []segment{{rattachement: *courant, texte: ligne}}
	}
	var out []segment
	if tete := strings.TrimSpace(ligne[:ms[0][0]]); tete != "" {
		out = append(out, segment{rattachement: *courant, texte: tete})
	}
	for i, m := range ms {
		*courant = normaliser(ligne[m[2]:m[3]])
		fin := len(ligne)
		if i+1 < len(ms) {
			fin = ms[i+1][0]
		}
		out = append(out, segment{rattachement: *courant, texte: ligne[m[1]:fin]})
	}
	return out
}

// entete reconnaît une formule d'ouverture et rend ce qui la suit. Le second
// retour dit si la ligne en était une : une ligne qui n'est pas un en-tête est
// traitée avec la fonction héritée de l'en-tête précédent.
func entete(ligne string) (fonction, sens, reste string, ok bool) {
	i := strings.Index(ligne, ":")
	tete := ligne
	if i > 0 && i < 300 {
		tete = ligne[:i]
	}
	switch {
	case reCessation.MatchString(tete):
		fonction, sens = "", SensCessation
	case reSectionMinistresDelegues.MatchString(tete):
		fonction, sens = FonctionMinistreDelegue, SensNomination
	case reSectionSecretaires.MatchString(tete):
		fonction, sens = FonctionSecretaireEtat, SensNomination
	case reSectionMinistresEtat.MatchString(tete):
		fonction, sens = FonctionMinistreEtat, SensNomination
	case reSectionHautCommissaire.MatchString(tete):
		fonction, sens = FonctionHautCommissaire, SensNomination
	case reSectionMinistres.MatchString(tete):
		fonction, sens = FonctionMinistre, SensNomination
	default:
		return "", "", ligne, false
	}
	// Une cessation ne dit pas la fonction dans son en-tête : elle la donne
	// après chaque nom. La fonction reste donc vide et sera déduite du libellé.
	//
	// Sans deux-points, la formule et le nom sont sur la même ligne :
	// « Il est mis fin aux fonctions de M. Jean-Pierre Soisson, ministre
	// d'Etat ». Rendre une suite vide dans ce cas faisait disparaître la ligne
	// entière — trois décrets de cessation ressortaient muets.
	if i > 0 && i < 300 {
		return fonction, sens, ligne[i+1:], true
	}
	return fonction, sens, ligne, true
}

// decouperNom sépare le prénom du patronyme.
//
// Deux conventions coexistent dans la même source, selon l'année :
//
//	« Gérard COLLOMB »        patronyme en capitales   (jusque vers 2022)
//	« Jean-Louis Thiériot »   casse ordinaire          (à partir de 2024)
//
// La première est sans ambiguïté et sert donc d'abord : le patronyme commence au
// premier mot entièrement en capitales, particule éventuelle comprise — ce qui
// donne « de MONTCHALIN », « Le HÉNANFF », « LE DRIAN ». Faute de capitales, la
// règle de repli est que le dernier mot est le patronyme, particules incluses.
func decouperNom(s string) (prenom, nom string) {
	mots := strings.Fields(normaliser(s))
	if len(mots) == 0 {
		return "", ""
	}
	if len(mots) == 1 {
		return "", mots[0]
	}

	debut := -1
	for i, m := range mots {
		if estCapitales(m) {
			debut = i
			break
		}
	}
	if debut < 0 {
		// Pas de capitales : le patronyme est le dernier mot.
		debut = len(mots) - 1
	}
	// Les particules qui précèdent immédiatement appartiennent au patronyme.
	for debut > 0 && particules[strings.ToLower(strings.Trim(mots[debut-1], "'’"))] {
		debut--
	}
	if debut == 0 {
		// Tout est patronyme : le décret n'a pas donné de prénom.
		return "", strings.Join(mots, " ")
	}
	return strings.Join(mots[:debut], " "), strings.Join(mots[debut:], " ")
}

// estCapitales dit si un mot est écrit entièrement en majuscules. Il faut au
// moins deux lettres : « M » ou une initiale isolée ne sont pas un patronyme.
func estCapitales(m string) bool {
	lettres := 0
	for _, r := range m {
		switch {
		case r >= 'a' && r <= 'z':
			return false
		case r >= 'à' && r <= 'ÿ':
			return false
		case r >= 'A' && r <= 'Z', r >= 'À' && r <= 'Þ':
			lettres++
		}
	}
	return lettres >= 2
}

// fonctionDePortefeuille déduit la fonction du libellé qui suit le nom, pour les
// formes qui n'ont pas d'en-tête de section.
func fonctionDePortefeuille(s string) string {
	b := strings.ToLower(normaliser(s))
	// « Etat » sans accent existe dans les décrets d'avant 2020 ; les deux
	// graphies sont donc acceptées partout.
	b = strings.NewReplacer("d'etat", "d'état", "d’etat", "d'état", "d’état", "d'état").Replace(b)
	switch {
	case strings.HasPrefix(b, "premier ministre"), strings.HasPrefix(b, "première ministre"):
		return FonctionPremierMinistre
	case strings.Contains(b, "ministre délégué"), strings.Contains(b, "ministre déléguée"):
		return FonctionMinistreDelegue
	case strings.Contains(b, "secrétaire d'état"):
		return FonctionSecretaireEtat
	case strings.Contains(b, "haut-commissaire"), strings.Contains(b, "haute-commissaire"):
		return FonctionHautCommissaire
	case strings.HasPrefix(b, "ministre d'état"):
		return FonctionMinistreEtat
	case strings.Contains(b, "ministre"), strings.Contains(b, "garde des sceaux"):
		return FonctionMinistre
	}
	return ""
}

func normaliser(s string) string {
	return strings.TrimSpace(reBlancsComp.ReplaceAllString(s, " "))
}
