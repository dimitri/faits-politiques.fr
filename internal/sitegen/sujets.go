package sitegen

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

type LinkPage struct{ Title, URL string }

type Topic struct {
	ID, Name, Doc string
	Pages         []LinkPage
	Family        *Family
	D             *Doc

	// Rempli par preparerSujets (sujet_page.go).
	Lead                         template.HTML
	Version                      string
	InBrief                      []FigureKey
	Data                         template.HTML
	Sections                     []SectionTopic
	CountFramework, CountControl int
}

// URL : chemin du sujet sous la racine du site, sans préfixe.
func (s *Topic) URL() string { return s.Family.Base + "/" + s.ID + "/" }

// Link : la page du sujet, ou sa première page de données quand le sujet n'a
// pas (encore) de dossier.
func (s *Topic) Link() string {
	if s.Doc == "" && len(s.Pages) > 0 {
		return s.Pages[0].URL
	}
	return s.URL()
}

// MapFamily : une carte illustrative pour la page /sujets/<famille>/,
// réutilisant une carte déjà calculée ailleurs dans le pipeline (zéro
// requête supplémentaire) — un simple <figure>, jamais le patron
// .bloc-carte.ligne des cartes choroplèthes départementales (Cartons/
// Teintes/Bornes), qui suppose une échelle de valeur que ces cartes n'ont
// pas toutes (bulles, monde, ou simple fond).
type MapFamily struct {
	SVG            template.HTML
	Legend         string
	Link, LinkText string
}

type Family struct {
	ID, Name, Intro string
	// Base : « sujets » ou « argent-public ».
	Base string
	// Fonctions COFOG dont relève la famille ; ParMille est calculé depuis
	// la base. Une politique peut relever de plusieurs fonctions : le poids
	// situe, il ne s'additionne pas.
	Cofog       []string
	PerThousand int
	// Largeur : ParMille rapporté à la famille la plus lourde, en % — calculé
	// par loadAccueil une fois tous les ParMille connus. Sert à la barre de
	// poids des cartes de famille (accueil et /sujets/), jamais au texte lui
	// seul, qui reste le chiffre exact.
	Width  float64
	Topics []*Topic
}

var families = []*Family{
	{ID: "protection-sociale-sante", Name: "Retraites, santé et protection sociale", Base: "sujets",
		Intro: "Retraites, maladie, chômage, famille, pauvreté : plus de la moitié de la dépense publique.",
		Cofog: []string{"GF10", "GF07"}, Topics: []*Topic{
			{ID: "retraites", Name: "Retraites", Doc: "retraite-donnees", Pages: []LinkPage{{"La vieillesse au-delà des retraites : dépendance, APA", "vieillesse/"}}},
			{ID: "sante", Name: "Santé et hôpitaux", Doc: "sante-donnees"},
			{ID: "chomage", Name: "Chômage", Doc: "chomage-donnees", Pages: []LinkPage{{"Chômage et minima sociaux, en graphiques", "chomage/"}}},
			{ID: "pauvrete", Name: "Pauvreté", Doc: "pauvrete-donnees"},
			{ID: "securite-sociale", Name: "Sécurité sociale", Doc: "securite-sociale-donnees", Pages: []LinkPage{{"La protection sociale depuis 1959", "protection-sociale/"}}},
			{ID: "cotisations", Name: "Cotisations et droits", Doc: "cotisations-et-droits"},
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
	{ID: "fiscalite", Name: "Fiscalité et patrimoine", Base: "sujets",
		Intro: "La TVA, les niches fiscales, la fraude et l'évasion, les montages patrimoniaux, la richesse.",
		Topics: []*Topic{
			{ID: "tva", Name: "TVA", Doc: "tva-donnees"},
			{ID: "depenses-fiscales", Name: "Dépenses fiscales (niches)", Doc: "depenses-fiscales-donnees"},
			{ID: "fraude-fiscale", Name: "Fraude fiscale", Doc: "fraude-fiscale-donnees"},
			{ID: "evasion-fiscale", Name: "Évasion fiscale", Doc: "evasion-fiscale-multinationales"},
			{ID: "sci-holding", Name: "SCI et holdings", Doc: "sci-holding-donnees"},
			{ID: "richesse", Name: "Répartition de la richesse", Doc: "repartition-richesse-donnees", Pages: []LinkPage{{"La répartition de la richesse, en graphiques", "richesse/"}}},
		}},
	// Scindé de l'ancienne famille « Travail, économie et entreprises »
	// (9 sujets, la plus chargée) le 20 septembre 2026 : d'un côté l'État
	// actionnaire et employeur (ce chantier), de l'autre la politique
	// industrielle et les filières stratégiques (« Industrie et
	// souveraineté », juste après).
	{ID: "economie-entreprises", Name: "Économie et entreprises", Base: "sujets",
		Intro: "L'État actionnaire, l'investissement public, l'emploi et les aides aux entreprises.",
		Cofog: []string{"GF04"}, Topics: []*Topic{
			{ID: "economie", Name: "Économie et participations de l'État", Doc: "economie-participations-donnees", Pages: []LinkPage{{"Dividendes et participations", "dividendes/"}}},
			{ID: "investissement", Name: "Investissement et dividendes des entreprises", Doc: "investissement-entreprises-donnees"},
			{ID: "nationalisation-privatisation", Name: "Nationalisations et privatisations", Doc: "nationalisation-privatisation-donnees"},
			{ID: "emploi", Name: "Emploi et aides aux entreprises", Doc: "emploi-aides-entreprises-donnees"},
		}},
	{ID: "industrie-souverainete", Name: "Industrie et souveraineté", Base: "sujets",
		Intro: "France 2030, souveraineté numérique, l'appareil productif, les grands ports.",
		Topics: []*Topic{
			{ID: "france-2030", Name: "France 2030", Doc: "france-2030-donnees"},
			{ID: "souverainete-numerique", Name: "Souveraineté numérique", Doc: "souverainete-numerique"},
			{ID: "appareil-productif", Name: "L'appareil productif français", Doc: "appareil-productif-donnees"},
			{ID: "ports", Name: "Les grands ports maritimes français", Doc: "ports-donnees"},
		}},
	{ID: "ecole-recherche-culture", Name: "École, recherche et culture", Base: "sujets",
		Intro: "L'enseignement scolaire, les universités et la recherche, la culture, le sport.",
		Cofog: []string{"GF09", "GF08"}, Topics: []*Topic{
			{ID: "education", Name: "Éducation nationale", Doc: "education-donnees"},
			{ID: "jeunesse", Name: "Jeunesse : études, apprentissage, insertion", Doc: "jeunesse-donnees", Pages: []LinkPage{{"La jeunesse : études supérieures, apprentissage, premiers emplois", "jeunesse/"}}},
			{ID: "recherche", Name: "Recherche et universités", Doc: "recherche-enseignement-superieur-donnees"},
			{ID: "culture", Name: "Culture et audiovisuel public", Doc: "culture-donnees"},
			{ID: "sport", Name: "Sport et vie associative", Doc: "sport-vie-associative-donnees"},
		}},
	{ID: "securite-justice-defense", Name: "Sécurité, justice et défense", Base: "sujets",
		Intro: "Police et délinquance enregistrée, tribunaux et prisons, armées.",
		Cofog: []string{"GF03", "GF02"}, Topics: []*Topic{
			{ID: "police", Name: "Police et délinquance", Doc: "securite-police-donnees", Pages: []LinkPage{{"La délinquance enregistrée, commune par commune", "securite/"}}},
			{ID: "justice", Name: "Justice", Doc: "justice-donnees"},
			{ID: "violences-policieres", Name: "Violences policières", Doc: "violences-policieres-donnees"},
			{ID: "defense", Name: "Défense", Doc: "defense-donnees"},
		}},
	// L'agriculture a rejoint ce groupe (20 septembre 2026) : elle en
	// partage la logique de territoire et de ruralité, plutôt que celle,
	// industrielle, d'« Industrie et souveraineté » d'où elle vient — son
	// vrai poids budgétaire (GF04) y reste néanmoins attaché, pas ici :
	// le classement thématique et la fonction COFOG divergent sciemment.
	{ID: "territoires-environnement", Name: "Territoires, ruralité et environnement", Base: "sujets",
		Intro: "Collectivités, logement, outre-mer, agriculture, climat et eau.",
		Cofog: []string{"GF06", "GF05"}, Topics: []*Topic{
			{ID: "collectivites", Name: "Collectivités", Doc: "collectivites-donnees", Pages: []LinkPage{{"Budgets et cartes des collectivités", "collectivites/"}}},
			{ID: "logement", Name: "Logement et territoires", Doc: "logement-territoires-donnees"},
			{ID: "outre-mer", Name: "Outre-mer", Doc: "outre-mer-donnees"},
			{ID: "agriculture", Name: "Agriculture et alimentation", Doc: "agriculture-donnees", Pages: []LinkPage{{"Agriculture et alimentation, en graphiques", "agriculture/"}}},
			{ID: "ecologie", Name: "Écologie et climat", Doc: "ecologie-donnees"},
			{ID: "eau", Name: "Eau", Doc: "bassins-versants-donnees"},
		}},
	{ID: "france-monde", Name: "Immigration, Europe et diplomatie", Base: "sujets",
		Intro: "Population et migrations, diplomatie et aide au développement, l'Union européenne, la Francophonie.",
		Topics: []*Topic{
			{ID: "immigration", Name: "Immigration", Doc: "immigration-donnees"},
			{ID: "diplomatie", Name: "Diplomatie et aide au développement", Doc: "action-exterieure-donnees"},
			{ID: "international", Name: "Comparaisons internationales", Doc: "international-donnees"},
			{ID: "union-europeenne", Name: "Union européenne", Doc: "union-europeenne-donnees", Pages: []LinkPage{{"Les votes des eurodéputés français", "europe/"}}},
			{ID: "francophonie", Name: "La Francophonie", Doc: "francophonie-donnees"},
			{ID: "climat-international", Name: "La France et le climat : accords de Paris, COP21", Doc: "climat-international-donnees"},
		}},
	// Séparé de « La France et le monde » (20 septembre 2026) : ces trois
	// dossiers regardent en arrière, pas les relations internationales
	// actuelles — un lecteur qui cherche l'immigration ou l'UE n'a plus à
	// traverser la Seconde Guerre mondiale et la décolonisation pour les
	// trouver, et réciproquement.
	{ID: "histoire", Name: "Histoire", Base: "sujets",
		Intro: "L'Empire colonial, la Seconde Guerre mondiale, les guerres de décolonisation.",
		Topics: []*Topic{
			{ID: "empire-colonial", Name: "La France coloniale", Doc: "empire-colonial-donnees"},
			{ID: "seconde-guerre-mondiale", Name: "La France dans la Seconde Guerre mondiale", Doc: "seconde-guerre-mondiale-donnees"},
			{ID: "guerres-decolonisation", Name: "Les guerres de décolonisation : Algérie et Indochine", Doc: "guerres-decolonisation-donnees"},
		}},
	// Les finances publiques elles-mêmes : l'entrée « Argent public » du menu.
	{ID: "argent-public", Name: "Argent public et État", Base: "argent-public",
		Intro: "Les budgets, la dette, le coût des institutions et de la fonction publique.",
		Cofog: []string{"GF01"}, Topics: []*Topic{
			{ID: "budget", Name: "Budget de l'État et de la Sécurité sociale", Doc: "budget-donnees", Pages: []LinkPage{{"Recettes, dépenses et solde, mois par mois", "budget/"}}},
			{ID: "dette", Name: "Dette publique", Doc: "dette-donnees", Pages: []LinkPage{{"La dette, en graphiques", "dette/"}}},
			{ID: "pouvoirs-publics", Name: "Coût des pouvoirs publics", Doc: "pouvoirs-publics-donnees"},
			{ID: "fonction-publique", Name: "Fonction publique", Doc: "fonction-publique-donnees"},
		}},
}

// anciennesBasesSujets : les sujets qui ont changé de Base (donc d'adresse)
// en rejoignant la famille « Fiscalité ». evasion-fiscale, fraude-fiscale et
// sci-holding en faisaient déjà partie sous Base "sujets" et n'apparaissent
// donc pas ici — seuls tva et depenses-fiscales venaient de "argent-public".
var formerBasesTopics = map[string]string{
	"tva":               "argent-public",
	"depenses-fiscales": "argent-public",
}

func init() {
	for _, f := range families {
		for _, s := range f.Topics {
			s.Family = f
		}
	}
}

// familiesTopics : les neuf familles de l'entrée « Sujets », sans l'argent public.
func familiesTopics() []*Family {
	var out []*Family
	for _, f := range families {
		if f.Base == "sujets" {
			out = append(out, f)
		}
	}
	return out
}

func familyMoney() *Family {
	for _, f := range families {
		if f.Base == "argent-public" {
			return f
		}
	}
	return nil
}

// topicDuDoc : le sujet qu'un dossier alimente, s'il en alimente un.
func topicDuDoc(slug string) *Topic {
	for _, f := range families {
		for _, s := range f.Topics {
			if s.Doc == slug {
				return s
			}
		}
	}
	return nil
}

// ── Données de l'accueil et de l'entrée « Argent public » ────────────────

// FunctionCofog : une ligne de « sur 1 000 € de dépense publique ».
type FunctionCofog struct {
	Code, Label, Detail string
	Slug                string
	Billions            float64
	PerThousand         int
	Width               float64 // en % de la plus grande fonction
	Family              *Family
}

func (f FunctionCofog) URL() string { return "fonction/" + f.Slug + "/" }

// slugCofog : un identifiant lisible pour l'URL de chaque fonction — plus
// clair dans un lien qu'un code GF à deviner.
var slugCofog = map[string]string{
	"GF10": "protection-sociale", "GF07": "sante", "GF01": "services-generaux",
	"GF04": "economie-transports", "GF09": "enseignement", "GF02": "defense",
	"GF03": "ordre-securite", "GF08": "culture-loisirs", "GF06": "logement",
	"GF05": "environnement",
}

type BudgetLevel struct {
	Name, Text, URL string
	Billions        float64
	Width           float64
}

type Marker struct{ Value, Label, Source string }

type TabMap struct {
	Label, Title, Question, Source, URL string
	// Carte : la carte PLEINE (tracés en clair, infobulles au survol), pas la
	// vignette décorative des grilles d'index — depuis la refonte du
	// 20 septembre 2026, l'accueil affiche la même carte que la page de
	// détail vers laquelle il pointe, à la même échelle que
	// /sujets/collectivites/ (le nouveau standard du site), pas une
	// miniature. Le calcul ne coûte rien de plus : PageCarte.Carte est déjà
	// produit pour la page de détail elle-même.
	Map Map
}

type DataHome struct {
	Year                        int
	Functions                   []FunctionCofog
	TotalBillions               float64
	Budgets                     []BudgetLevel
	Revenues, Expenses, Balance float64
	BalanceGdp                  float64
	Markers                     []Marker
	Families                    []*Family
	// NbSujets : total des sujets des neuf familles de campagne, hors
	// « argent public » qui n'en est pas une (voir familleArgent) — le
	// chiffre du bandeau d'ouverture de l'accueil, jamais recompté à la
	// main dans le gabarit.
	CountTopics int
	Money       *Family
	Tabs        []TabMap
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

func loadHome(ctx context.Context, pool *pgxpool.Pool, territory *StatsTerritories, sec *StatsSecurity) (*DataHome, error) {
	a := &DataHome{Families: familiesTopics(), Money: familyMoney()}
	for _, fam := range a.Families {
		a.CountTopics += len(fam.Topics)
	}

	// Dépense par fonction : la dernière année où les dix fonctions sont
	// publiées, pour que le total soit celui d'une même année. Les dix
	// divisions seulement (« GF01 » à « GF10 ») : les groupes à quatre
	// chiffres (« GF1002 », vieillesse) sont leurs sous-fonctions, et les
	// compter ensemble ferait deux fois la même dépense.
	// max(...) est une agrégation : la ligne existe même si aucune année n'a
	// encore ses dix fonctions complètes, avec une valeur NULL.
	var yearN sql.NullInt64
	if err := pool.QueryRow(ctx, `
		SELECT max(annee) FROM (SELECT annee FROM core.macro_value WHERE serie_code ~ '^depense\.GF[0-9]{2}$'
		GROUP BY annee HAVING count(*) = 10) t`).Scan(&yearN); err != nil {
		return nil, fmt.Errorf("dépense par fonction : %w", err)
	}
	a.Year = int(yearN.Int64)
	rows, err := pool.Query(ctx, `
		SELECT replace(v.serie_code, 'depense.', ''), replace(s.label, 'Dépense publique — ', ''), v.valeur::float8
		FROM core.macro_value v JOIN ref.macro_serie s ON s.code = v.serie_code
		WHERE v.serie_code ~ '^depense\.GF[0-9]{2}$' AND v.annee = $1
		ORDER BY v.valeur DESC`, a.Year)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var f FunctionCofog
		var meur float64
		if err := rows.Scan(&f.Code, &f.Label, &meur); err != nil {
			rows.Close()
			return nil, err
		}
		f.Billions = meur / 1000
		f.Detail = detailCofog[f.Code]
		f.Slug = slugCofog[f.Code]
		a.TotalBillions += f.Billions
		a.Functions = append(a.Functions, f)
	}
	rows.Close()
	if len(a.Functions) == 0 {
		return nil, fmt.Errorf("aucune dépense par fonction chargée (charger -only=macro)")
	}
	for i := range a.Functions {
		f := &a.Functions[i]
		f.PerThousand = int(1000*f.Billions/a.TotalBillions + 0.5)
		f.Width = 100 * f.Billions / a.Functions[0].Billions
		for _, fam := range families {
			for _, c := range fam.Cofog {
				if c == f.Code && f.Family == nil {
					f.Family = fam
					fam.PerThousand += f.PerThousand
				}
			}
		}
	}
	// Largeur de la barre de poids : relative à la famille « Sujets » la plus
	// lourde (l'argent public a sa propre entrée, pas de sens de la comparer
	// ici). Les familles sans fonction COFOG (Fiscalité, Histoire...) restent
	// à 0 — la carte l'indique en texte (« hors fonctions de dépense »), pas
	// une barre vide qu'on pourrait lire comme un poids nul.
	var maxPerThousandTopics int
	for _, fam := range familiesTopics() {
		if fam.PerThousand > maxPerThousandTopics {
			maxPerThousandTopics = fam.PerThousand
		}
	}
	if maxPerThousandTopics > 0 {
		for _, fam := range familiesTopics() {
			fam.Width = 100 * float64(fam.PerThousand) / float64(maxPerThousandTopics)
		}
	}

	// Les trois budgets, en comptabilité nationale : la même règle pour les trois.
	budgets := []struct{ code, name, text, url string }{
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
			WHERE secteur = $1 AND annee = $2`, b.code, a.Year).Scan(&dep); err != nil {
			return nil, fmt.Errorf("budget %s : %w", b.code, err)
		}
		a.Budgets = append(a.Budgets, BudgetLevel{Name: b.name, Text: b.text, URL: b.url, Billions: dep / 1000})
	}
	plusLarge := 0.0
	for _, b := range a.Budgets {
		if b.Billions > plusLarge {
			plusLarge = b.Billions
		}
	}
	for i := range a.Budgets {
		a.Budgets[i].Width = 100 * a.Budgets[i].Billions / plusLarge
	}
	if err := pool.QueryRow(ctx, `
		SELECT recettes_meur::float8/1000, depenses_meur::float8/1000, solde_meur::float8/1000
		FROM derived.budget_sous_secteur WHERE secteur = 'S13' AND annee = $1`, a.Year).
		Scan(&a.Revenues, &a.Expenses, &a.Balance); err != nil {
		return nil, fmt.Errorf("solde public : %w", err)
	}
	_ = pool.QueryRow(ctx, `SELECT valeur::float8 FROM core.macro_value WHERE serie_code='solde.public.pib' AND annee=$1`, a.Year).Scan(&a.BalanceGdp)

	// Repères : la dernière valeur publiée de chaque série, avec son année.
	markers := []struct {
		code, label, source string
		format              func(float64) string
	}{
		{"chomage.taux", "taux de chômage", "Eurostat, au sens du BIT", func(v float64) string { return Decimal(v, 1) + " %" }},
		{"pauvrete.taux", "taux de pauvreté", "Eurostat, seuil à 60 % du revenu médian", func(v float64) string { return Decimal(v, 1) + " %" }},
		{"dette.publique.meur", "dette publique", "Eurostat", func(v float64) string { return Count(int(v/1000+0.5)) + " Md€" }},
		{"dette.publique.pib", "dette publique rapportée au PIB", "Eurostat", func(v float64) string { return Decimal(v, 1) + " %" }},
		{"solde.public.pib", "solde public rapporté au PIB", "Eurostat", func(v float64) string { return Decimal(v, 1) + " %" }},
	}
	for _, r := range markers {
		var year int
		var v float64
		if err := pool.QueryRow(ctx, `SELECT annee, valeur::float8 FROM core.macro_value
			WHERE serie_code = $1 ORDER BY annee DESC LIMIT 1`, r.code).Scan(&year, &v); err != nil {
			continue
		}
		a.Markers = append(a.Markers, Marker{Value: r.format(v), Label: r.label,
			Source: fmt.Sprintf("%d · %s", year, r.source)})
	}

	// La carte à onglets : un sujet de campagne par onglet, dans l'affichage
	// en ligne des cartes départementales, outre-mer compris.
	mapTerritory := func(slug string) *MapTerritory {
		for i := range territory.Maps {
			if territory.Maps[i].Slug == slug {
				return &territory.Maps[i]
			}
		}
		return nil
	}
	if c := mapTerritory("medecins-generalistes"); c != nil {
		a.Tabs = append(a.Tabs, TabMap{"Santé", c.Title, c.Question, c.Source, "collectivites/carte/" + c.Slug + "/", c.Page.Map})
	}
	if c := mapTerritory("rsa"); c != nil {
		a.Tabs = append(a.Tabs, TabMap{"Solidarité", c.Title, c.Question, c.Source, "collectivites/carte/" + c.Slug + "/", c.Page.Map})
	}
	for _, ind := range sec.Indicators {
		if ind.Code == "cambriolages_de_logement" {
			a.Tabs = append(a.Tabs, TabMap{"Sécurité", ind.Label + " pour 1 000 habitants", ind.Question,
				ind.Page.Source + ", " + fmt.Sprint(sec.Year), "securite/" + ind.Slug + "/", ind.Page.Map})
		}
	}
	if c := mapTerritory("dette"); c != nil {
		a.Tabs = append(a.Tabs, TabMap{"Finances locales", c.Title, c.Question, c.Source, "collectivites/carte/" + c.Slug + "/", c.Page.Map})
	}
	return a, nil
}

// ── Pages ────────────────────────────────────────────────────────────────

// redirection : une page statique qui renvoie vers la nouvelle adresse. Le
// lien canonique dit aux moteurs où est la page ; le lien visible sert quand
// le rafraîchissement est bloqué.
func redirection(path, target string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	c := template.HTMLEscapeString(target)
	html := `<!doctype html><html lang="fr"><head><meta charset="utf-8">` +
		`<meta http-equiv="refresh" content="0; url=` + c + `">` +
		`<link rel="canonical" href="` + c + `"><title>Page déplacée</title></head>` +
		`<body><p>Cette page a déménagé : <a href="` + c + `">` + c + `</a>.</p></body></html>`
	return os.WriteFile(path, []byte(html), 0o644)
}

// rattacherDocs relie chaque sujet à son dossier chargé. Un sujet dont le
// dossier manque est une erreur de construction : la page serait vide.
func rattacherDocs(docs []*Doc) error {
	per := map[string]*Doc{}
	for _, d := range docs {
		per[d.Slug] = d
	}
	var missing []string
	for _, f := range families {
		for _, s := range f.Topics {
			if s.Doc == "" {
				continue
			}
			if d := per[s.Doc]; d != nil {
				s.D = d
			} else {
				missing = append(missing, s.Doc)
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("dossiers absents de docs/ : %s", strings.Join(missing, ", "))
	}
	return nil
}

var reLinkMD = regexp.MustCompile(`<a href="(?:\.\./)?(?:docs/)?([A-Za-z0-9_-]+)\.md(#[^"]*)?">([^<]*)</a>`)

// rewriteLinksDocs : dans le dépôt, un dossier renvoie à un autre par son
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
func rewriteLinksDocs(docs []*Doc, root string) {
	titles := map[string]string{}
	for _, d := range docs {
		titles[d.Slug] = d.Title
	}
	for _, d := range docs {
		d.Body = template.HTML(reLinkMD.ReplaceAllStringFunc(string(d.Body), func(m string) string {
			g := reLinkMD.FindStringSubmatch(m)
			slug, fragment, text := g[1], g[2], g[3]
			title, known := titles[slug]
			if !known {
				return m
			}
			target := root + "/comprendre/" + slug + "/"
			if s := topicDuDoc(slug); s != nil {
				target = root + "/" + s.URL()
			}
			if text == slug+".md" || text == "docs/"+slug+".md" {
				text = title
			}
			return `<a href="` + target + fragment + `">` + text + `</a>`
		}))
	}
}

var shortCofog = map[string]string{
	"GF10": "Protection sociale", "GF07": "Santé", "GF01": "Services généraux et dette",
	"GF04": "Économie, transports, agriculture", "GF09": "École et université", "GF02": "Défense",
	"GF03": "Police, justice, prisons", "GF08": "Culture, sport, loisirs", "GF06": "Logement, équipements",
	"GF05": "Environnement",
}

// couleurCofog : une teinte par fonction — qualitative, pas un dégradé : les
// dix fonctions sont dix catégories côte à côte, pas une grandeur qui varie.
var colorCofog = map[string]string{
	"GF10": "#125863", "GF07": "#5FA3AD", "GF01": "#8C8577", "GF04": "#C08A3E",
	"GF09": "#5B7CA6", "GF02": "#6E4B3A", "GF03": "#8770A0", "GF08": "#B15E4A",
	"GF06": "#A98C6B", "GF05": "#4C8F62",
}

// heroThousand : le graphique d'ouverture de l'accueil. Ce qui frappe à la
// première seconde et donne envie de creuser : sur 1 000 € de dépense publique,
// 415 vont à la protection sociale, 31 à la police et à la justice. Une
// mosaïque de 1 000 carrés — un par euro — sur ordinateur ; les mêmes chiffres
// en barres sur mobile, où 1 000 carrés seraient illisibles. Chaque ligne de
// la légende mène à la page de la fonction. Barres à l'échelle de la plus
// grande, en gris ; la première en encre, parce qu'elle sert d'étalon.
func heroThousand(a *DataHome, root string) template.HTML {
	if a == nil || len(a.Functions) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<figure class="hero-mille"><figcaption><span class="sur">Le budget réel · %d</span>`+
		`<strong>Sur 1&nbsp;000&nbsp;€ de dépense publique</strong></figcaption>`, a.Year)
	b.WriteString(`<div class="mille-corps">`)
	b.WriteString(mosaicThousand(a))
	b.WriteString(`<ol>`)
	for i, f := range a.Functions {
		name := shortCofog[f.Code]
		if name == "" {
			name = f.Label
		}
		cl := ""
		if i == 0 {
			cl = ` class="etalon"`
		}
		fmt.Fprintf(&b, `<li%s><a href="%s/%s"><i class="pastille" aria-hidden="true" style="background:%s"></i>`+
			`<span class="l">%s</span><span class="b" aria-hidden="true"><i style="width:%.1f%%"></i></span>`+
			`<span class="v">%d&nbsp;€</span></a></li>`,
			cl, root, f.URL(), colorCofog[f.Code], template.HTMLEscapeString(name), f.Width, f.PerThousand)
	}
	b.WriteString(`</ol></div>`)
	fmt.Fprintf(&b, `<p class="pied-mille">Un carré = 1&nbsp;€. Dépense constatée de l'État, de la Sécurité sociale et des collectivités&nbsp;: %s&nbsp;Md€. Eurostat / Insee. <a href="%s/argent-public/">Le détail →</a></p></figure>`,
		Decimal(a.TotalBillions, 1), root)
	return template.HTML(b.String())
}

// mosaicThousand : 1 000 carrés, un par euro, dans l'ordre des fonctions —
// visible sur ordinateur seulement (CSS), la légende ci-contre reste la
// version accessible et cliquable des mêmes chiffres.
func mosaicThousand(a *DataHome) string {
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
	for _, f := range a.Functions {
		color := colorCofog[f.Code]
		for n := 0; n < f.PerThousand && i < cols*rows; n++ {
			x, y := float64(i%cols)*cell, float64(i/cols)*cell
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="1.4" fill="%s"/>`,
				x+gap/2, y+gap/2, cell-gap, cell-gap, color)
			i++
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}
