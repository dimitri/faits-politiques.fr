package communes

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le service statistique ministériel de la sécurité intérieure publie le nombre
// de faits enregistrés par la police et la gendarmerie, commune par commune,
// pour dix-huit indicateurs, de 2016 à 2025.
//
// Ces chiffres comptent les faits ENREGISTRÉS, pas les faits commis : une
// hausse peut venir d'une hausse de la délinquance, d'une hausse des plaintes,
// d'un changement de doctrine d'enregistrement, ou de l'ouverture d'un
// commissariat. Le SSMSI le dit dans sa documentation.
//
// Et la commune ne commande pas ces forces : police nationale et gendarmerie
// relèvent de l'État. Le chargement rend les séries vérifiables ; il ne fonde
// aucune imputation.
var SourceSSMSI = archive.Source{
	Slug: "ssmsi", Label: "SSMSI — délinquance enregistrée par commune",
	Publisher:   "Service statistique ministériel de la sécurité intérieure",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : SSMSI, bases statistiques de la délinquance enregistrée",
	Cadence:     "annuelle",
	Notes: "Faits enregistrés, non faits commis. Secret statistique sur les " +
		"effectifs faibles : « non diffusé » n'est pas « zéro ».",
}

const ssmsiURL = "https://static.data.gouv.fr/resources/bases-statistiques-communale-departementale-et-regionale-de-la-delinquance-enregistree-par-la-police-et-la-gendarmerie-nationales/20260709-115942/donnee-data.gouv-2025-geographie2026-produit-le2026-06-25.csv.gz"

func IngestSSMSI(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSSMSI)
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

	f, err := arch.Fetch(ctx, srcID, runID, ssmsiURL, ".csv.gz")
	if err != nil {
		return fail(err)
	}

	connues, err := communesConnues(ctx, pool)
	if err != nil {
		return fail(err)
	}

	fh, err := os.Open(f.Path)
	if err != nil {
		return fail(err)
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return fail(err)
	}
	defer gz.Close()

	r := csv.NewReader(gz)
	r.Comma = ';'
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	head, err := r.Read()
	if err != nil {
		return fail(err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(strings.Trim(h, "\ufeff\""))] = i
	}
	for _, n := range []string{"CODGEO_2026", "annee", "indicateur", "unite_de_compte", "nombre", "est_diffuse"} {
		if _, ok := col[n]; !ok {
			return fail(fmt.Errorf("colonne %q absente : le format du SSMSI a changé", n))
		}
	}

	// Cinq millions de lignes : lecture en flux, écriture par COPY, et jamais
	// plus d'un lot en mémoire.
	indicateurs := map[string]string{}
	var lot [][]any
	var nTot, horsCOG int

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM core.commune_delinquance`); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE delinq_in (
		  commune_code text, cog_millesime int, annee int, indicateur_code text,
		  nombre int, taux_pour_mille numeric, diffuse boolean, population int,
		  source_id bigint
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}

	vider := func() error {
		if len(lot) == 0 {
			return nil
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"delinq_in"},
			[]string{"commune_code", "cog_millesime", "annee", "indicateur_code",
				"nombre", "taux_pour_mille", "diffuse", "population", "source_id"},
			pgx.CopyFromRows(lot))
		lot = lot[:0]
		return err
	}

	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		get := func(n string) string {
			i, ok := col[n]
			if !ok || i >= len(rec) {
				return ""
			}
			return strings.Trim(strings.TrimSpace(rec[i]), `"`)
		}
		code := get("CODGEO_2026")
		if code == "" || !connues[code] {
			if code != "" {
				horsCOG++
			}
			continue
		}
		annee, err := strconv.Atoi(get("annee"))
		if err != nil {
			continue
		}
		lib := get("indicateur")
		if lib == "" {
			continue
		}
		code_ind := codeIndicateur(lib)
		indicateurs[code_ind] = lib + "\x00" + get("unite_de_compte")

		// « ndiff » : le secret statistique s'applique. Le nombre reste NULL —
		// il n'est pas mis à zéro, ce qui inventerait une absence de faits.
		diffuse := get("est_diffuse") == "diff"
		lot = append(lot, []any{
			code, COGMillesime, annee, code_ind,
			entierNul(get("nombre")), decimalNul(get("taux_pour_mille")),
			diffuse, entierNul(get("insee_pop")), srcID,
		})
		nTot++
		if len(lot) >= 100000 {
			if err := vider(); err != nil {
				return fail(fmt.Errorf("copie : %w", err))
			}
		}
	}
	if err := vider(); err != nil {
		return fail(fmt.Errorf("copie : %w", err))
	}

	for code, v := range indicateurs {
		p := strings.SplitN(v, "\x00", 2)
		if _, err := tx.Exec(ctx, `
			INSERT INTO ref.indicateur_delinquance (code, libelle, unite_de_compte)
			VALUES ($1,$2,$3)
			ON CONFLICT (code) DO UPDATE SET libelle = EXCLUDED.libelle`,
			code, p[0], p[1]); err != nil {
			return fail(err)
		}
	}

	// DISTINCT ON : la source publie parfois plusieurs unités de compte pour un
	// même indicateur et une même commune. On retient la ligne diffusée, et à
	// défaut la première — plutôt que de laisser la clé primaire faire échouer
	// tout le chargement.
	res, err := tx.Exec(ctx, `
		INSERT INTO core.commune_delinquance
		  (commune_code, cog_millesime, annee, indicateur_code, nombre,
		   taux_pour_mille, diffuse, population, source_id)
		SELECT DISTINCT ON (commune_code, annee, indicateur_code)
		       commune_code, cog_millesime, annee, indicateur_code, nombre,
		       taux_pour_mille, diffuse, population, source_id
		  FROM delinq_in
		 ORDER BY commune_code, annee, indicateur_code, diffuse DESC`)
	if err != nil {
		return fail(fmt.Errorf("insertion : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"lignes": res.RowsAffected(), "indicateurs": len(indicateurs),
		"hors_cog": horsCOG}, "")
	fmt.Printf("  SSMSI : %d séries communales, %d indicateurs (%d lignes lues)\n",
		res.RowsAffected(), len(indicateurs), nTot)
	if horsCOG > 0 {
		fmt.Printf("  %d lignes de communes absentes du COG %d (ignorées)\n", horsCOG, COGMillesime)
	}
	return nil
}

// codeIndicateur fabrique un code stable à partir du libellé publié. Le SSMSI
// ne publie pas de code : le libellé est la seule clé, et il est conservé tel
// quel dans ref.indicateur_delinquance à côté du code.
func codeIndicateur(libelle string) string {
	s := strings.ToLower(libelle)
	for from, to := range map[string]string{
		"à": "a", "â": "a", "ç": "c", "é": "e", "è": "e", "ê": "e", "ë": "e",
		"î": "i", "ï": "i", "ô": "o", "ö": "o", "ù": "u", "û": "u", "ü": "u", "'": " ",
	} {
		s = strings.ReplaceAll(s, from, to)
	}
	var b strings.Builder
	for _, c := range s {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			b.WriteRune(c)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "_"):
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func entierNul(s string) any {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" {
		return nil
	}
	if i := strings.IndexAny(s, ",."); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return n
}

func decimalNul(s string) any {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	if s == "" || s == "NA" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return v
}
