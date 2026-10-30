package presidentielle

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les parrainages publiés par le Conseil constitutionnel (migration 0207).
var SourceParrainages = archive.Source{
	Slug: "cc-parrainages-pdr", Label: "Conseil constitutionnel — parrainages des candidats à l'élection présidentielle",
	Publisher: "Conseil constitutionnel (via data.gouv.fr)", Tier: "PRIMARY_OFFICIAL",
	License:     "Licence Ouverte v2.0",
	ReuseClass:  "OPEN",
	Attribution: "Source : Conseil constitutionnel, parrainages des candidats à l'élection présidentielle",
	Cadence:     "par scrutin ; plusieurs publications par semaine à partir de février, jusqu'à la liste des candidats",
	Notes: "Jeu CUMULATIF : chaque publication contient tous les parrainages connus à sa date, " +
		"et remplace la précédente — seule la dernière est chargée. Agrégé par candidat, catégorie " +
		"de mandat, département et date de publication (migration 0207) ; seuls les maires sont de plus " +
		"nommés, nom, commune et candidat (migration 0208), les autres élus restent agrégés. Un " +
		"parrainage retiré par son auteur disparaît du jeu suivant et n'apparaît donc plus ici.",
}

// Un jeu data.gouv.fr par scrutin. Le nom suit la même règle d'un scrutin à
// l'autre pour 2022 et 2027 ; 2017 a le sien (publié sous un autre intitulé).
var scrutinsParrainages = []struct {
	Election int
	Dataset  string
}{
	{2017, "parrainages-des-candidats-a-lelection-presidentielle-francaise-de-2017"},
	{2022, "parrainages-des-candidats-a-lelection-presidentielle-francaise-de-2022"},
	{2027, "parrainages-des-candidats-a-lelection-presidentielle-francaise-de-2027"},
}

const apiJeu = "https://www.data.gouv.fr/api/1/datasets/"

var reHorodatage = regexp.MustCompile(`/(\d{8})-(\d{6})/`)

type ressourceJeu struct {
	Title  string `json:"title"`
	Format string `json:"format"`
	URL    string `json:"url"`
}

// derniereCSV : le fichier CSV le plus récent du jeu. L'adresse d'un fichier
// porte l'horodatage de sa publication (/20220307-183308/), seule date fiable :
// les intitulés (« au 7 mars », « au 07 mars », « derniers parrainages connus »)
// ne le sont pas, et l'API ne les range pas dans l'ordre.
func derniereCSV(ressources []ressourceJeu) (ressourceJeu, bool) {
	var best ressourceJeu
	var bestStamp string
	for _, r := range ressources {
		if !strings.EqualFold(r.Format, "csv") {
			continue
		}
		m := reHorodatage.FindStringSubmatch(r.URL)
		if m == nil {
			continue
		}
		if stamp := m[1] + m[2]; stamp > bestStamp {
			best, bestStamp = r, stamp
		}
	}
	return best, bestStamp != ""
}

// categorieMandat range les intitulés de mandat du fichier — qui varient d'un
// scrutin à l'autre et selon le genre (« Conseillère départementale »,
// « Conseiller/ère départemental-e ») — dans les catégories de la migration 0207.
func categorieMandat(mandat string) string {
	m := strings.ToLower(strings.TrimSpace(mandat))
	switch {
	case strings.Contains(m, "délégu"):
		return "MAIRE_DELEGUE"
	case strings.Contains(m, "arrondissement"):
		return "MAIRE_ARRONDISSEMENT"
	case strings.HasPrefix(m, "maire"):
		return "MAIRE"
	case strings.Contains(m, "départemental"):
		return "CONSEILLER_DEPARTEMENTAL"
	case strings.Contains(m, "régional"):
		return "CONSEILLER_REGIONAL"
	case strings.Contains(m, "parlement européen"):
		return "EURODEPUTE"
	case strings.HasPrefix(m, "député"):
		return "DEPUTE"
	case strings.HasPrefix(m, "sénat"):
		return "SENATEUR"
	case strings.HasPrefix(m, "président") && (strings.Contains(m, "epci") || strings.Contains(m, "communauté") || strings.Contains(m, "métropole")):
		return "PRESIDENT_INTERCO"
	}
	return "AUTRE"
}

// colonnes repère les colonnes du CSV par leur en-tête et exige celles qu'on lui
// nomme : le fichier a changé de forme d'un scrutin à l'autre, un rang figé
// lirait la mauvaise colonne sans rien dire.
func colonnes(head []string, requises ...string) (map[string]int, error) {
	col := map[string]int{"mandat": -1, "departement": -1, "candidat": -1, "date": -1,
		"nom": -1, "prenom": -1, "commune": -1}
	for i, h := range head {
		h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, string(rune(0xFEFF)))))
		switch {
		case h == "mandat":
			col["mandat"] = i
		case h == "département" || h == "departement":
			col["departement"] = i
		case strings.HasPrefix(h, "candidat"):
			col["candidat"] = i
		case strings.HasPrefix(h, "date de publication"):
			col["date"] = i
		case h == "nom":
			col["nom"] = i
		case h == "prénom" || h == "prenom":
			col["prenom"] = i
		case h == "circonscription":
			col["commune"] = i
		}
	}
	for _, k := range requises {
		if col[k] < 0 {
			return nil, fmt.Errorf("colonne « %s » absente de l'en-tête %q — le format du fichier a changé", k, head)
		}
	}
	return col, nil
}

// maire : un parrainage donné par un maire, nommé, tel que le Conseil le publie.
type maire struct {
	nom, prenom, commune, departement, candidat string
	date                                        time.Time
}

// parseMaires lit dans le même CSV les seuls parrainages de maires (la catégorie
// MAIRE : ni maires délégués ni maires d'arrondissement, dont la
// « circonscription » n'est pas une commune). Le fait brut, rien d'autre : le
// site n'en tire aucune étiquette politique (migration 0208). Un même maire qui
// reparaît sur plusieurs dates pour le même candidat garde la plus ancienne.
func parseMaires(r io.Reader) ([]maire, error) {
	cr := csv.NewReader(r)
	cr.Comma = ';'
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("en-tête : %w", err)
	}
	col, err := colonnes(head, "mandat", "departement", "candidat", "date", "nom", "prenom", "commune")
	if err != nil {
		return nil, err
	}
	vus := map[[5]string]int{}
	var out []maire
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) <= col["date"] || categorieMandat(rec[col["mandat"]]) != "MAIRE" {
			continue
		}
		d, err := time.Parse("02/01/2006", strings.TrimSpace(rec[col["date"]]))
		if err != nil {
			return nil, fmt.Errorf("date de publication illisible %q", rec[col["date"]])
		}
		m := maire{
			nom: strings.TrimSpace(rec[col["nom"]]), prenom: strings.TrimSpace(rec[col["prenom"]]),
			commune: strings.TrimSpace(rec[col["commune"]]), departement: strings.TrimSpace(rec[col["departement"]]),
			candidat: strings.TrimSpace(rec[col["candidat"]]), date: d,
		}
		if m.nom == "" || m.commune == "" || m.candidat == "" {
			return nil, fmt.Errorf("ligne de maire incomplète (%v)", rec)
		}
		k := [5]string{m.nom, m.prenom, m.commune, m.departement, m.candidat}
		if i, ok := vus[k]; ok {
			if m.date.Before(out[i].date) {
				out[i].date = m.date
			}
			continue
		}
		vus[k] = len(out)
		out = append(out, m)
	}
	return out, nil
}

// parseParrainages lit le CSV du Conseil. Les colonnes sont repérées par leur
// en-tête, jamais par leur rang : la colonne du candidat s'appelle « Candidat »
// en 2022 et « Candidat-e parrainé-e » en 2017.
func parseParrainages(r io.Reader) (map[clef]int, int, error) {
	cr := csv.NewReader(r)
	cr.Comma = ';'
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("en-tête : %w", err)
	}
	col, err := colonnes(head, "mandat", "departement", "candidat", "date")
	if err != nil {
		return nil, 0, err
	}
	agg := map[clef]int{}
	lignes := 0
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		if len(rec) <= col["date"] {
			continue
		}
		d, err := time.Parse("02/01/2006", strings.TrimSpace(rec[col["date"]]))
		if err != nil {
			return nil, 0, fmt.Errorf("date de publication illisible %q", rec[col["date"]])
		}
		cand := strings.TrimSpace(rec[col["candidat"]])
		if cand == "" {
			return nil, 0, fmt.Errorf("ligne sans candidat (%v)", rec)
		}
		agg[clef{cand, categorieMandat(rec[col["mandat"]]), strings.TrimSpace(rec[col["departement"]]), d.Format("2006-01-02")}]++
		lignes++
	}
	if lignes == 0 {
		return nil, 0, fmt.Errorf("aucun parrainage dans le fichier")
	}
	return agg, lignes, nil
}

type clef struct{ candidat, categorie, departement, date string }

func IngestParrainages(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceParrainages)
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

	stats := map[string]any{}
	for _, s := range scrutinsParrainages {
		meta, err := arch.Fetch(ctx, srcID, runID, apiJeu+s.Dataset+"/", ".json")
		if err != nil {
			// Un scrutin dont le jeu n'est pas encore publié (2027 jusqu'en
			// février) n'est pas une erreur : le jeu n'existe simplement pas.
			if strings.Contains(err.Error(), fmt.Sprint(http.StatusNotFound)) {
				stats[fmt.Sprint(s.Election)] = "pas encore publié"
				fmt.Printf("  parrainages %d : jeu pas encore publié par le Conseil constitutionnel\n", s.Election)
				continue
			}
			return fail(fmt.Errorf("%d : %w", s.Election, err))
		}
		b, err := os.ReadFile(meta.Path)
		if err != nil {
			return fail(err)
		}
		var jeu struct {
			Resources []ressourceJeu `json:"resources"`
		}
		if err := json.Unmarshal(b, &jeu); err != nil {
			return fail(fmt.Errorf("%d : %w", s.Election, err))
		}
		res, ok := derniereCSV(jeu.Resources)
		if !ok {
			return fail(fmt.Errorf("%d : aucun fichier CSV daté dans le jeu %s", s.Election, s.Dataset))
		}
		f, err := arch.Fetch(ctx, srcID, runID, res.URL, ".csv")
		if err != nil {
			return fail(fmt.Errorf("%d : %w", s.Election, err))
		}
		fh, err := os.Open(f.Path)
		if err != nil {
			return fail(err)
		}
		agg, lignes, err := parseParrainages(fh)
		fh.Close()
		if err != nil {
			return fail(fmt.Errorf("%d (%s) : %w", s.Election, res.URL, err))
		}

		rows := make([][]any, 0, len(agg))
		keys := make([]clef, 0, len(agg))
		for k := range agg {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			a, b := keys[i], keys[j]
			return a.candidat+a.categorie+a.departement+a.date < b.candidat+b.categorie+b.departement+b.date
		})
		for _, k := range keys {
			d, _ := time.Parse("2006-01-02", k.date)
			rows = append(rows, []any{s.Election, k.candidat, k.categorie, k.departement, d, agg[k], srcID, f.DocumentID})
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fail(err)
		}
		// Table temporaire puis MERGE plutôt que DELETE+COPY : le jeu du Conseil
		// est cumulatif, la fusion ne touche que ce qui a changé, et le scope
		// sur l'élection empêche d'effacer les autres scrutins.
		if _, err := tx.Exec(ctx, `CREATE TEMP TABLE tmp_parrainage (LIKE core.parrainage_presidentielle) ON COMMIT DROP`); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_parrainage"},
			[]string{"election", "candidat", "categorie", "departement", "date_publication", "nombre", "source_id", "document_id"},
			pgx.CopyFromRows(rows)); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}
		if _, err := tx.Exec(ctx, `
			MERGE INTO core.parrainage_presidentielle AS tgt
			USING tmp_parrainage AS src
			ON tgt.election = src.election AND tgt.candidat = src.candidat AND tgt.categorie = src.categorie
			   AND tgt.departement = src.departement AND tgt.date_publication = src.date_publication
			WHEN MATCHED AND (tgt.nombre, tgt.source_id, tgt.document_id)
			                  IS DISTINCT FROM (src.nombre, src.source_id, src.document_id) THEN
			    UPDATE SET nombre = src.nombre, source_id = src.source_id, document_id = src.document_id
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (election, candidat, categorie, departement, date_publication, nombre, source_id, document_id)
			    VALUES (src.election, src.candidat, src.categorie, src.departement, src.date_publication,
			            src.nombre, src.source_id, src.document_id)
			WHEN NOT MATCHED BY SOURCE AND tgt.election = $1 THEN DELETE`, s.Election); err != nil {
			tx.Rollback(ctx)
			return fail(fmt.Errorf("fusion : %w", err))
		}
		if err := tx.Commit(ctx); err != nil {
			return fail(err)
		}
		nMaires, err := chargerMaires(ctx, pool, f.Path, s.Election, srcID, f.DocumentID)
		if err != nil {
			return fail(fmt.Errorf("%d : maires : %w", s.Election, err))
		}
		stats[fmt.Sprint(s.Election)] = map[string]any{"parrainages": lignes, "lignes": len(rows), "maires": nMaires, "fichier": res.Title}
		fmt.Printf("  parrainages %d : %d parrainages, %d lignes agrégées, %d maires nommés (%s)\n", s.Election, lignes, len(rows), nMaires, res.Title)
	}
	arch.EndRun(ctx, runID, "SUCCESS", stats, "")
	return nil
}

// chargerMaires remplace, pour un scrutin, les maires nommés par ceux du
// dernier jeu du Conseil. Même principe que l'agrégat : le jeu est cumulatif,
// la fusion ne touche que ce qui a changé, et un parrainage retiré par son
// auteur disparaît du jeu suivant — et donc d'ici.
func chargerMaires(ctx context.Context, pool *pgxpool.Pool, path string, election int, srcID, docID int64) (int, error) {
	fh, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	maires, err := parseMaires(fh)
	fh.Close()
	if err != nil {
		return 0, err
	}
	rows := make([][]any, 0, len(maires))
	for _, m := range maires {
		rows = append(rows, []any{election, m.nom, m.prenom, m.commune, m.departement, m.candidat, m.date, srcID, docID})
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	// Les colonnes générées (clés de comparaison) ne se copient pas : la table
	// temporaire n'a que les colonnes saisies.
	cols := []string{"election", "nom", "prenom", "commune", "departement", "candidat", "date_publication", "source_id", "document_id"}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE tmp_parrainage_maire (
		election smallint, nom text, prenom text, commune text, departement text, candidat text,
		date_publication date, source_id bigint, document_id bigint) ON COMMIT DROP`); err != nil {
		tx.Rollback(ctx)
		return 0, err
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_parrainage_maire"}, cols, pgx.CopyFromRows(rows)); err != nil {
		tx.Rollback(ctx)
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		MERGE INTO core.parrainage_maire AS tgt
		USING tmp_parrainage_maire AS src
		ON tgt.election = src.election AND tgt.nom = src.nom AND tgt.prenom = src.prenom
		   AND tgt.commune = src.commune AND tgt.departement = src.departement AND tgt.candidat = src.candidat
		WHEN MATCHED AND (tgt.date_publication, tgt.source_id, tgt.document_id)
		                  IS DISTINCT FROM (src.date_publication, src.source_id, src.document_id) THEN
		    UPDATE SET date_publication = src.date_publication, source_id = src.source_id, document_id = src.document_id
		WHEN NOT MATCHED BY TARGET THEN
		    INSERT (election, nom, prenom, commune, departement, candidat, date_publication, source_id, document_id)
		    VALUES (src.election, src.nom, src.prenom, src.commune, src.departement, src.candidat,
		            src.date_publication, src.source_id, src.document_id)
		WHEN NOT MATCHED BY SOURCE AND tgt.election = $1 THEN DELETE`, election); err != nil {
		tx.Rollback(ctx)
		return 0, fmt.Errorf("fusion : %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(maires), nil
}
