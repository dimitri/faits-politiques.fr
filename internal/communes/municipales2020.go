package communes

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les municipales de 2020, sans lesquelles aucune comparaison avant/après
// n'est possible : toutes les séries dont nous disposons — délinquance
// enregistrée 2016-2025, comptes communaux 2018-2025 — se déroulent pendant la
// mandature 2020-2026, et non pendant celle qui commence en mars 2026.
//
// LICENCE. Ces fichiers sont publiés par le ministère de l'Intérieur sans
// licence déclarée (`notspecified` sur data.gouv.fr). Le projet a pour règle
// qu'une absence de licence n'est pas une autorisation. L'exception a été
// demandée et accordée explicitement par le responsable du projet le
// 2026-09-12 (D-036) : la source est donc classée RESTRICTED, affichable avec
// attribution, et exclue de tout export ouvert.
var SourceMunicipales2020 = archive.Source{
	Slug: "municipales-2020", Label: "Élections municipales 2020 — résultats",
	Publisher: "Ministère de l'Intérieur", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Aucune licence déclarée par le producteur",
	ReuseClass:  "RESTRICTED",
	Attribution: "Source : ministère de l'Intérieur, résultats des élections municipales des 15 mars et 28 juin 2020",
	Cadence:     "par scrutin",
	Notes: "Licence non spécifiée : réutilisation autorisée par décision explicite " +
		"du responsable du projet, à ne pas reverser dans un export ouvert. " +
		"Seules les communes de 1 000 habitants et plus sont couvertes : en deçà, " +
		"le scrutin n'est pas de liste.",
}

const (
	Municipales2020Annee = 2020
	Circulaire2020       = 2020
	// Le seuil d'attribution des nuances était plus élevé en 2020 qu'en 2026 :
	// « LNC » couvre les communes en dessous. Ce n'est pas une nuance.
	nuanceNonCommuniquee = "LNC"

	m2020T1URL = "https://static.data.gouv.fr/resources/elections-municipales-2020-resultats/20200525-133704/2020-05-18-resultats-communes-de-1000-et-plus.txt"
	m2020T2URL = "https://static.data.gouv.fr/resources/municipales-2020-resultats-2nd-tour/20200629-192435/2020-06-29-resultats-t2-communes-de-1000-hab-et-plus.txt"

	// Le fichier répète un bloc de douze colonnes par liste, à partir de la
	// dix-neuvième. Les colonnes fixes décrivent la commune et la participation.
	m2020PremiereListe = 18
	m2020TailleBloc    = 12
)

func IngestMunicipales2020(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceMunicipales2020)
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

	connues, err := communesConnues(ctx, pool)
	if err != nil {
		return fail(err)
	}

	type ligne struct {
		tour                 int
		commune, nuance, lib string
		panneau              int
		nom, prenom          string
		voix, cm, cc         any
	}
	var lignes []ligne
	horsCOG := map[string]bool{}

	for tour, u := range map[int]string{1: m2020T1URL, 2: m2020T2URL} {
		f, err := arch.Fetch(ctx, srcID, runID, u, ".txt")
		if err != nil {
			return fail(err)
		}
		recs, err := lireTSVLatin1(f.Path)
		if err != nil {
			return fail(err)
		}
		for _, c := range recs {
			if len(c) <= m2020PremiereListe {
				continue
			}
			insee := codeINSEE(c[0], c[2])
			if insee == "" {
				continue
			}
			if !connues[insee] {
				// Commune fusionnée ou disparue depuis 2020. Elle est écartée,
				// jamais rattachée de force à une commune actuelle : la
				// correspondance passerait par ref.commune_change et demande
				// une décision qui n'est pas prise ici.
				horsCOG[insee] = true
				continue
			}
			for i := m2020PremiereListe; i+m2020TailleBloc <= len(c)+m2020TailleBloc-1 && i+5 < len(c); i += m2020TailleBloc {
				lib := strings.TrimSpace(c[i+5])
				if lib == "" {
					continue
				}
				pan, err := strconv.Atoi(strings.TrimSpace(c[i]))
				if err != nil {
					continue
				}
				nu := strings.TrimSpace(c[i+1])
				// « LNC » n'est pas une nuance : c'est l'absence d'attribution.
				if nu == nuanceNonCommuniquee {
					nu = ""
				}
				lignes = append(lignes, ligne{
					tour: tour, commune: insee, panneau: pan, nuance: nu, lib: lib,
					nom: strings.TrimSpace(c[i+3]), prenom: strings.TrimSpace(c[i+4]),
					cm:   entier(champ(c, i+6)),
					cc:   entier(champ(c, i+8)),
					voix: entier(champ(c, i+9)),
				})
			}
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY, scopé au scrutin par une vue temporaire :
	// core.municipal_list est partagée avec elections.go (municipales 2026),
	// et l'ancien DELETE payait le prix des triggers RI pour l'intégralité
	// des listes 2020 à chaque republication, changement ou non — ce qui
	// n'arrive plus, le scrutin de 2020 étant clos, mais la table garde le
	// même traitement que sa sœur de 2026 par cohérence.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_municipal_list_2020 (
			scrutin_annee int, tour smallint, commune_code text, cog_millesime int, panneau int,
			nuance_code text, circulaire_millesime int, libelle text, nom_candidat text,
			prenom_candidat text, voix int, sieges_cm int, sieges_cc int, source_id bigint
		) ON COMMIT DROP;
		CREATE OR REPLACE TEMPORARY VIEW municipal_list_2020 AS
		  SELECT * FROM core.municipal_list WHERE scrutin_annee = 2020
		  WITH LOCAL CHECK OPTION`); err != nil {
		return fail(err)
	}

	vus := map[string]bool{}
	var rows [][]any
	for _, l := range lignes {
		k := fmt.Sprintf("%d|%s|%d", l.tour, l.commune, l.panneau)
		if vus[k] {
			continue
		}
		vus[k] = true
		var nuance, mil any
		if l.nuance != "" {
			nuance, mil = l.nuance, Circulaire2020
		}
		rows = append(rows, []any{
			Municipales2020Annee, l.tour, l.commune, COGMillesime, l.panneau,
			nuance, mil, l.lib, nul(l.nom), nul(l.prenom), l.voix, l.cm, l.cc, srcID,
		})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_municipal_list_2020"},
		[]string{"scrutin_annee", "tour", "commune_code", "cog_millesime", "panneau",
			"nuance_code", "circulaire_millesime", "libelle", "nom_candidat",
			"prenom_candidat", "voix", "sieges_cm", "sieges_cc", "source_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return fail(fmt.Errorf("copie des listes 2020 : %w", err))
	}
	var n int64
	err = bulkload.SansContraintesFK(ctx, tx, "core.municipal_list", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO municipal_list_2020 AS tgt
			USING tmp_municipal_list_2020 AS src
			ON tgt.scrutin_annee = src.scrutin_annee AND tgt.tour = src.tour
			   AND tgt.commune_code = src.commune_code AND tgt.panneau = src.panneau
			WHEN MATCHED AND (tgt.cog_millesime, tgt.nuance_code, tgt.circulaire_millesime,
			                   tgt.libelle, tgt.nom_candidat, tgt.prenom_candidat,
			                   tgt.voix, tgt.sieges_cm, tgt.sieges_cc, tgt.source_id)
			                  IS DISTINCT FROM
			                  (src.cog_millesime, src.nuance_code, src.circulaire_millesime,
			                   src.libelle, src.nom_candidat, src.prenom_candidat,
			                   src.voix, src.sieges_cm, src.sieges_cc, src.source_id) THEN
			    UPDATE SET cog_millesime = src.cog_millesime, nuance_code = src.nuance_code,
			               circulaire_millesime = src.circulaire_millesime, libelle = src.libelle,
			               nom_candidat = src.nom_candidat, prenom_candidat = src.prenom_candidat,
			               voix = src.voix, sieges_cm = src.sieges_cm, sieges_cc = src.sieges_cc,
			               source_id = src.source_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (scrutin_annee, tour, commune_code, cog_millesime, panneau, nuance_code,
			            circulaire_millesime, libelle, nom_candidat, prenom_candidat,
			            voix, sieges_cm, sieges_cc, source_id)
			    VALUES (src.scrutin_annee, src.tour, src.commune_code, src.cog_millesime, src.panneau,
			            src.nuance_code, src.circulaire_millesime, src.libelle, src.nom_candidat,
			            src.prenom_candidat, src.voix, src.sieges_cm, src.sieges_cc, src.source_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return err
		}
		n = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return fail(fmt.Errorf("fusion des listes 2020 : %w", err))
	}

	if _, err := tx.Exec(ctx, `ANALYZE core.municipal_list`); err != nil {
		return fail(err)
	}
	if err := couleurs(ctx, tx, Municipales2020Annee); err != nil {
		return fail(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}

	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"listes": len(rows), "listes_touchees": n, "hors_cog": len(horsCOG)}, "")
	fmt.Printf("  municipales 2020 : %d listes (%d touchées par la fusion), %d communes disparues depuis (écartées)\n",
		len(rows), n, len(horsCOG))
	return nil
}

// codeINSEE reconstruit « 01004 » à partir d'un code département « 1 » et d'un
// code commune « 4 », que le fichier publie sans zéros de remplissage. La Corse
// et l'outre-mer gardent leur code littéral.
func codeINSEE(dep, com string) string {
	dep = strings.TrimSpace(dep)
	com = strings.TrimSpace(com)
	if dep == "" || com == "" {
		return ""
	}
	if _, err := strconv.Atoi(dep); err == nil {
		if len(dep) < 2 {
			dep = "0" + dep
		}
	}
	// Les codes d'outre-mer tiennent déjà sur trois chiffres.
	largeur := 5 - len(dep)
	for len(com) < largeur {
		com = "0" + com
	}
	return dep + com
}

func champ(c []string, i int) string {
	if i < len(c) {
		return c[i]
	}
	return ""
}

// lireTSVLatin1 lit un fichier séparé par des tabulations et encodé en
// ISO-8859-1, sans passer par encoding/csv : les libellés de liste contiennent
// des guillemets non échappés que le lecteur CSV refuserait.
func lireTSVLatin1(path string) ([][]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	texte := latin1TSV(b)
	var out [][]string
	for i, l := range strings.Split(texte, "\n") {
		if i == 0 {
			continue // en-tête
		}
		l = strings.TrimRight(l, "\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		out = append(out, strings.Split(l, "\t"))
	}
	return out, nil
}

func latin1TSV(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}
