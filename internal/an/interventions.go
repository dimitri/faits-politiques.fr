package an

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Les comptes rendus de séance publique.
//
// Ce jeu a une propriété que le Journal officiel n'a pas : chaque paragraphe
// porte `id_acteur`, l'identifiant du député qui parle. Le rattachement à une
// personne est donc un appariement PAR IDENTIFIANT — aucune homonymie à
// arbitrer, aucune prose à fouiller.
//
// Il porte aussi l'attribut stime : la POSITION de la prise de parole dans la
// séance, en secondes depuis son ouverture — et non sa durée. La confondre avec
// une durée donnait 539 310 heures de parole en deux ans. La durée se déduit de
// l'écart avec l'intervention suivante, et c'est une valeur dérivée.
var SourceInterventions = archive.Source{
	Slug: "an-comptes-rendus", Label: "Assemblée nationale — comptes rendus de séance",
	Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Licence Ouverte",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Source : Assemblée nationale, comptes rendus de la séance publique",
	Cadence:     "après chaque séance",
	Notes: "Version « avant_JO » pour les séances récentes : le compte rendu peut " +
		"être révisé avant publication au Journal officiel.",
}

const interventionsURL = Base + "/vp/syceronbrut/syseron.xml.zip"

type compteRendu struct {
	Metadonnees struct {
		DateSeance     string `xml:"dateSeance"`
		DateSeanceJour string `xml:"dateSeanceJour"`
		NumSeance      string `xml:"numSeance"`
		Legislature    string `xml:"legislature"`
		Session        string `xml:"session"`
	} `xml:"metadonnees"`
	Contenu struct {
		Points []point `xml:"point"`
	} `xml:"contenu"`
}

// Les paragraphes sont imbriqués dans des points de l'ordre du jour, eux-mêmes
// imbriqués. Le décodeur les collecte à toute profondeur.
type point struct {
	Points      []point      `xml:"point"`
	Paragraphes []paragraphe `xml:"paragraphe"`
}

type paragraphe struct {
	IDSyceron string `xml:"id_syceron,attr"`
	IDActeur  string `xml:"id_acteur,attr"`
	Ordre     string `xml:"ordre_absolu_seance,attr"`
	RoleDebat string `xml:"roledebat,attr"`
	Texte     struct {
		Stime   string `xml:"stime,attr"`
		Contenu string `xml:",innerxml"`
	} `xml:"texte"`
}

func IngestInterventions(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceInterventions)
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

	f, err := arch.Fetch(ctx, srcID, runID, interventionsURL, ".zip")
	if err != nil {
		return fail(err)
	}
	acteurs, err := indexActeurs(ctx, pool)
	if err != nil {
		return fail(err)
	}

	zr, err := zip.OpenReader(f.Path)
	if err != nil {
		return fail(err)
	}
	defer zr.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	for _, q := range []string{
		`SET LOCAL work_mem = '256MB'`,
		`DELETE FROM core.intervention WHERE institution = 'ASSEMBLEE_NATIONALE'`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			return fail(err)
		}
	}

	var lot [][]any
	var n, sansActeur, seances int
	vus := map[string]bool{}

	vider := func() error {
		if len(lot) == 0 {
			return nil
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"core", "intervention"},
			[]string{"slug", "institution", "source_uid", "person_id", "date_seance",
				"contenu", "legislature", "session", "numero_seance", "ordre",
				"role_debat", "instant_s", "source_id"},
			pgx.CopyFromRows(lot))
		lot = lot[:0]
		return err
	}

	for _, zf := range zr.File {
		if !strings.HasSuffix(zf.Name, ".xml") {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		var cr compteRendu
		errDec := xml.NewDecoder(rc).Decode(&cr)
		rc.Close()
		if errDec != nil {
			continue
		}
		date := dateSeance(cr.Metadonnees.DateSeance)
		if date == "" {
			continue
		}
		seances++

		for _, p := range aplatir(cr.Contenu.Points) {
			texte := texteBrut(p.Texte.Contenu)
			if p.IDSyceron == "" || texte == "" || vus[p.IDSyceron] {
				continue
			}
			vus[p.IDSyceron] = true

			var pid any
			if id, ok := acteurs[p.IDActeur]; ok {
				pid = id
			} else if p.IDActeur != "" {
				// Un orateur extérieur — ministre non député, invité — n'est
				// pas rattaché de force : il est compté.
				sansActeur++
			}

			lot = append(lot, []any{
				"cr-" + strings.ToLower(p.IDSyceron), "ASSEMBLEE_NATIONALE", p.IDSyceron,
				pid, date, texte,
				nulA(cr.Metadonnees.Legislature), nulA(session(cr.Metadonnees.Session)),
				nulA(cr.Metadonnees.NumSeance), entierNulA(p.Ordre),
				nulA(p.RoleDebat), decimalNulA(p.Texte.Stime), srcID,
			})
			n++
			if len(lot) >= 20000 {
				if err := vider(); err != nil {
					return fail(fmt.Errorf("copie des interventions : %w", err))
				}
			}
		}
	}
	if err := vider(); err != nil {
		return fail(fmt.Errorf("copie des interventions : %w", err))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS", map[string]any{
		"interventions": n, "seances": seances, "sans_acteur": sansActeur}, "")
	fmt.Printf("  interventions  %d sur %d séances (%d orateurs hors Assemblée)\n",
		n, seances, sansActeur)
	return nil
}

// aplatir descend dans les points de l'ordre du jour, qui s'imbriquent sans
// profondeur fixe.
func aplatir(points []point) []paragraphe {
	var out []paragraphe
	for _, p := range points {
		out = append(out, p.Paragraphes...)
		out = append(out, aplatir(p.Points)...)
	}
	return out
}

// dateSeance lit « 20241106140000000 ».
func dateSeance(s string) string {
	if len(s) < 8 {
		return ""
	}
	return s[:4] + "-" + s[4:6] + "-" + s[6:8]
}

// La métadonnée de session est saisie à la main dans les comptes rendus, et
// elle s'en ressent : « Session ordinaire 2025 -2026 » et « Session ordinaire
// 2025-2026 » désignent la même session, et coupaient en deux le temps de
// parole de chaque orateur. Deux règles suffisent : plus d'espace autour du
// tiret d'un intervalle d'années, et une majuscule initiale — « deuxième
// session extraordinaire 2025 » se rangeait ailleurs que ses sœurs.
var (
	reBlancsSeance = regexp.MustCompile(`\s+`)
	reAnneeTiret   = regexp.MustCompile(`(\d)\s*-\s*(\d)`)
)

func session(s string) string {
	s = strings.TrimSpace(reBlancsSeance.ReplaceAllString(s, " "))
	s = reAnneeTiret.ReplaceAllString(s, "$1-$2")
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[:1])) + string(r[1:])
}

func entierNulA(s string) any {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return n
}

func decimalNulA(s string) any {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return nil
	}
	return v
}

var _ = html.UnescapeString
