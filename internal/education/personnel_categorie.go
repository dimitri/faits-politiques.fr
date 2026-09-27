package education

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SourcePersonnelCategorie : le second degré (core.education_personnel_etablissement,
// effectifs.go) ne publie qu'un résidu « ETP autre » qui mélange direction,
// administratif et technique — voir docs/education-donnees.md § 3. La Depp
// publie la vraie décomposition, par catégorie précise, dans son Panorama
// statistique des personnels de l'enseignement scolaire, un document trouvé
// et vérifié directement (le PDF téléchargé, les tableaux lus cellule par
// cellule) — pas de fichier CSV/XLSX trouvé pour ces deux tableaux
// précisément (le site education.gouv.fr rejette les requêtes automatisées
// par un défi Cloudflare qui exige l'exécution de JavaScript, contrairement
// au chemin statique du PDF lui-même, atteignable sans contournement).
var SourcePersonnelCategorie = archive.Source{
	Slug: "depp-panorama-personnels-categorie", Label: "Depp — personnels non enseignants par catégorie précise (Panorama)",
	Publisher: "Direction de l'évaluation, de la prospective et de la performance (Depp)",
	Tier:      "PRIMARY_OFFICIAL",
	Licence:   "Licence Ouverte v2.0", ReuseClass: "OPEN",
	Attribution: "Source : ministère de l'Éducation nationale (Depp), Panorama statistique des personnels de l'enseignement scolaire",
	Cadence:     "annuelle (édition d'octobre)",
	Notes: "Rentrée 2024 seulement (édition 2024-2025 du Panorama). Extrait de deux tableaux du " +
		"PDF (Figures 3.1 et 3.10) par repérage positionnel des lignes « Ensemble » à l'intérieur " +
		"de chaque tableau, pas par un lecteur de tableau générique : la mise en page du PDF colle " +
		"parfois deux colonnes sans espace double (repéré sur les lignes de sous-total les plus " +
		"larges, ex. « 220 303 » de la Figure 3.10, dont l'ETP se retrouve amputé d'un chiffre) — " +
		"ces lignes-là sont ignorées, leurs totaux repris depuis la Figure 3.1 où elles sont bien " +
		"séparées. Chaque sous-total est revérifié à l'ingestion contre la somme de ses composantes.",
}

const urlPersonnelCategorie = "https://www.education.gouv.fr/sites/default/files/document/" +
	"panorama-statistique-des-personnels-de-l-enseignement-scolaire-2024-2025-475342.pdf"

const anneePersonnelCategorie = 2024

// ligneEnsemble : une ligne « Ensemble » d'un tableau du Panorama — effectif
// et ETP, les deux seules colonnes retenues (pas les pourcentages d'âge, de
// temps partiel, etc., hors du besoin de ce chargement).
type ligneEnsemble struct {
	Effectif int
	ETP      float64
}

var reEnsemble = regexp.MustCompile(`Ensemble\s+(\d{1,3}(?:\s\d{3})*)\s.*?(\d{1,3}(?:\s\d{3})*)\s*$`)

func parseNombreEspace(s string) (int, error) {
	return strconv.Atoi(strings.ReplaceAll(s, " ", ""))
}

// lignesEnsembleEntre extrait, dans l'ordre d'apparition, toutes les lignes
// « Ensemble » entre deux bornes textuelles (bornes incluses pour le début,
// exclues pour la fin) — une région correspondant à un seul tableau du PDF.
func lignesEnsembleEntre(lignes []string, debut, fin string) ([]ligneEnsemble, error) {
	iDebut := -1
	for i, l := range lignes {
		if strings.Contains(l, debut) {
			iDebut = i
			break
		}
	}
	if iDebut < 0 {
		return nil, fmt.Errorf("repère de début %q introuvable — le format du PDF a peut-être changé", debut)
	}
	iFin := len(lignes)
	for i := iDebut + 1; i < len(lignes); i++ {
		if strings.Contains(lignes[i], fin) {
			iFin = i
			break
		}
	}
	var out []ligneEnsemble
	for _, l := range lignes[iDebut:iFin] {
		m := reEnsemble.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		effectif, err := parseNombreEspace(m[1])
		if err != nil {
			return nil, fmt.Errorf("effectif illisible dans %q : %w", l, err)
		}
		etpInt, err := parseNombreEspace(m[2])
		if err != nil {
			return nil, fmt.Errorf("ETP illisible dans %q : %w", l, err)
		}
		out = append(out, ligneEnsemble{Effectif: effectif, ETP: float64(etpInt)})
	}
	return out, nil
}

func IngestPersonnelCategorie(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourcePersonnelCategorie)
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

	if _, err := exec.LookPath("pdftotext"); err != nil {
		return fail(fmt.Errorf("binaire pdftotext (paquet poppler-utils) introuvable sur le PATH : %w", err))
	}
	f, err := arch.Fetch(ctx, srcID, runID, urlPersonnelCategorie, ".pdf")
	if err != nil {
		return fail(err)
	}
	out, err := exec.CommandContext(ctx, "pdftotext", "-layout", f.Path, "-").Output()
	if err != nil {
		return fail(fmt.Errorf("pdftotext : %w", err))
	}
	lignes := strings.Split(string(out), "\n")

	// Figure 3.1 : répartition des personnels non enseignants par filière —
	// 15 lignes « Ensemble » dans cet ordre exact (vérifié à la main sur
	// l'édition 2024-2025) : direction, inspection, encadrement supérieur,
	// [total encadrement], éducation, assistance éducative, [total vie
	// scolaire], administrative, santé et sociale, technique, [total ASS],
	// ITRF, [total titulaires], [total non-titulaires], [total général].
	fig31, err := lignesEnsembleEntre(lignes, "Figure 3.1 –", "Figure 3.2 –")
	if err != nil {
		return fail(fmt.Errorf("Figure 3.1 : %w", err))
	}
	if len(fig31) != 15 {
		return fail(fmt.Errorf("Figure 3.1 : 15 lignes « Ensemble » attendues, %d trouvées — le format a peut-être changé", len(fig31)))
	}
	direction, inspection, encadrementSup := fig31[0], fig31[1], fig31[2]
	encadrementTotal, educationTotal := fig31[3], fig31[4]
	assistanceEducativeTotal, vieScolaireTotal := fig31[5], fig31[6]
	administrative, santeSociale, technique := fig31[7], fig31[8], fig31[9]
	assTotal, itrf := fig31[10], fig31[11]
	nonEnseignantsTotal := fig31[14]

	// Figure 3.10 : détail des personnels de vie scolaire — 10 lignes
	// « Ensemble » dans cet ordre : CPE, PsyEN/orientation, éducation non
	// titulaires, [total éducation], AESH, AED, [total assistance
	// éducative, ETP amputé par une mise en page collée — ignoré], [total
	// titulaires], [total non-titulaires], [total vie scolaire, même
	// défaut — ignoré].
	fig310, err := lignesEnsembleEntre(lignes, "Figure 3.10 –", "LES PERSONNELS ASS ET LES ITRF")
	if err != nil {
		return fail(fmt.Errorf("Figure 3.10 : %w", err))
	}
	if len(fig310) != 10 {
		return fail(fmt.Errorf("Figure 3.10 : 10 lignes « Ensemble » attendues, %d trouvées — le format a peut-être changé", len(fig310)))
	}
	cpe, psyen, educationNonTitulaires := fig310[0], fig310[1], fig310[2]
	aesh, aed := fig310[4], fig310[5]

	// Vérifications croisées : chaque sous-total doit être la somme de ses
	// composantes, à l'arrondi près (comme le compte de résultat des
	// hôpitaux publics, § 1.9 du dossier santé) — sinon le repérage
	// positionnel s'est décalé.
	verifier := func(nom string, total int, composantes ...int) error {
		somme := 0
		for _, c := range composantes {
			somme += c
		}
		if total != somme {
			return fmt.Errorf("%s : total %d ≠ somme des composantes %d — le repérage des lignes s'est probablement décalé", nom, total, somme)
		}
		return nil
	}
	for _, chk := range []error{
		verifier("encadrement", encadrementTotal.Effectif, direction.Effectif, inspection.Effectif, encadrementSup.Effectif),
		verifier("éducation", educationTotal.Effectif, cpe.Effectif, psyen.Effectif, educationNonTitulaires.Effectif),
		verifier("assistance éducative", assistanceEducativeTotal.Effectif, aesh.Effectif, aed.Effectif),
		verifier("vie scolaire", vieScolaireTotal.Effectif, educationTotal.Effectif, assistanceEducativeTotal.Effectif),
		verifier("ASS", assTotal.Effectif, administrative.Effectif, santeSociale.Effectif, technique.Effectif),
		verifier("non-enseignants", nonEnseignantsTotal.Effectif, encadrementTotal.Effectif, vieScolaireTotal.Effectif, assTotal.Effectif, itrf.Effectif),
	} {
		if chk != nil {
			return fail(chk)
		}
	}

	rows := [][]any{
		{anneePersonnelCategorie, "PERSONNELS_DIRECTION", direction.Effectif, direction.ETP, srcID},
		{anneePersonnelCategorie, "PERSONNELS_INSPECTION", inspection.Effectif, inspection.ETP, srcID},
		{anneePersonnelCategorie, "ENCADREMENT_SUPERIEUR", encadrementSup.Effectif, encadrementSup.ETP, srcID},
		{anneePersonnelCategorie, "ENCADREMENT_TOTAL", encadrementTotal.Effectif, encadrementTotal.ETP, srcID},
		{anneePersonnelCategorie, "CPE", cpe.Effectif, cpe.ETP, srcID},
		{anneePersonnelCategorie, "PSYEN_ORIENTATION", psyen.Effectif, psyen.ETP, srcID},
		{anneePersonnelCategorie, "EDUCATION_NON_TITULAIRES", educationNonTitulaires.Effectif, educationNonTitulaires.ETP, srcID},
		{anneePersonnelCategorie, "EDUCATION_TOTAL", educationTotal.Effectif, educationTotal.ETP, srcID},
		{anneePersonnelCategorie, "AESH", aesh.Effectif, aesh.ETP, srcID},
		{anneePersonnelCategorie, "AED", aed.Effectif, aed.ETP, srcID},
		{anneePersonnelCategorie, "ASSISTANCE_EDUCATIVE_TOTAL", assistanceEducativeTotal.Effectif, assistanceEducativeTotal.ETP, srcID},
		{anneePersonnelCategorie, "VIE_SCOLAIRE_TOTAL", vieScolaireTotal.Effectif, vieScolaireTotal.ETP, srcID},
		{anneePersonnelCategorie, "ADMINISTRATIVE", administrative.Effectif, administrative.ETP, srcID},
		{anneePersonnelCategorie, "SANTE_SOCIALE", santeSociale.Effectif, santeSociale.ETP, srcID},
		{anneePersonnelCategorie, "TECHNIQUE", technique.Effectif, technique.ETP, srcID},
		{anneePersonnelCategorie, "ASS_TOTAL", assTotal.Effectif, assTotal.ETP, srcID},
		{anneePersonnelCategorie, "ITRF", itrf.Effectif, itrf.ETP, srcID},
		{anneePersonnelCategorie, "NON_ENSEIGNANTS_TOTAL", nonEnseignantsTotal.Effectif, nonEnseignantsTotal.ETP, srcID},
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// Seule la rentrée anneePersonnelCategorie est chargée par ce connecteur ;
	// une année future, une fois chargée, ne doit pas être touchée ici.
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		CREATE OR REPLACE TEMPORARY VIEW education_personnel_categorie_scope AS
		SELECT * FROM core.education_personnel_categorie WHERE annee = %d
		WITH LOCAL CHECK OPTION`, anneePersonnelCategorie)); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_education_personnel_categorie (
			annee smallint, categorie text, effectif integer, etp numeric, source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_education_personnel_categorie"},
		[]string{"annee", "categorie", "effectif", "etp", "source_id"}, pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("education_personnel_categorie : %w", err))
	}
	ct, err := tx.Exec(ctx, `
		MERGE INTO education_personnel_categorie_scope AS tgt
		USING tmp_education_personnel_categorie AS src ON tgt.annee = src.annee AND tgt.categorie = src.categorie
		WHEN MATCHED AND (tgt.effectif, tgt.etp, tgt.source_id) IS DISTINCT FROM (src.effectif, src.etp, src.source_id)
		THEN UPDATE SET effectif = src.effectif, etp = src.etp, source_id = src.source_id
		WHEN NOT MATCHED BY TARGET THEN
		     INSERT (annee, categorie, effectif, etp, source_id)
		     VALUES (src.annee, src.categorie, src.effectif, src.etp, src.source_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`)
	if err != nil {
		return fail(fmt.Errorf("fusion education_personnel_categorie : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	n := ct.RowsAffected()
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": len(rows)}, "")
	fmt.Printf("  personnels non enseignants par catégorie (Depp Panorama) : %d lignes reçues, %d touchées par la fusion\n", len(rows), n)
	return nil
}
