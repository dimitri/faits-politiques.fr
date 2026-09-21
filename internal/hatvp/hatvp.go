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
	nonPublie       = "[Données non publiées]"
)

// Les blocs retenus, avec ce qu'on en extrait. Tout le reste du format est
// ignoré : ce sont des champs d'adresse, de pièce d'identité et de contact,
// c'est-à-dire précisément ce que la Haute Autorité ne publie pas.
var blocsRetenus = map[string]bool{
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
	Bloc        string
	Description string
	Employeur   string
	Commentaire string
	Annee       *int
	Montant     *float64
	NonPublie   bool
}

type declaration struct {
	UUID, Nom, Prenom       string
	Naissance               string
	TypeDeclaration         string
	DateDepot               string
	TypeMandat, LabelOrgane string
	Qualite                 string
	DebutMandat, FinMandat  string
	Items                   []item
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

	// Lu en flux (parcourir ne garde jamais plus d'une <declaration> en
	// mémoire à la fois — le XML fait 87 Mo, profondément imbriqué), mais
	// ACCUMULÉ ici plutôt qu'écrit ligne à ligne : les déclarations et
	// leurs items décodés sont de petites structures (quelques champs
	// texte/nombre), même par dizaines de milliers ça reste de l'ordre de
	// la centaine de Mo — sans commune mesure avec les milliers d'allers-
	// retours qu'une INSERT par déclaration (et par item) coûtait avant.
	// Un doublon d'uuid dans le flux garde la PREMIÈRE occurrence, comme
	// avant (ON CONFLICT DO NOTHING y suffisait ligne à ligne) : dédupliqué
	// ici en Go, exactement la même règle.
	vus := map[string]bool{}
	var decls []declaration
	if err := parcourir(f.Path, func(d declaration) error {
		if vus[d.UUID] {
			return nil
		}
		vus[d.UUID] = true
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

	// Reconstruction complète : une déclaration retirée par la Haute Autorité
	// doit disparaître d'ici aussi.
	if _, err := tx.Exec(ctx, `DELETE FROM core.declaration`); err != nil {
		return fail(err)
	}

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
		declRows[i] = []any{d.UUID, d.Nom, d.Prenom, dateFR(d.Naissance), d.TypeDeclaration,
			horodatageFR(d.DateDepot), nul(d.TypeMandat), nul(d.LabelOrgane),
			nul(d.Qualite), dateFR(d.DebutMandat), dateFR(d.FinMandat)}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_declaration"},
		[]string{"uuid", "nom", "prenom", "date_naissance", "type_declaration", "date_depot",
			"type_mandat", "label_organe", "qualite", "date_debut_mandat", "date_fin_mandat"},
		pgx.CopyFromRows(declRows)); err != nil {
		return fail(err)
	}
	res, err := tx.Query(ctx, `
		WITH upsert AS (
			INSERT INTO core.declaration
			  (uuid, nom, prenom, date_naissance, type_declaration, date_depot,
			   type_mandat, label_organe, qualite, date_debut_mandat, date_fin_mandat, source_id)
			SELECT uuid, nom, prenom, date_naissance, type_declaration, date_depot,
			       type_mandat, label_organe, qualite, date_debut_mandat, date_fin_mandat, $1
			  FROM tmp_declaration
			ON CONFLICT (uuid) DO NOTHING
			RETURNING id, uuid
		)
		SELECT uuid, id FROM upsert`, srcID)
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

	// Les items n'ont aucun conflit à résoudre (aucune contrainte d'unicité
	// dessus) : une COPY directe dans la vraie table suffit, comme
	// core.ballot ailleurs dans ce dépôt — pas la peine d'une table
	// temporaire pour un aller simple.
	var itemRows [][]any
	for _, d := range decls {
		declID, ok := declIDByUUID[d.UUID]
		if !ok {
			continue // doublon d'uuid déjà écarté au flux, ou conflit DB
		}
		for _, it := range d.Items {
			itemRows = append(itemRows, []any{declID, it.Bloc, nul(it.Description),
				nul(it.Employeur), nul(it.Commentaire), it.Annee, it.Montant, it.NonPublie})
		}
	}
	nItemsAffected, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "declaration_item"},
		[]string{"declaration_id", "bloc", "description", "employeur", "commentaire",
			"annee", "montant", "non_publie"},
		pgx.CopyFromRows(itemRows))
	if err != nil {
		return fail(fmt.Errorf("items : %w", err))
	}
	nItems := int(nItemsAffected)

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
		"declarations": nDecl, "items": nItems, "rapprochees": rec.RowsAffected()}, "")
	logs.Notice("HATVP normalisé", "declarations", nDecl, "items", nItems,
		"rapprochees", rec.RowsAffected())
	return nil
}

// parcourir lit le flux de déclarations sans jamais en garder plus d'une en
// mémoire : le fichier fait 87 Mo et la structure est profondément imbriquée.
func parcourir(path string, fn func(declaration) error) error {
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
		if err := lireDeclaration(dec, &d); err != nil {
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

// lireDeclaration consomme une <declaration> jusqu'à sa fermeture.
func lireDeclaration(dec *xml.Decoder, d *declaration) error {
	profondeur := 1
	var chemin []string
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			profondeur++
			chemin = append(chemin, t.Name.Local)
			if blocsRetenus[t.Name.Local] {
				items, err := lireBloc(dec, t.Name.Local)
				if err != nil {
					return err
				}
				d.Items = append(d.Items, items...)
				profondeur--
				chemin = chemin[:len(chemin)-1]
				continue
			}
			if t.Name.Local == "general" {
				if err := lireGeneral(dec, d); err != nil {
					return err
				}
				profondeur--
				chemin = chemin[:len(chemin)-1]
				continue
			}
		case xml.CharData:
			v := strings.TrimSpace(string(t))
			if v == "" || len(chemin) == 0 {
				break
			}
			switch chemin[len(chemin)-1] {
			case "uuid":
				if len(chemin) == 1 {
					d.UUID = v
				}
			case "dateDepot":
				if len(chemin) == 1 {
					d.DateDepot = v
				}
			}
		case xml.EndElement:
			profondeur--
			if len(chemin) > 0 {
				chemin = chemin[:len(chemin)-1]
			}
			if profondeur == 0 {
				return nil
			}
		}
	}
}

// lireGeneral extrait l'identité du déclarant et la qualité au titre de
// laquelle il déclare. Les champs d'adresse et de contact sont traversés sans
// être lus : la Haute Autorité ne les publie pas, et il n'y a aucune raison
// d'en garder la trace.
func lireGeneral(dec *xml.Decoder, d *declaration) error {
	profondeur := 1
	var chemin []string
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			profondeur++
			chemin = append(chemin, t.Name.Local)
		case xml.CharData:
			v := strings.TrimSpace(string(t))
			if v == "" || v == nonPublie || len(chemin) == 0 {
				break
			}
			parent := ""
			if len(chemin) >= 2 {
				parent = chemin[len(chemin)-2]
			}
			switch chemin[len(chemin)-1] {
			case "id":
				if parent == "typeDeclaration" {
					d.TypeDeclaration = v
				}
			case "nom":
				if parent == "declarant" {
					d.Nom = v
				}
			case "prenom":
				if parent == "declarant" {
					d.Prenom = v
				}
			case "dateNaissance":
				if parent == "declarant" {
					d.Naissance = v
				}
			case "codTypeMandatFichier":
				d.TypeMandat = v
			case "labelOrgane":
				if parent == "organe" && d.LabelOrgane == "" {
					d.LabelOrgane = v
				}
			case "qualiteDeclarant":
				d.Qualite = v
			case "dateDebutMandat":
				d.DebutMandat = v
			case "dateFinMandat":
				d.FinMandat = v
			}
		case xml.EndElement:
			profondeur--
			if len(chemin) > 0 {
				chemin = chemin[:len(chemin)-1]
			}
			if profondeur == 0 {
				return nil
			}
		}
	}
}

// lireBloc aplatit un bloc de déclaration. Chaque <items> imbriqué devient une
// ligne ; les montants publiés par année en produisent une par année, ce qui
// permet de suivre une rémunération dans le temps sans avoir à ouvrir le XML.
func lireBloc(dec *xml.Decoder, bloc string) ([]item, error) {
	profondeur := 1
	var chemin []string
	var out []item
	cur := item{Bloc: bloc}
	var annee *int
	ouvert := false

	pousser := func() {
		if cur.Description != "" || cur.Employeur != "" || cur.Montant != nil ||
			cur.Commentaire != "" || cur.NonPublie {
			out = append(out, cur)
		}
		cur = item{Bloc: bloc}
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			profondeur++
			chemin = append(chemin, t.Name.Local)
			if t.Name.Local == "items" {
				if ouvert {
					pousser()
				}
				ouvert = true
			}
			// Surtout PAS de remise à zéro de l'année ici. Le format imbrique
			// <montant><montant><annee>…</annee><montant>…</montant></montant>,
			// et remettre l'année à zéro à l'ouverture de chaque <montant>
			// l'effaçait juste avant de lire la valeur qu'elle datait : toutes
			// les séries de rémunération sortaient vides.
		case xml.CharData:
			v := strings.TrimSpace(string(t))
			if v == "" || len(chemin) == 0 {
				break
			}
			if v == nonPublie {
				cur.NonPublie = true
				break
			}
			switch chemin[len(chemin)-1] {
			case "description", "descriptionMandat", "nomStructure", "denomination",
				"libelle", "nature", "natureBien", "typeCompte", "regimeJuridique",
				"origine", "titulaire", "souscripteur":
				if cur.Description == "" {
					cur.Description = tronquer(v)
				}
			case "employeur", "nomEmployeur", "societe", "etablissement":
				if cur.Employeur == "" {
					cur.Employeur = tronquer(v)
				}
			case "commentaire", "observation":
				if cur.Commentaire == "" {
					cur.Commentaire = tronquer(v)
				}
			case "annee":
				if n, err := strconv.Atoi(v); err == nil {
					a := n
					annee = &a
				}
			case "montant", "valeur", "montantTotal", "valeurVenale",
				"prixAcquisition", "valeurRachat", "montantRemuneration":
				if m, ok := montant(v); ok {
					// Un montant par année produit sa propre ligne, pour que la
					// série soit lisible sans réouvrir le fichier.
					if annee != nil {
						l := cur
						l.Annee, l.Montant = annee, &m
						out = append(out, l)
						annee = nil
					} else if cur.Montant == nil {
						cur.Montant = &m
					}
				}
			}
		case xml.EndElement:
			profondeur--
			if len(chemin) > 0 {
				chemin = chemin[:len(chemin)-1]
			}
			if profondeur == 0 {
				if ouvert {
					pousser()
				}
				return out, nil
			}
		}
	}
}

// montant lit « 71 105 », « 71 105,50 » ou « 71105.50 ». Les espaces sont des
// séparateurs de milliers, y compris les espaces insécables.
func montant(s string) (float64, bool) {
	s = strings.NewReplacer(" ", "", " ", "", " ", "", "€", "").Replace(s)
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

func horodatageFR(s string) any {
	s = strings.TrimSpace(s)
	for _, f := range []string{"02/01/2006 15:04:05", "02/01/2006"} {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return nil
}

func nul(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return tronquer(s)
}

// tronquer borne les textes libres : certains commentaires font plusieurs
// milliers de caractères et n'apportent rien au-delà.
//
// La coupe se fait en RUNES et non en octets. Couper à 500 octets tranchait au
// milieu d'un caractère accentué et produisait une séquence UTF-8 invalide,
// que PostgreSQL refusait — le chargement s'arrêtait sur « invalid byte
// sequence for encoding UTF8 » sans qu'on voie le rapport avec une troncature.
// ToValidUTF8 traite le cas symétrique : des octets déjà invalides dans le
// fichier source.
func tronquer(s string) string {
	s = strings.ToValidUTF8(strings.Join(strings.Fields(s), " "), "")
	r := []rune(s)
	if len(r) > 500 {
		return string(r[:500])
	}
	return s
}
