// Package europe ingère les scrutins nominatifs du Parlement européen depuis
// HowTheyVote.eu, qui les collecte et les republie sous ODbL.
//
// Choix de conception : les objets européens entrent dans les MÊMES tables que
// ceux de l'Assemblée — un scrutin est un core.scrutin, un eurodéputé une
// core.person, un groupe une core.organization. La colonne « institution »
// suffit à les distinguer, et rien n'oblige à comparer les deux : ce sont des
// espaces de vote distincts, non superposables.
package europe

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/faits-politiques/faits-politiques/internal/watermark"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Version = "europe/1"

var Source = archive.Source{
	Slug: "howtheyvote", Label: "HowTheyVote.eu — scrutins nominatifs du Parlement européen",
	Publisher: "HowTheyVote.eu", Tier: "SECONDARY_PRESS",
	Licence: "ODbL", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : HowTheyVote.eu, données sous licence ODbL",
	Cadence:     "hebdomadaire",
	Notes: "Collecte et republie les scrutins publiés par le Parlement européen. " +
		"Seuls les votes des eurodéputés FRANÇAIS sont chargés ici : ce site est " +
		"français, et le fichier complet des votes nominatifs pèse 67 Mo compressés.",
}

const base = "https://github.com/HowTheyVote/data/releases/download/2026-09-05/"

var fichiers = []string{
	"members", "groups", "group_memberships", "votes",
	"eurovoc_concepts", "eurovoc_concept_votes", "member_votes",
}

// DownloadTargets liste les URL qu'Ingest récupère, sans les récupérer — pour
// la récupération concurrente inter-connecteurs (voir
// internal/ingest.PrefetchAll, utilisée par fpctl build).
func DownloadTargets() []archive.DownloadTarget {
	out := make([]archive.DownloadTarget, len(fichiers))
	for i, f := range fichiers {
		out[i] = archive.DownloadTarget{
			Nom: "europe-" + f, Source: Source, URL: base + f + ".csv.gz", Ext: ".csv.gz",
		}
	}
	return out
}

// positionOf traduit les positions publiées par HowTheyVote. DID_NOT_VOTE est
// une donnée manquante, pas une position politique.
var positionOf = map[string]string{
	"FOR": "FOR", "AGAINST": "AGAINST", "ABSTENTION": "ABSTAIN", "DID_NOT_VOTE": "ABSENT",
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, Source)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, Version)
	if err != nil {
		return err
	}

	chemins := map[string]string{}
	var hashVotes string
	for _, f := range fichiers {
		fetched, err := arch.Fetch(ctx, srcID, runID, base+f+".csv.gz", ".csv.gz")
		if err != nil {
			arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
			return fmt.Errorf("%s : %w", f, err)
		}
		chemins[f] = fetched.Path
		if f == "member_votes" {
			hashVotes = fetched.SHA256
		}
	}

	// member_votes.csv.gz est le seul fichier assez gros pour valoir un
	// cache (17,6M lignes — voir chargerVotesNominatifs) : sauter sa
	// reconstruction saute aussi la remise à zéro de core.ballot ET de
	// core.scrutin ensemble, jamais l'un sans l'autre — chargerVotes
	// réinsère les scrutins avec un NOUVEL id à chaque passage (upsert sur
	// une table déjà vidée par la remise à zéro, donc jamais un vrai
	// conflit), donc garder les ballots sans garder les scrutins qui les
	// portent romprait la FK dès la remise à zéro suivante. Même gotcha que
	// core.organization pour l'Assemblée (internal/an/normalize.go).
	const scopeBallots = "europe-member-votes"
	skipBallots, raison, err := watermark.FileDiff(ctx, pool, scopeBallots, hashVotes)
	if err != nil {
		return err
	}
	if skipBallots {
		var n int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM core.ballot b JOIN core.scrutin s ON s.id = b.scrutin_id
			 WHERE s.institution = 'PARLEMENT_EUROPEEN'`).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			skipBallots = false
			raison = "watermark says unchanged but core.ballot looks empty for PARLEMENT_EUROPEEN"
		}
	}

	// topic_assignment se reconstruit à part (eurovoc_concepts/
	// eurovoc_concept_votes, deux fichiers indépendants de member_votes) :
	// sa remise à zéro reste inconditionnelle, seules ballot/scrutin suivent
	// le cache ci-dessus.
	if _, err := pool.Exec(ctx, `
		DELETE FROM core.topic_assignment t USING core.scrutin s
		 WHERE s.id = t.scrutin_id AND s.institution = 'PARLEMENT_EUROPEEN'`); err != nil {
		return fmt.Errorf("remise à zéro : %w", err)
	}
	// Ni core.ballot ni core.scrutin ne sont plus wipés ici : chargerVotesNominatifs
	// fait maintenant un MERGE sur core.ballot (voir son commentaire), et ça
	// suppose des scrutin_id STABLES d'un passage à l'autre — chargerVotes
	// upserte déjà sur (institution, source_uid), un DELETE préalable ne
	// faisait que garantir que cet upsert ne rencontre jamais de conflit,
	// donc réattribuait un id neuf à chaque scrutin à chaque passage.

	groupes, err := chargerGroupes(ctx, pool, chemins["groups"])
	if err != nil {
		return err
	}
	membres, err := chargerMembres(ctx, pool, chemins["members"])
	if err != nil {
		return err
	}
	appartenances, err := chargerAppartenances(ctx, pool, chemins["group_memberships"], membres, groupes)
	if err != nil {
		return err
	}

	var scrutins map[string]int64
	var nBallots int
	if skipBallots {
		scrutins, err = scrutinsExistants(ctx, pool)
		if err != nil {
			return err
		}
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM core.ballot b JOIN core.scrutin s ON s.id = b.scrutin_id
			 WHERE s.institution = 'PARLEMENT_EUROPEEN'`).Scan(&nBallots); err != nil {
			return err
		}
	} else {
		logs.Notice("cache invalidated: " + raison)
		scrutins, err = chargerVotes(ctx, pool, chemins["votes"])
		if err != nil {
			return err
		}
		nBallots, err = chargerVotesNominatifs(ctx, pool, chemins["member_votes"], membres, scrutins, appartenances)
		if err != nil {
			return err
		}
		if err := watermark.Record(ctx, pool, scopeBallots, hashVotes); err != nil {
			return err
		}
	}
	nThemes, err := chargerEuroVoc(ctx, pool, chemins["eurovoc_concepts"], chemins["eurovoc_concept_votes"], scrutins)
	if err != nil {
		return err
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"groupes": len(groupes), "membres_fr": len(membres),
		"scrutins": len(scrutins), "votes": nBallots, "themes": nThemes}, "")
	logs.Notice(fmt.Sprintf("European Parliament: %s, %s, %s, %s, %s",
		logs.Plural(len(groupes), "group"), logs.Plural(len(membres), "French MEP"),
		logs.Plural(len(scrutins), "roll-call vote"), logs.Plural(nBallots, "individual ballot"),
		logs.Plural(nThemes, "EuroVoc assignment")))
	return nil
}

// lire ouvre un CSV éventuellement compressé et appelle fn sur chaque ligne.
func lire(path string, fn func(map[string]string) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer zr.Close()
		r = zr
	}
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return err
	}
	head[0] = strings.TrimPrefix(head[0], "\ufeff")
	// Les en-t\u00eates ne changent pas d'une ligne \u00e0 l'autre : les d\u00e9couper une
	// fois ici plut\u00f4t qu'\u00e0 chaque ligne, comme member_votes.csv.gz en fait
	// lire ~10 millions \u2014 r\u00e9p\u00e9ter TrimSpace(h) dessus \u00e0 chaque tour n'a
	// jamais \u00e9t\u00e9 qu'une n\u00e9gligence, jamais un besoin.
	for i, h := range head {
		head[i] = strings.TrimSpace(h)
	}
	// Une seule map, vid\u00e9e et r\u00e9utilis\u00e9e \u00e0 chaque ligne plut\u00f4t que
	// r\u00e9allou\u00e9e : fn() ne fait que LIRE m[...] et en tirer des valeurs
	// propres (aucun appelant ne conserve m elle-m\u00eame apr\u00e8s son retour),
	// donc la r\u00e9utiliser est s\u00fbre et \u00e9vite l'essentiel du co\u00fbt mesur\u00e9 sur
	// le plus gros fichier de ce paquet \u2014 des millions d'allocations de
	// map pour autant de lignes, avant m\u00eame d'\u00e9crire quoi que ce soit en
	// base.
	m := make(map[string]string, len(head))
	for {
		rec, err := cr.Read()
		if err != nil {
			return nil
		}
		clear(m)
		for i, h := range head {
			if i < len(rec) {
				m[h] = strings.TrimSpace(rec[i])
			}
		}
		if err := fn(m); err != nil {
			return err
		}
	}
}

func slugify(s string) string {
	repl := strings.NewReplacer("à", "a", "â", "a", "ä", "a", "é", "e", "è", "e", "ê", "e",
		"ë", "e", "î", "i", "ï", "i", "ô", "o", "ö", "o", "ù", "u", "û", "u", "ü", "u",
		"ç", "c", "ñ", "n", "œ", "oe", "æ", "ae", "ø", "o", "å", "a", "š", "s", "ž", "z")
	s = repl.Replace(strings.ToLower(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// ---------------------------------------------------------------- chargement

func chargerGroupes(ctx context.Context, pool *pgxpool.Pool, path string) (map[string]int64, error) {
	out := map[string]int64{}
	err := lire(path, func(m map[string]string) error {
		code := m["code"]
		if code == "" {
			return nil
		}
		nom := m["label"]
		if nom == "" {
			nom = m["official_label"]
		}
		var id int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO core.organization (slug, kind, name, short_name)
			VALUES ($1,'EP_GROUP',$2,NULLIF($3,''))
			ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`, "ep-"+slugify(code), nom, m["short_label"]).Scan(&id); err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO core.organization_identifier (organization_id, scheme, value)
			VALUES ($1,'EP_GROUP',$2) ON CONFLICT (scheme, value) DO NOTHING`, id, code); err != nil {
			return err
		}
		out[code] = id
		return nil
	})
	return out, err
}

// chargerMembres ne charge que les eurodéputés FRANÇAIS : ce site est français,
// et le fichier complet des votes nominatifs pèse 67 Mo compressés.
func chargerMembres(ctx context.Context, pool *pgxpool.Pool, path string) (map[string]int64, error) {
	type m struct{ id, prenom, nom, naissance string }
	var fr []m
	if err := lire(path, func(r map[string]string) error {
		if r["country_code"] != "FRA" || r["id"] == "" {
			return nil
		}
		fr = append(fr, m{r["id"], r["first_name"], r["last_name"], r["date_of_birth"]})
		return nil
	}); err != nil {
		return nil, err
	}

	// HowTheyVote publie le nom en capitales : on le remet en casse normale,
	// sans quoi les fiches jureraient à côté de celles de l'Assemblée.
	casse := func(s string) string {
		mots := strings.Fields(strings.ToLower(s))
		for i, w := range mots {
			r := []rune(w)
			r[0] = []rune(strings.ToUpper(string(r[0])))[0]
			mots[i] = string(r)
		}
		return strings.Join(mots, " ")
	}

	homonymes := map[string]int{}
	for _, x := range fr {
		homonymes[slugify(x.prenom+" "+x.nom)]++
	}

	out := map[string]int64{}
	for _, x := range fr {
		base := slugify(x.prenom + " " + x.nom)
		slug := base
		if homonymes[base] > 1 {
			slug = base + "-" + x.id
		}
		var pid int64
		if err := pool.QueryRow(ctx, `
			INSERT INTO core.person (slug, family_name, given_name, birth_date)
			VALUES ($1,$2,$3,NULLIF($4,'')::date)
			ON CONFLICT (slug) DO UPDATE SET family_name = EXCLUDED.family_name
			RETURNING id`, slug, casse(x.nom), casse(x.prenom), x.naissance).Scan(&pid); err != nil {
			return nil, fmt.Errorf("eurodéputé %s : %w", x.id, err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO core.person_identifier (person_id, scheme, value)
			VALUES ($1,'EP_MEP',$2) ON CONFLICT (scheme, value) DO NOTHING`, pid, x.id); err != nil {
			return nil, err
		}
		out[x.id] = pid
	}
	return out, nil
}

// chargerAppartenances crée les mandats d'eurodéputé et les appartenances de
// groupe, et retourne le groupe de chaque membre pour ventiler les votes.
func chargerAppartenances(ctx context.Context, pool *pgxpool.Pool, path string,
	membres, groupes map[string]int64) (map[string]int64, error) {

	if _, err := pool.Exec(ctx, `
		DELETE FROM core.mandate WHERE institution = 'PARLEMENT_EUROPEEN'`); err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM core.affiliation WHERE organization_kind = 'EP_GROUP'`); err != nil {
		return nil, err
	}

	dernier := map[string]int64{}
	err := lire(path, func(m map[string]string) error {
		pid, ok := membres[m["member_id"]]
		if !ok {
			return nil
		}
		gid, ok := groupes[m["group_code"]]
		if !ok {
			return nil
		}
		debut, fin := m["start_date"], m["end_date"]
		if debut == "" {
			return nil
		}
		var finArg any
		if fin != "" {
			finArg = fin
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO core.affiliation
			  (person_id, organization_id, organization_kind, validity, declared_via)
			VALUES ($1,$2,'EP_GROUP', daterange($3::date,$4::date,'[]'), 'EP_DECLARATION')
			ON CONFLICT DO NOTHING`, pid, gid, debut, finArg); err != nil &&
			!strings.Contains(err.Error(), "23P01") {
			return err
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO core.mandate (person_id, mandate_type, institution, validity)
			VALUES ($1,'DEPUTE_EUROPEEN','PARLEMENT_EUROPEEN', daterange($2::date,$3::date,'[]'))
			ON CONFLICT DO NOTHING`, pid, debut, finArg); err != nil &&
			!strings.Contains(err.Error(), "23P01") {
			return err
		}
		dernier[m["member_id"]] = gid
		return nil
	})
	return dernier, err
}

// scrutinsExistants relit source_uid -> id depuis core.scrutin, quand le
// cache du bulletin (voir Ingest, scopeBallots) permet de sauter chargerVotes
// : les ids restent ceux d'un run précédent, jamais recalculés.
func scrutinsExistants(ctx context.Context, pool *pgxpool.Pool) (map[string]int64, error) {
	rows, err := pool.Query(ctx, `
		SELECT source_uid, id FROM core.scrutin WHERE institution = 'PARLEMENT_EUROPEEN'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var uid string
		var id int64
		if err := rows.Scan(&uid, &id); err != nil {
			return nil, err
		}
		out[uid] = id
	}
	return out, rows.Err()
}

func chargerVotes(ctx context.Context, pool *pgxpool.Pool, path string) (map[string]int64, error) {
	if _, err := pool.Exec(ctx, `
		INSERT INTO core.legislature (institution, numero, validity)
		VALUES ('PARLEMENT_EUROPEEN', 9, daterange('2019-07-02','2024-07-15'))
		ON CONFLICT (institution, numero) DO UPDATE SET numero = EXCLUDED.numero`); err != nil {
		return nil, err
	}

	type ligne struct {
		slug, uid, numero, date, objet, typeVote string
		pour, contre, abstentions                int
	}
	var lignes []ligne
	if err := lire(path, func(m map[string]string) error {
		id := m["id"]
		if id == "" || len(m["timestamp"]) < 10 {
			return nil
		}
		objet := m["display_title"]
		if objet == "" {
			objet = m["procedure_title"]
		}
		if strings.TrimSpace(objet) == "" {
			objet = "Scrutin " + id // core.scrutin exige un objet non vide
		}
		atoi := func(k string) int { n, _ := strconv.Atoi(m[k]); return n }
		lignes = append(lignes, ligne{"pe-" + id, id, id, m["timestamp"][:10], objet,
			m["description"], atoi("count_for"), atoi("count_against"), atoi("count_abstention")})
		return nil
	}); err != nil {
		return nil, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_scrutin_pe (
			slug text, uid text, numero text, date_seance text, objet text, type_vote text,
			pour int, contre int, abstentions int
		) ON COMMIT DROP`); err != nil {
		return nil, err
	}
	rows := make([][]any, len(lignes))
	for i, l := range lignes {
		rows[i] = []any{l.slug, l.uid, l.numero, l.date, l.objet, l.typeVote,
			l.pour, l.contre, l.abstentions}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_scrutin_pe"},
		[]string{"slug", "uid", "numero", "date_seance", "objet", "type_vote",
			"pour", "contre", "abstentions"},
		pgx.CopyFromRows(rows)); err != nil {
		return nil, err
	}
	res, err := tx.Query(ctx, `
		WITH upsert AS (
			INSERT INTO core.scrutin
			  (slug, institution, source_uid, numero, date_seance, granularite, objet,
			   type_vote, nb_pour, nb_contre, nb_abstentions)
			SELECT slug, 'PARLEMENT_EUROPEEN'::core.institution, uid, numero, date_seance::date,
			       'INDIVIDUAL'::core.scrutin_granularite, objet, NULLIF(type_vote,''),
			       pour, contre, abstentions
			  FROM tmp_scrutin_pe
			ON CONFLICT (institution, source_uid) DO UPDATE SET objet = EXCLUDED.objet
			RETURNING id, source_uid
		)
		SELECT source_uid, id FROM upsert`)
	if err != nil {
		return nil, fmt.Errorf("scrutins PE : %w", err)
	}
	out := map[string]int64{}
	for res.Next() {
		var uid string
		var id int64
		if err := res.Scan(&uid, &id); err != nil {
			res.Close()
			return nil, err
		}
		out[uid] = id
	}
	res.Close()
	if err := res.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// chargerVotesNominatifs charge member_votes.csv.gz DIRECTEMENT dans
// Postgres — le flux décompressé va tel quel, via le protocole COPY, dans une
// table temporaire à colonnes texte ; aucun décodage CSV ne se fait plus côté
// Go. Le fichier porte le vote de chaque eurodéputé (~700, toutes
// nationalités) sur chaque scrutin — 17,6 millions de lignes — dont seule une
// fraction passe le filtre « eurodéputé français », le rapprochement au
// scrutin, et la traduction de la position : tout cela se fait en UNE seule
// INSERT...SELECT, jamais ligne à ligne en Go.
//
// Le gain n'est pas seulement la table de destination (déjà en COPY) : c'est
// le PARSING lui-même. encoding/csv + une map[string]string par ligne, même
// optimisée (voir lire()), reste des dizaines de millions d'allocations et de
// comparaisons de chaînes en Go pour un travail que le COPY natif de Postgres
// fait en C, et que la seule requête SQL qui suit exprime plus court que la
// boucle qu'elle remplace.
func chargerVotesNominatifs(ctx context.Context, pool *pgxpool.Pool, path string,
	membres, scrutins, groupeDe map[string]int64) (int, error) {

	logs.Notice("loading member_votes.csv.gz (17.6M rows, streamed straight into Postgres)")

	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer gz.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// work_mem par défaut (4 Mo) fait déborder sur disque le DISTINCT ON de
	// la fusion plus bas (~1,97M lignes) — mesuré ailleurs dans ce projet
	// sur une jointure de taille comparable (internal/communes/ssmsi.go) :
	// un tri qui tient en mémoire plutôt qu'un "external merge" sur disque.
	// maintenance_work_mem : ADD CONSTRAINT (SansContraintesFK) revalide la
	// FK par un scan ensembliste, qui puise dans ce budget-là, pas work_mem.
	if _, err := tx.Exec(ctx, `SET LOCAL work_mem = '256MB'; SET LOCAL maintenance_work_mem = '1GB'`); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_member_votes (
			rn bigint GENERATED ALWAYS AS IDENTITY,
			vote_id text, member_id text, position text, country_code text, group_code text
		) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	// Les trois petites tables de correspondance (~700 eurodéputés au plus,
	// ~25 000 scrutins) que la boucle Go consultait comme des map[string]int64
	// sont ici de simples tables temporaires : la jointure les remplace.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_membres (member_id text PRIMARY KEY, person_id bigint) ON COMMIT DROP;
		CREATE TEMP TABLE tmp_scrutins (vote_id text PRIMARY KEY, scrutin_id bigint) ON COMMIT DROP;
		CREATE TEMP TABLE tmp_groupe_de (member_id text PRIMARY KEY, organization_id bigint) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_membres"}, []string{"member_id", "person_id"},
		pgx.CopyFromSlice(len(membres), mapCopySource(membres))); err != nil {
		return 0, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_scrutins"}, []string{"vote_id", "scrutin_id"},
		pgx.CopyFromSlice(len(scrutins), mapCopySource(scrutins))); err != nil {
		return 0, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_groupe_de"}, []string{"member_id", "organization_id"},
		pgx.CopyFromSlice(len(groupeDe), mapCopySource(groupeDe))); err != nil {
		return 0, err
	}

	// Le COPY protocol lui-même, en direct depuis le flux décompressé — voir
	// le commentaire de tête : c'est ici que le gain a lieu, pas dans la
	// requête qui suit.
	if _, err := conn.Conn().PgConn().CopyFrom(ctx, gz,
		`COPY tmp_member_votes (vote_id, member_id, position, country_code, group_code)
		 FROM STDIN WITH (FORMAT csv, HEADER true)`); err != nil {
		return 0, fmt.Errorf("chargement de member_votes.csv.gz : %w", err)
	}

	logs.Notice("rebuilding ballots from the raw rows just loaded (this takes a while)")
	// DISTINCT ON reproduit exactement le "premier gagne" de la boucle Go
	// (seen[cle]) : rn porte l'ordre d'arrivée dans le fichier, le même que
	// map[[2]int64]bool y voyait ligne après ligne.
	//
	// MERGE plutôt que DELETE+INSERT (voir internal/senat/senat.go, le même
	// changement sur core.ballot pour le Sénat, pour la mesure complète) :
	// le DELETE+INSERT payait le prix des triggers RI de core.ballot
	// (~120s mesurés sur ces 1,97M lignes) pour la TABLE ENTIÈRE à chaque
	// passage, changement ou non. MERGE ne le paie que pour les lignes
	// réellement neuves/changées/disparues.
	//
	// CREATE OR REPLACE TEMPORARY VIEW ballot_pe, pas MERGE INTO core.ballot
	// directement : mesuré sur le Sénat, cibler la table entière oblige
	// Postgres à visiter TOUTES ses lignes (Assemblée + Sénat + Europe,
	// ~4,9M) pour décider lesquelles laisser tranquilles — 43,7s pour ne
	// rien écrire. La vue filtre institution='PARLEMENT_EUROPEEN' avant la
	// jointure complète, pas après.
	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE TEMPORARY VIEW ballot_pe AS
		  SELECT * FROM core.ballot
		   WHERE scrutin_id IN (SELECT id FROM core.scrutin WHERE institution = 'PARLEMENT_EUROPEEN')
		  WITH LOCAL CHECK OPTION`); err != nil {
		return 0, err
	}
	var nLignes int64
	err = bulkload.SansContraintesFK(ctx, tx, "core.ballot", func() error {
		_, err := tx.Exec(ctx, `
			WITH resolues AS (
				SELECT t.rn, s.scrutin_id, m.person_id, g.organization_id,
				       (CASE t.position
				         WHEN 'FOR' THEN 'FOR' WHEN 'AGAINST' THEN 'AGAINST'
				         WHEN 'ABSTENTION' THEN 'ABSTAIN' WHEN 'DID_NOT_VOTE' THEN 'ABSENT'
				       END)::core.vote_position AS position
				  FROM tmp_member_votes t
				  JOIN tmp_membres m ON m.member_id = t.member_id
				  JOIN tmp_scrutins s ON s.vote_id = t.vote_id
				  LEFT JOIN tmp_groupe_de g ON g.member_id = t.member_id
				 WHERE t.position IN ('FOR','AGAINST','ABSTENTION','DID_NOT_VOTE')
			),
			dedup AS (
				SELECT DISTINCT ON (scrutin_id, person_id) scrutin_id, person_id, organization_id, position
				  FROM resolues
				 ORDER BY scrutin_id, person_id, rn
			)
			MERGE INTO ballot_pe AS tgt
			USING dedup AS src
			ON tgt.scrutin_id = src.scrutin_id AND tgt.person_id = src.person_id
			WHEN MATCHED AND (tgt.position IS DISTINCT FROM src.position
			                   OR tgt.organization_id IS DISTINCT FROM src.organization_id) THEN
			    UPDATE SET position = src.position, organization_id = src.organization_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (scrutin_id, person_id, organization_id, position)
			    VALUES (src.scrutin_id, src.person_id, src.organization_id, src.position)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		return err
	})
	if err != nil {
		return 0, err
	}
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM core.ballot b JOIN core.scrutin s ON s.id = b.scrutin_id
		 WHERE s.institution = 'PARLEMENT_EUROPEEN'`).Scan(&nLignes); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(nLignes), nil
}

// mapCopySource adapte une map[string]int64 en source pour pgx.CopyFromSlice
// — les trois petites tables de correspondance ci-dessus n'ont besoin de rien
// de plus.
func mapCopySource(m map[string]int64) func(int) ([]any, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return func(i int) ([]any, error) {
		return []any{keys[i], m[keys[i]]}, nil
	}
}

// chargerEuroVoc transcrit la classification thématique OFFICIELLE de l'Union :
// EuroVoc est le thésaurus multilingue des institutions européennes, et les
// concepts sont attachés aux votes par la source. Ce n'est donc pas notre
// classement — c'est celui du producteur, repris tel quel.
func chargerEuroVoc(ctx context.Context, pool *pgxpool.Pool,
	pathConcepts, pathLiens string, scrutins map[string]int64) (int, error) {

	if _, err := pool.Exec(ctx, `
		INSERT INTO ref.taxonomy (version, label, published_at, url, frozen)
		VALUES ('eurovoc','EuroVoc — thésaurus multilingue de l''Union européenne',
		        '2026-09-05','https://op.europa.eu/fr/web/eu-vocabularies', false)
		ON CONFLICT (version) DO NOTHING`); err != nil {
		return 0, err
	}

	concepts := map[string]string{}
	if err := lire(pathConcepts, func(m map[string]string) error {
		id, label := m["id"], m["label"]
		if id == "" || label == "" {
			return nil
		}
		code := "eurovoc-" + id
		if _, err := pool.Exec(ctx, `
			INSERT INTO ref.topic (code, taxonomy_version, label, definition)
			VALUES ($1,'eurovoc',$2,$3)
			ON CONFLICT (code) DO UPDATE SET label = EXCLUDED.label`,
			code, label,
			"Concept du thésaurus EuroVoc, attaché à ce scrutin par le producteur de "+
				"la donnée. Ce site ne fait que transcrire cette affectation."); err != nil {
			return err
		}
		concepts[id] = code
		return nil
	}); err != nil {
		return 0, err
	}

	var rows [][]any
	if err := lire(pathLiens, func(m map[string]string) error {
		sid, ok := scrutins[m["vote_id"]]
		if !ok {
			return nil
		}
		code, ok := concepts[m["eurovoc_concept_id"]]
		if !ok {
			return nil
		}
		rows = append(rows, []any{code, sid, "OFFICIAL", "AUTO_VERIFIED"})
		return nil
	}); err != nil {
		return 0, err
	}
	n, err := pool.CopyFrom(ctx, pgx.Identifier{"core", "topic_assignment"},
		[]string{"topic_code", "scrutin_id", "provenance", "verification"},
		pgx.CopyFromRows(rows))
	return int(n), err
}
