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

// anneeSRU décrit un millésime : son adresse, son délimiteur réel et le nom
// EXACT de chaque colonne dans CE fichier. Les noms ne suivent aucun motif
// commun d'un millésime à l'autre (accents tantôt présents, tantôt non ;
// année tantôt dans le nom, tantôt absente) — vérifiés directement sur
// chaque fichier téléchargé plutôt que supposés identiques au millésime 2025
// déjà chargé.
type anneeSRU struct {
	Annee          int
	URL            string
	Delimiteur     rune
	SauterLignes   int // le fichier 2024 commence par une ligne entièrement vide avant l'en-tête
	ColDept        string
	DeptEstCode    bool // 2023/2024 : ColDept est un CODE de département (résolu via geo.contour) ; 2025/2026 : c'est déjà le nom
	ColPopulation  string
	ColTaux        string
	ColCible       string
	ColDeficitaire string
	ColCarencee    string
	ColExemptee    string
	ColPrelevement string // "" : colonne absente de ce millésime (2023)
}

var anneesSRU = []anneeSRU{
	{
		Annee: 2023, Delimiteur: ',', SauterLignes: 0,
		URL:     "https://static.data.gouv.fr/resources/communes-et-inventaire-sru/20250922-104633/donnees-sru-data-gouv-maj2023-vf.csv",
		ColDept: "Code_Departement", DeptEstCode: true,
		ColPopulation:  "Population_municipale_01_01_2023",
		ColTaux:        "Taux_SRU_au_01_01_2022",
		ColCible:       "Taux_cible_commune_2023_2025",
		ColDeficitaire: "Commune_deficitaire_2023",
		ColCarencee:    "Commune_carencee_2023_2025",
		ColExemptee:    "Commune_exemptee_2023_2025",
	},
	{
		Annee: 2024, Delimiteur: ',', SauterLignes: 1,
		URL:     "https://static.data.gouv.fr/resources/communes-et-inventaire-sru/20250929-132410/donnees-sru-data-gouv-maj2024-vf.csv",
		ColDept: "Code_Département", DeptEstCode: true,
		ColPopulation:  "Population_municipale_01_01_2024",
		ColTaux:        "Taux_SRU_au_01_01_2023",
		ColCible:       "Taux_cible_commune_2023_2025",
		ColDeficitaire: "Commune_déficitaire_2024",
		ColCarencee:    "Commune_carencée_2023_2025",
		ColExemptee:    "Commune_exemptée_2023_2025",
		ColPrelevement: "Prélèvement_net_2024_dont_majoration",
	},
	{
		Annee: 2025, Delimiteur: ';', SauterLignes: 0,
		URL:     "https://static.data.gouv.fr/resources/communes-et-inventaire-sru/20251219-143258/donnees-sru-data-gouv-2025-v2.csv",
		ColDept: "Departement", DeptEstCode: false,
		ColPopulation:  "Population_municipale_01_01_2025",
		ColTaux:        "Taux_SRU_au_01_01_2024",
		ColCible:       "Taux_cible_commune_2023_2025",
		ColDeficitaire: "commune_deficitaire",
		ColCarencee:    "Commune_carencée",
		ColExemptee:    "Commune_exemptée_2023_2025",
		ColPrelevement: "Prélèvement_net_2025_dont_majoration",
	},
	{
		Annee: 2026, Delimiteur: ';', SauterLignes: 0,
		URL:     "https://static.data.gouv.fr/resources/communes-et-inventaire-sru/20260811-123753/donnees-sru-data-gouv-2026.csv",
		ColDept: "Departement", DeptEstCode: false,
		ColPopulation:  "Population_municipale_01_01_2026",
		ColTaux:        "Taux_SRU_au_01_01_2025",
		ColCible:       "Taux_cible_commune",
		ColDeficitaire: "commune_deficitaire_au_01-01-2025",
		ColCarencee:    "Commune_carencee_2023_2025",
		ColExemptee:    "Commune_exemptee_2026_2028",
		ColPrelevement: "Prelevement_net_2026_dont_majoration",
	},
}

func parserPourcentageFr(s string) (float64, bool) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func aBool01(s string) bool { return strings.TrimSpace(s) == "1" }

// parserMontantEUR lit un montant en euros au format de la source
// (« 64 509,48 € », espace fine insécable en séparateur de milliers, virgule
// décimale, et « - € » ou « 0,00 € » pour un prélèvement nul — les deux
// formes valent 0, jamais NULL : la colonne existe et dit explicitement
// zéro, ce qui diffère du millésime 2023 où la colonne n'existe pas du tout.
func parserMontantEUR(s string) (float64, bool) {
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

type ligneSRU struct {
	annee                           int
	codeInsee, commune, departement string
	population, nbLLS               *int
	tauxSRU, tauxCible, prelevement *float64
	deficitaire, carencee, exemptee bool
}

// chargerDepartements : code -> nom, pour résoudre les millésimes 2023 et
// 2024 qui ne publient que le CODE du département (les millésimes 2025 et
// 2026 publient directement son nom, dans une colonne distincte).
func chargerDepartements(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx, `SELECT code_insee, nom FROM geo.contour WHERE niveau='DEPARTEMENT'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var code, nom string
		if err := rows.Scan(&code, &nom); err != nil {
			return nil, err
		}
		m[code] = nom
	}
	return m, rows.Err()
}

// parserAnneeSRU télécharge et lit un millésime. Le fichier est encodé en
// Windows-1252 (et non le Latin-1 pur utilisé par internal/macro/accord_paris.go
// ailleurs sur ce site) : la distinction n'était pas visible tant que personne
// ne lisait le prélèvement, mais son signe euro (octet 0x80) n'a pas la même
// signification dans les deux jeux de caractères — une conversion Latin-1
// l'aurait laissé comme caractère de contrôle invisible plutôt que « € ».
func parserAnneeSRU(ctx context.Context, arch *archive.Archive, srcID, runID int64,
	cfg anneeSRU, deptParCode map[string]string) ([]ligneSRU, error) {

	f, err := arch.Fetch(ctx, srcID, runID, cfg.URL, ".csv")
	if err != nil {
		return nil, fmt.Errorf("millésime %d : %w", cfg.Annee, err)
	}
	octets, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, err
	}
	texte, err := charmap.Windows1252.NewDecoder().String(string(octets))
	if err != nil {
		return nil, fmt.Errorf("millésime %d : décodage Windows-1252 : %w", cfg.Annee, err)
	}

	r := csv.NewReader(strings.NewReader(texte))
	r.Comma = cfg.Delimiteur
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	for i := 0; i < cfg.SauterLignes; i++ {
		if _, err := r.Read(); err != nil {
			return nil, fmt.Errorf("millésime %d : ligne à sauter illisible : %w", cfg.Annee, err)
		}
	}
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("millésime %d : en-tête illisible : %w", cfg.Annee, err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(h)] = i
	}
	must := []string{"Code_INSEE_commune", "Nom_commune", cfg.ColDept, cfg.ColPopulation,
		cfg.ColTaux, cfg.ColCible, cfg.ColDeficitaire, cfg.ColCarencee, cfg.ColExemptee}
	if cfg.ColPrelevement != "" {
		must = append(must, cfg.ColPrelevement)
	}
	for _, m := range must {
		if _, ok := col[m]; !ok {
			return nil, fmt.Errorf("millésime %d : colonne %q absente (en-tête : %v)", cfg.Annee, m, header)
		}
	}
	colLLS := -1
	for h, i := range col {
		if strings.HasPrefix(h, "Nombre_lls") {
			colLLS = i
		}
	}
	if colLLS == -1 {
		return nil, fmt.Errorf("millésime %d : colonne du nombre de logements sociaux introuvable", cfg.Annee)
	}

	var lignes []ligneSRU
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("millésime %d : ligne illisible : %w", cfg.Annee, err)
		}
		champ := func(nom string) string {
			i, ok := col[nom]
			if !ok || i >= len(rec) {
				return ""
			}
			return rec[i]
		}
		l := ligneSRU{
			annee:       cfg.Annee,
			codeInsee:   strings.TrimSpace(champ("Code_INSEE_commune")),
			commune:     strings.TrimSpace(champ("Nom_commune")),
			deficitaire: aBool01(champ(cfg.ColDeficitaire)),
			carencee:    aBool01(champ(cfg.ColCarencee)),
			exemptee:    aBool01(champ(cfg.ColExemptee)),
		}
		if l.codeInsee == "" {
			continue
		}
		dept := strings.TrimSpace(champ(cfg.ColDept))
		if cfg.DeptEstCode {
			if nom, ok := deptParCode[dept]; ok {
				l.departement = nom
			} else {
				l.departement = dept
			}
		} else {
			l.departement = dept
		}
		if v, err := strconv.Atoi(strings.TrimSpace(champ(cfg.ColPopulation))); err == nil {
			l.population = &v
		}
		if v, err := strconv.Atoi(strings.TrimSpace(rec[colLLS])); err == nil {
			l.nbLLS = &v
		}
		if v, ok := parserPourcentageFr(champ(cfg.ColTaux)); ok {
			l.tauxSRU = &v
		}
		if v, ok := parserPourcentageFr(champ(cfg.ColCible)); ok {
			l.tauxCible = &v
		}
		if cfg.ColPrelevement != "" {
			if v, ok := parserMontantEUR(champ(cfg.ColPrelevement)); ok {
				l.prelevement = &v
			}
		}
		lignes = append(lignes, l)
	}
	if len(lignes) < 2000 {
		return nil, fmt.Errorf("millésime %d : seulement %d lignes lues, attendu au moins 2000", cfg.Annee, len(lignes))
	}
	return lignes, nil
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

	deptParCode, err := chargerDepartements(ctx, pool)
	if err != nil {
		return fail(fmt.Errorf("départements (geo.contour) : %w", err))
	}
	if len(deptParCode) == 0 {
		return fail(fmt.Errorf("geo.contour ne contient aucun département : ingérer les contours avant SRU"))
	}

	stats := map[string]any{}
	for _, cfg := range anneesSRU {
		lignes, err := parserAnneeSRU(ctx, arch, srcID, runID, cfg, deptParCode)
		if err != nil {
			return fail(err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fail(err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM core.sru_commune WHERE annee = $1`, cfg.Annee); err != nil {
			tx.Rollback(ctx)
			return fail(err)
		}
		for _, l := range lignes {
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.sru_commune
					(code_insee, annee, commune, departement, population, nombre_logements_sociaux,
					 taux_sru_pct, taux_cible_pct, deficitaire, carencee, exemptee, prelevement_net, source_id)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
				ON CONFLICT (code_insee, annee) DO NOTHING`,
				l.codeInsee, l.annee, l.commune, l.departement, l.population, l.nbLLS,
				l.tauxSRU, l.tauxCible, l.deficitaire, l.carencee, l.exemptee, l.prelevement, srcID); err != nil {
				tx.Rollback(ctx)
				return fail(fmt.Errorf("millésime %d, %s : insertion : %w", cfg.Annee, l.codeInsee, err))
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return fail(err)
		}
		stats[fmt.Sprintf("communes_%d", cfg.Annee)] = len(lignes)
		fmt.Printf("  Inventaire SRU %d : %d communes\n", cfg.Annee, len(lignes))
	}

	arch.EndRun(ctx, runID, "SUCCESS", stats, "")
	return nil
}
