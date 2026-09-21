package an

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceDossiers = archive.Source{
	Slug: "an-dossiers", Label: "AN — Dossiers législatifs (17e législature)",
	Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
	Licence: "Licence Ouverte", ReuseClass: "ATTRIBUTION",
	Attribution: "Source : Assemblée nationale, open data",
	Cadence:     "continue",
	Notes: "Contient les dossiers et les documents (textes déposés, rapports). " +
		"Les exposés des motifs ne figurent PAS dans le JSON : seuls les titres, " +
		"auteurs, dates et étapes y sont. Un résumé rédigé ne peut donc pas en être " +
		"tiré mécaniquement.",
}

const DossiersURL = Base + "/loi/dossiers_legislatifs/Dossiers_Legislatifs.json.zip"

// rawBox conserve le JSON brut d'un sous-objet dont la forme varie.
type rawBox struct{ Raw json.RawMessage }

func (b *rawBox) UnmarshalJSON(data []byte) error { b.Raw = append([]byte(nil), data...); return nil }

type dossierParlementaire struct {
	UID          flexStr `json:"uid"`
	Legislature  flexStr `json:"legislature"`
	TitreDossier struct {
		Titre       flexStr `json:"titre"`
		TitreChemin flexStr `json:"titreChemin"`
		SenatChemin flexStr `json:"senatChemin"`
	} `json:"titreDossier"`
	ProcedureParlementaire struct {
		Libelle flexStr `json:"libelle"`
	} `json:"procedureParlementaire"`
	Initiateur struct {
		Acteurs rawBox `json:"acteurs"`
		Organes rawBox `json:"organes"`
	} `json:"initiateur"`
	ActesLegislatifs struct {
		ActeLegislatif json.RawMessage `json:"acteLegislatif"`
	} `json:"actesLegislatifs"`
}

type documentAN struct {
	UID    flexStr `json:"uid"`
	Titres struct {
		TitrePrincipal      flexStr `json:"titrePrincipal"`
		TitrePrincipalCourt flexStr `json:"titrePrincipalCourt"`
	} `json:"titres"`
	DenominationStructurelle flexStr `json:"denominationStructurelle"`
	DossierRef               flexStr `json:"dossierRef"`
	CycleDeVie               struct {
		Chrono struct {
			DateDepot flexStr `json:"dateDepot"`
		} `json:"chrono"`
	} `json:"cycleDeVie"`
	Classification struct {
		Type struct {
			Code    flexStr `json:"code"`
			Libelle flexStr `json:"libelle"`
		} `json:"type"`
	} `json:"classification"`
	Auteurs       json.RawMessage `json:"auteurs"`
	CoSignataires json.RawMessage `json:"coSignataires"`
}

// Les types de documents retenus comme « textes » au sens du modèle. Les
// rapports et avis sont des documents de travail, pas des textes soumis au vote.
var documentKind = map[string]string{
	"PRJL":  "PROJET_DE_LOI",
	"PION":  "PROPOSITION_DE_LOI",
	"PIONR": "PROPOSITION_DE_RESOLUTION",
	"PRJLC": "PROJET_DE_LOI",
	"PIONC": "PROPOSITION_DE_LOI",
	"PRJLO": "PROJET_DE_LOI",
	"PIONO": "PROPOSITION_DE_LOI",
}

// ligneAuteur : une ligne d'auteur (dossier ou texte), prête pour une COPY
// directe — ni dossier_author ni texte_author ne portent de contrainte de
// conflit, un aller simple suffit une fois le parent résolu.
type ligneAuteur struct {
	parentUID       string
	personID, orgID *int64
	role            string
	rang            int
}

// NormalizeDossiers reconstruit dossiers, textes et auteurs depuis raw, puis
// rattache les scrutins à leur dossier.
func NormalizeDossiers(ctx context.Context, pool *pgxpool.Pool,
	personByUID, orgByUID map[string]int64) error {

	// La remise à zéro est faite en amont, dans Normalize, en une seule passe
	// ordonnée : les dossiers doivent disparaître avant les organisations, dont
	// dossier_author et texte_author dépendent.

	var legID int64
	if err := pool.QueryRow(ctx,
		`SELECT id FROM core.legislature WHERE institution='ASSEMBLEE_NATIONALE' AND numero=17`).
		Scan(&legID); err != nil {
		return fmt.Errorf("législature : %w", err)
	}

	rows, err := pool.Query(ctx,
		`SELECT DISTINCT ON (natural_key) payload FROM raw.record
		  WHERE record_type = 'an.dossierParlementaire'
		  ORDER BY natural_key, extracted_at DESC, id DESC`)
	if err != nil {
		return err
	}
	var dossiers []dossierParlementaire
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var d dossierParlementaire
		if err := json.Unmarshal(raw, &d); err != nil || d.UID == "" {
			continue
		}
		dossiers = append(dossiers, d)
	}
	rows.Close()

	type ligneDossier struct{ uid, slug, titre, titreChemin, senatChemin string }
	seenSlug := map[string]bool{}
	var lignesDossier []ligneDossier
	var lignesInitiateur []ligneAuteur
	for _, d := range dossiers {
		titre := strings.TrimSpace(d.TitreDossier.Titre.String())
		if titre == "" {
			titre = d.UID.String()
		}
		slug := slugify(titre)
		if slug == "" || seenSlug[slug] {
			slug = strings.Trim(slug+"-"+strings.ToLower(d.UID.String()), "-")
		}
		seenSlug[slug] = true
		lignesDossier = append(lignesDossier, ligneDossier{
			uid: d.UID.String(), slug: slug, titre: titre,
			titreChemin: d.TitreDossier.TitreChemin.String(),
			senatChemin: d.TitreDossier.SenatChemin.String(),
		})
		lignesInitiateur = append(lignesInitiateur, initiateurLignes(d, personByUID, orgByUID)...)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_dossier (
			uid text, slug text, titre text, titre_chemin text, senat_chemin text
		) ON COMMIT DROP`); err != nil {
		return err
	}
	copieDossiers := make([][]any, len(lignesDossier))
	for i, l := range lignesDossier {
		copieDossiers[i] = []any{l.uid, l.slug, l.titre, nullable(l.titreChemin), nullable(l.senatChemin)}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_dossier"},
		[]string{"uid", "slug", "titre", "titre_chemin", "senat_chemin"},
		pgx.CopyFromRows(copieDossiers)); err != nil {
		return err
	}
	res, err := tx.Query(ctx, `
		WITH upsert AS (
			INSERT INTO core.dossier (slug, institution, source_uid, legislature_id, titre, titre_chemin, senat_chemin)
			SELECT slug, 'ASSEMBLEE_NATIONALE'::core.institution, uid, $1, titre, titre_chemin, senat_chemin
			  FROM tmp_dossier
			ON CONFLICT (institution, source_uid) DO UPDATE SET titre = EXCLUDED.titre
			RETURNING id, source_uid
		)
		SELECT source_uid, id FROM upsert`, legID)
	if err != nil {
		return fmt.Errorf("dossiers : %w", err)
	}
	dossierID := map[string]int64{}
	for res.Next() {
		var uid string
		var id int64
		if err := res.Scan(&uid, &id); err != nil {
			res.Close()
			return err
		}
		dossierID[uid] = id
	}
	res.Close()
	if err := res.Err(); err != nil {
		return err
	}

	nInitiateurs, err := copierAuteurs(ctx, tx, "core.dossier_author", "dossier_id", dossierID, lignesInitiateur)
	if err != nil {
		return fmt.Errorf("initiateurs : %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	logs.Notice(fmt.Sprintf("%s (%s)", logs.Plural(len(dossierID), "bill"), logs.Plural(nInitiateurs, "initiator")))

	nTextes, nAuteurs, err := normalizeDocuments(ctx, pool, dossierID, personByUID, orgByUID, seenSlug)
	if err != nil {
		return err
	}
	logs.Notice(fmt.Sprintf("%s (%s and co-signers)", logs.Plural(nTextes, "text"), logs.Plural(nAuteurs, "author")))

	nLies, err := lierScrutins(ctx, pool, dossierID)
	if err != nil {
		return err
	}
	logs.Notice(fmt.Sprintf("%s linked to their bill", logs.Plural(nLies, "vote")))
	return nil
}

// initiateurLignes transcrit l'initiateur publié par la source. Un dossier
// sans initiateur — environ un sur huit — reste sans auteur : l'absence est
// affichée, jamais comblée par une déduction à partir du titre.
//
// Le rang ne progresse que pour les acteurs : les organes qui suivent
// reprennent le rang où les acteurs l'ont laissé, sans l'incrémenter à leur
// tour — un trait du format d'origine, préservé tel quel plutôt que
// « corrigé » au passage à une écriture groupée.
func initiateurLignes(d dossierParlementaire, personByUID, orgByUID map[string]int64) []ligneAuteur {
	var out []ligneAuteur
	uid := d.UID.String()
	rang := 1
	var acteurs struct {
		Acteur json.RawMessage `json:"acteur"`
	}
	if len(d.Initiateur.Acteurs.Raw) > 0 {
		_ = json.Unmarshal(d.Initiateur.Acteurs.Raw, &acteurs)
		for _, e := range asSlice(acteurs.Acteur) {
			var a struct {
				ActeurRef json.RawMessage `json:"acteurRef"`
			}
			if json.Unmarshal(e, &a) != nil {
				continue
			}
			pid, ok := personByUID[str(a.ActeurRef)]
			if !ok {
				continue
			}
			out = append(out, ligneAuteur{parentUID: uid, personID: &pid, role: "INITIATEUR", rang: rang})
			rang++
		}
	}
	var organes struct {
		Organe json.RawMessage `json:"organe"`
	}
	if len(d.Initiateur.Organes.Raw) > 0 {
		_ = json.Unmarshal(d.Initiateur.Organes.Raw, &organes)
		for _, e := range asSlice(organes.Organe) {
			var o struct {
				OrganeRef json.RawMessage `json:"organeRef"`
			}
			if json.Unmarshal(e, &o) != nil {
				continue
			}
			if oid, ok := orgByUID[str(o.OrganeRef)]; ok {
				out = append(out, ligneAuteur{parentUID: uid, orgID: &oid, role: "GOUVERNEMENT", rang: rang})
			}
		}
	}
	return out
}

// copierAuteurs résout parentUID -> id via idByUID puis copie directement
// dans table : ni dossier_author ni texte_author ne portent de contrainte de
// conflit, une COPY simple suffit, sans détour par une table temporaire.
func copierAuteurs(ctx context.Context, tx pgx.Tx, table, parentCol string,
	idByUID map[string]int64, lignes []ligneAuteur) (int, error) {
	if len(lignes) == 0 {
		return 0, nil
	}
	rows := make([][]any, 0, len(lignes))
	for _, l := range lignes {
		pid, ok := idByUID[l.parentUID]
		if !ok {
			continue
		}
		var person, org any
		if l.personID != nil {
			person = *l.personID
		}
		if l.orgID != nil {
			org = *l.orgID
		}
		rows = append(rows, []any{pid, person, org, l.role, l.rang})
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier(strings.Split(table, ".")),
		[]string{parentCol, "person_id", "organization_id", "role", "rang"},
		pgx.CopyFromRows(rows))
	return int(n), err
}

func normalizeDocuments(ctx context.Context, pool *pgxpool.Pool, dossierID map[string]int64,
	personByUID, orgByUID map[string]int64, seenSlug map[string]bool) (int, int, error) {

	rows, err := pool.Query(ctx,
		`SELECT DISTINCT ON (natural_key) payload FROM raw.record
		  WHERE record_type = 'an.document'
		  ORDER BY natural_key, extracted_at DESC, id DESC`)
	if err != nil {
		return 0, 0, err
	}
	var docs []documentAN
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return 0, 0, err
		}
		var d documentAN
		if err := json.Unmarshal(raw, &d); err != nil || d.UID == "" {
			continue
		}
		docs = append(docs, d)
	}
	rows.Close()

	type ligneTexte struct{ uid, dossierUID, slug, kind, titre, dateDepot string }
	var lignesTexte []ligneTexte
	var lignesAuteur []ligneAuteur
	for _, d := range docs {
		kind, ok := documentKind[d.Classification.Type.Code.String()]
		if !ok {
			continue // rapport, avis, annexe : documents de travail
		}
		if _, ok := dossierID[d.DossierRef.String()]; !ok {
			continue // un texte sans dossier rattaché n'est pas exploitable ici
		}
		titre := strings.TrimSpace(d.Titres.TitrePrincipal.String())
		if titre == "" {
			titre = d.UID.String()
		}
		slug := slugify(titre)
		if slug == "" || seenSlug["t-"+slug] {
			slug = strings.Trim(slug+"-"+strings.ToLower(d.UID.String()), "-")
		}
		seenSlug["t-"+slug] = true
		lignesTexte = append(lignesTexte, ligneTexte{
			uid: d.UID.String(), dossierUID: d.DossierRef.String(), slug: slug, kind: kind, titre: titre,
			dateDepot: dateOnly(d.CycleDeVie.Chrono.DateDepot.String()),
		})
		// « Qui propose » est une dimension distincte de « qui vote », et c'est
		// celle qu'aucun outil français n'exploite. Elle est transcrite ici
		// telle que publiée, sans interprétation.
		lignesAuteur = append(lignesAuteur, auteurLignes(d, personByUID, orgByUID)...)
	}
	if len(lignesTexte) == 0 {
		return 0, 0, nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_texte (
			uid text, dossier_id bigint, slug text, kind text, titre text, date_depot text
		) ON COMMIT DROP`); err != nil {
		return 0, 0, err
	}
	copieTextes := make([][]any, len(lignesTexte))
	for i, l := range lignesTexte {
		copieTextes[i] = []any{l.uid, dossierID[l.dossierUID], l.slug, l.kind, l.titre, nullable(l.dateDepot)}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_texte"},
		[]string{"uid", "dossier_id", "slug", "kind", "titre", "date_depot"},
		pgx.CopyFromRows(copieTextes)); err != nil {
		return 0, 0, err
	}
	res, err := tx.Query(ctx, `
		WITH upsert AS (
			INSERT INTO core.texte (slug, dossier_id, institution, source_uid, kind, titre, date_depot)
			SELECT slug, dossier_id, 'ASSEMBLEE_NATIONALE'::core.institution, uid, kind, titre,
			       nullif(date_depot, '')::date
			  FROM tmp_texte
			ON CONFLICT (institution, source_uid) DO UPDATE SET titre = EXCLUDED.titre
			RETURNING id, source_uid
		)
		SELECT source_uid, id FROM upsert`)
	if err != nil {
		return 0, 0, fmt.Errorf("textes : %w", err)
	}
	texteID := map[string]int64{}
	for res.Next() {
		var uid string
		var id int64
		if err := res.Scan(&uid, &id); err != nil {
			res.Close()
			return 0, 0, err
		}
		texteID[uid] = id
	}
	res.Close()
	if err := res.Err(); err != nil {
		return 0, 0, err
	}

	nAuteurs, err := copierAuteurs(ctx, tx, "core.texte_author", "texte_id", texteID, lignesAuteur)
	if err != nil {
		return 0, 0, fmt.Errorf("auteurs de textes : %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return len(texteID), nAuteurs, nil
}

// auteurLignes transcrit auteurs et cosignataires d'un texte, dans cet ordre
// (rang 1 puis 2) — chaque appel à ajouterLignes reprend son propre rang,
// les deux ne se mélangent jamais.
func auteurLignes(d documentAN, personByUID, orgByUID map[string]int64) []ligneAuteur {
	var out []ligneAuteur
	uid := d.UID.String()
	var box struct {
		Auteur json.RawMessage `json:"auteur"`
	}
	if len(d.Auteurs) > 0 {
		_ = json.Unmarshal(d.Auteurs, &box)
		out = append(out, ajouterLignes(uid, box.Auteur, "AUTEUR", 1, personByUID, orgByUID)...)
	}
	var cbox struct {
		CoSignataire json.RawMessage `json:"coSignataire"`
	}
	if len(d.CoSignataires) > 0 {
		_ = json.Unmarshal(d.CoSignataires, &cbox)
		out = append(out, ajouterLignes(uid, cbox.CoSignataire, "COSIGNATAIRE", 2, personByUID, orgByUID)...)
	}
	return out
}

// ajouterLignes : le rang ne progresse que pour les acteurs, jamais pour les
// organes qui suivent — même trait que initiateurLignes, préservé à
// l'identique.
func ajouterLignes(parentUID string, raw json.RawMessage, role string, rang int,
	personByUID, orgByUID map[string]int64) []ligneAuteur {
	var out []ligneAuteur
	for _, e := range asSlice(raw) {
		var a struct {
			ActeurRef json.RawMessage `json:"acteurRef"`
			Organe    struct {
				OrganeRef json.RawMessage `json:"organeRef"`
			} `json:"organe"`
		}
		if err := json.Unmarshal(e, &a); err != nil {
			continue
		}
		if ref := str(a.ActeurRef); ref != "" {
			if pid, ok := personByUID[ref]; ok {
				out = append(out, ligneAuteur{parentUID: parentUID, personID: &pid, role: role, rang: rang})
				rang++
			}
		}
		if ref := str(a.Organe.OrganeRef); ref != "" {
			if oid, ok := orgByUID[ref]; ok {
				out = append(out, ligneAuteur{parentUID: parentUID, orgID: &oid, role: "GOUVERNEMENT", rang: rang})
			}
		}
	}
	return out
}

// lierScrutins rattache chaque scrutin à son dossier quand la source le publie.
// Environ deux scrutins sur trois portent cette référence ; pour les autres,
// l'absence est affichée comme telle plutôt que devinée à partir du libellé.
func lierScrutins(ctx context.Context, pool *pgxpool.Pool, dossierID map[string]int64) (int, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT ON (natural_key) natural_key,
		       payload->'objet'->>'dossierLegislatif'
		  FROM raw.record
		 WHERE record_type = 'an.scrutin' AND payload->'objet'->>'dossierLegislatif' IS NOT NULL
		 ORDER BY natural_key, extracted_at DESC, id DESC`)
	if err != nil {
		return 0, err
	}
	type ligneLien struct {
		uid string
		did int64
	}
	var liens []ligneLien
	for rows.Next() {
		var uid, obj string
		if err := rows.Scan(&uid, &obj); err != nil {
			return 0, err
		}
		var o struct {
			DossierRef string `json:"dossierRef"`
		}
		if json.Unmarshal([]byte(obj), &o) == nil && o.DossierRef != "" {
			if did, ok := dossierID[o.DossierRef]; ok {
				liens = append(liens, ligneLien{uid, did})
			}
		}
	}
	rows.Close()
	if len(liens) == 0 {
		return 0, nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`CREATE TEMP TABLE tmp_lien_scrutin (uid text, dossier_id bigint) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	rowsCopy := make([][]any, len(liens))
	for i, l := range liens {
		rowsCopy[i] = []any{l.uid, l.did}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_lien_scrutin"},
		[]string{"uid", "dossier_id"}, pgx.CopyFromRows(rowsCopy)); err != nil {
		return 0, err
	}
	ct, err := tx.Exec(ctx, `
		UPDATE core.scrutin s SET dossier_id = t.dossier_id
		  FROM tmp_lien_scrutin t
		 WHERE s.institution = 'ASSEMBLEE_NATIONALE' AND s.source_uid = t.uid`)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(ct.RowsAffected()), nil
}

func dateOnly(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return ""
}
