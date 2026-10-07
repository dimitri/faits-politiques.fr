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
var tiers = []struct {
	max int
	lib string
}{
	{500, "moins de 500 habitants"}, {2000, "500 à 2 000 habitants"},
	{10000, "2 000 à 10 000 habitants"}, {50000, "10 000 à 50 000 habitants"},
	{1 << 30, "plus de 50 000 habitants"},
}

func tier(pop int) int {
	for i, s := range tiers {
		if pop < s.max {
			return i
		}
	}
	return len(tiers) - 1
}

var indicsMunicipality = []struct{ code, lib string }{
	{"ofgl.fonctionnement_par_hab", "Dépenses de fonctionnement"},
	{"ofgl.investissement_par_hab", "Dépenses d'investissement"},
	{"ofgl.dette_par_hab", "Encours de dette"},
	{"ofgl.epargne_brute_par_hab", "Épargne brute"},
	{"ofgl.masse_salariale_par_hab", "Charges de personnel"},
}

// loadPagesMunicipalities fabrique les 34 875 pages en sept requêtes ensemblistes.
// Une requête par commune ferait 250 000 allers-retours et une construction de
// plusieurs heures.
func loadPagesMunicipalities(ctx context.Context, pool *pgxpool.Pool, r *Resolver,
	withProfile map[string]bool) (map[string]*PageMunicipality, error) {

	pages := make(map[string]*PageMunicipality, len(r.municipalities))
	for code, c := range r.municipalities {
		p := &PageMunicipality{Code: code, Name: c.Name, Population: c.Population}
		if l, ok := r.placeDept(c.Dept); ok {
			p.Dept = l
		}
		if l, ok := r.placeRegion(c.Region); ok {
			p.Region = l
		}
		if s := r.epciOfCom[code]; s != "" {
			e := r.epci[s]
			p.EPCI = &Place{Type: "EPCI", Code: s, Name: e.Name, URL: r.urlEPCI(s)}
			p.NatureEPCI = labelNature[e.Nature]
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
			var com, name, slug, typ, role, depuis string
			if err := rows.Scan(&com, &name, &slug, &typ, &role, &depuis); err != nil {
				return err
			}
			p := pages[com]
			if p == nil {
				continue
			}
			e := ElectedMunicipality{Name: NameClean(name), Slug: slug, Depuis: depuis, Profile: withProfile[slug]}
			if typ == "MAIRE" {
				e.Function, e.Order = "Maire", 0
				m := e
				p.Mayor = &m
				continue // le maire figure aussi comme conseiller : une seule ligne
			}
			e.Function = role
			e.Order = rankFunction(role)
			if strings.EqualFold(role, "maire") {
				continue
			}
			p.Elected = append(p.Elected, e)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, p := range pages {
			p.CountCouncillors = len(p.Elected)
			if p.Mayor != nil {
				p.CountCouncillors++
			}
			sortElected(p.Elected)
		}
		return nil
	})

	// 2. Finances : la dernière année publiée, indicateur par indicateur.
	g.Go(func() error {
		frows, err := pool.Query(gctx, `
			SELECT commune_code, indicator_code, period_year, value::float8
			FROM mv.commune_indicator_dernier
			WHERE indicator_code LIKE 'ofgl.%\_par\_hab'`)
		if err != nil {
			return err
		}
		defer frows.Close()
		values := map[string]map[string]float64{}
		perTier := map[int]map[string][]float64{}
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
			if an > p.YearFinances {
				p.YearFinances = an
			}
			if values[com] == nil {
				values[com] = map[string]float64{}
			}
			values[com][ind] = v
			s := tier(p.Population)
			if perTier[s] == nil {
				perTier[s] = map[string][]float64{}
			}
			perTier[s][ind] = append(perTier[s][ind], v)
		}
		if err := frows.Err(); err != nil {
			return err
		}
		medians := map[int]map[string]float64{}
		for s, m := range perTier {
			medians[s] = map[string]float64{}
			for ind, vs := range m {
				sort.Float64s(vs)
				medians[s][ind] = vs[len(vs)/2]
			}
		}
		for com, vals := range values {
			p := pages[com]
			s := tier(p.Population)
			for _, ind := range indicsMunicipality {
				v, ok := vals[ind.code]
				if !ok {
					continue
				}
				p.Finances = append(p.Finances, IndicatorMunicipality{
					Label: ind.lib, Value: eurInhabitants(v), Median: eurInhabitants(medians[s][ind.code]),
					Gross: v, GrossMedian: medians[s][ind.code],
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
			FROM mv.commune_delinquance_dernier`)
		if err != nil {
			return err
		}
		defer srows.Close()
		for srows.Next() {
			var com, ind string
			var an, n int
			var rate float64
			var diff bool
			if err := srows.Scan(&com, &an, &ind, &n, &rate, &diff); err != nil {
				return err
			}
			p := pages[com]
			if p == nil {
				continue
			}
			p.YearSecurity = an
			lib := labelSecurity[ind][0]
			if lib == "" {
				lib = ind
			}
			sc := SecurityMunicipality{Label: lib, Mask: !diff}
			if diff {
				sc.Rate, sc.Count = Decimal(rate, 1)+" ‰", Count(n)
			}
			p.Security = append(p.Security, sc)
		}
		if err := srows.Err(); err != nil {
			return err
		}
		for _, p := range pages {
			sort.Slice(p.Security, func(i, j int) bool {
				return KeySort(p.Security[i].Label) < KeySort(p.Security[j].Label)
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
		perRound := map[string]map[int][]ListMunicipal{}
		for lrows.Next() {
			var com string
			var round int
			var lm ListMunicipal
			var pct *float64
			if err := lrows.Scan(&com, &round, &lm.Label, &lm.Nuance, &lm.Votes, &lm.Seats, &pct); err != nil {
				return err
			}
			if pct != nil {
				lm.Pct = *pct
			}
			if perRound[com] == nil {
				perRound[com] = map[int][]ListMunicipal{}
			}
			perRound[com][round] = append(perRound[com][round], lm)
		}
		if err := lrows.Err(); err != nil {
			return err
		}
		for com, rounds := range perRound {
			p := pages[com]
			if p == nil {
				continue
			}
			final := 1
			if len(rounds[2]) > 0 {
				final = 2
				p.ListsT1 = rounds[1]
				sort.Slice(p.ListsT1, func(i, j int) bool { return p.ListsT1[i].Votes > p.ListsT1[j].Votes })
			}
			p.RoundFinal = final
			p.Lists = rounds[final]
			sort.Slice(p.Lists, func(i, j int) bool { return p.Lists[i].Votes > p.Lists[j].Votes })
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

// loadPagesEPCI : les groupements à fiscalité propre.
func loadPagesEPCI(ctx context.Context, pool *pgxpool.Pool, r *Resolver,
	col *StatsAuthorities, withProfile map[string]bool) (map[string]*PageEPCI, error) {

	pages := map[string]*PageEPCI{}
	for siren, e := range r.epci {
		if !e.Page {
			continue
		}
		p := &PageEPCI{Siren: siren, Name: e.Name, Nature: e.Nature,
			NatureLib: labelNature[e.Nature], FiscalYear: col.FiscalYear}
		if l, ok := r.placeDept(e.Dept); ok {
			p.Dept = l
		}
		pages[siren] = p
	}
	rows, err := pool.Query(ctx, `
		SELECT siren, coalesce(population_totale,0), coalesce(nb_membres,0),
		       trim(coalesce(president_prenom,'')||' '||coalesce(president_nom,''))
		FROM core.epci WHERE siren = ANY($1)`, keysEPCI(pages))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s, pres string
		var pop, count int
		if err := rows.Scan(&s, &pop, &count, &pres); err != nil {
			rows.Close()
			return nil, err
		}
		if p := pages[s]; p != nil {
			p.Population, p.CountMunicipalities, p.President = pop, count, NameClean(pres)
		}
	}
	rows.Close()

	for com, s := range r.epciOfCom {
		if p := pages[s]; p != nil {
			p.Municipalities = append(p.Municipalities, r.placeMunicipality(com))
		}
	}
	for _, p := range pages {
		sort.Slice(p.Municipalities, func(i, j int) bool {
			return KeySort(p.Municipalities[i].Name) < KeySort(p.Municipalities[j].Name)
		})
		// La région, par la première commune membre : un groupement est dans
		// un seul département de siège, et donc une seule région.
		if len(p.Municipalities) > 0 {
			if c, ok := r.municipalities[p.Municipalities[0].Code]; ok {
				if l, ok := r.placeRegion(c.Region); ok {
					p.Region = l
				}
			}
		}
	}

	crows, err := pool.Query(ctx, `
		SELECT ec.epci_siren, c.libelle FROM core.epci_competence ec
		JOIN ref.competence c ON c.code=ec.competence_code
		WHERE ec.epci_siren = ANY($1) ORDER BY c.libelle`, keysEPCI(pages))
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
			p.Responsibilities = append(p.Responsibilities, strings.TrimSpace(lib))
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
		var s, name, slug, role, depuis string
		if err := erows.Scan(&s, &name, &slug, &role, &depuis); err != nil {
			erows.Close()
			return nil, err
		}
		p := pages[s]
		if p == nil {
			continue
		}
		p.Elected = append(p.Elected, ElectedMunicipality{Name: NameClean(name), Slug: slug, Function: role,
			Depuis: depuis, Profile: withProfile[slug], Order: rankFunction(role)})
	}
	erows.Close()
	for _, p := range pages {
		p.CountCouncillors = len(p.Elected)
		sortElected(p.Elected)
	}

	// Finances, classées parmi les groupements de la même nature juridique :
	// une métropole et une communauté de communes rurale ne gèrent pas les
	// mêmes compétences.
	grp := map[string]map[string]float64{} // siren → indicateur → par hab
	totals := map[string]map[string]float64{}
	brows, err := pool.Query(ctx, `
		SELECT code, indicator_code, euros_par_hab::float8, montant::float8
		FROM core.collectivite_budget WHERE niveau='GROUPEMENT' AND exercice=$1`, col.FiscalYear)
	if err != nil {
		return nil, err
	}
	for brows.Next() {
		var s, ind string
		var inhabitants, total *float64
		if err := brows.Scan(&s, &ind, &inhabitants, &total); err != nil {
			brows.Close()
			return nil, err
		}
		if inhabitants == nil {
			continue
		}
		if grp[s] == nil {
			grp[s], totals[s] = map[string]float64{}, map[string]float64{}
		}
		grp[s][ind] = *inhabitants
		if total != nil {
			totals[s][ind] = *total
		}
	}
	brows.Close()
	perNature := map[string]map[string][]float64{}
	for s, m := range grp {
		p := pages[s]
		if p == nil {
			continue
		}
		if perNature[p.Nature] == nil {
			perNature[p.Nature] = map[string][]float64{}
		}
		for ind, v := range m {
			perNature[p.Nature][ind] = append(perNature[p.Nature][ind], v)
		}
	}
	for _, m := range perNature {
		for _, vs := range m {
			sort.Float64s(vs)
		}
	}
	for s, m := range grp {
		p := pages[s]
		if p == nil {
			continue
		}
		for _, ind := range indicsAuthority {
			v, ok := m[ind.Code]
			if !ok {
				continue
			}
			vs := perNature[p.Nature][ind.Code]
			l := LineFinance{Label: ind.Label, Total: totals[s][ind.Code], PerInhabitants: v,
				Median: vs[len(vs)/2], On: len(vs)}
			for _, other := range vs {
				if other > v {
					l.Rank++
				}
			}
			l.Rank++
			p.Finances = append(p.Finances, l)
		}
	}

	var vintageCog int
	_ = pool.QueryRow(ctx,
		`SELECT max(cog_millesime) FROM geo.contour_cog WHERE niveau='COMMUNE'`).Scan(&vintageCog)
	if vintageCog > 0 {
		// Le fond de carte départemental est chargé une fois par département,
		// pas une fois par groupement qui y a son siège — voir le commentaire
		// de contexteDept (internal/sitegen/carte_maillee.go). Les groupements
		// eux-mêmes sont d'abord réunis PAR département pour la même raison :
		// une géométrie par groupement (~1 250, deux requêtes chacune) est
		// devenue une géométrie par DÉPARTEMENT (~101, deux requêtes chacune)
		// — voir chargerGeometriesEPCI.
		sirensPerDept := map[string][]string{}
		for siren, p := range pages {
			sirensPerDept[p.Dept.Code] = append(sirensPerDept[p.Dept.Code], siren)
		}
		for deptCode, sirens := range sirensPerDept {
			var ctxDept *contextDept
			if deptCode != "" {
				c, err := loadContextDept(ctx, pool, deptCode, vintageCog)
				if err != nil {
					return nil, err
				}
				ctxDept = c
			}
			if ctxDept == nil {
				// Pas de fond départemental (outre-mer sans siège identifié,
				// par exemple) : repli sur la carte isolée, un groupement à
				// la fois — le cas rare que chargerGeometriesEPCI n'a pas à
				// couvrir en lot.
				for _, siren := range sirens {
					svg, n, err := mapMunicipalitiesEPCI(ctx, pool, siren, vintageCog, pages[siren].Name)
					if err != nil {
						return nil, err
					}
					pages[siren].MapMunicipalities, pages[siren].CountMunicipalitiesMap = svg, n
				}
				continue
			}
			geoms, err := loadGeometriesEPCI(ctx, pool, sirens, ctxDept.Srid, vintageCog)
			if err != nil {
				return nil, err
			}
			for _, siren := range sirens {
				p := pages[siren]
				g, n := geoms[siren], geoms[siren].n
				if n == 0 {
					svg, n2, err := mapMunicipalitiesEPCI(ctx, pool, siren, vintageCog, p.Name)
					if err != nil {
						return nil, err
					}
					p.MapMunicipalities, p.CountMunicipalitiesMap = svg, n2
					continue
				}
				p.MapMunicipalities, p.CountMunicipalitiesMap = assembleMapEPCI(ctxDept, p.Name, g), n
			}
		}
	}
	return pages, nil
}

func keysEPCI(m map[string]*PageEPCI) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (p *PageMunicipality) Title() string {
	return fmt.Sprintf("%s (%s)", p.Name, p.Dept.Code)
}
