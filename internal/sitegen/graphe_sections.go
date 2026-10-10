package sitegen

// Compléments de graphe.go : le patron répétitif "un chargement, une
// écriture" (addPageNode) et les quelques nœuds trop longs pour rester des
// closures inline dans buildRegistry sans le rendre illisible —
// chacun repris quasi verbatim du corps de l'ancien run() séquentiel,
// jamais réécrit en chemin.
import (
	"context"
	"fmt"
	"html/template"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"golang.org/x/sync/errgroup"
)

// nothingAPublish : v est-il le zéro d'un type qui peut légitimement le valoir
// quand sa source est hors chaîne par défaut (ingest.ChaineParDefaut) et
// n'a simplement rien chargé — un pointeur nil (chargerSecondeGuerreMondiale
// et consorts renvoient déjà ce signal), mais aussi une interface, un slice
// ou une map nil : les mêmes types que la plupart des chargeurs renvoient
// en cas d'absence, pour peu qu'ils renvoient LE ZÉRO plutôt qu'une valeur
// partiellement construite (voir loadRichesse, corrigé pour s'y conformer).
// reflect.Value.IsNil panique sur un type qui n'est pas de ces quatre
// natures (un struct nu, un int...) — Kind() est vérifié avant pour ne
// jamais l'atteindre sur un type qui ne peut de toute façon pas être nil.
func nothingAPublish(v any) bool {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return rv.IsNil()
	}
	return false
}

// addPageNode : le patron d'une section à un seul chargement — charger(),
// puis écrire() sous le nom de section nom lui-même, sauf si -only
// l'exclut. La valeur chargée reste disponible aux dépendantes (la carte
// vieillesse/jeunesse lit l'état renvoyé par sa propre page).
//
// Une page dont la source est hors chaîne par défaut (ingest.
// ChaineParDefaut) — richesse, empire colonial... — doit pouvoir dire
// « rien à publier » sans faire échouer toute la construction : charger()
// renvoyant le zéro de T (nil pour un pointeur/slice/map) est ce signal,
// reconnu ici une bonne fois pour toutes plutôt que par un garde ad hoc
// dans chaque gabarit. Avant cette vérification, une page dont le
// chargeur renvoyait nil était quand même écrite — le gabarit plantait
// alors lui-même sur un pointeur nil ou un index hors bornes, une erreur
// moins claire et plus tardive que de sauter la page ici.
func addPageNode[T any](region *pipeline.Registry, e *environment, name string, deps []string, title, template string,
	load func(ctx context.Context, d pipeline.Results) (T, error),
	data func(l Layout, v T) (string, any)) {
	region.Add(pipeline.Step{
		Name: name, Description: "page " + name, Dependencies: deps,
		Run: func(ctx context.Context, d pipeline.Results) (any, error) {
			v, err := load(ctx, d)
			if err != nil {
				return nil, err
			}
			if nothingAPublish(v) {
				logs.Notice(name + " : rien à publier (source hors chaîne non chargée), page sautée")
				return v, nil
			}
			l := e.layout
			l.Title = title
			path, data := data(l, v)
			if err := e.writeSection(name, e.page(template), path, data); err != nil {
				return nil, err
			}
			return v, nil
		},
	})
}

// buildMunicipalitiesAndEPCI reprend verbatim le court-circuit par empreinte
// (core.section_checksum, voir cache.go) que "communes" avait déjà avant ce
// graphe : recopier depuis le site précédent si données et gabarits sont
// inchangés, sinon recharger et réécrire, en parallèle (une commune, une
// clé de pagesCom, aucun état partagé entre deux itérations — même
// raisonnement que pour les scrutins).
func buildMunicipalitiesAndEPCI(ctx context.Context, e *environment, d pipeline.Results) error {
	places := dep[*Resolver](d, "lieux")
	background := dep[*backgroundSituation](d, "fond-situation")
	id := dep[identityBundle](d, "identite")
	cb := dep[localGovBundle](d, "collectivites-data")

	municipalitiesUnchanged, stateMunicipalities, err := sectionUnchanged(ctx, e.pool, e.previousCache(), "communes")
	if err != nil {
		return err
	}
	e.newCache.Sections["communes"] = stateMunicipalities
	if municipalitiesUnchanged {
		fmt.Println("    communes : données et gabarits inchangés, recopiées depuis le site précédent")
		if err := copyDirectory(filepath.Join(e.currentSite, "collectivites", "commune"),
			filepath.Join(e.out, "collectivites", "commune")); err != nil {
			return err
		}
		if err := copyDirectory(filepath.Join(e.currentSite, "collectivites", "epci"),
			filepath.Join(e.out, "collectivites", "epci")); err != nil {
			return err
		}
		fmt.Printf("  lieux : %d communes, %d intercommunalités — recopiées, données et gabarits inchangés (%s écoulées)\n",
			len(places.municipalities), len(places.epci), time.Since(e.start).Round(time.Second))
		return nil
	}

	pagesCom, err := loadPagesMunicipalities(ctx, e.pool, places, id.WithProfile)
	if err != nil {
		return err
	}
	fmt.Printf("    pages communes chargées : %s écoulées\n", time.Since(e.start).Round(time.Second))
	for code, pc := range pagesCom {
		pc.Situation = background.forMunicipality(code, pc.Name)
	}
	tcom := e.page("commune.gohtml")
	g := new(errgroup.Group)
	g.SetLimit(runtime.NumCPU())
	for code, pc := range pagesCom {
		code, pc := code, pc
		g.Go(func() error {
			lp := e.layout
			lp.Title = pc.Name + " (" + pc.Dept.Code + ")"
			return e.writeAlways(tcom, filepath.Join(e.out, "collectivites", "commune", code, "index.html"), struct {
				Layout
				C *PageMunicipality
			}{lp, pc})
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	fmt.Printf("    pages communes écrites : %s écoulées\n", time.Since(e.start).Round(time.Second))

	pagesEPCI, err := loadPagesEPCI(ctx, e.pool, places, cb.Col, id.WithProfile)
	if err != nil {
		return err
	}
	for siren, pe := range pagesEPCI {
		pe.Situation = background.forEPCI(siren, pe.Name, pe.Finances)
	}
	tepci := e.page("epci.gohtml")
	for siren, pe := range pagesEPCI {
		l := e.layout
		l.Title = pe.Name
		if err := e.writeAlways(tepci, filepath.Join(e.out, "collectivites", "epci", siren, "index.html"), struct {
			Layout
			E *PageEPCI
		}{l, pe}); err != nil {
			return err
		}
	}
	fmt.Printf("  lieux : %d communes, %d intercommunalités (%s écoulées)\n",
		len(places.municipalities), len(places.epci), time.Since(e.start).Round(time.Second))
	return nil
}

// buildTopicsData reprend verbatim la substitution des marqueurs
// <!-- schema:... --> dans les documents Markdown, puis rattacherDocs/
// reecrireLiensDocs/preparerSujets — voir la note de tête de graphe.go sur
// pourquoi ce nœud dépend de la trentaine de schémas plutôt que d'un
// sous-ensemble : quel marqueur vit dans quel document n'est su qu'en
// relisant le texte.
func buildTopicsData(ctx context.Context, e *environment, deps pipeline.Results) error {
	docs := dep[[]*Doc](deps, "docs")
	acc := dep[*DataHome](deps, "accueil-data")
	territory := dep[*StatsTerritories](deps, "territoires")
	circuit := dep[*CircuitChannels](deps, "circuit-canaux")
	section := dep[*StatsSectors](deps, "secteurs")
	thresholdsPoverty := dep[*ThresholdsPoverty](deps, "seuils-pauvrete")
	schemaRatePoverty := dep[template.HTML](deps, "taux-pauvrete-serie")
	schemaExpenseEnv := dep[template.HTML](deps, "depense-environnementale")
	schemaEmploymentTotal := dep[template.HTML](deps, "emploi-total-salarie")
	statsClimateInternational := dep[*StatsClimateInternational](deps, "climat-international")
	statsUnionEuropean := dep[*StatsUnionEuropean](deps, "union-europeenne")
	mapBasins := dep[*MapBasins](deps, "carte-bassins")
	mapEPTBEPAGE := dep[*MapEPTBEPAGE](deps, "carte-eptb-epage")
	schemaTrendExpensesFiscal := dep[template.HTML](deps, "depenses-fiscales-tendance")
	statsExpensesFiscal := dep[*StatsExpensesFiscal](deps, "depenses-fiscales-stats")
	mapIFI := dep[*MapIFI](deps, "carte-ifi")
	statsApparatusProductive := dep[*StatsApparatusProductive](deps, "appareil-productif")
	statsFrancophonie := dep[*StatsFrancophonie](deps, "francophonie")
	statsEmpireColonial := dep[*StatsEmpireColonial](deps, "empire-colonial")
	statsSGM := dep[*StatsSecondWorldWar](deps, "seconde-guerre-mondiale")
	populationWars := dep[*PopulationWars](deps, "population-guerres")
	controlFiscal := dep[*ControlFiscal](deps, "controle-fiscal")
	schemaPortsFrench := dep[template.HTML](deps, "ports-francais")
	schemaPortsEurope := dep[template.HTML](deps, "ports-europe")
	mapInfrastructurePorts := dep[*MapInfrastructurePorts](deps, "carte-infrastructure-ports")
	schemaReportModalPort := dep[template.HTML](deps, "report-modal-port")
	schemaReportModalContainers := dep[template.HTML](deps, "report-modal-conteneurs")
	statsJustice := dep[*StatsJustice](deps, "justice")
	schemaMapSemiconductors := dep[template.HTML](deps, "carte-semi-conducteurs")
	schemaContributoryNonContributory := dep[template.HTML](deps, "contributif-non-contributif")
	mapMuseums := dep[*MapMuseums](deps, "carte-musees")
	schemaGapPriceDOM := dep[template.HTML](deps, "ecart-prix-dom")
	schemaFoodDOM := dep[template.HTML](deps, "alimentaire-dom")
	mapStudents := dep[*MapStudents](deps, "carte-etudiants")
	mapSRU := dep[*MapSRU](deps, "carte-sru")
	levySRU := dep[*LevySRU](deps, "sru-prelevement")
	effortSearch := dep[*EffortSearch](deps, "effort-recherche")
	statsIndochinaPartition := dep[*StatsIndochinaPartition](deps, "indochine-partition")
	seriesDoctors := dep[template.HTML](deps, "medecins-evolution")
	schemaSIPRI := dep[template.HTML](deps, "sipri")
	schemaHistoryImmigration := dep[template.HTML](deps, "historique-immigration")
	schemaAgeStartPension := dep[template.HTML](deps, "age-depart-retraite")
	schemaRateReplacement := dep[template.HTML](deps, "taux-remplacement")
	root := e.root

	var mapDoctors *MapTerritory
	for i := range territory.Maps {
		if territory.Maps[i].Slug == "medecins-generalistes" {
			mapDoctors = &territory.Maps[i]
			break
		}
	}

	for _, d := range docs {
		if strings.Contains(string(d.Body), "<!-- schema:canaux -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:canaux -->",
				`<figure class="schema">`+string(circuit.SVG)+`<figcaption>`+
					`Trois largeurs sont <strong>proportionnelles</strong>, à la même échelle, pour `+
					fmt.Sprint(circuit.YearContributionsURSSAF)+`&nbsp;: les cotisations versées, les `+
					`exonérations (URSSAF, quatre catégories qui se somment exactement au total) et `+
					`la compensation qui leur répond (jaune budgétaire annexé au PLF 2024, exécution `+
					fmt.Sprint(circuit.YearCompensation)+`). Les autres flèches restent simples&nbsp;: `+
					`aucun montant comparable pour la même année n'a été trouvé. Non-compensation&nbsp;: `+
					`jaune budgétaire et LFSS `+fmt.Sprint(circuit.YearNonComp)+` — une mesure différente, `+
					`les mesures nouvelles décidées cette année-là, pas un solde cumulé comparable aux `+
					`largeurs ci-dessus. `+
					`<a href="`+root+`/budget/">La série annuelle des exonérations →</a></figcaption></figure>`))
		}
		if section != nil && strings.Contains(string(d.Body), "<!-- schema:s1311s1314 -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:s1311s1314 -->",
				`<figure class="schema">`+string(section.CurveS1311S1314)+`<figcaption>`+
					`Dépenses de l'administration centrale (S1311) contre la Sécurité sociale `+
					`(S1314), `+fmt.Sprint(section.Start)+`–`+fmt.Sprint(section.Year)+`.`+
					func() string {
						if section.YearIntersection1314 > 0 {
							return ` La Sécurité sociale dépense plus que l'État à partir de ` +
								fmt.Sprint(section.YearIntersection1314) + `.`
						}
						return ""
					}()+
					` <a href="`+root+`/budget/">Le détail par sous-secteur →</a></figcaption></figure>`))
		}
		if section != nil && strings.Contains(string(d.Body), "<!-- schema:financement34ans -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:financement34ans -->",
				`<figure class="schema">`+string(section.StackedFinancing)+
					`<div class="legende">`+
					`<span><i class="f0"></i>Cotisations des employeurs</span>`+
					`<span><i class="f1"></i>Cotisations des assurés</span>`+
					`<span><i class="f2"></i>Recettes fiscales affectées</span>`+
					`<span><i class="f3"></i>Recettes fiscales générales</span></div>`+
					`<figcaption>Les 34 années, en 100&nbsp;% empilé — pas seulement `+
					fmt.Sprint(section.StartEnd)+` et `+fmt.Sprint(section.AnnEnd)+
					`. Source&nbsp;: Eurostat ESSPROS, dataflow spr_rec_sumt. `+
					`<a href="`+root+`/budget/">Le tableau des deux dates →</a></figcaption></figure>`))
		}
		if thresholdsPoverty != nil && strings.Contains(string(d.Body), "<!-- schema:seuils-pauvrete -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:seuils-pauvrete -->",
				`<figure class="schema">`+string(thresholdsPoverty.SVG)+`<figcaption>`+
					`<strong>Comment lire ce graphique</strong> : classez tous les Français du `+
					`niveau de vie le plus bas au plus haut, puis coupez cette file en dix tas `+
					`du même nombre de personnes — 10&nbsp;% de la population dans chaque tas, `+
					`pas 10&nbsp;% du revenu total. D1 est le tas le plus pauvre, D9 le neuvième. `+
					`Chaque barre est le plafond de son tas, `+fmt.Sprint(thresholdsPoverty.Year)+
					` (Insee-Filosofi) : personne dans ce dixième de la population ne touche plus `+
					`que le montant affiché au-dessus de sa barre. Le dixième tas, D10 — les `+
					`10&nbsp;% les plus aisés —, n'a par définition aucun plafond&nbsp;: sa barre `+
					`s'estompe vers le haut plutôt que de s'arrêter net, pour montrer qu'elle `+
					`existe sans prétendre savoir où elle finit.`+
					`<br><strong>Le « niveau de vie »</strong> n'est pas le revenu du foyer tel quel, mais `+
					`ce revenu ramené à sa taille (un couple sans enfant n'a pas les mêmes besoins `+
					`qu'une famille de quatre) — de quoi comparer des foyers de toutes tailles sur `+
					`la même échelle, à peu près « ce que toucherait une personne seule pour vivre `+
					`aussi bien ».<br>`+
					`Les deux lignes pointillées sont les seuils de pauvreté&nbsp;: en dessous, `+
					`l'Insee et Eurostat considèrent qu'on est pauvre. Seul le premier dixième `+
					`(D1) plafonne sous le seuil à 60&nbsp;% (teinte plus sombre) ; le second (D2) `+
					`le chevauche — cohérent avec un taux de pauvreté à 60&nbsp;% légèrement `+
					`supérieur à 10&nbsp;%. L'axe part de zéro.`+
					`</figcaption></figure>`))
		}
		if schemaRatePoverty != "" && strings.Contains(string(d.Body), "<!-- schema:taux-pauvrete-serie -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:taux-pauvrete-serie -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaRatePoverty)+`</div>`+
					`<figcaption>Taux de pauvreté (seuil à 60 % du niveau de vie médian), 1996-2023 — `+
					`Insee. La refonte de l'enquête en 2021 (ERFS nouvelle formule) rend les deux `+
					`périodes non strictement comparables.</figcaption></figure>`))
		}
		if schemaExpenseEnv != "" && strings.Contains(string(d.Body), "<!-- schema:depense-environnementale -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:depense-environnementale -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaExpenseEnv)+`</div>`+
					`<figcaption>Dépense de protection de l'environment, ensemble de l'économie — `+
					`Eurostat (env_epea_neep), millions d'euros courants convertis en milliards.`+
					`</figcaption></figure>`))
		}
		if schemaEmploymentTotal != "" && strings.Contains(string(d.Body), "<!-- schema:emploi-total-salarie -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:emploi-total-salarie -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaEmploymentTotal)+`</div>`+
					`<figcaption>Emploi total et salarié, France, 1975-2025 — Eurostat (nama_10_pe). `+
					`L'écart entre les deux courbes est le nombre de non-salariés.</figcaption></figure>`))
		}
		if statsClimateInternational != nil && strings.Contains(string(d.Body), "<!-- schema:ratification-accord-paris -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:ratification-accord-paris -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsClimateInternational.CurveRatificationSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Nombre cumulé de pays ayant ratifié l'Accord de Paris — ONU, `+
						`collection des traités. %d pays au total, dont %d n'ont jamais ratifié (signature seule, `+
						`ou aucune des deux).</figcaption></figure>`,
						statsClimateInternational.CountCountry, statsClimateInternational.CountNeverRatified)))
		}
		if statsUnionEuropean != nil {
			if statsUnionEuropean.PIBSVG != "" && strings.Contains(string(d.Body), "<!-- schema:pib-blocs -->") {
				d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:pib-blocs -->",
					`<figure class="schema">`+string(statsUnionEuropean.PIBSVG)+
						fmt.Sprintf(`<figcaption>PIB, dollars courants, %d — Banque mondiale.</figcaption></figure>`,
							statsUnionEuropean.YearGdp)))
			}
			if statsUnionEuropean.SectorsSVG != "" && strings.Contains(string(d.Body), "<!-- schema:secteurs-blocs -->") {
				d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:secteurs-blocs -->",
					`<figure class="schema"><div class="carte-pleine">`+string(statsUnionEuropean.SectorsSVG)+`</div>`+
						fmt.Sprintf(`<figcaption>Valeur ajoutée par secteur, %% du PIB, %d — Banque mondiale.`+
							`</figcaption></figure>`, statsUnionEuropean.YearSectors)))
			}
			if statsUnionEuropean.TradeTable != "" && strings.Contains(string(d.Body), "<!-- tableau:commerce-blocs -->") {
				d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- tableau:commerce-blocs -->",
					string(statsUnionEuropean.TradeTable)))
			}
			if statsUnionEuropean.SectorsUSTable != "" && strings.Contains(string(d.Body), "<!-- tableau:secteurs-us -->") {
				d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- tableau:secteurs-us -->",
					string(statsUnionEuropean.SectorsUSTable)))
			}
		}
		if mapBasins != nil && strings.Contains(string(d.Body), "<!-- schema:carte-bassins -->") {
			var legend strings.Builder
			legend.WriteString(`<div class="repartition-legende">`)
			for _, bs := range mapBasins.Basins {
				fmt.Fprintf(&legend, `<div><i style="background:%s"></i><span>%s</span></div>`,
					bs.Color, template.HTMLEscapeString(bs.Name))
			}
			legend.WriteString(`</div>`)
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:carte-bassins -->",
				`<figure class="schema"><div class="carte-pleine">`+string(mapBasins.SVG)+`</div>`+
					legend.String()+
					`<figcaption>Les 7 bassins hydrographiques de France métropolitaine — les 6 `+
					`comités de bassin classiques plus la Corse, distincte hydrographiquement mais `+
					`rattachée administrativement à Rhône-Méditerranée (§ 1.1). Un découpage qui ne `+
					`suit aucune limite régionale ou départementale — le bassin Loire-Bretagne, le `+
					`plus vaste, traverse une douzaine de régions et départements actuels. `+
					`Source&nbsp;: BD Topage 2025, Sandre/IGN, Licence Ouverte.</figcaption></figure>`))
		}
		if mapEPTBEPAGE != nil && strings.Contains(string(d.Body), "<!-- schema:carte-eptb-epage -->") {
			legend := fmt.Sprintf(`<div class="repartition-legende">`+
				`<div><i style="background:%s"></i><span>EPTB (%d)</span></div>`+
				`<div><i style="background:%s"></i><span>EPAGE (%d)</span></div>`+
				`<div><i style="background:%s"></i><span>Double statut (%d)</span></div></div>`,
				colorsTypeEPTBEPAGE["EPTB"], mapEPTBEPAGE.CountEPTB,
				colorsTypeEPTBEPAGE["EPAGE"], mapEPTBEPAGE.CountEPAGE,
				colorsTypeEPTBEPAGE["EPTB_EPAGE"], mapEPTBEPAGE.CountDouble)
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:carte-eptb-epage -->",
				`<figure class="schema"><div class="carte-pleine">`+string(mapEPTBEPAGE.SVG)+`</div>`+
					legend+
					fmt.Sprintf(`<figcaption>%d structures sur %d trouvées dans BANATIC affichées ici : `+
						`en dessous de %.0f%% de membres reconstruits (communes ou EPCI déjà chargés dans `+
						`ce dépôt), le contour serait un fragment épars, plus trompeur qu'utile. Les zones `+
						`grises n'appartiennent à aucun EPTB/EPAGE reconstruit — pas forcément à aucun `+
						`EPTB/EPAGE réel (§ 1.2). Source&nbsp;: BANATIC (DGCL), contour reconstruit par ce `+
						`dépôt, pas téléchargé comme tel.</figcaption></figure>`,
						mapEPTBEPAGE.CountDisplayed, mapEPTBEPAGE.CountFound, thresholdResolutionEPTBEPAGE*100)))
		}
		if schemaTrendExpensesFiscal != "" && strings.Contains(string(d.Body), "<!-- schema:depenses-fiscales-tendance -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:depenses-fiscales-tendance -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaTrendExpensesFiscal)+`</div>`+
					`<figcaption>Total exécuté, année par année — chaque millésime du PLF ne publie l'exécution `+
					`que pour une seule année, jamais révisée dans un millésime ultérieur : sept millésimes, `+
					`sept années, sans doublon à trancher.</figcaption></figure>`))
		}
		if statsExpensesFiscal != nil && strings.Contains(string(d.Body), "<!-- tableau:depenses-fiscales-top -->") {
			var t strings.Builder
			t.WriteString(`<div class="scroll"><table><thead><tr><th>Dispositif</th><th>Impôt</th><th>Coût</th></tr></thead><tbody>`)
			for _, dsp := range statsExpensesFiscal.TopSchemes {
				fmt.Fprintf(&t, `<tr><td>%s</td><td>%s</td><td>%s M€</td></tr>`,
					template.HTMLEscapeString(dsp.Label), template.HTMLEscapeString(dsp.Tax), Count(int(dsp.AmountM)))
			}
			t.WriteString(`</tbody></table></div>`)
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- tableau:depenses-fiscales-top -->", t.String()))
		}
		if statsExpensesFiscal != nil && strings.Contains(string(d.Body), "<!-- tableau:depenses-fiscales-impot -->") {
			var t strings.Builder
			t.WriteString(`<div class="scroll"><table><thead><tr><th>Impôt</th><th>Dispositifs</th><th>Coût cumulé</th></tr></thead><tbody>`)
			for _, it := range statsExpensesFiscal.PerTax {
				fmt.Fprintf(&t, `<tr><td>%s</td><td>%s</td><td>%s Md€</td></tr>`,
					template.HTMLEscapeString(it.Tax), Count(it.CountScheme), Decimal(it.TotalMdEu, 1))
			}
			t.WriteString(`</tbody></table></div>`)
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- tableau:depenses-fiscales-impot -->", t.String()))
		}
		if mapIFI != nil && strings.Contains(string(d.Body), "<!-- schema:carte-ifi -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:carte-ifi -->",
				`<figure class="schema"><div class="carte-pleine">`+string(mapIFI.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>%d communes publiées par la DGFiP pour %d (plus de 20 000 `+
						`habitants et plus de 50 redevables à l'IFI — un seuil de publication de la DGFiP, `+
						`pas de ce dépôt). La surface de chaque cercle est proportionnelle au nombre de `+
						`redevables ; la couleur est uniforme. Les zones sans cercle n'ont pas de commune `+
						`publiée à ce niveau, pas forcément aucun redevable à l'IFI.</figcaption></figure>`,
						mapIFI.CountMunicipalities, mapIFI.Year)))
		}
		if statsApparatusProductive != nil && strings.Contains(string(d.Body), "<!-- schema:glissement-sectoriel -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:glissement-sectoriel -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsApparatusProductive.ChangeSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Part de l'emploi total par secteur, France, %d à %d — Eurostat, `+
						`nama_10_a10_e. « Services » est ici le complément (total moins agriculture, industrie et `+
						`construction), pas une addition de branches publiées séparément.</figcaption></figure>`,
						statsApparatusProductive.YearStartChange, statsApparatusProductive.YearEndChange)))
		}
		if statsApparatusProductive != nil && strings.Contains(string(d.Body), "<!-- schema:delocalisation-annuelle -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:delocalisation-annuelle -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsApparatusProductive.OffshoringAnnualSVG)+`</div>`+
					`<figcaption>Emplois en équivalent temps plein détectés comme délocalisés chaque année, `+
					`2001-2017 — la bande couvre les scénarios bas à haut du modèle Insee, la ligne est le `+
					`scénario central. Un chiffre encadré par une fourchette, pas une mesure exacte.</figcaption></figure>`))
		}
		if statsApparatusProductive != nil && strings.Contains(string(d.Body), "<!-- schema:delocalisation-departement -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:delocalisation-departement -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsApparatusProductive.OffshoringDeptSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Cumul 1995-2017 des emplois délocalisés (scénario central), par `+
						`département de résidence de l'entreprise — %d départements métropolitains. La surface de `+
						`chaque cercle est proportionnelle au nombre d'emplois.</figcaption></figure>`,
						statsApparatusProductive.CountDepartments)))
		}
		if statsApparatusProductive != nil && strings.Contains(string(d.Body), "<!-- tableau:delocalisation-csp -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- tableau:delocalisation-csp -->",
				string(statsApparatusProductive.OffshoringCSPTable)))
		}
		if statsApparatusProductive != nil {
			for _, marker := range []struct {
				key string
				st  *StatsTradeSector
			}{
				{"schema:commerce-automobile", statsApparatusProductive.TradeAutomobile},
				{"schema:commerce-textile", statsApparatusProductive.TradeTextile},
				{"schema:commerce-electronique-tv", statsApparatusProductive.TradeElectronicTV},
			} {
				if marker.st == nil {
					continue
				}
				m := "<!-- " + marker.key + " -->"
				if strings.Contains(string(d.Body), m) {
					d.Body = template.HTML(strings.ReplaceAll(string(d.Body), m,
						`<figure class="schema"><div class="carte-pleine">`+string(marker.st.SVG)+`</div>`+
							fmt.Sprintf(`<figcaption>Part de chaque partenaire dans les importations françaises de %s, `+
								`%d (point clair) et %d (point plein) — UN Comtrade, valeurs en dollars courants. `+
								`Total mondial : %s Md$ en %d, %s Md$ en %d.</figcaption></figure>`,
								marker.st.Label, marker.st.YearStart, marker.st.YearEnd,
								Decimal(marker.st.TotalUSDStart/1e9, 1), marker.st.YearStart,
								Decimal(marker.st.TotalUSDEnd/1e9, 1), marker.st.YearEnd)))
				}
			}
		}
		if statsFrancophonie != nil && strings.Contains(string(d.Body), "<!-- schema:francophonie-carte -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:francophonie-carte -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsFrancophonie.MapSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Part de francophones dans la population, par pays, 2025 — ODSEF/OIF. `+
						`%d des %d pays souverains du fichier source sont repérés sur ce fond de carte ; les pays en gris `+
						`n'ont pas de correspondance dans le fond Natural Earth utilisé ici, pas nécessairement aucune donnée.</figcaption></figure>`,
						statsFrancophonie.CountCountryMaps, statsFrancophonie.CountCountryTotal)))
		}
		if statsFrancophonie != nil && strings.Contains(string(d.Body), "<!-- tableau:francophonie-top-pct -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- tableau:francophonie-top-pct -->",
				string(statsFrancophonie.TopPerPctTable)))
		}
		if statsFrancophonie != nil && strings.Contains(string(d.Body), "<!-- tableau:francophonie-top-nombre -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- tableau:francophonie-top-nombre -->",
				string(statsFrancophonie.TopPerCountTable)))
		}
		if statsEmpireColonial != nil && strings.Contains(string(d.Body), "<!-- schema:empire-colonial-carte -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:empire-colonial-carte -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsEmpireColonial.MapSVG)+`</div>`+
					`<div class="echelle"><span><i class="vague-1"></i>1953-1956</span>`+
					`<span><i class="vague-2"></i>1958-1962</span>`+
					`<span><i class="vague-3"></i>1975-1977</span></div>`+
					fmt.Sprintf(`<figcaption>%d des %d territoires listés ont une géométrie CShapes ; `+
						`survolez chaque territoire pour sa date d'indépendance exacte.</figcaption></figure>`,
						statsEmpireColonial.CountMaps, statsEmpireColonial.CountTotal)))
		}
		if statsEmpireColonial != nil && strings.Contains(string(d.Body), "<!-- tableau:empire-colonial-territoires -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- tableau:empire-colonial-territoires -->",
				string(statsEmpireColonial.Table)))
		}
		if statsEmpireColonial != nil && statsEmpireColonial.ExtensionSVG != "" &&
			strings.Contains(string(d.Body), "<!-- schema:empire-colonial-extension -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:empire-colonial-extension -->",
				`<figure class="schema">`+string(statsEmpireColonial.ExtensionSVG)+
					`<figcaption>L'empire colonial français à quatre dates fixes, pas à sa dernière extension avant `+
					`chaque indépendance comme la carte ci-dessus — la croissance de l'empire, pas seulement son `+
					`rétrécissement. Un territoire absent d'une carte n'y était pas encore français, ou déjà `+
					`indépendant à cette date (CShapes 2.0).</figcaption></figure>`))
		}
		if statsIndochinaPartition != nil && strings.Contains(string(d.Body), "<!-- schema:indochine-1954 -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:indochine-1954 -->",
				`<figure class="schema">`+string(statsIndochinaPartition.MapSVG)+
					`<figcaption>Cambodge, Laos et Viêt Nam unifié à leur dernière extension coloniale (CShapes 2.0, `+
					`mêmes données que la carte de l'empire colonial), puis la partition du Viêt Nam actée par les `+
					`accords de Genève du 21 juillet 1954 jusqu'à la chute de Saïgon le 30 avril 1975 — le Laos et `+
					`le Cambodge, déjà indépendants depuis 1953, ne sont pas concernés par cette partition et sont `+
					`redessinés à titre de repère.</figcaption></figure>`))
		}
		if statsIndochinaPartition != nil && statsIndochinaPartition.Population != "" &&
			strings.Contains(string(d.Body), "<!-- schema:indochine-population -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:indochine-population -->",
				`<figure class="schema">`+string(statsIndochinaPartition.Population)+
					`<figcaption>Population du Viêt Nam, du Cambodge et du Laos, 1500-2000 (CLIO-INFRA, licence CC0-1.0) — `+
					`aux frontières ACTUELLES de ces trois pays, jamais au périmètre de l'Indochine française ni à un `+
					`recensement colonial. Trois échelles distinctes : le Viêt Nam pèse dix fois les deux autres réunis, `+
					`une échelle commune aurait aplati le Cambodge et le Laos.</figcaption></figure>`))
		}
		if statsSGM != nil && strings.Contains(string(d.Body), "<!-- schema:sgm-ligne-demarcation -->") {
			var b strings.Builder
			// Même intégration que carte-detail.gohtml (.cartes-lignes.carte-dossier,
			// carte à gauche, légende + résumé chiffré à droite), plutôt que le
			// .carte-pleine nu d'avant l'audit d'intégration cartographique du
			// 20 septembre 2026.
			b.WriteString(`<div class="cartes-lignes carte-dossier"><div class="bloc-carte ligne"><div>`)
			b.WriteString(string(statsSGM.MapSVG))
			b.WriteString(`</div><div>`)
			b.WriteString(`<div class="echelle"><span><i class="axe"></i>Axe</span>` +
				`<span><i class="occupe"></i>Occupé par l'Axe dès 1939-1940</span>` +
				`<span><i class="allie"></i>Allié</span>` +
				`<span><i class="neutre"></i>Neutre</span>` +
				`<span><i class="non-classe"></i>Hors classement</span></div>`)
			b.WriteString(`<div class="echelle"><span class="u">France</span>` +
				`<span><i class="occupee"></i>Zone occupée</span>` +
				`<span><i class="libre"></i>Zone libre</span></div>`)
			fmt.Fprintf(&b, `<dl class="legende-situation">`+
				`<div><dt>Pays de l'Axe</dt><dd><b>%d</b></dd></div>`+
				`<div><dt>Occupés par l'Axe dès 1939-1940</dt><dd><b>%d</b></dd></div>`+
				`<div><dt>Alliés</dt><dd><b>%d</b></dd></div>`+
				`<div><dt>Neutres</dt><dd><b>%d</b></dd></div>`+
				`<div><dt>Hors classement</dt><dd><b>%d</b><span>URSS, Europe centrale et Balkans, non traités par ce dossier</span></dd></div>`+
				`<div><dt>Ligne de démarcation</dt><dd><b>%s</b><span>km, zone occupée / zone libre, juin 1940 - mars 1943</span></dd></div>`+
				`</dl>`, statsSGM.CountAxis, statsSGM.CountOccupied, statsSGM.CountAlly, statsSGM.CountNeutral, statsSGM.CountNonClass,
				Decimal(statsSGM.LengthKm, 0))
			b.WriteString(`<span class="src">Frontières des pays voisins au 1ᵉʳ septembre 1940 (CShapes 2.0), ` +
				`pas les frontières actuelles : à cette date, la Pologne s'étend encore à l'est de sa frontière ` +
				`d'aujourd'hui, et l'Ukraine, la Biélorussie et les pays baltes ne sont pas des États — leur ` +
				`territoire relève soit de la Pologne d'avant-guerre, soit de l'URSS. CShapes ne subdivise ni la ` +
				`Tchécoslovaquie ni la Yougoslavie entre occupants. Statuts simplifiés à 1940 (l'Italie a changé de ` +
				`camp en 1943, non représenté) ; convention de couleurs Axe/Alliés/Neutre reprise de la légende ` +
				`Wikimedia « Map of participants in World War II », la plus citée mais pas la seule en usage. Fond ` +
				`de carte : CShapes 2.0 (ETH Zürich) ; cours d'eau : Natural Earth, domaine public. Ni l'annexion ` +
				`de fait de l'Alsace-Moselle ni la zone d'occupation italienne (à partir de novembre 1942) n'ont ` +
				`de géométrie vérifiée trouvée ; non représentées ici, voir § 2.</span>`)
			b.WriteString(`</div></div></div>`)
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:sgm-ligne-demarcation -->", b.String()))
		}
		if statsSGM != nil && statsSGM.LandingSVG != "" &&
			strings.Contains(string(d.Body), "<!-- schema:sgm-debarquements -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:sgm-debarquements -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsSGM.LandingSVG)+`</div>`+
					`<div class="echelle"><span><i class="overlord"></i>Overlord (Normandie, 6 juin)</span>`+
					`<span><i class="dragoon"></i>Dragoon (Provence, 15 août)</span></div>`+
					`<figcaption>Emplacement des cinq plages de Normandie et des trois secteurs de Provence, 1944 — `+
					`repère géographique, pas une carte des lignes de front ou des zones contrôlées. `+
					`Coordonnées relevées sur le lieu-dit ou le chef-lieu de chaque plage/secteur, pas la précision `+
					`d'un relevé militaire. Le tracé estompé de l'ancienne ligne de démarcation n'est qu'un repère : `+
					`elle avait disparu dans les faits depuis novembre 1942, bien avant ces deux débarquements `+
					`(voir § 2).</figcaption></figure>`))
		}
		if populationWars != nil && strings.Contains(string(d.Body), "<!-- schema:population-guerres -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:population-guerres -->",
				`<figure class="schema"><div class="carte-pleine">`+string(populationWars.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Population de la France (hors Mayotte), recensements 1876-1999 (Insee) — `+
						`entre 1911 et 1921, la population recule de %s à %s habitants, soit %s (%s %%). Aucun recul `+
						`comparable n'est visible entre 1936 et 1954 : la source ne publie aucun point en 1946, l'année `+
						`où le recul de la Seconde Guerre mondiale aurait été le plus visible.</figcaption></figure>`,
						Count(int(populationWars.Pop1911)), Count(int(populationWars.Pop1921)),
						Count(int(populationWars.DecreaseAbsolute)), Decimal(populationWars.DecreasePct, 1))))
		}
		if controlFiscal != nil && strings.Contains(string(d.Body), "<!-- schema:controle-fiscal -->") {
			calculated := ""
			if controlFiscal.LastNotifiedCalculated {
				calculated = " (déduit de l'écart notifié/encaissé publié, non cité tel quel par la source)"
			}
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:controle-fiscal -->",
				`<figure class="schema"><div class="carte-pleine">`+string(controlFiscal.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Résultats du contrôle fiscal, France entière, 2015-%d (Sénat, commission `+
						`des finances) — en %d, %s Md€ notifiés%s contre %s Md€ effectivement encaissés. Le notifié `+
						`2022 et 2023 n'a été retrouvé dans aucune des trois sources primaires consultées : la ligne `+
						`pointillée s'interrompt sur ces deux années plutôt que d'être devinée.</figcaption></figure>`,
						controlFiscal.LastYear, controlFiscal.LastYear,
						Decimal(controlFiscal.LastNotified, 1), calculated, Decimal(controlFiscal.LastCash, 1))))
		}
		if schemaPortsFrench != "" && strings.Contains(string(d.Body), "<!-- schema:ports-francais -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:ports-francais -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaPortsFrench)+`</div>`+
					`<figcaption>Trafic total (marchandises et tare, entrées et sorties confondues), `+
					`quatre grands ports maritimes, 2000-2025 — SDES.</figcaption></figure>`))
		}
		if schemaPortsEurope != "" && strings.Contains(string(d.Body), "<!-- schema:ports-europe -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:ports-europe -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaPortsEurope)+`</div>`+
					`<figcaption>Trafic total, dernière année disponible par port (Eurostat, mar_go_aa) — `+
					`l'année diffère selon le port, indiquée entre parenthèses.</figcaption></figure>`))
		}
		if mapInfrastructurePorts != nil && strings.Contains(string(d.Body), "<!-- schema:ports-infrastructure -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:ports-infrastructure -->",
				`<figure class="schema"><div class="carte-pleine">`+string(mapInfrastructurePorts.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Autoroutes (%d tronçons) et voies ferrées classées « voie portuaire » `+
						`par SNCF Réseau (%d tronçons) à moins de 80 km de chacun des quatre ports (OpenStreetMap, `+
						`SNCF Réseau) — un accès physique, pas une mesure de trafic : aucune donnée ouverte ne `+
						`distingue le fret des voyageurs sur le réseau ferré français.</figcaption></figure>`,
						mapInfrastructurePorts.CountHighways, mapInfrastructurePorts.CountLinesRail)))
		}
		if schemaReportModalPort != "" && strings.Contains(string(d.Body), "<!-- schema:report-modal-port -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:report-modal-port -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaReportModalPort)+`</div>`+
					`<figcaption>Part du fer et du fleuve dans le pré- et post-acheminement des marchandises, `+
					`2023 (DGITM, Observatoire de la performance portuaire) — un seul millésime, pas une série. `+
					`Marseille-Fos n'a publié qu'un plafond, pas un chiffre exact.</figcaption></figure>`))
		}
		if schemaReportModalContainers != "" && strings.Contains(string(d.Body), "<!-- schema:report-modal-conteneurs -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:report-modal-conteneurs -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaReportModalContainers)+`</div>`+
					`<figcaption>Répartition modale du transport de CONTENEURS vers l'arrière-pays — jamais à `+
					`comparer aux chiffres tous-trafics ci-dessus. HAROPA et Hambourg ne publient pas de `+
					`répartition conteneurs seuls vérifiable.</figcaption></figure>`))
		}
		if statsJustice != nil && strings.Contains(string(d.Body), "<!-- schema:justice-surpeuplement -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:justice-surpeuplement -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsJustice.TopOvercrowdingSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Les vingt quartiers d'établissement les plus densément peuplés sur %d `+
						`lignes chargées (établissement × quartier), ministère de la Justice, dernière donnée mensuelle. `+
						`La ligne verticale marque 100 %% (capacité atteinte).</figcaption></figure>`, statsJustice.CountLines)))
		}
		if schemaMapSemiconductors != "" && strings.Contains(string(d.Body), "<!-- schema:semi-conducteurs-carte -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:semi-conducteurs-carte -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaMapSemiconductors)+`</div>`+
					`<figcaption>Cinq sites de production identifiés par leur unité légale Sirene, géocodés à la `+
					`commune — pas à l'adresse exacte de l'usine. La Cour des comptes relève elle-même l'absence de `+
					`cartographie officielle de cette filière (§ 2).</figcaption></figure>`))
		}
		if strings.Contains(string(d.Body), "<!-- schema:contributif-non-contributif -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:contributif-non-contributif -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaContributoryNonContributory)+`</div>`+
					`<figcaption>Protection sociale par risque, 2024 (DREES) — la vieillesse est presque `+
					`entièrement contributive, les soins et la famille presque entièrement non contributifs. `+
					`Classement discuté poste par poste au paragraphe suivant.</figcaption></figure>`))
		}
		if mapMuseums != nil && strings.Contains(string(d.Body), "<!-- schema:carte-musees -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:carte-musees -->",
				`<figure class="schema"><div class="carte-pleine">`+string(mapMuseums.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>%d musées labellisés « Musée de France » (Muséofile, ministère de la `+
						`Culture), répartis sur %d départements — la surface de chaque cercle est proportionnelle `+
						`au nombre de musées, pas à leur taille ou à leur fréquentation.</figcaption></figure>`,
						mapMuseums.CountMuseums, mapMuseums.CountDepartments)))
		}
		if schemaGapPriceDOM != "" && strings.Contains(string(d.Body), "<!-- schema:ecart-prix-dom -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:ecart-prix-dom -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaGapPriceDOM)+`</div>`+
					`<figcaption>Écart de prix (indice de Fisher) avec la France métropolitaine, 2010 et 2022 `+
					`— Insee, enquête de comparaison spatiale des prix. Mayotte : donnée 2010 non disponible dans `+
					`la source.</figcaption></figure>`))
		}
		if schemaFoodDOM != "" && strings.Contains(string(d.Body), "<!-- schema:alimentaire-dom -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:alimentaire-dom -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaFoodDOM)+`</div>`+
					`<figcaption>Écart général contre écart sur les seuls produits alimentaires et boissons non `+
					`alcoolisées, 2022 (Insee) — les deux séries ne se ressemblent pas, jamais à confondre.</figcaption></figure>`))
		}
		if mapStudents != nil && strings.Contains(string(d.Body), "<!-- schema:carte-etudiants -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:carte-etudiants -->",
				`<figure class="schema"><div class="carte-pleine">`+string(mapStudents.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>%s étudiants répartis sur %d communes, rentrée %d (SIES) — la surface `+
						`de chaque cercle est proportionnelle à l'effectif. Paris apparaît par arrondissement, comme `+
						`dans la source.</figcaption></figure>`,
						Count(int(mapStudents.Total)), mapStudents.CountMunicipalities, mapStudents.Year)))
		}
		if mapSRU != nil && strings.Contains(string(d.Body), "<!-- schema:carte-sru -->") {
			// Même intégration que les autres cartes de dossier
			// (.cartes-lignes.carte-dossier, carte à gauche, légende +
			// résumé chiffré à droite), plutôt que le .carte-pleine nu
			// d'avant l'audit d'intégration cartographique du 20 septembre
			// 2026. L'exemption (colonne « 4 bis » du fichier source, voir
			// docs/logement-territoires-donnees.md) est une dimension à
			// part, jamais un quatrième statut de couleur sur la carte —
			// elle a donc sa propre ligne dans le résumé chiffré plutôt
			// qu'une entrée dans l'échelle de couleurs.
			var b strings.Builder
			b.WriteString(`<div class="cartes-lignes carte-dossier"><div class="bloc-carte ligne"><div>`)
			b.WriteString(string(mapSRU.SVG))
			b.WriteString(`</div><div>`)
			b.WriteString(`<div class="echelle"><span><i class="sru-conforme"></i>Conforme ou au-delà</span>` +
				`<span><i class="sru-deficitaire"></i>Déficitaire</span>` +
				`<span><i class="sru-carencee"></i>Carencée</span></div>`)
			fmt.Fprintf(&b, `<dl class="legende-situation">`+
				`<div><dt>Communes soumises à la loi SRU</dt><dd><b>%d</b></dd></div>`+
				`<div><dt>Carencées</dt><dd><b>%d</b></dd></div>`+
				`<div><dt>Déficitaires non carencées</dt><dd><b>%d</b></dd></div>`+
				`<div><dt>Conformes ou au-delà</dt><dd><b>%d</b></dd></div>`+
				`<div><dt>Exemptées (art. L.302-5 CCH)</dt><dd><b>%d</b><span>dimension distincte du statut ci-dessus — `+
				`une commune exemptée peut rester classée déficitaire ou carencée</span></dd></div>`+
				`<div><dt>Exemptées avec un prélèvement dû</dt><dd><b>%d</b><span>écart entre les deux colonnes du `+
				`fichier source, non expliqué par ce dossier</span></dd></div>`+
				`</dl>`, mapSRU.CountMunicipalities, mapSRU.CountDeficient, mapSRU.CountDeficit, mapSRU.CountCompliant,
				mapSRU.CountExempted, mapSRU.CountExemptedDeducted)
			fmt.Fprintf(&b, `<span class="src">%d communes soumises à la loi SRU au 1ᵉʳ janvier %d (DGALN/DHUP) · `+
				`taille du cercle proportionnelle à la population · contours&nbsp;: © les contributeurs OpenStreetMap, `+
				`ODbL&nbsp;1.0</span>`, mapSRU.CountMunicipalities, mapSRU.Year)
			b.WriteString(`</div></div></div>`)
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:carte-sru -->", b.String()))
		}
		if levySRU != nil && strings.Contains(string(d.Body), "<!-- schema:sru-prelevement -->") {
			var top strings.Builder
			top.WriteString(`<table><thead><tr><th>commune</th><th>département</th>` +
				`<th style="text-align:right">prélèvement net</th></tr></thead><tbody>`)
			for _, r := range levySRU.Top {
				fmt.Fprintf(&top, `<tr><td>%s</td><td>%s</td><td style="text-align:right">%s €</td></tr>`,
					template.HTMLEscapeString(r.Municipality), template.HTMLEscapeString(r.Department), Count(int(r.Amount)))
			}
			top.WriteString(`</tbody></table>`)
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:sru-prelevement -->",
				`<figure class="schema"><div class="carte-pleine">`+string(levySRU.Trend)+`</div>`+
					top.String()+
					fmt.Sprintf(`<figcaption>Communes carencées (barres, échelle de gauche) et prélèvement SRU net total `+
						`(ligne, échelle de droite), %d à %d — le prélèvement n'existe comme donnée qu'à partir de %d. `+
						`Au dernier millésime (%d) : %s € prélevés sur %d communes, dont les cinq plus lourdement `+
						`prélevées ci-dessus.</figcaption></figure>`,
						levySRU.YearStart, levySRU.YearEnd, 2024,
						levySRU.Year, Count(int(levySRU.Total)), levySRU.CountMunicipalitiesLevy)))
		}
		if effortSearch != nil && strings.Contains(string(d.Body), "<!-- schema:effort-recherche -->") {
			estimate := ""
			if effortSearch.Estimate {
				estimate = " (dernier point estimé par l'OCDE)"
			}
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:effort-recherche -->",
				`<figure class="schema"><div class="carte-pleine">`+string(effortSearch.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>DIRD (dépense intérieure de recherche et développement) rapportée au PIB, `+
						`%d à %d (Insee, sources MESR-SIES et OCDE)%s. En %d, la part portée par les entreprises seules `+
						`(DIRDE/PIB) est de %s %% en France contre %s %% en UE27.</figcaption></figure>`,
						effortSearch.YearStart, effortSearch.YearEnd, estimate, effortSearch.YearEnd,
						Decimal(effortSearch.DirdeFrLast, 2), Decimal(effortSearch.DirdeEuLast, 2))))
		}
		if schemaSIPRI != "" && strings.Contains(string(d.Body), "<!-- schema:sipri-milex -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:sipri-milex -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaSIPRI)+`</div>`+
					`<figcaption>Dépense militaire, % du PIB, 1949-2025 — SIPRI. France, Russie et `+
					`Arabie saoudite nommées (leurs trajectoires sont les plus commentées) ; les six autres `+
					`pays de la comparaison restent en gris, chacun identifiable au survol de son point final.`+
					`</figcaption></figure>`))
		}
		if schemaHistoryImmigration != "" && strings.Contains(string(d.Body), "<!-- schema:historique-immigration -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:historique-immigration -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaHistoryImmigration)+`</div>`+
					`<figcaption>Part d'immigrés dans la population, 1921-2025 — Insee, recensements. Les bandes `+
					`marquent les changements de champ ou de protocole (métropole puis France, Mayotte incluse `+
					`en 2014, protocole de collecte revu en 2024) : chaque régime se lit pour lui-même, pas comme `+
					`une évolution lissée d'un bout à l'autre.</figcaption></figure>`))
		}
		if schemaAgeStartPension != "" && strings.Contains(string(d.Body), "<!-- schema:age-depart-retraite -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:age-depart-retraite -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaAgeStartPension)+`</div>`+
					`<figcaption>Âge conjoncturel moyen de départ à la retraite, 2004-2022 — Drees. Le creux de 2010 `+
					`précède la réforme qui recule ensuite progressivement l'âge légal.</figcaption></figure>`))
		}
		if schemaRateReplacement != "" && strings.Contains(string(d.Body), "<!-- schema:taux-remplacement-retraite -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:taux-remplacement-retraite -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaRateReplacement)+`</div>`+
					`<figcaption>Dispersion du taux de remplacement (niveau de vie), cohorte 2020 — Drees. Boîte : `+
					`du 1ᵉʳ au 3ᵉ quartile, avec la médiane ; tige : du 1ᵉʳ au 9ᵉ décile. 100 = pension égale au `+
					`revenu d'avant la retraite.</figcaption></figure>`))
		}
		if strings.Contains(string(d.Body), "<!-- schema:holding-mere-fille -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:holding-mere-fille -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaHoldingParentSubsidiary())+`</div>`+
					`<figcaption>Exemple pédagogique sur un bénéfice initial rond (100 000 €) ; les taux (25 % `+
					`d'IS, 5 % de quote-part, 1 % en cas d'intégration fiscale, 30 % de PFU) sont réels et `+
					`sourcés — voir le Cadre et les Enjeux ci-dessus. Le point central : l'IS de la filiale `+
					`est payé AVANT que le dividende n'existe, pas une fois qu'il est dans la holding.</figcaption></figure>`))
		}
		if strings.Contains(string(d.Body), "<!-- schema:tva-entreprises -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:tva-entreprises -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaVatCompanies())+`</div>`+
					`<figcaption>Exemple pédagogique (pas une chaîne de transactions réelles observées, voir `+
					`« Ce que les données ne disent pas ») : à chaque étage, TVA collectée moins TVA déductible `+
					`donne la TVA nette versée à l'État — la somme des trois retombe exactement sur la TVA payée `+
					`par le consommateur final.</figcaption></figure>`))
		}
		if mapDoctors != nil && strings.Contains(string(d.Body), "<!-- schema:carte-medecins-generalistes -->") {
			p := mapDoctors.Page
			c := p.Map
			var b strings.Builder
			// Même intégration que carte-detail.gohtml (.cartes-lignes.carte-dossier,
			// carte à gauche, légende + résumé chiffré à droite) — jamais le
			// .carte-pleine nu utilisé ici avant l'audit d'intégration cartographique
			// du 20 septembre 2026, seule carte du site restée à l'ancien format.
			if p.Question != "" {
				fmt.Fprintf(&b, `<p class="q">%s</p>`, template.HTMLEscapeString(p.Question))
			}
			b.WriteString(`<div class="cartes-lignes carte-dossier"><div class="bloc-carte ligne"><div>`)
			b.WriteString(string(c.SVG))
			b.WriteString(tilesHTML(c.Tiles))
			b.WriteString(`</div><div>`)
			fmt.Fprintf(&b, `<div class="echelle"><span class="u">%s</span>`, template.HTMLEscapeString(c.Unit))
			for i, bound := range c.Bounds {
				fmt.Fprintf(&b, `<span><i style="background:%s"></i>%s</span>`, c.Shades[i], bound)
			}
			if c.CountAbsents > 0 {
				b.WriteString(`<span><i style="background:#EFEBE2"></i>aucune donnée</span>`)
			}
			b.WriteString(`</div>`)
			if r := p.Summary; r != nil {
				fmt.Fprintf(&b, `<dl class="legende-situation">`+
					`<div><dt>Départements couverts</dt><dd><b>%s</b></dd></div>`+
					`<div><dt>Médiane</dt><dd><b>%s</b></dd></div>`+
					`<div><dt>Maximum</dt><dd><b>%s</b><span>%s</span></dd></div>`+
					`<div><dt>Minimum</dt><dd><b>%s</b><span>%s</span></dd></div></dl>`,
					Count(r.Count), r.MedianValue, r.MaxValue, template.HTMLEscapeString(r.MaxName),
					r.MinValue, template.HTMLEscapeString(r.MinName))
			}
			fmt.Fprintf(&b, `<span class="src">%s · %d départements renseignés`,
				template.HTMLEscapeString(p.Source), c.Total)
			if c.CountAbsents > 0 {
				fmt.Fprintf(&b, `, %d sans donnée`, c.CountAbsents)
			}
			b.WriteString(` · contours&nbsp;: © les contributeurs OpenStreetMap, ODbL&nbsp;1.0</span>`)
			if p.Note != "" {
				fmt.Fprintf(&b, `<span class="src">%s</span>`, template.HTMLEscapeString(p.Note))
			}
			fmt.Fprintf(&b, `<a class="voir" href="%s/collectivites/carte/medecins-generalistes/">Le classement des `+
				`101 départements et la méthode →</a>`, root)
			b.WriteString(`</div></div></div>`)
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:carte-medecins-generalistes -->", b.String()))
		}
		if seriesDoctors != "" && strings.Contains(string(d.Body), "<!-- schema:medecins-evolution -->") {
			d.Body = template.HTML(strings.ReplaceAll(string(d.Body), "<!-- schema:medecins-evolution -->",
				`<figure class="schema">`+string(seriesDoctors)+
					`<figcaption>Effectif total de médecins, tous secteurs et catégories confondus, France `+
					`entière, 2010-2024 (Cnam, Démographie secteurs conventionnels). L'axe part de zéro&nbsp;: `+
					`une baisse de quelques pour cent ne doit pas ressembler à un effondrement.</figcaption></figure>`))
		}
	}

	if err := rattacherDocs(docs); err != nil {
		return err
	}
	rewriteLinksDocs(docs, root)
	return prepareTopics(ctx, e.pool, e.out, root, acc)
}

// buildGuidesAndTopics reprend verbatim la double boucle sur docs :
// chaque document est soit un sujet (sa propre section -only, l'ID du
// sujet — "fpctl build <sujet.id>" ne construit que celui-là), soit un
// document de méthode accumulé dans methode pour la section "comprendre".
func buildGuidesAndTopics(e *environment, docs []*Doc) error {
	ts := e.page("sujet.gohtml")
	var method []*Doc
	for _, d := range docs {
		s := topicDuDoc(d.Slug)
		if s == nil {
			method = append(method, d)
			continue
		}
		if e.excluded(s.ID) {
			continue
		}
		l := e.layout
		l.Title = s.Name
		if err := e.writeAlways(ts, filepath.Join(e.out, filepath.FromSlash(s.URL()), "index.html"), struct {
			Layout
			S *Topic
		}{l, s}); err != nil {
			return err
		}
		if err := redirection(filepath.Join(e.out, "comprendre", d.Slug, "index.html"), e.root+"/"+s.URL()); err != nil {
			return err
		}
		if formerBase, moved := formerBasesTopics[s.ID]; moved {
			if err := redirection(filepath.Join(e.out, formerBase, s.ID, "index.html"), e.root+"/"+s.URL()); err != nil {
				return err
			}
		}
	}

	l := e.layout
	l.Title = "Documents de méthode"
	if err := e.writeSection("comprendre", e.page("comprendre.gohtml"), filepath.Join(e.out, "comprendre", "index.html"), struct {
		Layout
		Docs   []*Doc
		Groups []GroupDocs
	}{l, method, GroupDocsByFamily(method)}); err != nil {
		return err
	}
	td := e.page("doc.gohtml")
	for _, d := range method {
		var others []*Doc
		for _, o := range method {
			if o.Slug != d.Slug {
				others = append(others, o)
			}
		}
		l := e.layout
		l.Title = d.Title
		if err := e.writeSection("comprendre", td, filepath.Join(e.out, "comprendre", d.Slug, "index.html"), struct {
			Layout
			D      *Doc
			Others []*Doc
		}{l, d, others}); err != nil {
			return err
		}
	}
	return nil
}

// buildProfilePages reprend verbatim les cinq boucles de fiches (personnes,
// candidats, organisations, référentiels, groupes) — toutes sous la même
// section -only "fiches", exactement comme cmd/fpctl/build.go les groupe
// déjà.
func buildProfilePages(e *environment, d pipeline.Results) error {
	id := dep[identityBundle](d, "identite")
	e27 := dep[*Stats2027](d, "election2027")
	premises := dep[map[string]*ReconciliationRNE](d, "mandats-locaux")

	per2027 := map[string]*Candidate2027{}
	for _, k := range e27.Candidates {
		per2027[k.Slug] = k
	}
	byCand := map[string]*Candidate{}
	for _, c := range id.Candidates {
		if c.Person != nil {
			byCand[c.Person.Slug] = c
		}
	}

	tp := e.page("personne.gohtml")
	for _, p := range id.Persons {
		l := e.layout
		l.Title = p.FirstName + " " + p.Name
		data := struct {
			Layout
			P              *Person
			Cand           *Candidate
			Local          *ReconciliationRNE
			K              *Candidate2027
			Defs           template.HTML
			TotalElections int
		}{l, p, byCand[p.Slug], nil, nil, "", e.layout.Cov.Elections}
		if c := byCand[p.Slug]; c != nil {
			data.Local, data.K = premises[c.Slug], per2027[c.Slug]
			if data.K != nil && !data.K.Overview.Empty {
				data.Defs = e27.Defs
			}
		}
		if err := e.writeSection("fiches", tp, filepath.Join(e.out, "depute", p.Slug, "index.html"), data); err != nil {
			return err
		}
	}
	for _, c := range id.Candidates {
		l := e.layout
		l.Title = c.FirstName + " " + c.Name
		p := c.Person
		if p == nil {
			p = &Person{Slug: c.Slug, FirstName: c.FirstName, Name: c.Name}
		}
		k := per2027[c.Slug]
		var defs template.HTML
		if k != nil && !k.Overview.Empty {
			defs = e27.Defs
		}
		data := struct {
			Layout
			P              *Person
			Cand           *Candidate
			Local          *ReconciliationRNE
			K              *Candidate2027
			Defs           template.HTML
			TotalElections int
		}{l, p, c, premises[c.Slug], k, defs, e.layout.Cov.Elections}
		if err := e.writeSection("fiches", tp, filepath.Join(e.out, "candidat", c.Slug, "index.html"), data); err != nil {
			return err
		}
	}

	to := e.page("organisation.gohtml")
	for _, o := range id.Orgs {
		l := e.layout
		l.Title = o.Label
		if err := e.writeSection("fiches", to, filepath.Join(e.out, "organisation", o.Slug, "index.html"), struct {
			Layout
			O *Organization
		}{l, o}); err != nil {
			return err
		}
	}

	tr := e.page("referentiel.gohtml")
	for _, r := range id.Refs {
		var affected []*Organization
		for _, o := range id.OrgList {
			for _, c := range o.Classifications {
				if c.SetSlug == r.Slug {
					affected = append(affected, o)
					break
				}
			}
		}
		l := e.layout
		l.Title = r.Title
		if err := e.writeSection("fiches", tr, filepath.Join(e.out, "referentiel", r.Slug, "index.html"), struct {
			Layout
			R    *Reference
			Orgs []*Organization
		}{l, r, affected}); err != nil {
			return err
		}
	}

	tg := e.page("groupe.gohtml")
	for _, g := range id.Groups {
		l := e.layout
		l.Title = g.Name
		if err := e.writeSection("fiches", tg, filepath.Join(e.out, "groupe", g.Slug, "index.html"), struct {
			Layout
			G *Group
		}{l, g}); err != nil {
			return err
		}
	}
	return nil
}

// buildBallotSection reprend verbatim le court-circuit par empreinte que
// "scrutin" avait déjà avant ce graphe.
func buildBallotSection(ctx context.Context, e *environment, thresholds map[string]Threshold) (int, error) {
	srcElections := SourceInfo{Attribution: "Assemblée nationale, open data, Licence Ouverte"}
	for _, s := range e.layout.Sources {
		if strings.Contains(strings.ToLower(s.Label), "scrutins") {
			srcElections = s
			break
		}
	}
	fmt.Printf("  avant les scrutins : %s écoulées\n", time.Since(e.start).Round(time.Second))

	electionsUnchanged, stateElections, err := sectionUnchanged(ctx, e.pool, e.previousCache(), "scrutin")
	if err != nil {
		return 0, err
	}
	if e.maxElections == 0 {
		e.newCache.Sections["scrutin"] = stateElections
	} else if previous, ok := e.previousCache().Sections["scrutin"]; ok {
		e.newCache.Sections["scrutin"] = previous
	}
	if e.maxElections == 0 && electionsUnchanged {
		n := e.previousCache().Sections["scrutin"].N
		stateElections.N = n
		e.newCache.Sections["scrutin"] = stateElections
		fmt.Printf("    scrutins : données et gabarits inchangés, recopiés depuis le site précédent (%d)\n", n)
		if err := copyDirectory(filepath.Join(e.currentSite, "scrutin"), filepath.Join(e.out, "scrutin")); err != nil {
			return 0, err
		}
		return n, nil
	}
	n, err := buildElections(ctx, e.pool, e.page("scrutin.gohtml"), e.layout, e.out, e.maxElections, thresholds, srcElections)
	if err != nil {
		return 0, err
	}
	if e.maxElections == 0 {
		stateElections.N = n
		e.newCache.Sections["scrutin"] = stateElections
	}
	return n, nil
}

// sectionNodes : chaque nom -only reconnu par ecrire() ailleurs dans
// ce fichier et graphe.go, vers le ou les nœuds du registre qui écrivent
// sous ce nom. Presque toujours un seul ; plusieurs quand une section
// historique s'est retrouvée à cheval sur deux nœuds séparés du graphe
// (vieillesse/sa carte, collectivités/ses cartes/ses pages locales) — sans
// cette table, demander -only=vieillesse n'aurait tiré que le premier des
// deux, l'autre n'étant relié à rien qui le rende atteignable depuis lui.
var sectionNodes = map[string][]string{
	"frise":            {"frise"},
	"dette":            {"dette"},
	"chomage":          {"chomage"},
	"richesse":         {"richesse"},
	"dividendes":       {"dividendes"},
	"agriculture":      {"agriculture"},
	"europe":           {"europe-page"},
	"themes":           {"themes-page"},
	"senat":            {"senat-page"},
	"vieillesse":       {"vieillesse", "vieillesse-carte"},
	"jeunesse":         {"jeunesse", "jeunesse-carte"},
	"securite":         {"securite-page"},
	"election2027":     {"election2027-page"},
	"gouvernement":     {"gouvernement-page"},
	"collectivites":    {"collectivites-page", "collectivites-cartes", "collectivites-pages-locales"},
	"circonscriptions": {"circonscriptions-data"},
	"communes":         {"communes"},
	"budget":           {"budget"},
	"social":           {"social"},
	"qui-decide":       {"qui-decide"},
	"sources":          {"sources"},
	"candidats":        {"candidats"},
	"partis":           {"partis"},
	"assemblee":        {"assemblee"},
	"accueil":          {"accueil"},
	"sujets":           {"sujets"},
	"argent-public":    {"argent-public"},
	"comprendre":       {"comprendre"},
	"fiches":           {"fiches"},
	"recherche":        {"recherche"},
	"scrutin":          {"scrutin"},
}

// Sections lists every section name a caller may request (cmd/fpctl builds
// its per-section subcommands from this rather than hand-copying the list a
// second time) — sorted for a stable --help/completion order.
func Sections() []string {
	out := make([]string, 0, len(sectionNodes))
	for name := range sectionNodes {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Topics lists every individual sujet ID (fpctl build topic <id> reconstructs
// just that one) — read from the same familles/Sujets data the site itself
// renders from, so a new sujet is reachable the moment it's declared there.
func Topics() []string {
	var out []string
	for _, f := range families {
		for _, s := range f.Topics {
			out = append(out, s.ID)
		}
	}
	sort.Strings(out)
	return out
}

// ResolveTargets translates the requested section names into Registry.Run
// targets: nil/empty means everything (every node in sectionNodes, once each),
// a name not in sectionNodes is assumed to be an individual sujet ID (fpctl
// build topic <id>) and falls back to "comprendre", the node that dispatches
// all of them.
func ResolveTargets(sections []string) []string {
	if len(sections) == 0 {
		var targets []string
		for _, nodes := range sectionNodes {
			targets = append(targets, nodes...)
		}
		sort.Strings(targets) // stable plan across runs
		return targets
	}
	var targets []string
	seen := map[string]bool{}
	addTargets := func(nodes []string) {
		for _, n := range nodes {
			if !seen[n] {
				seen[n] = true
				targets = append(targets, n)
			}
		}
	}
	for _, name := range sections {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if nodes, ok := sectionNodes[name]; ok {
			addTargets(nodes)
		} else {
			addTargets(sectionNodes["comprendre"])
		}
	}
	return targets
}
