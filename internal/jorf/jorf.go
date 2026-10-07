// Package jorf charge les actes du Journal officiel diffusés par la DILA.
//
// Objet : documenter la CARRIÈRE PUBLIQUE des responsables — nominations dans
// les corps d'État, entrées au Gouvernement, missions temporaires. C'est la
// seule source ouverte qui le fasse : le RNE n'a aucune profondeur, la HATVP ne
// couvre que cinq ans, et les annuaires d'anciens élèves sont fermés.
//
// Ce que le format impose, et qu'aucune astuce ne contourne :
//
//   - aucun élément dédié aux personnes ; les noms sont en prose libre, sous
//     deux graphies dans le même fichier — « GOULARD (Guillaume) » au titre,
//     « Guillaume GOULARD » au corps ;
//   - aucune date de naissance dans les actes de nomination, alors que c'est
//     notre seul discriminant fiable ailleurs ;
//   - 41,9 % de nos élus ont un homonyme exact en base.
//
// D'où la règle de ce connecteur : il EXTRAIT et il QUALIFIE, il n'attribue
// pas. Une mention reste CANDIDAT tant qu'un indice contextuel ou une décision
// humaine ne l'a pas confirmée, et rien de ce qui est CANDIDAT ne doit être
// publié comme un fait.
package jorf

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/balisage"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ConnectorVersion = "jorf-v1"
	MethodVersion    = "mention-v1"
	indexURL         = "https://echanges.dila.gouv.fr/OPENDATA/JORFSIMPLE/"
)

var Source = archive.Source{
	Slug: "jorf", Label: "Journal officiel — édition Lois et décrets",
	Publisher:   "Direction de l'information légale et administrative",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte",
	ReuseClass:  "OPEN",
	Attribution: "Source : DILA, Journal officiel de la République française",
	Cadence:     "deux livraisons par jour",
	Notes: "Les archives incrémentales contiennent aussi des RÉÉDITIONS de fiches " +
		"anciennes : le chargement est un upsert sur l'identifiant, jamais un ajout.",
}

// Les actes qui nous intéressent se reconnaissent à leur titre. Ce filtre
// divise le volume par six et évite de stocker des textes réglementaires qui
// ne nomment personne.
var reNominatif = regexp.MustCompile(
	`(?i)(nomination|nommé|détachement|intégration|mission temporaire|cessation de fonctions|composition du gouvernement)`)

var (
	// « GOULARD (Guillaume) » — la graphie des titres.
	reTitreNom = regexp.MustCompile(`\b([A-ZÉÈÊÀÂÎÔÛÇ][A-ZÉÈÊÀÂÎÔÛÇ'’\-]{2,})\s*\(([^)]{2,40})\)`)
	// « M. Guillaume GOULARD » — la graphie des corps de texte. La particule
	// est acceptée en minuscules : « Amélie de MONTCHALIN » était manquée par
	// une version plus naïve.
	reCorpsNom = regexp.MustCompile(
		`(?:M\.|Mme|MM\.)\s+([A-ZÉÈÊÀÂÎÔÛÇ][\p{L}'’\-]+(?:\s+[A-ZÉÈÊÀÂÎÔÛÇ][\p{L}'’\-]+)?)\s+((?:d['’]|de\s+|du\s+|des\s+|le\s+|la\s+)?[A-ZÉÈÊÀÂÎÔÛÇ][A-ZÉÈÊÀÂÎÔÛÇ'’\-\s]{1,40}?)(?:,|\s+est\b|\s+sont\b|\.|;)`)
	reBlancs = regexp.MustCompile(`\s+`)
)

type joText struct {
	ID              string `xml:"ID"`
	Nature          string `xml:"NATURE"`
	Num             string `xml:"NUM"`
	NOR             string `xml:"NOR"`
	PublicationDate string `xml:"DATE_PUBLI"`
	TextDate        string `xml:"DATE_TEXTE"`
	Title           string `xml:"TITRE"`
	FullTitle       string `xml:"TITREFULL"`
	Ministry        string `xml:"MINISTERE"`
	// Le numéro du Journal officiel où l'acte a paru : « JORF n°0117 du 18 mai
	// 2017 ». C'est le lien vers le sommaire, et la référence qui fait foi.
	PublicationOrigin string `xml:"ORIGINE_PUBLI"`
	// Le corps n'est PAS décodé par un champ de cette structure, et ce n'est pas
	// un oubli : <BLOC_TEXTUEL> ne se trouve pas à la même profondeur selon la
	// publication. Les livraisons quotidiennes le placent directement sous
	// <TEXTE> ; la base complète le range sous <STRUCT><ARTICLE>. Un chemin figé
	// marchait donc sur les quatre décrets récents et rendait vide les deux cents
	// autres — sans erreur, puisqu'un texte sans corps est un cas possible.
	//
	// Le corps est collecté par readBody(), qui PARCOURT le document et prend
	// tous les <BLOC_TEXTUEL><CONTENU> où qu'ils soient.
}

// Ingest charge les N archives les plus récentes. Le dump complet fait un
// gigaoctet et couvre 1970-2025 ; il se charge à part, par le même chemin, une
// fois qu'on aura décidé jusqu'où remonter.
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, archiveCount int) error {
	srcID, err := arch.EnsureSource(ctx, Source)
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

	fIndex, err := arch.Fetch(ctx, srcID, runID, indexURL, ".html")
	if err != nil {
		return fail(err)
	}
	names, err := listArchives(fIndex.Path)
	if err != nil {
		return fail(err)
	}
	if len(names) == 0 {
		return fail(fmt.Errorf("aucune archive listée : le format de l'index a changé"))
	}
	// Les plus récentes d'abord.
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if archiveCount > 0 && archiveCount < len(names) {
		names = names[:archiveCount]
	}

	var b summary
	for _, name := range names {
		f, err := arch.Fetch(ctx, srcID, runID, indexURL+name, ".tar.gz")
		if err != nil {
			return fail(fmt.Errorf("%s : %w", name, err))
		}
		if err := loadArchive(ctx, pool, f.Path, srcID, &b); err != nil {
			return fail(fmt.Errorf("%s : %w", name, err))
		}
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"archives": len(names), "fichiers": b.files, "decodes": b.decoded,
		"echecs": b.failed, "sans_id": b.withoutID, "actes": b.acts,
		"nominatifs": b.nominative, "mentions": b.mentions}, "")
	logs.Notice(fmt.Sprintf("JORF: %s, %s, %s decoded (%d failed, %d without an ID)",
		logs.Plural(len(names), "archive"), logs.Plural(b.files, "file"), logs.Plural(b.decoded, "file"),
		b.failed, b.withoutID))
	logs.Notice(fmt.Sprintf("JORF: %s, of which %d nominative, %s",
		logs.Plural(b.acts, "act"), b.nominative, logs.Plural(b.mentions, "person mentioned")))
	return nil
}

// summary compte ce qui entre et ce qui est écarté. Un fichier ignoré en
// silence est la pire des pannes : la première version de ce connecteur a
// rendu « 1 349 actes » sur 60 archives qui en contenaient huit fois plus,
// sans le moindre message.
type summary struct {
	files, decoded, failed, withoutID, acts, nominative, mentions int
}

// actRow et mentionRow portent une ligne le temps de la COPY groupée :
// tout un tar.gz est décodé en mémoire (quelques milliers de textes, jamais
// le gigaoctet du dump complet à la fois — un seul .tar.gz à la fois) avant
// une poignée d'allers-retours à la base plutôt qu'un par fichier.
type actRow struct {
	id, nature, number, nor, title, fullTitle, ministry, content string
	publicationDate, textDate                                    any
	nominative                                                   bool
}

type mentionRow struct {
	actID, name, firstName, context, origin string
}

func loadArchive(ctx context.Context, pool *pgxpool.Pool, path string, srcID int64, b *summary) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return err
	}
	defer gz.Close()

	var acts []actRow
	var nominativeIDs []string
	var mentions []mentionRow

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Seuls les JORFTEXT portent de la donnée : les JORFCONT sont des
		// sommaires, et les versions.xml de simples pointeurs d'indexation.
		if h.Typeflag != tar.TypeReg || !strings.Contains(h.Name, "JORFTEXT") ||
			!strings.HasSuffix(h.Name, ".xml") {
			continue
		}
		raw, err := io.ReadAll(tr)
		if err != nil {
			return fmt.Errorf("lecture de %s : %w", h.Name, err)
		}
		b.files++
		t, err := decode(raw)
		if err != nil {
			b.failed++
			continue
		}
		if t.ID == "" {
			b.withoutID++
			continue
		}
		b.decoded++

		content := readBody(raw)
		nominative := reNominatif.MatchString(t.FullTitle) || reNominatif.MatchString(t.Title)

		row := actRow{
			id: t.ID, nominative: nominative,
			publicationDate: dateJO(t.PublicationDate), textDate: dateJO(t.TextDate),
		}
		if v := nullIfEmpty(t.Nature); v != nil {
			row.nature = v.(string)
		}
		if v := nullIfEmpty(t.Num); v != nil {
			row.number = v.(string)
		}
		if v := nullIfEmpty(t.NOR); v != nil {
			row.nor = v.(string)
		}
		if v := nullIfEmpty(t.Title); v != nil {
			row.title = v.(string)
		}
		if v := nullIfEmpty(t.FullTitle); v != nil {
			row.fullTitle = v.(string)
		}
		if v := nullIfEmpty(t.Ministry); v != nil {
			row.ministry = v.(string)
		}
		if v := nullIfEmpty(content); v != nil {
			row.content = v.(string)
		}
		acts = append(acts, row)
		b.acts++
		if !nominative {
			continue
		}
		b.nominative++
		nominativeIDs = append(nominativeIDs, t.ID)
		for _, m := range extractMentions(t.FullTitle, content) {
			if m.name == "" {
				continue
			}
			mentions = append(mentions, mentionRow{t.ID, m.name, m.firstName, m.context, m.origin})
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := copyActs(ctx, tx, acts, srcID); err != nil {
		return err
	}
	n, err := copyMentions(ctx, tx, mentions, nominativeIDs)
	if err != nil {
		return err
	}
	b.mentions += n

	return tx.Commit(ctx)
}

// copyActs charge les textes du JO. Upsert : les archives incrémentales
// rééditent des fiches anciennes.
func copyActs(ctx context.Context, tx pgx.Tx, acts []actRow, srcID int64) error {
	// date_publi/date_texte restent du texte dans la table temporaire, casté en
	// ::date au SELECT : le protocole binaire de CopyFrom exige un type Go
	// concordant pour une colonne "date" (time.Time), pas une chaîne — la même
	// leçon qu'ailleurs dans ce dépôt (voir internal/an/normalize.go).
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_acte_jo (
			id text, nature text, numero text, nor text, date_publi text, date_texte text,
			titre text, titre_complet text, ministere text, contenu text, nominatif boolean
		) ON COMMIT DROP`); err != nil {
		return err
	}
	rows := make([][]any, len(acts))
	for i, a := range acts {
		rows[i] = []any{a.id, ntext(a.nature), ntext(a.number), ntext(a.nor), a.publicationDate, a.textDate,
			ntext(a.title), ntext(a.fullTitle), ntext(a.ministry), ntext(a.content), a.nominative}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_acte_jo"},
		[]string{"id", "nature", "numero", "nor", "date_publi", "date_texte", "titre",
			"titre_complet", "ministere", "contenu", "nominatif"},
		pgx.CopyFromRows(rows)); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO core.acte_jo
		  (id, nature, numero, nor, date_publi, date_texte, titre, titre_complet,
		   ministere, contenu, nominatif, source_id)
		SELECT id, nature, numero, nor, date_publi::date, date_texte::date, titre, titre_complet,
		       ministere, contenu, nominatif, $1
		  FROM tmp_acte_jo
		ON CONFLICT (id) DO UPDATE SET
		  titre_complet = EXCLUDED.titre_complet, contenu = EXCLUDED.contenu,
		  date_publi = EXCLUDED.date_publi, date_texte = EXCLUDED.date_texte,
		  nominatif = EXCLUDED.nominatif, charge_le = now()`, srcID)
	return err
}

// ntext ramène une chaîne vide à NULL, pour repasser par une colonne texte
// de table temporaire sans perdre la distinction absence/chaîne vide.
func ntext(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// copyMentions qualifie chaque personne citée sans jamais décider seule du
// rattachement, exactement comme l'ancienne version ligne à ligne — mais le
// rapprochement (nom, prénom) -> core.person se fait en UNE requête pour
// tout l'archive plutôt qu'une par mention : les mentions partagent
// massivement le même nom d'un acte à l'autre (une même personne nommée
// dans plusieurs décrets), et une résolution par nom distinct suffit.
//
// MERGE plutôt que DELETE(scopé par acte_id)+COPY, sur une vue restreinte aux
// actes nominatifs de ce lot : l'ancien DELETE payait le prix des triggers RI
// pour l'intégralité des mentions de ces actes à chaque republication,
// changement ou non. Pas de clé naturelle publiée : chaque correspondance
// regex devient sa propre ligne sans déduplication, et de vrais doublons
// existent (acte_id/nom/prenom/origine/contexte identiques) — rang fixe la
// position d'apparition dans l'acte (migration 0187), la même logique que
// core.declaration_item (HATVP).
func copyMentions(ctx context.Context, tx pgx.Tx, mentions []mentionRow, nominativeIDs []string) (int, error) {
	if len(mentions) == 0 {
		return 0, nil
	}

	type nameKey struct{ name, firstName string }
	keys := map[nameKey]bool{}
	for _, m := range mentions {
		keys[nameKey{m.name, m.firstName}] = true
	}
	nameRows := make([][]any, 0, len(keys))
	for c := range keys {
		nameRows = append(nameRows, []any{c.name, c.firstName})
	}

	if _, err := tx.Exec(ctx,
		`CREATE TEMP TABLE tmp_nom_mention (nom text, prenom text) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_nom_mention"}, []string{"nom", "prenom"},
		pgx.CopyFromRows(nameRows)); err != nil {
		return 0, err
	}
	// LATERAL + LIMIT 50 reproduit exactement le comportement ligne à ligne :
	// au-delà de 50 homonymes, le compte plafonne à 50 plutôt que de refléter
	// le vrai total. Un défaut préexistant, pas introduit ici.
	rows, err := tx.Query(ctx, `
		SELECT t.nom, t.prenom, array_agg(p.id)
		  FROM tmp_nom_mention t
		  JOIN LATERAL (
		        SELECT id FROM core.person
		         WHERE core.f_unaccent(lower(family_name)) = core.f_unaccent(lower(t.nom))
		           AND core.f_unaccent(lower(given_name))  = core.f_unaccent(lower(t.prenom))
		         LIMIT 50
		       ) p ON true
		 GROUP BY t.nom, t.prenom`)
	if err != nil {
		return 0, err
	}
	resolved := map[nameKey][]int64{}
	for rows.Next() {
		var name, firstName string
		var ids []int64
		if err := rows.Scan(&name, &firstName, &ids); err != nil {
			rows.Close()
			return 0, err
		}
		resolved[nameKey{name, firstName}] = ids
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	rankByAct := map[string]int{}
	mentionRows := make([][]any, len(mentions))
	for i, m := range mentions {
		ids := resolved[nameKey{m.name, m.firstName}]
		status := "ABSENT"
		var personID any
		switch {
		case len(ids) == 1:
			// Un seul porteur du nom : CANDIDAT, pas CONFIRMÉ. Sans date de
			// naissance dans l'acte, rien ne prouve que c'est la bonne personne.
			status = "CANDIDAT"
			personID = ids[0]
		case len(ids) > 1:
			status = "AMBIGU"
		}
		rank := rankByAct[m.actID]
		rankByAct[m.actID] = rank + 1
		mentionRows[i] = []any{m.actID, rank, m.name, ntext(m.firstName), ntext(m.context), m.origin,
			personID, status, len(ids), MethodVersion}
	}

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_acte_jo_mention (
			acte_id text, rang int, nom text, prenom text, contexte text, origine text,
			person_id bigint, statut text, homonymes int, method_version text
		) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_acte_jo_mention"},
		[]string{"acte_id", "rang", "nom", "prenom", "contexte", "origine", "person_id", "statut",
			"homonymes", "method_version"},
		pgx.CopyFromRows(mentionRows)); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`CREATE TEMP TABLE tmp_acte_jo_mention_scope (acte_id text) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	actIDRows := make([][]any, len(nominativeIDs))
	for i, id := range nominativeIDs {
		actIDRows[i] = []any{id}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_acte_jo_mention_scope"}, []string{"acte_id"},
		pgx.CopyFromRows(actIDRows)); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		CREATE OR REPLACE TEMPORARY VIEW acte_jo_mention_scope AS
		  SELECT * FROM core.acte_jo_mention
		   WHERE acte_id IN (SELECT acte_id FROM tmp_acte_jo_mention_scope)
		  WITH LOCAL CHECK OPTION`); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO acte_jo_mention_scope AS tgt
		USING tmp_acte_jo_mention AS src
		ON tgt.acte_id = src.acte_id AND tgt.rang = src.rang
		WHEN MATCHED AND (tgt.nom, tgt.prenom, tgt.contexte, tgt.origine, tgt.person_id, tgt.statut,
		                   tgt.homonymes, tgt.method_version)
		                  IS DISTINCT FROM
		                  (src.nom, src.prenom, src.contexte, src.origine, src.person_id, src.statut,
		                   src.homonymes, src.method_version) THEN
		    UPDATE SET nom = src.nom, prenom = src.prenom, contexte = src.contexte, origine = src.origine,
		               person_id = src.person_id, statut = src.statut, homonymes = src.homonymes,
		               method_version = src.method_version
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (acte_id, rang, nom, prenom, contexte, origine, person_id, statut, homonymes, method_version)
		    VALUES (src.acte_id, src.rang, src.nom, src.prenom, src.contexte, src.origine, src.person_id,
		            src.statut, src.homonymes, src.method_version)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`); err != nil {
		return 0, err
	}
	return len(mentionRows), nil
}

// decode tolère les écarts au XML strict. Les actes du JO enferment du HTML
// dans <CONTENU> : balises non fermées, entités de traitement de texte. Le
// décodeur strict les rejette en bloc, et rejette avec elles les métadonnées
// parfaitement valides qui les précèdent.
func decode(raw []byte) (joText, error) {
	var t joText
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	err := d.Decode(&t)
	return t, err
}

type mention struct {
	name, firstName, context, origin string
}

// extractMentions repère les personnes citées, sous les deux graphies. Les
// doublons entre titre et corps sont conservés : ils portent des contextes
// différents, et c'est le contexte qui permettra de lever un doute.
func extractMentions(title, body string) []mention {
	var out []mention
	for _, m := range reTitreNom.FindAllStringSubmatch(title, -1) {
		out = append(out, mention{
			name: clean(m[1]), firstName: clean(m[2]),
			context: extract(title, m[0]), origin: "TITRE",
		})
	}
	for _, m := range reCorpsNom.FindAllStringSubmatch(body, -1) {
		out = append(out, mention{
			name: clean(m[2]), firstName: clean(m[1]),
			context: extract(body, m[0]), origin: "CORPS",
		})
	}
	return out
}

// extract renvoie ce qui suit le nom : c'est là que se trouve la qualité, seul
// substitut à la date de naissance pour lever une homonymie.
// extract rend les 120 caractères qui suivent le motif. La découpe se fait sur
// des CARACTÈRES et non sur des octets : couper « é » en son milieu produisait
// un 0xc3 orphelin, que PostgreSQL refuse — le chargement s'arrêtait net sur
// « invalid byte sequence for encoding UTF8 ».
func extract(text, pattern string) string {
	i := strings.Index(text, pattern)
	if i < 0 {
		return ""
	}
	rest := []rune(text[i+len(pattern):])
	if len(rest) > 120 {
		rest = rest[:120]
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(rest), ""))
}

func listArchives(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`href="(JORFSIMPLE_[0-9-]+\.tar\.gz)"`)
	var out []string
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out, nil
}

func clean(s string) string {
	return strings.TrimSpace(reBlancs.ReplaceAllString(s, " "))
}

// dateJO neutralise la date sentinelle de la DILA.
//
// Quand la date d'un texte est inconnue, le Journal officiel ne laisse pas le
// champ vide : il écrit « 2999-01-01 ». 419 actes de notre corpus la portent,
// dont dix-huit décrets de composition du Gouvernement des années 1990. Prise
// au mot, elle datait ces décrets du trentième siècle et les rangeait après
// tout le reste — l'ordre chronologique dont dépend la déduction des périodes
// ministérielles s'en trouvait faux, sans que rien ne le signale.
//
// Une sentinelle n'est pas une date. Elle devient NULL, et c'est la date de
// PUBLICATION qui sert alors de repère.
func dateJO(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if len(s) >= 4 && s[:4] > "2100" {
		return nil
	}
	return s
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// readBody récupère le texte des blocs <BLOC_TEXTUEL><CONTENU>. Les
// <SM><CONTENU/> voisins sont ignorés : ils portent la structure du texte, pas
// son contenu.
// readBody assemble les blocs textuels de l'acte, convertis en texte lisible
// par un analyseur lexical — pas par une expression régulière (voir le paquet
// balisage pour ce que la seconde ne sait pas faire).
//
// Le parcours est fait en PROFONDEUR LIBRE : on prend tout <CONTENU> dont le
// parent est <BLOC_TEXTUEL>, sans présumer de l'endroit où il se trouve. La
// DILA range ce bloc directement sous <TEXTE> dans ses livraisons quotidiennes
// et sous <STRUCT><ARTICLE> dans sa base complète ; un chemin figé n'aurait
// jamais lu que l'une des deux, et l'autre serait ressortie vide sans erreur.
//
// Les <CONTENU> de <NOTICE>, <VISAS>, <ABRO> ou <SM> sont volontairement laissés
// de côté : ce sont la notice explicative, les visas et les mentions
// d'abrogation, pas le dispositif. Les prendre ferait entrer dans le corps des
// phrases que l'acte n'édicte pas.
func readBody(raw []byte) string {
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity

	var b strings.Builder
	inBlock := 0
	for {
		t, err := d.Token()
		if err != nil {
			break
		}
		switch v := t.(type) {
		case xml.StartElement:
			switch v.Name.Local {
			case "BLOC_TEXTUEL":
				inBlock++
			case "CONTENU":
				if inBlock == 0 {
					continue
				}
				var inner struct {
					XML string `xml:",innerxml"`
				}
				if err := d.DecodeElement(&inner, &v); err != nil {
					continue
				}
				s := balisage.Texte(inner.XML)
				if s == "" {
					continue
				}
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(s)
			}
		case xml.EndElement:
			if v.Name.Local == "BLOC_TEXTUEL" && inBlock > 0 {
				inBlock--
			}
		}
	}
	return b.String()
}
