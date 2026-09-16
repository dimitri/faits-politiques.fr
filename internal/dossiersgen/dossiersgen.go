// Package dossiersgen écrit, dans chaque dossier docs/<dossier>.md, les
// sections tirées de ref.fait_dossier : contexte (dont les mentions dans les
// débats de l'Assemblée), enjeux, cadre, contrôles et évaluations, situation,
// et, pour les dossiers qui suivent des missions de l'État, le tableau des
// crédits par programme (derived.dossier_budget_programme). Appelé par fpctl
// (voir cmd/fpctl) : fpctl generate dossiers.
//
// Chaque section est délimitée par deux marqueurs ; le texte entre eux est
// régénéré à chaque exécution, le reste du document n'est jamais touché.
//
// Pourquoi générer plutôt qu'écrire : une citation d'élu ou de ministre doit
// porter le lien vers sa fiche, une qualité (officiel, déclaratif, presse) et une
// preuve chargée. Les écrire à la main dans vingt documents, c'est garantir
// qu'un jour l'une d'elles divergera de la base.
package dossiersgen

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

var sections = []struct{ code, vide string }{
	{"CONTEXTE", ""},
	{"ENJEUX", ""},
	{"CADRE", "Aucun texte n'est encore chargé pour ce dossier."},
	{"CONTROLE", "Aucun contrôle ni aucune évaluation n'est encore chargé pour ce dossier."},
	{"SITUATION", ""},
	{"BUDGET", ""},
}

var mois = []string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}

func dateFr(t time.Time) string {
	j := fmt.Sprint(t.Day())
	if t.Day() == 1 {
		j = "1er"
	}
	return fmt.Sprintf("%s %s %d", j, mois[t.Month()-1], t.Year())
}

type fait struct {
	id, section, theme, typ, auteur, intitule, constat, url, page, qualite, libelleQualite string
	date                                                                                   *time.Time
	personne, slug                                                                         *string
	aFiche                                                                                 bool
	seance                                                                                 bool
}

func marqueurs(section string) (string, string) {
	return "<!-- faits:" + section + ":debut — généré par cmd/sections-dossiers depuis ref.fait_dossier, ne pas modifier à la main -->",
		"<!-- faits:" + section + ":fin -->"
}

// Run exécute la commande dossiers. Ne prend aucune option ; args n'existe
// que pour l'uniformité avec les autres commandes routées par fpctl.
func Run(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("dossiers ne prend aucune option (%q inattendu)", args[0])
	}
	ctx := context.Background()
	pool, err := store.Open(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT f.dossier, f.id, f.section, coalesce(f.theme,''), f.type, f.auteur, f.intitule, f.constat, f.source_url,
		       coalesce(f.page,''), f.qualite, q.libelle, f.date_fait, p.nom, p.slug, coalesce(p.a_une_fiche, false),
		       f.intervention_slug IS NOT NULL
		FROM ref.fait_dossier f
		JOIN ref.qualite_fait q ON q.code = f.qualite
		LEFT JOIN derived.fait_dossier_personne p ON p.fait_id = f.id
		ORDER BY f.dossier, f.section, f.theme NULLS FIRST, f.date_fait NULLS LAST, f.id`)
	if err != nil {
		return err
	}
	parDossier := map[string][]fait{}
	for rows.Next() {
		var d string
		var f fait
		if err := rows.Scan(&d, &f.id, &f.section, &f.theme, &f.typ, &f.auteur, &f.intitule, &f.constat, &f.url, &f.page,
			&f.qualite, &f.libelleQualite, &f.date, &f.personne, &f.slug, &f.aFiche, &f.seance); err != nil {
			return err
		}
		parDossier[d] = append(parDossier[d], f)
	}
	rows.Close()

	type mention struct {
		libelle             string
		interventions, orat int
		premiere, derniere  time.Time
	}
	mentions := map[string][]mention{}
	mrows, err := pool.Query(ctx, `SELECT dossier, libelle, interventions, orateurs, premiere, derniere
		FROM derived.dossier_mentions_an_total ORDER BY dossier, interventions DESC`)
	if err != nil {
		return err
	}
	for mrows.Next() {
		var d string
		var m mention
		if err := mrows.Scan(&d, &m.libelle, &m.interventions, &m.orat, &m.premiere, &m.derniere); err != nil {
			return err
		}
		mentions[d] = append(mentions[d], m)
	}
	mrows.Close()
	budgets, err := lireBudgets(ctx, pool)
	if err != nil {
		return err
	}
	var couverture [2]time.Time
	if err := pool.QueryRow(ctx, `SELECT min(date_seance), max(date_seance) FROM core.intervention
		WHERE institution = 'ASSEMBLEE_NATIONALE'`).Scan(&couverture[0], &couverture[1]); err != nil {
		return err
	}

	dossiers := map[string]bool{}
	for d := range parDossier {
		dossiers[d] = true
	}
	for d := range mentions {
		dossiers[d] = true
	}
	for d := range budgets {
		dossiers[d] = true
	}
	// Un dossier rangé dans le plan commun sans aucun fait chargé reçoit quand
	// même ses sections, pour qu'il dise « aucun texte n'est encore chargé ».
	docs, _ := filepath.Glob("docs/*.md")
	for _, chemin := range docs {
		if b, err := os.ReadFile(chemin); err == nil && bytes.Contains(b, []byte("<!-- faits:CADRE:debut")) {
			dossiers[strings.TrimSuffix(filepath.Base(chemin), ".md")] = true
		}
	}
	var noms []string
	for d := range dossiers {
		noms = append(noms, d)
	}
	sort.Strings(noms)

	for _, d := range noms {
		chemin := filepath.Join("docs", d+".md")
		src, err := os.ReadFile(chemin)
		if err != nil {
			return fmt.Errorf("%s : %w", d, err)
		}
		for _, s := range sections {
			var b strings.Builder
			if s.code == "CONTEXTE" && len(mentions[d]) > 0 {
				fmt.Fprintf(&b, "Dans les comptes rendus de séance de l'Assemblée nationale chargés (du %s au %s), les interventions qui emploient les mots du dossier :\n\n",
					dateFr(couverture[0]), dateFr(couverture[1]))
				b.WriteString("| expression | interventions | orateurs distincts | première | dernière |\n|---|---:|---:|---|---|\n")
				for _, m := range mentions[d] {
					fmt.Fprintf(&b, "| %s | %d | %d | %s | %s |\n", m.libelle, m.interventions, m.orat, dateFr(m.premiere), dateFr(m.derniere))
				}
				b.WriteString("\nUne mention ne dit pas la position de l'orateur (`derived.dossier_mentions_an`).\n\n")
			}
			if s.code == "BUDGET" {
				b.WriteString(budgets[d])
			}
			n := 0
			theme := "\x00"
			for _, f := range parDossier[d] {
				if f.section != s.code {
					continue
				}
				if f.theme != theme && f.theme != "" && s.code == "SITUATION" {
					fmt.Fprintf(&b, "\n**%s**\n\n", capitaliser(f.theme))
				}
				theme = f.theme
				b.WriteString(ligne(f))
				n++
			}
			if n == 0 && b.Len() == 0 && s.vide != "" {
				b.WriteString(s.vide + "\n")
			}
			debut, fin := marqueurs(s.code)
			i, j := bytes.Index(src, []byte(debut)), bytes.Index(src, []byte(fin))
			if i < 0 || j < i {
				if b.Len() > 0 && !(n == 0 && s.vide != "" && strings.TrimSpace(b.String()) == s.vide) {
					return fmt.Errorf("%s : marqueurs %s absents alors que la base a du contenu pour cette section", chemin, s.code)
				}
				continue
			}
			var out bytes.Buffer
			out.Write(src[:i+len(debut)])
			out.WriteString("\n\n" + strings.TrimSpace(b.String()) + "\n\n")
			out.Write(src[j:])
			src = out.Bytes()
		}
		if err := os.WriteFile(chemin, src, 0o644); err != nil {
			return err
		}
		fmt.Printf("  %-40s %d faits\n", chemin, len(parDossier[d]))
	}
	return nil
}

func capitaliser(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// ligne : un fait en une puce. Le lien vers la fiche de la personne n'est écrit
// que si la fiche existe (mandat national ou vote chargé), sinon le nom seul.
func ligne(f fait) string {
	var b strings.Builder
	b.WriteString("- **" + f.intitule + "**")
	if f.date != nil {
		b.WriteString(" (" + dateFr(*f.date) + ")")
	}
	b.WriteString(". " + strings.TrimSuffix(f.constat, ".") + ". — ")
	if f.personne != nil {
		if f.aFiche {
			fmt.Fprintf(&b, "[%s](/depute/%s/), ", *f.personne, *f.slug)
		} else {
			b.WriteString(*f.personne + ", ")
		}
		b.WriteString(minuscule(f.auteur))
	} else {
		b.WriteString(f.auteur)
	}
	if f.seance {
		b.WriteString(", Assemblée nationale, compte rendu de la séance")
		if f.page != "" {
			b.WriteString(" (" + f.page + ")")
		}
	} else {
		fmt.Fprintf(&b, " · [source](%s)", f.url)
		if f.page != "" {
			if strings.HasPrefix(f.page, "séance") {
				b.WriteString(", " + f.page)
			} else {
				b.WriteString(", p. " + f.page)
			}
		}
	}
	b.WriteString(" · *" + f.libelleQualite + "*\n")
	return b.String()
}

func minuscule(s string) string {
	r := []rune(s)
	if len(r) > 1 && r[1] >= 'a' && r[1] <= 'z' {
		return strings.ToLower(string(r[0])) + string(r[1:])
	}
	return s
}

type ligneBudget struct {
	mission, code, libelle string
	exercice               int
	ae, cp                 float64
}

// lireBudgets : pour chaque dossier, un tableau par mission suivie, crédits de
// paiement par programme sur les exercices chargés. Les montants sont ceux du
// projet de loi de finances : le texte le redit sous chaque tableau, pour qu'un
// tableau recopié seul ne perde pas cette précision.
func lireBudgets(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx, `SELECT dossier, mission, programme_code, programme_libelle, exercice,
		       coalesce(autorisation_engagement, 0), coalesce(credit_paiement, 0)
		FROM derived.dossier_budget_programme ORDER BY dossier, mission, programme_code, exercice`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	parDossier := map[string][]ligneBudget{}
	for rows.Next() {
		var d string
		var l ligneBudget
		if err := rows.Scan(&d, &l.mission, &l.code, &l.libelle, &l.exercice, &l.ae, &l.cp); err != nil {
			return nil, err
		}
		parDossier[d] = append(parDossier[d], l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for d, lignes := range parDossier {
		exSet := map[int]bool{}
		var missionsOrdre []string
		vues := map[string]bool{}
		for _, l := range lignes {
			exSet[l.exercice] = true
			if !vues[l.mission] {
				vues[l.mission] = true
				missionsOrdre = append(missionsOrdre, l.mission)
			}
		}
		var ex []int
		for e := range exSet {
			ex = append(ex, e)
		}
		sort.Ints(ex)
		var b strings.Builder
		for _, m := range missionsOrdre {
			type prog struct {
				libelle string
				cp      map[int]float64
			}
			progs := map[string]*prog{}
			var codes []string
			totCP, totAE := map[int]float64{}, map[int]float64{}
			for _, l := range lignes {
				if l.mission != m {
					continue
				}
				p := progs[l.code]
				if p == nil {
					p = &prog{cp: map[int]float64{}}
					progs[l.code] = p
					codes = append(codes, l.code)
				}
				p.libelle = l.libelle
				p.cp[l.exercice] += l.cp
				totCP[l.exercice] += l.cp
				totAE[l.exercice] += l.ae
			}
			fmt.Fprintf(&b, "**Mission « %s »** — crédits de paiement demandés, en millions d'euros\n\n| programme |", m)
			for _, e := range ex {
				fmt.Fprintf(&b, " PLF %d |", e)
			}
			b.WriteString(" évolution |\n|---|")
			for range ex {
				b.WriteString("---:|")
			}
			b.WriteString("---:|\n")
			for _, c := range codes {
				p := progs[c]
				fmt.Fprintf(&b, "| %s — %s |", c, p.libelle)
				for _, e := range ex {
					if v, ok := p.cp[e]; ok {
						b.WriteString(" " + millions(v) + " |")
					} else {
						b.WriteString(" — |")
					}
				}
				b.WriteString(" " + evolution(p.cp, ex) + " |\n")
			}
			b.WriteString("| **Total** |")
			for _, e := range ex {
				b.WriteString(" **" + millions(totCP[e]) + "** |")
			}
			b.WriteString(" **" + evolution(totCP, ex) + "** |\n")
			b.WriteString("| *Autorisations d'engagement (total)* |")
			for _, e := range ex {
				b.WriteString(" *" + millions(totAE[e]) + "* |")
			}
			b.WriteString(" *" + evolution(totAE, ex) + "* |\n\n")
		}
		b.WriteString("Montants demandés au projet de loi de finances de chaque année, ni votés ni exécutés ; les crédits de personnel incluent les cotisations au compte « Pensions » (`derived.dossier_budget_programme`, source : Direction du budget).\n")
		out[d] = b.String()
	}
	return out, nil
}

// millions : 4567123456.7 → « 4 567,1 ».
func millions(v float64) string {
	s := fmt.Sprintf("%.1f", v/1e6)
	entier, dec, _ := strings.Cut(s, ".")
	neg := strings.HasPrefix(entier, "-")
	entier = strings.TrimPrefix(entier, "-")
	var g []string
	for len(entier) > 3 {
		g = append([]string{entier[len(entier)-3:]}, g...)
		entier = entier[:len(entier)-3]
	}
	g = append([]string{entier}, g...)
	r := strings.Join(g, "\u202f") + "," + dec
	if neg {
		r = "−" + r
	}
	return r
}

// evolution : écart relatif entre le premier et le dernier exercice chargés ;
// un tiret quand le programme n'existe pas aux deux bornes.
func evolution(v map[int]float64, ex []int) string {
	if len(ex) < 2 {
		return "—"
	}
	a, okA := v[ex[0]]
	z, okZ := v[ex[len(ex)-1]]
	if !okA || !okZ || a == 0 {
		return "—"
	}
	p := (z - a) / a * 100
	s := strings.Replace(fmt.Sprintf("%+.1f\u00a0%%", p), ".", ",", 1)
	return strings.Replace(s, "-", "−", 1)
}
