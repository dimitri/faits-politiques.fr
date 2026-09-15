package dossiers

// Dossier « Souveraineté numérique », partie semi-conducteurs. Le point de départ
// est une série d'articles de Laurent Bloch (ancien chef du service d'informatique
// scientifique de l'Institut Pasteur) sur l'usine STMicroelectronics de Crolles et
// le règlement européen sur les puces : ils sont cités comme analyses signées
// (DECLARATIF), et chaque chiffre repris dans le dossier vient d'une source publique
// relue au chargement (Cour des comptes, Commission européenne, DGE, dossier de
// concertation publié par la CNDP).

const (
	themeSemi       = "semi-conducteurs"
	urlCCSemi       = "https://www.ccomptes.fr/sites/default/files/2026-04/20260421-Soutien-filiere-des-semi-conducteurs.pdf"
	auteurCCSemi    = "Cour des comptes (rapport public thématique « Le soutien à la filière des semi-conducteurs », avril 2026)"
	urlChipsAct     = "https://eur-lex.europa.eu/legal-content/FR/TXT/HTML/?uri=CELEX:32023R1781"
	urlCOM202245    = "https://eur-lex.europa.eu/legal-content/FR/TXT/HTML/?uri=CELEX:52022DC0045"
	urlDGELiberty   = "https://www.entreprises.gouv.fr/la-dge/actualites/la-mega-usine-de-semi-conducteurs-officiellement-commence-sa-production"
	urlCNDPCrolles  = "https://www.debatpublic.fr/sites/default/files/2024-03/Dossier_de_concertationSTM_1.pdf"
	urlBlochCrolles = "https://www.laurentbloch.net/MySpip3/L-usine-microelectronique-STMicro-a-Crolles"
	urlBlochChips   = "https://www.laurentbloch.net/MySpip3/Le-Chips-Act-peut-il-sauver-l-industrie-europeenne"
	auteurBloch     = "Laurent Bloch (ancien chef du service d'informatique scientifique de l'Institut Pasteur), article sur son site personnel"
	groupeST        = "STMicroelectronics N.V."
)

func init() {
	faits = append(faits, faitsSemiConducteurs...)
}

var faitsSemiConducteurs = []Fait{
	// Enjeux : tels que les formulent la Commission européenne et un analyste signé.
	{ID: "commission-com-2022-45", Dossier: dossierSouv, Section: "ENJEUX", Theme: themeSemi, Type: "CONSTAT", Date: "2022-02-08",
		Auteur:   "Commission européenne (communication COM(2022) 45, « Une législation sur les semi-conducteurs pour l'Europe »)",
		Intitule: "La part de l'Union dans les revenus mondiaux des puces : environ 10 %",
		Constat: "La Commission estime la part de l'Union à environ 10 % des revenus mondiaux liés aux puces semi-conducteurs " +
			"et fixe l'objectif d'atteindre au moins 20 % de la production mondiale.",
		URL: urlCOM202245, Qualite: "OFFICIEL",
		Attendus: []string{"représente environ 10 % du total", "au moins 20 % de la production mondiale"}},
	{ID: "bloch-dependance-itar-2022", Dossier: dossierSouv, Section: "ENJEUX", Theme: themeSemi, Type: "DECLARATION", Date: "2022-03-06",
		Auteur:   auteurBloch,
		Intitule: "Composants de conception américaine et licences d'exportation ITAR",
		Constat: "L'auteur écrit que l'industrie militaire française dépend, pour ses produits les plus avancés, de composants de conception " +
			"américaine soumis aux licences d'exportation ITAR, de plus en plus difficiles à obtenir, et que STMicroelectronics et NXP " +
			"disent se satisfaire de technologies de 28 nm et plus. Analyse signée, non vérifiée par une source officielle chargée.",
		URL: urlBlochChips, Qualite: "DECLARATIF",
		Attendus: []string{"licences d'exportation conformes à la législation américaine ITAR", "(28 nm et plus)"}},

	// Cadre : le règlement européen et l'aide d'État française.
	{ID: "reglement-puces-2023-1781", Dossier: dossierSouv, Section: "CADRE", Theme: themeSemi, Type: "TEXTE", Date: "2023-09-13",
		Auteur:   "Parlement européen et Conseil (règlement (UE) 2023/1781)",
		Intitule: "Règlement européen sur les puces (« Chips Act »)",
		Constat: "Le règlement établit un cadre pour renforcer l'écosystème européen des semi-conducteurs, dont l'initiative " +
			"« Semi-conducteurs pour l'Europe ». La Cour des comptes chiffre à 43 Md€ les subventions prévues, financées principalement par les États membres.",
		URL: urlChipsAct, Qualite: "OFFICIEL",
		Attendus: []string{"établissant un cadre de mesures pour renforcer l'écosystème européen des semi-conducteurs"}},
	{ID: "dge-liberty-crolles-2023", Dossier: dossierSouv, Section: "CADRE", Theme: themeSemi, Type: "AIDE", Date: "2023-06-05",
		Auteur: "Direction générale des entreprises (ministère de l'Économie)", Groupe: groupeST,
		Intitule: "Usine de Crolles : soutien de l'État de 2,9 Md€ au plus",
		Constat: "La nouvelle usine de semi-conducteurs de Crolles (STMicroelectronics et GlobalFoundries) a officiellement commencé sa production " +
			"en juin 2023, avec un soutien financier maximal de l'État de 2,9 Md€ et l'objectif de doubler la production de puces en France d'ici 2028.",
		Montant: "2900000000", Nature: "PLAFOND", URL: urlDGELiberty, Qualite: "OFFICIEL",
		Attendus: []string{"soutien financier maximal de 2,9 milliards d'euros", "doubler la production de puces en France d'ici 2028"}},

	// Contrôles : la Cour des comptes, avril 2026.
	{ID: "ccomptes-semi-soutiens-2018-2025", Dossier: dossierSouv, Section: "CONTROLE", Theme: themeSemi, Type: "EVALUATION", Date: "2026-04-21",
		Auteur: auteurCCSemi, Intitule: "8,7 Md€ d'aides publiques programmées à la filière de 2018 à 2025, dont 5 Md€ versés",
		Constat: "Aucune consolidation des soutiens publics n'existait : la Cour les recense et les estime à 8,7 Md€ programmés sur 2018-2025, " +
			"dont 5 Md€ effectivement versés, hors participations publiques au capital (3,6 Md€) et crédit d'impôt recherche inclus.",
		Montant: "8700000000", Nature: "AIDE", URL: urlCCSemi, Page: "9", Qualite: "OFFICIEL",
		Attendus: []string{"8,7 Md€ sur la période 2018 à 2025, dont 5 Md€ d'aides effectivement versées", "actionnariat public (3,6 Md€"}},
	{ID: "ccomptes-semi-conditionnalite", Dossier: dossierSouv, Section: "CONTROLE", Theme: themeSemi, Type: "EVALUATION", Date: "2026-04-21",
		Auteur: auteurCCSemi, Intitule: "Des aides peu conditionnées à la production nationale et à l'emploi",
		Constat: "À la différence des États-Unis ou du Japon, les soutiens publics français se caractérisent selon la Cour par une faible " +
			"conditionnalité en termes de production nationale et d'emploi, et ne prennent quasiment jamais la forme d'avances remboursables.",
		URL: urlCCSemi, Page: "9", Qualite: "OFFICIEL",
		Attendus: []string{"une faible conditionnalité en termes de production nationale et d'emploi"}},
	{ID: "ccomptes-liberty-versements", Dossier: dossierSouv, Section: "CONTROLE", Theme: themeSemi, Type: "EVALUATION", Date: "2026-04-21",
		Auteur: auteurCCSemi, Groupe: groupeST, Intitule: "Projet Liberty : 574 M€ versés à STMicroelectronics, rien à GlobalFoundries",
		Constat: "Sur 2,9 Md€ de subventions accordées (1,8 Md€ pour GlobalFoundries, 1,1 Md€ pour STMicroelectronics), 574 M€ avaient été versés " +
			"à STMicroelectronics à fin juin 2025 ; GlobalFoundries n'a pas commencé sa part du projet. La Cour relève que l'évaluation " +
			"socio-économique préalable exigée au-delà de 20 M€ est incomplète.",
		Montant: "574000000", Nature: "PAYE", URL: urlCCSemi, Page: "11", Qualite: "OFFICIEL",
		Attendus: []string{"574 M€ avaient été versés à STMicroelectronics", "Aucun paiement n'a été réalisé pour GlobalFoundries", "Pour Liberty, cette évaluation est incomplète"}},
	{ID: "ccomptes-semi-cartographie", Dossier: dossierSouv, Section: "CONTROLE", Theme: themeSemi, Type: "RECOMMANDATION", Date: "2026-04-21",
		Auteur: auteurCCSemi, Intitule: "Cartographier l'offre et la demande de puces, chiffrer les objectifs",
		Constat: "L'État ne dispose pas de cartographie précise de la filière ni de cibles de production par type de puces : il n'est donc pas " +
			"en mesure, selon la Cour, de quantifier les progrès en matière de souveraineté industrielle. Elle recommande de cartographier " +
			"l'offre et la demande en 2026 et de fixer des objectifs chiffrés par type de puces.",
		URL: urlCCSemi, Page: "10-12", Qualite: "OFFICIEL",
		Attendus: []string{"l'État ne dispose pas aujourd'hui de cartographie précise de la filière", "il n'est pas en mesure de quantifier les progrès obtenus en matière de souveraineté industrielle"}},

	// Situation : ce que disent les chiffres publics, puis le témoignage de 2014.
	{ID: "ccomptes-semi-filiere-france", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSemi, Type: "DONNEE", Date: "2026-04-21",
		Auteur: auteurCCSemi, Intitule: "La filière française : 53 600 salariés, 11 % de la production européenne",
		Constat: "La filière compte une centaine d'entreprises et 53 600 salariés ; cinq entreprises, dont STMicroelectronics, réalisent 85 % " +
			"de la production. Son chiffre d'affaires de 18,2 Md€ en 2022 représente 11 % de la production européenne, " +
			"elle-même 7 % de la production mondiale de puces. Excédent commercial : 1,8 Md€ en 2024.",
		Montant: "18200000000", Nature: "CHIFFRE_AFFAIRES", URL: urlCCSemi, Page: "7-8", Qualite: "OFFICIEL",
		Attendus: []string{"53 600 salariés", "représente 11 % de la production européenne de semi-conducteurs", "L'Union européenne ne représente que 7 % de la production mondiale de puces", "excédent commercial (1,8 Md€ en 2024)"}},
	{ID: "cndp-crolles-concertation-2024", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSemi, Type: "DECLARATION", Date: "2024-03-01",
		Auteur: "STMicroelectronics, maître d'ouvrage (dossier de concertation publié par la Commission nationale du débat public)", Groupe: groupeST,
		Intitule: "Extension de Crolles : 7,5 Md€ d'investissement annoncés, 300 m³/h de prélèvement d'eau au plus",
		Constat: "Le dossier présenté à la concertation annonce un investissement de l'ordre de 7,5 Md€ porté avec GlobalFoundries, " +
			"pour doubler la capacité de production en 300 mm à horizon 2028 ; l'entreprise déclare 7 500 salariés en Isère dont plus de 5 100 à Crolles, " +
			"et un débit maximal de prélèvement d'eau de 300 m³/h.",
		URL: urlCNDPCrolles, Page: "2, 17, 46", Qualite: "DECLARATIF",
		Attendus: []string{"un investissement total de l'ordre de 7,5 milliards d'euros", "Avec 7 500 salariés en Isère dont plus de 5 100 à Crolles",
			"vise à doubler la capacité de production en technologie 300 mm à horizon 2028", "à un débit maximal de prélèvement de 300 m3/h"}},
	{ID: "bloch-crolles-2014", Dossier: dossierSouv, Section: "SITUATION", Theme: themeSemi, Type: "DECLARATION", Date: "2014-10-24",
		Auteur: auteurBloch, Groupe: groupeST,
		Intitule: "Crolles en 2014 : la seule usine européenne de processeurs de pointe, selon l'auteur",
		Constat: "Après une journée portes ouvertes, l'auteur recense six entreprises capables de fabriquer des processeurs à l'état de l'art, " +
			"dont une seule européenne, STMicroelectronics, dont l'unité de pointe est à Crolles (22/32 nm en 2014), qui fabrique notamment " +
			"des processeurs ARM sous licence. Chiffres de 2014 non actualisés par l'auteur.",
		URL: urlBlochCrolles, Qualite: "DECLARATIF",
		Attendus: []string{"Une seule de ces entreprises est européenne", "Les processeurs ARM sont fabriqués sous licence par STMicroelectronics"}},
}
