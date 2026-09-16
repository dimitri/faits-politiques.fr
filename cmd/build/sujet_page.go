package main

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// La page de sujet, telle que le lecteur la voit.
//
// Le dossier au plan commun (docs/<slug>.md) est un document de travail : dense,
// écrit pour être relu et vérifié. Le rendre tel quel donnait une page de texte
// continue — les faits en puces, les crédits en tableau, les graphiques du
// sujet restés sur une autre page. Ici la page se construit en couches :
//
//  1. « En bref » : trois ou quatre chiffres clés datés et sourcés ;
//  2. « Les chiffres » : les graphiques et cartes de la page de données du
//     sujet quand elle existe (collectivités, chômage, budget…), repris tels
//     quels ;
//  3. le dossier, section par section, où les parties tirées de la base sont
//     redessinées : les faits en frise avec leur qualité, les crédits et les
//     mentions en séance en barres à l'échelle ;
//  4. glossaire, sources, annexe technique et versions, repliés.

type ChiffreCle struct{ Valeur, Libelle, Source string }

type SectionSujet struct {
	ID, Titre      string
	HTML           template.HTML
	Encart, Replie bool
}

// ── Les faits des dossiers, depuis ref.fait_dossier ─────────────────────

type faitSujet struct {
	section, auteur, intitule, constat, url, page, qualite string
	annee, dateFr                                          string
	nom, slug                                              string
	fiche, seance                                          bool
}

func chargerFaitsSujets(ctx context.Context, pool *pgxpool.Pool) (map[string][]faitSujet, error) {
	rows, err := pool.Query(ctx, `
		SELECT f.dossier, f.section, f.auteur, f.intitule, f.constat, f.source_url, coalesce(f.page,''),
		       f.qualite, f.date_fait, coalesce(p.nom,''), coalesce(p.slug,''), coalesce(p.a_une_fiche,false),
		       f.intervention_slug IS NOT NULL
		FROM ref.fait_dossier f
		LEFT JOIN derived.fait_dossier_personne p ON p.fait_id = f.id
		ORDER BY f.dossier, f.section, f.date_fait NULLS LAST, f.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]faitSujet{}
	for rows.Next() {
		var d string
		var f faitSujet
		var date *time.Time
		if err := rows.Scan(&d, &f.section, &f.auteur, &f.intitule, &f.constat, &f.url, &f.page,
			&f.qualite, &date, &f.nom, &f.slug, &f.fiche, &f.seance); err != nil {
			return nil, err
		}
		if date != nil {
			f.annee = strconv.Itoa(date.Year())
			f.dateFr = dateFr(*date)
		}
		out[d] = append(out[d], f)
	}
	return out, rows.Err()
}

var libQualite = map[string][2]string{
	"OFFICIEL":   {"officiel", "q-off"},
	"DECLARATIF": {"déclaratif", "q-decl"},
	"PRESSE":     {"presse", "q-presse"},
}

// frise : les faits d'une section, un par ligne, l'année en marge.
func frise(faits []faitSujet, section, root string) string {
	var b strings.Builder
	n := 0
	for _, f := range faits {
		if f.section != section {
			continue
		}
		if n == 0 {
			b.WriteString(`<ol class="faits-frise">`)
		}
		n++
		e := template.HTMLEscapeString
		q := libQualite[f.qualite]
		qui := e(f.auteur)
		if f.nom != "" {
			nom := e(f.nom)
			if f.fiche {
				nom = `<a href="` + root + `/depute/` + e(f.slug) + `/">` + nom + `</a>`
			}
			qui = nom + ", " + e(f.auteur)
		}
		preuve := `<a href="` + e(f.url) + `">source</a>`
		if f.seance {
			preuve = "compte rendu de séance de l'Assemblée nationale"
			if f.dateFr != "" {
				preuve += " du " + f.dateFr
			}
		} else if f.page != "" {
			preuve += ", p.&nbsp;" + e(f.page)
		}
		fmt.Fprintf(&b, `<li><span class="f-date">%s</span><div><p class="f-t">%s</p><p class="f-c">%s</p>`+
			`<p class="f-src"><span class="qual %s">%s</span> %s · %s</p></div></li>`,
			e(f.annee), e(f.intitule), e(f.constat), q[1], q[0], qui, preuve)
	}
	if n > 0 {
		b.WriteString(`</ol>`)
	}
	return b.String()
}

// ── Les crédits par programme, en barres ────────────────────────────────

type ligneCredit struct {
	mission, code, libelle string
	exercice               int
	cp                     float64
}

func chargerCredits(ctx context.Context, pool *pgxpool.Pool) (map[string][]ligneCredit, error) {
	rows, err := pool.Query(ctx, `
		SELECT dossier, mission, programme_code, programme_libelle, exercice, coalesce(credit_paiement,0)::float8
		FROM derived.dossier_budget_programme ORDER BY dossier, mission, programme_code, exercice`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]ligneCredit{}
	for rows.Next() {
		var d string
		var l ligneCredit
		if err := rows.Scan(&d, &l.mission, &l.code, &l.libelle, &l.exercice, &l.cp); err != nil {
			return nil, err
		}
		out[d] = append(out[d], l)
	}
	return out, rows.Err()
}

// barresCredits : pour chaque mission, les programmes du dernier exercice
// chargé, à l'échelle du plus gros, avec l'écart au précédent.
func barresCredits(lignes []ligneCredit) string {
	if len(lignes) == 0 {
		return ""
	}
	var exs []int
	vus := map[int]bool{}
	var missions []string
	mvus := map[string]bool{}
	for _, l := range lignes {
		if !vus[l.exercice] {
			vus[l.exercice] = true
			exs = append(exs, l.exercice)
		}
		if !mvus[l.mission] {
			mvus[l.mission] = true
			missions = append(missions, l.mission)
		}
	}
	sort.Ints(exs)
	der := exs[len(exs)-1]
	var b strings.Builder
	for _, m := range missions {
		type prog struct {
			lib      string
			cp, prec float64
			aPrec    bool
		}
		progs := map[string]*prog{}
		var codes []string
		total, totalPrec := 0.0, 0.0
		for _, l := range lignes {
			if l.mission != m {
				continue
			}
			p := progs[l.code]
			if p == nil {
				p = &prog{}
				progs[l.code] = p
				codes = append(codes, l.code)
			}
			if l.exercice == der {
				p.cp += l.cp
				p.lib = l.libelle
				total += l.cp
			} else if len(exs) > 1 && l.exercice == exs[len(exs)-2] {
				p.prec += l.cp
				p.aPrec = true
				totalPrec += l.cp
			}
		}
		sort.Slice(codes, func(i, j int) bool { return progs[codes[i]].cp > progs[codes[j]].cp })
		max := 0.0
		for _, c := range codes {
			if progs[c].cp > max {
				max = progs[c].cp
			}
		}
		evol := func(a, z float64, ok bool) string {
			if !ok || a == 0 {
				return ""
			}
			v := 100 * (z - a) / a
			s := Decimal(v, 1)
			if v >= 0 {
				s = "+" + s
			}
			return strings.Replace(s, "-", "−", 1) + "&nbsp;%"
		}
		fmt.Fprintf(&b, `<figure class="credits"><figcaption><strong>Mission « %s »</strong> : %s&nbsp;Md€ demandés au projet de loi de finances %d`,
			template.HTMLEscapeString(m), Decimal(total/1e9, 2), der)
		if e := evol(totalPrec, total, totalPrec > 0); e != "" {
			fmt.Fprintf(&b, ` <span class="evol">(%s par rapport au projet %d)</span>`, e, exs[len(exs)-2])
		}
		b.WriteString(`</figcaption><div class="barres-h">`)
		for _, c := range codes {
			p := progs[c]
			if p.cp <= 0 {
				continue
			}
			fmt.Fprintf(&b, `<div class="barre"><span class="l">%s</span><span class="b" aria-hidden="true"><i style="width:%.1f%%"></i></span>`+
				`<span class="v">%s&nbsp;M€</span><span class="e">%s</span></div>`,
				template.HTMLEscapeString(p.lib), 100*p.cp/max, Nombre(int(p.cp/1e6+0.5)), evol(p.prec, p.cp, p.aPrec))
		}
		b.WriteString(`</div></figure>`)
	}
	b.WriteString(`<p class="src-bloc">Crédits de paiement <strong>demandés</strong> aux projets de loi de finances, ni votés ni exécutés ; les crédits de personnel incluent les cotisations de retraite des fonctionnaires. Source : Direction du budget.</p>`)
	return b.String()
}

// ── Les mentions en séance : le tableau généré devient des barres ───────

var reLigneMention = regexp.MustCompile(`(?s)<tr>\s*<td>(.*?)</td>\s*<td[^>]*>([\d\s\x{202f}\x{a0}]+)</td>\s*<td[^>]*>([\d\s\x{202f}\x{a0}]+)</td>\s*<td>(.*?)</td>\s*<td>(.*?)</td>\s*</tr>`)
var reIntroMention = regexp.MustCompile(`(?s)<p>(Dans les comptes rendus.*?)</p>`)

func barresMentions(bloc string) string {
	lignes := reLigneMention.FindAllStringSubmatch(bloc, -1)
	if len(lignes) == 0 {
		return bloc
	}
	entier := func(s string) int {
		s = strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, s)
		n, _ := strconv.Atoi(s)
		return n
	}
	max := 0
	for _, l := range lignes {
		if n := entier(l[2]); n > max {
			max = n
		}
	}
	var b strings.Builder
	if m := reIntroMention.FindStringSubmatch(bloc); m != nil {
		b.WriteString(`<p class="intro-mentions">` + m[1] + `</p>`)
	}
	b.WriteString(`<div class="barres-h mentions">`)
	for _, l := range lignes {
		n := entier(l[2])
		fmt.Fprintf(&b, `<div class="barre"><span class="l">« %s »</span><span class="b" aria-hidden="true"><i style="width:%.1f%%"></i></span>`+
			`<span class="v">%s</span><span class="e">%s orateurs</span></div>`,
			l[1], 100*float64(n)/float64(max), Nombre(n), Nombre(entier(l[3])))
	}
	b.WriteString(`</div><p class="src-bloc">Interventions en séance publique de l'Assemblée nationale qui emploient ces mots. Une mention ne dit pas la position de l'orateur.</p>`)
	return b.String()
}

// ── Le découpage du dossier ──────────────────────────────────────────────

var (
	reH1Doc       = regexp.MustCompile(`(?s)^\s*<h1[^>]*>.*?</h1>`)
	reBlockquote  = regexp.MustCompile(`(?s)<blockquote>(.*?)</blockquote>`)
	reParagraphes = regexp.MustCompile(`(?s)<p>(.*?)</p>`)
	reBlocGenere  = regexp.MustCompile(`(?s)<!-- faits:([A-Z]+):debut[^>]*-->(.*?)<!-- faits:[A-Z]+:fin -->`)
	reH2Doc       = regexp.MustCompile(`<h2 id="([^"]+)">(.*?)</h2>`)
)

var sectionsRepliees = map[string]bool{"Glossaire": true, "Sources": true, "Annexe technique": true, "Versions": true}
var sectionsEncart = map[string]bool{"Ce que les données ne disent pas": true, "Pièges de lecture": true}

func decouperDossier(s *Sujet, faits []faitSujet, credits []ligneCredit, root string) {
	corps := reH1Doc.ReplaceAllString(string(s.D.Corps), "")
	if m := reBlockquote.FindStringSubmatchIndex(corps); m != nil {
		ps := reParagraphes.FindAllStringSubmatch(corps[m[2]:m[3]], -1)
		var chap []string
		for _, p := range ps {
			t := strings.TrimSpace(p[1])
			if strings.HasPrefix(t, "<strong>Dossier</strong>") || strings.HasPrefix(t, "<strong>Méthode</strong>") {
				s.Version = reBalises.ReplaceAllString(t, "")
				continue
			}
			chap = append(chap, t)
		}
		s.Chapeau = template.HTML(strings.Join(chap, " "))
		corps = corps[:m[0]] + corps[m[1]:]
	}
	corps = strings.Replace(corps, "<hr>", "", 1)

	// Les blocs générés sont redessinés depuis la base.
	corps = reBlocGenere.ReplaceAllStringFunc(corps, func(bloc string) string {
		m := reBlocGenere.FindStringSubmatch(bloc)
		switch m[1] {
		case "CONTEXTE":
			return barresMentions(m[2]) + frise(faits, "CONTEXTE", root)
		case "BUDGET":
			return barresCredits(credits)
		case "ENJEUX", "CADRE", "CONTROLE", "SITUATION":
			if f := frise(faits, m[1], root); f != "" {
				return f
			}
			return ""
		}
		return m[2]
	})

	idx := reH2Doc.FindAllStringSubmatchIndex(corps, -1)
	for i, m := range idx {
		fin := len(corps)
		if i+1 < len(idx) {
			fin = idx[i+1][0]
		}
		titre := strings.TrimSpace(reBalises.ReplaceAllString(corps[m[4]:m[5]], ""))
		contenu := strings.TrimSpace(corps[m[1]:fin])
		if reBalises.ReplaceAllString(contenu, "") == "" && !strings.Contains(contenu, "<svg") {
			continue
		}
		s.Sections = append(s.Sections, SectionSujet{
			ID: corps[m[2]:m[3]], Titre: html.UnescapeString(titre), HTML: template.HTML(contenu),
			Encart: sectionsEncart[titre], Replie: sectionsRepliees[titre],
		})
	}
	for _, f := range faits {
		switch f.section {
		case "CADRE":
			s.NbCadre++
		case "CONTROLE":
			s.NbControle++
		}
	}
}

// ── La page de données du sujet, reprise dans la page de sujet ──────────

var (
	reMain = regexp.MustCompile(`(?s)<main class="wrap" id="contenu">(.*)</main>`)
	reFil  = regexp.MustCompile(`(?s)^\s*<p class="fil">.*?</p>`)
	reH1   = regexp.MustCompile(`(?s)<h1([^>]*)>(.*?)</h1>`)
)

// donneesDePage lit une page déjà écrite et en extrait le contenu, sans fil
// d'Ariane, titre principal rétrogradé. Une page absente est simplement omise.
func donneesDePage(out, url string) template.HTML {
	src, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(url), "index.html"))
	if err != nil {
		return ""
	}
	m := reMain.FindSubmatch(src)
	if m == nil {
		return ""
	}
	c := reFil.ReplaceAllString(string(m[1]), "")
	c = reH1.ReplaceAllString(c, `<h2 class="h2"$1>$2</h2>`)
	return template.HTML(c)
}

// donneesCollectivites reprend, pour la page de sujet, une sélection de
// /collectivites/ plutôt que la page entière (D-075) : les trois cartes de
// « qui dépense où » deviennent des onglets — une seule à l'écran à la fois,
// comme sur /collectivites/departement/<code>/ — et les tableaux exhaustifs
// (103 départements, 9 000 groupements, les cartes commune par commune)
// restent sur /collectivites/, avec un lien pour les y retrouver plutôt
// qu'une seconde copie sur la page de sujet.
var reH2Any = regexp.MustCompile(`(?s)<h2[^>]*>(.*?)</h2>`)

func donneesCollectivites(out, root string) template.HTML {
	src, err := os.ReadFile(filepath.Join(out, "collectivites", "index.html"))
	if err != nil {
		return ""
	}
	m := reMain.FindSubmatch(src)
	if m == nil {
		return ""
	}
	corps := reFil.ReplaceAllString(string(m[1]), "")
	corps = reH1.ReplaceAllString(corps, `<h2 class="h2"$1>$2</h2>`)

	idx := reH2Any.FindAllStringSubmatchIndex(corps, -1)
	if len(idx) == 0 {
		return template.HTML(corps)
	}
	avant := corps[:idx[0][0]] // la note « aucun président élu directement », avant le premier h2
	sections := map[string]string{}
	for i, m := range idx {
		fin := len(corps)
		if i+1 < len(idx) {
			fin = idx[i+1][0]
		}
		titre := strings.TrimSpace(reBalises.ReplaceAllString(corps[m[2]:m[3]], ""))
		sections[titre] = corps[m[0]:fin]
	}

	var b strings.Builder
	b.WriteString(avant)

	// Les trois cartes, en onglets plutôt qu'empilées : un seul territoire à
	// la fois, comme sur une page de département.
	if trois, ok := sections["Trois niveaux, trois cartes"]; ok {
		b.WriteString(onglezCartesNiveaux(trois))
	}
	for _, titre := range []string{"Qui dépense quoi, en " + anneeDe(sections), "D'où vient l'argent"} {
		if sec, ok := sections[titre]; ok {
			b.WriteString(sec)
		}
	}
	if regions, ok := regionsAvecTitre(sections); ok {
		b.WriteString(regions)
	}
	fmt.Fprintf(&b, `<p class="q">Chaque département, chaque groupement de communes, et les cartes `+
		`commune par commune, sujet par sujet&nbsp;: <a href="%s/collectivites/">toutes les données `+
		`des collectivités →</a></p>`, root)
	return template.HTML(b.String())
}

// anneeDe retrouve l'année de l'exercice depuis le titre « Qui dépense quoi,
// en 2025 » déjà présent dans les sections extraites, plutôt que de la
// recalculer : une seule source pour ce chiffre.
func anneeDe(sections map[string]string) string {
	for titre := range sections {
		if strings.HasPrefix(titre, "Qui dépense quoi, en ") {
			return strings.TrimPrefix(titre, "Qui dépense quoi, en ")
		}
	}
	return ""
}

func regionsAvecTitre(sections map[string]string) (string, bool) {
	for titre, sec := range sections {
		if strings.HasPrefix(titre, "Les ") && strings.HasSuffix(titre, " régions") {
			return sec, true
		}
	}
	return "", false
}

// onglezCartesNiveaux transforme les trois cartes empilées (régions,
// départements, intercommunalités) de /collectivites/ en trois onglets — le
// même composant, sans script, que les cartes de l'accueil. Les balises div
// sont comptées plutôt que bornées par une expression régulière : chaque
// carte imbrique elle-même l'échelle et les cartons d'outre-mer dans leurs
// propres <div>, à une profondeur qu'une regex ne borne pas de façon fiable.
func onglezCartesNiveaux(section string) string {
	titre := reH2Any.FindString(section)
	corps := section[len(titre):]

	debutEnv := strings.Index(corps, `<div class="cartes-empilees">`)
	if debutEnv < 0 {
		return section
	}
	finEnv, ok := finDiv(corps, debutEnv)
	if !ok {
		return section
	}
	interieur := corps[debutEnv+len(`<div class="cartes-empilees">`) : finEnv]
	avant, apres := corps[:debutEnv], corps[finEnv+len("</div>"):]

	var cartes []string
	reste := interieur
	for {
		d := strings.Index(reste, `<div class="bloc-carte ligne"`)
		if d < 0 {
			break
		}
		f, ok := finDiv(reste, d)
		if !ok {
			return section
		}
		cartes = append(cartes, reste[d:f+len("</div>")])
		reste = reste[f+len("</div>"):]
	}
	if len(cartes) != 3 {
		return section // la mise en page de /collectivites/ a changé : mieux vaut la page complète qu'une carte perdue
	}

	// L'ordre suit celui des .bloc-carte dans collectivites.gohtml : EPCI en
	// premier, le sujet réel de cette page (élection indirecte), pas les
	// régions par habitude de tri administratif.
	libelles := []string{"Intercommunalités", "Régions", "Départements"}
	var b strings.Builder
	b.WriteString(titre)
	b.WriteString(avant)
	b.WriteString(`<div class="onglets-carte">`)
	for i := range cartes {
		checked := ""
		if i == 0 {
			checked = " checked"
		}
		fmt.Fprintf(&b, `<input type="radio" name="onglet-carte-niveau" id="ocn%d" class="vh"%s>`, i+1, checked)
	}
	b.WriteString(`<div class="etiquettes" role="presentation">`)
	for i, l := range libelles {
		fmt.Fprintf(&b, `<label for="ocn%d">%s</label>`, i+1, l)
	}
	b.WriteString(`</div><div class="panneaux">`)
	for i, c := range cartes {
		b.WriteString(reBlocCarteClasse.ReplaceAllString(c, `<div class="bloc-carte ligne p`+fmt.Sprint(i+1)+`">`))
	}
	b.WriteString(`</div></div>`)
	b.WriteString(apres)
	return b.String()
}

var reBlocCarteClasse = regexp.MustCompile(`^<div class="bloc-carte ligne"[^>]*>`)

// finDiv trouve, pour un <div ...> qui commence à l'indice debut, l'indice de
// son </div> correspondant — en comptant les ouvertures et fermetures
// imbriquées, pas en s'arrêtant à la première rencontrée.
func finDiv(s string, debut int) (int, bool) {
	depth := 0
	i := debut
	for i < len(s) {
		o := strings.Index(s[i:], "<div")
		c := strings.Index(s[i:], "</div>")
		if c < 0 {
			return 0, false
		}
		if o >= 0 && o < c {
			depth++
			i += o + len("<div")
			continue
		}
		depth--
		if depth == 0 {
			return i + c, true
		}
		i += c + len("</div>")
	}
	return 0, false
}

// ── En bref : les chiffres clés, curatés par sujet, lus dans la base ────

func enBref(ctx context.Context, pool *pgxpool.Pool, s *Sujet, acc *DonneesAccueil, credits []ligneCredit) []ChiffreCle {
	var out []ChiffreCle
	ajouter := func(c ChiffreCle, ok bool) {
		if ok && c.Valeur != "" {
			out = append(out, c)
		}
	}
	macro := func(code, libelle, source string, format func(float64) string) {
		var annee int
		var v float64
		if err := pool.QueryRow(ctx, `SELECT annee, valeur::float8 FROM core.macro_value WHERE serie_code=$1 ORDER BY annee DESC LIMIT 1`, code).Scan(&annee, &v); err != nil {
			return
		}
		ajouter(ChiffreCle{format(v), libelle, fmt.Sprintf("%d · %s", annee, source)}, true)
	}
	requete := func(q, libelle, source string, format func(float64) string) {
		var annee int
		var v float64
		if err := pool.QueryRow(ctx, q).Scan(&annee, &v); err != nil {
			return
		}
		ajouter(ChiffreCle{format(v), libelle, fmt.Sprintf("%d · %s", annee, source)}, true)
	}
	cofog := func(code string) {
		for _, f := range acc.Fonctions {
			if f.Code == code {
				ajouter(ChiffreCle{Decimal(f.Milliards, 1) + "\u00a0Md€", "de dépense publique pour la fonction « " + strings.ToLower(f.Libelle[:1]) + f.Libelle[1:] + " », soit " + strconv.Itoa(f.ParMille) + "\u00a0€ sur 1\u00a0000",
					fmt.Sprintf("%d · Eurostat / Insee, toutes administrations", acc.Annee)}, true)
			}
		}
	}
	mission := func(libelle string) {
		requete(`SELECT exercice, sum(credit_paiement)::float8 FROM core.budget_programme
			WHERE mission_libelle = '`+strings.ReplaceAll(libelle, "'", "''")+`' GROUP BY exercice ORDER BY exercice DESC LIMIT 1`,
			"crédits demandés pour la mission « "+libelle+" »", "projet de loi de finances, Direction du budget",
			func(v float64) string { return Decimal(v/1e9, 2) + "\u00a0Md€" })
	}
	credits0 := func() {
		if len(credits) == 0 {
			return
		}
		ex := 0
		for _, l := range credits {
			if l.exercice > ex {
				ex = l.exercice
			}
		}
		tot := 0.0
		for _, l := range credits {
			if l.exercice == ex {
				tot += l.cp
			}
		}
		ajouter(ChiffreCle{Decimal(tot/1e9, 2) + "\u00a0Md€", "de crédits demandés pour les missions du sujet",
			fmt.Sprintf("%d · projet de loi de finances, Direction du budget", ex)}, true)
	}
	sousSecteur := func(code, libelle string) {
		requete(`SELECT annee, depenses_meur::float8 FROM derived.budget_sous_secteur WHERE secteur='`+code+`' AND depenses_meur IS NOT NULL ORDER BY annee DESC LIMIT 1`,
			libelle, "Eurostat, comptes des administrations publiques", func(v float64) string { return Decimal(v/1000, 1) + "\u00a0Md€" })
	}
	pct := func(v float64) string { return Decimal(v, 1) + "\u00a0%" }
	md := func(v float64) string { return Nombre(int(v/1000+0.5)) + "\u00a0Md€" }

	switch s.ID {
	case "retraites":
		macro("protection.depense.vieillesse", "de prestations vieillesse", "Eurostat, ESSPROS", md)
		requete(`SELECT annee, age_ensemble::float8 FROM core.age_depart_retraite ORDER BY annee DESC LIMIT 1`, "ans : âge conjoncturel moyen de départ", "Drees", func(v float64) string { return Decimal(v, 1) })
		requete(`SELECT annee, ratio_demographique::float8 FROM core.cotisants_retraites_ratio ORDER BY annee DESC LIMIT 1`, "cotisants par retraité", "Insee, tous régimes", func(v float64) string { return Decimal(v, 2) })
	case "sante":
		cofog("GF07")
	case "chomage":
		macro("chomage.taux", "taux de chômage", "Eurostat, au sens du BIT", pct)
		macro("chomeurs.nombre", "chômeurs", "Eurostat", func(v float64) string {
			return Decimal(v/1000, 2) + "\u00a0million" + map[bool]string{true: "s"}[v >= 2000]
		})
		macro("protection.depense.chomage", "de dépense de protection sociale au titre du chômage", "Eurostat, ESSPROS", md)
	case "pauvrete":
		macro("pauvrete.taux", "taux de pauvreté", "Eurostat, seuil à 60 % du revenu médian", pct)
		macro("pauvrete.nombre", "personnes sous le seuil de pauvreté", "Eurostat", func(v float64) string { return Decimal(v/1000, 1) + "\u00a0millions" })
		macro("rsa.foyers", "foyers allocataires du RSA", "Cnaf / Drees", func(v float64) string { return Nombre(int(v + 0.5)) })
	case "securite-sociale":
		sousSecteur("S1314", "de dépenses des administrations de sécurité sociale")
		requete(`SELECT annee, solde_meur::float8 FROM derived.budget_sous_secteur WHERE secteur='S1314' AND solde_meur IS NOT NULL ORDER BY annee DESC LIMIT 1`,
			"de solde des administrations de sécurité sociale", "Eurostat", func(v float64) string { return Decimal(v/1000, 1) + "\u00a0Md€" })
	case "cotisations":
		macro("protection.financement.cotisations.employeurs", "de cotisations des employeurs dans le financement de la protection sociale", "Eurostat, ESSPROS", md)
		macro("protection.financement.cotisations.protegees", "de cotisations des assurés", "Eurostat, ESSPROS", md)
	case "education":
		cofog("GF09")
		mission("Enseignement scolaire")
		requete(`SELECT annee, sum(nombre_eleves)::float8 FROM core.education_effectif_eleves GROUP BY annee ORDER BY annee DESC LIMIT 1`, "élèves dans le premier degré", "Depp", func(v float64) string { return Nombre(int(v + 0.5)) })
	case "police", "justice":
		cofog("GF03")
		if s.ID == "justice" {
			credits0()
		}
		mission("Sécurités")
	case "defense":
		cofog("GF02")
		mission("Défense")
	case "ecologie":
		cofog("GF05")
		credits0()
		mission("Écologie, développement et mobilité durables")
	case "logement":
		cofog("GF06")
		credits0()
	case "culture":
		cofog("GF08")
		credits0()
	case "collectivites":
		sousSecteur("S1313", "de dépenses des administrations publiques locales")
		requete(`SELECT extract(year from now())::int, count(DISTINCT commune_code)::float8 FROM core.commune_indicator`, "communes couvertes par les comptes chargés", "OFGL / DGCL", func(v float64) string { return Nombre(int(v)) })
	case "immigration":
		requete(`SELECT annee, sum(effectif)::float8 FROM core.titre_sejour_stock WHERE annee=(SELECT max(annee) FROM core.titre_sejour_stock) GROUP BY annee`, "titres de séjour valides au 31 décembre", "DGEF, ministère de l'Intérieur", func(v float64) string { return Nombre(int(v + 0.5)) })
		requete(`SELECT annee, premiere_demande::float8 FROM core.demande_asile_ofpra WHERE niveau='TOTAL' ORDER BY annee DESC LIMIT 1`, "premières demandes d'asile", "Ofpra", func(v float64) string { return Nombre(int(v + 0.5)) })
	case "violences-policieres":
		requete(`SELECT max(extract(year from date_arret))::int, count(*)::float8 FROM core.cedh_arret`, "arrêts de la Cour européenne des droits de l'homme concernant la France", "CEDH, base HUDOC", func(v float64) string { return Nombre(int(v)) })
	case "souverainete-numerique":
		requete(`SELECT extract(year from max(catalogue_du))::int, count(*)::float8 FROM core.qualification_secnumcloud WHERE catalogue_du=(SELECT max(catalogue_du) FROM core.qualification_secnumcloud)`, "services Cloud qualifiés SecNumCloud", "ANSSI", func(v float64) string { return Nombre(int(v)) })
		requete(`SELECT extract(year from now())::int, count(*)::float8 FROM core.marche_numerique`, "marchés publics informatiques recensés depuis 2018", "données essentielles de la commande publique", func(v float64) string { return Nombre(int(v)) })
	case "budget":
		sousSecteur("S1311", "de dépenses de l'administration centrale (État)")
		macro("solde.public.pib", "de solde public rapporté au PIB", "Eurostat", pct)
	case "dette":
		macro("dette.publique.meur", "de dette publique", "Eurostat", md)
		macro("dette.publique.pib", "de dette rapportée au PIB", "Eurostat", pct)
	case "pouvoirs-publics":
		mission("Pouvoirs publics")
	default:
		credits0()
	}
	return out
}

// preparerSujets complète chaque sujet : chiffres clés, page de données,
// dossier découpé. Appelé après l'écriture des pages de données.
func preparerSujets(ctx context.Context, pool *pgxpool.Pool, out, root string, acc *DonneesAccueil) error {
	faits, err := chargerFaitsSujets(ctx, pool)
	if err != nil {
		return err
	}
	credits, err := chargerCredits(ctx, pool)
	if err != nil {
		return err
	}
	for _, f := range familles {
		for _, s := range f.Sujets {
			if s.D == nil {
				continue
			}
			decouperDossier(s, faits[s.Doc], credits[s.Doc], root)
			s.EnBref = enBref(ctx, pool, s, acc, credits[s.Doc])
			switch {
			case s.ID == "collectivites":
				s.Donnees = donneesCollectivites(out, root)
			case len(s.Pages) > 0:
				s.Donnees = donneesDePage(out, s.Pages[0].URL)
			}
		}
	}
	return nil
}
