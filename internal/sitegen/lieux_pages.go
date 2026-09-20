package sitegen

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

// Strates de population : une médiane de dette par habitant « toutes communes »
// comparerait Paris à un village de cent habitants. On compare donc chaque
// commune à celles de sa taille, comme le fait l'OFGL.
var strates = []struct {
	max int
	lib string
}{
	{500, "moins de 500 habitants"}, {2000, "500 à 2 000 habitants"},
	{10000, "2 000 à 10 000 habitants"}, {50000, "10 000 à 50 000 habitants"},
	{1 << 30, "plus de 50 000 habitants"},
}

func strate(pop int) int {
	for i, s := range strates {
		if pop < s.max {
			return i
		}
	}
	return len(strates) - 1
}

var indicsCommune = []struct{ code, lib string }{
	{"ofgl.fonctionnement_par_hab", "Dépenses de fonctionnement"},
	{"ofgl.investissement_par_hab", "Dépenses d'investissement"},
	{"ofgl.dette_par_hab", "Encours de dette"},
	{"ofgl.epargne_brute_par_hab", "Épargne brute"},
	{"ofgl.masse_salariale_par_hab", "Charges de personnel"},
}

// chargerPagesCommunes fabrique les 34 875 pages en sept requêtes ensemblistes.
// Une requête par commune ferait 250 000 allers-retours et une construction de
// plusieurs heures.
func chargerPagesCommunes(ctx context.Context, pool *pgxpool.Pool, r *Resolveur,
	avecFiche map[string]bool) (map[string]*PageCommune, error) {

	pages := make(map[string]*PageCommune, len(r.communes))
	for code, c := range r.communes {
		p := &PageCommune{Code: code, Nom: c.Nom, Population: c.Population}
		if l, ok := r.lieuDept(c.Dept); ok {
			p.Dept = l
		}
		if l, ok := r.lieuRegion(c.Region); ok {
			p.Region = l
		}
		if s := r.epciDeCom[code]; s != "" {
			e := r.epci[s]
			p.EPCI = &Lieu{Type: "EPCI", Code: s, Nom: e.Nom, URL: r.urlEPCI(s)}
			p.NatureEPCI = libelleNature[e.Nature]
		}
		pages[code] = p
	}

	// Cinq requêtes indépendantes : chacune ne lit que sa propre ligne de
	// résultat et n'écrit que ses propres champs de *PageCommune (Maire/Elus,
	// Finances, Securite, Listes, Associations — jamais les mêmes que sa
	// voisine). pages lui-même n'est plus modifié après la boucle ci-dessus
	// (aucune clé ajoutée ou retirée) : des lectures concurrentes de pages[com]
	// depuis cinq buts sont donc sûres, et cinq allers-retours à Postgres qui
	// n'ont aucune raison d'attendre l'un après l'autre se recouvrent.
	g, gctx := errgroup.WithContext(ctx)

	// 1. Le conseil municipal en cours.
	g.Go(func() error {
		rows, err := pool.Query(gctx, `
			SELECT m.commune_code, p.given_name||' '||p.family_name, p.slug,
			       m.mandate_type::text, coalesce(m.role,''),
			       to_char(lower(m.validity),'DD/MM/YYYY')
			FROM core.mandate m JOIN core.person p ON p.id=m.person_id
			WHERE m.mandate_type IN ('MAIRE','CONSEILLER_MUNICIPAL')
			  AND m.commune_code IS NOT NULL AND upper(m.validity) IS NULL`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var com, nom, slug, typ, role, depuis string
			if err := rows.Scan(&com, &nom, &slug, &typ, &role, &depuis); err != nil {
				return err
			}
			p := pages[com]
			if p == nil {
				continue
			}
			e := EluCommune{Nom: NomPropre(nom), Slug: slug, Depuis: depuis, Fiche: avecFiche[slug]}
			if typ == "MAIRE" {
				e.Fonction, e.Ordre = "Maire", 0
				m := e
				p.Maire = &m
				continue // le maire figure aussi comme conseiller : une seule ligne
			}
			e.Fonction = role
			e.Ordre = rangFonction(role)
			if strings.EqualFold(role, "maire") {
				continue
			}
			p.Elus = append(p.Elus, e)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, p := range pages {
			p.NbConseillers = len(p.Elus)
			if p.Maire != nil {
				p.NbConseillers++
			}
			trierElus(p.Elus)
		}
		return nil
	})

	// 2. Finances : la dernière année publiée, indicateur par indicateur.
	g.Go(func() error {
		frows, err := pool.Query(gctx, `
			SELECT DISTINCT ON (commune_code, indicator_code)
			       commune_code, indicator_code, period_year, value::float8
			FROM core.commune_indicator
			WHERE indicator_code LIKE 'ofgl.%\_par\_hab'
			ORDER BY commune_code, indicator_code, period_year DESC`)
		if err != nil {
			return err
		}
		defer frows.Close()
		valeurs := map[string]map[string]float64{}
		parStrate := map[int]map[string][]float64{}
		for frows.Next() {
			var com, ind string
			var an int
			var v float64
			if err := frows.Scan(&com, &ind, &an, &v); err != nil {
				return err
			}
			p := pages[com]
			if p == nil {
				continue
			}
			if an > p.AnneeFinances {
				p.AnneeFinances = an
			}
			if valeurs[com] == nil {
				valeurs[com] = map[string]float64{}
			}
			valeurs[com][ind] = v
			s := strate(p.Population)
			if parStrate[s] == nil {
				parStrate[s] = map[string][]float64{}
			}
			parStrate[s][ind] = append(parStrate[s][ind], v)
		}
		if err := frows.Err(); err != nil {
			return err
		}
		medianes := map[int]map[string]float64{}
		for s, m := range parStrate {
			medianes[s] = map[string]float64{}
			for ind, vs := range m {
				sort.Float64s(vs)
				medianes[s][ind] = vs[len(vs)/2]
			}
		}
		for com, vals := range valeurs {
			p := pages[com]
			s := strate(p.Population)
			for _, ind := range indicsCommune {
				v, ok := vals[ind.code]
				if !ok {
					continue
				}
				p.Finances = append(p.Finances, IndicCommune{
					Libelle: ind.lib, Valeur: eurHab(v), Mediane: eurHab(medianes[s][ind.code]),
					Brut: v, BrutMediane: medianes[s][ind.code],
				})
			}
		}
		return nil
	})

	// 3. Sécurité : la dernière année, les quinze indicateurs.
	g.Go(func() error {
		srows, err := pool.Query(gctx, `
			SELECT commune_code, annee, indicateur_code, coalesce(nombre,0),
			       coalesce(taux_pour_mille,0)::float8, diffuse
			FROM core.commune_delinquance
			WHERE annee=(SELECT max(annee) FROM core.commune_delinquance)`)
		if err != nil {
			return err
		}
		defer srows.Close()
		for srows.Next() {
			var com, ind string
			var an, n int
			var taux float64
			var diff bool
			if err := srows.Scan(&com, &an, &ind, &n, &taux, &diff); err != nil {
				return err
			}
			p := pages[com]
			if p == nil {
				continue
			}
			p.AnneeSecurite = an
			lib := libelleSecurite[ind][0]
			if lib == "" {
				lib = ind
			}
			sc := SecuriteCommune{Libelle: lib, Masque: !diff}
			if diff {
				sc.Taux, sc.Nombre = Decimal(taux, 1)+" ‰", Nombre(n)
			}
			p.Securite = append(p.Securite, sc)
		}
		if err := srows.Err(); err != nil {
			return err
		}
		for _, p := range pages {
			sort.Slice(p.Securite, func(i, j int) bool {
				return CleTri(p.Securite[i].Libelle) < CleTri(p.Securite[j].Libelle)
			})
		}
		return nil
	})

	// 4. Municipales : le tour décisif, avec ses sièges, et le premier tour à
	// part quand il y en a eu deux. Les sièges ne sont publiés que sur la ligne
	// du tour qui les attribue : afficher le premier tour seul donnait « 0 siège »
	// à la liste élue.
	g.Go(func() error {
		lrows, err := pool.Query(gctx, `
			WITH d AS (SELECT max(scrutin_annee) a FROM core.municipal_list)
			SELECT l.commune_code, l.tour, l.libelle, coalesce(l.nuance_code,''),
			       coalesce(l.voix,0), coalesce(l.sieges_cm,0),
			       100.0*coalesce(l.voix,0)/nullif(sum(l.voix) OVER (PARTITION BY l.commune_code, l.tour),0)
			FROM core.municipal_list l, d
			WHERE l.scrutin_annee=d.a`)
		if err != nil {
			return err
		}
		defer lrows.Close()
		parTour := map[string]map[int][]ListeMunicipale{}
		for lrows.Next() {
			var com string
			var tour int
			var lm ListeMunicipale
			var pct *float64
			if err := lrows.Scan(&com, &tour, &lm.Libelle, &lm.Nuance, &lm.Voix, &lm.Sieges, &pct); err != nil {
				return err
			}
			if pct != nil {
				lm.Pct = *pct
			}
			if parTour[com] == nil {
				parTour[com] = map[int][]ListeMunicipale{}
			}
			parTour[com][tour] = append(parTour[com][tour], lm)
		}
		if err := lrows.Err(); err != nil {
			return err
		}
		for com, tours := range parTour {
			p := pages[com]
			if p == nil {
				continue
			}
			final := 1
			if len(tours[2]) > 0 {
				final = 2
				p.ListesT1 = tours[1]
				sort.Slice(p.ListesT1, func(i, j int) bool { return p.ListesT1[i].Voix > p.ListesT1[j].Voix })
			}
			p.TourFinal = final
			p.Listes = tours[final]
			sort.Slice(p.Listes, func(i, j int) bool { return p.Listes[i].Voix > p.Listes[j].Voix })
		}
		return nil
	})

	// 5. Associations déclarées — mv.commune_association_count (internal/
	// matview) remplace le GROUP BY sur la totalité de core.association
	// (1,18 million de lignes).
	g.Go(func() error {
		arows, err := pool.Query(gctx, `
			SELECT commune_code, nombre_associations FROM mv.commune_association_count`)
		if err != nil {
			return err
		}
		defer arows.Close()
		for arows.Next() {
			var com string
			var n int
			if err := arows.Scan(&com, &n); err != nil {
				return err
			}
			if p := pages[com]; p != nil {
				p.Associations = n
			}
		}
		return arows.Err()
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return pages, nil
}

// chargerPagesEPCI : les groupements à fiscalité propre.
func chargerPagesEPCI(ctx context.Context, pool *pgxpool.Pool, r *Resolveur,
	col *StatsCollectivites, avecFiche map[string]bool) (map[string]*PageEPCI, error) {

	pages := map[string]*PageEPCI{}
	for siren, e := range r.epci {
		if !e.Page {
			continue
		}
		p := &PageEPCI{Siren: siren, Nom: e.Nom, Nature: e.Nature,
			NatureLib: libelleNature[e.Nature], Exercice: col.Exercice}
		if l, ok := r.lieuDept(e.Dept); ok {
			p.Dept = l
		}
		pages[siren] = p
	}
	rows, err := pool.Query(ctx, `
		SELECT siren, coalesce(population_totale,0), coalesce(nb_membres,0),
		       trim(coalesce(president_prenom,'')||' '||coalesce(president_nom,''))
		FROM core.epci WHERE siren = ANY($1)`, clesEPCI(pages))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s, pres string
		var pop, nb int
		if err := rows.Scan(&s, &pop, &nb, &pres); err != nil {
			rows.Close()
			return nil, err
		}
		if p := pages[s]; p != nil {
			p.Population, p.NbCommunes, p.President = pop, nb, NomPropre(pres)
		}
	}
	rows.Close()

	for com, s := range r.epciDeCom {
		if p := pages[s]; p != nil {
			p.Communes = append(p.Communes, r.lieuCommune(com))
		}
	}
	for _, p := range pages {
		sort.Slice(p.Communes, func(i, j int) bool {
			return CleTri(p.Communes[i].Nom) < CleTri(p.Communes[j].Nom)
		})
		// La région, par la première commune membre : un groupement est dans
		// un seul département de siège, et donc une seule région.
		if len(p.Communes) > 0 {
			if c, ok := r.communes[p.Communes[0].Code]; ok {
				if l, ok := r.lieuRegion(c.Region); ok {
					p.Region = l
				}
			}
		}
	}

	crows, err := pool.Query(ctx, `
		SELECT ec.epci_siren, c.libelle FROM core.epci_competence ec
		JOIN ref.competence c ON c.code=ec.competence_code
		WHERE ec.epci_siren = ANY($1) ORDER BY c.libelle`, clesEPCI(pages))
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var s, lib string
		if err := crows.Scan(&s, &lib); err != nil {
			crows.Close()
			return nil, err
		}
		if p := pages[s]; p != nil {
			p.Competences = append(p.Competences, strings.TrimSpace(lib))
		}
	}
	crows.Close()

	// Le conseil communautaire : les mandats dont la circonscription commence
	// par le SIREN du groupement.
	erows, err := pool.Query(ctx, `
		SELECT split_part(m.constituency,' ',1), p.given_name||' '||p.family_name, p.slug,
		       coalesce(m.role,''), to_char(lower(m.validity),'DD/MM/YYYY')
		FROM core.mandate m JOIN core.person p ON p.id=m.person_id
		WHERE m.mandate_type='CONSEILLER_COMMUNAUTAIRE' AND m.constituency IS NOT NULL
		  AND upper(m.validity) IS NULL`)
	if err != nil {
		return nil, err
	}
	for erows.Next() {
		var s, nom, slug, role, depuis string
		if err := erows.Scan(&s, &nom, &slug, &role, &depuis); err != nil {
			erows.Close()
			return nil, err
		}
		p := pages[s]
		if p == nil {
			continue
		}
		p.Elus = append(p.Elus, EluCommune{Nom: NomPropre(nom), Slug: slug, Fonction: role,
			Depuis: depuis, Fiche: avecFiche[slug], Ordre: rangFonction(role)})
	}
	erows.Close()
	for _, p := range pages {
		p.NbConseillers = len(p.Elus)
		trierElus(p.Elus)
	}

	// Finances, classées parmi les groupements de la même nature juridique :
	// une métropole et une communauté de communes rurale ne gèrent pas les
	// mêmes compétences.
	grp := map[string]map[string]float64{} // siren → indicateur → par hab
	totaux := map[string]map[string]float64{}
	brows, err := pool.Query(ctx, `
		SELECT code, indicator_code, euros_par_hab::float8, montant::float8
		FROM core.collectivite_budget WHERE niveau='GROUPEMENT' AND exercice=$1`, col.Exercice)
	if err != nil {
		return nil, err
	}
	for brows.Next() {
		var s, ind string
		var hab, tot *float64
		if err := brows.Scan(&s, &ind, &hab, &tot); err != nil {
			brows.Close()
			return nil, err
		}
		if hab == nil {
			continue
		}
		if grp[s] == nil {
			grp[s], totaux[s] = map[string]float64{}, map[string]float64{}
		}
		grp[s][ind] = *hab
		if tot != nil {
			totaux[s][ind] = *tot
		}
	}
	brows.Close()
	parNature := map[string]map[string][]float64{}
	for s, m := range grp {
		p := pages[s]
		if p == nil {
			continue
		}
		if parNature[p.Nature] == nil {
			parNature[p.Nature] = map[string][]float64{}
		}
		for ind, v := range m {
			parNature[p.Nature][ind] = append(parNature[p.Nature][ind], v)
		}
	}
	for _, m := range parNature {
		for _, vs := range m {
			sort.Float64s(vs)
		}
	}
	for s, m := range grp {
		p := pages[s]
		if p == nil {
			continue
		}
		for _, ind := range indicsCollectivite {
			v, ok := m[ind.Code]
			if !ok {
				continue
			}
			vs := parNature[p.Nature][ind.Code]
			l := LigneFinance{Libelle: ind.Libelle, Total: totaux[s][ind.Code], ParHab: v,
				Mediane: vs[len(vs)/2], Sur: len(vs)}
			for _, autre := range vs {
				if autre > v {
					l.Rang++
				}
			}
			l.Rang++
			p.Finances = append(p.Finances, l)
		}
	}

	var millesimeCog int
	_ = pool.QueryRow(ctx,
		`SELECT max(cog_millesime) FROM geo.contour_cog WHERE niveau='COMMUNE'`).Scan(&millesimeCog)
	if millesimeCog > 0 {
		// Le fond de carte départemental est chargé une fois par département,
		// pas une fois par groupement qui y a son siège — voir le commentaire
		// de contexteDept (internal/sitegen/carte_maillee.go).
		contextesDept := map[string]*contexteDept{}
		for siren, p := range pages {
			var ctxDept *contexteDept
			if p.Dept.Code != "" {
				if c, ok := contextesDept[p.Dept.Code]; ok {
					ctxDept = c
				} else {
					c, err := chargerContexteDept(ctx, pool, p.Dept.Code, millesimeCog)
					if err != nil {
						return nil, err
					}
					contextesDept[p.Dept.Code] = c
					ctxDept = c
				}
			}
			svg, n, err := carteCommunesEPCIAvecContexte(ctx, pool, ctxDept, siren, millesimeCog, p.Nom)
			if err != nil {
				return nil, err
			}
			p.CarteCommunes, p.NbCommunesCarte = svg, n
		}
	}
	return pages, nil
}

func clesEPCI(m map[string]*PageEPCI) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (p *PageCommune) Titre() string {
	return fmt.Sprintf("%s (%s)", p.Nom, p.Dept.Code)
}
