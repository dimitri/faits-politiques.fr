package sitegen

// Compléments de graphe.go : le patron répétitif "un chargement, une
// écriture" (ajouterPage) et les quelques nœuds trop longs pour rester des
// closures inline dans construireRegistre sans le rendre illisible —
// chacun repris quasi verbatim du corps de l'ancien run() séquentiel,
// jamais réécrit en chemin.
import (
	"context"
	"fmt"
	"html/template"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/faits-politiques/faits-politiques/internal/pipeline"
	"golang.org/x/sync/errgroup"
)

// ajouterPage : le patron d'une section à un seul chargement — charger(),
// puis écrire() sous le nom de section nom lui-même, sauf si -only
// l'exclut. La valeur chargée reste disponible aux dépendantes (la carte
// vieillesse/jeunesse lit l'état renvoyé par sa propre page).
func ajouterPage[T any](reg *pipeline.Registre, e *environnement, nom string, deps []string, titre, gabarit string,
	charger func(ctx context.Context, d pipeline.Resultats) (T, error),
	donnees func(l Layout, v T) (string, any)) {
	reg.Ajouter(pipeline.Etape{
		Nom: nom, Description: "page " + nom, Dependances: deps,
		Executer: func(ctx context.Context, d pipeline.Resultats) (any, error) {
			v, err := charger(ctx, d)
			if err != nil {
				return nil, err
			}
			l := e.layout
			l.Title = titre
			chemin, data := donnees(l, v)
			if err := e.ecrire(nom, e.page(gabarit), chemin, data); err != nil {
				return nil, err
			}
			return v, nil
		},
	})
}

// construireCommunesEPCI reprend verbatim le court-circuit par empreinte
// (core.section_checksum, voir cache.go) que "communes" avait déjà avant ce
// graphe : recopier depuis le site précédent si données et gabarits sont
// inchangés, sinon recharger et réécrire, en parallèle (une commune, une
// clé de pagesCom, aucun état partagé entre deux itérations — même
// raisonnement que pour les scrutins).
func construireCommunesEPCI(ctx context.Context, e *environnement, d pipeline.Resultats) error {
	lieux := dep[*Resolveur](d, "lieux")
	fond := dep[*fondSituation](d, "fond-situation")
	id := dep[identiteBundle](d, "identite")
	cb := dep[collectivitesBundle](d, "collectivites-data")

	communesInchangees, etatCommunes, err := sectionInchangee(ctx, e.pool, e.ancienCacheValeur(), "communes")
	if err != nil {
		return err
	}
	e.nouveauCache.Sections["communes"] = etatCommunes
	if communesInchangees {
		fmt.Println("    communes : données et gabarits inchangés, recopiées depuis le site précédent")
		if err := copierRepertoire(filepath.Join(e.siteActuel, "collectivites", "commune"),
			filepath.Join(e.out, "collectivites", "commune")); err != nil {
			return err
		}
		if err := copierRepertoire(filepath.Join(e.siteActuel, "collectivites", "epci"),
			filepath.Join(e.out, "collectivites", "epci")); err != nil {
			return err
		}
		fmt.Printf("  lieux : %d communes, %d intercommunalités — recopiées, données et gabarits inchangés (%s écoulées)\n",
			len(lieux.communes), len(lieux.epci), time.Since(e.start).Round(time.Second))
		return nil
	}

	pagesCom, err := chargerPagesCommunes(ctx, e.pool, lieux, id.AvecFiche)
	if err != nil {
		return err
	}
	fmt.Printf("    pages communes chargées : %s écoulées\n", time.Since(e.start).Round(time.Second))
	for code, pc := range pagesCom {
		pc.Situation = fond.pourCommune(code, pc.Nom)
	}
	tcom := e.page("commune.gohtml")
	g := new(errgroup.Group)
	g.SetLimit(runtime.NumCPU())
	for code, pc := range pagesCom {
		code, pc := code, pc
		g.Go(func() error {
			lp := e.layout
			lp.Title = pc.Nom + " (" + pc.Dept.Code + ")"
			return e.write(tcom, filepath.Join(e.out, "collectivites", "commune", code, "index.html"), struct {
				Layout
				C *PageCommune
			}{lp, pc})
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	fmt.Printf("    pages communes écrites : %s écoulées\n", time.Since(e.start).Round(time.Second))

	pagesEPCI, err := chargerPagesEPCI(ctx, e.pool, lieux, cb.Col, id.AvecFiche)
	if err != nil {
		return err
	}
	for siren, pe := range pagesEPCI {
		pe.Situation = fond.pourEPCI(siren, pe.Nom, pe.Finances)
	}
	tepci := e.page("epci.gohtml")
	for siren, pe := range pagesEPCI {
		l := e.layout
		l.Title = pe.Nom
		if err := e.write(tepci, filepath.Join(e.out, "collectivites", "epci", siren, "index.html"), struct {
			Layout
			E *PageEPCI
		}{l, pe}); err != nil {
			return err
		}
	}
	fmt.Printf("  lieux : %d communes, %d intercommunalités (%s écoulées)\n",
		len(lieux.communes), len(lieux.epci), time.Since(e.start).Round(time.Second))
	return nil
}

// construireSujetsData reprend verbatim la substitution des marqueurs
// <!-- schema:... --> dans les documents Markdown, puis rattacherDocs/
// reecrireLiensDocs/preparerSujets — voir la note de tête de graphe.go sur
// pourquoi ce nœud dépend de la trentaine de schémas plutôt que d'un
// sous-ensemble : quel marqueur vit dans quel document n'est su qu'en
// relisant le texte.
func construireSujetsData(ctx context.Context, e *environnement, deps pipeline.Resultats) error {
	docs := dep[[]*Doc](deps, "docs")
	acc := dep[*DonneesAccueil](deps, "accueil-data")
	terr := dep[*StatsTerritoires](deps, "territoires")
	circuit := dep[*CircuitCanaux](deps, "circuit-canaux")
	sect := dep[*StatsSecteurs](deps, "secteurs")
	seuilsPauvrete := dep[*SeuilsPauvrete](deps, "seuils-pauvrete")
	schemaTauxPauvrete := dep[template.HTML](deps, "taux-pauvrete-serie")
	schemaDepenseEnv := dep[template.HTML](deps, "depense-environnementale")
	schemaEmploiTotal := dep[template.HTML](deps, "emploi-total-salarie")
	statsClimatInternational := dep[*StatsClimatInternational](deps, "climat-international")
	statsUnionEuropeenne := dep[*StatsUnionEuropeenne](deps, "union-europeenne")
	carteBassins := dep[*CarteBassins](deps, "carte-bassins")
	carteEPTBEPAGE := dep[*CarteEPTBEPAGE](deps, "carte-eptb-epage")
	schemaTendanceDepensesFiscales := dep[template.HTML](deps, "depenses-fiscales-tendance")
	statsDepensesFiscales := dep[*StatsDepensesFiscales](deps, "depenses-fiscales-stats")
	carteIFI := dep[*CarteIFI](deps, "carte-ifi")
	statsAppareilProductif := dep[*StatsAppareilProductif](deps, "appareil-productif")
	statsFrancophonie := dep[*StatsFrancophonie](deps, "francophonie")
	statsEmpireColonial := dep[*StatsEmpireColonial](deps, "empire-colonial")
	statsSGM := dep[*StatsSecondeGuerreMondiale](deps, "seconde-guerre-mondiale")
	populationGuerres := dep[*PopulationGuerres](deps, "population-guerres")
	controleFiscal := dep[*ControleFiscal](deps, "controle-fiscal")
	schemaPortsFrancais := dep[template.HTML](deps, "ports-francais")
	schemaPortsEurope := dep[template.HTML](deps, "ports-europe")
	carteInfrastructurePorts := dep[*CarteInfrastructurePorts](deps, "carte-infrastructure-ports")
	schemaReportModalPort := dep[template.HTML](deps, "report-modal-port")
	schemaReportModalConteneurs := dep[template.HTML](deps, "report-modal-conteneurs")
	statsJustice := dep[*StatsJustice](deps, "justice")
	schemaCarteSemiConducteurs := dep[template.HTML](deps, "carte-semi-conducteurs")
	schemaContributifNonContributif := dep[template.HTML](deps, "contributif-non-contributif")
	carteMusees := dep[*CarteMusees](deps, "carte-musees")
	schemaEcartPrixDOM := dep[template.HTML](deps, "ecart-prix-dom")
	schemaAlimentaireDOM := dep[template.HTML](deps, "alimentaire-dom")
	carteEtudiants := dep[*CarteEtudiants](deps, "carte-etudiants")
	carteSRU := dep[*CarteSRU](deps, "carte-sru")
	effortRecherche := dep[*EffortRecherche](deps, "effort-recherche")
	schemaSIPRI := dep[template.HTML](deps, "sipri")
	schemaHistoriqueImmigration := dep[template.HTML](deps, "historique-immigration")
	schemaAgeDepartRetraite := dep[template.HTML](deps, "age-depart-retraite")
	schemaTauxRemplacement := dep[template.HTML](deps, "taux-remplacement")
	root := e.root

	var carteMedecins *CarteTerritoire
	for i := range terr.Cartes {
		if terr.Cartes[i].Slug == "medecins-generalistes" {
			carteMedecins = &terr.Cartes[i]
			break
		}
	}

	for _, d := range docs {
		if strings.Contains(string(d.Corps), "<!-- schema:canaux -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:canaux -->",
				`<figure class="schema">`+string(circuit.SVG)+`<figcaption>`+
					`Trois largeurs sont <strong>proportionnelles</strong>, à la même échelle, pour `+
					fmt.Sprint(circuit.AnneeCotisationsURSSAF)+`&nbsp;: les cotisations versées, les `+
					`exonérations (URSSAF, quatre catégories qui se somment exactement au total) et `+
					`la compensation qui leur répond (jaune budgétaire annexé au PLF 2024, exécution `+
					fmt.Sprint(circuit.AnneeCompensation)+`). Les autres flèches restent simples&nbsp;: `+
					`aucun montant comparable pour la même année n'a été trouvé. Non-compensation&nbsp;: `+
					`jaune budgétaire et LFSS `+fmt.Sprint(circuit.AnneeNonComp)+` — une mesure différente, `+
					`les mesures nouvelles décidées cette année-là, pas un solde cumulé comparable aux `+
					`largeurs ci-dessus. `+
					`<a href="`+root+`/budget/">La série annuelle des exonérations →</a></figcaption></figure>`))
		}
		if sect != nil && strings.Contains(string(d.Corps), "<!-- schema:s1311s1314 -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:s1311s1314 -->",
				`<figure class="schema">`+string(sect.CourbeS1311S1314)+`<figcaption>`+
					`Dépenses de l'administration centrale (S1311) contre la Sécurité sociale `+
					`(S1314), `+fmt.Sprint(sect.Debut)+`–`+fmt.Sprint(sect.Annee)+`.`+
					func() string {
						if sect.AnneeCroisement1314 > 0 {
							return ` La Sécurité sociale dépense plus que l'État à partir de ` +
								fmt.Sprint(sect.AnneeCroisement1314) + `.`
						}
						return ""
					}()+
					` <a href="`+root+`/budget/">Le détail par sous-secteur →</a></figcaption></figure>`))
		}
		if sect != nil && strings.Contains(string(d.Corps), "<!-- schema:financement34ans -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:financement34ans -->",
				`<figure class="schema">`+string(sect.EmpileesFinancement)+
					`<div class="legende">`+
					`<span><i class="f0"></i>Cotisations des employeurs</span>`+
					`<span><i class="f1"></i>Cotisations des assurés</span>`+
					`<span><i class="f2"></i>Recettes fiscales affectées</span>`+
					`<span><i class="f3"></i>Recettes fiscales générales</span></div>`+
					`<figcaption>Les 34 années, en 100&nbsp;% empilé — pas seulement `+
					fmt.Sprint(sect.DebutFin)+` et `+fmt.Sprint(sect.AnnFin)+
					`. Source&nbsp;: Eurostat ESSPROS, dataflow spr_rec_sumt. `+
					`<a href="`+root+`/budget/">Le tableau des deux dates →</a></figcaption></figure>`))
		}
		if seuilsPauvrete != nil && strings.Contains(string(d.Corps), "<!-- schema:seuils-pauvrete -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:seuils-pauvrete -->",
				`<figure class="schema">`+string(seuilsPauvrete.SVG)+`<figcaption>`+
					`<strong>Comment lire ce graphique</strong> : classez tous les Français du `+
					`niveau de vie le plus bas au plus haut, puis coupez cette file en dix tas `+
					`du même nombre de personnes — 10&nbsp;% de la population dans chaque tas, `+
					`pas 10&nbsp;% du revenu total. D1 est le tas le plus pauvre, D9 le neuvième. `+
					`Chaque barre est le plafond de son tas, `+fmt.Sprint(seuilsPauvrete.Annee)+
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
		if schemaTauxPauvrete != "" && strings.Contains(string(d.Corps), "<!-- schema:taux-pauvrete-serie -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:taux-pauvrete-serie -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaTauxPauvrete)+`</div>`+
					`<figcaption>Taux de pauvreté (seuil à 60 % du niveau de vie médian), 1996-2023 — `+
					`Insee. La refonte de l'enquête en 2021 (ERFS nouvelle formule) rend les deux `+
					`périodes non strictement comparables.</figcaption></figure>`))
		}
		if schemaDepenseEnv != "" && strings.Contains(string(d.Corps), "<!-- schema:depense-environnementale -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:depense-environnementale -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaDepenseEnv)+`</div>`+
					`<figcaption>Dépense de protection de l'environnement, ensemble de l'économie — `+
					`Eurostat (env_epea_neep), millions d'euros courants convertis en milliards.`+
					`</figcaption></figure>`))
		}
		if schemaEmploiTotal != "" && strings.Contains(string(d.Corps), "<!-- schema:emploi-total-salarie -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:emploi-total-salarie -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaEmploiTotal)+`</div>`+
					`<figcaption>Emploi total et salarié, France, 1975-2025 — Eurostat (nama_10_pe). `+
					`L'écart entre les deux courbes est le nombre de non-salariés.</figcaption></figure>`))
		}
		if statsClimatInternational != nil && strings.Contains(string(d.Corps), "<!-- schema:ratification-accord-paris -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:ratification-accord-paris -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsClimatInternational.CourbeRatificationSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Nombre cumulé de pays ayant ratifié l'Accord de Paris — ONU, `+
						`collection des traités. %d pays au total, dont %d n'ont jamais ratifié (signature seule, `+
						`ou aucune des deux).</figcaption></figure>`,
						statsClimatInternational.NbPays, statsClimatInternational.NbJamaisRatifie)))
		}
		if statsUnionEuropeenne != nil {
			if statsUnionEuropeenne.PIBSVG != "" && strings.Contains(string(d.Corps), "<!-- schema:pib-blocs -->") {
				d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:pib-blocs -->",
					`<figure class="schema">`+string(statsUnionEuropeenne.PIBSVG)+
						fmt.Sprintf(`<figcaption>PIB, dollars courants, %d — Banque mondiale.</figcaption></figure>`,
							statsUnionEuropeenne.AnneePIB)))
			}
			if statsUnionEuropeenne.SecteursSVG != "" && strings.Contains(string(d.Corps), "<!-- schema:secteurs-blocs -->") {
				d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:secteurs-blocs -->",
					`<figure class="schema"><div class="carte-pleine">`+string(statsUnionEuropeenne.SecteursSVG)+`</div>`+
						fmt.Sprintf(`<figcaption>Valeur ajoutée par secteur, %% du PIB, %d — Banque mondiale.`+
							`</figcaption></figure>`, statsUnionEuropeenne.AnneeSecteurs)))
			}
			if statsUnionEuropeenne.CommerceTable != "" && strings.Contains(string(d.Corps), "<!-- tableau:commerce-blocs -->") {
				d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:commerce-blocs -->",
					string(statsUnionEuropeenne.CommerceTable)))
			}
			if statsUnionEuropeenne.SecteursUSTable != "" && strings.Contains(string(d.Corps), "<!-- tableau:secteurs-us -->") {
				d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:secteurs-us -->",
					string(statsUnionEuropeenne.SecteursUSTable)))
			}
		}
		if carteBassins != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-bassins -->") {
			var legende strings.Builder
			legende.WriteString(`<div class="repartition-legende">`)
			for _, bs := range carteBassins.Bassins {
				fmt.Fprintf(&legende, `<div><i style="background:%s"></i><span>%s</span></div>`,
					bs.Couleur, template.HTMLEscapeString(bs.Nom))
			}
			legende.WriteString(`</div>`)
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-bassins -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteBassins.SVG)+`</div>`+
					legende.String()+
					`<figcaption>Les 7 bassins hydrographiques de France métropolitaine — les 6 `+
					`comités de bassin classiques plus la Corse, distincte hydrographiquement mais `+
					`rattachée administrativement à Rhône-Méditerranée (§ 1.1). Un découpage qui ne `+
					`suit aucune limite régionale ou départementale — le bassin Loire-Bretagne, le `+
					`plus vaste, traverse une douzaine de régions et départements actuels. `+
					`Source&nbsp;: BD Topage 2025, Sandre/IGN, Licence Ouverte.</figcaption></figure>`))
		}
		if carteEPTBEPAGE != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-eptb-epage -->") {
			legende := fmt.Sprintf(`<div class="repartition-legende">`+
				`<div><i style="background:%s"></i><span>EPTB (%d)</span></div>`+
				`<div><i style="background:%s"></i><span>EPAGE (%d)</span></div>`+
				`<div><i style="background:%s"></i><span>Double statut (%d)</span></div></div>`,
				couleursTypeEPTBEPAGE["EPTB"], carteEPTBEPAGE.NbEPTB,
				couleursTypeEPTBEPAGE["EPAGE"], carteEPTBEPAGE.NbEPAGE,
				couleursTypeEPTBEPAGE["EPTB_EPAGE"], carteEPTBEPAGE.NbDouble)
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-eptb-epage -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteEPTBEPAGE.SVG)+`</div>`+
					legende+
					fmt.Sprintf(`<figcaption>%d structures sur %d trouvées dans BANATIC affichées ici : `+
						`en dessous de %.0f%% de membres reconstruits (communes ou EPCI déjà chargés dans `+
						`ce dépôt), le contour serait un fragment épars, plus trompeur qu'utile. Les zones `+
						`grises n'appartiennent à aucun EPTB/EPAGE reconstruit — pas forcément à aucun `+
						`EPTB/EPAGE réel (§ 1.2). Source&nbsp;: BANATIC (DGCL), contour reconstruit par ce `+
						`dépôt, pas téléchargé comme tel.</figcaption></figure>`,
						carteEPTBEPAGE.NbAffiches, carteEPTBEPAGE.NbTrouves, seuilResolutionEPTBEPAGE*100)))
		}
		if schemaTendanceDepensesFiscales != "" && strings.Contains(string(d.Corps), "<!-- schema:depenses-fiscales-tendance -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:depenses-fiscales-tendance -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaTendanceDepensesFiscales)+`</div>`+
					`<figcaption>Total exécuté, année par année — chaque millésime du PLF ne publie l'exécution `+
					`que pour une seule année, jamais révisée dans un millésime ultérieur : sept millésimes, `+
					`sept années, sans doublon à trancher.</figcaption></figure>`))
		}
		if statsDepensesFiscales != nil && strings.Contains(string(d.Corps), "<!-- tableau:depenses-fiscales-top -->") {
			var t strings.Builder
			t.WriteString(`<div class="scroll"><table><thead><tr><th>Dispositif</th><th>Impôt</th><th>Coût</th></tr></thead><tbody>`)
			for _, dsp := range statsDepensesFiscales.TopDispositifs {
				fmt.Fprintf(&t, `<tr><td>%s</td><td>%s</td><td>%s M€</td></tr>`,
					template.HTMLEscapeString(dsp.Libelle), template.HTMLEscapeString(dsp.Impot), Nombre(int(dsp.MontantM)))
			}
			t.WriteString(`</tbody></table></div>`)
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:depenses-fiscales-top -->", t.String()))
		}
		if statsDepensesFiscales != nil && strings.Contains(string(d.Corps), "<!-- tableau:depenses-fiscales-impot -->") {
			var t strings.Builder
			t.WriteString(`<div class="scroll"><table><thead><tr><th>Impôt</th><th>Dispositifs</th><th>Coût cumulé</th></tr></thead><tbody>`)
			for _, it := range statsDepensesFiscales.ParImpot {
				fmt.Fprintf(&t, `<tr><td>%s</td><td>%s</td><td>%s Md€</td></tr>`,
					template.HTMLEscapeString(it.Impot), Nombre(it.NbDisp), Decimal(it.TotalMdEu, 1))
			}
			t.WriteString(`</tbody></table></div>`)
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:depenses-fiscales-impot -->", t.String()))
		}
		if carteIFI != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-ifi -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-ifi -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteIFI.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>%d communes publiées par la DGFiP pour %d (plus de 20 000 `+
						`habitants et plus de 50 redevables à l'IFI — un seuil de publication de la DGFiP, `+
						`pas de ce dépôt). La surface de chaque cercle est proportionnelle au nombre de `+
						`redevables ; la couleur est uniforme. Les zones sans cercle n'ont pas de commune `+
						`publiée à ce niveau, pas forcément aucun redevable à l'IFI.</figcaption></figure>`,
						carteIFI.NbCommunes, carteIFI.Annee)))
		}
		if statsAppareilProductif != nil && strings.Contains(string(d.Corps), "<!-- schema:glissement-sectoriel -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:glissement-sectoriel -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsAppareilProductif.GlissementSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Part de l'emploi total par secteur, France, %d à %d — Eurostat, `+
						`nama_10_a10_e. « Services » est ici le complément (total moins agriculture, industrie et `+
						`construction), pas une addition de branches publiées séparément.</figcaption></figure>`,
						statsAppareilProductif.AnneeDebutGlissement, statsAppareilProductif.AnneeFinGlissement)))
		}
		if statsAppareilProductif != nil && strings.Contains(string(d.Corps), "<!-- schema:delocalisation-annuelle -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:delocalisation-annuelle -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsAppareilProductif.DelocalisationAnnuelleSVG)+`</div>`+
					`<figcaption>Emplois en équivalent temps plein détectés comme délocalisés chaque année, `+
					`2001-2017 — la bande couvre les scénarios bas à haut du modèle Insee, la ligne est le `+
					`scénario central. Un chiffre encadré par une fourchette, pas une mesure exacte.</figcaption></figure>`))
		}
		if statsAppareilProductif != nil && strings.Contains(string(d.Corps), "<!-- schema:delocalisation-departement -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:delocalisation-departement -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsAppareilProductif.DelocalisationDeptSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Cumul 1995-2017 des emplois délocalisés (scénario central), par `+
						`département de résidence de l'entreprise — %d départements métropolitains. La surface de `+
						`chaque cercle est proportionnelle au nombre d'emplois.</figcaption></figure>`,
						statsAppareilProductif.NbDepartements)))
		}
		if statsAppareilProductif != nil && strings.Contains(string(d.Corps), "<!-- tableau:delocalisation-csp -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:delocalisation-csp -->",
				string(statsAppareilProductif.DelocalisationCSPTable)))
		}
		if statsAppareilProductif != nil {
			for _, marqueur := range []struct {
				cle string
				st  *StatsCommerceSecteur
			}{
				{"schema:commerce-automobile", statsAppareilProductif.CommerceAutomobile},
				{"schema:commerce-textile", statsAppareilProductif.CommerceTextile},
				{"schema:commerce-electronique-tv", statsAppareilProductif.CommerceElectroniqueTV},
			} {
				if marqueur.st == nil {
					continue
				}
				m := "<!-- " + marqueur.cle + " -->"
				if strings.Contains(string(d.Corps), m) {
					d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), m,
						`<figure class="schema"><div class="carte-pleine">`+string(marqueur.st.SVG)+`</div>`+
							fmt.Sprintf(`<figcaption>Part de chaque partenaire dans les importations françaises de %s, `+
								`%d (point clair) et %d (point plein) — UN Comtrade, valeurs en dollars courants. `+
								`Total mondial : %s Md$ en %d, %s Md$ en %d.</figcaption></figure>`,
								marqueur.st.Libelle, marqueur.st.AnneeDebut, marqueur.st.AnneeFin,
								Decimal(marqueur.st.TotalUSDDebut/1e9, 1), marqueur.st.AnneeDebut,
								Decimal(marqueur.st.TotalUSDFin/1e9, 1), marqueur.st.AnneeFin)))
				}
			}
		}
		if statsFrancophonie != nil && strings.Contains(string(d.Corps), "<!-- schema:francophonie-carte -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:francophonie-carte -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsFrancophonie.CarteSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Part de francophones dans la population, par pays, 2025 — ODSEF/OIF. `+
						`%d des %d pays souverains du fichier source sont repérés sur ce fond de carte ; les pays en gris `+
						`n'ont pas de correspondance dans le fond Natural Earth utilisé ici, pas nécessairement aucune donnée.</figcaption></figure>`,
						statsFrancophonie.NbPaysCartes, statsFrancophonie.NbPaysTotal)))
		}
		if statsFrancophonie != nil && strings.Contains(string(d.Corps), "<!-- tableau:francophonie-top-pct -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:francophonie-top-pct -->",
				string(statsFrancophonie.TopParPctTable)))
		}
		if statsFrancophonie != nil && strings.Contains(string(d.Corps), "<!-- tableau:francophonie-top-nombre -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:francophonie-top-nombre -->",
				string(statsFrancophonie.TopParNombreTable)))
		}
		if statsEmpireColonial != nil && strings.Contains(string(d.Corps), "<!-- schema:empire-colonial-carte -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:empire-colonial-carte -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsEmpireColonial.CarteSVG)+`</div>`+
					`<div class="echelle"><span><i class="vague-1"></i>1953-1956</span>`+
					`<span><i class="vague-2"></i>1958-1962</span>`+
					`<span><i class="vague-3"></i>1975-1977</span></div>`+
					fmt.Sprintf(`<figcaption>%d des %d territoires listés ont une géométrie CShapes ; `+
						`survolez chaque territoire pour sa date d'indépendance exacte.</figcaption></figure>`,
						statsEmpireColonial.NbCartes, statsEmpireColonial.NbTotal)))
		}
		if statsEmpireColonial != nil && strings.Contains(string(d.Corps), "<!-- tableau:empire-colonial-territoires -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- tableau:empire-colonial-territoires -->",
				string(statsEmpireColonial.Table)))
		}
		if statsSGM != nil && strings.Contains(string(d.Corps), "<!-- schema:sgm-ligne-demarcation -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:sgm-ligne-demarcation -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsSGM.CarteSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Tracé de la ligne de démarcation entre zone occupée et zone libre, `+
						`1940-1942 (%s km) — Département de l'Ain. Ni l'annexion de fait de l'Alsace-Moselle ni `+
						`la zone d'occupation italienne (à partir de novembre 1942) n'ont de géométrie vérifiée `+
						`trouvée ; non représentées ici, voir § 2.</figcaption></figure>`, Decimal(statsSGM.LongueurKm, 0))))
		}
		if populationGuerres != nil && strings.Contains(string(d.Corps), "<!-- schema:population-guerres -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:population-guerres -->",
				`<figure class="schema"><div class="carte-pleine">`+string(populationGuerres.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Population de la France (hors Mayotte), recensements 1876-1999 (Insee) — `+
						`entre 1911 et 1921, la population recule de %s à %s habitants, soit %s (%s %%). Aucun recul `+
						`comparable n'est visible entre 1936 et 1954 : la source ne publie aucun point en 1946, l'année `+
						`où le recul de la Seconde Guerre mondiale aurait été le plus visible.</figcaption></figure>`,
						Nombre(int(populationGuerres.Pop1911)), Nombre(int(populationGuerres.Pop1921)),
						Nombre(int(populationGuerres.BaisseAbsolue)), Decimal(populationGuerres.BaissePct, 1))))
		}
		if controleFiscal != nil && strings.Contains(string(d.Corps), "<!-- schema:controle-fiscal -->") {
			calcule := ""
			if controleFiscal.DernierNotifieCalcule {
				calcule = " (déduit de l'écart notifié/encaissé publié, non cité tel quel par la source)"
			}
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:controle-fiscal -->",
				`<figure class="schema"><div class="carte-pleine">`+string(controleFiscal.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Résultats du contrôle fiscal, France entière, 2015-%d (Sénat, commission `+
						`des finances) — en %d, %s Md€ notifiés%s contre %s Md€ effectivement encaissés. Le notifié `+
						`2022 et 2023 n'a été retrouvé dans aucune des trois sources primaires consultées : la ligne `+
						`pointillée s'interrompt sur ces deux années plutôt que d'être devinée.</figcaption></figure>`,
						controleFiscal.DernierAnnee, controleFiscal.DernierAnnee,
						Decimal(controleFiscal.DernierNotifie, 1), calcule, Decimal(controleFiscal.DernierEncaisse, 1))))
		}
		if schemaPortsFrancais != "" && strings.Contains(string(d.Corps), "<!-- schema:ports-francais -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:ports-francais -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaPortsFrancais)+`</div>`+
					`<figcaption>Trafic total (marchandises et tare, entrées et sorties confondues), `+
					`quatre grands ports maritimes, 2000-2025 — SDES.</figcaption></figure>`))
		}
		if schemaPortsEurope != "" && strings.Contains(string(d.Corps), "<!-- schema:ports-europe -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:ports-europe -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaPortsEurope)+`</div>`+
					`<figcaption>Trafic total, dernière année disponible par port (Eurostat, mar_go_aa) — `+
					`l'année diffère selon le port, indiquée entre parenthèses.</figcaption></figure>`))
		}
		if carteInfrastructurePorts != nil && strings.Contains(string(d.Corps), "<!-- schema:ports-infrastructure -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:ports-infrastructure -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteInfrastructurePorts.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Autoroutes (%d tronçons) et voies ferrées classées « voie portuaire » `+
						`par SNCF Réseau (%d tronçons) à moins de 80 km de chacun des quatre ports (OpenStreetMap, `+
						`SNCF Réseau) — un accès physique, pas une mesure de trafic : aucune donnée ouverte ne `+
						`distingue le fret des voyageurs sur le réseau ferré français.</figcaption></figure>`,
						carteInfrastructurePorts.NbAutoroutes, carteInfrastructurePorts.NbVoiesFerrees)))
		}
		if schemaReportModalPort != "" && strings.Contains(string(d.Corps), "<!-- schema:report-modal-port -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:report-modal-port -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaReportModalPort)+`</div>`+
					`<figcaption>Part du fer et du fleuve dans le pré- et post-acheminement des marchandises, `+
					`2023 (DGITM, Observatoire de la performance portuaire) — un seul millésime, pas une série. `+
					`Marseille-Fos n'a publié qu'un plafond, pas un chiffre exact.</figcaption></figure>`))
		}
		if schemaReportModalConteneurs != "" && strings.Contains(string(d.Corps), "<!-- schema:report-modal-conteneurs -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:report-modal-conteneurs -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaReportModalConteneurs)+`</div>`+
					`<figcaption>Répartition modale du transport de CONTENEURS vers l'arrière-pays — jamais à `+
					`comparer aux chiffres tous-trafics ci-dessus. HAROPA et Hambourg ne publient pas de `+
					`répartition conteneurs seuls vérifiable.</figcaption></figure>`))
		}
		if statsJustice != nil && strings.Contains(string(d.Corps), "<!-- schema:justice-surpeuplement -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:justice-surpeuplement -->",
				`<figure class="schema"><div class="carte-pleine">`+string(statsJustice.TopSurpeuplementSVG)+`</div>`+
					fmt.Sprintf(`<figcaption>Les vingt quartiers d'établissement les plus densément peuplés sur %d `+
						`lignes chargées (établissement × quartier), ministère de la Justice, dernière donnée mensuelle. `+
						`La ligne verticale marque 100 %% (capacité atteinte).</figcaption></figure>`, statsJustice.NbLignes)))
		}
		if schemaCarteSemiConducteurs != "" && strings.Contains(string(d.Corps), "<!-- schema:semi-conducteurs-carte -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:semi-conducteurs-carte -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaCarteSemiConducteurs)+`</div>`+
					`<figcaption>Cinq sites de production identifiés par leur unité légale Sirene, géocodés à la `+
					`commune — pas à l'adresse exacte de l'usine. La Cour des comptes relève elle-même l'absence de `+
					`cartographie officielle de cette filière (§ 2).</figcaption></figure>`))
		}
		if strings.Contains(string(d.Corps), "<!-- schema:contributif-non-contributif -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:contributif-non-contributif -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaContributifNonContributif)+`</div>`+
					`<figcaption>Protection sociale par risque, 2024 (DREES) — la vieillesse est presque `+
					`entièrement contributive, les soins et la famille presque entièrement non contributifs. `+
					`Classement discuté poste par poste au paragraphe suivant.</figcaption></figure>`))
		}
		if carteMusees != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-musees -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-musees -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteMusees.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>%d musées labellisés « Musée de France » (Muséofile, ministère de la `+
						`Culture), répartis sur %d départements — la surface de chaque cercle est proportionnelle `+
						`au nombre de musées, pas à leur taille ou à leur fréquentation.</figcaption></figure>`,
						carteMusees.NbMusees, carteMusees.NbDepartements)))
		}
		if schemaEcartPrixDOM != "" && strings.Contains(string(d.Corps), "<!-- schema:ecart-prix-dom -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:ecart-prix-dom -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaEcartPrixDOM)+`</div>`+
					`<figcaption>Écart de prix (indice de Fisher) avec la France métropolitaine, 2010 et 2022 `+
					`— Insee, enquête de comparaison spatiale des prix. Mayotte : donnée 2010 non disponible dans `+
					`la source.</figcaption></figure>`))
		}
		if schemaAlimentaireDOM != "" && strings.Contains(string(d.Corps), "<!-- schema:alimentaire-dom -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:alimentaire-dom -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaAlimentaireDOM)+`</div>`+
					`<figcaption>Écart général contre écart sur les seuls produits alimentaires et boissons non `+
					`alcoolisées, 2022 (Insee) — les deux séries ne se ressemblent pas, jamais à confondre.</figcaption></figure>`))
		}
		if carteEtudiants != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-etudiants -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-etudiants -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteEtudiants.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>%s étudiants répartis sur %d communes, rentrée %d (SIES) — la surface `+
						`de chaque cercle est proportionnelle à l'effectif. Paris apparaît par arrondissement, comme `+
						`dans la source.</figcaption></figure>`,
						Nombre(int(carteEtudiants.Total)), carteEtudiants.NbCommunes, carteEtudiants.Annee)))
		}
		if carteSRU != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-sru -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-sru -->",
				`<figure class="schema"><div class="carte-pleine">`+string(carteSRU.SVG)+`</div>`+
					`<div class="echelle"><span><i class="sru-conforme"></i>Conforme ou au-delà</span>`+
					`<span><i class="sru-deficitaire"></i>Déficitaire</span>`+
					`<span><i class="sru-carencee"></i>Carencée</span></div>`+
					fmt.Sprintf(`<figcaption>%d communes soumises à la loi SRU au 1ᵉʳ janvier 2025 (DGALN/DHUP), dont `+
						`%d carencées et %d déficitaires non carencées. Taille du cercle proportionnelle à la `+
						`population.</figcaption></figure>`, carteSRU.NbCommunes, carteSRU.NbCarencees, carteSRU.NbDeficitaires)))
		}
		if effortRecherche != nil && strings.Contains(string(d.Corps), "<!-- schema:effort-recherche -->") {
			estim := ""
			if effortRecherche.Estimation {
				estim = " (dernier point estimé par l'OCDE)"
			}
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:effort-recherche -->",
				`<figure class="schema"><div class="carte-pleine">`+string(effortRecherche.SVG)+`</div>`+
					fmt.Sprintf(`<figcaption>DIRD (dépense intérieure de recherche et développement) rapportée au PIB, `+
						`%d à %d (Insee, sources MESR-SIES et OCDE)%s. En %d, la part portée par les entreprises seules `+
						`(DIRDE/PIB) est de %s %% en France contre %s %% en UE27.</figcaption></figure>`,
						effortRecherche.AnneeDebut, effortRecherche.AnneeFin, estim, effortRecherche.AnneeFin,
						Decimal(effortRecherche.DirdeFrDernier, 2), Decimal(effortRecherche.DirdeUeDernier, 2))))
		}
		if schemaSIPRI != "" && strings.Contains(string(d.Corps), "<!-- schema:sipri-milex -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:sipri-milex -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaSIPRI)+`</div>`+
					`<figcaption>Dépense militaire, % du PIB, 1949-2025 — SIPRI. France, Russie et `+
					`Arabie saoudite nommées (leurs trajectoires sont les plus commentées) ; les six autres `+
					`pays de la comparaison restent en gris, chacun identifiable au survol de son point final.`+
					`</figcaption></figure>`))
		}
		if schemaHistoriqueImmigration != "" && strings.Contains(string(d.Corps), "<!-- schema:historique-immigration -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:historique-immigration -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaHistoriqueImmigration)+`</div>`+
					`<figcaption>Part d'immigrés dans la population, 1921-2025 — Insee, recensements. Les bandes `+
					`marquent les changements de champ ou de protocole (métropole puis France, Mayotte incluse `+
					`en 2014, protocole de collecte revu en 2024) : chaque régime se lit pour lui-même, pas comme `+
					`une évolution lissée d'un bout à l'autre.</figcaption></figure>`))
		}
		if schemaAgeDepartRetraite != "" && strings.Contains(string(d.Corps), "<!-- schema:age-depart-retraite -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:age-depart-retraite -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaAgeDepartRetraite)+`</div>`+
					`<figcaption>Âge conjoncturel moyen de départ à la retraite, 2004-2022 — Drees. Le creux de 2010 `+
					`précède la réforme qui recule ensuite progressivement l'âge légal.</figcaption></figure>`))
		}
		if schemaTauxRemplacement != "" && strings.Contains(string(d.Corps), "<!-- schema:taux-remplacement-retraite -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:taux-remplacement-retraite -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaTauxRemplacement)+`</div>`+
					`<figcaption>Dispersion du taux de remplacement (niveau de vie), cohorte 2020 — Drees. Boîte : `+
					`du 1ᵉʳ au 3ᵉ quartile, avec la médiane ; tige : du 1ᵉʳ au 9ᵉ décile. 100 = pension égale au `+
					`revenu d'avant la retraite.</figcaption></figure>`))
		}
		if strings.Contains(string(d.Corps), "<!-- schema:holding-mere-fille -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:holding-mere-fille -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaHoldingMereFille())+`</div>`+
					`<figcaption>Exemple pédagogique sur un bénéfice initial rond (100 000 €) ; les taux (25 % `+
					`d'IS, 5 % de quote-part, 1 % en cas d'intégration fiscale, 30 % de PFU) sont réels et `+
					`sourcés — voir le Cadre et les Enjeux ci-dessus. Le point central : l'IS de la filiale `+
					`est payé AVANT que le dividende n'existe, pas une fois qu'il est dans la holding.</figcaption></figure>`))
		}
		if strings.Contains(string(d.Corps), "<!-- schema:tva-entreprises -->") {
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:tva-entreprises -->",
				`<figure class="schema"><div class="carte-pleine">`+string(schemaTVAEntreprises())+`</div>`+
					`<figcaption>Exemple pédagogique (pas une chaîne de transactions réelles observées, voir `+
					`« Ce que les données ne disent pas ») : à chaque étage, TVA collectée moins TVA déductible `+
					`donne la TVA nette versée à l'État — la somme des trois retombe exactement sur la TVA payée `+
					`par le consommateur final.</figcaption></figure>`))
		}
		if carteMedecins != nil && strings.Contains(string(d.Corps), "<!-- schema:carte-medecins-generalistes -->") {
			c := carteMedecins.Page.Carte
			var echelle strings.Builder
			echelle.WriteString(`<div class="echelle"><span class="u">` + template.HTMLEscapeString(c.Unite) + `</span>`)
			for i, b := range c.Bornes {
				fmt.Fprintf(&echelle, `<span><i style="background:%s"></i>%s</span>`, c.Teintes[i], b)
			}
			echelle.WriteString(`</div>`)
			d.Corps = template.HTML(strings.ReplaceAll(string(d.Corps), "<!-- schema:carte-medecins-generalistes -->",
				`<figure class="schema"><div class="carte-pleine">`+string(c.SVG)+`</div>`+
					echelle.String()+
					`<figcaption>`+template.HTMLEscapeString(carteMedecins.Question)+` `+
					template.HTMLEscapeString(carteMedecins.Note)+` `+
					`<a href="`+root+`/collectivites/carte/medecins-generalistes/">Le classement des 101 `+
					`départements et la méthode →</a></figcaption></figure>`))
		}
	}

	if err := rattacherDocs(docs); err != nil {
		return err
	}
	reecrireLiensDocs(docs, root)
	return preparerSujets(ctx, e.pool, e.out, root, acc)
}

// construireComprendreEtSujets reprend verbatim la double boucle sur docs :
// chaque document est soit un sujet (sa propre section -only, l'ID du
// sujet — "fpctl build <sujet.id>" ne construit que celui-là), soit un
// document de méthode accumulé dans methode pour la section "comprendre".
func construireComprendreEtSujets(e *environnement, docs []*Doc) error {
	ts := e.page("sujet.gohtml")
	var methode []*Doc
	for _, d := range docs {
		s := sujetDuDoc(d.Slug)
		if s == nil {
			methode = append(methode, d)
			continue
		}
		if e.exclu(s.ID) {
			continue
		}
		l := e.layout
		l.Title = s.Nom
		if err := e.write(ts, filepath.Join(e.out, filepath.FromSlash(s.URL()), "index.html"), struct {
			Layout
			S *Sujet
		}{l, s}); err != nil {
			return err
		}
		if err := redirection(filepath.Join(e.out, "comprendre", d.Slug, "index.html"), e.root+"/"+s.URL()); err != nil {
			return err
		}
		if ancienneBase, deplace := anciennesBasesSujets[s.ID]; deplace {
			if err := redirection(filepath.Join(e.out, ancienneBase, s.ID, "index.html"), e.root+"/"+s.URL()); err != nil {
				return err
			}
		}
	}

	l := e.layout
	l.Title = "Documents de méthode"
	if err := e.ecrire("comprendre", e.page("comprendre.gohtml"), filepath.Join(e.out, "comprendre", "index.html"), struct {
		Layout
		Docs    []*Doc
		Groupes []GroupeDocs
	}{l, methode, GrouperDocs(methode)}); err != nil {
		return err
	}
	td := e.page("doc.gohtml")
	for _, d := range methode {
		var autres []*Doc
		for _, o := range methode {
			if o.Slug != d.Slug {
				autres = append(autres, o)
			}
		}
		l := e.layout
		l.Title = d.Titre
		if err := e.ecrire("comprendre", td, filepath.Join(e.out, "comprendre", d.Slug, "index.html"), struct {
			Layout
			D      *Doc
			Autres []*Doc
		}{l, d, autres}); err != nil {
			return err
		}
	}
	return nil
}

// construireFiches reprend verbatim les cinq boucles de fiches (personnes,
// candidats, organisations, référentiels, groupes) — toutes sous la même
// section -only "fiches", exactement comme cmd/fpctl/build.go les groupe
// déjà.
func construireFiches(e *environnement, d pipeline.Resultats) error {
	id := dep[identiteBundle](d, "identite")
	e27 := dep[*Stats2027](d, "election2027")
	locaux := dep[map[string]*RapprochementRNE](d, "mandats-locaux")

	par2027 := map[string]*Candidat2027{}
	for _, k := range e27.Candidats {
		par2027[k.Slug] = k
	}
	byCand := map[string]*Candidat{}
	for _, c := range id.Candidats {
		if c.Person != nil {
			byCand[c.Person.Slug] = c
		}
	}

	tp := e.page("personne.gohtml")
	for _, p := range id.Persons {
		l := e.layout
		l.Title = p.Prenom + " " + p.Nom
		data := struct {
			Layout
			P             *Person
			Cand          *Candidat
			Local         *RapprochementRNE
			K             *Candidat2027
			Defs          template.HTML
			TotalScrutins int
		}{l, p, byCand[p.Slug], nil, nil, "", e.layout.Cov.Scrutins}
		if c := byCand[p.Slug]; c != nil {
			data.Local, data.K = locaux[c.Slug], par2027[c.Slug]
			if data.K != nil && !data.K.Apercu.Vide {
				data.Defs = e27.Defs
			}
		}
		if err := e.ecrire("fiches", tp, filepath.Join(e.out, "depute", p.Slug, "index.html"), data); err != nil {
			return err
		}
	}
	for _, c := range id.Candidats {
		l := e.layout
		l.Title = c.Prenom + " " + c.Nom
		p := c.Person
		if p == nil {
			p = &Person{Slug: c.Slug, Prenom: c.Prenom, Nom: c.Nom}
		}
		k := par2027[c.Slug]
		var defs template.HTML
		if k != nil && !k.Apercu.Vide {
			defs = e27.Defs
		}
		data := struct {
			Layout
			P             *Person
			Cand          *Candidat
			Local         *RapprochementRNE
			K             *Candidat2027
			Defs          template.HTML
			TotalScrutins int
		}{l, p, c, locaux[c.Slug], k, defs, e.layout.Cov.Scrutins}
		if err := e.ecrire("fiches", tp, filepath.Join(e.out, "candidat", c.Slug, "index.html"), data); err != nil {
			return err
		}
	}

	to := e.page("organisation.gohtml")
	for _, o := range id.Orgs {
		l := e.layout
		l.Title = o.Libelle
		if err := e.ecrire("fiches", to, filepath.Join(e.out, "organisation", o.Slug, "index.html"), struct {
			Layout
			O *Organisation
		}{l, o}); err != nil {
			return err
		}
	}

	tr := e.page("referentiel.gohtml")
	for _, r := range id.Refs {
		var concernees []*Organisation
		for _, o := range id.OrgList {
			for _, c := range o.Classifications {
				if c.SetSlug == r.Slug {
					concernees = append(concernees, o)
					break
				}
			}
		}
		l := e.layout
		l.Title = r.Titre
		if err := e.ecrire("fiches", tr, filepath.Join(e.out, "referentiel", r.Slug, "index.html"), struct {
			Layout
			R    *Referentiel
			Orgs []*Organisation
		}{l, r, concernees}); err != nil {
			return err
		}
	}

	tg := e.page("groupe.gohtml")
	for _, g := range id.Groupes {
		l := e.layout
		l.Title = g.Nom
		if err := e.ecrire("fiches", tg, filepath.Join(e.out, "groupe", g.Slug, "index.html"), struct {
			Layout
			G *Groupe
		}{l, g}); err != nil {
			return err
		}
	}
	return nil
}

// construireScrutin reprend verbatim le court-circuit par empreinte que
// "scrutin" avait déjà avant ce graphe.
func construireScrutin(ctx context.Context, e *environnement, seuils map[string]Seuil) (int, error) {
	srcScrutins := SourceInfo{Attribution: "Assemblée nationale, open data, Licence Ouverte"}
	for _, s := range e.layout.Sources {
		if strings.Contains(strings.ToLower(s.Label), "scrutins") {
			srcScrutins = s
			break
		}
	}
	fmt.Printf("  avant les scrutins : %s écoulées\n", time.Since(e.start).Round(time.Second))

	scrutinsInchanges, etatScrutins, err := sectionInchangee(ctx, e.pool, e.ancienCacheValeur(), "scrutin")
	if err != nil {
		return 0, err
	}
	if e.maxScrutins == 0 {
		e.nouveauCache.Sections["scrutin"] = etatScrutins
	} else if prec, ok := e.ancienCacheValeur().Sections["scrutin"]; ok {
		e.nouveauCache.Sections["scrutin"] = prec
	}
	if e.maxScrutins == 0 && scrutinsInchanges {
		n := e.ancienCacheValeur().Sections["scrutin"].N
		etatScrutins.N = n
		e.nouveauCache.Sections["scrutin"] = etatScrutins
		fmt.Printf("    scrutins : données et gabarits inchangés, recopiés depuis le site précédent (%d)\n", n)
		if err := copierRepertoire(filepath.Join(e.siteActuel, "scrutin"), filepath.Join(e.out, "scrutin")); err != nil {
			return 0, err
		}
		return n, nil
	}
	n, err := buildScrutins(ctx, e.pool, e.page("scrutin.gohtml"), e.layout, e.out, e.maxScrutins, seuils, srcScrutins)
	if err != nil {
		return 0, err
	}
	if e.maxScrutins == 0 {
		etatScrutins.N = n
		e.nouveauCache.Sections["scrutin"] = etatScrutins
	}
	return n, nil
}

// sectionsVersNoeuds : chaque nom -only reconnu par ecrire() ailleurs dans
// ce fichier et graphe.go, vers le ou les nœuds du registre qui écrivent
// sous ce nom. Presque toujours un seul ; plusieurs quand une section
// historique s'est retrouvée à cheval sur deux nœuds séparés du graphe
// (vieillesse/sa carte, collectivités/ses cartes/ses pages locales) — sans
// cette table, demander -only=vieillesse n'aurait tiré que le premier des
// deux, l'autre n'étant relié à rien qui le rende atteignable depuis lui.
var sectionsVersNoeuds = map[string][]string{
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

// ciblesDe traduit -only en cibles pour Registre.Executer : vide veut dire
// tout (chaque nœud de sectionsVersNoeuds une seule fois), une valeur non
// reconnue est supposée être l'ID d'un sujet individuel (fpctl build
// <sujet-id>) et retombe sur "comprendre", le nœud qui les distribue tous.
func ciblesDe(only string) []string {
	if only == "" {
		var cibles []string
		for _, noms := range sectionsVersNoeuds {
			cibles = append(cibles, noms...)
		}
		sort.Strings(cibles) // plan reproductible d'un lancement à l'autre
		return cibles
	}
	var cibles []string
	vus := map[string]bool{}
	ajouterCibles := func(noms []string) {
		for _, n := range noms {
			if !vus[n] {
				vus[n] = true
				cibles = append(cibles, n)
			}
		}
	}
	for _, tok := range strings.Split(only, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if noms, ok := sectionsVersNoeuds[tok]; ok {
			ajouterCibles(noms)
		} else {
			ajouterCibles(sectionsVersNoeuds["comprendre"])
		}
	}
	return cibles
}
