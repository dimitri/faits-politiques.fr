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
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/faits-politiques/faits-politiques/internal/watermark"
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

// DownloadTargets liste toutes les URL que le connecteur du Sénat récupère —
// pas seulement celle d'Ingest : internal/ingest.ingestSenat l'enchaîne avec
// IngestSenateurs et IngestCommissions, qui téléchargent chacun la leur.
// Réunies ici pour la récupération concurrente inter-connecteurs (voir
// internal/ingest.PrefetchAll, utilisée par fpctl build).
func DownloadTargets() []archive.DownloadTarget {
	return []archive.DownloadTarget{
		{Nom: "senat-dosleg", Source: Source, URL: DumpURL, Ext: ".zip"},
		{Nom: "senat-senateurs", Source: SourceSenateurs, URL: senateursURL, Ext: ".csv"},
		{Nom: "senat-commissions", Source: SourceSenateurs, URL: commissionsURL, Ext: ".csv"},
	}
}

// Ingest télécharge le dump, le restaure dans un schéma dédié, puis en extrait
// ce qui entre dans le modèle. Le schéma senat_raw joue ici le rôle que
// raw.record joue pour l'Assemblée : la copie fidèle de ce qui a été publié.
// ATTENTION À L'ORDRE. Ce connecteur reconstruit core.dossier pour le Sénat,
// donc avec de nouveaux identifiants. derived.scrutin_topic pointe ces dossiers
// par une clé étrangère ON DELETE CASCADE : ses 4 806 thèmes hérités par la
// navette disparaissent à chaque passage, silencieusement. Il faut relancer la
// cartographie après (`go run ./cmd/ingest -only=carto`), et c'est un contrôle
// de cmd/verify qui l'a révélé, pas une relecture du code.
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

	// Un seul watermark (le sha256 du dump) gouverne DEUX étapes : restaurer
	// (l'unzip+psql, coûteux) ET extraire (la dérivation SQL vers core.*,
	// coûteuse elle aussi — voir extraire, le MERGE sur core.ballot). Calculé
	// une fois ici plutôt que deux fois séparément, pour ne journaliser
	// « cache invalidated » qu'une seule fois.
	const scope = "senat-dosleg"
	unchanged, raison, err := watermark.FileDiff(ctx, pool, scope, f.SHA256)
	if err != nil {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	if unchanged {
		var nBal int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM core.ballot b JOIN core.scrutin s ON s.id = b.scrutin_id
			 WHERE s.institution = 'SENAT'`).Scan(&nBal); err != nil {
			arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
			return err
		}
		if nBal == 0 {
			unchanged = false
			raison = "watermark says unchanged but core.ballot looks empty for SENAT"
		}
	}
	if !unchanged {
		logs.Notice("cache invalidated: " + raison)
	}

	if err := restaurer(ctx, pool, f.Path, workDir, unchanged); err != nil {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return fmt.Errorf("restauration : %w", err)
	}

	nSen, nScr, nVot, nThemes, err := extraire(ctx, pool, unchanged)
	if err != nil {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}
	if !unchanged {
		if err := watermark.Record(ctx, pool, scope, f.SHA256); err != nil {
			arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
			return err
		}
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"senateurs": nSen, "scrutins": nScr, "votes": nVot, "themes": nThemes}, "")
	logs.Notice(fmt.Sprintf("Senate: %s, %s, %s, %s", logs.Plural(nSen, "senator"),
		logs.Plural(nScr, "roll-call vote"), logs.Plural(nVot, "individual ballot"),
		logs.Plural(nThemes, "official topic")))
	return nil
}

// restaurer déplie le dump et le charge dans le schéma senat_raw. Le dump vise
// « public » : on le redirige, pour ne jamais écrire à côté du modèle.
//
// unchanged vient de l'appelant (Ingest), qui a déjà comparé le sha256 du zip
// téléchargé au watermark — la même décision gouverne aussi extraire, donc
// elle se prend UNE FOIS, pas deux. Avant ce watermark, la garde ci-dessous
// testait seulement « senat_raw.votsen existe-t-elle déjà » — vrai pour
// toujours après le premier passage, donc une nouvelle publication du dump
// Dosleg n'aurait jamais été reprise tant que le schéma restait en place.
func restaurer(ctx context.Context, pool *pgxpool.Pool, zipPath, workDir string, unchanged bool) error {
	if unchanged {
		var dejaLa int
		_ = pool.QueryRow(ctx, `
			SELECT count(*) FROM information_schema.tables
			WHERE table_schema = 'senat_raw' AND table_name = 'votsen'`).Scan(&dejaLa)
		if dejaLa > 0 {
			return nil // dump inchangé et déjà restauré : la restauration est coûteuse
		}
		// Le watermark dit « inchangé » mais senat_raw a disparu — état
		// impossible en fonctionnement normal (voir la même garde dans
		// internal/an/scrutins.go) : on restaure quand même plutôt que de
		// faire confiance à un signal qui contredit ce qu'on observe.
		logs.Notice("cache invalidated: watermark says unchanged but senat_raw.votsen is missing")
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
	//
	// -d $DATABASE_URL, explicite : sans elle, psql se rabat sur ses valeurs
	// par défaut (socket Unix local), qui n'existent pas quand Postgres tourne
	// dans un conteneur — invisible en local, où senat_raw existe déjà depuis
	// longtemps et court-circuite cette fonction (dejaLa > 0 ci-dessus), mais
	// immédiat sur une base neuve (CI, ou tout premier chargement).
	cmd := exec.CommandContext(ctx, "psql", "-q", "-v", "ON_ERROR_STOP=0", "-d", os.Getenv("DATABASE_URL"))
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

// comptesExistants relit les quatre totaux qu'extraire rapporte d'ordinaire,
// sans rien recalculer — utilisé quand le watermark permet de sauter la
// reconstruction (voir extraire).
func comptesExistants(ctx context.Context, pool *pgxpool.Pool) (nSen, nScr, nVot, nThemes int, err error) {
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM core.person_identifier WHERE scheme = 'SENAT_MATRICULE'`).Scan(&nSen); err != nil {
		return
	}
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM core.scrutin WHERE institution = 'SENAT'`).Scan(&nScr); err != nil {
		return
	}
	if err = pool.QueryRow(ctx, `
		SELECT count(*) FROM core.ballot b JOIN core.scrutin s ON s.id = b.scrutin_id
		 WHERE s.institution = 'SENAT'`).Scan(&nVot); err != nil {
		return
	}
	err = pool.QueryRow(ctx,
		`SELECT count(*) FROM ref.topic WHERE taxonomy_version = 'senat'`).Scan(&nThemes)
	return
}

func extraire(ctx context.Context, pool *pgxpool.Pool, unchanged bool) (int, int, int, int, error) {
	if unchanged {
		nSen, nScr, nVot, nThemes, err := comptesExistants(ctx, pool)
		if err != nil {
			return 0, 0, 0, 0, err
		}
		return nSen, nScr, nVot, nThemes, nil
	}

	// Ce connecteur ne reconstruit que ce qu'il possède. Le scrutin n'est
	// PLUS wipé ici (voir l'upsert plus bas, ON CONFLICT ... DO UPDATE) :
	// des ids stables sont ce qui permet au MERGE sur core.ballot de ne
	// toucher que ce qui a réellement changé, plutôt que de tout réécrire à
	// chaque passage — un scrutin détruit puis réinséré recevrait un id
	// neuf, et le MERGE ne verrait alors plus jamais rien « déjà en place ».
	if _, err := pool.Exec(ctx, `DELETE FROM core.mandate WHERE institution = 'SENAT'`); err != nil {
		return 0, 0, 0, 0, err
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

	// Scrutins publics du Sénat — upsert à id stable (ON CONFLICT ... DO
	// UPDATE, jamais DO NOTHING après un DELETE) : voir le commentaire en
	// tête de cette fonction, c'est ce qui rend le MERGE des votes possible.
	if _, err := pool.Exec(ctx, `
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
		ON CONFLICT (institution, source_uid) DO UPDATE SET
		  objet = EXCLUDED.objet, nb_votants = EXCLUDED.nb_votants,
		  nb_pour = EXCLUDED.nb_pour, nb_contre = EXCLUDED.nb_contre`, legID); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("scrutins : %w", err)
	}
	var nScr int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM core.scrutin WHERE institution = 'SENAT'`).Scan(&nScr); err != nil {
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
	//
	// MERGE plutôt que DELETE+INSERT : mesuré par EXPLAIN ANALYZE sur cette
	// base (BEGIN/ROLLBACK), l'ancien DELETE (institution='SENAT', 1,65M
	// lignes) + INSERT prenait 4,1s + 95,1s — l'écrasante majorité de ces
	// 95s étant le déclenchement des triggers RI de core.ballot, UNE FOIS
	// PAR LIGNE RÉÉCRITE, même quand rien n'a changé pour cette ligne. Le
	// MERGE ci-dessous, sur la même donnée (rien de changé depuis le
	// dernier passage), a pris 43,7s — un simple balayage plus jointure,
	// zéro ligne à écrire puisque tout correspond déjà (ids de scrutin
	// stables, voir plus haut). Le vrai gain n'est pas ce cas-là mais le
	// suivant : quand seules quelques centaines de votes changent d'un
	// passage à l'autre (une correction, un scrutin de plus), MERGE ne paie
	// le prix des triggers RI que pour CETTE poignée de lignes, jamais pour
	// les 1,65 million — l'ancien DELETE+INSERT payait ce prix INTÉGRAL à
	// chaque fois, qu'un seul vote ait changé ou aucun.
	//
	// bulkload.SansContraintesFK reste utile malgré tout : sur un tout
	// premier chargement (ou une refonte massive de senat_raw), le MERGE
	// écrirait alors la totalité des lignes, et paierait plein tarif de
	// triggers RI sans lui.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	defer tx.Rollback(ctx)
	if err := bulkload.SansContraintesFK(ctx, tx, "core.ballot", func() error {
		_, err := tx.Exec(ctx, `
			MERGE INTO core.ballot AS tgt
			USING (
			    SELECT ms.scrutin_id, mp.person_id,
			           (CASE v.posvotcod WHEN '1' THEN 'FOR' WHEN '2' THEN 'AGAINST'
			                             WHEN '3' THEN 'ABSTAIN' ELSE 'ABSENT' END)::core.vote_position AS position
			      FROM senat_raw.votsen v
			      JOIN senat_raw.map_scrutin  ms ON ms.sesann = v.sesann AND ms.scrnum = v.scrnum
			      JOIN senat_raw.map_personne mp ON mp.senmat = v.senmat
			     WHERE v.posvotcod IS NOT NULL
			) AS src
			ON tgt.scrutin_id = src.scrutin_id AND tgt.person_id = src.person_id
			WHEN MATCHED AND tgt.position IS DISTINCT FROM src.position THEN
			    UPDATE SET position = src.position
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (scrutin_id, person_id, position) VALUES (src.scrutin_id, src.person_id, src.position)
			WHEN NOT MATCHED BY SOURCE AND EXISTS (
			    SELECT 1 FROM core.scrutin s WHERE s.id = tgt.scrutin_id AND s.institution = 'SENAT'
			) THEN DELETE`)
		return err
	}); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("votes : %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, 0, 0, err
	}
	var nVot int64
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM core.ballot b JOIN core.scrutin s ON s.id = b.scrutin_id
		 WHERE s.institution = 'SENAT'`).Scan(&nVot); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("votes : %w", err)
	}

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
	logs.Notice(fmt.Sprintf("%s, %s",
		logs.Plural(int(compter(ctx, pool, `SELECT count(*) FROM core.dossier WHERE institution='SENAT'`)), "Senate bill"),
		logs.Plural(int(nAff), "official topic assignment")))

	return nSen, nScr, int(nVot), nThemes, nil
}

func compter(ctx context.Context, pool *pgxpool.Pool, q string) int64 {
	var n int64
	_ = pool.QueryRow(ctx, q).Scan(&n)
	return n
}

var _ = pgx.Identifier{}
