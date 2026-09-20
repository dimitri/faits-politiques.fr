package main

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// L'organisation du site par sujets de campagne (proposition d'accueil du
// 15 septembre 2026, D-066) : la présidentielle de 2027 est l'entrée, les
// sujets dont parlent les candidats en sont la clé de lecture, et chaque sujet
// se lit avec l'argent public qui le finance.
//
// Un sujet est un dossier au plan commun (docs/<slug>.md), rendu en page à
// /sujets/<id>/ — ou /argent-public/<id>/ pour les dossiers sur les finances
// publiques elles-mêmes —, avec les pages de données qui existaient déjà.
// Les anciennes adresses /comprendre/<slug>/ de ces dossiers renvoient vers la
// nouvelle : un lien cité ailleurs ne casse pas.

type LienPage struct{ Titre, URL string }

type Sujet struct {
	ID, Nom, Doc string
	Pages        []LienPage
	Famille      *Famille
	D            *Doc

	// Rempli par preparerSujets (sujet_page.go).
	Chapeau             template.HTML
	Version             string
	EnBref              []ChiffreCle
	Donnees             template.HTML
	Sections            []SectionSujet
	NbCadre, NbControle int
}

// URL : chemin du sujet sous la racine du site, sans préfixe.
func (s *Sujet) URL() string { return s.Famille.Base + "/" + s.ID + "/" }

// Lien : la page du sujet, ou sa première page de données quand le sujet n'a
// pas (encore) de dossier.
func (s *Sujet) Lien() string {
	if s.Doc == "" && len(s.Pages) > 0 {
		return s.Pages[0].URL
	}
	return s.URL()
}

type Famille struct {
	ID, Nom, Intro string
	// Base : « sujets » ou « argent-public ».
	Base string
	// Fonctions COFOG dont relève la famille ; ParMille est calculé depuis
	// la base. Une politique peut relever de plusieurs fonctions : le poids
	// situe, il ne s'additionne pas.
	Cofog    []string
	ParMille int
	// Largeur : ParMille rapporté à la famille la plus lourde, en % — calculé
	// par loadAccueil une fois tous les ParMille connus. Sert à la barre de
	// poids des cartes de famille (accueil et /sujets/), jamais au texte lui
	// seul, qui reste le chiffre exact.
	Largeur float64
	Sujets  []*Sujet
}

var familles = []*Famille{
	{ID: "protection-sociale-sante", Nom: "Retraites, santé et protection sociale", Base: "sujets",
		Intro: "Retraites, maladie, chômage, famille, pauvreté : plus de la moitié de la dépense publique.",
		Cofog: []string{"GF10", "GF07"}, Sujets: []*Sujet{
			{ID: "retraites", Nom: "Retraites", Doc: "retraite-donnees", Pages: []LienPage{{"La vieillesse au-delà des retraites : dépendance, APA", "vieillesse/"}}},
			{ID: "sante", Nom: "Santé et hôpitaux", Doc: "sante-donnees"},
			{ID: "chomage", Nom: "Chômage", Doc: "chomage-donnees", Pages: []LienPage{{"Chômage et minima sociaux, en graphiques", "chomage/"}}},
			{ID: "pauvrete", Nom: "Pauvreté", Doc: "pauvrete-donnees"},
			{ID: "securite-sociale", Nom: "Sécurité sociale", Doc: "securite-sociale-donnees", Pages: []LienPage{{"La protection sociale depuis 1959", "protection-sociale/"}}},
			{ID: "cotisations", Nom: "Cotisations et droits", Doc: "cotisations-et-droits"},
		}},
	// Regroupe des sujets qui répondaient tous, jusqu'ici séparément, à la
	// même question (qui échappe à l'impôt, par quel mécanisme, pour quel
	// coût) et qui se citaient déjà mutuellement dans leurs sections
	// « Ce que les données ne disent pas » — auparavant éclatés entre
	// « Travail, économie et entreprises » et « Argent public et État ».
	// La répartition de la richesse les a rejoints (20 septembre 2026,
	// réorganisation en 9 familles) : la question qu'elle pose (qui détient
	// quoi) se lit avec celles de ce groupe (qui paie quoi, par quel
	// montage), pas avec les prestations de protection sociale d'où elle
	// venait.
	{ID: "fiscalite", Nom: "Fiscalité et patrimoine", Base: "sujets",
		Intro: "La TVA, les niches fiscales, la fraude et l'évasion, les montages patrimoniaux, la richesse.",
		Sujets: []*Sujet{
			{ID: "tva", Nom: "TVA", Doc: "tva-donnees"},
			{ID: "depenses-fiscales", Nom: "Dépenses fiscales (niches)", Doc: "depenses-fiscales-donnees"},
			{ID: "fraude-fiscale", Nom: "Fraude fiscale", Doc: "fraude-fiscale-donnees"},
			{ID: "evasion-fiscale", Nom: "Évasion fiscale", Doc: "evasion-fiscale-multinationales"},
			{ID: "sci-holding", Nom: "SCI et holdings", Doc: "sci-holding-donnees"},
			{ID: "richesse", Nom: "Répartition de la richesse", Doc: "repartition-richesse-donnees", Pages: []LienPage{{"La répartition de la richesse, en graphiques", "richesse/"}}},
		}},
	// Scindé de l'ancienne famille « Travail, économie et entreprises »
	// (9 sujets, la plus chargée) le 20 septembre 2026 : d'un côté l'État
	// actionnaire et employeur (ce chantier), de l'autre la politique
	// industrielle et les filières stratégiques (« Industrie et
	// souveraineté », juste après).
	{ID: "economie-entreprises", Nom: "Économie et entreprises", Base: "sujets",
		Intro: "L'État actionnaire, l'investissement public, l'emploi et les aides aux entreprises.",
		Cofog: []string{"GF04"}, Sujets: []*Sujet{
			{ID: "economie", Nom: "Économie et participations de l'État", Doc: "economie-participations-donnees", Pages: []LienPage{{"Dividendes et participations", "dividendes/"}}},
			{ID: "investissement", Nom: "Investissement et dividendes des entreprises", Doc: "investissement-entreprises-donnees"},
			{ID: "nationalisation-privatisation", Nom: "Nationalisations et privatisations", Doc: "nationalisation-privatisation-donnees"},
			{ID: "emploi", Nom: "Emploi et aides aux entreprises", Doc: "emploi-aides-entreprises-donnees"},
		}},
	{ID: "industrie-souverainete", Nom: "Industrie et souveraineté", Base: "sujets",
		Intro: "France 2030, souveraineté numérique, l'appareil productif, les grands ports.",
		Sujets: []*Sujet{
			{ID: "france-2030", Nom: "France 2030", Doc: "france-2030-donnees"},
			{ID: "souverainete-numerique", Nom: "Souveraineté numérique", Doc: "souverainete-numerique"},
			{ID: "appareil-productif", Nom: "L'appareil productif français", Doc: "appareil-productif-donnees"},
			{ID: "ports", Nom: "Les grands ports maritimes français", Doc: "ports-donnees"},
		}},
	{ID: "ecole-recherche-culture", Nom: "École, recherche et culture", Base: "sujets",
		Intro: "L'enseignement scolaire, les universités et la recherche, la culture, le sport.",
		Cofog: []string{"GF09", "GF08"}, Sujets: []*Sujet{
			{ID: "education", Nom: "Éducation nationale", Doc: "education-donnees"},
			{ID: "jeunesse", Nom: "Jeunesse : études, apprentissage, insertion", Doc: "jeunesse-donnees", Pages: []LienPage{{"La jeunesse : études supérieures, apprentissage, premiers emplois", "jeunesse/"}}},
			{ID: "recherche", Nom: "Recherche et universités", Doc: "recherche-enseignement-superieur-donnees"},
			{ID: "culture", Nom: "Culture et audiovisuel public", Doc: "culture-donnees"},
			{ID: "sport", Nom: "Sport et vie associative", Doc: "sport-vie-associative-donnees"},
		}},
	{ID: "securite-justice-defense", Nom: "Sécurité, justice et défense", Base: "sujets",
		Intro: "Police et délinquance enregistrée, tribunaux et prisons, armées.",
		Cofog: []string{"GF03", "GF02"}, Sujets: []*Sujet{
			{ID: "police", Nom: "Police et délinquance", Doc: "securite-police-donnees", Pages: []LienPage{{"La délinquance enregistrée, commune par commune", "securite/"}}},
			{ID: "justice", Nom: "Justice", Doc: "justice-donnees"},
			{ID: "violences-policieres", Nom: "Violences policières", Doc: "violences-policieres-donnees"},
			{ID: "defense", Nom: "Défense", Doc: "defense-donnees"},
		}},
	// L'agriculture a rejoint ce groupe (20 septembre 2026) : elle en
	// partage la logique de territoire et de ruralité, plutôt que celle,
	// industrielle, d'« Industrie et souveraineté » d'où elle vient — son
	// vrai poids budgétaire (GF04) y reste néanmoins attaché, pas ici :
	// le classement thématique et la fonction COFOG divergent sciemment.
	{ID: "territoires-environnement", Nom: "Territoires, ruralité et environnement", Base: "sujets",
		Intro: "Collectivités, logement, outre-mer, agriculture, climat et eau.",
		Cofog: []string{"GF06", "GF05"}, Sujets: []*Sujet{
			{ID: "collectivites", Nom: "Collectivités", Doc: "collectivites-donnees", Pages: []LienPage{{"Budgets et cartes des collectivités", "collectivites/"}}},
			{ID: "logement", Nom: "Logement et territoires", Doc: "logement-territoires-donnees"},
			{ID: "outre-mer", Nom: "Outre-mer", Doc: "outre-mer-donnees"},
			{ID: "agriculture", Nom: "Agriculture et alimentation", Doc: "agriculture-donnees", Pages: []LienPage{{"Agriculture et alimentation, en graphiques", "agriculture/"}}},
			{ID: "ecologie", Nom: "Écologie et climat", Doc: "ecologie-donnees"},
			{ID: "eau", Nom: "Eau", Doc: "bassins-versants-donnees"},
		}},
	{ID: "france-monde", Nom: "Immigration, Europe et diplomatie", Base: "sujets",
		Intro: "Population et migrations, diplomatie et aide au développement, l'Union européenne, la Francophonie.",
		Sujets: []*Sujet{
			{ID: "immigration", Nom: "Immigration", Doc: "immigration-donnees"},
			{ID: "diplomatie", Nom: "Diplomatie et aide au développement", Doc: "action-exterieure-donnees"},
			{ID: "international", Nom: "Comparaisons internationales", Doc: "international-donnees"},
			{ID: "union-europeenne", Nom: "Union européenne", Doc: "union-europeenne-donnees", Pages: []LienPage{{"Les votes des eurodéputés français", "europe/"}}},
			{ID: "francophonie", Nom: "La Francophonie", Doc: "francophonie-donnees"},
			{ID: "climat-international", Nom: "La France et le climat : accords de Paris, COP21", Doc: "climat-international-donnees"},
		}},
	// Séparé de « La France et le monde » (20 septembre 2026) : ces trois
	// dossiers regardent en arrière, pas les relations internationales
	// actuelles — un lecteur qui cherche l'immigration ou l'UE n'a plus à
	// traverser la Seconde Guerre mondiale et la décolonisation pour les
	// trouver, et réciproquement.
	{ID: "histoire", Nom: "Histoire", Base: "sujets",
		Intro: "L'Empire colonial, la Seconde Guerre mondiale, les guerres de décolonisation.",
		Sujets: []*Sujet{
			{ID: "empire-colonial", Nom: "La France coloniale", Doc: "empire-colonial-donnees"},
			{ID: "seconde-guerre-mondiale", Nom: "La France dans la Seconde Guerre mondiale", Doc: "seconde-guerre-mondiale-donnees"},
			{ID: "guerres-decolonisation", Nom: "Les guerres de décolonisation : Algérie et Indochine", Doc: "guerres-decolonisation-donnees"},
		}},
	// Les finances publiques elles-mêmes : l'entrée « Argent public » du menu.
	{ID: "argent-public", Nom: "Argent public et État", Base: "argent-public",
		Intro: "Les budgets, la dette, le coût des institutions et de la fonction publique.",
		Cofog: []string{"GF01"}, Sujets: []*Sujet{
			{ID: "budget", Nom: "Budget de l'État et de la Sécurité sociale", Doc: "budget-donnees", Pages: []LienPage{{"Recettes, dépenses et solde, mois par mois", "budget/"}}},
			{ID: "dette", Nom: "Dette publique", Doc: "dette-donnees", Pages: []LienPage{{"La dette, en graphiques", "dette/"}}},
			{ID: "pouvoirs-publics", Nom: "Coût des pouvoirs publics", Doc: "pouvoirs-publics-donnees"},
			{ID: "fonction-publique", Nom: "Fonction publique", Doc: "fonction-publique-donnees"},
		}},
}

// anciennesBasesSujets : les sujets qui ont changé de Base (donc d'adresse)
// en rejoignant la famille « Fiscalité ». evasion-fiscale, fraude-fiscale et
// sci-holding en faisaient déjà partie sous Base "sujets" et n'apparaissent
// donc pas ici — seuls tva et depenses-fiscales venaient de "argent-public".
var anciennesBasesSujets = map[string]string{
	"tva":               "argent-public",
	"depenses-fiscales": "argent-public",
}

func init() {
	for _, f := range familles {
		for _, s := range f.Sujets {
			s.Famille = f
		}
	}
}

// famillesSujets : les neuf familles de l'entrée « Sujets », sans l'argent public.
func famillesSujets() []*Famille {
	var out []*Famille
	for _, f := range familles {
		if f.Base == "sujets" {
			out = append(out, f)
		}
	}
	return out
}

func familleArgent() *Famille {
	for _, f := range familles {
		if f.Base == "argent-public" {
			return f
		}
	}
	return nil
}

// sujetDuDoc : le sujet qu'un dossier alimente, s'il en alimente un.
func sujetDuDoc(slug string) *Sujet {
	for _, f := range familles {
		for _, s := range f.Sujets {
			if s.Doc == slug {
				return s
			}
		}
	}
	return nil
}

// ── Données de l'accueil et de l'entrée « Argent public » ────────────────

// FonctionCofog : une ligne de « sur 1 000 € de dépense publique ».
type FonctionCofog struct {
	Code, Libelle, Detail string
	Slug                  string
	Milliards             float64
	ParMille              int
	Largeur               float64 // en % de la plus grande fonction
	Famille               *Famille
}

func (f FonctionCofog) URL() string { return "fonction/" + f.Slug + "/" }

// slugCofog : un identifiant lisible pour l'URL de chaque fonction — plus
// clair dans un lien qu'un code GF à deviner.
var slugCofog = map[string]string{
	"GF10": "protection-sociale", "GF07": "sante", "GF01": "services-generaux",
	"GF04": "economie-transports", "GF09": "enseignement", "GF02": "defense",
	"GF03": "ordre-securite", "GF08": "culture-loisirs", "GF06": "logement",
	"GF05": "environnement",
}

type BudgetNiveau struct {
	Nom, Texte, URL string
	Milliards       float64
	Largeur         float64
}

type Repere struct{ Valeur, Libelle, Source string }

type OngletCarte struct {
	Libelle, Titre, Question, Source, URL string
	Apercu                                Carte
}

type DonneesAccueil struct {
	Annee                     int
	Fonctions                 []FonctionCofog
	TotalMilliards            float64
	Budgets                   []BudgetNiveau
	Recettes, Depenses, Solde float64
	SoldePIB                  float64
	Reperes                   []Repere
	Familles                  []*Famille
	Argent                    *Famille
	Onglets                   []OngletCarte
	Defs                      template.HTML
}

// Le détail sous chaque fonction dit ce que la nomenclature y range, parce
// que c'est là que la lecture se trompe : les allocations logement sont dans
// la protection sociale, les intérêts de la dette dans les services généraux.
var detailCofog = map[string]string{
	"GF10": "retraites, chômage, famille, pauvreté, allocations logement",
	"GF07": "hôpital, soins de ville, médicaments",
	"GF01": "administration, intérêts de la dette, aide extérieure",
	"GF04": "transports, énergie, agriculture, aides aux entreprises",
	"GF09": "école, université",
	"GF02": "",
	"GF03": "police, justice, prisons",
	"GF08": "",
	"GF06": "",
	"GF05": "",
}

func loadAccueil(ctx context.Context, pool *pgxpool.Pool, terr *StatsTerritoires, sec *StatsSecurite) (*DonneesAccueil, error) {
	a := &DonneesAccueil{Familles: famillesSujets(), Argent: familleArgent()}

	// Dépense par fonction : la dernière année où les dix fonctions sont
	// publiées, pour que le total soit celui d'une même année. Les dix
	// divisions seulement (« GF01 » à « GF10 ») : les groupes à quatre
	// chiffres (« GF1002 », vieillesse) sont leurs sous-fonctions, et les
	// compter ensemble ferait deux fois la même dépense.
	// max(...) est une agrégation : la ligne existe même si aucune année n'a
	// encore ses dix fonctions complètes, avec une valeur NULL.
	var anneeN sql.NullInt64
	if err := pool.QueryRow(ctx, `
		SELECT max(annee) FROM (SELECT annee FROM core.macro_value WHERE serie_code ~ '^depense\.GF[0-9]{2}$'
		GROUP BY annee HAVING count(*) = 10) t`).Scan(&anneeN); err != nil {
		return nil, fmt.Errorf("dépense par fonction : %w", err)
	}
	a.Annee = int(anneeN.Int64)
	rows, err := pool.Query(ctx, `
		SELECT replace(v.serie_code, 'depense.', ''), replace(s.label, 'Dépense publique — ', ''), v.valeur::float8
		FROM core.macro_value v JOIN ref.macro_serie s ON s.code = v.serie_code
		WHERE v.serie_code ~ '^depense\.GF[0-9]{2}$' AND v.annee = $1
		ORDER BY v.valeur DESC`, a.Annee)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var f FonctionCofog
		var meur float64
		if err := rows.Scan(&f.Code, &f.Libelle, &meur); err != nil {
			rows.Close()
			return nil, err
		}
		f.Milliards = meur / 1000
		f.Detail = detailCofog[f.Code]
		f.Slug = slugCofog[f.Code]
		a.TotalMilliards += f.Milliards
		a.Fonctions = append(a.Fonctions, f)
	}
	rows.Close()
	if len(a.Fonctions) == 0 {
		return nil, fmt.Errorf("aucune dépense par fonction chargée (charger -only=macro)")
	}
	for i := range a.Fonctions {
		f := &a.Fonctions[i]
		f.ParMille = int(1000*f.Milliards/a.TotalMilliards + 0.5)
		f.Largeur = 100 * f.Milliards / a.Fonctions[0].Milliards
		for _, fam := range familles {
			for _, c := range fam.Cofog {
				if c == f.Code && f.Famille == nil {
					f.Famille = fam
					fam.ParMille += f.ParMille
				}
			}
		}
	}
	// Largeur de la barre de poids : relative à la famille « Sujets » la plus
	// lourde (l'argent public a sa propre entrée, pas de sens de la comparer
	// ici). Les familles sans fonction COFOG (Fiscalité, Histoire...) restent
	// à 0 — la carte l'indique en texte (« hors fonctions de dépense »), pas
	// une barre vide qu'on pourrait lire comme un poids nul.
	var maxParMilleSujets int
	for _, fam := range famillesSujets() {
		if fam.ParMille > maxParMilleSujets {
			maxParMilleSujets = fam.ParMille
		}
	}
	if maxParMilleSujets > 0 {
		for _, fam := range famillesSujets() {
			fam.Largeur = 100 * float64(fam.ParMille) / float64(maxParMilleSujets)
		}
	}

	// Les trois budgets, en comptabilité nationale : la même règle pour les trois.
	budgets := []struct{ code, nom, texte, url string }{
		{"S1314", "Sécurité sociale", "Retraites de base, assurance maladie, famille, chômage. Des objectifs de dépense, pas des plafonds.", "sujets/securite-sociale/"},
		{"S1311", "État", "Les missions du budget : école, défense, police, justice, et la charge de la dette.", "argent-public/budget/"},
		{"S1313", "Collectivités", "Communes, départements, régions : d'où vient leur argent, et ce qu'un euro par habitant ne dit pas.", "sujets/collectivites/"},
	}
	// La même année que la dépense par fonction et que le solde : trois
	// budgets d'années différentes ne se comparent pas.
	for _, b := range budgets {
		var dep float64
		if err := pool.QueryRow(ctx, `
			SELECT depenses_meur::float8 FROM derived.budget_sous_secteur
			WHERE secteur = $1 AND annee = $2`, b.code, a.Annee).Scan(&dep); err != nil {
			return nil, fmt.Errorf("budget %s : %w", b.code, err)
		}
		a.Budgets = append(a.Budgets, BudgetNiveau{Nom: b.nom, Texte: b.texte, URL: b.url, Milliards: dep / 1000})
	}
	plusGrand := 0.0
	for _, b := range a.Budgets {
		if b.Milliards > plusGrand {
			plusGrand = b.Milliards
		}
	}
	for i := range a.Budgets {
		a.Budgets[i].Largeur = 100 * a.Budgets[i].Milliards / plusGrand
	}
	if err := pool.QueryRow(ctx, `
		SELECT recettes_meur::float8/1000, depenses_meur::float8/1000, solde_meur::float8/1000
		FROM derived.budget_sous_secteur WHERE secteur = 'S13' AND annee = $1`, a.Annee).
		Scan(&a.Recettes, &a.Depenses, &a.Solde); err != nil {
		return nil, fmt.Errorf("solde public : %w", err)
	}
	_ = pool.QueryRow(ctx, `SELECT valeur::float8 FROM core.macro_value WHERE serie_code='solde.public.pib' AND annee=$1`, a.Annee).Scan(&a.SoldePIB)

	// Repères : la dernière valeur publiée de chaque série, avec son année.
	reperes := []struct {
		code, libelle, source string
		format                func(float64) string
	}{
		{"chomage.taux", "taux de chômage", "Eurostat, au sens du BIT", func(v float64) string { return Decimal(v, 1) + " %" }},
		{"pauvrete.taux", "taux de pauvreté", "Eurostat, seuil à 60 % du revenu médian", func(v float64) string { return Decimal(v, 1) + " %" }},
		{"dette.publique.meur", "dette publique", "Eurostat", func(v float64) string { return Nombre(int(v/1000+0.5)) + " Md€" }},
		{"dette.publique.pib", "dette publique rapportée au PIB", "Eurostat", func(v float64) string { return Decimal(v, 1) + " %" }},
		{"solde.public.pib", "solde public rapporté au PIB", "Eurostat", func(v float64) string { return Decimal(v, 1) + " %" }},
	}
	for _, r := range reperes {
		var annee int
		var v float64
		if err := pool.QueryRow(ctx, `SELECT annee, valeur::float8 FROM core.macro_value
			WHERE serie_code = $1 ORDER BY annee DESC LIMIT 1`, r.code).Scan(&annee, &v); err != nil {
			continue
		}
		a.Reperes = append(a.Reperes, Repere{Valeur: r.format(v), Libelle: r.libelle,
			Source: fmt.Sprintf("%d · %s", annee, r.source)})
	}

	// La carte à onglets : un sujet de campagne par onglet, dans l'affichage
	// en ligne des cartes départementales, outre-mer compris.
	carteTerr := func(slug string) *CarteTerritoire {
		for i := range terr.Cartes {
			if terr.Cartes[i].Slug == slug {
				return &terr.Cartes[i]
			}
		}
		return nil
	}
	if c := carteTerr("medecins-generalistes"); c != nil {
		a.Onglets = append(a.Onglets, OngletCarte{"Santé", c.Titre, c.Question, c.Source, "collectivites/carte/" + c.Slug + "/", c.Apercu})
	}
	if c := carteTerr("rsa"); c != nil {
		a.Onglets = append(a.Onglets, OngletCarte{"Solidarité", c.Titre, c.Question, c.Source, "collectivites/carte/" + c.Slug + "/", c.Apercu})
	}
	for _, ind := range sec.Indicateurs {
		if ind.Code == "cambriolages_de_logement" {
			a.Onglets = append(a.Onglets, OngletCarte{"Sécurité", ind.Libelle + " pour 1 000 habitants", ind.Question,
				ind.Page.Source + ", " + fmt.Sprint(sec.Annee), "securite/" + ind.Slug + "/", ind.Apercu})
		}
	}
	if c := carteTerr("dette"); c != nil {
		a.Onglets = append(a.Onglets, OngletCarte{"Finances locales", c.Titre, c.Question, c.Source, "collectivites/carte/" + c.Slug + "/", c.Apercu})
	}
	a.Defs = terr.Defs
	return a, nil
}

// ── Pages ────────────────────────────────────────────────────────────────

// redirection : une page statique qui renvoie vers la nouvelle adresse. Le
// lien canonique dit aux moteurs où est la page ; le lien visible sert quand
// le rafraîchissement est bloqué.
func redirection(path, cible string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	c := template.HTMLEscapeString(cible)
	html := `<!doctype html><html lang="fr"><head><meta charset="utf-8">` +
		`<meta http-equiv="refresh" content="0; url=` + c + `">` +
		`<link rel="canonical" href="` + c + `"><title>Page déplacée</title></head>` +
		`<body><p>Cette page a déménagé : <a href="` + c + `">` + c + `</a>.</p></body></html>`
	return os.WriteFile(path, []byte(html), 0o644)
}

// rattacherDocs relie chaque sujet à son dossier chargé. Un sujet dont le
// dossier manque est une erreur de construction : la page serait vide.
func rattacherDocs(docs []*Doc) error {
	par := map[string]*Doc{}
	for _, d := range docs {
		par[d.Slug] = d
	}
	var manquants []string
	for _, f := range familles {
		for _, s := range f.Sujets {
			if s.Doc == "" {
				continue
			}
			if d := par[s.Doc]; d != nil {
				s.D = d
			} else {
				manquants = append(manquants, s.Doc)
			}
		}
	}
	if len(manquants) > 0 {
		return fmt.Errorf("dossiers absents de docs/ : %s", strings.Join(manquants, ", "))
	}
	return nil
}

var reLienMD = regexp.MustCompile(`<a href="(?:\.\./)?(?:docs/)?([A-Za-z0-9_-]+)\.md(#[^"]*)?">([^<]*)</a>`)

// reecrireLiensDocs : dans le dépôt, un dossier renvoie à un autre par son
// fichier (« cotisations-et-droits.md », parfois « docs/cotisations-et-droits.md »
// — la convention même du texte du lien, jamais un titre). Sur le site, ce
// lien doit mener à la page : celle du sujet pour un dossier, celle de
// /comprendre/ sinon. Le texte visible, quand il n'est que ce nom de fichier,
// devient le vrai titre du dossier visé (§ 1 de sa propre note) — un lecteur
// ne sait pas ce que « cotisations-et-droits.md » désigne, alors que le
// dépôt, lui, connaît déjà le titre en question. Un texte de lien déjà écrit
// à la main (« D-012 », « Onglet Gouvernement »…) n'est pas ce nom de
// fichier : il reste intact. Un fichier absent de docs/ garde son lien tel
// quel, pour que l'erreur se voie.
func reecrireLiensDocs(docs []*Doc, root string) {
	titres := map[string]string{}
	for _, d := range docs {
		titres[d.Slug] = d.Titre
	}
	for _, d := range docs {
		d.Corps = template.HTML(reLienMD.ReplaceAllStringFunc(string(d.Corps), func(m string) string {
			g := reLienMD.FindStringSubmatch(m)
			slug, fragment, texte := g[1], g[2], g[3]
			titre, connu := titres[slug]
			if !connu {
				return m
			}
			cible := root + "/comprendre/" + slug + "/"
			if s := sujetDuDoc(slug); s != nil {
				cible = root + "/" + s.URL()
			}
			if texte == slug+".md" || texte == "docs/"+slug+".md" {
				texte = titre
			}
			return `<a href="` + cible + fragment + `">` + texte + `</a>`
		}))
	}
}

var courtsCofog = map[string]string{
	"GF10": "Protection sociale", "GF07": "Santé", "GF01": "Services généraux et dette",
	"GF04": "Économie, transports, agriculture", "GF09": "École et université", "GF02": "Défense",
	"GF03": "Police, justice, prisons", "GF08": "Culture, sport, loisirs", "GF06": "Logement, équipements",
	"GF05": "Environnement",
}

// couleurCofog : une teinte par fonction — qualitative, pas un dégradé : les
// dix fonctions sont dix catégories côte à côte, pas une grandeur qui varie.
var couleurCofog = map[string]string{
	"GF10": "#125863", "GF07": "#5FA3AD", "GF01": "#8C8577", "GF04": "#C08A3E",
	"GF09": "#5B7CA6", "GF02": "#6E4B3A", "GF03": "#8770A0", "GF08": "#B15E4A",
	"GF06": "#A98C6B", "GF05": "#4C8F62",
}

// heroMille : le graphique d'ouverture de l'accueil. Ce qui frappe à la
// première seconde et donne envie de creuser : sur 1 000 € de dépense publique,
// 415 vont à la protection sociale, 31 à la police et à la justice. Une
// mosaïque de 1 000 carrés — un par euro — sur ordinateur ; les mêmes chiffres
// en barres sur mobile, où 1 000 carrés seraient illisibles. Chaque ligne de
// la légende mène à la page de la fonction. Barres à l'échelle de la plus
// grande, en gris ; la première en encre, parce qu'elle sert d'étalon.
func heroMille(a *DonneesAccueil, root string) template.HTML {
	if a == nil || len(a.Fonctions) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<figure class="hero-mille"><figcaption><span class="sur">Le budget réel · %d</span>`+
		`<strong>Sur 1&nbsp;000&nbsp;€ de dépense publique</strong></figcaption>`, a.Annee)
	b.WriteString(`<div class="mille-corps">`)
	b.WriteString(mosaiqueMille(a))
	b.WriteString(`<ol>`)
	for i, f := range a.Fonctions {
		nom := courtsCofog[f.Code]
		if nom == "" {
			nom = f.Libelle
		}
		cl := ""
		if i == 0 {
			cl = ` class="etalon"`
		}
		fmt.Fprintf(&b, `<li%s><a href="%s/%s"><i class="pastille" aria-hidden="true" style="background:%s"></i>`+
			`<span class="l">%s</span><span class="b" aria-hidden="true"><i style="width:%.1f%%"></i></span>`+
			`<span class="v">%d&nbsp;€</span></a></li>`,
			cl, root, f.URL(), couleurCofog[f.Code], template.HTMLEscapeString(nom), f.Largeur, f.ParMille)
	}
	b.WriteString(`</ol></div>`)
	fmt.Fprintf(&b, `<p class="pied-mille">Un carré = 1&nbsp;€. Dépense constatée de l'État, de la Sécurité sociale et des collectivités&nbsp;: %s&nbsp;Md€. Eurostat / Insee. <a href="%s/argent-public/">Le détail →</a></p></figure>`,
		Decimal(a.TotalMilliards, 1), root)
	return template.HTML(b.String())
}

// mosaiqueMille : 1 000 carrés, un par euro, dans l'ordre des fonctions —
// visible sur ordinateur seulement (CSS), la légende ci-contre reste la
// version accessible et cliquable des mêmes chiffres.
func mosaiqueMille(a *DonneesAccueil) string {
	const cols, rows, cell, gap = 40, 25, 10.0, 1.4
	var b strings.Builder
	// preserveAspectRatio="none" : la mosaïque remplit toute la hauteur
	// disponible (celle de la légende, à sa droite — voir la règle CSS
	// .hero-mille .mosaique) plutôt que de garder le ratio 40:25 de la
	// grille et de laisser un vide sous elle. Les cases s'étirent d'à
	// peine quelques pour cent, invisible à l'œil sur 1 000 petits carrés.
	fmt.Fprintf(&b, `<svg class="mosaique" viewBox="0 0 %g %g" preserveAspectRatio="none" aria-hidden="true" focusable="false">`,
		cols*cell, rows*cell)
	i := 0
	for _, f := range a.Fonctions {
		couleur := couleurCofog[f.Code]
		for n := 0; n < f.ParMille && i < cols*rows; n++ {
			x, y := float64(i%cols)*cell, float64(i/cols)*cell
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="1.4" fill="%s"/>`,
				x+gap/2, y+gap/2, cell-gap, cell-gap, couleur)
			i++
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}
