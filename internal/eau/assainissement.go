package eau

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

// normaliserCleColonne : les en-têtes SISPEA changent de casse d'un export
// à l'autre (« mode_gestion » côté collectif, « Mode_gestion » côté non
// collectif, vérifié à l'inspection) — comparaison insensible à la casse
// plutôt qu'une liste de colonnes dupliquée par variante.
func normaliserCleColonne(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

const ConnectorVersionAssainissement = "eau-assainissement-v1"

// SourceAssainissement : les jeux « exploités pour les rapports nationaux
// SISPEA », assainissement collectif et non collectif — à la différence de
// l'export eau potable (service par service), ceux-ci sont publiés au
// niveau de la commune, avec le service qui la dessert : la composition
// communale que l'export eau potable ne donne pas.
var SourceAssainissement = archive.Source{
	Slug: "sispea-assainissement", Label: "SISPEA — services publics d'assainissement, par commune",
	Publisher: "Observatoire des services publics d'eau et d'assainissement (OFB, eaufrance.fr)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : SISPEA, Office français de la biodiversité (eaufrance.fr)",
	Cadence:     "annuelle",
	Notes: "Millésime 2023 seulement : comme pour l'eau potable, l'export 2024 (collectif et " +
		"non collectif) est un binaire .xls hérité (OLE2/CFBF), lisible avec une bibliothèque " +
		"tierce (github.com/extrame/xls) mais dont les en-têtes fusionnés se désalignent des " +
		"colonnes de données pour ce millésime (vérifié : « Nom de l'entité de gestion » pointe en " +
		"réalité vers une valeur de compétence, pas un nom) — un mappage fiable demanderait plus " +
		"de vérification que ce chargement n'en a fait, non deviné. Les colonnes d'indicateur " +
		"(prix, conformité...) ne sont pas chargées : leur code (d201_0, d301_0...) n'a pas été " +
		"vérifié contre leur définition officielle, à la différence de d101_0/d102_0 déjà " +
		"vérifiés pour l'eau potable.",
}

const (
	urlAssainissementCollectif    = "https://data.ofb.fr/catalogue/srv/api/records/5feec4e9-03a6-409a-a522-d51346d5f4c9/attachments/SISPEA_extraction_2023_AC.7z"
	urlAssainissementNonCollectif = "https://data.ofb.fr/catalogue/srv/api/records/96f91c3e-cc33-4f7a-a0fa-6620ff79d168/attachments/SISPEA_extraction_2023_ANC.7z"
)

type ligneAssainissement struct {
	CodeInsee                       string
	IDCollectivite, NomCollectivite *string
	IDService, NomService           *string
	ModeGestion, NomOperateur       *string
	PopulationDesservie             *int
	AgenceDeLEau                    *string
}

// extraireXLSXDe7z : les deux exports (AC et ANC) sont, comme l'eau potable,
// livrés en .7z contenant un .xlsx unique — même contournement que
// sispea.go (le 7z lui-même n'est pas un format qu'excelize sait ouvrir).
func extraireXLSXDe7z(ctx context.Context, cheminArchive string) (string, func(), error) {
	if _, err := exec.LookPath("7z"); err != nil {
		return "", nil, fmt.Errorf("binaire 7z introuvable sur le PATH : %w", err)
	}
	tmp, err := os.MkdirTemp("", "sispea-assainissement-*")
	if err != nil {
		return "", nil, err
	}
	nettoyer := func() { os.RemoveAll(tmp) }
	cmd := exec.CommandContext(ctx, "7z", "e", cheminArchive, "-o"+tmp, "-y", "-r", "*.xlsx")
	out, err := cmd.CombinedOutput()
	if err != nil {
		nettoyer()
		return "", nil, fmt.Errorf("extraction 7z : %w : %s", err, out)
	}
	matches, err := filepath.Glob(filepath.Join(tmp, "*.xlsx"))
	if err != nil {
		nettoyer()
		return "", nil, err
	}
	if len(matches) != 1 {
		nettoyer()
		return "", nil, fmt.Errorf("attendu un seul .xlsx extrait, trouvé %d — le format a peut-être changé", len(matches))
	}
	return matches[0], nettoyer, nil
}

// chargerAssainissement lit un des deux exports (colonnes vérifiées
// identiques entre collectif et non collectif à ceci près que la casse de
// « Mode_gestion » diffère — la recherche de colonne est insensible à ce
// détail).
func chargerAssainissement(cheminXLSX string) ([]ligneAssainissement, error) {
	wb, err := excelize.OpenFile(cheminXLSX)
	if err != nil {
		return nil, fmt.Errorf("classeur illisible : %w", err)
	}
	defer wb.Close()
	rows, err := wb.GetRows(wb.GetSheetList()[0])
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("moins de 2 lignes")
	}
	idx := map[string]int{}
	for i, h := range rows[0] {
		idx[normaliserCleColonne(h)] = i
	}
	col := func(nom string) (int, bool) { i, ok := idx[normaliserCleColonne(nom)]; return i, ok }
	iCommune, ok := col("n_insee_si_commune")
	if !ok {
		return nil, fmt.Errorf("colonne commune absente — le format a peut-être changé")
	}
	iIDColl, _ := col("id_sispea_coll")
	iNomColl, _ := col("nom_coll")
	iIDServ, _ := col("id_sispea_serv")
	iNomServ, _ := col("nom_serv")
	iMode, _ := col("mode_gestion")
	iOperateur, _ := col("nom_operateur")
	iPop, _ := col("pop_comm_adh")
	iAgence, _ := col("agence_de_leau")

	get := func(r []string, i int) *string {
		if i < 0 || i >= len(r) || r[i] == "" || r[i] == "." {
			return nil
		}
		v := r[i]
		return &v
	}

	var lignes []ligneAssainissement
	for _, r := range rows[1:] {
		commune := get(r, iCommune)
		if commune == nil {
			continue // ligne d'agrégat régional/collectivité sans commune propre
		}
		mode, err := modeGestionCode(strVal(get(r, iMode)))
		if err != nil {
			return nil, err
		}
		var pop *int
		if p := get(r, iPop); p != nil {
			v, err := entierNullable(*p)
			if err != nil {
				return nil, fmt.Errorf("commune %s, population : %w", *commune, err)
			}
			pop = v
		}
		lignes = append(lignes, ligneAssainissement{
			CodeInsee: *commune, IDCollectivite: get(r, iIDColl), NomCollectivite: get(r, iNomColl),
			IDService: get(r, iIDServ), NomService: get(r, iNomServ),
			ModeGestion: mode, NomOperateur: get(r, iOperateur),
			PopulationDesservie: pop, AgenceDeLEau: get(r, iAgence),
		})
	}
	return lignes, nil
}

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// IngestAssainissement charge la composition communale des services
// d'assainissement collectif et non collectif (SISPEA 2023). Voir
// docs/bassins-versants-donnees.md.
func IngestAssainissement(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceAssainissement)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersionAssainissement)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	charger := func(url, competence string) ([]ligneAssainissement, error) {
		f, err := arch.Fetch(ctx, srcID, runID, url, ".7z")
		if err != nil {
			return nil, err
		}
		xlsxPath, nettoyer, err := extraireXLSXDe7z(ctx, f.Path)
		if err != nil {
			return nil, fmt.Errorf("%s : %w", competence, err)
		}
		defer nettoyer()
		lignes, err := chargerAssainissement(xlsxPath)
		if err != nil {
			return nil, fmt.Errorf("%s : %w", competence, err)
		}
		return lignes, nil
	}

	lignesAC, err := charger(urlAssainissementCollectif, "assainissement collectif")
	if err != nil {
		return fail(err)
	}
	lignesANC, err := charger(urlAssainissementNonCollectif, "assainissement non collectif")
	if err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// Seul le millésime 2023 est chargé ici : une vue scopée reproduit
	// exactement la portée de l'ancien « DELETE ... WHERE annee = 2023 »,
	// au cas où d'autres millésimes seraient chargés un jour par ailleurs.
	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE TEMPORARY VIEW service_assainissement_scope AS
		SELECT * FROM core.service_assainissement WHERE annee = 2023
		WITH LOCAL CHECK OPTION`); err != nil {
		return fail(err)
	}
	rows := make([][]any, 0, len(lignesAC)+len(lignesANC))
	ajouter := func(competence string, lignes []ligneAssainissement) {
		for _, l := range lignes {
			rows = append(rows, []any{
				competence, 2023, l.CodeInsee, l.IDCollectivite, l.NomCollectivite,
				l.IDService, l.NomService, l.ModeGestion, l.NomOperateur,
				l.PopulationDesservie, l.AgenceDeLEau, srcID,
			})
		}
	}
	ajouter("COLLECTIF", lignesAC)
	ajouter("NON_COLLECTIF", lignesANC)
	if len(rows) == 0 {
		return fail(fmt.Errorf("assainissement : aucune ligne à charger"))
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_service_assainissement (
			competence text NOT NULL,
			annee integer NOT NULL,
			code_insee text NOT NULL,
			id_sispea_collectivite text,
			nom_collectivite text,
			id_sispea_service text NOT NULL,
			nom_service text,
			mode_gestion text,
			nom_operateur text,
			population_desservie integer,
			agence_de_leau text,
			source_id bigint NOT NULL
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_service_assainissement"},
		[]string{"competence", "annee", "code_insee", "id_sispea_collectivite", "nom_collectivite",
			"id_sispea_service", "nom_service", "mode_gestion", "nom_operateur",
			"population_desservie", "agence_de_leau", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("service_assainissement : %w", err))
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO service_assainissement_scope AS tgt
		USING tmp_service_assainissement AS src
		ON tgt.competence = src.competence AND tgt.annee = src.annee
			AND tgt.code_insee = src.code_insee AND tgt.id_sispea_service = src.id_sispea_service
		WHEN MATCHED AND (tgt.id_sispea_collectivite, tgt.nom_collectivite, tgt.nom_service,
				tgt.mode_gestion, tgt.nom_operateur, tgt.population_desservie,
				tgt.agence_de_leau, tgt.source_id)
			IS DISTINCT FROM (src.id_sispea_collectivite, src.nom_collectivite, src.nom_service,
				src.mode_gestion, src.nom_operateur, src.population_desservie,
				src.agence_de_leau, src.source_id) THEN
			UPDATE SET id_sispea_collectivite = src.id_sispea_collectivite,
				nom_collectivite = src.nom_collectivite, nom_service = src.nom_service,
				mode_gestion = src.mode_gestion, nom_operateur = src.nom_operateur,
				population_desservie = src.population_desservie,
				agence_de_leau = src.agence_de_leau, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
			INSERT (competence, annee, code_insee, id_sispea_collectivite, nom_collectivite,
				id_sispea_service, nom_service, mode_gestion, nom_operateur,
				population_desservie, agence_de_leau, source_id)
			VALUES (src.competence, src.annee, src.code_insee, src.id_sispea_collectivite,
				src.nom_collectivite, src.id_sispea_service, src.nom_service, src.mode_gestion,
				src.nom_operateur, src.population_desservie, src.agence_de_leau, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return fail(fmt.Errorf("service_assainissement, fusion : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"collectif": len(lignesAC), "non_collectif": len(lignesANC)}, "")
	fmt.Printf("  assainissement (SISPEA 2023) : %d communes (collectif), %d (non collectif)\n",
		len(lignesAC), len(lignesANC))
	return nil
}
