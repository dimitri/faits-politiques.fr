package macro

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/encoding/charmap"
)

var SourceSRU = archive.Source{
	Slug: "sru-inventaire-communes", Label: "Inventaire SRU par commune",
	Publisher: "DGALN/DHUP (ministère du Logement)", Tier: "PRIMARY_OFFICIAL",
	License: "Licence Ouverte", ReuseClass: "OPEN",
	Attribution: "Source : DGALN/DHUP, data.gouv.fr",
	Cadence:     "annuelle",
	Notes: "Ne couvre que les communes dans le périmètre de l'article 55 de la loi SRU " +
		"(entre ~2 150 et ~2 210 communes selon le millésime), pas l'ensemble des communes " +
		"françaises. Quatre millésimes chargés (2023 à 2026), vérifiés fichier par fichier : " +
		"le délimiteur change (virgule en 2023-2024, point-virgule en 2025-2026), le nom des " +
		"colonnes change d'une année sur l'autre (parfois avec ou sans accent DANS un même " +
		"fichier), et le prélèvement SRU net (majoration de carence comprise) n'existe comme " +
		"colonne qu'à partir du millésime 2024.",
}

// sruYearConfig décrit un millésime : son adresse, son délimiteur réel et le nom
// EXACT de chaque colonne dans CE fichier. Les noms ne suivent aucun motif
// commun d'un millésime à l'autre (accents tantôt présents, tantôt non ;
// année tantôt dans le nom, tantôt absente) — vérifiés directement sur
// chaque fichier téléchargé plutôt que supposés identiques au millésime 2025
// déjà chargé.
type sruYearConfig struct {
	Year          int
	URL           string
	Delimiter     rune
	SkipLines     int // le fichier 2024 commence par une ligne entièrement vide avant l'en-tête
	DeptCol       string
	DeptIsCode    bool // 2023/2024 : DeptCol est un CODE de département (résolu via geo.contour) ; 2025/2026 : c'est déjà le nom
	PopulationCol string
	RateCol       string
	TargetCol     string
	DeficientCol  string
	SanctionedCol string
	ExemptCol     string
	LevyCol       string // "" : colonne absente de ce millésime (2023)
}

var sruYears = []sruYearConfig{
	{
		Year: 2023, Delimiter: ',', SkipLines: 0,
		URL:     "https://static.data.gouv.fr/resources/communes-et-inventaire-sru/20250922-104633/donnees-sru-data-gouv-maj2023-vf.csv",
		DeptCol: "Code_Departement", DeptIsCode: true,
		PopulationCol: "Population_municipale_01_01_2023",
		RateCol:       "Taux_SRU_au_01_01_2022",
		TargetCol:     "Taux_cible_commune_2023_2025",
		DeficientCol:  "Commune_deficitaire_2023",
		SanctionedCol: "Commune_carencee_2023_2025",
		ExemptCol:     "Commune_exemptee_2023_2025",
	},
	{
		Year: 2024, Delimiter: ',', SkipLines: 1,
		URL:     "https://static.data.gouv.fr/resources/communes-et-inventaire-sru/20250929-132410/donnees-sru-data-gouv-maj2024-vf.csv",
		DeptCol: "Code_Département", DeptIsCode: true,
		PopulationCol: "Population_municipale_01_01_2024",
		RateCol:       "Taux_SRU_au_01_01_2023",
		TargetCol:     "Taux_cible_commune_2023_2025",
		DeficientCol:  "Commune_déficitaire_2024",
		SanctionedCol: "Commune_carencée_2023_2025",
		ExemptCol:     "Commune_exemptée_2023_2025",
		LevyCol:       "Prélèvement_net_2024_dont_majoration",
	},
	{
		Year: 2025, Delimiter: ';', SkipLines: 0,
		URL:     "https://static.data.gouv.fr/resources/communes-et-inventaire-sru/20251219-143258/donnees-sru-data-gouv-2025-v2.csv",
		DeptCol: "Departement", DeptIsCode: false,
		PopulationCol: "Population_municipale_01_01_2025",
		RateCol:       "Taux_SRU_au_01_01_2024",
		TargetCol:     "Taux_cible_commune_2023_2025",
		DeficientCol:  "commune_deficitaire",
		SanctionedCol: "Commune_carencée",
		ExemptCol:     "Commune_exemptée_2023_2025",
		LevyCol:       "Prélèvement_net_2025_dont_majoration",
	},
	{
		Year: 2026, Delimiter: ';', SkipLines: 0,
		URL:     "https://static.data.gouv.fr/resources/communes-et-inventaire-sru/20260811-123753/donnees-sru-data-gouv-2026.csv",
		DeptCol: "Departement", DeptIsCode: false,
		PopulationCol: "Population_municipale_01_01_2026",
		RateCol:       "Taux_SRU_au_01_01_2025",
		TargetCol:     "Taux_cible_commune",
		DeficientCol:  "commune_deficitaire_au_01-01-2025",
		SanctionedCol: "Commune_carencee_2023_2025",
		ExemptCol:     "Commune_exemptee_2026_2028",
		LevyCol:       "Prelevement_net_2026_dont_majoration",
	},
}

func parseFrenchPercent(s string) (float64, bool) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func boolFrom01(s string) bool { return strings.TrimSpace(s) == "1" }

// parseEuroAmount lit un montant en euros au format de la source
// (« 64 509,48 € », espace fine insécable en séparateur de milliers, virgule
// décimale, et « - € » ou « 0,00 € » pour un prélèvement nul — les deux
// formes valent 0, jamais NULL : la colonne existe et dit explicitement
// zéro, ce qui diffère du millésime 2023 où la colonne n'existe pas du tout.
func parseEuroAmount(s string) (float64, bool) {
	s = strings.ReplaceAll(s, "€", "")
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == ' ' || r == ' ' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" || s == "-" {
		return 0, true
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

type sruRow struct {
	year                                int
	codeInsee, municipality, department string
	population, socialHousingCount      *int
	sruRate, targetRate, levy           *float64
	deficient, sanctioned, exempt       bool
}

// loadDepartments : code -> nom, pour résoudre les millésimes 2023 et
// 2024 qui ne publient que le CODE du département (les millésimes 2025 et
// 2026 publient directement son nom, dans une colonne distincte).
func loadDepartments(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx, `SELECT code_insee, nom FROM geo.contour WHERE niveau='DEPARTEMENT'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byCode := map[string]string{}
	for rows.Next() {
		var code, name string
		if err := rows.Scan(&code, &name); err != nil {
			return nil, err
		}
		byCode[code] = name
	}
	return byCode, rows.Err()
}

// parseSRUYear télécharge et lit un millésime. Le fichier est encodé en
// Windows-1252 (et non le Latin-1 pur utilisé par internal/macro/accord_paris.go
// ailleurs sur ce site) : la distinction n'était pas visible tant que personne
// ne lisait le prélèvement, mais son signe euro (octet 0x80) n'a pas la même
// signification dans les deux jeux de caractères — une conversion Latin-1
// l'aurait laissé comme caractère de contrôle invisible plutôt que « € ».
func parseSRUYear(ctx context.Context, arch *archive.Archive, srcID, runID int64,
	cfg sruYearConfig, deptByCode map[string]string) ([]sruRow, error) {

	f, err := arch.Fetch(ctx, srcID, runID, cfg.URL, ".csv")
	if err != nil {
		return nil, fmt.Errorf("millésime %d : %w", cfg.Year, err)
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, err
	}
	text, err := charmap.Windows1252.NewDecoder().String(string(raw))
	if err != nil {
		return nil, fmt.Errorf("millésime %d : décodage Windows-1252 : %w", cfg.Year, err)
	}

	r := csv.NewReader(strings.NewReader(text))
	r.Comma = cfg.Delimiter
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	for i := 0; i < cfg.SkipLines; i++ {
		if _, err := r.Read(); err != nil {
			return nil, fmt.Errorf("millésime %d : ligne à sauter illisible : %w", cfg.Year, err)
		}
	}
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("millésime %d : en-tête illisible : %w", cfg.Year, err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(h)] = i
	}
	must := []string{"Code_INSEE_commune", "Nom_commune", cfg.DeptCol, cfg.PopulationCol,
		cfg.RateCol, cfg.TargetCol, cfg.DeficientCol, cfg.SanctionedCol, cfg.ExemptCol}
	if cfg.LevyCol != "" {
		must = append(must, cfg.LevyCol)
	}
	for _, m := range must {
		if _, ok := col[m]; !ok {
			return nil, fmt.Errorf("millésime %d : colonne %q absente (en-tête : %v)", cfg.Year, m, header)
		}
	}
	socialHousingCol := -1
	for h, i := range col {
		if strings.HasPrefix(h, "Nombre_lls") {
			socialHousingCol = i
		}
	}
	if socialHousingCol == -1 {
		return nil, fmt.Errorf("millésime %d : colonne du nombre de logements sociaux introuvable", cfg.Year)
	}

	var rows []sruRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("millésime %d : ligne illisible : %w", cfg.Year, err)
		}
		field := func(name string) string {
			i, ok := col[name]
			if !ok || i >= len(rec) {
				return ""
			}
			return rec[i]
		}
		row := sruRow{
			year:         cfg.Year,
			codeInsee:    strings.TrimSpace(field("Code_INSEE_commune")),
			municipality: strings.TrimSpace(field("Nom_commune")),
			deficient:    boolFrom01(field(cfg.DeficientCol)),
			sanctioned:   boolFrom01(field(cfg.SanctionedCol)),
			exempt:       boolFrom01(field(cfg.ExemptCol)),
		}
		if row.codeInsee == "" {
			continue
		}
		dept := strings.TrimSpace(field(cfg.DeptCol))
		if cfg.DeptIsCode {
			if name, ok := deptByCode[dept]; ok {
				row.department = name
			} else {
				row.department = dept
			}
		} else {
			row.department = dept
		}
		if v, err := strconv.Atoi(strings.TrimSpace(field(cfg.PopulationCol))); err == nil {
			row.population = &v
		}
		if v, err := strconv.Atoi(strings.TrimSpace(rec[socialHousingCol])); err == nil {
			row.socialHousingCount = &v
		}
		if v, ok := parseFrenchPercent(field(cfg.RateCol)); ok {
			row.sruRate = &v
		}
		if v, ok := parseFrenchPercent(field(cfg.TargetCol)); ok {
			row.targetRate = &v
		}
		if cfg.LevyCol != "" {
			if v, ok := parseEuroAmount(field(cfg.LevyCol)); ok {
				row.levy = &v
			}
		}
		rows = append(rows, row)
	}
	if len(rows) < 2000 {
		return nil, fmt.Errorf("millésime %d : seulement %d lignes lues, attendu au moins 2000", cfg.Year, len(rows))
	}
	return rows, nil
}

// IngestSRU charge l'inventaire SRU par commune, sur les quatre millésimes
// publiés par la source (2023 à 2026). Voir docs/logement-territoires-donnees.md.
func IngestSRU(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSRU)
	if err != nil {
		return err
	}
	runID, err := arch.StartRun(ctx, srcID, "sru-v2-pluriannuel")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
		return err
	}

	deptByCode, err := loadDepartments(ctx, pool)
	if err != nil {
		return fail(fmt.Errorf("départements (geo.contour) : %w", err))
	}
	if len(deptByCode) == 0 {
		return fail(fmt.Errorf("geo.contour ne contient aucun département : ingérer les contours avant SRU"))
	}

	stats := map[string]any{}
	for _, cfg := range sruYears {
		rows, err := parseSRUYear(ctx, arch, srcID, runID, cfg, deptByCode)
		if err != nil {
			return fail(err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fail(err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM core.sru_commune WHERE annee = $1`, cfg.Year); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}
		for _, row := range rows {
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.sru_commune
					(code_insee, annee, commune, departement, population, nombre_logements_sociaux,
					 taux_sru_pct, taux_cible_pct, deficitaire, carencee, exemptee, prelevement_net, source_id)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
				ON CONFLICT (code_insee, annee) DO NOTHING`,
				row.codeInsee, row.year, row.municipality, row.department, row.population, row.socialHousingCount,
				row.sruRate, row.targetRate, row.deficient, row.sanctioned, row.exempt, row.levy, srcID); err != nil {
				tx.Rollback(ctx)
				return fail(fmt.Errorf("millésime %d, %s : insertion : %w", cfg.Year, row.codeInsee, err))
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return fail(err)
		}
		stats[fmt.Sprintf("communes_%d", cfg.Year)] = len(rows)
		fmt.Printf("  Inventaire SRU %d : %d communes\n", cfg.Year, len(rows))
	}

	arch.EndRun(ctx, runID, "SUCCESS", stats, "")
	return nil
}
