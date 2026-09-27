// Package dette charge les séries qui expliquent la dette publique : son
// encours selon plusieurs définitions, ses détenteurs, ses échéances, son
// coût, et les points de comparaison européens et suisses.
// Voir docs/dette-donnees.md.
//
// Toutes les séries entrent dans le même modèle long (ref.dette_serie,
// core.dette_observation) : les sources ne partagent pas leurs dimensions,
// et un tableau par source empêcherait les contrôles croisés qui font la
// valeur de l'ensemble — la dette négociable de l'AFT contre la détention
// mesurée par la Banque de France, la dette Maastricht de l'INSEE contre
// celle d'Eurostat.
package dette

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "dette-v1"

// Serie reprend une ligne de ref.dette_serie. Les dimensions laissées vides
// prennent la valeur totale ('_T', 'W0') au chargement.
type Serie struct {
	Code             string
	CodeSource       string
	Libelle          string
	Pays             string
	Frequence        string
	Unite            string
	Concept          string
	Mesure           string
	SecteurEmetteur  string
	ZoneDetenteur    string
	SecteurDetenteur string
	Echeance         string
	BaseEcheance     string
	Instrument       string
	MonnaieEmission  string
	Notes            string
	URL              string
}

type Obs struct {
	Periode    string
	Valeur     float64
	Statut     string
	DocumentID int64
}

// lot accumule les séries d'une source avant de les écrire d'un bloc : une
// source se recharge entièrement ou pas du tout. Les producteurs révisent
// leurs séries (l'INSEE à chaque compte trimestriel, Eurostat deux fois
// l'an) : compléter plutôt que remplacer mêlerait deux millésimes.
type lot struct {
	series []*Serie
	obs    map[string][]Obs
}

func nouveauLot() *lot { return &lot{obs: map[string][]Obs{}} }

func (l *lot) ajouter(s *Serie, obs []Obs) {
	if len(obs) == 0 {
		return // une série sans valeur n'apporte rien et fausserait les comptages
	}
	if _, deja := l.obs[s.Code]; !deja {
		l.series = append(l.series, s)
	}
	l.obs[s.Code] = append(l.obs[s.Code], obs...)
}

func (l *lot) nObs() int {
	n := 0
	for _, o := range l.obs {
		n += len(o)
	}
	return n
}

func defaut(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func nul(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// charger remplace toutes les séries de la source par celles du lot, dans
// une transaction.
func charger(ctx context.Context, pool *pgxpool.Pool, srcID int64, l *lot) error {
	if len(l.series) == 0 {
		return fmt.Errorf("aucune série à charger : la source a changé de forme")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// ref.dette_serie et core.dette_observation sont partagées par toutes les
	// sources de dette (INSEE, Eurostat, FMI, Suisse, AFT, Banque de France),
	// chacune avec son propre source_id : des vues scopées reproduisent
	// exactement la portée de l'ancien « DELETE ... WHERE source_id = $1 »,
	// pour que fusionner l'une ne touche jamais les séries d'une autre.
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		CREATE OR REPLACE TEMPORARY VIEW dette_serie_scope AS
		SELECT * FROM ref.dette_serie WHERE source_id = %d
		WITH LOCAL CHECK OPTION`, srcID)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`
		CREATE OR REPLACE TEMPORARY VIEW dette_observation_scope AS
		SELECT * FROM core.dette_observation
		WHERE serie IN (SELECT code FROM ref.dette_serie WHERE source_id = %d)
		WITH LOCAL CHECK OPTION`, srcID)); err != nil {
		return err
	}
	var series, obs [][]any
	vus := map[string]bool{}
	for _, s := range l.series {
		echeance := defaut(s.Echeance, "_T")
		if (echeance == "_T") != (s.BaseEcheance == "") {
			return fmt.Errorf("%s : échéance %q sans base d'échéance cohérente", s.Code, echeance)
		}
		series = append(series, []any{
			s.Code, srcID, s.CodeSource, s.Libelle, s.Pays, s.Frequence, s.Unite, s.Concept,
			s.Mesure, s.SecteurEmetteur, defaut(s.ZoneDetenteur, "W0"),
			defaut(s.SecteurDetenteur, "_T"), echeance, nul(s.BaseEcheance),
			defaut(s.Instrument, "_T"), defaut(s.MonnaieEmission, "_T"), nul(s.Notes), nul(s.URL),
		})
		for _, o := range l.obs[s.Code] {
			cle := s.Code + "|" + o.Periode
			if vus[cle] {
				return fmt.Errorf("%s : période %s en double", s.Code, o.Periode)
			}
			vus[cle] = true
			debut, err := debutPeriode(o.Periode)
			if err != nil {
				return fmt.Errorf("%s : %w", s.Code, err)
			}
			obs = append(obs, []any{s.Code, o.Periode, debut, o.Valeur, nul(o.Statut), o.DocumentID})
		}
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_dette_serie (
			code text NOT NULL,
			source_id bigint NOT NULL,
			code_source text NOT NULL,
			libelle text NOT NULL,
			pays text NOT NULL,
			frequence text NOT NULL,
			unite text NOT NULL,
			concept text NOT NULL,
			mesure text NOT NULL,
			secteur_emetteur text NOT NULL,
			zone_detenteur text NOT NULL,
			secteur_detenteur text NOT NULL,
			echeance text NOT NULL,
			base_echeance text,
			instrument text NOT NULL,
			monnaie_emission text NOT NULL,
			notes text,
			url text
		) ON COMMIT DROP`); err != nil {
		return err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_dette_serie"},
		[]string{"code", "source_id", "code_source", "libelle", "pays", "frequence", "unite",
			"concept", "mesure", "secteur_emetteur", "zone_detenteur", "secteur_detenteur",
			"echeance", "base_echeance", "instrument", "monnaie_emission", "notes", "url"},
		pgx.CopyFromRows(series)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_dette_observation (
			serie text NOT NULL,
			periode text NOT NULL,
			debut date NOT NULL,
			valeur numeric NOT NULL,
			statut text,
			document_id bigint NOT NULL
		) ON COMMIT DROP`); err != nil {
		return err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_dette_observation"},
		[]string{"serie", "periode", "debut", "valeur", "statut", "document_id"},
		pgx.CopyFromRows(obs)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO dette_serie_scope AS tgt
		USING tmp_dette_serie AS src
		ON tgt.code = src.code
		WHEN MATCHED AND (tgt.code_source, tgt.libelle, tgt.pays, tgt.frequence, tgt.unite,
				tgt.concept, tgt.mesure, tgt.secteur_emetteur, tgt.zone_detenteur,
				tgt.secteur_detenteur, tgt.echeance, tgt.base_echeance, tgt.instrument,
				tgt.monnaie_emission, tgt.notes, tgt.url)
			IS DISTINCT FROM (src.code_source, src.libelle, src.pays, src.frequence, src.unite,
				src.concept, src.mesure, src.secteur_emetteur, src.zone_detenteur,
				src.secteur_detenteur, src.echeance, src.base_echeance, src.instrument,
				src.monnaie_emission, src.notes, src.url) THEN
			UPDATE SET code_source = src.code_source, libelle = src.libelle, pays = src.pays,
				frequence = src.frequence, unite = src.unite, concept = src.concept,
				mesure = src.mesure, secteur_emetteur = src.secteur_emetteur,
				zone_detenteur = src.zone_detenteur, secteur_detenteur = src.secteur_detenteur,
				echeance = src.echeance, base_echeance = src.base_echeance,
				instrument = src.instrument, monnaie_emission = src.monnaie_emission,
				notes = src.notes, url = src.url
		WHEN NOT MATCHED BY TARGET THEN
			INSERT (code, source_id, code_source, libelle, pays, frequence, unite, concept,
				mesure, secteur_emetteur, zone_detenteur, secteur_detenteur, echeance,
				base_echeance, instrument, monnaie_emission, notes, url)
			VALUES (src.code, src.source_id, src.code_source, src.libelle, src.pays, src.frequence,
				src.unite, src.concept, src.mesure, src.secteur_emetteur, src.zone_detenteur,
				src.secteur_detenteur, src.echeance, src.base_echeance, src.instrument,
				src.monnaie_emission, src.notes, src.url)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return fmt.Errorf("dette_serie, fusion : %w", err)
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO dette_observation_scope AS tgt
		USING tmp_dette_observation AS src
		ON tgt.serie = src.serie AND tgt.periode = src.periode
		WHEN MATCHED AND (tgt.debut, tgt.valeur, tgt.statut, tgt.document_id)
			IS DISTINCT FROM (src.debut, src.valeur, src.statut, src.document_id) THEN
			UPDATE SET debut = src.debut, valeur = src.valeur, statut = src.statut,
				document_id = src.document_id
		WHEN NOT MATCHED BY TARGET THEN
			INSERT (serie, periode, debut, valeur, statut, document_id)
			VALUES (src.serie, src.periode, src.debut, src.valeur, src.statut, src.document_id)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return fmt.Errorf("dette_observation, fusion : %w", err)
	}
	return tx.Commit(ctx)
}

// debutPeriode lit les trois formes de période des producteurs : 'AAAA',
// 'AAAA-Qn' et 'AAAA-MM'.
func debutPeriode(p string) (time.Time, error) {
	if len(p) < 4 {
		return time.Time{}, fmt.Errorf("période inattendue : %q", p)
	}
	annee, err := strconv.Atoi(p[:4])
	if err != nil {
		return time.Time{}, fmt.Errorf("période inattendue : %q", p)
	}
	mois := 1
	switch {
	case len(p) == 4:
	case len(p) == 7 && strings.HasPrefix(p[4:], "-Q"):
		q, err := strconv.Atoi(p[6:])
		if err != nil || q < 1 || q > 4 {
			return time.Time{}, fmt.Errorf("période inattendue : %q", p)
		}
		mois = 3*(q-1) + 1
	case len(p) == 7 && p[4] == '-':
		mois, err = strconv.Atoi(p[5:])
		if err != nil || mois < 1 || mois > 12 {
			return time.Time{}, fmt.Errorf("période inattendue : %q", p)
		}
	default:
		return time.Time{}, fmt.Errorf("période inattendue : %q", p)
	}
	return time.Date(annee, time.Month(mois), 1, 0, 0, 0, 0, time.UTC), nil
}

// multiplicateur applique UNIT_MULT (puissance de dix) : 6 = millions.
func multiplicateur(unitMult string) (float64, error) {
	if unitMult == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(unitMult)
	if err != nil || n < 0 || n > 12 {
		return 0, fmt.Errorf("UNIT_MULT inattendu : %q", unitMult)
	}
	m := 1.0
	for i := 0; i < n; i++ {
		m *= 10
	}
	return m, nil
}

// executer encadre un connecteur : source, exécution, échec tracé.
func executer(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, src archive.Source,
	f func(srcID, runID int64) (*lot, error)) error {
	srcID, err := arch.EnsureSource(ctx, src)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
	if err != nil {
		return err
	}
	l, err := f(srcID, runID)
	if err == nil {
		err = charger(ctx, pool, srcID, l)
	}
	if err != nil {
		err = fmt.Errorf("%s : %w", src.Slug, err)
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"series": len(l.series), "observations": l.nObs()}, "")
	fmt.Printf("  %-28s %4d séries  %6d observations\n", src.Slug, len(l.series), l.nObs())
	return nil
}

// Ingest charge toutes les sources de la dette. La Banque de France exige une
// clé d'API (WEBSTAT_API_KEY) : sans elle, la détention est sautée avec un
// avertissement plutôt que de bloquer les sources ouvertes.
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	etapes := []func(context.Context, *pgxpool.Pool, *archive.Archive) error{
		IngestINSEE, IngestEurostat, IngestFMI, IngestSuisse, IngestAFT, IngestDepensesFiscales, IngestBanqueDeFrance,
	}
	for _, e := range etapes {
		if err := e(ctx, pool, arch); err != nil {
			return err
		}
	}
	return nil
}
