package numerique

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceSanctionsCNIL = archive.Source{
	Slug: "cnil-sanctions", Label: "CNIL — liste des sanctions prononcées",
	Publisher:   "Commission nationale de l'informatique et des libertés",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Page publique de la CNIL, citée avec lien",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : CNIL, les sanctions prononcées par la CNIL",
	Cadence:     "au fil des décisions",
	Notes: "Tableaux annuels de 2011 à l'année en cours. L'organisme est désigné par sa catégorie : la " +
		"publicité nominative d'une sanction est limitée dans le temps et la CNIL retire le nom à son " +
		"expiration. Ce projet ne ré-identifie pas les organismes (D-065). Le jeu data.gouv de la CNIL ne " +
		"donne que des agrégats annuels.",
}

const urlSanctionsCNIL = "https://www.cnil.fr/fr/les-sanctions-prononcees-par-la-cnil"

var (
	reTable      = regexp.MustCompile(`(?s)<table.*?</table>`)
	reRow        = regexp.MustCompile(`(?s)<tr.*?</tr>`)
	reCell       = regexp.MustCompile(`(?s)<t[dh][^>]*>(.*?)</t[dh]>`)
	reLink       = regexp.MustCompile(`href="([^"]+)"`)
	reDateCNIL   = regexp.MustCompile(`^(\d{2})/(\d{2})/(\d{4})`)
	reNumber     = regexp.MustCompile(`(\d[\d .]*(?:,\d+)?)(?:\s*(millions?|milliards?))?`) // espaces insécables déjà normalisées
	reAmount     = regexp.MustCompile(`(?i)(amende|sanctions? p[ée]cuniaires?)[^.]*?\beuros\b`)
	reSimplified = regexp.MustCompile(`(?i)\(?proc[ée]dure simplifi[ée]e\)?`)
	// Catégories publiées qui désignent une personne publique. Écrites sans
	// accents : la CNIL écrit tantôt MINISTÈRE, tantôt MINISTERE.
	rePublic = regexp.MustCompile(`^(MINISTERE|COMMUNE|COLLECTIVITE TERRITORIALE|ETABLISSEMENT PUBLIC|ETABLISSEMENT ADMINISTRATIF|` +
		`UNIVERSITE|ADMINISTRATION|CENTRE HOSPITALIER|DEPARTEMENT|REGION|CONSEIL DEPARTEMENTAL|CONSEIL REGIONAL|PREFECTURE|` +
		`GROUPEMENT REGIONAL D'APPUI AU DEVELOPPEMENT DE LA E-SANTE)`)
)

func IngestSanctionsCNIL(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return run(ctx, arch, SourceSanctionsCNIL, func(srcID, runID int64) (map[string]any, error) {
		f, err := arch.Fetch(ctx, srcID, runID, urlSanctionsCNIL, ".html")
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, err
		}
		var rows [][]any
		years := map[int]bool{}
		amounts := 0
		for _, table := range reTable.FindAllString(string(b), -1) {
			columns := map[string]int{}
			for _, tr := range reRow.FindAllString(table, -1) {
				cs := reCell.FindAllStringSubmatch(tr, -1)
				texts := make([]string, len(cs))
				for i, c := range cs {
					texts[i] = htmlText(c[1])
				}
				// La ligne d'en-tête nomme les colonnes ; les tableaux anciens
				// ont une colonne « thème » en plus.
				if len(columns) == 0 {
					for i, t := range texts {
						u := withoutAccents(strings.ToUpper(t))
						switch {
						case u == "DATE":
							columns["date"] = i
						case strings.Contains(u, "ORGANISME"):
							columns["organisme"] = i
						case strings.Contains(u, "MANQUEMENT"):
							columns["manquements"] = i
						case strings.Contains(u, "DECISION"):
							columns["decision"] = i
						}
					}
					if len(columns) > 0 && len(columns) != 4 {
						return nil, fmt.Errorf("en-tête de tableau inattendu : %q", texts)
					}
					continue
				}
				if len(cs) <= columns["decision"] {
					continue
				}
				m := reDateCNIL.FindStringSubmatch(texts[columns["date"]])
				if m == nil {
					continue // ligne sans date (intertitre)
				}
				day, _ := strconv.Atoi(m[1])
				month, _ := strconv.Atoi(m[2])
				year, _ := strconv.Atoi(m[3])
				date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
				years[year] = true
				entity := texts[columns["organisme"]]
				simplified := reSimplified.MatchString(entity)
				entity = strings.TrimSpace(reSimplified.ReplaceAllString(entity, ""))
				decision := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(texts[columns["decision"]]), "Voir la délibération"))
				var link any
				if l := reLink.FindStringSubmatch(cs[columns["decision"]][1]); l != nil {
					link = l[1]
				}
				amount := sanctionAmount(decision)
				if amount != nil {
					amounts++
				}
				public := rePublic.MatchString(withoutAccents(strings.ToUpper(entity)))
				rows = append(rows, []any{len(rows) + 1, date, entity, nilIfEmpty(texts[columns["manquements"]]), decision,
					amount, link, public, simplified, f.DocumentID})
			}
		}
		// La liste couvre 2011 à l'année en cours : moins de 300 lignes ou une
		// année manquante signalerait un changement de mise en page.
		if len(rows) < 300 || !years[2011] || !years[2020] {
			return nil, fmt.Errorf("%d sanctions lues, années %v", len(rows), years)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `
			CREATE TEMP TABLE tmp_sanction_cnil (
				rang int, date_decision date, organisme text, manquements text, sanction text,
				montant_eur numeric, deliberation_url text, public boolean, procedure_simplifiee boolean,
				document_id bigint
			) ON COMMIT DROP`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_sanction_cnil"},
			[]string{"rang", "date_decision", "organisme", "manquements", "sanction", "montant_eur", "deliberation_url",
				"public", "procedure_simplifiee", "document_id"}, pgx.CopyFromRows(rows)); err != nil {
			return nil, err
		}
		// MERGE plutôt que DELETE+COPY : l'ancien DELETE (table entière, ce
		// connecteur en est l'unique propriétaire) payait le prix des triggers
		// RI à chaque republication de la page, changement ou non.
		ct, err := tx.Exec(ctx, `
			MERGE INTO core.sanction_cnil AS tgt
			USING tmp_sanction_cnil AS src
			ON tgt.rang = src.rang AND tgt.document_id = src.document_id
			WHEN MATCHED AND (tgt.date_decision, tgt.organisme, tgt.manquements, tgt.sanction,
			                   tgt.montant_eur, tgt.deliberation_url, tgt.public, tgt.procedure_simplifiee)
			                  IS DISTINCT FROM
			                  (src.date_decision, src.organisme, src.manquements, src.sanction,
			                   src.montant_eur, src.deliberation_url, src.public, src.procedure_simplifiee) THEN
			    UPDATE SET date_decision = src.date_decision, organisme = src.organisme,
			               manquements = src.manquements, sanction = src.sanction,
			               montant_eur = src.montant_eur, deliberation_url = src.deliberation_url,
			               public = src.public, procedure_simplifiee = src.procedure_simplifiee
			WHEN NOT MATCHED BY TARGET THEN
			    INSERT (rang, date_decision, organisme, manquements, sanction, montant_eur,
			            deliberation_url, public, procedure_simplifiee, document_id)
			    VALUES (src.rang, src.date_decision, src.organisme, src.manquements, src.sanction,
			            src.montant_eur, src.deliberation_url, src.public, src.procedure_simplifiee,
			            src.document_id)
			WHEN NOT MATCHED BY SOURCE THEN DELETE`)
		if err != nil {
			return nil, err
		}
		return map[string]any{"sanctions": len(rows), "avec_montant": amounts,
			"touchees": ct.RowsAffected()}, tx.Commit(ctx)
	})
}

// sanctionAmount lit « amende de 27 millions d'euros », « sanction pécuniaire
// de 150 000 000 euros » ou « sanctions pécuniaires de 60 et 40 millions
// d'euros » (100 M€). NULL quand le libellé ne publie pas de montant.
func sanctionAmount(s string) any {
	m := reAmount.FindString(s)
	if m == "" {
		return nil
	}
	// Le multiplicateur écrit après le dernier nombre vaut pour tous
	// (« 60 et 40 millions »).
	numbers := reNumber.FindAllStringSubmatch(m, -1)
	mult := 1.0
	if len(numbers) > 0 {
		switch u := numbers[len(numbers)-1][2]; {
		case strings.HasPrefix(u, "million"):
			mult = 1e6
		case strings.HasPrefix(u, "milliard"):
			mult = 1e9
		}
	}
	total := 0.0
	for _, n := range numbers {
		v := strings.NewReplacer(" ", "", "\u00a0", "", "\u202f", "", ".", "").Replace(n[1])
		v = strings.Replace(v, ",", ".", 1)
		x, err := strconv.ParseFloat(v, 64)
		if err != nil {
			continue
		}
		total += x
	}
	if total == 0 && strings.Contains(m, "un million") {
		total, mult = 1, 1e6 // « amende administrative d'un million d'euros »
	}
	if total == 0 {
		return nil
	}
	return strconv.FormatFloat(total*mult, 'f', 0, 64)
}

// withoutAccents suffit pour les catégories en capitales de la CNIL.
var withoutAccentsR = strings.NewReplacer("É", "E", "È", "E", "Ê", "E", "À", "A", "Â", "A", "Î", "I", "Ô", "O", "Û", "U", "Ç", "C", "Œ", "OE")

func withoutAccents(s string) string { return withoutAccentsR.Replace(s) }
