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

type texteJO struct {
	ID        string `xml:"ID"`
	Nature    string `xml:"NATURE"`
	Num       string `xml:"NUM"`
	NOR       string `xml:"NOR"`
	DatePubli string `xml:"DATE_PUBLI"`
	DateTexte string `xml:"DATE_TEXTE"`
	Titre     string `xml:"TITRE"`
	TitreFull string `xml:"TITREFULL"`
	Ministere string `xml:"MINISTERE"`
	// Le corps n'est PAS décodé par un champ de cette structure, et ce n'est pas
	// un oubli : <BLOC_TEXTUEL> ne se trouve pas à la même profondeur selon la
	// publication. Les livraisons quotidiennes le placent directement sous
	// <TEXTE> ; la base complète le range sous <STRUCT><ARTICLE>. Un chemin figé
	// marchait donc sur les quatre décrets récents et rendait vide les deux cents
	// autres — sans erreur, puisqu'un texte sans corps est un cas possible.
	//
	// Le corps est collecté par corps(), qui PARCOURT le document et prend tous
	// les <BLOC_TEXTUEL><CONTENU> où qu'ils soient.
}

// Ingest charge les N archives les plus récentes. Le dump complet fait un
// gigaoctet et couvre 1970-2025 ; il se charge à part, par le même chemin, une
// fois qu'on aura décidé jusqu'où remonter.
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive, nbArchives int) error {
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
	noms, err := listerArchives(fIndex.Path)
	if err != nil {
		return fail(err)
	}
	if len(noms) == 0 {
		return fail(fmt.Errorf("aucune archive listée : le format de l'index a changé"))
	}
	// Les plus récentes d'abord.
	sort.Sort(sort.Reverse(sort.StringSlice(noms)))
	if nbArchives > 0 && nbArchives < len(noms) {
		noms = noms[:nbArchives]
	}

	var b bilan
	for _, nom := range noms {
		f, err := arch.Fetch(ctx, srcID, runID, indexURL+nom, ".tar.gz")
		if err != nil {
			return fail(fmt.Errorf("%s : %w", nom, err))
		}
		if err := chargerArchive(ctx, pool, f.Path, srcID, &b); err != nil {
			return fail(fmt.Errorf("%s : %w", nom, err))
		}
	}

	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"archives": len(noms), "fichiers": b.fichiers, "decodes": b.decodes,
		"echecs": b.echecs, "sans_id": b.sansID, "actes": b.actes,
		"nominatifs": b.nominatifs, "mentions": b.mentions}, "")
	fmt.Printf("  JORF : %d archives, %d fichiers, %d décodés (%d échecs, %d sans identifiant)\n",
		len(noms), b.fichiers, b.decodes, b.echecs, b.sansID)
	fmt.Printf("         %d actes dont %d nominatifs, %d mentions de personnes\n",
		b.actes, b.nominatifs, b.mentions)
	return nil
}

// bilan compte ce qui entre et ce qui est écarté. Un fichier ignoré en
// silence est la pire des pannes : la première version de ce connecteur a
// rendu « 1 349 actes » sur 60 archives qui en contenaient huit fois plus,
// sans le moindre message.
type bilan struct {
	fichiers, decodes, echecs, sansID, actes, nominatifs, mentions int
}

func chargerArchive(ctx context.Context, pool *pgxpool.Pool, chemin string, srcID int64, b *bilan) error {
	fh, err := os.Open(chemin)
	if err != nil {
		return err
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return err
	}
	defer gz.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

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
		b.fichiers++
		t, err := decoder(raw)
		if err != nil {
			b.echecs++
			continue
		}
		if t.ID == "" {
			b.sansID++
			continue
		}
		b.decodes++

		contenu := corps(raw)
		nominatif := reNominatif.MatchString(t.TitreFull) || reNominatif.MatchString(t.Titre)

		// Upsert : les archives incrémentales rééditent des fiches anciennes.
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.acte_jo
			  (id, nature, numero, nor, date_publi, date_texte, titre, titre_complet,
			   ministere, contenu, nominatif, source_id)
			VALUES ($1,$2,$3,$4,$5::date,$6::date,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (id) DO UPDATE SET
			  titre_complet = EXCLUDED.titre_complet, contenu = EXCLUDED.contenu,
			  date_publi = EXCLUDED.date_publi, date_texte = EXCLUDED.date_texte,
			  nominatif = EXCLUDED.nominatif, charge_le = now()`,
			t.ID, nul(t.Nature), nul(t.Num), nul(t.NOR), dateJO(t.DatePubli), dateJO(t.DateTexte),
			nul(t.Titre), nul(t.TitreFull), nul(t.Ministere), nul(contenu),
			nominatif, srcID); err != nil {
			return fmt.Errorf("acte %s : %w", t.ID, err)
		}
		b.actes++
		if !nominatif {
			continue
		}
		b.nominatifs++

		if _, err := tx.Exec(ctx,
			`DELETE FROM core.acte_jo_mention WHERE acte_id = $1`, t.ID); err != nil {
			return err
		}
		for _, m := range extraireMentions(t.TitreFull, contenu) {
			if err := enregistrerMention(ctx, tx, t.ID, m); err != nil {
				return err
			}
			b.mentions++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

// decoder tolère les écarts au XML strict. Les actes du JO enferment du HTML
// dans <CONTENU> : balises non fermées, entités de traitement de texte. Le
// décodeur strict les rejette en bloc, et rejette avec elles les métadonnées
// parfaitement valides qui les précèdent.
func decoder(raw []byte) (texteJO, error) {
	var t texteJO
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	err := d.Decode(&t)
	return t, err
}

type mention struct {
	nom, prenom, contexte, origine string
}

// extraireMentions repère les personnes citées, sous les deux graphies. Les
// doublons entre titre et corps sont conservés : ils portent des contextes
// différents, et c'est le contexte qui permettra de lever un doute.
func extraireMentions(titre, corps string) []mention {
	var out []mention
	for _, m := range reTitreNom.FindAllStringSubmatch(titre, -1) {
		out = append(out, mention{
			nom: propre(m[1]), prenom: propre(m[2]),
			contexte: extrait(titre, m[0]), origine: "TITRE",
		})
	}
	for _, m := range reCorpsNom.FindAllStringSubmatch(corps, -1) {
		out = append(out, mention{
			nom: propre(m[2]), prenom: propre(m[1]),
			contexte: extrait(corps, m[0]), origine: "CORPS",
		})
	}
	return out
}

// enregistrerMention qualifie le rattachement sans jamais le décider seul.
func enregistrerMention(ctx context.Context, tx pgx.Tx, acteID string, m mention) error {
	if m.nom == "" {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id FROM core.person
		 WHERE core.f_unaccent(lower(family_name)) = core.f_unaccent(lower($1))
		   AND core.f_unaccent(lower(given_name))  = core.f_unaccent(lower($2))
		 LIMIT 50`, m.nom, m.prenom)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()

	statut := "ABSENT"
	var personID any
	switch {
	case len(ids) == 1:
		// Un seul porteur du nom : CANDIDAT, pas CONFIRMÉ. Sans date de
		// naissance dans l'acte, rien ne prouve que c'est la bonne personne.
		statut = "CANDIDAT"
		personID = ids[0]
	case len(ids) > 1:
		statut = "AMBIGU"
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO core.acte_jo_mention
		  (acte_id, nom, prenom, contexte, origine, person_id, statut, homonymes, method_version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		acteID, m.nom, nul(m.prenom), nul(m.contexte), m.origine,
		personID, statut, len(ids), MethodVersion)
	return err
}

// extrait renvoie ce qui suit le nom : c'est là que se trouve la qualité, seul
// substitut à la date de naissance pour lever une homonymie.
// extrait rend les 120 caractères qui suivent le motif. La découpe se fait sur
// des CARACTÈRES et non sur des octets : couper « é » en son milieu produisait
// un 0xc3 orphelin, que PostgreSQL refuse — le chargement s'arrêtait net sur
// « invalid byte sequence for encoding UTF8 ».
func extrait(texte, motif string) string {
	i := strings.Index(texte, motif)
	if i < 0 {
		return ""
	}
	suite := []rune(texte[i+len(motif):])
	if len(suite) > 120 {
		suite = suite[:120]
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(suite), ""))
}

func listerArchives(chemin string) ([]string, error) {
	b, err := os.ReadFile(chemin)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`href="(JORFSIMPLE_[0-9-]+\.tar\.gz)"`)
	var out []string
	vus := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		if !vus[m[1]] {
			vus[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out, nil
}

func propre(s string) string {
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

func nul(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// extraireCorps récupère le texte des blocs <BLOC_TEXTUEL><CONTENU>. Les
// <SM><CONTENU/> voisins sont ignorés : ils portent la structure du texte, pas
// son contenu.
// corps assemble les blocs textuels de l'acte, convertis en texte lisible par
// un analyseur lexical — pas par une expression régulière (voir le paquet
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
func corps(raw []byte) string {
	d := xml.NewDecoder(bytes.NewReader(raw))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity

	var b strings.Builder
	dansBloc := 0
	for {
		t, err := d.Token()
		if err != nil {
			break
		}
		switch v := t.(type) {
		case xml.StartElement:
			switch v.Name.Local {
			case "BLOC_TEXTUEL":
				dansBloc++
			case "CONTENU":
				if dansBloc == 0 {
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
			if v.Name.Local == "BLOC_TEXTUEL" && dansBloc > 0 {
				dansBloc--
			}
		}
	}
	return b.String()
}
