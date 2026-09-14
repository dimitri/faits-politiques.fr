// Package macro charge les grandes séries nationales : dette, dépenses par
// fonction, recettes fiscales, chômage, pauvreté, minima sociaux.
//
// Elles servent à situer une présidence ou un gouvernement dans son époque, et
// à rien d'autre. Une courbe de dette qui monte sous une présidence ne dit pas
// que cette présidence l'a fait monter : les décisions produisent leurs effets
// avec retard, et les chocs extérieurs ne demandent l'avis de personne. La
// frise situe, elle n'explique pas.
package macro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "macro-v1"

// Eurostat republie les comptes nationaux transmis par l'INSEE, sous une forme
// stable, documentée et comparable d'une année sur l'autre depuis 1995. C'est
// la raison de le préférer aux fichiers annuels épars : une série longue vaut
// mieux que trente fichiers qu'il faudrait raccorder soi-même.
var SourceEurostat = archive.Source{
	Slug: "eurostat", Label: "Eurostat — comptes nationaux et conditions de vie",
	Publisher: "Eurostat / Commission européenne", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Réutilisation autorisée avec mention de la source (décision 2011/833/UE)",
	ReuseClass:  "OPEN",
	Attribution: "Source : Eurostat, d'après les comptes nationaux transmis par l'INSEE",
	Cadence:     "annuelle, avec révisions",
	Notes: "Les séries sont révisées : une valeur peut changer d'un millésime à " +
		"l'autre. Le chargement reconstruit donc tout, plutôt que de compléter.",
}

var SourceCNAF = archive.Source{
	Slug: "cnaf", Label: "CNAF — allocataires du RSA",
	Publisher: "Caisse nationale des allocations familiales", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte",
	ReuseClass:  "OPEN",
	Attribution: "Source : CNAF, données nationales du RSA",
	Cadence:     "mensuelle",
	Notes: "Régime général uniquement : les allocataires relevant de la MSA ne " +
		"sont pas comptés. Série disponible à partir de juin 2016 seulement.",
}

const eurostatBase = "https://ec.europa.eu/eurostat/api/dissemination/statistics/1.0/data/"

type serie struct {
	Code, Label, Unite, Famille, Cofog, Definition string
	Requete                                        string
}

// Les dix fonctions de la nomenclature COFOG, telle qu'Eurostat la publie.
// C'est la seule ventilation des dépenses publiques qui soit à la fois
// officielle, stable dans le temps et comparable : les « missions » du budget
// de l'État changent de périmètre à chaque réforme.
var cofog = []struct{ code, label string }{
	{"GF01", "Services généraux des administrations publiques"},
	{"GF02", "Défense"},
	{"GF03", "Ordre et sécurité publics"},
	{"GF04", "Affaires économiques"},
	{"GF05", "Protection de l'environnement"},
	{"GF06", "Logement et équipements collectifs"},
	{"GF07", "Santé"},
	{"GF08", "Loisirs, culture et culte"},
	{"GF09", "Enseignement"},
	{"GF10", "Protection sociale"},
}

func series() []serie {
	out := []serie{
		{
			Code: "dette.publique.meur", Label: "Dette publique", Unite: "MEUR", Famille: "DETTE",
			Definition: "Dette brute consolidée des administrations publiques au sens de Maastricht, " +
				"en millions d'euros courants. Couvre l'État, les collectivités et la sécurité sociale.",
			Requete: "gov_10dd_edpt1?format=JSON&lang=FR&geo=FR&na_item=GD&sector=S13&unit=MIO_EUR",
		},
		{
			Code: "dette.publique.pib", Label: "Dette publique en part du PIB", Unite: "PCT_PIB", Famille: "DETTE",
			Definition: "Même dette, rapportée au produit intérieur brut. C'est le ratio qui sert " +
				"aux comparaisons, la valeur en euros n'étant pas comparable d'une décennie à l'autre.",
			Requete: "gov_10dd_edpt1?format=JSON&lang=FR&geo=FR&na_item=GD&sector=S13&unit=PC_GDP",
		},
		{
			Code: "solde.public.pib", Label: "Solde public en part du PIB", Unite: "PCT_PIB", Famille: "DETTE",
			Definition: "Capacité (+) ou besoin (−) de financement des administrations publiques, " +
				"rapporté au PIB. C'est le « déficit » au sens des critères européens.",
			Requete: "gov_10dd_edpt1?format=JSON&lang=FR&geo=FR&na_item=B9&sector=S13&unit=PC_GDP",
		},
		{
			Code: "recettes.fiscales.meur", Label: "Recettes fiscales et cotisations", Unite: "MEUR", Famille: "RECETTE",
			Definition: "Impôts et cotisations sociales effectives perçus par les administrations " +
				"publiques, nets des montants non recouvrables, en millions d'euros courants.",
			Requete: "gov_10a_taxag?format=JSON&lang=FR&geo=FR&sector=S13&unit=MIO_EUR&na_item=D2_D5_D91_D61_M_D995",
		},
		{
			Code: "chomeurs.nombre", Label: "Chômeurs", Unite: "MILLIERS", Famille: "EMPLOI",
			Definition: "Personnes de 15 à 74 ans sans emploi, disponibles et en recherche active, " +
				"au sens du Bureau international du travail. Ce n'est PAS le nombre d'inscrits " +
				"à France Travail, qui obéit à des règles administratives différentes.",
			Requete: "lfsa_ugan?format=JSON&lang=FR&geo=FR&sex=T&age=Y15-74&unit=THS_PER&citizen=TOTAL",
		},
		{
			Code: "chomage.taux", Label: "Taux de chômage", Unite: "PCT", Famille: "EMPLOI",
			Definition: "Part des chômeurs au sens du BIT dans la population active.",
			Requete:    "une_rt_a?format=JSON&lang=FR&geo=FR&sex=T&age=Y15-74&unit=PC_ACT",
		},
		// « Capacités excédentaires sur le marché du travail » (labour market
		// slack) : la mesure qu'Eurostat construit précisément parce que le
		// chômage au sens du BIT laisse dehors des gens qui, de l'avis même des
		// offices statistiques, cherchent ou voudraient un emploi. Quatre
		// composantes qui se somment exactement au total SLACK ci-dessous.
		{
			Code: "chomage.sous_emploi_temps_partiel", Label: "Personnes sous-employées à temps partiel",
			Unite: "MILLIERS", Famille: "EMPLOI",
			Definition: "Personnes en emploi à temps partiel qui voudraient travailler davantage " +
				"et sont disponibles pour le faire. Elles ont un emploi : le chômage au sens du " +
				"BIT ne les compte pas.",
			Requete: "lfsi_sla_a?format=JSON&lang=FR&geo=FR&sex=T&age=Y15-74&unit=THS_PER&wstatus=UEMP_PT",
		},
		{
			Code: "chomage.cherchent_indisponibles", Label: "Cherchent un emploi mais indisponibles",
			Unite: "MILLIERS", Famille: "EMPLOI",
			Definition: "Personnes qui cherchent activement un emploi mais ne peuvent pas commencer " +
				"dans les deux semaines — une garde d'enfant à trouver, une formation en cours. Le " +
				"critère de disponibilité immédiate du BIT les exclut du chômage.",
			Requete: "lfsi_sla_a?format=JSON&lang=FR&geo=FR&sex=T&age=Y15-74&unit=THS_PER&wstatus=SEEK_NAVL",
		},
		{
			Code: "chomage.disponibles_sans_recherche", Label: "Disponibles mais ne cherchant pas — le halo",
			Unite: "MILLIERS", Famille: "EMPLOI",
			Definition: "Personnes disponibles pour travailler mais qui n'ont pas cherché activement " +
				"dans le mois — parce qu'elles pensent ne rien trouver, ou pour toute autre raison. " +
				"L'INSEE et Eurostat nomment ce groupe le « halo autour du chômage ». Ni au chômage " +
				"BIT, ni comptées comme actives.",
			Requete: "lfsi_sla_a?format=JSON&lang=FR&geo=FR&sex=T&age=Y15-74&unit=THS_PER&wstatus=NSEEK_AVL",
		},
		{
			Code: "chomage.halo_total", Label: "Capacités excédentaires sur le marché du travail",
			Unite: "MILLIERS", Famille: "EMPLOI",
			Definition: "Somme du chômage au sens du BIT et des trois catégories ci-dessus " +
				"(sous-emploi à temps partiel, recherche sans disponibilité immédiate, disponibilité " +
				"sans recherche active). C'est la mesure la plus large qu'Eurostat publie du " +
				"« manque de travail » — largement supérieure au seul chômage BIT, et c'est " +
				"précisément pourquoi elle existe comme indicateur à part.",
			Requete: "lfsi_sla_a?format=JSON&lang=FR&geo=FR&sex=T&age=Y15-74&unit=THS_PER&wstatus=SLACK",
		},
		{
			Code: "pauvrete.nombre", Label: "Personnes sous le seuil de pauvreté", Unite: "MILLIERS", Famille: "PAUVRETE",
			Definition: "Personnes vivant dans un ménage dont le revenu disponible par unité de " +
				"consommation est inférieur à 60 % de la médiane nationale. Le seuil est relatif : " +
				"il bouge avec le niveau de vie médian, et une baisse du médian peut faire reculer " +
				"le nombre de pauvres sans que personne se soit enrichi.",
			Requete: "ilc_li02?format=JSON&lang=FR&geo=FR&unit=THS_PER&rskpovth=B_60&sex=T&age=TOTAL&statinfo=MED_EI",
		},
		{
			Code: "pauvrete.taux", Label: "Taux de pauvreté", Unite: "PCT", Famille: "PAUVRETE",
			Definition: "Même définition, exprimée en part de la population.",
			Requete:    "ilc_li02?format=JSON&lang=FR&geo=FR&unit=PC&rskpovth=B_60&sex=T&age=TOTAL&statinfo=MED_EI",
		},
	}

	// Le compte des sociétés, dans le même cadre comptable que la dette et les
	// dépenses publiques. C'est ce qui rend la corrélation possible : dividendes
	// versés, impôts acquittés, rémunérations et valeur ajoutée sont mesurés par
	// l'INSEE selon les mêmes conventions que le solde public, et sur la même
	// période.
	//
	// Ces séries remontent à 1971 — plus de cinquante ans, contre trente pour
	// les finances publiques. Elles décrivent TOUTES les sociétés résidentes, et
	// non les seules grandes entreprises : c'est un agrégat, il ne se rapporte à
	// aucune société identifiable et ne remplace pas ce qu'un rapport annuel
	// publie sur une société donnée.
	entreprise := []struct{ code, label, naItem, secteur, sens, definition string }{
		{"dividendes.verses.snf", "Dividendes versés par les sociétés non financières",
			"D42", "S11", "PAID",
			"Revenus distribués des sociétés (D.42) versés par les sociétés non financières " +
				"résidentes : dividendes et prélèvements sur les revenus des quasi-sociétés. " +
				"Versés à TOUS les détenteurs — ménages, autres sociétés, non-résidents — et " +
				"non aux seuls actionnaires français."},
		{"dividendes.verses.sf", "Dividendes versés par les sociétés financières",
			"D42", "S12", "PAID",
			"Même agrégat pour les banques et assurances résidentes."},
		{"dividendes.recus.menages", "Dividendes reçus par les ménages",
			"D42", "S14_S15", "RECV",
			"Part des revenus distribués qui arrive aux ménages résidents. L'écart avec le " +
				"total versé mesure ce qui va aux autres sociétés et au reste du monde."},
		{"impots.revenu.payes.snf", "Impôts sur le revenu payés par les sociétés non financières",
			"D51", "S11", "PAID",
			"Impôts courants sur le revenu et le patrimoine (D.51) acquittés par les sociétés " +
				"non financières. C'est le montant effectivement payé, du point de vue des " +
				"sociétés."},
		{"remuneration.salaries.snf", "Rémunération des salariés versée par les sociétés non financières",
			"D1", "S11", "PAID",
			"Salaires bruts et cotisations sociales employeurs (D.1). C'est la masse salariale " +
				"au sens des comptes nationaux."},
		{"ebe.snf", "Excédent brut d'exploitation des sociétés non financières",
			"B2A3G", "S11", "PAID",
			"Ce qui reste de la valeur ajoutée après rémunération des salariés et impôts sur " +
				"la production. Sert de dénominateur : rapporter les dividendes à l'EBE dit " +
				"quelle part du profit est distribuée."},
		{"valeur.ajoutee.snf", "Valeur ajoutée des sociétés non financières",
			"B1G", "S11", "PAID",
			"Production moins consommations intermédiaires. Le partage de cette valeur entre " +
				"salaires, impôts et profits est la question que ces séries permettent de poser."},
	}
	for _, e := range entreprise {
		out = append(out, serie{
			Code: e.code, Label: e.label, Unite: "MEUR", Famille: "ENTREPRISES",
			Definition: e.definition,
			Requete: "nasa_10_nf_tr?format=JSON&lang=FR&geo=FR&unit=CP_MEUR&na_item=" +
				e.naItem + "&sector=" + e.secteur + "&direct=" + e.sens,
		})
	}

	// L'impôt sur les sociétés vu du côté de l'État : ce que les administrations
	// publiques ENCAISSENT. Il ne coïncide pas avec le D.51 payé par les sociétés
	// non financières ci-dessus, qui exclut les sociétés financières et suit une
	// autre convention de rattachement. Les deux sont publiés, les deux sont
	// justes, et l'écart entre eux est un fait à montrer plutôt qu'à masquer.
	out = append(out, serie{
		Code: "impot.societes.encaisse", Label: "Impôt sur les bénéfices des sociétés encaissé",
		Unite: "MEUR", Famille: "ENTREPRISES",
		Definition: "Recettes des administrations publiques au titre de l'impôt sur le revenu " +
			"ou les bénéfices des sociétés (D.51B). Point de vue de l'État, à ne pas confondre " +
			"avec le D.51 payé par les seules sociétés non financières.",
		Requete: "gov_10a_taxag?format=JSON&lang=FR&geo=FR&sector=S13&unit=MIO_EUR&na_item=D51B",
	})

	// Les sous-secteurs des administrations publiques.
	//
	// C'est le SEUL endroit où l'État et la Sécurité sociale se mesurent sur la
	// même règle. Un solde de loi de finances et un solde de loi de financement
	// ne se comparent pas : ils ne sont pas dans la même comptabilité, ils n'ont
	// pas le même périmètre, et l'un autorise la dépense quand l'autre la prévoit.
	// La comptabilité nationale, elle, les mesure tous les deux en droits
	// constatés sur un périmètre consolidé — et donne le fait central du sujet :
	// en 2025, les administrations de sécurité sociale dépensent 803,5 Md€ contre
	// 680,8 Md€ pour l'administration centrale, et le besoin de financement est
	// celui de l'État (−130,2 Md€) bien plus que celui du social (−6,7 Md€).
	//
	// S1312 (États fédérés) n'existe pas en France : les quatre secteurs
	// ci-dessous épuisent le champ.
	secteurs := []struct{ code, label, definition string }{
		{"S13", "toutes administrations publiques",
			"État, organismes divers, collectivités et sécurité sociale réunis, APRÈS " +
				"consolidation des flux entre eux. C'est le périmètre du déficit au sens " +
				"de Maastricht."},
		{"S1311", "administration centrale",
			"L'État et les organismes divers d'administration centrale. Ce n'est PAS le " +
				"budget général de l'État : les opérateurs y sont inclus, et les flux vers " +
				"les autres administrations ne sont pas retirés."},
		{"S1313", "administrations publiques locales",
			"Communes, départements, régions, groupements et leurs satellites."},
		{"S1314", "administrations de sécurité sociale",
			"Régimes obligatoires, mais AUSSI l'assurance chômage et les retraites " +
				"complémentaires, que la loi de financement de la sécurité sociale ne " +
				"couvre pas. Le périmètre est donc plus large que celui de la LFSS."},
	}
	agregats := []struct{ code, naItem, label, definition string }{
		{"depense", "TE", "Dépenses totales",
			"Dépense totale au sens du SEC 2010 : prestations, rémunérations, " +
				"consommations, investissement et intérêts."},
		{"recette", "TR", "Recettes totales",
			"Recette totale au sens du SEC 2010 : impôts, cotisations, ventes et " +
				"transferts reçus."},
		{"solde", "B9", "Capacité (+) ou besoin (−) de financement",
			"Recettes moins dépenses. Un nombre négatif est un besoin de financement, " +
				"c'est-à-dire un déficit."},
	}
	for _, sec := range secteurs {
		for _, a := range agregats {
			out = append(out, serie{
				Code:  a.code + "." + sec.code,
				Label: a.label + " — " + sec.label,
				Unite: "MEUR", Famille: "FINANCES_PUBLIQUES",
				Definition: a.definition + " Sous-secteur " + sec.code + " : " + sec.definition +
					" Comptabilité NATIONALE (SEC 2010), droits constatés, sous-secteur " +
					"consolidé : ces montants NE SONT PAS COMPARABLES à un solde de loi de " +
					"finances ou de loi de financement, qui relèvent de la comptabilité " +
					"budgétaire. La comptabilité nationale ne connaît que l'exécuté, " +
					"retraité, et à dix-huit mois de délai pour les comptes définitifs.",
				Requete: "gov_10a_main?format=JSON&lang=FR&geo=FR&unit=MIO_EUR&na_item=" +
					a.naItem + "&sector=" + sec.code,
			})
		}
	}

	// ESSPROS : la structure de FINANCEMENT de la protection sociale, depuis 1990.
	//
	// C'est la mesure directe de la fiscalisation du modèle social français, et
	// elle est comparable en Europe. Trente-quatre points annuels permettent de
	// DATER le basculement des cotisations vers l'impôt affecté plutôt que de
	// l'affirmer.
	//
	// Périmètre ESSPROS = protection sociale, assurance chômage et retraites
	// complémentaires comprises. Plus large que la LFSS : rapprocher ces montants
	// d'un solde de LFSS serait une faute.
	//
	// spr_exp_sum, que la documentation ancienne cite partout, est RETIRÉ et
	// renvoie 404 ; les dépenses sont sous spr_exp_func et ses déclinaisons.
	financement := []struct{ code, sptype, label, definition string }{
		{"protection.financement.total", "TOTAL", "Financement total de la protection sociale",
			"Toutes ressources du système de protection sociale."},
		{"protection.financement.cotisations.employeurs", "SCO_EMPL",
			"Cotisations à la charge des employeurs",
			"Cotisations sociales versées par les employeurs."},
		{"protection.financement.cotisations.protegees", "SCO_PER_PRO",
			"Cotisations à la charge des personnes protégées",
			"Cotisations des salariés, indépendants, retraités et autres assurés."},
		{"protection.financement.impot.affecte", "GOV_GEN_EM",
			"Contributions publiques — recettes fiscales affectées",
			"Impôts et taxes affectés à la protection sociale : c'est la CSG et les " +
				"fractions de TVA transférées. Cette ligne est le canal État → Sécurité " +
				"sociale vu de l'extérieur."},
		{"protection.financement.impot.general", "GOV_GEN_REVGEN",
			"Contributions publiques — recettes fiscales générales",
			"Financement par le budget général, sans affectation."},
	}
	for _, f := range financement {
		out = append(out, serie{
			Code: f.code, Label: f.label, Unite: "MEUR", Famille: "PROTECTION_SOCIALE",
			Definition: f.definition + " Source ESSPROS (Eurostat), champ PROTECTION SOCIALE : " +
				"assurance chômage et retraites complémentaires comprises, donc plus large " +
				"que la loi de financement de la sécurité sociale.",
			Requete: "spr_rec_sumt?format=JSON&lang=FR&geo=FR&unit=MIO_EUR&sptype=" + f.sptype,
		})
	}

	// ESSPROS : la DÉPENSE par fonction, pas seulement le financement — la
	// ligne que docs/budget-donnees.md § 4.5 signalait « non chargée à ce
	// jour ». Deux fonctions seulement, celles qui pèsent le plus dans les
	// notes docs/chomage-donnees.md et docs/retraite-donnees.md : chômage et
	// vieillesse. spdep=SPR (prestations de protection sociale, la ligne la
	// plus large de la nomenclature) et spdepm=TOTAL (moyens-testées et non
	// moyens-testées réunies) évitent d'avoir à sommer des sous-catégories
	// dont l'emboîtement n'est pas garanti la même façon pour toutes les
	// fonctions.
	depenseFonction := []struct{ code, spr, label, definition string }{
		{"protection.depense.chomage", "spr_exp_fun", "Dépense de la fonction chômage",
			"Prestations de protection sociale versées au titre du risque chômage : " +
				"indemnisation, insertion, retraite anticipée pour raison de marché du " +
				"travail — pas seulement l'assurance chômage au sens de l'Unédic."},
		{"protection.depense.vieillesse", "spr_exp_fol", "Dépense de la fonction vieillesse",
			"Prestations de protection sociale versées au titre du risque vieillesse : " +
				"pensions de retraite, y compris anticipées et partielles, allocations " +
				"dépendance liées à l'âge."},
	}
	for _, d := range depenseFonction {
		out = append(out, serie{
			Code: d.code, Label: d.label, Unite: "MEUR", Famille: "PROTECTION_SOCIALE",
			Definition: d.definition + " Source ESSPROS (Eurostat), champ PROTECTION SOCIALE — " +
				"plus large que la loi de financement de la sécurité sociale (assurance " +
				"chômage et retraites complémentaires comprises).",
			Requete: d.spr + "?format=JSON&lang=FR&geo=FR&spdep=SPR&spdepm=TOTAL&unit=MIO_EUR",
		})
	}

	for _, c := range cofog {
		out = append(out, serie{
			Code: "depense." + c.code, Label: "Dépense publique — " + c.label,
			Unite: "MEUR", Famille: "DEPENSE", Cofog: c.code,
			Definition: "Dépense totale des administrations publiques pour la fonction « " +
				c.label + " » (classification COFOG), en millions d'euros courants. " +
				"Toutes administrations confondues, pas seulement l'État.",
			Requete: "gov_10a_exp?format=JSON&lang=FR&geo=FR&sector=S13&unit=MIO_EUR&na_item=TE&cofog99=" + c.code,
		})
	}
	return out
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceEurostat)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	var nSeries, nValeurs int
	for _, s := range series() {
		f, err := arch.Fetch(ctx, srcID, runID, eurostatBase+s.Requete, ".json")
		if err != nil {
			return fail(fmt.Errorf("%s : %w", s.Code, err))
		}
		valeurs, err := lireEurostat(f.Path)
		if err != nil {
			return fail(fmt.Errorf("%s : %w", s.Code, err))
		}
		if len(valeurs) == 0 {
			// Une série vide est un signal, pas un détail : la requête est
			// fausse ou la dimension a changé de nom chez Eurostat.
			return fail(fmt.Errorf("%s : aucune valeur — requête ou dimension à revoir", s.Code))
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ref.macro_serie (code, label, unite, producteur, definition, famille, cofog, url)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (code) DO UPDATE SET label=EXCLUDED.label, unite=EXCLUDED.unite,
			  definition=EXCLUDED.definition, famille=EXCLUDED.famille, url=EXCLUDED.url`,
			s.Code, s.Label, s.Unite, "Eurostat / INSEE", s.Definition, s.Famille,
			nulStr(s.Cofog), eurostatBase+s.Requete); err != nil {
			return fail(err)
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM core.macro_value WHERE serie_code = $1`, s.Code); err != nil {
			return fail(err)
		}
		for _, v := range valeurs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.macro_value (serie_code, annee, valeur, source_id)
				VALUES ($1,$2,$3,$4)`, s.Code, v.annee, v.valeur, srcID); err != nil {
				return fail(err)
			}
			nValeurs++
		}
		nSeries++
		fmt.Printf("    %-26s %3d valeurs  %d-%d\n", s.Code, len(valeurs),
			valeurs[0].annee, valeurs[len(valeurs)-1].annee)
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"series": nSeries, "valeurs": nValeurs}, "")
	fmt.Printf("  Eurostat : %d séries, %d valeurs\n", nSeries, nValeurs)
	return nil
}

type point struct {
	annee  int
	valeur float64
}

// lireEurostat décode le format JSON-stat : les valeurs sont indexées par la
// position de l'année dans la dimension temps, pas par l'année elle-même.
func lireEurostat(path string) ([]point, error) {
	b, err := lireFichier(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Error     []struct{ Label string } `json:"error"`
		Value     map[string]float64       `json:"value"`
		Dimension struct {
			Time struct {
				Category struct {
					Index map[string]int `json:"index"`
				} `json:"category"`
			} `json:"time"`
		} `json:"dimension"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	if len(doc.Error) > 0 {
		return nil, fmt.Errorf("Eurostat : %s", doc.Error[0].Label)
	}
	var out []point
	for annee, idx := range doc.Dimension.Time.Category.Index {
		v, ok := doc.Value[strconv.Itoa(idx)]
		if !ok {
			// Année sans valeur : elle reste absente. Rien n'est interpolé.
			continue
		}
		n, err := strconv.Atoi(annee)
		if err != nil {
			continue
		}
		out = append(out, point{n, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].annee < out[j].annee })
	return out, nil
}

func lireFichier(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func nulStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// IngestRSA charge le nombre national de foyers allocataires du RSA.
//
// Trois décisions, écrites ici parce qu'elles ne sont pas dans la source :
//
//  1. Le mois retenu est DÉCEMBRE. Le RSA est publié chaque mois et varie avec
//     la saison ; prendre la moyenne annuelle mélangerait des mois de nature
//     différente, prendre janvier daterait la valeur de l'année précédente.
//  2. Tous les types sont additionnés — RSA socle et RSA majoré — parce que la
//     question posée est « combien de foyers perçoivent le RSA ».
//  3. L'unité est le FOYER allocataire, pas la personne : un foyer couvre en
//     moyenne un peu plus de deux personnes.
//
// Limite de la source : régime général uniquement, la MSA n'y est pas, et la
// série ne commence qu'en juin 2016. Elle ne permet donc aucune comparaison
// avec les années Chirac ou Sarkozy, ni avec le RMI qui a précédé le RSA.
func IngestRSA(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceCNAF)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	const base = "https://data.caf.fr/api/explore/v2.1/catalog/datasets/rsa_s_type_age_1_nat/records"

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO ref.macro_serie (code, label, unite, producteur, definition, famille, url)
		VALUES ('rsa.foyers', 'Foyers allocataires du RSA', 'FOYERS',
		        'CNAF',
		        'Nombre de foyers percevant le revenu de solidarité active au mois de '
		        'décembre, régime général, tous types de RSA additionnés. L''unité est le '
		        'FOYER et non la personne. Les allocataires relevant de la MSA ne sont pas '
		        'comptés. Série disponible à partir de 2016 seulement.',
		        'PAUVRETE', $1)
		ON CONFLICT (code) DO UPDATE SET definition = EXCLUDED.definition`, base); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM core.macro_value WHERE serie_code = 'rsa.foyers'`); err != nil {
		return fail(err)
	}

	var n int
	for annee := 2016; annee <= 2025; annee++ {
		url := fmt.Sprintf("%s?select=sum(indfoy_rsa)%%20as%%20foyers&where=dtreffre%%3Ddate%%27%d-12-01%%27&limit=1",
			base, annee)
		f, err := arch.Fetch(ctx, srcID, runID, url, ".json")
		if err != nil {
			return fail(fmt.Errorf("RSA %d : %w", annee, err))
		}
		b, err := lireFichier(f.Path)
		if err != nil {
			return fail(err)
		}
		var doc struct {
			Results []struct {
				Foyers *float64 `json:"foyers"`
			} `json:"results"`
		}
		if err := json.Unmarshal(b, &doc); err != nil {
			return fail(err)
		}
		if len(doc.Results) == 0 || doc.Results[0].Foyers == nil {
			continue // décembre non encore publié : l'année reste absente
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.macro_value (serie_code, annee, valeur, source_id)
			VALUES ('rsa.foyers', $1, $2, $3)`, annee, *doc.Results[0].Foyers, srcID); err != nil {
			return fail(err)
		}
		n++
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"annees": n}, "")
	fmt.Printf("  CNAF : %d années de foyers RSA\n", n)
	return nil
}
