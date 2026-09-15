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
	reTable     = regexp.MustCompile(`(?s)<table.*?</table>`)
	reLigne     = regexp.MustCompile(`(?s)<tr.*?</tr>`)
	reCellule   = regexp.MustCompile(`(?s)<t[dh][^>]*>(.*?)</t[dh]>`)
	reLien      = regexp.MustCompile(`href="([^"]+)"`)
	reDateCNIL  = regexp.MustCompile(`^(\d{2})/(\d{2})/(\d{4})`)
	reNombre    = regexp.MustCompile(`(\d[\d .]*(?:,\d+)?)(?:\s*(millions?|milliards?))?`) // espaces insécables déjà normalisées
	reMontant   = regexp.MustCompile(`(?i)(amende|sanctions? p[ée]cuniaires?)[^.]*?\beuros\b`)
	reSimplifie = regexp.MustCompile(`(?i)\(?proc[ée]dure simplifi[ée]e\)?`)
	// Catégories publiées qui désignent une personne publique. Écrites sans
	// accents : la CNIL écrit tantôt MINISTÈRE, tantôt MINISTERE.
	rePublic = regexp.MustCompile(`^(MINISTERE|COMMUNE|COLLECTIVITE TERRITORIALE|ETABLISSEMENT PUBLIC|ETABLISSEMENT ADMINISTRATIF|` +
		`UNIVERSITE|ADMINISTRATION|CENTRE HOSPITALIER|DEPARTEMENT|REGION|CONSEIL DEPARTEMENTAL|CONSEIL REGIONAL|PREFECTURE|` +
		`GROUPEMENT REGIONAL D'APPUI AU DEVELOPPEMENT DE LA E-SANTE)`)
)

func IngestSanctionsCNIL(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executer(ctx, arch, SourceSanctionsCNIL, func(srcID, runID int64) (map[string]any, error) {
		f, err := arch.Fetch(ctx, srcID, runID, urlSanctionsCNIL, ".html")
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, err
		}
		var lignes [][]any
		annees := map[int]bool{}
		montants := 0
		for _, table := range reTable.FindAllString(string(b), -1) {
			col := map[string]int{}
			for _, tr := range reLigne.FindAllString(table, -1) {
				cs := reCellule.FindAllStringSubmatch(tr, -1)
				textes := make([]string, len(cs))
				for i, c := range cs {
					textes[i] = texteHTML(c[1])
				}
				// La ligne d'en-tête nomme les colonnes ; les tableaux anciens
				// ont une colonne « thème » en plus.
				if len(col) == 0 {
					for i, t := range textes {
						u := sansAccents(strings.ToUpper(t))
						switch {
						case u == "DATE":
							col["date"] = i
						case strings.Contains(u, "ORGANISME"):
							col["organisme"] = i
						case strings.Contains(u, "MANQUEMENT"):
							col["manquements"] = i
						case strings.Contains(u, "DECISION"):
							col["decision"] = i
						}
					}
					if len(col) > 0 && len(col) != 4 {
						return nil, fmt.Errorf("en-tête de tableau inattendu : %q", textes)
					}
					continue
				}
				if len(cs) <= col["decision"] {
					continue
				}
				m := reDateCNIL.FindStringSubmatch(textes[col["date"]])
				if m == nil {
					continue // ligne sans date (intertitre)
				}
				j, _ := strconv.Atoi(m[1])
				mo, _ := strconv.Atoi(m[2])
				a, _ := strconv.Atoi(m[3])
				date := time.Date(a, time.Month(mo), j, 0, 0, 0, 0, time.UTC)
				annees[a] = true
				organisme := textes[col["organisme"]]
				simplifiee := reSimplifie.MatchString(organisme)
				organisme = strings.TrimSpace(reSimplifie.ReplaceAllString(organisme, ""))
				decision := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(textes[col["decision"]]), "Voir la délibération"))
				var lien any
				if l := reLien.FindStringSubmatch(cs[col["decision"]][1]); l != nil {
					lien = l[1]
				}
				montant := montantSanction(decision)
				if montant != nil {
					montants++
				}
				public := rePublic.MatchString(sansAccents(strings.ToUpper(organisme)))
				lignes = append(lignes, []any{len(lignes) + 1, date, organisme, nul(textes[col["manquements"]]), decision,
					montant, lien, public, simplifiee, f.DocumentID})
			}
		}
		// La liste couvre 2011 à l'année en cours : moins de 300 lignes ou une
		// année manquante signalerait un changement de mise en page.
		if len(lignes) < 300 || !annees[2011] || !annees[2020] {
			return nil, fmt.Errorf("%d sanctions lues, années %v", len(lignes), annees)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `DELETE FROM core.sanction_cnil`); err != nil {
			return nil, err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "sanction_cnil"},
			[]string{"rang", "date_decision", "organisme", "manquements", "sanction", "montant_eur", "deliberation_url",
				"public", "procedure_simplifiee", "document_id"}, pgx.CopyFromRows(lignes)); err != nil {
			return nil, err
		}
		return map[string]any{"sanctions": len(lignes), "avec_montant": montants}, tx.Commit(ctx)
	})
}

// montantSanction lit « amende de 27 millions d'euros », « sanction pécuniaire
// de 150 000 000 euros » ou « sanctions pécuniaires de 60 et 40 millions
// d'euros » (100 M€). NULL quand le libellé ne publie pas de montant.
func montantSanction(s string) any {
	m := reMontant.FindString(s)
	if m == "" {
		return nil
	}
	// Le multiplicateur écrit après le dernier nombre vaut pour tous
	// (« 60 et 40 millions »).
	nombres := reNombre.FindAllStringSubmatch(m, -1)
	mult := 1.0
	if len(nombres) > 0 {
		switch u := nombres[len(nombres)-1][2]; {
		case strings.HasPrefix(u, "million"):
			mult = 1e6
		case strings.HasPrefix(u, "milliard"):
			mult = 1e9
		}
	}
	total := 0.0
	for _, n := range nombres {
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

// sansAccents suffit pour les catégories en capitales de la CNIL.
var sansAccentsR = strings.NewReplacer("É", "E", "È", "E", "Ê", "E", "À", "A", "Â", "A", "Î", "I", "Ô", "O", "Û", "U", "Ç", "C", "Œ", "OE")

func sansAccents(s string) string { return sansAccentsR.Replace(s) }
