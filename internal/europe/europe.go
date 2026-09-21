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
	"github.com/faits-politiques/faits-politiques/internal/logs"
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
	for _, f := range fichiers {
		fetched, err := arch.Fetch(ctx, srcID, runID, base+f+".csv.gz", ".csv.gz")
		if err != nil {
			arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
			return fmt.Errorf("%s : %w", f, err)
		}
		chemins[f] = fetched.Path
	}

	// Ce connecteur ne reconstruit QUE ce qu'il possède : les objets dont
	// l'institution est le Parlement européen.
	for _, q := range []string{
		`DELETE FROM core.topic_assignment t USING core.scrutin s
		  WHERE s.id = t.scrutin_id AND s.institution = 'PARLEMENT_EUROPEEN'`,
		`DELETE FROM core.ballot b USING core.scrutin s
		  WHERE s.id = b.scrutin_id AND s.institution = 'PARLEMENT_EUROPEEN'`,
		`DELETE FROM core.scrutin WHERE institution = 'PARLEMENT_EUROPEEN'`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("remise à zéro : %w", err)
		}
	}

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
	scrutins, err := chargerVotes(ctx, pool, chemins["votes"])
	if err != nil {
		return err
	}
	nBallots, err := chargerVotesNominatifs(ctx, pool, chemins["member_votes"], membres, scrutins, appartenances)
	if err != nil {
		return err
	}
	nThemes, err := chargerEuroVoc(ctx, pool, chemins["eurovoc_concepts"], chemins["eurovoc_concept_votes"], scrutins)
	if err != nil {
		return err
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"groupes": len(groupes), "membres_fr": len(membres),
		"scrutins": len(scrutins), "votes": nBallots, "themes": nThemes}, "")
	logs.Notice("Europe normalisée", "groupes", len(groupes), "eurodeputes_fr", len(membres),
		"scrutins", len(scrutins), "votes", nBallots, "themes_eurovoc", nThemes)
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
	for {
		rec, err := cr.Read()
		if err != nil {
			return nil
		}
		m := map[string]string{}
		for i, h := range head {
			if i < len(rec) {
				m[strings.TrimSpace(h)] = strings.TrimSpace(rec[i])
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

func chargerVotesNominatifs(ctx context.Context, pool *pgxpool.Pool, path string,
	membres, scrutins, groupeDe map[string]int64) (int, error) {

	type ligne struct {
		scrutin, personne int64
		org               *int64
		position          string
	}
	var lignes []ligne
	seen := map[[2]int64]bool{}
	err := lire(path, func(m map[string]string) error {
		pid, ok := membres[m["member_id"]]
		if !ok {
			return nil // eurodéputé non français : hors périmètre
		}
		sid, ok := scrutins[m["vote_id"]]
		if !ok {
			return nil
		}
		pos, ok := positionOf[m["position"]]
		if !ok {
			return nil
		}
		cle := [2]int64{sid, pid}
		if seen[cle] {
			return nil
		}
		seen[cle] = true
		var org *int64
		if g, ok := groupeDe[m["member_id"]]; ok {
			org = &g
		}
		lignes = append(lignes, ligne{sid, pid, org, pos})
		return nil
	})
	if err != nil {
		return 0, err
	}
	n, err := pool.CopyFrom(ctx, pgx.Identifier{"core", "ballot"},
		[]string{"scrutin_id", "person_id", "organization_id", "position"},
		pgx.CopyFromSlice(len(lignes), func(i int) ([]any, error) {
			l := lignes[i]
			var org any
			if l.org != nil {
				org = *l.org
			}
			return []any{l.scrutin, l.personne, org, l.position}, nil
		}))
	return int(n), err
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
