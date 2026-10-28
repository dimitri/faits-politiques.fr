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
		"de mandat, département et date de publication : les élus ne sont pas nommés (voir la migration " +
		"0207). Un parrainage retiré par son auteur disparaît du jeu suivant et n'apparaît donc plus ici.",
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
	col := map[string]int{"mandat": -1, "departement": -1, "candidat": -1, "date": -1}
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
		}
	}
	for k, v := range col {
		if v < 0 {
			return nil, 0, fmt.Errorf("colonne « %s » absente de l'en-tête %q — le format du fichier a changé", k, head)
		}
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
		if _, err := tx.Exec(ctx, `DELETE FROM core.parrainage_presidentielle WHERE election = $1`, s.Election); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "parrainage_presidentielle"},
			[]string{"election", "candidat", "categorie", "departement", "date_publication", "nombre", "source_id", "document_id"},
			pgx.CopyFromRows(rows)); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fail(err)
		}
		stats[fmt.Sprint(s.Election)] = map[string]any{"parrainages": lignes, "lignes": len(rows), "fichier": res.Title}
		fmt.Printf("  parrainages %d : %d parrainages, %d lignes agrégées (%s)\n", s.Election, lignes, len(rows), res.Title)
	}
	arch.EndRun(ctx, runID, "SUCCESS", stats, "")
	return nil
}
