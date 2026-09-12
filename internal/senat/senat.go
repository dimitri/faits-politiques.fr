// Package senat ingère la base Dosleg du Sénat.
//
// CORRECTION IMPORTANTE : ce projet a longtemps affirmé que le Sénat ne
// publiait que des positions de groupe, avec les exceptions nommées. C'est
// faux. La base Dosleg contient la table votsen, soit 1,98 million de positions
// NOMINATIVES depuis 2006. L'erreur venait de la présentation des scrutins sur
// le site du Sénat, pas de ses données ouvertes.
package senat

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Version = "senat/1"

var Source = archive.Source{
	Slug: "senat-dosleg", Label: "Sénat — base Dosleg (dossiers législatifs, scrutins, votes)",
	Publisher: "Sénat", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Sénat, base Dosleg, Licence Ouverte",
	Cadence:     "périodique",
	Notes: "Dump PostgreSQL complet de 126 Mo. Contient les votes INDIVIDUELS des " +
		"sénateurs (table votsen) et une classification THÉMATIQUE OFFICIELLE des lois " +
		"(tables the et loithe) — la seule qui existe pour le Parlement français. " +
		"Ne contient pas l'appartenance des sénateurs à un groupe politique.",
}

const DumpURL = "https://data.senat.fr/data/dosleg/dosleg.zip"

// Ingest télécharge le dump, le restaure dans un schéma dédié, puis en extrait
// ce qui entre dans le modèle. Le schéma senat_raw joue ici le rôle que
// raw.record joue pour l'Assemblée : la copie fidèle de ce qui a été publié.
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, workDir string) error {
	srcID, err := arch.EnsureSource(ctx, Source)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, Version)
	if err != nil {
		return err
	}
	f, err := arch.Fetch(ctx, srcID, runID, DumpURL, ".zip")
	if err != nil {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	if err := restaurer(ctx, pool, f.Path, workDir); err != nil {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return fmt.Errorf("restauration : %w", err)
	}

	nSen, nScr, nVot, nThemes, err := extraire(ctx, pool)
	if err != nil {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"senateurs": nSen, "scrutins": nScr, "votes": nVot, "themes": nThemes}, "")
	fmt.Printf("  Sénat         %d sénateurs, %d scrutins, %d votes nominatifs, %d thèmes officiels\n",
		nSen, nScr, nVot, nThemes)
	return nil
}

// restaurer déplie le dump et le charge dans le schéma senat_raw. Le dump vise
// « public » : on le redirige, pour ne jamais écrire à côté du modèle.
func restaurer(ctx context.Context, pool *pgxpool.Pool, zipPath, workDir string) error {
	var dejaLa int
	_ = pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'senat_raw' AND table_name = 'votsen'`).Scan(&dejaLa)
	if dejaLa > 0 {
		return nil // dump déjà restauré : la restauration est coûteuse et idempotente
	}

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, "unzip", "-qo", zipPath, "-d", workDir).CombinedOutput(); err != nil {
		return fmt.Errorf("unzip : %v (%s)", err, out)
	}
	brut, err := os.ReadFile(workDir + "/dosleg.sql")
	if err != nil {
		return err
	}
	sql := strings.ReplaceAll(string(brut), "public.", "senat_raw.")
	sql = strings.ReplaceAll(sql, "SET search_path = public", "SET search_path = senat_raw")

	if _, err := pool.Exec(ctx, `DROP SCHEMA IF EXISTS senat_raw CASCADE; CREATE SCHEMA senat_raw`); err != nil {
		return err
	}
	// Le dump contient des instructions de suppression préalables qui échouent
	// sur un schéma vide : on les laisse échouer plutôt que de le réécrire.
	cmd := exec.CommandContext(ctx, "psql", "-q", "-v", "ON_ERROR_STOP=0")
	cmd.Env = append(os.Environ(), "PGOPTIONS=--search_path=senat_raw")
	cmd.Stdin = strings.NewReader(sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("psql : %v (%s)", err, lastLines(string(out), 3))
	}
	return nil
}

func lastLines(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, " | ")
}

var positionDe = map[string]string{
	"1": "FOR", "2": "AGAINST", "3": "ABSTAIN", "4": "ABSENT",
}

func extraire(ctx context.Context, pool *pgxpool.Pool) (int, int, int, int, error) {
	// Ce connecteur ne reconstruit que ce qu'il possède.
	for _, q := range []string{
		`DELETE FROM core.ballot b USING core.scrutin s
		  WHERE s.id = b.scrutin_id AND s.institution = 'SENAT'`,
		`DELETE FROM core.scrutin WHERE institution = 'SENAT'`,
		`DELETE FROM core.mandate WHERE institution = 'SENAT'`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			return 0, 0, 0, 0, err
		}
	}

	var legID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO core.legislature (institution, numero, validity)
		VALUES ('SENAT', 1, daterange('2006-10-01', NULL))
		ON CONFLICT (institution, numero) DO UPDATE SET numero = EXCLUDED.numero
		RETURNING id`).Scan(&legID); err != nil {
		return 0, 0, 0, 0, err
	}

	// Sénateurs : seuls ceux qui ont effectivement voté entrent dans le modèle.
	var nSen int
	if err := pool.QueryRow(ctx, `
		WITH src AS (
		  SELECT DISTINCT a.autmat AS mat,
		         initcap(lower(trim(a.nomuse))) AS nom,
		         initcap(lower(coalesce(trim(a.prenom),''))) AS prenom
		  FROM senat_raw.auteur a
		  WHERE EXISTS (SELECT 1 FROM senat_raw.votsen v WHERE v.senmat = a.autmat)
		), slugs AS (
		  SELECT mat, nom, prenom,
		         'sen-' || regexp_replace(lower(core.f_unaccent(prenom||'-'||nom)),
		                                  '[^a-z0-9]+','-','g') AS slug
		  FROM src
		), ins AS (
		  INSERT INTO core.person (slug, family_name, given_name)
		  SELECT DISTINCT ON (slug) trim(both '-' from slug), nom, prenom FROM slugs
		  ON CONFLICT (slug) DO UPDATE SET family_name = EXCLUDED.family_name
		  RETURNING id, slug
		)
		INSERT INTO core.person_identifier (person_id, scheme, value)
		SELECT i.id, 'SENAT_MATRICULE', s.mat
		FROM ins i JOIN slugs s ON trim(both '-' from s.slug) = i.slug
		ON CONFLICT (scheme, value) DO NOTHING
		RETURNING 1`).Scan(&nSen); err != nil && !strings.Contains(err.Error(), "no rows") {
		return 0, 0, 0, 0, fmt.Errorf("sénateurs : %w", err)
	}
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM core.person_identifier WHERE scheme='SENAT_MATRICULE'`).Scan(&nSen)

	// Scrutins publics du Sénat.
	var nScr int
	if err := pool.QueryRow(ctx, `
		WITH ins AS (
		  INSERT INTO core.scrutin
		    (slug, institution, legislature_id, source_uid, numero, date_seance,
		     granularite, objet, nb_votants, nb_pour, nb_contre)
		  SELECT 'senat-' || s.sesann || '-' || s.scrnum, 'SENAT', $1,
		         s.sesann || '/' || s.scrnum, s.scrnum::text, s.scrdat::date,
		         'INDIVIDUAL',
		         coalesce(nullif(trim(s.scrint), ''), 'Scrutin ' || s.scrnum),
		         s.scrvot, s.scrpou, s.scrcon
		  FROM senat_raw.scr s
		  WHERE s.scrdat IS NOT NULL
		  ON CONFLICT (institution, source_uid) DO NOTHING
		  RETURNING 1)
		SELECT count(*) FROM ins`, legID).Scan(&nScr); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("scrutins : %w", err)
	}

	// Votes nominatifs.
	//
	// La jointure naïve se faisait sur une CONCATÉNATION (sesann || '/' || scrnum)
	// et sur un character(6) comparé à du text : aucun index n'était utilisable,
	// et PostgreSQL triait 2 millions de lignes. On matérialise donc deux petites
	// tables de correspondance — 4 764 et 971 lignes — sur lesquelles la jointure
	// devient un simple hachage.
	for _, q := range []string{
		`DROP TABLE IF EXISTS senat_raw.map_scrutin, senat_raw.map_personne`,
		`CREATE TABLE senat_raw.map_scrutin AS
		   SELECT split_part(source_uid,'/',1)::bigint AS sesann,
		          split_part(source_uid,'/',2)::bigint AS scrnum,
		          id AS scrutin_id
		     FROM core.scrutin WHERE institution = 'SENAT'`,
		`ALTER TABLE senat_raw.map_scrutin ADD PRIMARY KEY (sesann, scrnum)`,
		`CREATE TABLE senat_raw.map_personne AS
		   SELECT value::char(6) AS senmat, person_id
		     FROM core.person_identifier WHERE scheme = 'SENAT_MATRICULE'`,
		`ALTER TABLE senat_raw.map_personne ADD PRIMARY KEY (senmat)`,
		`ANALYZE senat_raw.map_scrutin`,
		`ANALYZE senat_raw.map_personne`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			return 0, 0, 0, 0, fmt.Errorf("correspondances : %w", err)
		}
	}

	// La clé primaire de votsen garantit l'unicité de (session, scrutin,
	// sénateur) : le DISTINCT ON était inutile, et c'est lui qui imposait le tri.
	var nVot int64
	ct, err := pool.Exec(ctx, `
		INSERT INTO core.ballot (scrutin_id, person_id, position)
		SELECT ms.scrutin_id, mp.person_id,
		       (CASE v.posvotcod WHEN '1' THEN 'FOR' WHEN '2' THEN 'AGAINST'
		                         WHEN '3' THEN 'ABSTAIN' ELSE 'ABSENT' END)::core.vote_position
		  FROM senat_raw.votsen v
		  JOIN senat_raw.map_scrutin  ms ON ms.sesann = v.sesann AND ms.scrnum = v.scrnum
		  JOIN senat_raw.map_personne mp ON mp.senmat = v.senmat
		 WHERE v.posvotcod IS NOT NULL`)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("votes : %w", err)
	}
	nVot = ct.RowsAffected()

	// Dossiers législatifs du Sénat. Ils sont chargés pour une raison précise :
	// la classification thématique officielle porte sur EUX, pas sur les
	// scrutins. Sans eux, les 30 thèmes du Sénat resteraient une nomenclature
	// sans objet rattaché.
	if _, err := pool.Exec(ctx, `
		DELETE FROM core.topic_assignment t USING core.dossier d
		 WHERE d.id = t.dossier_id AND d.institution = 'SENAT'`); err != nil {
		return 0, 0, 0, 0, err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM core.dossier WHERE institution = 'SENAT'`); err != nil {
		return 0, 0, 0, 0, err
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO core.dossier (slug, institution, source_uid, legislature_id, titre)
		SELECT 'senat-loi-' || trim(l.loicod), 'SENAT', trim(l.loicod), $1,
		       coalesce(nullif(trim(l.loiint),''), nullif(trim(l.loitit),''), trim(l.loicod))
		  FROM senat_raw.loi l
		 ON CONFLICT (institution, source_uid) DO NOTHING`, legID); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("dossiers Sénat : %w", err)
	}

	// Classification thématique OFFICIELLE du Sénat : la seule qui existe pour
	// le Parlement français. Elle porte sur les lois, pas sur les scrutins.
	if _, err := pool.Exec(ctx, `
		INSERT INTO ref.taxonomy (version, label, published_at, url, frozen)
		VALUES ('senat','Thèmes du Sénat — classification officielle des lois',
		        '2026-09-12','https://data.senat.fr/dosleg/', false)
		ON CONFLICT (version) DO NOTHING`); err != nil {
		return 0, 0, 0, 0, err
	}
	var nThemes int
	if err := pool.QueryRow(ctx, `
		WITH ins AS (
		  INSERT INTO ref.topic (code, taxonomy_version, label, definition)
		  SELECT 'senat-' || t.thecle, 'senat', t.thelib,
		         'Thème de la classification officielle du Sénat, attribué par ses ' ||
		         'services aux textes de loi. Transcrit tel quel.'
		  FROM senat_raw.the t
		  ON CONFLICT (code) DO UPDATE SET label = EXCLUDED.label
		  RETURNING 1)
		SELECT count(*) FROM ins`).Scan(&nThemes); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("thèmes : %w", err)
	}

	// Rattachement des dossiers à leur thème. C'est une TRANSCRIPTION : la
	// classification est faite par les services du Sénat, pas par ce site.
	var nAff int64
	if ct, err := pool.Exec(ctx, `
		INSERT INTO core.topic_assignment (topic_code, dossier_id, provenance, verification)
		SELECT 'senat-' || lt.thecle, d.id, 'OFFICIAL', 'AUTO_VERIFIED'
		  FROM senat_raw.loithe lt
		  JOIN core.dossier d ON d.institution = 'SENAT' AND d.source_uid = trim(lt.loicod)
		 WHERE EXISTS (SELECT 1 FROM ref.topic t WHERE t.code = 'senat-' || lt.thecle)`); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("affectations thématiques : %w", err)
	} else {
		nAff = ct.RowsAffected()
	}
	fmt.Printf("                %d dossiers du Sénat, %d affectations thématiques officielles\n",
		compter(ctx, pool, `SELECT count(*) FROM core.dossier WHERE institution='SENAT'`), nAff)

	return nSen, nScr, int(nVot), nThemes, nil
}

func compter(ctx context.Context, pool *pgxpool.Pool, q string) int64 {
	var n int64
	_ = pool.QueryRow(ctx, q).Scan(&n)
	return n
}

var _ = pgx.Identifier{}
