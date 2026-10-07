package dossiers

// Souveraineté numérique : le contexte tiré des débats de l'Assemblée, les
// solutions françaises nommées par les sources, les mots suivis et les acteurs.

func init() {
	facts = append(facts, factsSouverainetePlus...)
	terms = append(terms,
		Term{dossierSouv, "souveraineté numérique", `souveraineté numérique`},
		Term{dossierSouv, "SecNumCloud", `secnumcloud`},
		Term{dossierSouv, "Cloud Act, extraterritorialité", `cloud act|extraterritorial`},
		Term{dossierSouv, "logiciel libre", `logiciels? libres?`},
	)
	actors = append(actors, actorsNumeriques...)
}

const (
	themeSolutions  = "solutions françaises"
	themeDebats     = "débats à l'Assemblée"
	ficheSirene     = "https://annuaire-entreprises.data.gouv.fr/entreprise/"
	urlCatalogueSNC = "https://messervices.cyber.gouv.fr/visas/catalogue-produits-services-profils-de-protection-sites-certifies-qualifies-agrees-anssi.pdf"
)

var factsSouverainetePlus = []Fact{
	// Contexte : ce que des membres du Gouvernement ont dit en séance.
	{ID: "chappaz-secnumcloud-2025-02-12", Dossier: dossierSouv, Section: "CONTEXTE", Theme: themeDebats, Type: "DECLARATION",
		Date: "2025-02-12", Author: "Ministre déléguée chargée de l'intelligence artificielle et du numérique", Person: "clara-chappaz",
		Title: "Une stratégie de souveraineté numérique appuyée sur SecNumCloud depuis 2021",
		Finding: "Depuis 2021, avec l'ANSSI, la stratégie de sécurisation des données s'appuie notamment sur SecNumCloud ; un appel à " +
			"projets du plan France 2030 vise à faire monter en compétence des acteurs comme OVHcloud ou Scaleway.",
		Quality: "DECLARATIF", Session: "2025-02-12",
		Expected: []string{"notre stratégie de sécurisation des données s'appuie notamment sur SecNumCloud", "OVHcloud ou Scaleway"}},
	{ID: "chappaz-health-data-hub-2025-04-08", Dossier: dossierSouv, Section: "CONTEXTE", Theme: themeDebats, Type: "DECLARATION",
		Date: "2025-04-08", Author: "Ministre déléguée chargée de l'intelligence artificielle et du numérique", Person: "clara-chappaz",
		Title: "Le Health Data Hub sans « hébergeur ultrasécurisé » : un appel d'offres de migration annoncé",
		Finding: "Les données du système national des données de santé ne sont pas dans le Health Data Hub faute d'hébergeur " +
			"ultrasécurisé ; la ministre annonce un appel d'offres pour le migrer vers un hébergeur sécurisé.",
		Quality: "DECLARATIF", Session: "2025-04-08",
		Expected: []string{"n'a pas d'hébergeur ultrasécurisé", "lancer un appel d'offres pour faire migrer le Health Data Hub"}},
	{ID: "ferracci-data-centers-2025-04-30", Dossier: dossierSouv, Section: "CONTEXTE", Theme: themeDebats, Type: "DECLARATION",
		Date: "2025-04-30", Author: "Ministre chargé de l'industrie et de l'énergie", Person: "marc-ferracci",
		Title: "Les centres de données, élément d'une souveraineté numérique",
		Finding: "Défendant un article de loi sur l'implantation des centres de données, le ministre y voit la capacité de " +
			"s'approprier les éléments d'une souveraineté numérique que d'autres pays construisent.",
		Quality: "DECLARATIF", Session: "2025-04-30",
		Expected: []string{"nous approprier les éléments d'une souveraineté numérique"}},
	{ID: "montchalin-data-centers-2025-11-17", Dossier: dossierSouv, Section: "CONTEXTE", Theme: themeDebats, Type: "DECLARATION",
		Date: "2025-11-17", Author: "Ministre de l'action et des comptes publics", Person: "amelie-de-montchalin",
		Title: "La fiscalité des centres de données et le plan France 2030",
		Finding: "Le Gouvernement s'oppose à la suppression d'une réduction fiscale pour les centres de données, au nom de la " +
			"souveraineté numérique soutenue financièrement par le plan France 2030.",
		Quality: "DECLARATIF", Session: "2025-11-17",
		Expected: []string{"il y va de la souveraineté numérique que nous soutenons financièrement dans le cadre du plan France 2030"}},

	// Situation : les solutions françaises nommées par les sources.
	{ID: "latombe-hebergeurs-francais-2025", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "DECLARATION",
		Date: "2025-04-08", Author: "Député, rapporteur d'une mission d'information sur la souveraineté numérique (audition au Sénat)",
		Person: "philippe-latombe",
		Title:  "Des hébergeurs français nommés : OVH, Scaleway, NumSpot, Outscale, Cloud Temple",
		Finding: "Devant la commission d'enquête du Sénat, le député cite OVH, Scaleway, NumSpot, Outscale et Cloud Temple parmi les " +
			"entreprises françaises capables d'héberger des données publiques.",
		URL: urlSenat830, Page: "267-268", Quality: "DECLARATIF",
		Expected: []string{"je citerai OVH, Scaleway et NumSpot"}},
	{ID: "ovhcloud-senat-2025", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "DECLARATION",
		Author: "OVHcloud (rencontre avec la commission d'enquête du Sénat)", Group: "OVH Groupe",
		Title: "OVHcloud : 3 000 salariés, un milliard d'euros de chiffre d'affaires, 350 M€ d'investissement par an",
		Finding: "Chiffres donnés à la commission lors de sa rencontre avec le président d'OVHcloud : environ 3 000 salariés, un " +
			"chiffre d'affaires de l'ordre d'un milliard d'euros, 350 M€ investis chaque année, 600 M€ visés en 2030.",
		URL: urlSenat830, Page: "268", Quality: "DECLARATIF",
		Expected: []string{"emploie ainsi 3 000 personnes", "de l'ordre d'un milliard d'euros"}},
	{ID: "poupard-acteurs-prets-2025", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "DECLARATION",
		Date: "2025-05-27", Author: "Directeur général adjoint de Docaposte, ancien directeur général de l'ANSSI (audition au Sénat)",
		Title: "« Des acteurs français sont prêts », avec un catalogue moins complet",
		Finding: "Selon l'ancien directeur de l'ANSSI, les fournisseurs Cloud français proposent les services dont l'État a besoin, " +
			"même si leur catalogue est moins complet que celui des grands groupes américains.",
		URL: urlSenat830, Page: "268", Quality: "DECLARATIF",
		Expected: []string{"des acteurs français sont prêts"}},
	{ID: "gendarmerie-linux-2009", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "CONSTAT",
		Author: senatCommande,
		Title:  "La gendarmerie nationale sous Linux depuis 2009",
		Finding: "La gendarmerie nationale a adopté le système d'exploitation libre Linux en 2009 ; elle n'est pas concernée par le " +
			"remplacement de postes qu'impose la fin de Windows 10 à la police nationale.",
		URL: urlSenat830, Page: "259", Quality: "OFFICIEL",
		Expected: []string{"La gendarmerie nationale, qui a adopté le système d'exploitation open source Linux depuis 2009"}},
	{ID: "education-open-source-2025", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "DECLARATION",
		Date: "2025-06-03", Author: "Sous-directeur du socle numérique, direction du numérique pour l'éducation (audition au Sénat)",
		Title: "Le système d'information de l'Éducation nationale « basé à 98 % sur de l'open source »",
		Finding: "Selon le ministère, son système d'information repose à 98 % sur des logiciels libres (Red Hat Linux notamment), les " +
			"licences Microsoft servant surtout aux postes de travail.",
		URL: urlSenat830, Page: "253", Quality: "DECLARATIF",
		Expected: []string{"98 % sur de l'open source"}},
	{ID: "suite-numerique-dinum-2025", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "TEXTE",
		Date: "2025-04-22", Author: "Ministres de l'Action publique, des Comptes publics et du Numérique (courrier aux membres du Gouvernement)",
		Title: "Des offres de l'État : clouds interministériels et « La Suite numérique »",
		Finding: "Le courrier invite les ministères à recourir aux offres Cloud qualifiées SecNumCloud ou aux clouds interministériels " +
			"(cloud Pi de l'Intérieur, Nubo des Finances), et aux outils collaboratifs de la DINUM, « La Suite numérique ».",
		URL: urlSenat830, Page: "256", Quality: "OFFICIEL",
		Expected: []string{"« La Suite numérique »", "le cloud PI du ministère de l'intérieur"}},
	{ID: "sitzenstuhl-stmicroelectronics-2025-05-27", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "DECLARATION",
		Date: "2025-05-27", Author: "Député", Person: "charles-sitzenstuhl",
		Title: "STMicroelectronics en débat : présence industrielle et participation publique",
		Finding: "En séance, le député rappelle que Bpifrance est actionnaire de STMicroelectronics et défend son développement " +
			"face aux objections sur sa consommation d'eau.",
		Quality: "DECLARATIF", Session: "2025-05-27",
		Expected: []string{"est actionnaire de STMicroelectronics"}},
	{ID: "tavel-stmicroelectronics-2025-05-27", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "DECLARATION",
		Date: "2025-05-27", Author: "Député", Person: "matthias-tavel",
		Title: "STMicroelectronics en débat : suppressions d'emplois annoncées",
		Finding: "Dans le même débat, le député interroge l'accord de Bpifrance, actionnaire, à la suppression annoncée d'un millier " +
			"d'emplois en France par STMicroelectronics.",
		Quality: "DECLARATIF", Session: "2025-05-27",
		Expected: []string{"STMicroelectronics s'apprête à supprimer 1 000 emplois en France"}},
}

var actorsNumeriques = []Actor{
	// Semi-conducteurs. Le code d'activité Sirene 26.11Z est « fabrication de
	// composants électroniques » : il ne dit pas quelles puces sont produites.
	{Siren: "399395581", Name: "STMicroelectronics (Crolles 2)", Category: "SEMI_CONDUCTEURS", Group: "STMicroelectronics N.V.",
		Basis:   "Unité légale Sirene, activité 26.11Z (fabrication de composants électroniques) ; bénéficiaire du projet « Liberty » d'usine de semi-conducteurs à Crolles (registre européen des aides d'État)",
		Quality: "OFFICIEL", URL: ficheSirene + "399395581", Check: "SIRENE", Pattern: `^STMICROELECTRONICS \(CROLLES 2\)`},
	{Siren: "341459386", Name: "STMicroelectronics France", Category: "SEMI_CONDUCTEURS", Group: "STMicroelectronics N.V.",
		Basis: "Unité légale Sirene, activité 26.11Z", Quality: "OFFICIEL", URL: ficheSirene + "341459386", Check: "SIRENE", Pattern: `^STMICROELECTRONICS FRANCE$`},
	{Siren: "414969584", Name: "STMicroelectronics Rousset", Category: "SEMI_CONDUCTEURS", Group: "STMicroelectronics N.V.",
		Basis: "Unité légale Sirene, activité 26.11Z", Quality: "OFFICIEL", URL: ficheSirene + "414969584", Check: "SIRENE", Pattern: `^STMICROELECTRONICS ROUSSET`},
	{Siren: "380932590", Name: "STMicroelectronics Tours", Category: "SEMI_CONDUCTEURS", Group: "STMicroelectronics N.V.",
		Basis: "Unité légale Sirene, activité 26.11Z", Quality: "OFFICIEL", URL: ficheSirene + "380932590", Check: "SIRENE", Pattern: `^STMICROELECTRONICS \(TOURS\)`},
	{Siren: "504941337", Name: "STMicroelectronics (Grenoble 2)", Category: "SEMI_CONDUCTEURS", Group: "STMicroelectronics N.V.",
		Basis: "Unité légale Sirene, activité 72.19Z (recherche-développement)", Quality: "OFFICIEL", URL: ficheSirene + "504941337", Check: "SIRENE", Pattern: `^STMICROELECTRONICS \(GRENOBLE 2\)`},
	{Siren: "384711909", Name: "Soitec", Category: "SEMI_CONDUCTEURS",
		Basis:   "Unité légale Sirene, activité 26.11Z ; se présente comme fabricant de matériaux pour semi-conducteurs",
		Quality: "DECLARATIF", URL: "https://www.soitec.com/fr", Check: "PAGE", Expected: []string{"matériaux innovants pour semi-conducteurs"}},

	// Cloud : location d'ordinateurs dans des salles serveurs.
	{Siren: "424761419", Name: "OVH (OVHcloud)", Category: "CLOUD", Group: "OVH Groupe",
		Basis: "Services qualifiés SecNumCloud au catalogue de l'ANSSI", Quality: "OFFICIEL", URL: urlCatalogueSNC, Check: "SECNUMCLOUD"},
	{Siren: "433115904", Name: "Scaleway", Category: "CLOUD", Group: "groupe iliad",
		Basis:   "Unité légale Sirene, activité 63.11Z (traitement de données, hébergement) ; se présente comme filiale du groupe iliad",
		Quality: "DECLARATIF", URL: "https://www.scaleway.com/fr/a-propos/", Check: "PAGE", Expected: []string{"Filiale du Groupe iliad"}},
	{Siren: "527594493", Name: "Outscale", Category: "CLOUD",
		Basis: "Service qualifié SecNumCloud au catalogue de l'ANSSI", Quality: "OFFICIEL", URL: urlCatalogueSNC, Check: "SECNUMCLOUD"},
	{Siren: "825400336", Name: "Cloud Temple", Category: "CLOUD",
		Basis: "Services qualifiés SecNumCloud au catalogue de l'ANSSI", Quality: "OFFICIEL", URL: urlCatalogueSNC, Check: "SECNUMCLOUD"},
	{Siren: "948608948", Name: "Numspot", Category: "CLOUD",
		Basis: "Service qualifié SecNumCloud au catalogue de l'ANSSI", Quality: "OFFICIEL", URL: urlCatalogueSNC, Check: "SECNUMCLOUD"},
	{Siren: "345039416", Name: "Orange Business Services", Category: "CLOUD", Group: "Orange",
		Basis: "Service qualifié SecNumCloud au catalogue de l'ANSSI", Quality: "OFFICIEL", URL: urlCatalogueSNC, Check: "SECNUMCLOUD"},
	{Siren: "908211980", Name: "Thales Cloud Sécurisé (offre S3NS)", Category: "CLOUD", Group: "Thales",
		Basis: "Services qualifiés SecNumCloud au catalogue de l'ANSSI, sur technologie Google Cloud", Quality: "OFFICIEL", URL: urlCatalogueSNC, Check: "SECNUMCLOUD"},
	{Siren: "378901946", Name: "Worldline", Category: "CLOUD",
		Basis: "Service qualifié SecNumCloud au catalogue de l'ANSSI", Quality: "OFFICIEL", URL: urlCatalogueSNC, Check: "SECNUMCLOUD"},

	// Logiciel : éditeurs qualifiés ou prestataires du logiciel libre.
	{Siren: "384351599", Name: "Index Éducation (Pronote)", Category: "LOGICIEL",
		Basis: "Services SaaS qualifiés SecNumCloud au catalogue de l'ANSSI", Quality: "OFFICIEL", URL: urlCatalogueSNC, Check: "SECNUMCLOUD"},
	{Siren: "528893522", Name: "Cloud Solutions (Wimi)", Category: "LOGICIEL",
		Basis: "Services SaaS qualifiés SecNumCloud au catalogue de l'ANSSI", Quality: "OFFICIEL", URL: urlCatalogueSNC, Check: "SECNUMCLOUD"},
	{Siren: "432735082", Name: "Oodrive", Category: "LOGICIEL",
		Basis: "Services SaaS qualifiés SecNumCloud au catalogue de l'ANSSI", Quality: "OFFICIEL", URL: urlCatalogueSNC, Check: "SECNUMCLOUD"},
	{Siren: "519139497", Name: "Whaller", Category: "LOGICIEL",
		Basis: "Service SaaS qualifié SecNumCloud au catalogue de l'ANSSI", Quality: "OFFICIEL", URL: urlCatalogueSNC, Check: "SECNUMCLOUD"},
	{Siren: "431473669", Name: "Linagora", Category: "LOGICIEL",
		Basis:   "Unité légale Sirene, activité 62.02A (conseil en systèmes et logiciels) ; prestataire déclaré au socle interministériel de logiciels libres",
		Quality: "OFFICIEL", URL: ficheSirene + "431473669", Check: "SIRENE", Pattern: `^LINAGORA$`},
	{Siren: "477865281", Name: "XWiki", Category: "LOGICIEL",
		Basis:   "Unité légale Sirene, activité 63.11Z ; prestataire déclaré au socle interministériel de logiciels libres",
		Quality: "OFFICIEL", URL: ficheSirene + "477865281", Check: "SIRENE", Pattern: `^XWIKI$`},

	// Intelligence artificielle.
	{Siren: "952418325", Name: "Mistral AI", Category: "IA",
		Basis:   "Unité légale Sirene ; se présente comme éditeur de modèles d'IA « ouverts »",
		Quality: "DECLARATIF", URL: "https://mistral.ai/fr/about", Check: "PAGE", Expected: []string{"ouverte à tous"}},

	// Pôles de compétitivité du numérique : chacun se présente ainsi.
	{Siren: "485364525", Name: "Systematic Paris-Region", Category: "POLE",
		Basis:   "Association (Sirene : SYSTEM@TIC PARIS REGION) ; se présente comme pôle européen des deep tech, en Île-de-France",
		Quality: "DECLARATIF", URL: "https://www.systematic-paris-region.org/", Check: "PAGE", Expected: []string{"Le Pôle Systematic Paris-Region"}},
	{Siren: "489749291", Name: "Cap Digital", Category: "POLE",
		Basis:   "Association (Sirene) ; se présente comme pôle européen de la transition numérique et écologique, à Paris",
		Quality: "DECLARATIF", URL: "https://www.capdigital.com/", Check: "PAGE", Expected: []string{"le pôle européen de la transition numérique et écologique"}},
	{Siren: "485361133", Name: "Minalogic", Category: "POLE",
		Basis:   "Association (Sirene) ; se présente comme pôle de compétitivité de la transformation numérique en Auvergne-Rhône-Alpes",
		Quality: "DECLARATIF", URL: "https://www.minalogic.com/", Check: "PAGE", Expected: []string{"Le pôle de compétitivité de la transformation numérique"}},
	{Siren: "487612368", Name: "Images & Réseaux", Category: "POLE",
		Basis:   "Association (Sirene) ; se présente comme pôle de compétitivité de l'innovation numérique en Bretagne et Pays de la Loire",
		Quality: "DECLARATIF", URL: "https://www.images-et-reseaux.com/", Check: "PAGE", Expected: []string{"pôle de compétitivité de l'innovation numérique en Bretagne et Pays de la Loire"}},

	// Organisations professionnelles et d'utilisateurs.
	{Siren: "810490052", Name: "Hexatrust", Category: "FILIERE",
		Basis:   "Association ; se présente comme le groupement des acteurs français et européens de la cybersécurité et du Cloud de confiance",
		Quality: "DECLARATIF", URL: "https://www.hexatrust.com/", Check: "PAGE", Expected: []string{"cybersécurité et du cloud de confiance"}},
	{Siren: "823219985", Name: "Conseil national du logiciel libre (CNLL)", Category: "FILIERE",
		Basis:   "Association (Sirene) ; se donne pour mission de structurer la filière du logiciel libre en France",
		Quality: "DECLARATIF", URL: "https://www.cnll.fr/", Check: "PAGE", Expected: []string{"structurer la filière du logiciel libre en France"}},
	{Siren: "494276108", Name: "AFUL (Association francophone des utilisateurs de logiciels libres)", Category: "FILIERE",
		Basis: "Association (Sirene)", Quality: "OFFICIEL", URL: ficheSirene + "494276108", Check: "SIRENE",
		Pattern: `^ASSOCIATION FRANCOPHONE DES UTILISATEURS DE LOGICIELS LIBRES$`},
	{Siren: "384719001", Name: "Numeum", Category: "FILIERE",
		Basis:   "Syndicat professionnel (Sirene) ; se présente comme le syndicat des entreprises du numérique",
		Quality: "DECLARATIF", URL: "https://numeum.fr/", Check: "PAGE", Expected: []string{"Entreprises du Numérique"}},
}
