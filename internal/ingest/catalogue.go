package ingest

import (
	"context"
	"path/filepath"

	"github.com/faits-politiques/faits-politiques/internal/agriculture"
	"github.com/faits-politiques/faits-politiques/internal/aides"
	"github.com/faits-politiques/faits-politiques/internal/an"
	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/associations"
	"github.com/faits-politiques/faits-politiques/internal/budget"
	"github.com/faits-politiques/faits-politiques/internal/campagne"
	"github.com/faits-politiques/faits-politiques/internal/carto"
	"github.com/faits-politiques/faits-politiques/internal/communes"
	"github.com/faits-politiques/faits-politiques/internal/damir"
	"github.com/faits-politiques/faits-politiques/internal/decp"
	"github.com/faits-politiques/faits-politiques/internal/dette"
	"github.com/faits-politiques/faits-politiques/internal/dossiers"
	"github.com/faits-politiques/faits-politiques/internal/eau"
	"github.com/faits-politiques/faits-politiques/internal/ecologie"
	"github.com/faits-politiques/faits-politiques/internal/education"
	"github.com/faits-politiques/faits-politiques/internal/entreprises"
	"github.com/faits-politiques/faits-politiques/internal/europe"
	"github.com/faits-politiques/faits-politiques/internal/fiscalite"
	"github.com/faits-politiques/faits-politiques/internal/geo"
	"github.com/faits-politiques/faits-politiques/internal/hatvp"
	"github.com/faits-politiques/faits-politiques/internal/hydro"
	"github.com/faits-politiques/faits-politiques/internal/immigration"
	"github.com/faits-politiques/faits-politiques/internal/international"
	"github.com/faits-politiques/faits-politiques/internal/jeunesse"
	"github.com/faits-politiques/faits-politiques/internal/jorf"
	"github.com/faits-politiques/faits-politiques/internal/macro"
	"github.com/faits-politiques/faits-politiques/internal/numerique"
	"github.com/faits-politiques/faits-politiques/internal/paie"
	"github.com/faits-politiques/faits-politiques/internal/prefets"
	"github.com/faits-politiques/faits-politiques/internal/presidentielle"
	"github.com/faits-politiques/faits-politiques/internal/sante"
	"github.com/faits-politiques/faits-politiques/internal/senat"
	"github.com/faits-politiques/faits-politiques/internal/vieillesse"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Catégories : un regroupement thématique des sources, pour la navigation en
// ligne de commande (fpctl ingest <catégorie> ...) — n'affecte ni l'ordre
// d'exécution ni les dépendances entre sources, seulement leur présentation.
const (
	CategorieParlement     = "parlement"
	CategorieCollectivites = "collectivites"
	CategorieBudget        = "budget"
	CategorieSocial        = "social"
	CategorieEnvironnement = "environnement"
	CategorieEconomie      = "economie"
	CategorieTransparence  = "transparence"
	CategorieInternational = "international"
	CategorieSysteme       = "systeme"
)

// Categories : l'ordre de présentation des catégories (fpctl ingest, sans
// argument, les liste dans cet ordre).
func Categories() []string {
	return []string{
		CategorieParlement, CategorieCollectivites, CategorieBudget, CategorieSocial,
		CategorieEnvironnement, CategorieEconomie, CategorieTransparence,
		CategorieInternational, CategorieSysteme,
	}
}

// Source : une entrée du catalogue — le remplacement de chaque valeur de
// l'ancien -only, nommée et rangée dans une catégorie plutôt qu'un simple
// mot posé sur un flag. C'est la RÉFÉRENCE UNIQUE : cmd/fpctl construit son
// arbre de sous-commandes en itérant ce catalogue, il ne le recopie jamais —
// deux listes à tenir synchronisées était précisément ce que l'ancien -only
// évitait en restant un flag plat ; un catalogue au même endroit obtient la
// même garantie sans en payer le prix (une aide en ligne de commande plate,
// une soixantaine de valeurs à deviner).
type Source struct {
	Nom, Categorie, Description string
	Executer                    func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error
	// Dependances : les autres sources qui doivent avoir réussi avant
	// celle-ci — vide pour la quasi-totalité du catalogue (des connecteurs
	// indépendants les uns des autres), déclarée seulement pour le socle
	// parlementaire (download, partis, normalize, carto, senat, europe,
	// themes), le seul audité et exécuté via internal/pipeline.Registre
	// (résolution automatique, concurrence, simulation) — voir
	// registreParlement() dans ingest.go. Une source hors de ce socle garde
	// l'ordre implicite qu'elle a toujours eu (le nom de sa catégorie ne
	// suffit pas à garantir qu'elle est indépendante des autres).
	Dependances []string
}

// SourcesDeCategorie : les sources d'une catégorie, dans l'ordre du
// catalogue — celui-ci place déjà les préalables connus avant ce qui en
// dépend (ex. cog avant rne/epci/collectivites), voir les commentaires
// ci-dessous ; « all » sur une catégorie les exécute dans cet ordre.
func SourcesDeCategorie(categorie string) []Source {
	var out []Source
	for _, s := range catalogue {
		if s.Categorie == categorie {
			out = append(out, s)
		}
	}
	return out
}

// SourceParNom : résout un nom de source (ce qu'on tapait après -only=)
// dans le catalogue, quelle que soit sa catégorie.
func SourceParNom(nom string) (Source, bool) {
	for _, s := range catalogue {
		if s.Nom == nom {
			return s, true
		}
	}
	return Source{}, false
}

// TousLesNoms : pour les messages d'erreur (« source inconnue, attendu : »).
func TousLesNoms() []string {
	noms := make([]string, len(catalogue))
	for i, s := range catalogue {
		noms[i] = s.Nom
	}
	return noms
}

var catalogue = []Source{
	// --- parlement : Assemblée nationale, Sénat, Parlement européen, votes,
	// textes, élections nationales.
	{Nom: "download", Categorie: CategorieParlement,
		Description: "téléchargement et scellement des données de l'Assemblée (AMO, scrutins, dossiers)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return telechargerAssemblee(ctx, pool, arch)
		}},
	{Nom: "partis", Categorie: CategorieParlement, Dependances: []string{"download"},
		Description: "référentiels sur les organisations politiques (comptes de campagne, PopuList, CHES)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return ingestPartis(ctx, pool, arch)
		}},
	{Nom: "normalize", Categorie: CategorieParlement, Dependances: []string{"download", "partis"},
		Description: "normalisation raw -> core (Assemblée)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return normaliserAssemblee(ctx, pool)
		}},
	{Nom: "carto", Categorie: CategorieParlement, Dependances: []string{"normalize"},
		Description: "cartographie éditoriale (partis -> groupes, gouvernements, présidences)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return cartographie(ctx, pool)
		}},
	{Nom: "senat", Categorie: CategorieParlement, Dependances: []string{"normalize"},
		Description: "Sénat : scrutins, sénateurs, fusion, mandats, commissions",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return ingestSenat(ctx, pool, arch, rawDir)
		}},
	{Nom: "senat-repertoire", Categorie: CategorieParlement, Description: "répertoire des sénateurs seul",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return senat.IngestSenateurs(ctx, pool, arch)
		}},
	{Nom: "senat-fusion", Categorie: CategorieParlement, Description: "fusion des fiches de sénateurs seule",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return senat.Fusionner(ctx, pool)
		}},
	{Nom: "senat-mandats", Categorie: CategorieParlement, Description: "mandats de sénateurs seuls (recalculés depuis senat_raw)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return senat.NormalizeMandats(ctx, pool)
		}},
	{Nom: "senat-commissions", Categorie: CategorieParlement, Description: "commissions du Sénat seules",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return senat.IngestCommissions(ctx, pool, arch)
		}},
	{Nom: "senat-presentations", Categorie: CategorieParlement, Description: "présentations des dossiers du Sénat seules",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return senat.NormalizePresentations(ctx, pool)
		}},
	{Nom: "europe", Categorie: CategorieParlement, Dependances: []string{"normalize"},
		Description: "Parlement européen : scrutins et votes des eurodéputés français",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return europe.Ingest(ctx, pool, arch)
		}},
	// La couverture PARLEMENT_EUROPEEN d'un thème dépend des scrutins
	// européens déjà chargés — un thème calculé avant que l'Europe ait
	// tourné manquait cette couverture en silence (le bug qui a motivé ce
	// nœud séparé plutôt qu'un appel logé dans senat ou carto).
	{Nom: "themes", Categorie: CategorieParlement, Dependances: []string{"senat", "europe"},
		Description: "thèmes applicables aux scrutins (dépend du Sénat ET de l'Europe)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return carto.Themes(ctx, pool)
		}},
	{Nom: "amendements", Categorie: CategorieParlement, Description: "amendements et exposés sommaires",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return an.IngestAmendements(ctx, pool, arch)
		}},
	{Nom: "exposes", Categorie: CategorieParlement, Description: "exposés des motifs",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return an.IngestExposes(ctx, pool, arch)
		}},
	{Nom: "exposes-reparse", Categorie: CategorieParlement, Description: "réextraction des exposés depuis l'archive déjà scellée",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return an.ReparseExposes(ctx, pool, rawDir)
		}},
	{Nom: "interventions", Categorie: CategorieParlement, Description: "interventions en séance",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return an.IngestInterventions(ctx, pool, arch)
		}},
	{Nom: "deports", Categorie: CategorieParlement, Description: "déclarations de déport",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return an.NormalizeDeports(ctx, pool)
		}},
	{Nom: "promulgation", Categorie: CategorieParlement, Description: "rattachement des dossiers à la loi promulguée",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return an.PromulgationDossiers(ctx, pool)
		}},
	{Nom: "campagne", Categorie: CategorieParlement, Description: "comptes de campagne",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return campagne.Ingest(ctx, pool, arch)
		}},
	{Nom: "jorf", Categorie: CategorieParlement, Description: "actes nominatifs du Journal officiel (les plus récents)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return jorf.Ingest(ctx, pool, arch, 60)
		}},
	{Nom: "jorf-complet", Categorie: CategorieParlement, Description: "Journal officiel en entier, 1861-2025 (1,24 million d'actes)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return jorf.IngestComplet(ctx, pool, arch)
		}},
	{Nom: "jorf-elus", Categorie: CategorieParlement, Description: "reconnaissance des élus dans le corpus JORF déjà chargé",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return jorf.NormalizeElus(ctx, pool)
		}},
	{Nom: "jorf-gouvernement", Categorie: CategorieParlement, Description: "décrets de composition du Gouvernement (2014-2026, flux JORF)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return jorf.IngestGouvernement(ctx, pool, arch)
		}},
	{Nom: "gouvernement-membres", Categorie: CategorieParlement, Description: "membres du Gouvernement, d'après les décrets déjà scellés",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return jorf.NormalizeMembres(ctx, pool)
		}},
	{Nom: "presidentielle", Categorie: CategorieParlement, Description: "élection présidentielle, population par âge, participation comparée",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			if err := presidentielle.Ingest(ctx, pool, arch); err != nil {
				return err
			}
			if err := presidentielle.IngestPopulation(ctx, pool, arch); err != nil {
				return err
			}
			return presidentielle.IngestTurnout(ctx, pool, arch)
		}},

	// --- collectivites : communes, intercommunalités, élections locales,
	// finances locales.
	{Nom: "communes", Categorie: CategorieCollectivites,
		Description: "dimension communale complète (référentiel géographique, maires, municipales, OFGL, BANATIC, municipales 2020, collectivités, délinquance)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return dimensionLocale(ctx, pool, arch)
		}},
	{Nom: "cog", Categorie: CategorieCollectivites, Description: "référentiel géographique (COG) seul",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return communes.IngestCOG(ctx, pool, arch)
		}},
	{Nom: "rne", Categorie: CategorieCollectivites, Description: "répertoire national des élus (maires) seul",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return communes.IngestRNE(ctx, pool, arch)
		}},
	{Nom: "epci", Categorie: CategorieCollectivites, Description: "intercommunalités et compétences (BANATIC) seules",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return communes.IngestBANATIC(ctx, pool, arch)
		}},
	{Nom: "municipales2020", Categorie: CategorieCollectivites, Description: "élections municipales 2020 seules",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return communes.IngestMunicipales2020(ctx, pool, arch)
		}},
	{Nom: "collectivites", Categorie: CategorieCollectivites, Description: "comptes des régions, départements et groupements seuls",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return communes.IngestCollectivites(ctx, pool, arch)
		}},
	{Nom: "ssmsi", Categorie: CategorieCollectivites, Description: "délinquance enregistrée par commune seule",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return communes.IngestSSMSI(ctx, pool, arch)
		}},
	{Nom: "associations", Categorie: CategorieCollectivites, Description: "tissu associatif (RNA)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return associations.Ingest(ctx, pool, arch)
		}},
	{Nom: "fiscalite-locale", Categorie: CategorieCollectivites, Description: "fiscalité directe locale par mécanisme (OFGL/REI)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return communes.IngestFiscaliteDirecteLocale(ctx, pool, arch)
		}},
	{Nom: "population-age", Categorie: CategorieCollectivites, Description: "population par département et grande tranche d'âge (Insee)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return communes.IngestPopulationAgeDepartement(ctx, pool, arch)
		}},
	{Nom: "population-historique", Categorie: CategorieCollectivites, Description: "population communale historique, 1876-1999 (Insee)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return communes.IngestPopulationHistorique(ctx, pool, arch)
		}},

	// --- budget : finances publiques, aides aux entreprises, paie.
	{Nom: "budget", Categorie: CategorieBudget, Description: "budget de l'État et de la Sécurité sociale (exécution)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return budget.Ingest(ctx, pool, arch)
		}},
	{Nom: "macro", Categorie: CategorieBudget, Description: "grandes séries nationales (RSA, prestations, fiscalité, chômage, retraite...) et comptes des grandes sociétés",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return ingestMacro(ctx, pool, arch)
		}},
	{Nom: "dette", Categorie: CategorieBudget, Description: "dette publique : encours, détenteurs, coût, comparaisons (WEBSTAT_API_KEY requis pour la détention)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return dette.Ingest(ctx, pool, arch)
		}},
	{Nom: "paie", Categorie: CategorieBudget, Description: "barème de paie 2026, destinataires et budgets, bulletin d'exemple",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return paie.Ingest(ctx, pool, arch)
		}},
	{Nom: "aides", Categorie: CategorieBudget, Description: "aides aux entreprises : taille des bénéficiaires",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return aides.Ingest(ctx, pool, arch)
		}},
	{Nom: "aides-urssaf", Categorie: CategorieBudget, Description: "exonérations URSSAF par taille d'entreprise",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return aides.IngestUrssafTaille(ctx, pool, arch)
		}},
	{Nom: "aides-nominatives", Categorie: CategorieBudget, Description: "aides publiées bénéficiaire par bénéficiaire (ADEME, minimis, TAM — SIRENE requis)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			for _, f := range []func(context.Context, *pgxpool.Pool, *archive.Archive) error{
				aides.IngestADEME, aides.IngestMinimis, aides.IngestTAM} {
				if err := f(ctx, pool, arch); err != nil {
					return err
				}
			}
			return nil
		}},
	{Nom: "ademe", Categorie: CategorieBudget, Description: "aides ADEME seules",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return aides.IngestADEME(ctx, pool, arch)
		}},
	{Nom: "minimis", Categorie: CategorieBudget, Description: "registre européen de minimis seul",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return aides.IngestMinimis(ctx, pool, arch)
		}},
	{Nom: "tam", Categorie: CategorieBudget, Description: "registre de transparence des aides (TAM) seul",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return aides.IngestTAM(ctx, pool, arch)
		}},
	{Nom: "sirene", Categorie: CategorieBudget, Description: "répertoire SIRENE (catégorie d'entreprise de chaque personne morale, ~1 Go)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return aides.IngestSirene(ctx, pool, arch)
		}},
	{Nom: "entreprises", Categorie: CategorieBudget, Description: "comptes déposés des grandes sociétés",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return entreprises.Ingest(ctx, pool, arch)
		}},
	{Nom: "investissement-entreprises", Categorie: CategorieBudget, Description: "investissement, dividendes et tissu productif par catégorie d'entreprise",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return entreprises.IngestInvestissement(ctx, pool, arch)
		}},

	// --- social : santé, vieillesse, jeunesse, pauvreté/richesse, éducation,
	// logement, justice, culture.
	{Nom: "hatvp", Categorie: CategorieSocial, Description: "déclarations d'intérêts et de patrimoine (HATVP)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return hatvp.Ingest(ctx, pool, arch)
		}},
	{Nom: "vieillesse", Categorie: CategorieSocial, Description: "branche autonomie (APA à domicile)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return vieillesse.IngestAPA(ctx, pool, arch)
		}},
	{Nom: "jeunesse", Categorie: CategorieSocial, Description: "insertion des apprentis (InserJeunes)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return jeunesse.IngestInserJeunes(ctx, pool, arch)
		}},
	{Nom: "richesse", Categorie: CategorieSocial, Description: "hauts revenus et hauts patrimoines (dernier décile)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestHautsRevenusPatrimoine(ctx, pool, arch)
		}},
	{Nom: "heritage", Categorie: CategorieSocial, Description: "héritage et concentration patrimoine/niveau de vie (Gini)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestHeritageConcentration(ctx, pool, arch)
		}},
	{Nom: "ifi", Categorie: CategorieSocial, Description: "IFI par commune (IFICOM)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestIFICOM(ctx, pool, arch)
		}},
	{Nom: "socle", Categorie: CategorieSocial, Description: "socle universel : seuil, ménages, pensions, chômage, déciles (micro-simulation)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return ingestSocle(ctx, pool, arch)
		}},
	{Nom: "immigration", Categorie: CategorieSocial, Description: "immigration et nationalité",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return immigration.Ingest(ctx, pool, arch)
		}},
	{Nom: "education", Categorie: CategorieSocial, Description: "effectifs et statuts des personnels de l'Éducation nationale",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return education.Ingest(ctx, pool, arch)
		}},
	{Nom: "education-personnel-categorie", Categorie: CategorieSocial,
		Description: "éducation nationale : personnels non enseignants par catégorie précise (direction, CPE, AESH...)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return education.IngestPersonnelCategorie(ctx, pool, arch)
		}},
	{Nom: "sante", Categorie: CategorieSocial, Description: "FINESS et secteurs conventionnels",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return sante.Ingest(ctx, pool, arch)
		}},
	{Nom: "rpps", Categorie: CategorieSocial, Description: "professionnels de santé, Annuaire Santé (fichier plat ~820 Mo)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return sante.IngestRPPS(ctx, pool, arch)
		}},
	{Nom: "sae", Categorie: CategorieSocial, Description: "SAE : personnel par fonction et passages aux urgences (binaire 7z requis)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return sante.IngestSAE(ctx, pool, arch)
		}},
	{Nom: "hopital-finances", Categorie: CategorieSocial, Description: "situation économique et financière des hôpitaux publics (Drees)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return sante.IngestHopitalFinances(ctx, pool, arch)
		}},
	{Nom: "damir", Categorie: CategorieSocial, Description: "Open Damir : remboursements de l'Assurance Maladie (~11 Go)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return damir.Ingest(ctx, pool, arch)
		}},
	{Nom: "etablissements-penitentiaires", Categorie: CategorieSocial, Description: "population détenue par établissement pénitentiaire",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestEtablissementsPenitentiaires(ctx, pool, arch)
		}},
	{Nom: "sru", Categorie: CategorieSocial, Description: "inventaire SRU par commune (logement social)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestSRU(ctx, pool, arch)
		}},
	{Nom: "effectifs-etudiants", Categorie: CategorieSocial, Description: "effectifs étudiants par commune (SIES)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestEffectifsEtudiants(ctx, pool, arch)
		}},
	{Nom: "effort-recherche", Categorie: CategorieSocial, Description: "effort de recherche, DIRD/PIB (Insee)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestEffortRecherche(ctx, pool, arch)
		}},
	{Nom: "museofile", Categorie: CategorieSocial, Description: "musées labellisés Musée de France (Muséofile)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestMuseesFrance(ctx, pool, arch)
		}},

	// --- environnement : eau, écologie, agriculture, géographie hydrologique.
	{Nom: "hydro", Categorie: CategorieEnvironnement, Description: "bassins hydrographiques (BD Topage)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return hydro.Ingest(ctx, pool, arch)
		}},
	{Nom: "sous-bassins", Categorie: CategorieEnvironnement, Description: "sous-bassins versants topographiques (résolution fine)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return hydro.IngestSousBassins(ctx, pool, arch)
		}},
	{Nom: "cours-eau", Categorie: CategorieEnvironnement, Description: "tracé des grands cours d'eau (repère cartographique)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return hydro.IngestCoursEau(ctx, pool, arch)
		}},
	{Nom: "eau-potable", Categorie: CategorieEnvironnement, Description: "services publics d'eau potable (SISPEA)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return eau.IngestSISPEAEauPotable(ctx, pool, arch)
		}},
	{Nom: "eau-aides-loire-bretagne", Categorie: CategorieEnvironnement, Description: "aides de l'agence de l'eau Loire-Bretagne",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return eau.IngestAidesLoireBretagne(ctx, pool, arch)
		}},
	{Nom: "eau-aides-artois-picardie", Categorie: CategorieEnvironnement, Description: "aides de l'agence de l'eau Artois-Picardie",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return eau.IngestAidesArtoisPicardie(ctx, pool, arch)
		}},
	{Nom: "eau-aides-rhin-meuse", Categorie: CategorieEnvironnement, Description: "aides de l'agence de l'eau Rhin-Meuse",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return eau.IngestAidesRhinMeuse(ctx, pool, arch)
		}},
	{Nom: "eau-eptb-epage", Categorie: CategorieEnvironnement, Description: "EPTB/EPAGE, reconstruits depuis BANATIC",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return eau.IngestEPTBEPAGE(ctx, pool, arch)
		}},
	{Nom: "eau-budget-annexe", Categorie: CategorieEnvironnement, Description: "budget annexe eau/assainissement (OFGL, M49/M49A)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return eau.IngestBudgetAnnexeEau(ctx, pool, arch)
		}},
	{Nom: "assainissement", Categorie: CategorieEnvironnement, Description: "assainissement collectif et non collectif par commune (SISPEA)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return eau.IngestAssainissement(ctx, pool, arch)
		}},
	{Nom: "ecologie", Categorie: CategorieEnvironnement, Description: "dépense de protection de l'environnement (Eurostat)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return ecologie.Ingest(ctx, pool, arch)
		}},
	{Nom: "agriculture", Categorie: CategorieEnvironnement, Description: "bilans alimentaires et appareil de production agricole",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return agriculture.Ingest(ctx, pool, arch)
		}},
	{Nom: "revenu-agricole", Categorie: CategorieEnvironnement, Description: "revenu agricole réel par unité de travail (France, UE)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestRevenuAgricole(ctx, pool, arch)
		}},

	// --- economie : appareil productif, commerce, entreprises, numérique.
	{Nom: "appareil-productif", Categorie: CategorieEconomie, Description: "emploi par secteur (NACE A10) et délocalisations",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			if err := macro.IngestEmploiSecteurNACE(ctx, pool, arch); err != nil {
				return err
			}
			return macro.IngestDelocalisationsInsee(ctx, pool, arch)
		}},
	{Nom: "commerce-partenaires", Categorie: CategorieEconomie, Description: "commerce extérieur par partenaire, secteur par secteur (UN Comtrade)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestCommercePartenaires(ctx, pool, arch)
		}},
	{Nom: "ports", Categorie: CategorieEconomie, Description: "trafic portuaire français et comparaison européenne",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			if err := macro.IngestTraficPortuaire(ctx, pool, arch); err != nil {
				return err
			}
			return macro.IngestTraficPortuaireEurope(ctx, pool, arch)
		}},
	{Nom: "ports-infra", Categorie: CategorieEconomie, Description: "accès terrestre aux grands ports et report modal",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			if err := macro.IngestAutoroutesPortuaires(ctx, pool, arch); err != nil {
				return err
			}
			if err := macro.IngestVoiesFerreesPortuaires(ctx, pool, arch); err != nil {
				return err
			}
			if err := macro.IngestReportModalPort(ctx, pool, arch); err != nil {
				return err
			}
			return macro.IngestReportModalConteneurs(ctx, pool, arch)
		}},
	{Nom: "decp", Categorie: CategorieEconomie, Description: "DECP : commande publique consolidée (fichier Parquet ~235 Mo)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return decp.Ingest(ctx, pool, arch)
		}},

	// --- transparence : intégrité publique, fiscalité comparée, souveraineté
	// numérique, moteur « dossiers/faits ».
	{Nom: "controle-fiscal", Categorie: CategorieTransparence, Description: "contrôle fiscal, résultats notifiés/encaissés (2015-2024)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestControleFiscal(ctx, pool, arch)
		}},
	{Nom: "fiscalite", Categorie: CategorieTransparence, Description: "fiscalité comparée et filiales de groupes étrangers (SIRENE requis, GLEIF ~500 Mo)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return fiscalite.Ingest(ctx, pool, arch)
		}},
	{Nom: "fiscalite-listes", Categorie: CategorieTransparence, Description: "listes officielles de paradis fiscaux",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return fiscalite.IngestListes(ctx, pool, arch)
		}},
	{Nom: "fiscalite-ocde", Categorie: CategorieTransparence, Description: "impôt sur les sociétés, comparaison OCDE",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return fiscalite.IngestOCDEImpotSocietes(ctx, pool, arch)
		}},
	{Nom: "fiscalite-ide", Categorie: CategorieTransparence, Description: "investissements directs étrangers (OCDE)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return fiscalite.IngestOCDEIDE(ctx, pool, arch)
		}},
	{Nom: "fiscalite-fats", Categorie: CategorieTransparence, Description: "filiales étrangères sous contrôle (FATS)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return fiscalite.IngestFATS(ctx, pool, arch)
		}},
	{Nom: "fiscalite-twz", Categorie: CategorieTransparence, Description: "estimations académiques des profits déplacés (Tørsløv-Wier-Zucman)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return fiscalite.IngestTWZ(ctx, pool, arch)
		}},
	{Nom: "fiscalite-filiales", Categorie: CategorieTransparence, Description: "filiales françaises de groupes étrangers (GLEIF, SIRENE requis)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return fiscalite.IngestFiliales(ctx, pool, arch)
		}},
	{Nom: "fiscalite-comptes", Categorie: CategorieTransparence, Description: "comptes déposés des filiales identifiées",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return fiscalite.IngestComptes(ctx, pool, arch)
		}},
	{Nom: "fiscalite-marches", Categorie: CategorieTransparence, Description: "marchés publics remportés par ces filiales",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return fiscalite.IngestMarches(ctx, pool, arch)
		}},
	{Nom: "fiscalite-faits", Categorie: CategorieTransparence, Description: "faits constatés du chantier fiscalité comparée",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return fiscalite.IngestFaits(ctx, pool, arch)
		}},
	{Nom: "fiscalite-transparence", Categorie: CategorieTransparence, Description: "indicateurs de transparence fiscale",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return fiscalite.IngestTransparence(ctx, pool, arch)
		}},
	{Nom: "numerique", Categorie: CategorieTransparence, Description: "souveraineté numérique (SecNumCloud, CNIL, SILL, marchés informatiques — pdftotext requis)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return numerique.Ingest(ctx, pool, arch)
		}},
	{Nom: "numerique-anssi", Categorie: CategorieTransparence, Description: "catalogue SecNumCloud de l'ANSSI",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return numerique.IngestQualifications(ctx, pool, arch)
		}},
	{Nom: "numerique-cnil", Categorie: CategorieTransparence, Description: "sanctions de la CNIL",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return numerique.IngestSanctionsCNIL(ctx, pool, arch)
		}},
	{Nom: "numerique-sill", Categorie: CategorieTransparence, Description: "socle interministériel des logiciels libres (SILL)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return numerique.IngestSILL(ctx, pool, arch)
		}},
	{Nom: "numerique-marches", Categorie: CategorieTransparence, Description: "marchés informatiques publics (relit les DECP consolidées, ~2,5 Go)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return numerique.IngestMarches(ctx, pool, arch)
		}},
	{Nom: "dossiers", Categorie: CategorieTransparence, Description: "faits de tous les dossiers et acteurs nommés (moteur transversal, D-066)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return dossiers.Ingest(ctx, pool, arch)
		}},
	{Nom: "dossiers-faits", Categorie: CategorieTransparence, Description: "faits des dossiers seuls",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return dossiers.IngestFaits(ctx, pool, arch)
		}},
	{Nom: "dossiers-acteurs", Categorie: CategorieTransparence, Description: "acteurs nommés des dossiers seuls",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return dossiers.IngestActeurs(ctx, pool, arch)
		}},

	// --- international : comparaisons et géopolitique.
	{Nom: "international", Categorie: CategorieInternational, Description: "comparaisons internationales : salaire minimum, PIB, épargne nette",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return international.Ingest(ctx, pool, arch)
		}},
	{Nom: "francophonie", Categorie: CategorieInternational, Description: "population et francophones par entité (ODSEF/OIF)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestFrancophonie(ctx, pool, arch)
		}},
	{Nom: "accord-paris", Categorie: CategorieInternational, Description: "Accord de Paris, ratifications pays par pays (ONU)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestAccordParis(ctx, pool, arch)
		}},
	{Nom: "ecart-prix-dom", Categorie: CategorieInternational, Description: "écart de prix DOM/métropole (Insee ECSP 2022)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return macro.IngestEcartPrixDOM(ctx, pool, arch)
		}},

	// --- systeme : infrastructure partagée, pas propre à un dossier.
	{Nom: "media", Categorie: CategorieSysteme, Description: "portraits et logos librement réutilisables",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return ingestMedia(ctx, pool, arch, "data", "web/media")
		}},
	{Nom: "checksums", Categorie: CategorieSysteme, Description: "recalcule seulement les empreintes de section (core.section_checksum)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return recalculerEmpreintes(ctx, pool)
		}},
	{Nom: "contours", Categorie: CategorieSysteme, Description: "contours IGN par millésime du COG",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			if err := communes.IngestCOG(ctx, pool, arch); err != nil {
				return err
			}
			return geo.Ingest(ctx, pool, arch, filepath.Join("data", "geo-projections.csv"), communes.COGMillesime)
		}},
	{Nom: "circonscriptions", Categorie: CategorieSysteme, Description: "circonscriptions législatives : contours, communes, indicateurs",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return geo.IngestCirconscriptions(ctx, pool, arch, filepath.Join("data", "geo-projections.csv"))
		}},
	{Nom: "contour-pays", Categorie: CategorieSysteme, Description: "fond de carte mondial par pays (Natural Earth)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return geo.IngestContourPays(ctx, pool, arch)
		}},
	{Nom: "empire-colonial", Categorie: CategorieSysteme, Description: "Empire colonial français, géographie et chronologie (CShapes 2.0)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return geo.IngestTerritoireColonial(ctx, pool, arch)
		}},
	{Nom: "ligne-demarcation", Categorie: CategorieSysteme, Description: "ligne de démarcation 1940-1942 (Département de l'Ain)",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return geo.IngestLigneDemarcation(ctx, pool, arch)
		}},
	{Nom: "prefets", Categorie: CategorieSysteme, Description: "représentation de l'État dans les départements",
		Executer: func(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, rawDir string) error {
			return prefets.Ingest(ctx, pool, arch)
		}},
}
