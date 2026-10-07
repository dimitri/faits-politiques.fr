// Package hatvp charge les déclarations publiées par la Haute Autorité pour la
// transparence de la vie publique.
//
// C'est la seule source publique qui donne le PARCOURS d'un responsable :
// activités professionnelles des cinq dernières années, mandats détenus,
// rémunérations déclarées année par année. Le Répertoire national des élus, lui,
// ne publie que la mandature en cours et aucune date de fin (D-025).
//
// Ce qui n'est pas publié l'est visiblement. La source remplace les champs
// couverts par le secret — adresse, téléphone, certains montants — par la
// mention « [Données non publiées] ». Elle est transcrite telle quelle plutôt
// que traduite en NULL : « non publié » et « néant » sont deux faits
// différents, et les confondre ferait dire à la base ce que la source ne dit
// pas.
package hatvp

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/bulkload"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "hatvp-v1"

var Source = archive.Source{
	Slug: "hatvp", Label: "HATVP — déclarations d'intérêts et de patrimoine",
	Publisher:   "Haute Autorité pour la transparence de la vie publique",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte",
	ReuseClass:  "OPEN",
	Attribution: "Source : Haute Autorité pour la transparence de la vie publique",
	Cadence:     "en continu",
	Notes: "Les champs couverts par le secret portent la mention « [Données non " +
		"publiées] », transcrite comme telle. Les déclarations de situation " +
		"patrimoniale ne sont publiées que pour les responsables dont la loi le prévoit.",
}

const (
	DeclarationsURL = "https://www.hatvp.fr/livraison/merge/declarations.xml"
	notPublished    = "[Données non publiées]"
)

// Les blocs retenus, avec ce qu'on en extrait. Tout le reste du format est
// ignoré : ce sont des champs d'adresse, de pièce d'identité et de contact,
// c'est-à-dire précisément ce que la Haute Autorité ne publie pas.
var retainedBlocks = map[string]bool{
	// Intérêts
	"activProfCinqDerniereDto":   true,
	"activProfConjointDto":       true,
	"activConsultantDto":         true,
	"fonctionBenevoleDto":        true,
	"mandatElectifDto":           true,
	"participationDirigeantDto":  true,
	"participationFinanciereDto": true,
	"activCollaborateursDto":     true,
	// Patrimoine
	"immeubleDto":           true,
	"comptesBancaireDto":    true,
	"assuranceVieDto":       true,
	"valeursEnBourseDto":    true,
	"valeursNonEnBourseDto": true,
	"fondDto":               true,
	"sciDto":                true,
	"vehiculeDto":           true,
	"bienDiverDto":          true,
	"bienEtrangerDto":       true,
	"autreBienDto":          true,
	"passifDto":             true,
	"revenuMandatDto":       true,
}

type item struct {
	Block        string
	Rank         int
	Description  string
	Employer     string
	Comment      string
	Year         *int
	Amount       *float64
	NotPublished bool
}

type declaration struct {
	UUID, LastName, FirstName string
	BirthDate                 string
	TypeDeclaration           string
	FilingDate                string
	MandateType, BodyLabel    string
	Role                      string
	MandateStart, MandateEnd  string
	Items                     []item
}

func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
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

	f, err := arch.Fetch(ctx, srcID, runID, DeclarationsURL, ".xml")
	if err != nil {
		return fail(err)
	}

	// Lu en flux (walkDeclarations ne garde jamais plus d'une <declaration> en
	// mémoire à la fois — le XML fait 87 Mo, profondément imbriqué), mais
	// ACCUMULÉ ici plutôt qu'écrit ligne à ligne : les déclarations et
	// leurs items décodés sont de petites structures (quelques champs
	// texte/nombre), même par dizaines de milliers ça reste de l'ordre de
	// la centaine de Mo — sans commune mesure avec les milliers d'allers-
	// retours qu'une INSERT par déclaration (et par item) coûtait avant.
	// Un doublon d'uuid dans le flux garde la PREMIÈRE occurrence, comme
	// avant (ON CONFLICT DO NOTHING y suffisait ligne à ligne) : dédupliqué
	// ici en Go, exactement la même règle.
	seen := map[string]bool{}
	var decls []declaration
	if err := walkDeclarations(f.Path, func(d declaration) error {
		if seen[d.UUID] {
			return nil
		}
		seen[d.UUID] = true
		decls = append(decls, d)
		return nil
	}); err != nil {
		return fail(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	// MERGE plutôt que DELETE+COPY : l'ancien DELETE (table entière, ce
	// connecteur en est l'unique propriétaire) payait le prix des triggers RI
	// pour l'intégralité des déclarations et de leurs items à chaque
	// republication de la HATVP, changement ou non. La clé naturelle des
	// déclarations est leur uuid ; celle des items est (déclaration, bloc,
	// rang) — voir la migration 0179, rang n'existant qu'à cette fin.
	//
	// Pas de RETURNING sur le MERGE des déclarations : RETURNING n'émet une
	// ligne QUE pour une action qui se déclenche vraiment, donc une
	// déclaration MATCHED mais inchangée n'en produirait aucune — la carte
	// uuid -> id serait incomplète dès qu'une déclaration existante n'a pas
	// changé. Un SELECT séparé, sans dépendre d'un WHEN, la reconstruit en
	// entier.
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_declaration (
			uuid text, nom text, prenom text, date_naissance date, type_declaration text,
			date_depot timestamptz, type_mandat text, label_organe text, qualite text,
			date_debut_mandat date, date_fin_mandat date
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	declRows := make([][]any, len(decls))
	for i, d := range decls {
		declRows[i] = []any{d.UUID, d.LastName, d.FirstName, dateFR(d.BirthDate), d.TypeDeclaration,
			timestampFR(d.FilingDate), nilIfEmpty(d.MandateType), nilIfEmpty(d.BodyLabel),
			nilIfEmpty(d.Role), dateFR(d.MandateStart), dateFR(d.MandateEnd)}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_declaration"},
		[]string{"uuid", "nom", "prenom", "date_naissance", "type_declaration", "date_depot",
			"type_mandat", "label_organe", "qualite", "date_debut_mandat", "date_fin_mandat"},
		pgx.CopyFromRows(declRows)); err != nil {
		return fail(err)
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO core.declaration AS tgt
		USING tmp_declaration AS src
		ON tgt.uuid = src.uuid
		WHEN MATCHED AND (tgt.nom, tgt.prenom, tgt.date_naissance, tgt.type_declaration,
		                   tgt.date_depot, tgt.type_mandat, tgt.label_organe, tgt.qualite,
		                   tgt.date_debut_mandat, tgt.date_fin_mandat, tgt.source_id)
		                  IS DISTINCT FROM
		                  (src.nom, src.prenom, src.date_naissance, src.type_declaration,
		                   src.date_depot, src.type_mandat, src.label_organe, src.qualite,
		                   src.date_debut_mandat, src.date_fin_mandat, $1) THEN
		    UPDATE SET nom = src.nom, prenom = src.prenom, date_naissance = src.date_naissance,
		               type_declaration = src.type_declaration, date_depot = src.date_depot,
		               type_mandat = src.type_mandat, label_organe = src.label_organe,
		               qualite = src.qualite, date_debut_mandat = src.date_debut_mandat,
		               date_fin_mandat = src.date_fin_mandat, source_id = $1
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (uuid, nom, prenom, date_naissance, type_declaration, date_depot,
		            type_mandat, label_organe, qualite, date_debut_mandat, date_fin_mandat, source_id)
		    VALUES (src.uuid, src.nom, src.prenom, src.date_naissance, src.type_declaration,
		            src.date_depot, src.type_mandat, src.label_organe, src.qualite,
		            src.date_debut_mandat, src.date_fin_mandat, $1)
		WHEN NOT MATCHED BY SOURCE THEN DELETE`, srcID); err != nil {
		return fail(fmt.Errorf("fusion des déclarations : %w", err))
	}
	res, err := tx.Query(ctx, `
		SELECT t.uuid, d.id FROM tmp_declaration t JOIN core.declaration d ON d.uuid = t.uuid`)
	if err != nil {
		return fail(fmt.Errorf("déclarations : %w", err))
	}
	declIDByUUID := map[string]int64{}
	for res.Next() {
		var uuid string
		var id int64
		if err := res.Scan(&uuid, &id); err != nil {
			res.Close()
			return fail(err)
		}
		declIDByUUID[uuid] = id
	}
	res.Close()
	if err := res.Err(); err != nil {
		return fail(err)
	}
	nDecl := len(declIDByUUID)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_declaration_item (
			declaration_id bigint, bloc text, rang int, description text, employeur text,
			commentaire text, annee int, montant numeric, non_publie boolean
		) ON COMMIT DROP`); err != nil {
		return fail(err)
	}
	var itemRows [][]any
	for _, d := range decls {
		declID, ok := declIDByUUID[d.UUID]
		if !ok {
			continue // doublon d'uuid déjà écarté au flux, ou conflit DB
		}
		for _, it := range d.Items {
			itemRows = append(itemRows, []any{declID, it.Block, it.Rank, nilIfEmpty(it.Description),
				nilIfEmpty(it.Employer), nilIfEmpty(it.Comment), it.Year, it.Amount, it.NotPublished})
		}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_declaration_item"},
		[]string{"declaration_id", "bloc", "rang", "description", "employeur", "commentaire",
			"annee", "montant", "non_publie"},
		pgx.CopyFromRows(itemRows)); err != nil {
		return fail(fmt.Errorf("copie des items : %w", err))
	}
	var nItems int
	err = bulkload.SansContraintesFK(ctx, tx, "core.declaration_item", func() error {
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.declaration_item AS tgt
			USING tmp_declaration_item AS src
			ON tgt.declaration_id = src.declaration_id AND tgt.bloc = src.bloc AND tgt.rang = src.rang
			WHEN MATCHED AND (tgt.description, tgt.employeur, tgt.commentaire, tgt.annee,
			                   tgt.montant, tgt.non_publie)
			                  IS DISTINCT FROM
			                  (src.description, src.employeur, src.commentaire, src.annee,
			                   src.montant, src.non_publie) THEN
			    UPDATE SET description = src.description, employeur = src.employeur,
			               commentaire = src.commentaire, annee = src.annee,
			               montant = src.montant, non_publie = src.non_publie
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (declaration_id, bloc, rang, description, employeur, commentaire,
			            annee, montant, non_publie)
			    VALUES (src.declaration_id, src.bloc, src.rang, src.description, src.employeur,
			            src.commentaire, src.annee, src.montant, src.non_publie)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return err
		}
		nItems = int(ct.RowsAffected())
		return nil
	})
	if err != nil {
		return fail(fmt.Errorf("fusion des items : %w", err))
	}

	// Rapprochement sur le triplet EXACT (nom, prénom, date de naissance),
	// insensible aux accents et à la casse — la même règle que pour le RNE
	// (D-025). Sans date de naissance, aucun rapprochement : deux homonymes
	// restent deux personnes.
	rec, err := tx.Exec(ctx, `
		UPDATE core.declaration d SET person_id = p.id
		  FROM core.person p
		 WHERE d.date_naissance IS NOT NULL
		   AND p.birth_date = d.date_naissance
		   AND core.f_unaccent(lower(p.family_name)) = core.f_unaccent(lower(d.nom))
		   AND core.f_unaccent(lower(p.given_name))  = core.f_unaccent(lower(d.prenom))`)
	if err != nil {
		return fail(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"declarations": nDecl, "items": len(itemRows), "items_touchees": nItems,
		"rapprochees": rec.RowsAffected()}, "")
	logs.Notice(fmt.Sprintf("HATVP: %s (%d items, %d touched by the merge), %d matched to a known person",
		logs.Plural(nDecl, "declaration"), len(itemRows), nItems, rec.RowsAffected()))
	return nil
}

// walkDeclarations lit le flux de déclarations sans jamais en garder plus
// d'une en mémoire : le fichier fait 87 Mo et la structure est profondément
// imbriquée.
func walkDeclarations(path string, fn func(declaration) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	dec := xml.NewDecoder(f)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "declaration" {
			continue
		}
		var d declaration
		if err := readDeclaration(dec, &d); err != nil {
			return err
		}
		if d.UUID == "" {
			continue
		}
		if err := fn(d); err != nil {
			return err
		}
	}
}

// readDeclaration consomme une <declaration> jusqu'à sa fermeture.
func readDeclaration(dec *xml.Decoder, d *declaration) error {
	depth := 1
	var path []string
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			path = append(path, t.Name.Local)
			if retainedBlocks[t.Name.Local] {
				items, err := readBlock(dec, t.Name.Local)
				if err != nil {
					return err
				}
				d.Items = append(d.Items, items...)
				depth--
				path = path[:len(path)-1]
				continue
			}
			if t.Name.Local == "general" {
				if err := readGeneral(dec, d); err != nil {
					return err
				}
				depth--
				path = path[:len(path)-1]
				continue
			}
		case xml.CharData:
			v := strings.TrimSpace(string(t))
			if v == "" || len(path) == 0 {
				break
			}
			switch path[len(path)-1] {
			case "uuid":
				if len(path) == 1 {
					d.UUID = v
				}
			case "dateDepot":
				if len(path) == 1 {
					d.FilingDate = v
				}
			}
		case xml.EndElement:
			depth--
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
			if depth == 0 {
				return nil
			}
		}
	}
}

// readGeneral extrait l'identité du déclarant et la qualité au titre de
// laquelle il déclare. Les champs d'adresse et de contact sont traversés sans
// être lus : la Haute Autorité ne les publie pas, et il n'y a aucune raison
// d'en garder la trace.
func readGeneral(dec *xml.Decoder, d *declaration) error {
	depth := 1
	var path []string
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			path = append(path, t.Name.Local)
		case xml.CharData:
			v := strings.TrimSpace(string(t))
			if v == "" || v == notPublished || len(path) == 0 {
				break
			}
			parent := ""
			if len(path) >= 2 {
				parent = path[len(path)-2]
			}
			switch path[len(path)-1] {
			case "id":
				if parent == "typeDeclaration" {
					d.TypeDeclaration = v
				}
			case "nom":
				if parent == "declarant" {
					d.LastName = v
				}
			case "prenom":
				if parent == "declarant" {
					d.FirstName = v
				}
			case "dateNaissance":
				if parent == "declarant" {
					d.BirthDate = v
				}
			case "codTypeMandatFichier":
				d.MandateType = v
			case "labelOrgane":
				if parent == "organe" && d.BodyLabel == "" {
					d.BodyLabel = v
				}
			case "qualiteDeclarant":
				d.Role = v
			case "dateDebutMandat":
				d.MandateStart = v
			case "dateFinMandat":
				d.MandateEnd = v
			}
		case xml.EndElement:
			depth--
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
			if depth == 0 {
				return nil
			}
		}
	}
}

// readBlock aplatit un bloc de déclaration. Chaque <items> imbriqué devient
// une ligne ; les montants publiés par année en produisent une par année, ce
// qui permet de suivre une rémunération dans le temps sans avoir à ouvrir le
// XML.
func readBlock(dec *xml.Decoder, block string) ([]item, error) {
	depth := 1
	var path []string
	var out []item
	cur := item{Block: block}
	var year *int
	opened := false

	push := func() {
		if cur.Description != "" || cur.Employer != "" || cur.Amount != nil ||
			cur.Comment != "" || cur.NotPublished {
			cur.Rank = len(out)
			out = append(out, cur)
		}
		cur = item{Block: block}
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			path = append(path, t.Name.Local)
			if t.Name.Local == "items" {
				if opened {
					push()
				}
				opened = true
			}
			// Surtout PAS de remise à zéro de l'année ici. Le format imbrique
			// <montant><montant><annee>…</annee><montant>…</montant></montant>,
			// et remettre l'année à zéro à l'ouverture de chaque <montant>
			// l'effaçait juste avant de lire la valeur qu'elle datait : toutes
			// les séries de rémunération sortaient vides.
		case xml.CharData:
			v := strings.TrimSpace(string(t))
			if v == "" || len(path) == 0 {
				break
			}
			if v == notPublished {
				cur.NotPublished = true
				break
			}
			switch path[len(path)-1] {
			case "description", "descriptionMandat", "nomStructure", "denomination",
				"libelle", "nature", "natureBien", "typeCompte", "regimeJuridique",
				"origine", "titulaire", "souscripteur":
				if cur.Description == "" {
					cur.Description = truncate(v)
				}
			case "employeur", "nomEmployeur", "societe", "etablissement":
				if cur.Employer == "" {
					cur.Employer = truncate(v)
				}
			case "commentaire", "observation":
				if cur.Comment == "" {
					cur.Comment = truncate(v)
				}
			case "annee":
				if n, err := strconv.Atoi(v); err == nil {
					yr := n
					year = &yr
				}
			case "montant", "valeur", "montantTotal", "valeurVenale",
				"prixAcquisition", "valeurRachat", "montantRemuneration":
				if m, ok := parseAmount(v); ok {
					// Un montant par année produit sa propre ligne, pour que la
					// série soit lisible sans réouvrir le fichier.
					if year != nil {
						l := cur
						l.Year, l.Amount = year, &m
						l.Rank = len(out)
						out = append(out, l)
						year = nil
					} else if cur.Amount == nil {
						cur.Amount = &m
					}
				}
			}
		case xml.EndElement:
			depth--
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
			if depth == 0 {
				if opened {
					push()
				}
				return out, nil
			}
		}
	}
}

// parseAmount lit « 71 105 », « 71 105,50 » ou « 71105.50 ». Les espaces sont
// des séparateurs de milliers, y compris les espaces insécables.
func parseAmount(s string) (float64, bool) {
	s = strings.NewReplacer(" ", "", " ", "", " ", "", "€", "").Replace(s)
	s = strings.Replace(s, ",", ".", 1)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func dateFR(s string) any {
	t, err := time.Parse("02/01/2006", strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return t
}

func timestampFR(s string) any {
	s = strings.TrimSpace(s)
	for _, f := range []string{"02/01/2006 15:04:05", "02/01/2006"} {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return nil
}

func nilIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return truncate(s)
}

// truncate borne les textes libres : certains commentaires font plusieurs
// milliers de caractères et n'apportent rien au-delà.
//
// La coupe se fait en RUNES et non en octets. Couper à 500 octets tranchait au
// milieu d'un caractère accentué et produisait une séquence UTF-8 invalide,
// que PostgreSQL refusait — le chargement s'arrêtait sur « invalid byte
// sequence for encoding UTF8 » sans qu'on voie le rapport avec une troncature.
// ToValidUTF8 traite le cas symétrique : des octets déjà invalides dans le
// fichier source.
func truncate(s string) string {
	s = strings.ToValidUTF8(strings.Join(strings.Fields(s), " "), "")
	r := []rune(s)
	if len(r) > 500 {
		return string(r[:500])
	}
	return s
}
