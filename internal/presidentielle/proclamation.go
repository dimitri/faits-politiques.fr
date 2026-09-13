package presidentielle

import (
	"context"
	"fmt"
	"html"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/balisage"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "presidentielle-v1"

// Les décisions du Conseil constitutionnel ne sont pas un jeu de données : ce
// sont des actes juridictionnels, publiés au Journal officiel. Le site du
// Conseil en est la reproduction de référence, et c'est elle qu'on scelle.
var SourceProclamation = archive.Source{
	Slug: "cc-proclamation-pdr", Label: "Conseil constitutionnel — proclamations de l'élection présidentielle",
	Publisher: "Conseil constitutionnel", Tier: "PRIMARY_OFFICIAL",
	Licence:     "Texte officiel de nature juridictionnelle — hors champ du droit d'auteur",
	ReuseClass:  "OPEN",
	Attribution: "Source : Conseil constitutionnel, décisions de proclamation des résultats de l'élection présidentielle",
	Cadence:     "par scrutin",
	Notes: "Les chiffres proclamés diffèrent de ceux annoncés le soir du scrutin : le " +
		"Conseil tranche les réclamations et peut annuler les suffrages de bureaux entiers. " +
		"Avant 2017 la décision ne publie ni blancs ni nuls ; en 2017 les blancs seuls ; " +
		"les deux séparément à partir de 2022.",
}

// En typographie française, l'espace qui précède un deux-points est souvent
// insécable (U+00A0) ou fine insécable (U+202F). Or \s, en Go, ne couvre que
// l'espace ordinaire : une classe d'espaces explicite est indispensable, sinon
// aucune décision ancienne n'est reconnue.
const esp = `[\s\x{00a0}\x{202f}]`

// Les chiffres eux-mêmes mêlent points (1965), espaces ordinaires et insécables,
// et de vraies coquilles de saisie (« 36 398762 »). On capture large et on ne
// garde que les chiffres ; la capture s'arrête à la première lettre, si bien que
// « 41 191 169Votants » donne 41191169 plutôt qu'une erreur.
const nb = `[\d\s.\x{00a0}\x{202f}]+`

var (
	reInscrits = regexp.MustCompile(`(?i)[EÉ]lecteurs` + esp + `+inscrits` + esp + `*:` + esp + `*(` + nb + `)`)
	reVotants  = regexp.MustCompile(`(?i)Votants` + esp + `*\.?` + esp + `*:` + esp + `*(` + nb + `)`)
	reBlancs   = regexp.MustCompile(`(?i)Bulletins` + esp + `+blancs` + esp + `*:` + esp + `*(` + nb + `)`)
	reNuls     = regexp.MustCompile(`(?i)Bulletins` + esp + `+nuls` + esp + `*:` + esp + `*(` + nb + `)`)
	reExprimes = regexp.MustCompile(`(?i)Suffrages` + esp + `+exprimés` + esp + `*:` + esp + `*(` + nb + `)`)

	// Deux formulations coexistent dans la série. Jusqu'en 1981 : « Suffrages
	// obtenus par X : N ». À partir de 1988 : « Ont obtenu : M. X : N ».
	reObtenusPar = regexp.MustCompile(`(?i)Suffrages` + esp + `+obtenus` + esp + `+par` + esp + `+(?:M\.|Mme|Mlle|Monsieur|Madame)?` + esp + `*([^:]{2,60}?)` + esp + `*:` + esp + `*(` + nb + `)`)
	reOntObtenu  = regexp.MustCompile(`(?:M\.|Mme|Mlle|Monsieur|Madame)` + esp + `+([^:]{2,60}?)` + esp + `*:` + esp + `*(` + nb + `)`)

	reSpaces  = regexp.MustCompile(`[\s\x{00a0}\x{202f}]+`)
	reNonDigi = regexp.MustCompile(`[^\d]`)
)

type proclamation struct {
	annee, tour int
	url         string
}

type chiffres struct {
	inscrits, votants, exprimes int64
	blancs, nuls                *int64
	candidats                   []candidat
}

type candidat struct {
	nom  string
	voix int64
}

// Ingest lit ref.pdr_proclamation, scelle chaque décision, en extrait les
// chiffres et les charge. La table de référence pilote le connecteur : ajouter
// une élection, c'est ajouter une ligne, pas modifier du code.
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	srcID, err := arch.EnsureSource(ctx, SourceProclamation)
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

	rows, err := pool.Query(ctx,
		`SELECT annee, tour, decision_url FROM ref.pdr_proclamation ORDER BY annee, tour`)
	if err != nil {
		return fail(err)
	}
	var todo []proclamation
	for rows.Next() {
		var p proclamation
		if err := rows.Scan(&p.annee, &p.tour, &p.url); err != nil {
			rows.Close()
			return fail(err)
		}
		todo = append(todo, p)
	}
	rows.Close()
	if len(todo) == 0 {
		return fail(fmt.Errorf("ref.pdr_proclamation est vide : la migration 0037 n'a pas été appliquée"))
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback(ctx)

	var nVoix int
	for _, p := range todo {
		f, err := arch.Fetch(ctx, srcID, runID, p.url, ".html")
		if err != nil {
			return fail(fmt.Errorf("%d tour %d : %w", p.annee, p.tour, err))
		}
		raw, err := os.ReadFile(f.Path)
		if err != nil {
			return fail(err)
		}
		c, err := extraire(string(raw))
		if err != nil {
			return fail(fmt.Errorf("%d tour %d (%s) : %w", p.annee, p.tour, p.url, err))
		}
		if err := controler(p, c); err != nil {
			return fail(err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO core.pdr_resultat
			  (annee, tour, inscrits, votants, blancs, nuls, exprimes, source_id, document_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (annee, tour) DO UPDATE SET
			  inscrits = EXCLUDED.inscrits, votants = EXCLUDED.votants,
			  blancs = EXCLUDED.blancs, nuls = EXCLUDED.nuls,
			  exprimes = EXCLUDED.exprimes, document_id = EXCLUDED.document_id`,
			p.annee, p.tour, c.inscrits, c.votants, c.blancs, c.nuls, c.exprimes,
			srcID, f.DocumentID); err != nil {
			return fail(fmt.Errorf("%d : %w", p.annee, err))
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM core.pdr_voix WHERE annee = $1 AND tour = $2`, p.annee, p.tour); err != nil {
			return fail(err)
		}
		// L'élu est celui qui a le plus de voix — et le contrôle ci-dessus a
		// déjà vérifié qu'il dépasse la majorité absolue des exprimés.
		best := 0
		for i, k := range c.candidats {
			if k.voix > c.candidats[best].voix {
				best = i
			}
		}
		for i, k := range c.candidats {
			if _, err := tx.Exec(ctx, `
				INSERT INTO core.pdr_voix (annee, tour, candidat, voix, elu)
				VALUES ($1,$2,$3,$4,$5)`, p.annee, p.tour, k.nom, k.voix, i == best); err != nil {
				return fail(fmt.Errorf("%d, %s : %w", p.annee, k.nom, err))
			}
			nVoix++
		}
		fmt.Printf("  %d tour %d : %d inscrits, %d exprimés, %d candidats\n",
			p.annee, p.tour, c.inscrits, c.exprimes, len(c.candidats))
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(err)
	}
	arch.EndRun(ctx, runID, "SUCCESS",
		map[string]any{"proclamations": len(todo), "lignes_de_voix": nVoix}, "")
	return nil
}

// extraire réduit la page à du texte, se place sur le bloc de résultats, et en
// tire les chiffres. L'ancrage sur « Électeurs inscrits : » avec les deux-points
// n'est pas un détail : plusieurs décisions discutent par ailleurs du « nombre
// d'électeurs inscrits » d'une commune contestée, sans deux-points. Sans cette
// ancre, on chargerait les chiffres d'un bureau de vote annulé.
func extraire(page string) (chiffres, error) {
	var c chiffres
	// Le texte de la page passe par l'analyseur lexical de internal/balisage,
	// pas par un motif : une décision du Conseil constitutionnel se lit
	// entièrement ou pas du tout.
	t := balisage.Ligne(page)
	t = html.UnescapeString(t)
	t = reSpaces.ReplaceAllString(t, " ")

	loc := reInscrits.FindStringIndex(t)
	if loc == nil {
		return c, fmt.Errorf("bloc de résultats introuvable")
	}
	fin := loc[0] + 1400
	if fin > len(t) {
		fin = len(t)
	}
	bloc := t[loc[0]:fin]

	var err error
	if c.inscrits, err = nombre(reInscrits, bloc, "électeurs inscrits"); err != nil {
		return c, err
	}
	// On cherche « Votants » après les inscrits, sans quoi le mot pourrait être
	// capté dans une phrase antérieure.
	if c.votants, err = nombre(reVotants, bloc, "votants"); err != nil {
		return c, err
	}
	if c.exprimes, err = nombre(reExprimes, bloc, "suffrages exprimés"); err != nil {
		return c, err
	}
	if v, err := nombre(reBlancs, bloc, "blancs"); err == nil {
		c.blancs = &v
	}
	if v, err := nombre(reNuls, bloc, "nuls"); err == nil {
		c.nuls = &v
	}

	for _, m := range reObtenusPar.FindAllStringSubmatch(bloc, -1) {
		c.candidats = append(c.candidats, candidat{nom: nettoyerNom(m[1]), voix: entier(m[2])})
	}
	if len(c.candidats) == 0 {
		i := strings.Index(bloc, "Ont obtenu")
		if i < 0 {
			return c, fmt.Errorf("aucun candidat trouvé")
		}
		reste := bloc[i:]
		for _, fin := range []string{"Qu'ainsi", "Ainsi,", "En conséquence"} {
			if j := strings.Index(reste, fin); j > 0 {
				reste = reste[:j]
			}
		}
		for _, m := range reOntObtenu.FindAllStringSubmatch(reste, -1) {
			c.candidats = append(c.candidats, candidat{nom: nettoyerNom(m[1]), voix: entier(m[2])})
		}
	}
	sort.Slice(c.candidats, func(i, j int) bool { return c.candidats[i].voix > c.candidats[j].voix })
	return c, nil
}

// controler refuse tout ce qui ne boucle pas. Un connecteur qui charge une
// transcription incohérente est pire qu'un connecteur qui échoue : l'erreur
// devient une donnée, et la donnée devient une citation.
func controler(p proclamation, c chiffres) error {
	ctx := fmt.Sprintf("%d tour %d", p.annee, p.tour)
	if c.votants > c.inscrits {
		return fmt.Errorf("%s : %d votants pour %d inscrits", ctx, c.votants, c.inscrits)
	}
	if c.exprimes > c.votants {
		return fmt.Errorf("%s : %d exprimés pour %d votants", ctx, c.exprimes, c.votants)
	}
	if p.tour == 2 && len(c.candidats) != 2 {
		return fmt.Errorf("%s : %d candidats au second tour, attendu 2 (%v)", ctx, len(c.candidats), c.candidats)
	}
	var somme int64
	var max int64
	for _, k := range c.candidats {
		somme += k.voix
		if k.voix > max {
			max = k.voix
		}
	}
	if somme != c.exprimes {
		return fmt.Errorf("%s : somme des voix %d ≠ suffrages exprimés %d", ctx, somme, c.exprimes)
	}
	if p.tour == 2 && max*2 <= c.exprimes {
		return fmt.Errorf("%s : aucun candidat n'atteint la majorité absolue", ctx)
	}
	if c.blancs != nil && c.nuls != nil && *c.blancs+*c.nuls != c.votants-c.exprimes {
		return fmt.Errorf("%s : blancs %d + nuls %d ≠ votants - exprimés %d",
			ctx, *c.blancs, *c.nuls, c.votants-c.exprimes)
	}
	return nil
}

func nombre(re *regexp.Regexp, s, quoi string) (int64, error) {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("%s : introuvable", quoi)
	}
	v := entier(m[1])
	if v == 0 {
		return 0, fmt.Errorf("%s : valeur illisible %q", quoi, m[1])
	}
	return v, nil
}

func entier(s string) int64 {
	s = reNonDigi.ReplaceAllString(s, "")
	var n int64
	for _, r := range s {
		n = n*10 + int64(r-'0')
	}
	return n
}

func nettoyerNom(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "Monsieur ")
	s = strings.TrimPrefix(s, "Madame ")
	s = strings.TrimPrefix(s, "M. ")
	s = strings.TrimPrefix(s, "Mme ")
	s = strings.TrimPrefix(s, "Mlle ")
	return strings.TrimSpace(s)
}
