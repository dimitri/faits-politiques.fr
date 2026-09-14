package aides

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// aide est une ligne de core.aide_nominative avant le rapprochement avec
// SIRENE, qui décide ce qu'on garde du nom et de l'identifiant.
type aide struct {
	reference, identifiant, nom, typeDeclare string
	regime, intitule, instrument, objectif   string
	secteur, region, autorite, operateur     string
	nominal, esb, trancheMin, trancheMax     *float64
	dateOctroi, datePublication              *time.Time
	document                                 int64
}

// sirenDe tire un SIREN d'un identifiant publié : 9 chiffres (SIREN) ou 14
// (SIRET), espaces et points ignorés. La clé de Luhn est vérifiée — un
// identifiant mal saisi rattacherait l'aide à une autre entreprise — sauf pour
// La Poste, dont les SIRET dérogent à la règle.
func sirenDe(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '.' || r == '-' || r == '\u00a0':
		default:
			return "" // lettres : identifiant étranger ou numéro de TVA
		}
	}
	d := b.String()
	if len(d) != 9 && len(d) != 14 {
		return ""
	}
	siren := d[:9]
	if siren == "356000000" {
		return siren
	}
	if !luhn(siren) {
		return ""
	}
	return siren
}

func luhn(s string) bool {
	somme := 0
	for i := len(s) - 1; i >= 0; i-- {
		n := int(s[i] - '0')
		if (len(s)-1-i)%2 == 1 {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		somme += n
	}
	return somme%10 == 0
}

func texte(s string) any {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil
	}
	return s
}

func num(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func date(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

// chargerAides remplace toutes les aides d'une source. Les lignes passent par
// une table temporaire sans contrainte, puis entrent dans core.aide_nominative
// par jointure avec ref.unite_legale : c'est là, et seulement là, qu'on sait
// si le bénéficiaire est une personne morale dont on peut garder le nom.
func chargerAides(ctx context.Context, pool *pgxpool.Pool, source string, srcID int64, aides []aide) (int, int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE aide_charge (
		reference text, siren text, identifiant_publie text, nom_beneficiaire text, type_declare text,
		regime text, intitule text, instrument text, objectif text, secteur text, region text,
		montant_nominal_eur numeric, montant_esb_eur numeric, tranche_min_eur numeric, tranche_max_eur numeric,
		date_octroi date, date_publication date, autorite text, operateur text, document_id bigint) ON COMMIT DROP`); err != nil {
		return 0, 0, err
	}
	rows := make([][]any, 0, len(aides))
	vues := map[string]bool{}
	for _, a := range aides {
		if vues[a.reference] {
			return 0, 0, fmt.Errorf("%s : référence %s en double", source, a.reference)
		}
		vues[a.reference] = true
		var siren any
		if s := sirenDe(a.identifiant); s != "" {
			siren = s
		}
		rows = append(rows, []any{a.reference, siren, texte(a.identifiant), texte(a.nom), texte(a.typeDeclare),
			texte(a.regime), texte(a.intitule), texte(a.instrument), texte(a.objectif), texte(a.secteur), texte(a.region),
			num(a.nominal), num(a.esb), num(a.trancheMin), num(a.trancheMax),
			date(a.dateOctroi), date(a.datePublication), texte(a.autorite), texte(a.operateur), a.document})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"aide_charge"},
		[]string{"reference", "siren", "identifiant_publie", "nom_beneficiaire", "type_declare", "regime", "intitule",
			"instrument", "objectif", "secteur", "region", "montant_nominal_eur", "montant_esb_eur", "tranche_min_eur",
			"tranche_max_eur", "date_octroi", "date_publication", "autorite", "operateur", "document_id"},
		pgx.CopyFromRows(rows)); err != nil {
		return 0, 0, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM core.aide_nominative WHERE source = $1`, source); err != nil {
		return 0, 0, err
	}
	var n, pm int
	if err := tx.QueryRow(ctx, `
		WITH ins AS (
		  INSERT INTO core.aide_nominative
		    (source, reference, siren, identifiant_publie, personne_morale, nom_beneficiaire, type_declare, regime,
		     intitule, instrument, objectif, secteur, region, montant_nominal_eur, montant_esb_eur, tranche_min_eur,
		     tranche_max_eur, date_octroi, date_publication, autorite, operateur, source_id, document_id)
		  SELECT $1, t.reference,
		         CASE WHEN u.siren IS NOT NULL THEN t.siren END,
		         CASE WHEN u.siren IS NOT NULL THEN t.identifiant_publie END,
		         u.siren IS NOT NULL,
		         CASE WHEN u.siren IS NOT NULL THEN t.nom_beneficiaire END,
		         t.type_declare, t.regime, t.intitule, t.instrument, t.objectif, t.secteur, t.region,
		         t.montant_nominal_eur, t.montant_esb_eur, t.tranche_min_eur, t.tranche_max_eur,
		         t.date_octroi, t.date_publication, t.autorite, t.operateur, $2, t.document_id
		  FROM aide_charge t LEFT JOIN ref.unite_legale u ON u.siren = t.siren
		  RETURNING personne_morale)
		SELECT count(*), count(*) FILTER (WHERE personne_morale) FROM ins`, source, srcID).Scan(&n, &pm); err != nil {
		return 0, 0, err
	}
	return n, pm, tx.Commit(ctx)
}

// Le SIREN n'est gardé que pour les personnes morales : sans ref.unite_legale
// chargée, aucune aide ne serait rattachée. On refuse plutôt que de charger
// une table entièrement anonyme sans le dire.
func verifierSirene(ctx context.Context, pool *pgxpool.Pool) error {
	var n int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ref.unite_legale`).Scan(&n); err != nil {
		return err
	}
	if n < 1_000_000 {
		return fmt.Errorf("ref.unite_legale vide ou incomplète (%d lignes) : charger SIRENE d'abord (-only=sirene)", n)
	}
	return nil
}

func executerAides(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, src archive.Source, code string,
	lire func(srcID, runID int64) ([]aide, map[string]any, error)) error {
	if err := verifierSirene(ctx, pool); err != nil {
		return err
	}
	srcID, err := arch.EnsureSource(ctx, src)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		err = fmt.Errorf("%s : %w", src.Slug, err)
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	aides, stats, err := lire(srcID, runID)
	if err != nil {
		return fail(err)
	}
	if len(aides) == 0 {
		return fail(fmt.Errorf("aucune aide lue"))
	}
	n, pm, err := chargerAides(ctx, pool, code, srcID, aides)
	if err != nil {
		return fail(err)
	}
	if stats == nil {
		stats = map[string]any{}
	}
	stats["aides"], stats["personnes_morales"] = n, pm
	arch.EndRun(ctx, runID, "SUCCESS", stats, "")
	fmt.Printf("  %-28s %7d aides, dont %d à des personnes morales de SIRENE\n", src.Slug, n, pm)
	return nil
}

// montantPoint lit un montant à l'anglaise, « 1,234.56 » ; vide = nil.
func montantPoint(s string) (*float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return nil, true
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, false
	}
	return &v, true
}
