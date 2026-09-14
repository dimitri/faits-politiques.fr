package fiscalite

import (
	"context"
	"fmt"
	"html"
	"os"
	"regexp"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

var SourceFaitsMultinationales = archive.Source{
	Slug: "faits-multinationales", Label: "Contrats publics, règlements fiscaux et constats d'enquête concernant des multinationales",
	Publisher:   "Sénat, Assemblée nationale, Conseil d'État, Agence française anticorruption, ministère de l'Économie ; presse et communiqués d'entreprise, signalés comme tels",
	Tier:        "PRIMARY_OFFICIAL",
	Licence:     "Documents publics des institutions, cités avec lien ; articles de presse cités sans reproduction",
	ReuseClass:  "ATTRIBUTION",
	Attribution: "Sources citées fait par fait (colonne source_url)",
	Cadence:     "au fil des faits",
	Notes: "Faits établis hors données ouvertes, transcrits un par un. La qualité dit qui établit le fait : " +
		"OFFICIEL (texte ou réponse d'une institution, page scellée), PRESSE (révélation d'un média, non " +
		"confirmée officiellement, page non archivée), ENTREPRISE (communiqué du groupe). Les montants " +
		"portent leur nature : un plafond d'accord-cadre n'est pas une dépense.",
}

type fait struct {
	id, groupe, typ, date, periode, cocontractant, acheteur, intitule string
	montant, nature, qualite, constat, url                            string
}

const urlSenat830 = "https://www.senat.fr/rap/r24-830-1/r24-830-11.pdf"

var faits = []fait{
	{"microsoft-defense-ppr-2017", "Microsoft Corporation", "CONTRAT", "2017-10-16", "2009-2021", "Microsoft Ireland Operations Limited", "Ministère de la Défense",
		"Accord-cadre de droits d'usage des logiciels Microsoft, dit « open bar »", "120000000", "ESTIME", "OFFICIEL",
		"Selon la proposition de résolution de douze sénateurs, le contrat a été conclu avec la société irlandaise plutôt qu'avec la filiale française, par procédure négociée sans publicité ni mise en concurrence, et renouvelé en 2013 puis fin 2016 ; 120 M€ pour 2013-2017, montant repris des révélations de la presse. Les auteurs y voient un défaut d'exemplarité fiscale de l'État.",
		"https://www.senat.fr/leg/ppr17-027.html"},
	{"microsoft-defense-reponse-2020", "Microsoft Corporation", "CONTRAT", "2020-01-09", "2017-2021", "", "Ministère des Armées",
		"Réponse du ministère sur le renouvellement du contrat Microsoft", "", "", "OFFICIEL",
		"Le ministère confirme une procédure négociée sans publicité ni mise en concurrence, fondée sur une attestation d'exclusivité de Microsoft, récuse le terme « open bar » et ne publie ni montant ni cocontractant.",
		"https://www.senat.fr/questions/base/2019/qSEQ191012547.html"},
	{"microsoft-defense-2009-2013", "Microsoft Corporation", "CONTRAT", "", "2009-2013", "", "Ministère de la Défense",
		"Premier contrat Microsoft du ministère de la Défense", "82000000", "PAYE", "PRESSE",
		"82 M€ dépensés sur la première période, selon les documents révélés par la presse spécialisée ; montant non confirmé par le ministère.",
		"https://www.silicon.fr/Thematique/actualites-1367/Breves/Contrat-Microsoft-Defense-l-Open-Bar-passe-de-82-a-439321.htm"},
	{"microsoft-education-2025", "Microsoft Corporation", "CONTRAT", "", "2025-2029", "", "Ministères de l'Éducation nationale et de l'Enseignement supérieur",
		"Accord-cadre de solutions Microsoft, présenté par le ministère comme passé « avec Microsoft »", "152000000", "PLAFOND", "OFFICIEL",
		"Le ministère décrit un accord-cadre de quatre ans avec Microsoft, sans minimum d'achat, plafonné à 152 M€ HT, couvrant près d'un million de postes et serveurs, et annonce une messagerie libre pour mi-2026.",
		"https://www.assemblee-nationale.fr/dyn/17/questions/QANR5L17QE5312"},
	{"microsoft-education-2025-titulaires", "Microsoft Corporation", "CONTRAT", "2025-03-14", "2025-2029", "Crayon France (lots 1 et 2), Open SAS (lot 3)", "Ministères de l'Éducation nationale et de l'Enseignement supérieur",
		"Titulaires et montant estimé du même accord-cadre", "74720000", "ESTIME", "OFFICIEL",
		"La commission d'enquête du Sénat précise que les licences Microsoft sont fournies par le revendeur Crayon France (64 M€ et 6 M€ estimés) et le support par Open SAS (4,72 M€), pour 74,72 M€ HT estimés sur quatre ans (rapport n° 830, p. 253-254).",
		urlSenat830},
	{"microsoft-ugap-2024", "Microsoft Corporation", "CONTRAT", "", "2024", "", "Union des groupements d'achats publics (UGAP)",
		"Ventes de produits Microsoft par la centrale d'achat publique", "230000000", "PAYE", "OFFICIEL",
		"Environ 230 M€ de ventes de produits Microsoft par l'UGAP en 2024 (100 M€ pour Oracle) ; au premier trimestre 2025, sept des dix prestations de services les plus vendues par l'UGAP concernaient des produits Microsoft (rapport n° 830, p. 266).",
		urlSenat830},
	{"microsoft-audition-senat-2025", "Microsoft Corporation", "CONTROVERSE", "2025-06-10", "", "Microsoft France", "Sénat",
		"Microsoft France ne peut pas garantir que les données ne seront pas transmises à des autorités étrangères", "", "", "OFFICIEL",
		"Auditionné sous serment, le directeur des affaires publiques et juridiques de Microsoft France répond « Non, je ne peux pas le garantir » à la question de la transmission de données de citoyens français à des autorités étrangères sans accord des autorités françaises (rapport n° 830, p. 242).",
		urlSenat830},
	{"oracle-ugap-2024", "Oracle Corporation", "CONTRAT", "", "2024", "", "Union des groupements d'achats publics (UGAP)",
		"Ventes de produits Oracle par la centrale d'achat publique", "100000000", "PAYE", "OFFICIEL",
		"Environ 100 M€ de ventes de produits Oracle par l'UGAP en 2024 (rapport n° 830, p. 266).",
		urlSenat830},
	{"aws-ugap-cloud-2020-2025", "Amazon.com Inc.", "CONTRAT", "", "2020-2025", "Crayon (distributeur), auparavant Capgemini", "État et opérateurs, via l'UGAP",
		"Marché d'hébergement en nuage de l'UGAP : part d'Amazon Web Services", "", "", "OFFICIEL",
		"146 M€ de commandes cumulées d'octobre 2020 à mai 2025 sur le marché cloud de l'UGAP, dont 8 % pour AWS, 19 % pour Microsoft et 37 % pour OVHcloud ; en 2024, 2,2 M€ pour AWS et 8,1 M€ pour Microsoft sur 44 M€. Le montant, qui couvre tous les fournisseurs, n'inclut ni le logiciel en ligne ni les achats hors UGAP (rapport n° 830, p. 266-267).",
		urlSenat830},
	{"aws-bpifrance-2022", "Amazon.com Inc.", "CONTRAT", "2022-03-22", "2020", "Amazon Web Services", "Bpifrance",
		"Plateforme des prêts garantis par l'État hébergée sur Amazon Web Services, sans appel d'offres", "", "", "OFFICIEL",
		"Le Gouvernement confirme que Bpifrance a recouru à Amazon Web Services pour la plateforme des attestations de prêts garantis par l'État, montée en moins de cinq jours, au motif que l'offre « n'avait pas d'équivalent » parmi ses hébergeurs référencés (Amazon, Microsoft, OVH) ; montant non publié.",
		"https://www.assemblee-nationale.fr/dyn/15/questions/QANR5L15QE36407"},
	{"aws-doctolib-2021", "Amazon.com Inc.", "CONTROVERSE", "2021-03-12", "", "AWS (hébergeur de Doctolib)", "Ministère de la Santé",
		"Rendez-vous de vaccination contre la covid-19 gérés par Doctolib, hébergé par AWS", "", "", "OFFICIEL",
		"Le juge des référés du Conseil d'État refuse de suspendre le partenariat : pas de données de santé, conservation limitée, chiffrement par un tiers de confiance établi en France.",
		"https://conseil-etat.fr/actualites/le-juge-des-referes-ne-suspend-pas-le-partenariat-entre-le-ministere-de-la-sante-et-doctolib-pour-la-gestion-des-rendez-vous-de-vaccination-contre"},
	{"google-education-2022", "Alphabet Inc.", "CONTROVERSE", "2022-11-15", "", "Google", "Ministère de l'Éducation nationale",
		"Arrêt du déploiement de Google Workspace et d'Office 365 dans les établissements", "", "", "OFFICIEL",
		"Le ministère indique avoir demandé aux recteurs, dès octobre 2021, d'arrêter tout déploiement d'Office 365 « ainsi que celle de Google, qui seraient contraires au RGPD ».",
		"https://www.assemblee-nationale.fr/dyn/16/questions/QANR5L16QE971"},
	{"s3ns-secnumcloud-2025", "S3NS (Thales-Google Cloud)", "CONTROVERSE", "2025-12-17", "", "S3NS", "Organismes publics et privés",
		"Qualification SecNumCloud de l'offre de S3NS, bâtie sur Google Cloud", "", "", "ENTREPRISE",
		"Coentreprise de Thales et de Google Cloud, S3NS annonce la qualification SecNumCloud de son offre PREMI3NS ; en juillet 2025, le Sénat relevait que Bleu et S3NS étaient encore en cours de qualification (rapport n° 830, p. 249).",
		"https://www.s3ns.io/en/news/premi3ns-secnumcloud-qualification"},
	{"capgemini-health-data-hub", "Capgemini SE (groupe français)", "CONTRAT", "", "2018-2019", "Capgemini", "Direction de la recherche, des études, de l'évaluation et des statistiques (Drees)",
		"Appui à la préfiguration de la plateforme des données de santé, finalement hébergée sur Microsoft Azure", "", "", "OFFICIEL",
		"La phase de préfiguration de la plateforme des données de santé a été menée sous l'égide de la Drees avec l'appui du cabinet Capgemini ; la commission d'enquête n'a pas eu accès à des éléments attestant d'une réelle consultation d'autres hébergeurs (rapport n° 830, p. 165).",
		urlSenat830},
	{"microsoft-health-data-hub-2020", "Microsoft Corporation", "CONTROVERSE", "", "2020", "Microsoft", "Plateforme des données de santé (Health Data Hub)",
		"Hébergement des données de santé sur Microsoft Azure", "", "", "OFFICIEL",
		"Le Conseil d'État refuse de suspendre l'hébergement par Microsoft mais demande des précautions dans l'attente d'une solution pérenne, en raison du risque de transfert de données vers les États-Unis.",
		"https://www.conseil-etat.fr/actualites/health-data-hub-et-protection-de-donnees-personnelles-des-precautions-doivent-etre-prises-dans-l-attente-d-une-solution-perenne"},
	{"bleu-lancement-2024", "Bleu (Orange-Capgemini, technologies Microsoft)", "CONTROVERSE", "", "2024", "Bleu", "État, collectivités, hôpitaux, opérateurs d'importance vitale",
		"Lancement commercial de Bleu, « cloud de confiance » bâti sur Microsoft 365 et Azure", "", "", "ENTREPRISE",
		"Coentreprise d'Orange et de Capgemini, Bleu exploite sous licence les services Microsoft 365 et Azure pour l'État et les organismes publics, en visant la qualification SecNumCloud ; Microsoft est rémunéré par les licences, dont le montant n'est pas public.",
		"https://www.capgemini.com/fr-fr/actualites/communiques-de-presse/capgemini-et-orange-annoncent-le-lancement-des-activites-commerciales-de-bleu-leur-future-plateforme-de-cloud-de-confiance/"},
	{"google-cjip-2019-amende", "Alphabet Inc.", "REGULARISATION", "", "2019 (faits 2005-2018)", "Google France et Google Ireland Limited", "Parquet national financier",
		"Convention judiciaire d'intérêt public : amende", "500000000", "AMENDE", "OFFICIEL",
		"Amende d'intérêt public de 500 M€ pour clore les poursuites pour fraude fiscale aggravée ; l'administration soutenait que Google Ireland exerçait en France une activité imposable.",
		"https://www.agence-francaise-anticorruption.gouv.fr/fr/document/convention-judiciaire-dinteret-public-cjip-conclue-entre-parquet-national-financier-et-societes-sarl"},
	{"google-cjip-2019-impot", "Alphabet Inc.", "REGULARISATION", "", "2019 (faits 2005-2018)", "Google France et Google Ireland Limited", "Administration fiscale",
		"Règlement fiscal associé à la convention judiciaire", "465000000", "IMPOT", "OFFICIEL",
		"465 M€ de droits réglés à l'administration fiscale, en plus de l'amende.",
		"https://www.agence-francaise-anticorruption.gouv.fr/fr/document/convention-judiciaire-dinteret-public-cjip-conclue-entre-parquet-national-financier-et-societes-sarl"},
	{"mcdonalds-2022-amende", "McDonald's Corporation", "REGULARISATION", "2022-06-16", "", "McDonald's France", "Parquet national financier",
		"Convention judiciaire d'intérêt public : amende", "508000000", "AMENDE", "OFFICIEL",
		"Amende d'intérêt public de 508 M€ pour fraude fiscale : taux de redevance versée à la société mère luxembourgeoise jugé artificiellement gonflé.",
		"https://presse.economie.gouv.fr/16-06-2022-la-direction-generale-des-finances-publiques-salue-le-reglement-du-litige-relatif-a-limposition-de-mc-donalds-en-france/"},
	{"mcdonalds-2022-impot", "McDonald's Corporation", "REGULARISATION", "2022-06-16", "", "McDonald's France", "Administration fiscale",
		"Règlement fiscal associé", "737000000", "IMPOT", "OFFICIEL",
		"737 M€ d'impôt sur les sociétés réglés à l'administration fiscale.",
		"https://presse.economie.gouv.fr/16-06-2022-la-direction-generale-des-finances-publiques-salue-le-reglement-du-litige-relatif-a-limposition-de-mc-donalds-en-france/"},
	{"mckinsey-is-2022", "McKinsey & Company", "CONSTAT_FISCAL", "2022-03-16", "2011-2020", "McKinsey & Company Inc. France et McKinsey & Company SAS", "",
		"Aucun impôt sur les sociétés payé en France pendant au moins dix ans", "329000000", "CHIFFRE_AFFAIRES", "OFFICIEL",
		"Chiffre d'affaires de 329 M€ en France en 2020, impôt sur les sociétés nul de 2011 à 2020 : les prix de transfert versés à la maison mère du Delaware ramènent le résultat imposable à zéro.",
		"https://www.senat.fr/rap/r21-578-1/r21-578-121.html"},
	{"mckinsey-parjure-2022", "McKinsey & Company", "CONTROVERSE", "2022-03-25", "", "McKinsey & Company", "Sénat",
		"Saisine de la justice pour faux témoignage", "", "", "OFFICIEL",
		"Le Sénat saisit la justice : un dirigeant avait affirmé sous serment que le cabinet payait l'impôt sur les sociétés en France.",
		"https://www.senat.fr/salle-de-presse/communiques-de-presse/presse/cp20220325.html"},
	{"palantir-dgsi-2025", "Palantir Technologies Inc.", "CONTRAT", "", "2025", "Palantir Technologies", "Direction générale de la sécurité intérieure",
		"Renouvellement pour trois ans du contrat de la DGSI", "", "", "OFFICIEL",
		"Une question écrite au Sénat relève le renouvellement pour trois ans d'un partenariat noué en 2015-2016 et présenté alors comme transitoire ; montant non public.",
		"https://www.senat.fr/questions/base/2025/qSEQ251207120.html"},
	{"netflix-facturation-2021", "Netflix Inc.", "CONSTAT_FISCAL", "2021-01-01", "", "Netflix Services France", "",
		"Les abonnés français facturés par la société française", "", "", "PRESSE",
		"Depuis janvier 2021, les abonnés français contractent avec Netflix Services France et non plus avec la filiale néerlandaise ; le chiffre d'affaires de la société française passe de 47 M€ (2020) à 1,2 Md€ (2021).",
		"https://www.satellifacts.com/fr/tour/news/288306/netflix-plateforme-va-declarer-francais-france-2021-avise-abonnes.html"},
}

// Ce que chaque page officielle doit contenir pour fonder son fait.
var attendusFaits = map[string][]string{
	"https://www.senat.fr/leg/ppr17-027.html":                                           {"Microsoft", "Irlande"},
	"https://www.senat.fr/questions/base/2019/qSEQ191012547.html":                       {"Microsoft", "mise en concurrence"},
	"https://www.assemblee-nationale.fr/dyn/17/questions/QANR5L17QE5312":                {"Microsoft", "152"},
	"https://www.senat.fr/rap/r21-578-1/r21-578-121.html":                               {"McKinsey"},
	"https://www.senat.fr/salle-de-presse/communiques-de-presse/presse/cp20220325.html": {"McKinsey"},
	"https://www.senat.fr/questions/base/2025/qSEQ251207120.html":                       {"Palantir"},
	"https://presse.economie.gouv.fr/16-06-2022-la-direction-generale-des-finances-publiques-salue-le-reglement-du-litige-relatif-a-limposition-de-mc-donalds-en-france/": {"Donald"},
}

func IngestFaits(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	return executer(ctx, arch, SourceFaitsMultinationales, func(srcID, runID int64) (map[string]any, error) {
		docs := map[string]any{}
		archives, echecs := 0, 0
		for _, f := range faits {
			if _, fait := docs[f.url]; fait {
				continue
			}
			if f.qualite == "PRESSE" {
				docs[f.url] = nil // cité, pas archivé
				continue
			}
			ext := ".html"
			if strings.HasSuffix(f.url, ".pdf") {
				ext = ".pdf" // scellé sans relecture : pas de lecteur PDF dans le projet
			}
			d, err := arch.Fetch(ctx, srcID, runID, f.url, ext)
			if err != nil {
				if f.qualite == "OFFICIEL" {
					return nil, fmt.Errorf("%s : %w", f.id, err)
				}
				docs[f.url] = nil
				echecs++
				continue
			}
			if att, ok := attendusFaits[f.url]; ok {
				t, err := texteHTML(d.Path)
				if err != nil {
					return nil, err
				}
				for _, a := range att {
					if !strings.Contains(t, a) {
						return nil, fmt.Errorf("%s : « %s » absent de %s", f.id, a, f.url)
					}
				}
			}
			docs[f.url] = d.DocumentID
			archives++
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `DELETE FROM ref.fait_multinationale`); err != nil {
			return nil, err
		}
		for _, f := range faits {
			if _, err := tx.Exec(ctx, `INSERT INTO ref.fait_multinationale
				(id, groupe, type, date_fait, periode, cocontractant, acheteur, intitule, montant_eur, nature_montant,
				 qualite, constat, source_url, source_id, document_id)
				VALUES ($1,$2,$3,$4::date,$5,$6,$7,$8,$9::numeric,$10,$11,$12,$13,$14,$15)`,
				f.id, f.groupe, f.typ, nul(f.date), nul(f.periode), nul(f.cocontractant), nul(f.acheteur), f.intitule,
				nul(f.montant), nul(f.nature), f.qualite, f.constat, f.url, srcID, docs[f.url]); err != nil {
				return nil, fmt.Errorf("%s : %w", f.id, err)
			}
		}
		return map[string]any{"faits": len(faits), "pages_archivees": archives, "echecs_non_officiels": echecs}, tx.Commit(ctx)
	})
}

var (
	reBalisesHTML = regexp.MustCompile(`(?s)<script.*?</script>|<style.*?</style>|<[^>]+>`)
	reBlancsHTML  = regexp.MustCompile(`\s+`)
)

// texteHTML rend le texte lisible d'une page scellée, pour vérifier qu'elle dit
// encore ce qu'on lui fait dire.
func texteHTML(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	t := html.UnescapeString(reBalisesHTML.ReplaceAllString(string(b), " "))
	t = strings.NewReplacer("\u00a0", " ", "\u202f", " ").Replace(t)
	return reBlancsHTML.ReplaceAllString(t, " "), nil
}
