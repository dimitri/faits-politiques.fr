package dossiers

// Souveraineté numérique : le contexte tiré des débats de l'Assemblée, les
// solutions françaises nommées par les sources, les mots suivis et les acteurs.

func init() {
	faits = append(faits, faitsSouverainetePlus...)
	termes = append(termes,
		Terme{dossierSouv, "souveraineté numérique", `souveraineté numérique`},
		Terme{dossierSouv, "SecNumCloud", `secnumcloud`},
		Terme{dossierSouv, "Cloud Act, extraterritorialité", `cloud act|extraterritorial`},
		Terme{dossierSouv, "logiciel libre", `logiciels? libres?`},
	)
	acteurs = append(acteurs, acteursNumeriques...)
}

const (
	themeSolutions  = "solutions françaises"
	themeDebats     = "débats à l'Assemblée"
	ficheSirene     = "https://annuaire-entreprises.data.gouv.fr/entreprise/"
	urlCatalogueSNC = "https://messervices.cyber.gouv.fr/visas/catalogue-produits-services-profils-de-protection-sites-certifies-qualifies-agrees-anssi.pdf"
)

var faitsSouverainetePlus = []Fait{
	// Contexte : ce que des membres du Gouvernement ont dit en séance.
	{ID: "chappaz-secnumcloud-2025-02-12", Dossier: dossierSouv, Section: "CONTEXTE", Theme: themeDebats, Type: "DECLARATION",
		Date: "2025-02-12", Auteur: "Ministre déléguée chargée de l'intelligence artificielle et du numérique", Personne: "clara-chappaz",
		Intitule: "Une stratégie de souveraineté numérique appuyée sur SecNumCloud depuis 2021",
		Constat: "Depuis 2021, avec l'ANSSI, la stratégie de sécurisation des données s'appuie notamment sur SecNumCloud ; un appel à " +
			"projets du plan France 2030 vise à faire monter en compétence des acteurs comme OVHcloud ou Scaleway.",
		Qualite: "DECLARATIF", Seance: "2025-02-12",
		Attendus: []string{"notre stratégie de sécurisation des données s'appuie notamment sur SecNumCloud", "OVHcloud ou Scaleway"}},
	{ID: "chappaz-health-data-hub-2025-04-08", Dossier: dossierSouv, Section: "CONTEXTE", Theme: themeDebats, Type: "DECLARATION",
		Date: "2025-04-08", Auteur: "Ministre déléguée chargée de l'intelligence artificielle et du numérique", Personne: "clara-chappaz",
		Intitule: "Le Health Data Hub sans « hébergeur ultrasécurisé » : un appel d'offres de migration annoncé",
		Constat: "Les données du système national des données de santé ne sont pas dans le Health Data Hub faute d'hébergeur " +
			"ultrasécurisé ; la ministre annonce un appel d'offres pour le migrer vers un hébergeur sécurisé.",
		Qualite: "DECLARATIF", Seance: "2025-04-08",
		Attendus: []string{"n'a pas d'hébergeur ultrasécurisé", "lancer un appel d'offres pour faire migrer le Health Data Hub"}},
	{ID: "ferracci-data-centers-2025-04-30", Dossier: dossierSouv, Section: "CONTEXTE", Theme: themeDebats, Type: "DECLARATION",
		Date: "2025-04-30", Auteur: "Ministre chargé de l'industrie et de l'énergie", Personne: "marc-ferracci",
		Intitule: "Les centres de données, élément d'une souveraineté numérique",
		Constat: "Défendant un article de loi sur l'implantation des centres de données, le ministre y voit la capacité de " +
			"s'approprier les éléments d'une souveraineté numérique que d'autres pays construisent.",
		Qualite: "DECLARATIF", Seance: "2025-04-30",
		Attendus: []string{"nous approprier les éléments d'une souveraineté numérique"}},
	{ID: "montchalin-data-centers-2025-11-17", Dossier: dossierSouv, Section: "CONTEXTE", Theme: themeDebats, Type: "DECLARATION",
		Date: "2025-11-17", Auteur: "Ministre de l'action et des comptes publics", Personne: "amelie-de-montchalin",
		Intitule: "La fiscalité des centres de données et le plan France 2030",
		Constat: "Le Gouvernement s'oppose à la suppression d'une réduction fiscale pour les centres de données, au nom de la " +
			"souveraineté numérique soutenue financièrement par le plan France 2030.",
		Qualite: "DECLARATIF", Seance: "2025-11-17",
		Attendus: []string{"il y va de la souveraineté numérique que nous soutenons financièrement dans le cadre du plan France 2030"}},

	// Situation : les solutions françaises nommées par les sources.
	{ID: "latombe-hebergeurs-francais-2025", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "DECLARATION",
		Date: "2025-04-08", Auteur: "Député, rapporteur d'une mission d'information sur la souveraineté numérique (audition au Sénat)",
		Personne: "philippe-latombe",
		Intitule: "Des hébergeurs français nommés : OVH, Scaleway, NumSpot, Outscale, Cloud Temple",
		Constat: "Devant la commission d'enquête du Sénat, le député cite OVH, Scaleway, NumSpot, Outscale et Cloud Temple parmi les " +
			"entreprises françaises capables d'héberger des données publiques.",
		URL: urlSenat830, Page: "267-268", Qualite: "DECLARATIF",
		Attendus: []string{"je citerai OVH, Scaleway et NumSpot"}},
	{ID: "ovhcloud-senat-2025", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "DECLARATION",
		Auteur: "OVHcloud (rencontre avec la commission d'enquête du Sénat)", Groupe: "OVH Groupe",
		Intitule: "OVHcloud : 3 000 salariés, un milliard d'euros de chiffre d'affaires, 350 M€ d'investissement par an",
		Constat: "Chiffres donnés à la commission lors de sa rencontre avec le président d'OVHcloud : environ 3 000 salariés, un " +
			"chiffre d'affaires de l'ordre d'un milliard d'euros, 350 M€ investis chaque année, 600 M€ visés en 2030.",
		URL: urlSenat830, Page: "268", Qualite: "DECLARATIF",
		Attendus: []string{"emploie ainsi 3 000 personnes", "de l'ordre d'un milliard d'euros"}},
	{ID: "poupard-acteurs-prets-2025", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "DECLARATION",
		Date: "2025-05-27", Auteur: "Directeur général adjoint de Docaposte, ancien directeur général de l'ANSSI (audition au Sénat)",
		Intitule: "« Des acteurs français sont prêts », avec un catalogue moins complet",
		Constat: "Selon l'ancien directeur de l'ANSSI, les fournisseurs Cloud français proposent les services dont l'État a besoin, " +
			"même si leur catalogue est moins complet que celui des grands groupes américains.",
		URL: urlSenat830, Page: "268", Qualite: "DECLARATIF",
		Attendus: []string{"des acteurs français sont prêts"}},
	{ID: "gendarmerie-linux-2009", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "CONSTAT",
		Auteur:   senatCommande,
		Intitule: "La gendarmerie nationale sous Linux depuis 2009",
		Constat: "La gendarmerie nationale a adopté le système d'exploitation libre Linux en 2009 ; elle n'est pas concernée par le " +
			"remplacement de postes qu'impose la fin de Windows 10 à la police nationale.",
		URL: urlSenat830, Page: "259", Qualite: "OFFICIEL",
		Attendus: []string{"La gendarmerie nationale, qui a adopté le système d'exploitation open source Linux depuis 2009"}},
	{ID: "education-open-source-2025", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "DECLARATION",
		Date: "2025-06-03", Auteur: "Sous-directeur du socle numérique, direction du numérique pour l'éducation (audition au Sénat)",
		Intitule: "Le système d'information de l'Éducation nationale « basé à 98 % sur de l'open source »",
		Constat: "Selon le ministère, son système d'information repose à 98 % sur des logiciels libres (Red Hat Linux notamment), les " +
			"licences Microsoft servant surtout aux postes de travail.",
		URL: urlSenat830, Page: "253", Qualite: "DECLARATIF",
		Attendus: []string{"98 % sur de l'open source"}},
	{ID: "suite-numerique-dinum-2025", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "TEXTE",
		Date: "2025-04-22", Auteur: "Ministres de l'Action publique, des Comptes publics et du Numérique (courrier aux membres du Gouvernement)",
		Intitule: "Des offres de l'État : clouds interministériels et « La Suite numérique »",
		Constat: "Le courrier invite les ministères à recourir aux offres Cloud qualifiées SecNumCloud ou aux clouds interministériels " +
			"(cloud Pi de l'Intérieur, Nubo des Finances), et aux outils collaboratifs de la DINUM, « La Suite numérique ».",
		URL: urlSenat830, Page: "256", Qualite: "OFFICIEL",
		Attendus: []string{"« La Suite numérique »", "le cloud PI du ministère de l'intérieur"}},
	{ID: "sitzenstuhl-stmicroelectronics-2025-05-27", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "DECLARATION",
		Date: "2025-05-27", Auteur: "Député", Personne: "charles-sitzenstuhl",
		Intitule: "STMicroelectronics en débat : présence industrielle et participation publique",
		Constat: "En séance, le député rappelle que Bpifrance est actionnaire de STMicroelectronics et défend son développement " +
			"face aux objections sur sa consommation d'eau.",
		Qualite: "DECLARATIF", Seance: "2025-05-27",
		Attendus: []string{"est actionnaire de STMicroelectronics"}},
	{ID: "tavel-stmicroelectronics-2025-05-27", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSolutions, Type: "DECLARATION",
		Date: "2025-05-27", Auteur: "Député", Personne: "matthias-tavel",
		Intitule: "STMicroelectronics en débat : suppressions d'emplois annoncées",
		Constat: "Dans le même débat, le député interroge l'accord de Bpifrance, actionnaire, à la suppression annoncée d'un millier " +
			"d'emplois en France par STMicroelectronics.",
		Qualite: "DECLARATIF", Seance: "2025-05-27",
		Attendus: []string{"STMicroelectronics s'apprête à supprimer 1 000 emplois en France"}},
}

var acteursNumeriques = []Acteur{
	// Semi-conducteurs. Le code d'activité Sirene 26.11Z est « fabrication de
	// composants électroniques » : il ne dit pas quelles puces sont produites.
	{Siren: "399395581", Nom: "STMicroelectronics (Crolles 2)", Categorie: "SEMI_CONDUCTEURS", Groupe: "STMicroelectronics N.V.",
		Fondement: "Unité légale Sirene, activité 26.11Z (fabrication de composants électroniques) ; bénéficiaire du projet « Liberty » d'usine de semi-conducteurs à Crolles (registre européen des aides d'État)",
		Qualite:   "OFFICIEL", URL: ficheSirene + "399395581", Verif: "SIRENE", Motif: `^STMICROELECTRONICS \(CROLLES 2\)`},
	{Siren: "341459386", Nom: "STMicroelectronics France", Categorie: "SEMI_CONDUCTEURS", Groupe: "STMicroelectronics N.V.",
		Fondement: "Unité légale Sirene, activité 26.11Z", Qualite: "OFFICIEL", URL: ficheSirene + "341459386", Verif: "SIRENE", Motif: `^STMICROELECTRONICS FRANCE$`},
	{Siren: "414969584", Nom: "STMicroelectronics Rousset", Categorie: "SEMI_CONDUCTEURS", Groupe: "STMicroelectronics N.V.",
		Fondement: "Unité légale Sirene, activité 26.11Z", Qualite: "OFFICIEL", URL: ficheSirene + "414969584", Verif: "SIRENE", Motif: `^STMICROELECTRONICS ROUSSET`},
	{Siren: "380932590", Nom: "STMicroelectronics Tours", Categorie: "SEMI_CONDUCTEURS", Groupe: "STMicroelectronics N.V.",
		Fondement: "Unité légale Sirene, activité 26.11Z", Qualite: "OFFICIEL", URL: ficheSirene + "380932590", Verif: "SIRENE", Motif: `^STMICROELECTRONICS \(TOURS\)`},
	{Siren: "504941337", Nom: "STMicroelectronics (Grenoble 2)", Categorie: "SEMI_CONDUCTEURS", Groupe: "STMicroelectronics N.V.",
		Fondement: "Unité légale Sirene, activité 72.19Z (recherche-développement)", Qualite: "OFFICIEL", URL: ficheSirene + "504941337", Verif: "SIRENE", Motif: `^STMICROELECTRONICS \(GRENOBLE 2\)`},
	{Siren: "384711909", Nom: "Soitec", Categorie: "SEMI_CONDUCTEURS",
		Fondement: "Unité légale Sirene, activité 26.11Z ; se présente comme fabricant de matériaux pour semi-conducteurs",
		Qualite:   "DECLARATIF", URL: "https://www.soitec.com/fr", Verif: "PAGE", Attendus: []string{"matériaux innovants pour semi-conducteurs"}},

	// Cloud : location d'ordinateurs dans des salles serveurs.
	{Siren: "424761419", Nom: "OVH (OVHcloud)", Categorie: "CLOUD", Groupe: "OVH Groupe",
		Fondement: "Services qualifiés SecNumCloud au catalogue de l'ANSSI", Qualite: "OFFICIEL", URL: urlCatalogueSNC, Verif: "SECNUMCLOUD"},
	{Siren: "433115904", Nom: "Scaleway", Categorie: "CLOUD", Groupe: "groupe iliad",
		Fondement: "Unité légale Sirene, activité 63.11Z (traitement de données, hébergement) ; se présente comme filiale du groupe iliad",
		Qualite:   "DECLARATIF", URL: "https://www.scaleway.com/fr/a-propos/", Verif: "PAGE", Attendus: []string{"Filiale du Groupe iliad"}},
	{Siren: "527594493", Nom: "Outscale", Categorie: "CLOUD",
		Fondement: "Service qualifié SecNumCloud au catalogue de l'ANSSI", Qualite: "OFFICIEL", URL: urlCatalogueSNC, Verif: "SECNUMCLOUD"},
	{Siren: "825400336", Nom: "Cloud Temple", Categorie: "CLOUD",
		Fondement: "Services qualifiés SecNumCloud au catalogue de l'ANSSI", Qualite: "OFFICIEL", URL: urlCatalogueSNC, Verif: "SECNUMCLOUD"},
	{Siren: "948608948", Nom: "Numspot", Categorie: "CLOUD",
		Fondement: "Service qualifié SecNumCloud au catalogue de l'ANSSI", Qualite: "OFFICIEL", URL: urlCatalogueSNC, Verif: "SECNUMCLOUD"},
	{Siren: "345039416", Nom: "Orange Business Services", Categorie: "CLOUD", Groupe: "Orange",
		Fondement: "Service qualifié SecNumCloud au catalogue de l'ANSSI", Qualite: "OFFICIEL", URL: urlCatalogueSNC, Verif: "SECNUMCLOUD"},
	{Siren: "908211980", Nom: "Thales Cloud Sécurisé (offre S3NS)", Categorie: "CLOUD", Groupe: "Thales",
		Fondement: "Services qualifiés SecNumCloud au catalogue de l'ANSSI, sur technologie Google Cloud", Qualite: "OFFICIEL", URL: urlCatalogueSNC, Verif: "SECNUMCLOUD"},
	{Siren: "378901946", Nom: "Worldline", Categorie: "CLOUD",
		Fondement: "Service qualifié SecNumCloud au catalogue de l'ANSSI", Qualite: "OFFICIEL", URL: urlCatalogueSNC, Verif: "SECNUMCLOUD"},

	// Logiciel : éditeurs qualifiés ou prestataires du logiciel libre.
	{Siren: "384351599", Nom: "Index Éducation (Pronote)", Categorie: "LOGICIEL",
		Fondement: "Services SaaS qualifiés SecNumCloud au catalogue de l'ANSSI", Qualite: "OFFICIEL", URL: urlCatalogueSNC, Verif: "SECNUMCLOUD"},
	{Siren: "528893522", Nom: "Cloud Solutions (Wimi)", Categorie: "LOGICIEL",
		Fondement: "Services SaaS qualifiés SecNumCloud au catalogue de l'ANSSI", Qualite: "OFFICIEL", URL: urlCatalogueSNC, Verif: "SECNUMCLOUD"},
	{Siren: "432735082", Nom: "Oodrive", Categorie: "LOGICIEL",
		Fondement: "Services SaaS qualifiés SecNumCloud au catalogue de l'ANSSI", Qualite: "OFFICIEL", URL: urlCatalogueSNC, Verif: "SECNUMCLOUD"},
	{Siren: "519139497", Nom: "Whaller", Categorie: "LOGICIEL",
		Fondement: "Service SaaS qualifié SecNumCloud au catalogue de l'ANSSI", Qualite: "OFFICIEL", URL: urlCatalogueSNC, Verif: "SECNUMCLOUD"},
	{Siren: "431473669", Nom: "Linagora", Categorie: "LOGICIEL",
		Fondement: "Unité légale Sirene, activité 62.02A (conseil en systèmes et logiciels) ; prestataire déclaré au socle interministériel de logiciels libres",
		Qualite:   "OFFICIEL", URL: ficheSirene + "431473669", Verif: "SIRENE", Motif: `^LINAGORA$`},
	{Siren: "477865281", Nom: "XWiki", Categorie: "LOGICIEL",
		Fondement: "Unité légale Sirene, activité 63.11Z ; prestataire déclaré au socle interministériel de logiciels libres",
		Qualite:   "OFFICIEL", URL: ficheSirene + "477865281", Verif: "SIRENE", Motif: `^XWIKI$`},

	// Intelligence artificielle.
	{Siren: "952418325", Nom: "Mistral AI", Categorie: "IA",
		Fondement: "Unité légale Sirene ; se présente comme éditeur de modèles d'IA « ouverts »",
		Qualite:   "DECLARATIF", URL: "https://mistral.ai/fr/about", Verif: "PAGE", Attendus: []string{"ouverte à tous"}},

	// Pôles de compétitivité du numérique : chacun se présente ainsi.
	{Siren: "485364525", Nom: "Systematic Paris-Region", Categorie: "POLE",
		Fondement: "Association (Sirene : SYSTEM@TIC PARIS REGION) ; se présente comme pôle européen des deep tech, en Île-de-France",
		Qualite:   "DECLARATIF", URL: "https://www.systematic-paris-region.org/", Verif: "PAGE", Attendus: []string{"Le Pôle Systematic Paris-Region"}},
	{Siren: "489749291", Nom: "Cap Digital", Categorie: "POLE",
		Fondement: "Association (Sirene) ; se présente comme pôle européen de la transition numérique et écologique, à Paris",
		Qualite:   "DECLARATIF", URL: "https://www.capdigital.com/", Verif: "PAGE", Attendus: []string{"le pôle européen de la transition numérique et écologique"}},
	{Siren: "485361133", Nom: "Minalogic", Categorie: "POLE",
		Fondement: "Association (Sirene) ; se présente comme pôle de compétitivité de la transformation numérique en Auvergne-Rhône-Alpes",
		Qualite:   "DECLARATIF", URL: "https://www.minalogic.com/", Verif: "PAGE", Attendus: []string{"Le pôle de compétitivité de la transformation numérique"}},
	{Siren: "487612368", Nom: "Images & Réseaux", Categorie: "POLE",
		Fondement: "Association (Sirene) ; se présente comme pôle de compétitivité de l'innovation numérique en Bretagne et Pays de la Loire",
		Qualite:   "DECLARATIF", URL: "https://www.images-et-reseaux.com/", Verif: "PAGE", Attendus: []string{"pôle de compétitivité de l'innovation numérique en Bretagne et Pays de la Loire"}},

	// Organisations professionnelles et d'utilisateurs.
	{Siren: "810490052", Nom: "Hexatrust", Categorie: "FILIERE",
		Fondement: "Association ; se présente comme le groupement des acteurs français et européens de la cybersécurité et du Cloud de confiance",
		Qualite:   "DECLARATIF", URL: "https://www.hexatrust.com/", Verif: "PAGE", Attendus: []string{"cybersécurité et du cloud de confiance"}},
	{Siren: "823219985", Nom: "Conseil national du logiciel libre (CNLL)", Categorie: "FILIERE",
		Fondement: "Association (Sirene) ; se donne pour mission de structurer la filière du logiciel libre en France",
		Qualite:   "DECLARATIF", URL: "https://www.cnll.fr/", Verif: "PAGE", Attendus: []string{"structurer la filière du logiciel libre en France"}},
	{Siren: "494276108", Nom: "AFUL (Association francophone des utilisateurs de logiciels libres)", Categorie: "FILIERE",
		Fondement: "Association (Sirene)", Qualite: "OFFICIEL", URL: ficheSirene + "494276108", Verif: "SIRENE",
		Motif: `^ASSOCIATION FRANCOPHONE DES UTILISATEURS DE LOGICIELS LIBRES$`},
	{Siren: "384719001", Nom: "Numeum", Categorie: "FILIERE",
		Fondement: "Syndicat professionnel (Sirene) ; se présente comme le syndicat des entreprises du numérique",
		Qualite:   "DECLARATIF", URL: "https://numeum.fr/", Verif: "PAGE", Attendus: []string{"Entreprises du Numérique"}},
}
