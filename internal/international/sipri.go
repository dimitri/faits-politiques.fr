package international

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

// L'effort de défense comparé — SIPRI est la seule base qui remonte à 1949
// pour tous les pays de comparaison, y compris l'Arabie saoudite (absente
// de la dette FMI hors zone euro pour d'autres raisons, docs/dette-
// donnees.md § 5, mais présente ici). Part du PIB, pas un montant en
// devise : directement comparable d'un pays à l'autre sans conversion de
// change ni effet d'inflation, la même raison qui fait préférer le taux de
// dépassement au montant brut ailleurs dans ce dépôt.
//
// Licence non commerciale avec attribution (SIPRI, conditions générales) —
// compatible avec ce site, qui n'a aucune vocation commerciale.
var SourceSIPRIMilex = archive.Source{
	Slug: "sipri-milex", Label: "SIPRI — dépense militaire, part du PIB",
	Publisher: "Stockholm International Peace Research Institute (SIPRI)", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Usage non commercial avec attribution (conditions SIPRI) — ce site n'a pas de vocation commerciale",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : SIPRI Military Expenditure Database",
	Cadence:     "annuelle",
	Notes: "Feuille « Share of GDP » du classeur officiel, 1949-2025. Les valeurs marquées " +
		"« xxx » (pays non indépendant) ou « . . » (donnée indisponible) dans la source ne " +
		"deviennent pas des zéros : ces années sont simplement absentes de la table plutôt que " +
		"complétées par une estimation.",
}

const sipriMilexURL = "https://www.sipri.org/sites/default/files/SIPRI-Milex-data-1949-2025_v1.2.xlsx"

const sipriFeuille = "Share of GDP"

// Libellés SIPRI, distincts par endroits de ceux des autres sources déjà
// chargées (« United States of America », pas « United States » ; « UK »,
// pas « United Kingdom » selon l'édition) — vérifiés sur le classeur réel
// avant d'écrire cette table plutôt que devinés.
var paysSIPRI = map[string]string{
	"France": "FR", "United States of America": "US", "Japan": "JP", "Germany": "DE",
	"UK": "GB", "United Kingdom": "GB", "Italy": "IT", "Canada": "CA",
	"Russia": "RU", "Russian Federation": "RU", "China": "CN", "Saudi Arabia": "SA",
}

func IngestSIPRIMilex(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceSIPRIMilex)
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

	f, err := arch.Fetch(ctx, srcID, runID, sipriMilexURL, ".xlsx")
	if err != nil {
		return fail(err)
	}
	wb, err := excelize.OpenFile(f.Path)
	if err != nil {
		return fail(fmt.Errorf("classeur SIPRI illisible : %w", err))
	}
	defer wb.Close()

	toutesLignes, err := wb.GetRows(sipriFeuille)
	if err != nil {
		return fail(fmt.Errorf("feuille %q : %w", sipriFeuille, err))
	}
	if len(toutesLignes) < 7 {
		return fail(fmt.Errorf("feuille %q : trop courte (%d lignes) — format changé", sipriFeuille, len(toutesLignes)))
	}
	entete := toutesLignes[5] // ligne 6 : "Country", "Notes", années...
	if strings.TrimSpace(entete[0]) != "Country" {
		return fail(fmt.Errorf("feuille %q : l'en-tête attendu en ligne 6 (%q) n'est plus 'Country' — format changé", sipriFeuille, entete[0]))
	}
	colAnnee := map[int]int{}
	for i, h := range entete {
		if a, err := strconv.Atoi(strings.TrimSpace(h)); err == nil {
			colAnnee[i] = a
		}
	}

	trouve := map[string]bool{}
	var rows [][]any
	for _, ligne := range toutesLignes[6:] {
		if len(ligne) == 0 {
			continue
		}
		pays2, ok := paysSIPRI[strings.TrimSpace(ligne[0])]
		if !ok {
			continue
		}
		trouve[pays2] = true
		for col, annee := range colAnnee {
			if col >= len(ligne) {
				continue
			}
			brut := strings.TrimSpace(ligne[col])
			if brut == "" || brut == "xxx" || brut == ". ." || brut == "..." {
				continue
			}
			// Le classeur mélange deux mises en forme de cellule pour la
			// même colonne : certaines cases affichent une fraction brute
			// (« 0.0203 »), d'autres un pourcentage déjà mis en forme par
			// Excel (« 5.35% ») — excelize restitue le texte affiché, pas la
			// valeur numérique sous-jacente. Les deux formes coexistent dans
			// le fichier réel, vérifié colonne par colonne avant d'écrire ce
			// connecteur plutôt que supposées uniformes.
			enPourcent := strings.HasSuffix(brut, "%")
			valeur, err := strconv.ParseFloat(strings.TrimSuffix(brut, "%"), 64)
			if err != nil {
				return fail(fmt.Errorf("%s %d : valeur %q illisible : %w", ligne[0], annee, brut, err))
			}
			if !enPourcent {
				valeur *= 100
			}
			rows = append(rows, []any{pays2, libellesPays[pays2], "SIPRI_DEPENSE_MILITAIRE_PIB", annee, valeur, srcID})
		}
	}
	for pays3, pays2 := range map[string]string{"FRA": "FR", "USA": "US", "JPN": "JP", "DEU": "DE",
		"GBR": "GB", "ITA": "IT", "CAN": "CA", "RUS": "RU", "CHN": "CN", "SAU": "SA"} {
		if !trouve[pays2] {
			return fail(fmt.Errorf("pays de comparaison introuvable dans le classeur SIPRI : %s (%s)", pays3, pays2))
		}
	}
	if len(rows) == 0 {
		return fail(fmt.Errorf("aucune valeur lue"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM core.indicateur_mondial WHERE indicateur = 'SIPRI_DEPENSE_MILITAIRE_PIB'`); err != nil {
		return fail(err)
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "indicateur_mondial"},
		[]string{"pays_code", "pays_label", "indicateur", "annee", "valeur", "source_id"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fail(fmt.Errorf("indicateur_mondial (SIPRI) : %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"lignes": n}, "")
	fmt.Printf("  Dépense militaire, part du PIB (SIPRI) : %d lignes, 10 pays\n", n)
	return nil
}
