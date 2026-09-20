package sitegen

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Le budget de l'État et celui de la Sécurité sociale, à partir des deux seules
// séries que ce site ait ingérées en propre :
//
//	DGFiP   situations mensuelles du budget de l'État — CUMUL depuis janvier
//	DREES   comptes de la protection sociale — prestations, 1959→2024
//
// Deux avertissements structurent ces pages, parce qu'ils sont la première
// source d'erreur de lecture :
//
//  1. une situation mensuelle est un CUMUL depuis le 1er janvier, pas un flux
//     du mois ni un total d'année. « 145,9 Md€ de déficit en juillet 2026 »
//     veut dire « de janvier à juillet », et se compare à juillet 2025, jamais
//     à l'année 2025 entière ;
//  2. une situation mensuelle n'est pas la loi de règlement. Elle est
//     provisoire, en comptabilité budgétaire (encaissements-décaissements),
//     et le chiffre définitif de l'exercice sera différent.
type LigneBudget struct {
	Niveau                          int
	Categorie, SousCategorie, Ligne string
	Montant, Precedent              float64
	Ecart                           float64
	AvecPrecedent                   bool
}

type CourbeExercice struct {
	Exercice  int
	Path      template.HTML
	Dernier   string
	MoisFin   int
	X, Y      float64
	Incomplet bool
}

type StatsBudget struct {
	Arrete, MoisNom           string
	Mois, Exercice            int
	Recettes, Depenses, Solde float64
	Lignes                    []LigneBudget
	Courbes                   []CourbeExercice
	Grille                    template.HTML
	Exercices                 []int
	// RecettesDetail / DepensesDetail : les lignes NOMMÉES de la situation
	// mensuelle (TVA, IR, IS, TICPE… ; personnel, intervention, dette…), en
	// dehors du tableau hiérarchique complet — la réponse à « où passe l'argent »
	// sans avoir à dérouler les 26 lignes de la source.
	RecettesDetail []SousSecteur
	DepensesDetail []SousSecteur
	BarresRecettes template.HTML
	BarresDepenses template.HTML
}

var moisFr = [...]string{"", "janvier", "février", "mars", "avril", "mai", "juin",
	"juillet", "août", "septembre", "octobre", "novembre", "décembre"}

// mdEur : un montant en euros, écrit en milliards. Les budgets se lisent en
// milliards ; les afficher à l'euro près donnerait une précision que la
// situation mensuelle, provisoire, n'a pas.
func mdEur(v float64) string { return Decimal(v/1e9, 1) + "\u202fMd€" }

func loadBudget(ctx context.Context, pool *pgxpool.Pool) (*StatsBudget, error) {
	st := &StatsBudget{}
	// max(...) est une agrégation : la ligne existe même sans exécution
	// budgétaire encore ingérée, avec des valeurs NULL.
	var arreteN sql.NullString
	var moisN, exerciceN sql.NullInt64
	err := pool.QueryRow(ctx, `
		SELECT to_char(max(date_arrete),'YYYY-MM-DD'),
		       extract(month FROM max(date_arrete))::int,
		       max(exercice)::int
		FROM core.execution_etat`).Scan(&arreteN, &moisN, &exerciceN)
	if err != nil || !arreteN.Valid {
		return nil, err
	}
	arrete := arreteN.String
	st.Mois, st.Exercice = int(moisN.Int64), int(exerciceN.Int64)
	st.MoisNom = moisFr[st.Mois]
	st.Arrete = st.MoisNom + " " + fmt.Sprint(st.Exercice)

	// Les trois nombres de tête. Ils viennent de lignes nommées, jamais d'une
	// somme faite ici : additionner des lignes hiérarchisées double-compterait.
	tete := func(cat, ligne string) float64 {
		var v *float64
		_ = pool.QueryRow(ctx, `
			SELECT montant_eur FROM core.execution_etat
			WHERE date_arrete=$1::date AND categorie=$2 AND ligne=$3`,
			arrete, cat, ligne).Scan(&v)
		if v == nil {
			return 0
		}
		return *v
	}
	st.Recettes = tete("Recettes", "Total recettes nettes du budget général")
	st.Depenses = tete("Dépenses", "Total dépenses nettes du budget général")
	st.Solde = tete("Solde budgétaire", "Solde budgétaire")

	// Le même mois de l'exercice précédent : c'est la seule comparaison qui ait
	// un sens sur un cumul. Comparer juillet à l'année pleine d'avant serait
	// comparer sept mois à douze.
	rows, err := pool.Query(ctx, `
		SELECT e.niveau, e.categorie, e.sous_categorie, e.ligne, e.montant_eur, p.montant_eur
		FROM core.execution_etat e
		LEFT JOIN core.execution_etat p
		  ON p.categorie=e.categorie AND p.sous_categorie=e.sous_categorie
		 AND p.ligne=e.ligne
		 AND p.date_arrete = (e.date_arrete - interval '1 year')::date
		WHERE e.date_arrete=$1::date
		ORDER BY e.categorie, e.sous_categorie, e.niveau, e.ligne`, arrete)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var l LigneBudget
		var m, p *float64
		if err := rows.Scan(&l.Niveau, &l.Categorie, &l.SousCategorie, &l.Ligne, &m, &p); err != nil {
			rows.Close()
			return nil, err
		}
		if m != nil {
			l.Montant = *m
		}
		if p != nil && *p != 0 {
			l.Precedent, l.AvecPrecedent = *p, true
			l.Ecart = 100 * (l.Montant - *p) / abs(*p)
		}
		st.Lignes = append(st.Lignes, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Le solde cumulé, mois par mois. En BARRES : un cumul mensuel est une
	// série de douze mesures distinctes, pas un phénomène continu — une courbe
	// suggère qu'on peut lire une valeur « à la mi-mars », ce que la source ne
	// publie pas. Trois barres par mois, une par exercice, même axe, origine à
	// zéro puisqu'il s'agit d'un solde.
	srows, err := pool.Query(ctx, `
		SELECT exercice::int, extract(month FROM date_arrete)::int, montant_eur
		FROM core.execution_etat
		WHERE categorie='Solde budgétaire' AND ligne='Solde budgétaire'
		ORDER BY 1,2`)
	if err != nil {
		return nil, err
	}
	par := map[int]map[int]float64{}
	var bas, haut float64
	for srows.Next() {
		var e, m int
		var v *float64
		if err := srows.Scan(&e, &m, &v); err != nil {
			break
		}
		if v == nil {
			continue
		}
		if par[e] == nil {
			par[e] = map[int]float64{}
		}
		par[e][m] = *v
		if *v < bas {
			bas = *v
		}
		if *v > haut {
			haut = *v
		}
	}
	srows.Close()
	for e := range par {
		st.Exercices = append(st.Exercices, e)
	}
	sort.Ints(st.Exercices)

	const gw, gh, gl, gt, gb, gr = 720.0, 260.0, 96.0, 16.0, 30.0, 10.0
	if haut < 0 {
		haut = 0
	}
	if bas == haut {
		bas = haut - 1
	}
	y := func(v float64) float64 { return gt + (gh-gt-gb)*(haut-v)/(haut-bas) }
	pas := (gw - gl - gr) / 12
	n := len(st.Exercices)
	if n == 0 {
		n = 1
	}
	larg := (pas - 6) / float64(n)

	var g strings.Builder
	// Zéro n'est pas une graduation comme les autres sur un solde : c'est la
	// frontière entre déficit et excédent, donc un trait plein.
	fmt.Fprintf(&g, `<line class="zero" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
		gl-4, y(0), gw-gr, y(0))
	for _, v := range []float64{bas, bas / 2} {
		fmt.Fprintf(&g, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">%s</text>`,
			gl-8, y(v)+3, mdEur(v))
	}
	fmt.Fprintf(&g, `<text class="an" x="%.1f" y="%.1f" text-anchor="end">0</text>`, gl-8, y(0)+3)
	mois := []string{"", "J", "F", "M", "A", "M", "J", "J", "A", "S", "O", "N", "D"}
	for m := 1; m <= 12; m++ {
		fmt.Fprintf(&g, `<text class="an" x="%.1f" y="%.1f" text-anchor="middle">%s</text>`,
			gl+pas*(float64(m)-0.5), gh-10, mois[m])
	}
	st.Grille = template.HTML(g.String())

	for i, e := range st.Exercices {
		var b strings.Builder
		c := CourbeExercice{Exercice: e}
		for m := 1; m <= 12; m++ {
			v, ok := par[e][m]
			if !ok {
				continue
			}
			x := gl + pas*float64(m-1) + 3 + larg*float64(i)
			y0, y1 := y(0), y(v)
			if y1 < y0 {
				y0, y1 = y1, y0
			}
			h := y1 - y0
			if h < 1 {
				h = 1
			}
			fmt.Fprintf(&b, `<rect class="b e%d" x="%.1f" y="%.1f" width="%.1f" height="%.1f">`+
				`<title>%d, cumul à fin %s : %s</title></rect>`,
				i, x, y0, larg-1, h, e, moisFr[m], mdEur(v))
			c.Dernier, c.MoisFin = mdEur(v), m
		}
		c.Incomplet = c.MoisFin < 12
		c.Path = template.HTML(b.String())
		st.Courbes = append(st.Courbes, c)
	}

	// Les lignes nommées de la situation mensuelle, niveau par niveau : les
	// quatre grandes recettes fiscales, puis les natures de dépense. Ce sont
	// des lignes RÉELLES de la source (categorie/niveau), pas une somme faite
	// ici — une case de la même table, jamais recalculée.
	detail := func(categorie string, niveau int) ([]SousSecteur, float64) {
		drows, err := pool.Query(ctx, `
			SELECT ligne, montant_eur FROM core.execution_etat
			WHERE date_arrete=$1::date AND categorie=$2 AND niveau=$3
			ORDER BY montant_eur DESC`, arrete, categorie, niveau)
		if err != nil {
			return nil, 0
		}
		defer drows.Close()
		var out []SousSecteur
		var total float64
		for drows.Next() {
			var s SousSecteur
			if err := drows.Scan(&s.Libelle, &s.Depenses); err != nil {
				break
			}
			out = append(out, s)
			total += s.Depenses
		}
		for i := range out {
			if total > 0 {
				out[i].PartDepenses = 100 * out[i].Depenses / total
			}
		}
		return out, total
	}
	st.RecettesDetail, _ = detail("Recettes", 3)
	st.DepensesDetail, _ = detail("Dépenses", 2)
	st.BarresRecettes = barresSecteurs(st.RecettesDetail)
	st.BarresDepenses = barresSecteurs(st.DepensesDetail)
	return st, nil
}

// pctFr : un pourcentage signé, virgule décimale. « +4.6 % » est un anglicisme
// typographique dans une page française.
func pctFr(v float64) string {
	signe := "+"
	if v < 0 {
		signe = ""
	}
	return signe + Decimal(v, 1) + "\u202f%"
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// ── Protection sociale ────────────────────────────────────────────────

type RisqueSocial struct {
	Code, Libelle string
	Montant       float64
	Part          float64
}

type StatsSocial struct {
	Annee, Debut  int
	Total         float64
	Risques       []RisqueSocial
	Serie         []PointAnnee
	Courbe        template.HTML
	SousRisques   []RisqueSocial
	NbRegimes     int
	PartSecuStric float64
	Rupture       int
	Population    []PointAnnee
	CourbeAvecPop template.HTML
	Donut         template.HTML
	// Part65, Part20 : la structure par âge que la courbe de population totale
	// ne montre pas (voir la note abs de la page) — deux parts de la MÊME
	// population, sur le MÊME axe 0-100, donc comparables directement.
	Part65, Part20  []PointAnnee
	CourbeAges      template.HTML
	AnneeCroisement int
}

func loadSocial(ctx context.Context, pool *pgxpool.Pool) (*StatsSocial, error) {
	st := &StatsSocial{}
	// Debut est celui de la SÉRIE affichée, pas de la table : le compte remonte
	// à 1959 pour certains postes, mais le total tous régimes ne commence qu'en
	// 1981. Titrer « depuis 1959 » sur une courbe qui part de 1981 serait faux.
	// max(...) est une agrégation : la ligne existe même sans protection
	// sociale encore ingérée, avec une année NULL.
	var anneeN sql.NullInt64
	err := pool.QueryRow(ctx, `
		SELECT max(annee)::int, count(DISTINCT regime) FROM core.protection_sociale`).
		Scan(&anneeN, &st.NbRegimes)
	if err != nil {
		return nil, err
	}
	st.Annee = int(anneeN.Int64)

	// « Tous régimes » et le niveau 1 : six risques qui se somment exactement au
	// total. Descendre plus bas ou mélanger les niveaux double-compterait.
	const filtre = `si_code='S1' AND regime='Total tous régimes'`
	_ = pool.QueryRow(ctx, `SELECT valeur_meur*1e6 FROM core.protection_sociale
		WHERE annee=$1 AND ps_niveau=0 AND `+filtre, st.Annee).Scan(&st.Total)

	for _, niv := range []int{1, 2} {
		rows, err := pool.Query(ctx, `
			SELECT ps_code, ps_libelle, valeur_meur*1e6 FROM core.protection_sociale
			WHERE annee=$1 AND ps_niveau=$2 AND `+filtre+` ORDER BY 3 DESC`, st.Annee, niv)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r RisqueSocial
			if err := rows.Scan(&r.Code, &r.Libelle, &r.Montant); err != nil {
				break
			}
			if st.Total > 0 {
				r.Part = 100 * r.Montant / st.Total
			}
			if niv == 1 {
				st.Risques = append(st.Risques, r)
			} else {
				st.SousRisques = append(st.SousRisques, r)
			}
		}
		rows.Close()
	}
	st.Donut = donutRisques(st.Risques, st.Total, mdEur(st.Total))

	// La série longue vient de la vue derived.protection_sociale_total, somme
	// des six risques : elle rejoint EXACTEMENT le total « tous régimes » à
	// partir de 1981 (127,4 Md€) et le prolonge jusqu'en 1959. Avant 1981, le
	// libellé du périmètre change dans la source — « tous secteurs
	// institutionnels » au lieu de « total tous régimes » — et l'année de
	// rupture est affichée sur la page plutôt que lissée.
	srows, err := pool.Query(ctx, `
		SELECT annee::int, sum(prestations_meur)*1e6 FROM derived.protection_sociale_total
		GROUP BY annee ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var p PointAnnee
		if err := srows.Scan(&p.Annee, &p.Valeur); err != nil {
			break
		}
		st.Serie = append(st.Serie, p)
	}
	srows.Close()
	_ = pool.QueryRow(ctx, `
		SELECT min(annee) FROM core.protection_sociale
		WHERE ps_niveau=0 AND si_code='S1' AND regime='Total tous régimes'`).Scan(&st.Rupture)
	if len(st.Serie) > 0 {
		st.Debut = st.Serie[0].Annee
	}
	st.Courbe = courbe(st.Serie, mdEur)

	// La population totale, en superposition : la même hausse des prestations
	// se lit très différemment selon qu'elle vient de plus de bénéficiaires ou
	// de prestations plus généreuses par personne. Champ METRO parce que sa
	// série remonte à 1901, donc couvre tout l'historique des prestations ;
	// FRANCE entière ne commence qu'en 1991.
	prows, err := pool.Query(ctx, `
		SELECT annee, sum(population)::float8 FROM core.population_age
		WHERE champ='METRO' GROUP BY annee ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var p PointAnnee
		if err := prows.Scan(&p.Annee, &p.Valeur); err != nil {
			break
		}
		st.Population = append(st.Population, p)
	}
	prows.Close()
	millions := func(v float64) string { return Decimal(v/1e6, 1) + "\u202fM" }
	st.CourbeAvecPop = courbeAvecLigne(st.Serie, st.Population, mdEur, millions)

	// La structure par \u00e2ge : \u00ab la population a aussi vieilli \u00bb, affirm\u00e9 dans
	// la note ci-dessous mais jamais montr\u00e9 ailleurs sur le site avant cette
	// courbe. Part des moins de 20 ans et des 65 ans et plus, sur la m\u00eame
	// population et le m\u00eame axe 0-100 \u2014 directement comparables, contrairement
	// \u00e0 la courbe de population totale ci-dessus.
	arows, err := pool.Query(ctx, `
		SELECT annee, sum(population) FILTER (WHERE age<20)::float8,
		       sum(population) FILTER (WHERE age>=65)::float8, sum(population)::float8
		FROM core.population_age WHERE champ='METRO' GROUP BY annee ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	pct := "%"
	for arows.Next() {
		var an int
		var p20, p65, tot float64
		if err := arows.Scan(&an, &p20, &p65, &tot); err != nil {
			arows.Close()
			return nil, err
		}
		if tot <= 0 {
			continue
		}
		st.Part20 = append(st.Part20, PointAnnee{Annee: an, Valeur: 100 * p20 / tot})
		st.Part65 = append(st.Part65, PointAnnee{Annee: an, Valeur: 100 * p65 / tot})
	}
	arows.Close()
	if err := arows.Err(); err != nil {
		return nil, err
	}
	// L'ann\u00e9e de croisement : la premi\u00e8re o\u00f9 les 65 ans et plus d\u00e9passent les
	// moins de 20 ans \u2014 un fait dat\u00e9, pas une tendance qu'on affirme \u00e0 l'\u0153il.
	for i, p := range st.Part65 {
		if p.Valeur > st.Part20[i].Valeur {
			st.AnneeCroisement = p.Annee
			break
		}
	}
	st.CourbeAges = deuxCourbes(st.Part65, st.Part20,
		"65 ans et plus", "Moins de 20 ans", func(v float64) string { return Decimal(v, 1) + pct })
	return st, nil
}

// ── Les quatre bourses publiques ──────────────────────────────────────

// Le budget de l'État n'est PAS le budget public. Il en fait 40 % ; la Sécurité
// sociale en fait presque la moitié, et les collectivités un cinquième. Les
// trois sont votés par des assemblées différentes, tenus dans des comptabilités
// différentes, et le déficit dont on parle au journal est la somme des trois.
type SousSecteur struct {
	Code, Libelle             string
	Depenses, Recettes, Solde float64
	PartDepenses              float64
}

type SerieSecteur struct {
	Annee                     int
	Depenses, Recettes, Solde float64
}

type StatsSecteurs struct {
	// Consolidation : la somme des dépenses des trois sous-secteurs dépasse le
	// total consolidé, parce qu'un transfert de l'État à une collectivité est
	// une dépense de l'un ET finance une dépense de l'autre. Les soldes, eux,
	// s'additionnent exactement.
	SommeDepenses      float64
	EcartConsolidation float64
	// DepRec : dépenses et recettes des administrations publiques, année par
	// année, en barres appariées. Le solde se lit dans l'écart entre les deux.
	DepRec    template.HTML
	Annee     int
	Debut     int
	Secteurs  []SousSecteur
	Total     SousSecteur
	Series    map[string][]SerieSecteur
	Barres    template.HTML
	Financem  []LigneFinancement
	AnnFin    int
	DebutFin  int
	BarresFin template.HTML
	// CourbeS1311S1314 : les dépenses de l'administration centrale contre
	// celles de la Sécurité sociale, sur toute la série — le fait que la
	// seconde dépense plus que la première n'est montré nulle part ailleurs
	// qu'en un instantané d'une seule année (Secteurs, ci-dessus).
	CourbeS1311S1314    template.HTML
	AnneeCroisement1314 int
	// EmpileesFinancement : les quatre postes de financement de la protection
	// sociale, empilés à 100 % sur TOUTE la série (1990–2023) — barresFin,
	// ci-dessus, ne montre que deux dates.
	EmpileesFinancement template.HTML
}

type LigneFinancement struct {
	Code, Libelle string
	Montant       float64
	Part          float64
	PartDebut     float64
}

func loadSecteurs(ctx context.Context, pool *pgxpool.Pool) (*StatsSecteurs, error) {
	st := &StatsSecteurs{Series: map[string][]SerieSecteur{}}
	// max/min sont des agrégations : la ligne existe même sans ce dérivé
	// encore calculé, avec des bornes NULL.
	var anneeN, debutN sql.NullInt64
	if err := pool.QueryRow(ctx,
		`SELECT max(annee), min(annee) FROM derived.budget_sous_secteur`).
		Scan(&anneeN, &debutN); err != nil {
		return nil, err
	}
	st.Annee, st.Debut = int(anneeN.Int64), int(debutN.Int64)
	rows, err := pool.Query(ctx, `
		SELECT annee, secteur, perimetre_label,
		       depenses_meur::float8*1e6, recettes_meur::float8*1e6, solde_meur::float8*1e6
		FROM derived.budget_sous_secteur ORDER BY annee`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var an int
		var code, lib string
		var d, r, so float64
		if err := rows.Scan(&an, &code, &lib, &d, &r, &so); err != nil {
			break
		}
		st.Series[code] = append(st.Series[code], SerieSecteur{an, d, r, so})
		if an == st.Annee {
			s := SousSecteur{Code: code, Libelle: lib, Depenses: d, Recettes: r, Solde: so}
			if code == "S13" {
				st.Total = s
			} else {
				st.Secteurs = append(st.Secteurs, s)
			}
		}
	}
	rows.Close()
	for _, x := range st.Secteurs {
		st.SommeDepenses += x.Depenses
	}
	st.EcartConsolidation = st.SommeDepenses - st.Total.Depenses
	for i := range st.Secteurs {
		if st.Total.Depenses > 0 {
			st.Secteurs[i].PartDepenses = 100 * st.Secteurs[i].Depenses / st.Total.Depenses
		}
	}
	sort.Slice(st.Secteurs, func(i, j int) bool {
		return st.Secteurs[i].Depenses > st.Secteurs[j].Depenses
	})
	st.Barres = barresSecteurs(st.Secteurs)
	var paires []PaireAnnee
	for _, x := range st.Series["S13"] {
		paires = append(paires, PaireAnnee{x.Annee, x.Depenses, x.Recettes})
	}
	st.DepRec = barresAppariees(paires, "Dépenses", "Recettes", mdEur, 5)

	// Administration centrale contre Sécurité sociale, dépenses, toute la
	// série : « c'est la Sécurité sociale qui dépense le plus » (la note plus
	// bas) devient une forme, pas seulement un chiffre pour la dernière année.
	var s1311, s1314 []PointAnnee
	for _, x := range st.Series["S1311"] {
		s1311 = append(s1311, PointAnnee{Annee: x.Annee, Valeur: x.Depenses})
	}
	for _, x := range st.Series["S1314"] {
		s1314 = append(s1314, PointAnnee{Annee: x.Annee, Valeur: x.Depenses})
	}
	if len(s1311) == len(s1314) {
		for i, p := range s1314 {
			if p.Valeur > s1311[i].Valeur {
				st.AnneeCroisement1314 = p.Annee
				break
			}
		}
		st.CourbeS1311S1314 = deuxCourbes(s1314, s1311,
			"Sécurité sociale (S1314)", "Administration centrale (S1311)", mdEur)
	}

	// Le financement de la protection sociale : la bascule cotisations → impôt.
	// C'est le fait le plus mal connu du budget social, et il est publié tel
	// quel par la DREES — aucune interprétation n'est nécessaire pour le voir.
	postes := []struct{ code, lib string }{
		{"protection.financement.cotisations.employeurs", "Cotisations des employeurs"},
		{"protection.financement.cotisations.protegees", "Cotisations des assurés"},
		{"protection.financement.impot.affecte", "Recettes fiscales affectées"},
		{"protection.financement.impot.general", "Recettes fiscales générales"},
	}
	_ = pool.QueryRow(ctx, `
		SELECT max(annee), min(annee) FROM mv.macro_value
		WHERE serie_code='protection.financement.total'`).Scan(&st.AnnFin, &st.DebutFin)
	var totFin, totDeb float64
	_ = pool.QueryRow(ctx, `
		SELECT valeur::float8 FROM mv.macro_value
		WHERE serie_code='protection.financement.total' AND annee=$1`, st.AnnFin).Scan(&totFin)
	_ = pool.QueryRow(ctx, `
		SELECT valeur::float8 FROM mv.macro_value
		WHERE serie_code='protection.financement.total' AND annee=$1`, st.DebutFin).Scan(&totDeb)
	for _, p := range postes {
		var fin, deb *float64
		_ = pool.QueryRow(ctx, `
			SELECT max(valeur) FILTER (WHERE annee=$2)::float8,
			       max(valeur) FILTER (WHERE annee=$3)::float8
			FROM mv.macro_value WHERE serie_code=$1`, p.code, st.AnnFin, st.DebutFin).
			Scan(&fin, &deb)
		l := LigneFinancement{Code: p.code, Libelle: p.lib}
		if fin != nil {
			l.Montant = *fin * 1e6
			if totFin > 0 {
				l.Part = 100 * *fin / totFin
			}
		}
		if deb != nil && totDeb > 0 {
			l.PartDebut = 100 * *deb / totDeb
		}
		st.Financem = append(st.Financem, l)
	}
	st.BarresFin = barresFinancement(st.Financem, st.DebutFin, st.AnnFin)

	// La même bascule, sur les 34 années, en 100 % empilé — barresFinancement
	// ne compare que deux dates parce qu'un empilement en série devient un mur
	// de couleurs ; ici il ne l'est pas trop, quatre postes sur 34 ans.
	frows, err := pool.Query(ctx, `
		SELECT annee, serie_code, valeur::float8 FROM mv.macro_value
		WHERE serie_code = ANY($1) ORDER BY annee`,
		[]string{"protection.financement.cotisations.employeurs",
			"protection.financement.cotisations.protegees",
			"protection.financement.impot.affecte",
			"protection.financement.impot.general"})
	if err != nil {
		return nil, err
	}
	valeursFin := map[string]map[int]float64{}
	var anneesFin []int
	vuAnneeFin := map[int]bool{}
	for frows.Next() {
		var an int
		var code string
		var v float64
		if err := frows.Scan(&an, &code, &v); err != nil {
			frows.Close()
			return nil, err
		}
		if valeursFin[code] == nil {
			valeursFin[code] = map[int]float64{}
		}
		valeursFin[code][an] = v
		if !vuAnneeFin[an] {
			vuAnneeFin[an] = true
			anneesFin = append(anneesFin, an)
		}
	}
	frows.Close()
	if err := frows.Err(); err != nil {
		return nil, err
	}
	sort.Ints(anneesFin)
	// 100 % empilé : chaque poste en PART de l'année, pas en montant — sinon
	// l'inflation ferait grossir la barre entière, et le propos est la
	// COMPOSITION, pas le total.
	totAnnee := map[int]float64{}
	for _, code := range []string{"protection.financement.cotisations.employeurs",
		"protection.financement.cotisations.protegees", "protection.financement.impot.affecte",
		"protection.financement.impot.general"} {
		for an, v := range valeursFin[code] {
			totAnnee[an] += v
		}
	}
	pctFin := map[string]map[int]float64{}
	for _, p := range postes {
		pctFin[p.code] = map[int]float64{}
		for an, v := range valeursFin[p.code] {
			if totAnnee[an] > 0 {
				pctFin[p.code][an] = 100 * v / totAnnee[an]
			}
		}
	}
	st.EmpileesFinancement = barresEmpileesAnnuelles(anneesFin, []SerieEmpilee{
		{Libelle: postes[0].lib, Couleur: "#1E5C69", Valeurs: pctFin[postes[0].code]},
		{Libelle: postes[1].lib, Couleur: "#4A8894", Valeurs: pctFin[postes[1].code]},
		{Libelle: postes[2].lib, Couleur: "#B0CFD5", Valeurs: pctFin[postes[2].code]},
		{Libelle: postes[3].lib, Couleur: "#DCE9EC", Valeurs: pctFin[postes[3].code]},
	}, func(v float64) string { return Decimal(v, 1) + " %" })
	return st, nil
}

func barresSecteurs(ss []SousSecteur) template.HTML {
	var max float64
	for _, s := range ss {
		if s.Depenses > max {
			max = s.Depenses
		}
	}
	if max <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="barres">`)
	for _, s := range ss {
		fmt.Fprintf(&b, `<div class="ligne"><span class="n">%s</span>`+
			`<span class="piste"><i style="width:%.1f%%"></i></span>`+
			`<span class="v">%s</span><span class="c">%s</span></div>`,
			template.HTMLEscapeString(s.Libelle), 100*s.Depenses/max,
			mdEur(s.Depenses), Decimal(s.PartDepenses, 0)+" %")
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// barresFinancement : deux barres empilées à 100 %, la première année et la
// dernière. Un empilement en pourcentage ne se lit bien qu'à deux ou trois
// dates ; en série annuelle il devient un mur de couleurs.
func barresFinancement(ls []LigneFinancement, debut, fin int) template.HTML {
	if len(ls) == 0 {
		return ""
	}
	var b strings.Builder
	for _, cas := range []struct {
		an   int
		part func(LigneFinancement) float64
	}{{debut, func(l LigneFinancement) float64 { return l.PartDebut }},
		{fin, func(l LigneFinancement) float64 { return l.Part }}} {
		fmt.Fprintf(&b, `<div class="empil"><span class="an">%d</span><span class="pile">`, cas.an)
		for i, l := range ls {
			fmt.Fprintf(&b, `<i class="f%d" style="width:%.2f%%" title="%s : %s"></i>`,
				i, cas.part(l), template.HTMLEscapeString(l.Libelle),
				Decimal(cas.part(l), 1)+" %")
		}
		b.WriteString(`</span></div>`)
	}
	b.WriteString(`<div class="legende">`)
	for i, l := range ls {
		fmt.Fprintf(&b, `<span><i class="f%d"></i>%s</span>`, i,
			template.HTMLEscapeString(l.Libelle))
	}
	b.WriteString(`</div>`)
	return template.HTML(`<div class="empils">` + b.String() + `</div>`)
}
