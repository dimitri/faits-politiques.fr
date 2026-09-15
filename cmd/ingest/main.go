// Commande ingest : télécharge les jeux de données, les scelle dans l'archive,
// puis reconstruit core à partir de raw.
//
//	go run ./cmd/ingest              chaîne complète
//	go run ./cmd/ingest -only=migrate
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/agriculture"
	"github.com/faits-politiques/faits-politiques/internal/aides"
	"github.com/faits-politiques/faits-politiques/internal/an"
	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/associations"
	"github.com/faits-politiques/faits-politiques/internal/budget"
	"github.com/faits-politiques/faits-politiques/internal/campagne"
	"github.com/faits-politiques/faits-politiques/internal/carto"
	"github.com/faits-politiques/faits-politiques/internal/communes"
	"github.com/faits-politiques/faits-politiques/internal/entreprises"
	"github.com/faits-politiques/faits-politiques/internal/damir"
	"github.com/faits-politiques/faits-politiques/internal/decp"
	"github.com/faits-politiques/faits-politiques/internal/dette"
	"github.com/faits-politiques/faits-politiques/internal/dossiers"
	"github.com/faits-politiques/faits-politiques/internal/ecologie"
	"github.com/faits-politiques/faits-politiques/internal/education"
	"github.com/faits-politiques/faits-politiques/internal/europe"
	"github.com/faits-politiques/faits-politiques/internal/fiscalite"
	"github.com/faits-politiques/faits-politiques/internal/geo"
	"github.com/faits-politiques/faits-politiques/internal/hatvp"
	"github.com/faits-politiques/faits-politiques/internal/immigration"
	"github.com/faits-politiques/faits-politiques/internal/international"
	"github.com/faits-politiques/faits-politiques/internal/jorf"
	"github.com/faits-politiques/faits-politiques/internal/macro"
	"github.com/faits-politiques/faits-politiques/internal/migrate"
	"github.com/faits-politiques/faits-politiques/internal/numerique"
	"github.com/faits-politiques/faits-politiques/internal/partis"
	"github.com/faits-politiques/faits-politiques/internal/paie"
	"github.com/faits-politiques/faits-politiques/internal/prefets"
	"github.com/faits-politiques/faits-politiques/internal/presidentielle"
	"github.com/faits-politiques/faits-politiques/internal/hydro"
	"github.com/faits-politiques/faits-politiques/internal/sante"
	"github.com/faits-politiques/faits-politiques/internal/senat"
	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/faits-politiques/faits-politiques/internal/vieillesse"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	only := flag.String("only", "", "migrate | download | partis | europe | senat | normalize | carto | communes | cog | rne | epci | collectivites | associations | ssmsi | municipales2020 | entreprises | agriculture | exposes | exposes-reparse | promulgation | deports | amendements | interventions | campagne | jorf | jorf-complet | jorf-elus | jorf-gouvernement | gouvernement-membres | senat-repertoire | senat-mandats | senat-commissions | senat-fusion | senat-presentations | hatvp | macro | prefets | contours | circonscriptions | socle | immigration | education | sante | vieillesse | sae | rpps | hydro | ecologie | international | decp | damir | dette | aides | aides-urssaf | sirene | aides-nominatives | tam | ademe | minimis | fiscalite | paie | budget | presidentielle | media")
	rawDir := flag.String("raw", "raw", "répertoire de l'archive scellée")
	migDir := flag.String("migrations", "db/migrations", "répertoire des migrations")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, *only, *rawDir, *migDir); err != nil {
		fmt.Fprintf(os.Stderr, "\nerreur : %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, only, rawDir, migDir string) error {
	start := time.Now()
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	fmt.Println("migrations")
	if err := migrate.Up(ctx, pool, migDir); err != nil {
		return err
	}
	if only == "migrate" {
		return nil
	}

	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return err
	}
	arch := &archive.Archive{Root: rawDir, Pool: pool}

	if only == "" || only == "download" {
		fmt.Println("\ntéléchargement et scellement")
		fetched, err := an.Download(ctx, arch)
		if err != nil {
			return err
		}
		fmt.Println("\nextraction vers raw.record")
		for slug, f := range fetched {
			n, err := an.Extract(ctx, pool, f)
			if err != nil {
				return fmt.Errorf("%s : %w", slug, err)
			}
			fmt.Printf("  %-12s %d enregistrements\n", slug, n)
		}
	}
	if only == "download" {
		return nil
	}

	if only == "" || only == "partis" {
		fmt.Println("\nréférentiels sur les organisations politiques")
		if err := partis.IngestComptes(ctx, pool, arch); err != nil {
			return err
		}
		if err := partis.IngestPopuList(ctx, pool, arch); err != nil {
			return err
		}
		if err := partis.IngestCHES(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "partis" {
		return nil
	}

	if only == "media" {
		fmt.Println("\nportraits et logos librement réutilisables")
		return ingestMedia(ctx, pool, arch, "data", "web/media")
	}

	// La normalisation de l'Assemblée vient AVANT le RNE et le Sénat.
	// L'ordre n'est pas cosmétique : le RNE n'insère un mandat de député que si
	// l'Assemblée n'en a pas déjà publié un (« le RNE complète, il n'écrase
	// pas »). Tant que la normalisation passait en dernier, ce garde-fou ne
	// gardait rien — le RNE arrivait le premier avec sa version pauvre, sans
	// circonscription ni date de fin, et la contrainte d'exclusion faisait
	// rejeter celle de l'Assemblée. En silence.
	if only == "" || only == "normalize" {
		fmt.Println("\nnormalisation raw -> core")
		if err := an.Normalize(ctx, pool); err != nil {
			return err
		}
		// Les déports sont déjà dans raw.record : normalisation seule.
		if err := an.NormalizeDeports(ctx, pool); err != nil {
			return err
		}
	}
	if only == "normalize" {
		return cartographie(ctx, pool)
	}

	if only == "" || only == "senat" {
		fmt.Println("\nSénat")
		if err := senat.Ingest(ctx, pool, arch, filepath.Join(rawDir, "senat-work")); err != nil {
			return err
		}
		// Le répertoire des sénateurs vient après les votes : il complète les
		// personnes issues des scrutins, et crée les sénateurs plus anciens
		// qu'aucun vote n'a fait connaître.
		if err := senat.IngestSenateurs(ctx, pool, arch); err != nil {
			return err
		}
		// Puis la fusion, car le Sénat ne partage aucun identifiant avec les
		// autres sources : sans elle, un sénateur également conseiller
		// municipal existe en deux fiches, chacune amputée de la moitié de sa
		// vie publique. Elle vient avant les mandats et les commissions pour
		// qu'ils se rattachent à la fiche unique.
		if err := senat.Fusionner(ctx, pool); err != nil {
			return err
		}
		// Recharger le Sénat reconstruit ses dossiers avec de NOUVEAUX
		// identifiants, et derived.scrutin_topic les référence en cascade : les
		// 4 806 thèmes hérités par la navette disparaissent sans un message.
		// Un commentaire dans le connecteur n'a pas suffi — le piège s'est
		// refermé deux fois. Le recalcul est donc fait ici, où il ne peut plus
		// être oublié.
		// Les mandats viennent du même dump, déjà scellé : rien à télécharger.
		if err := senat.NormalizeMandats(ctx, pool); err != nil {
			return err
		}
		if err := senat.IngestCommissions(ctx, pool, arch); err != nil {
			return err
		}
		// L'objet des dossiers est dans le même dump : il se reprend sans rien
		// retélécharger.
		if err := senat.NormalizePresentations(ctx, pool); err != nil {
			return err
		}
		fmt.Println("\nthèmes applicables aux scrutins")
		if err := carto.Themes(ctx, pool); err != nil {
			return err
		}
	}
	if only == "senat" {
		return nil
	}

	if only == "" || only == "europe" {
		fmt.Println("\nParlement européen")
		if err := europe.Ingest(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "europe" {
		return nil
	}

	if only == "carto" {
		return cartographie(ctx, pool)
	}

	if only == "" || only == "communes" {
		if err := dimensionLocale(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "communes" {
		return nil
	}

	// Le seul groupement : recharger BANATIC sans repasser par les 615 000
	// mandats du RNE ni les 1,7 million de valeurs de l'OFGL.
	if only == "cog" {
		fmt.Println("\nréférentiel géographique")
		return communes.IngestCOG(ctx, pool, arch)
	}

	// Le seul RNE : après une renormalisation de l'Assemblée, ses mandats
	// parlementaires doivent être recalculés — le garde-fou « le RNE complète,
	// il n'écrase pas » ne peut trancher qu'une fois l'Assemblée chargée.
	if only == "rne" {
		fmt.Println("\nrépertoire national des élus")
		return communes.IngestRNE(ctx, pool, arch)
	}

	if only == "epci" {
		fmt.Println("\nintercommunalités et compétences")
		return communes.IngestBANATIC(ctx, pool, arch)
	}

	if only == "municipales2020" {
		fmt.Println("\nélections municipales 2020")
		return communes.IngestMunicipales2020(ctx, pool, arch)
	}

	if only == "" || only == "associations" {
		fmt.Println("\ntissu associatif")
		if err := associations.Ingest(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "associations" {
		return nil
	}

	if only == "collectivites" {
		fmt.Println("\ncomptes des régions, départements et groupements")
		return communes.IngestCollectivites(ctx, pool, arch)
	}

	if only == "ssmsi" {
		fmt.Println("\ndélinquance enregistrée par commune")
		return communes.IngestSSMSI(ctx, pool, arch)
	}

	if only == "" || only == "hatvp" {
		fmt.Println("\ndéclarations d'intérêts et de patrimoine")
		if err := hatvp.Ingest(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "hatvp" {
		return nil
	}

	if only == "" || only == "presidentielle" {
		fmt.Println("\nélection présidentielle, population par âge, participation comparée")
		if err := presidentielle.Ingest(ctx, pool, arch); err != nil {
			return err
		}
		if err := presidentielle.IngestPopulation(ctx, pool, arch); err != nil {
			return err
		}
		if err := presidentielle.IngestTurnout(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "presidentielle" {
		return nil
	}

	if only == "" || only == "budget" {
		fmt.Println("\nbudget de l'État et de la Sécurité sociale")
		if err := budget.Ingest(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "budget" {
		return nil
	}

	if only == "" || only == "macro" {
		fmt.Println("\ngrandes séries nationales")
		if err := macro.Ingest(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestRSA(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestPrestationsSolidarite(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestRecettesFiscales(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestChomageINSEE(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestMinimaSociaux(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestAgeDepartRetraite(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestDemandeursEmploi(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestPrimeActivite(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestTauxRemplacement(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestCotisantsRetraites(ctx, pool, arch); err != nil {
			return err
		}
		if err := prefets.Ingest(ctx, pool, arch); err != nil {
			return err
		}
	}
	// Contours communaux et intercommunaux, un jeu par millésime du COG. Après
	// BANATIC : pour le millésime courant, l'appartenance aux intercommunalités
	// se lit dans core.epci_membre.
	// En rechargement ciblé, le COG est rechargé d'abord ; dans la chaîne
	// complète, le bloc communes l'a déjà fait.
	if only == "" || only == "contours" {
		fmt.Println("\ncontours IGN par millésime")
		if only == "contours" {
			if err := communes.IngestCOG(ctx, pool, arch); err != nil {
				return err
			}
		}
		if err := geo.Ingest(ctx, pool, arch, filepath.Join("data", "geo-projections.csv"), communes.COGMillesime); err != nil {
			return err
		}
	}
	// Circonscriptions législatives (Insee) : contours, communes, indicateurs.
	// Hors chaîne par défaut : le découpage ne change qu'à un redécoupage.
	if only == "circonscriptions" {
		fmt.Println("\ncirconscriptions législatives")
		return geo.IngestCirconscriptions(ctx, pool, arch, filepath.Join("data", "geo-projections.csv"))
	}
	// Rechargement ciblé : la série des préfets se met à jour une fois par an,
	// il serait absurde de retélécharger tout le bloc macro pour elle.
	if only == "prefets" {
		fmt.Println("\nreprésentation de l'État dans les départements")
		return prefets.Ingest(ctx, pool, arch)
	}
	// Les données de la micro-simulation du socle universel
	// (docs/revenu-universel-microsimulation.md) : hors chaîne par défaut, ce
	// n'est pas un fait constaté mais l'instruction d'une hypothèse chiffrée.
	if only == "socle" {
		fmt.Println("\nsocle universel : seuil, ménages, pensions, chômage, déciles")
		if err := macro.IngestPauvrete(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestAideAlimentaire(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestPauvreteTauxEU(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestMenagesDREES(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestMenagesEffectif(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestPensionsEIR(ctx, pool, arch); err != nil {
			return err
		}
		if err := macro.IngestChomageUnedic(ctx, pool, arch); err != nil {
			return err
		}
		return macro.IngestFilosofiDeciles(ctx, pool, arch)
	}
	// La population immigrée et la population étrangère : deux notions
	// distinctes, voir docs/immigration-donnees.md. Hors chaîne par défaut,
	// comme les autres blocs thématiques ajoutés au fil des demandes.
	if only == "immigration" {
		fmt.Println("\nimmigration et nationalité")
		return immigration.Ingest(ctx, pool, arch)
	}
	// Effectifs et statuts des personnels de l'Éducation nationale, par
	// établissement. Voir docs/education-donnees.md. Hors chaîne par défaut,
	// comme les autres blocs thématiques ajoutés au fil des demandes.
	if only == "education" {
		fmt.Println("\néducation nationale")
		return education.Ingest(ctx, pool, arch)
	}
	// FINESS et secteurs conventionnels : voir docs/sante-donnees.md. Hors
	// chaîne par défaut, comme les autres blocs thématiques ajoutés au fil
	// des demandes.
	if only == "sante" {
		fmt.Println("\nsanté : FINESS et secteurs conventionnels")
		return sante.Ingest(ctx, pool, arch)
	}
	// La branche autonomie : voir docs/vieillesse-donnees.md. Hors chaîne
	// par défaut, comme les autres blocs thématiques ajoutés au fil des
	// demandes.
	if only == "vieillesse" {
		fmt.Println("\nvieillesse : branche autonomie")
		return vieillesse.IngestAPA(ctx, pool, arch)
	}
	// Les bassins hydrographiques (BD Topage) : la couche géographique du
	// dossier bassins versants, voir docs/bassins-versants-donnees.md. Hors
	// chaîne par défaut, comme les autres blocs thématiques ajoutés au fil
	// des demandes.
	if only == "hydro" {
		fmt.Println("\nbassins hydrographiques")
		return hydro.Ingest(ctx, pool, arch)
	}
	// Dépense de protection de l'environnement (CEP/Eurostat), voir
	// docs/ecologie-donnees.md § 4. Hors chaîne par défaut.
	if only == "ecologie" {
		fmt.Println("\ndépense de protection de l'environnement")
		return ecologie.Ingest(ctx, pool, arch)
	}
	// Comparaisons internationales (chantier 11) : salaire minimum, PIB et
	// épargne nette ajustée. Voir docs/international-donnees.md. Hors chaîne
	// par défaut.
	if only == "international" {
		fmt.Println("\ncomparaisons internationales")
		return international.Ingest(ctx, pool, arch)
	}
	// SAE (bordereau Q24, personnel par fonction) : à part de "sante" parce
	// que ce seul connecteur exige le binaire 7z sur la machine — voir
	// internal/sante/sae.go. Voir docs/sante-donnees.md.
	if only == "sae" {
		fmt.Println("\nSAE : personnel par fonction (bordereau Q24)")
		return sante.IngestSAE(ctx, pool, arch)
	}
	// RPPS (Annuaire Santé) : à part de "sante" parce que ce seul connecteur
	// télécharge et relit un fichier plat de ~820 Mo (2,4 millions de
	// lignes) — voir internal/sante/rpps.go. Voir docs/sante-donnees.md.
	if only == "rpps" {
		fmt.Println("\nRPPS : professionnels de santé (Annuaire Santé)")
		return sante.IngestRPPS(ctx, pool, arch)
	}
	// DECP (commande publique) : à part parce que ce seul connecteur lit un
	// fichier Parquet de 235 Mo (3,3 millions de lignes) — voir
	// internal/decp/decp.go et le commentaire de core.public_contract.
	if only == "decp" {
		fmt.Println("\nDECP : commande publique consolidée")
		return decp.Ingest(ctx, pool, arch)
	}
	// Open Damir (remboursements Assurance Maladie) : à part, un seul
	// exercice pèse ~11 Go en téléchargement (970 Mo × 12 mois) — voir
	// internal/damir/damir.go et le commentaire de core.remboursement_national.
	if only == "damir" {
		fmt.Println("\nOpen Damir : remboursements de l'Assurance Maladie")
		return damir.Ingest(ctx, pool, arch)
	}
	// La dette : encours, détenteurs, coût, comparaisons européenne et
	// suisse. Voir docs/dette-donnees.md. La détention (Banque de France)
	// demande WEBSTAT_API_KEY dans l'environnement.
	if only == "dette" {
		fmt.Println("\ndette publique")
		return dette.Ingest(ctx, pool, arch)
	}
	// Qui reçoit les aides : exonérations par taille d'entreprise (URSSAF) et
	// catégorie d'entreprise de chaque personne morale (SIRENE, ~1 Go).
	if only == "aides" {
		fmt.Println("\naides aux entreprises : taille des bénéficiaires")
		return aides.Ingest(ctx, pool, arch)
	}
	if only == "aides-urssaf" {
		return aides.IngestUrssafTaille(ctx, pool, arch)
	}
	// Les aides publiées bénéficiaire par bénéficiaire (SIRENE requis).
	if only == "aides-nominatives" {
		fmt.Println("\naides nominatives : ADEME, minimis, registre européen (TAM)")
		for _, f := range []func(context.Context, *pgxpool.Pool, *archive.Archive) error{
			aides.IngestADEME, aides.IngestMinimis, aides.IngestTAM} {
			if err := f(ctx, pool, arch); err != nil {
				return err
			}
		}
		return nil
	}
	if only == "tam" {
		return aides.IngestTAM(ctx, pool, arch)
	}
	if only == "ademe" {
		return aides.IngestADEME(ctx, pool, arch)
	}
	if only == "minimis" {
		return aides.IngestMinimis(ctx, pool, arch)
	}
	// Barème de paie 2026, destinataires et budgets, bulletin d'exemple.
	if only == "paie" {
		fmt.Println("\nbarème de paie et destinataires des prélèvements")
		return paie.Ingest(ctx, pool, arch)
	}
	// « La France est-elle un paradis fiscal ? » : listes officielles, OCDE,
	// Eurostat, estimations académiques, filiales de groupes étrangers
	// (SIRENE requis). Le répertoire GLEIF pèse ~500 Mo.
	if only == "fiscalite" {
		fmt.Println("\nfiscalité comparée et filiales de groupes étrangers")
		return fiscalite.Ingest(ctx, pool, arch)
	}
	for nom, f := range map[string]func(context.Context, *pgxpool.Pool, *archive.Archive) error{
		"fiscalite-listes": fiscalite.IngestListes, "fiscalite-ocde": fiscalite.IngestOCDEImpotSocietes,
		"fiscalite-ide": fiscalite.IngestOCDEIDE, "fiscalite-fats": fiscalite.IngestFATS,
		"fiscalite-twz": fiscalite.IngestTWZ, "fiscalite-filiales": fiscalite.IngestFiliales,
		"fiscalite-comptes": fiscalite.IngestComptes,
		"fiscalite-marches": fiscalite.IngestMarches, "fiscalite-faits": fiscalite.IngestFaits,
		"fiscalite-transparence": fiscalite.IngestTransparence,
	} {
		if only == nom {
			return f(ctx, pool, arch)
		}
	}
	// Souveraineté numérique : catalogue SecNumCloud de l'ANSSI, sanctions de
	// la CNIL, SILL, marchés informatiques. Exige pdftotext ;
	// relit les DECP consolidées (2,5 Go). Voir docs/souverainete-numerique.md.
	for nom, f := range map[string]func(context.Context, *pgxpool.Pool, *archive.Archive) error{
		"numerique": numerique.Ingest, "numerique-anssi": numerique.IngestQualifications,
		"numerique-cnil": numerique.IngestSanctionsCNIL, "numerique-sill": numerique.IngestSILL,
		"numerique-marches": numerique.IngestMarches,
		// Faits de tous les dossiers (docs/*.md, D-066) et acteurs nommés : après
		// jorf-complet, interventions, sirene et numerique-anssi, qu'ils relisent.
		"dossiers": dossiers.Ingest, "dossiers-faits": dossiers.IngestFaits, "dossiers-acteurs": dossiers.IngestActeurs,
	} {
		if only == nom {
			return f(ctx, pool, arch)
		}
	}
	if only == "sirene" {
		fmt.Println("\nrépertoire SIRENE")
		return aides.IngestSirene(ctx, pool, arch)
	}
	if only == "" || only == "agriculture" {
		fmt.Println("\nbilans alimentaires et appareil de production agricole")
		if err := agriculture.Ingest(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "agriculture" {
		return nil
	}

	// Les mandats sénatoriaux se recalculent depuis senat_raw, déjà scellé :
	// inutile de retélécharger 200 Mo de dump pour les seules dates.
	if only == "senat-repertoire" {
		fmt.Println("\nrépertoire des sénateurs")
		return senat.IngestSenateurs(ctx, pool, arch)
	}

	if only == "senat-commissions" {
		fmt.Println("\ncommissions du Sénat")
		return senat.IngestCommissions(ctx, pool, arch)
	}

	if only == "senat-mandats" {
		fmt.Println("\nmandats de sénateurs")
		return senat.NormalizeMandats(ctx, pool)
	}

	if only == "senat-presentations" {
		fmt.Println("\nprésentations des dossiers du Sénat")
		return senat.NormalizePresentations(ctx, pool)
	}

	if only == "senat-fusion" {
		fmt.Println("\nfusion des fiches de sénateurs")
		return senat.Fusionner(ctx, pool)
	}

	// Le nombre d'archives est borné : le flux complet fait 784 livraisons et
	// deux gigaoctets. On commence par les plus récentes, qui couvrent les
	// responsables en fonction.
	if only == "jorf" {
		fmt.Println("\nactes nominatifs du Journal officiel")
		return jorf.Ingest(ctx, pool, arch, 60)
	}

	// La base complète du Journal officiel, traversée en flux pour n'en retenir
	// que les décrets de composition du Gouvernement. C'est la seule source qui
	// couvre 2014-2026 : le jeu officiel des services du Premier ministre
	// s'arrête en 2014 (voir internal/jorf/gouvernement.go).
	if only == "jorf-gouvernement" {
		fmt.Println("\ndécrets de composition du Gouvernement")
		return jorf.IngestGouvernement(ctx, pool, arch)
	}

	// Le Journal officiel en entier : 1,24 million d'actes de 1861 à 2025,
	// chargés en une traversée par quatre flux COPY parallèles.
	// Voir internal/jorf/copie.go.
	if only == "jorf-complet" {
		fmt.Println("\nchargement complet du Journal officiel")
		return jorf.IngestComplet(ctx, pool, arch)
	}

	// Le thésaurus des élus, appliqué au corpus déjà chargé.
	if only == "jorf-elus" {
		fmt.Println("\nreconnaissance des élus dans le Journal officiel")
		return jorf.NormalizeElus(ctx, pool)
	}

	// La lecture des décrets déjà scellés : corriger l'analyse d'une phrase ne
	// doit pas obliger à retraverser un gigaoctet d'archive.
	if only == "gouvernement-membres" {
		fmt.Println("\nmembres du Gouvernement, d'après les décrets")
		return jorf.NormalizeMembres(ctx, pool)
	}

	if only == "campagne" {
		fmt.Println("\ncomptes de campagne")
		return campagne.Ingest(ctx, pool, arch)
	}

	if only == "interventions" {
		fmt.Println("\ninterventions en séance")
		return an.IngestInterventions(ctx, pool, arch)
	}

	if only == "amendements" {
		fmt.Println("\namendements et exposés sommaires")
		return an.IngestAmendements(ctx, pool, arch)
	}

	if only == "deports" {
		fmt.Println("\ndéclarations de déport")
		return an.NormalizeDeports(ctx, pool)
	}

	if only == "exposes-reparse" {
		fmt.Println("\nréextraction des exposés depuis l'archive")
		return an.ReparseExposes(ctx, pool, rawDir)
	}

	if only == "exposes" {
		fmt.Println("\nexposés des motifs")
		return an.IngestExposes(ctx, pool, arch)
	}

	if only == "promulgation" {
		fmt.Println("\nrattachement des dossiers à la loi promulguée")
		return an.PromulgationDossiers(ctx, pool)
	}

	if only == "entreprises" {
		fmt.Println("\ncomptes déposés des grandes sociétés")
		return entreprises.Ingest(ctx, pool, arch)
	}

	if only == "" || only == "macro" {
		fmt.Println("\ncomptes déposés des grandes sociétés")
		if err := entreprises.Ingest(ctx, pool, arch); err != nil {
			return err
		}
	}
	if only == "macro" {
		return nil
	}

	// Les travaux qui s'appuient sur core.texte et core.dossier viennent après
	// la normalisation, jamais avant : ils y font référence par clé étrangère.
	if only == "" {
		fmt.Println("\namendements et exposés sommaires")
		if err := an.IngestAmendements(ctx, pool, arch); err != nil {
			return err
		}
		fmt.Println("\nexposés des motifs")
		if err := an.IngestExposes(ctx, pool, arch); err != nil {
			return err
		}
		fmt.Println("\ninterventions en séance")
		if err := an.IngestInterventions(ctx, pool, arch); err != nil {
			return err
		}
		fmt.Println("\ncomptes de campagne")
		if err := campagne.Ingest(ctx, pool, arch); err != nil {
			return err
		}
		fmt.Println("\nactes nominatifs du Journal officiel")
		if err := jorf.Ingest(ctx, pool, arch, 60); err != nil {
			return err
		}
		// Après jorf.Ingest, jamais avant : le rattachement compare la référence
		// NOR publiée par l'Assemblée à jo.texte.nor, qui vient d'être rempli.
		fmt.Println("\nrattachement des dossiers à la loi promulguée")
		if err := an.PromulgationDossiers(ctx, pool); err != nil {
			return err
		}
	}

	if err := cartographie(ctx, pool); err != nil {
		return err
	}

	fmt.Println("\nportraits et logos librement réutilisables")
	if err := ingestMedia(ctx, pool, arch, "data", "web/media"); err != nil {
		return err
	}

	fmt.Printf("\nterminé en %s\n", time.Since(start).Round(time.Second))
	return nil
}

// cartographie charge les décisions de rattachement puis en déduit le thème
// applicable à chaque scrutin. Les deux vont ensemble : sans le rattachement
// parti -> groupe, un thème ne se relie à aucune famille politique.
func cartographie(ctx context.Context, pool *pgxpool.Pool) error {
	fmt.Println("\ncartographie éditoriale")
	if err := carto.Ingest(ctx, pool, filepath.Join("data", "organisations.csv")); err != nil {
		return err
	}
	fmt.Println("\ngouvernements de la Ve République")
	if err := carto.IngestGouvernements(ctx, pool, filepath.Join("data", "gouvernements.csv")); err != nil {
		return err
	}
	fmt.Println("\nprésidences de la République")
	if err := carto.IngestPresidents(ctx, pool, filepath.Join("data", "presidents.csv")); err != nil {
		return err
	}
	fmt.Println("\nthèmes applicables aux scrutins")
	return carto.Themes(ctx, pool)
}

// dimensionLocale charge la dimension communale, dans un ordre contraint :
// ref.commune est référencé par tout le reste, et les résultats électoraux ne
// peuvent pas être rattachés à une commune qui n'existe pas encore.
func dimensionLocale(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	fmt.Println("\nréférentiel géographique")
	if err := communes.IngestCOG(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nmaires")
	if err := communes.IngestRNE(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nélections municipales")
	if err := communes.IngestMunicipales(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\ncomptes des communes")
	if err := communes.IngestOFGL(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nintercommunalités et compétences")
	if err := communes.IngestBANATIC(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\nélections municipales 2020")
	if err := communes.IngestMunicipales2020(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\ncomptes des régions, départements et groupements")
	if err := communes.IngestCollectivites(ctx, pool, arch); err != nil {
		return err
	}
	fmt.Println("\ndélinquance enregistrée par commune")
	return communes.IngestSSMSI(ctx, pool, arch)
}
