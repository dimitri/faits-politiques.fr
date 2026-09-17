package dossiers

// Dossier « Souveraineté numérique de l'État » (docs/souverainete-numerique.md).
// Les données (qualifications SecNumCloud, sanctions de la CNIL, marchés
// informatiques, SILL) sont chargées par internal/numerique ; ce fichier porte les
// faits du dossier, les mots suivis dans les débats et les acteurs français nommés.

const (
	dossierSouv = "souverainete-numerique"
	urlSenat830 = "https://www.senat.fr/rap/r24-830-1/r24-830-11.pdf"
	urlDoctrine = "https://www.numerique.gouv.fr/services/cloud/doctrine/"
	urlCloudAct = "https://www.govinfo.gov/content/pkg/PLAW-115publ141/html/PLAW-115publ141.htm"
	urlFISA702  = "https://www.govinfo.gov/content/pkg/USCODE-2023-title50/html/USCODE-2023-title50-chap36-subchapVI-sec1881a.htm"
	// EUR-Lex bloque désormais (17 septembre 2026, vérifié directement) tout
	// le chemin legal-content par un défi WAF (x-amzn-waf-action: challenge),
	// quel que soit le format ou le user-agent — pas un blocage ciblé sur ce
	// dépôt. Remplacés par le communiqué de presse officiel de l'institution
	// concernée, sur un domaine distinct, jamais par une source secondaire.
	urlSchremsII  = "https://curia.europa.eu/site/upload/docs/application/pdf/2020-07/cp200091fr.pdf"
	urlDPF        = "https://ec.europa.eu/commission/presscorner/api/files/document/print/fr/ip_23_3721/IP_23_3721_FR.pdf"
	urlLatombe    = "https://curia.europa.eu/jcms/upload/docs/application/pdf/2025-09/cp250106fr.pdf"
	urlDPCMeta    = "https://www.dataprotection.ie/en/news-media/press-releases/Data-Protection-Commission-announces-conclusion-of-inquiry-into-Meta-Ireland"
	urlCNILGoogle = "https://www.cnil.fr/fr/publicites-inserees-entre-les-courriels-et-cookies-la-cnil-sanctionne-google-dune-amende-de-325"
	joLoiSREN     = "JORFTEXT000049563368"
	urlLoiSREN    = "https://www.legifrance.gouv.fr/jorf/id/" + joLoiSREN
	urlJORF       = "https://www.legifrance.gouv.fr/jorf/id/"
	senatCommande = "Sénat, commission d'enquête sur les coûts et les modalités effectifs de la commande publique (rapport n° 830, 2025)"
	groupeMS      = "Microsoft Corporation"
)

func init() {
	faits = append(faits, faitsSouverainete...)
}

var faitsSouverainete = []Fait{
	// Le contexte : l'intitulé du ministre chargé de l'économie.
	{ID: "ministere-souverainete-numerique-2022", Dossier: dossierSouv, Section: "CONTEXTE", Type: "TEXTE", Theme: "intitulés ministériels", Date: "2022-06-01", Auteur: "Gouvernement (décret n° 2022-826 du 1er juin 2022)",
		Intitule: "Le ministre de l'Économie devient ministre « de la souveraineté industrielle et numérique »",
		Constat: "Pour la première fois, la souveraineté numérique figure dans l'intitulé du ministre de l'Économie et des Finances. " +
			"En 2014, l'intitulé comprenait « le numérique », sans souveraineté.",
		URL: urlJORF + "JORFTEXT000045847934", Qualite: "OFFICIEL", JO: "JORFTEXT000045847934",
		Attendus: []string{"souveraineté industrielle et numérique"}},
	{ID: "ministere-intitule-2024", Dossier: dossierSouv, Section: "CONTEXTE", Type: "TEXTE", Theme: "intitulés ministériels", Date: "2024-10-10", Auteur: "Gouvernement (décret n° 2024-916 du 10 octobre 2024)",
		Intitule: "L'intitulé disparaît : ministre « de l'économie, des finances et de l'industrie »",
		Constat: "Dans le gouvernement formé en septembre 2024, le ministre de l'Économie n'a plus la souveraineté numérique dans son intitulé ; " +
			"le numérique relève d'une secrétaire d'État chargée de l'intelligence artificielle et du numérique.",
		URL: urlJORF + "JORFTEXT000050330364", Qualite: "OFFICIEL", JO: "JORFTEXT000050330364",
		Attendus: []string{"ministre de l'économie, des finances et de l'industrie"}},
	{ID: "ministere-intitule-2025", Dossier: dossierSouv, Section: "CONTEXTE", Type: "TEXTE", Theme: "intitulés ministériels", Date: "2025-01-08", Auteur: "Gouvernement (décret n° 2025-20 du 8 janvier 2025)",
		Intitule: "L'intitulé revient : « souveraineté industrielle et numérique »",
		Constat:  "Le gouvernement formé en décembre 2024 rétablit la souveraineté industrielle et numérique dans l'intitulé du ministre de l'Économie.",
		URL:      urlJORF + "JORFTEXT000050959974", Qualite: "OFFICIEL", JO: "JORFTEXT000050959974",
		Attendus: []string{"souveraineté industrielle et numérique"}},

	// Les lois étrangères et la jurisprudence européenne.
	{ID: "cloud-act-2018", Dossier: dossierSouv, Section: "ENJEUX", Type: "TEXTE", Theme: "lois extraterritoriales", Date: "2018-03-23", Auteur: "Congrès des États-Unis",
		Intitule: "CLOUD Act (Clarifying Lawful Overseas Use of Data Act), section 103, 18 U.S.C. 2713",
		Constat: "Un fournisseur de services de communication électronique ou d'informatique à distance soumis au droit américain " +
			"doit préserver ou divulguer les contenus et données de ses clients qu'il a en sa possession, sa garde ou son contrôle, " +
			"que ces données se trouvent ou non aux États-Unis.",
		URL: urlCloudAct, Qualite: "OFFICIEL",
		Attendus: []string{"regardless of whether such communication, record, or other information is located within or outside of the United States"}},
	{ID: "fisa-702", Dossier: dossierSouv, Section: "ENJEUX", Type: "TEXTE", Theme: "lois extraterritoriales", Auteur: "Congrès des États-Unis",
		Intitule: "FISA, section 702 (50 U.S.C. 1881a) : collecte de renseignement visant des personnes situées hors des États-Unis",
		Constat: "Sur autorisation conjointe du ministre de la Justice et du directeur du renseignement national, approuvée par la cour FISA, " +
			"un fournisseur de services de communication électronique peut être contraint de fournir immédiatement toute information " +
			"et toute assistance nécessaires, en gardant le secret. Texte lu dans l'édition 2023 du code ; la section a été prorogée " +
			"en avril 2024 pour deux ans, et son état après avril 2026 n'est pas vérifié ici.",
		URL: urlFISA702, Qualite: "OFFICIEL",
		Attendus: []string{"immediately provide the Government with all information, facilities, or assistance necessary"}},
	{ID: "cjue-schrems-ii-2020", Dossier: dossierSouv, Section: "ENJEUX", Type: "TEXTE", Theme: "données personnelles", Date: "2020-07-16", Auteur: "Cour de justice de l'Union européenne (affaire C-311/18, communiqué de presse n° 91/20)",
		Intitule: "Arrêt « Schrems II » : invalidation du bouclier de protection des données UE-États-Unis",
		Constat: "La Cour invalide la décision d'adéquation de 2016 (Privacy Shield) : les programmes de surveillance fondés sur le droit " +
			"américain ne limitent pas l'accès aux données transférées au strict nécessaire et n'offrent pas de recours effectif aux " +
			"personnes concernées.",
		URL: urlSchremsII, Qualite: "OFFICIEL",
		Attendus: []string{"la Cour déclare la décision 2016/1250 invalide"}},
	{ID: "ue-adequation-dpf-2023", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "données personnelles", Date: "2023-07-10", Auteur: "Commission européenne (décision d'exécution (UE) 2023/1795, communiqué de presse IP/23/3721)",
		Intitule: "Nouveau cadre de transfert UE-États-Unis (Data Privacy Framework)",
		Constat: "La Commission constate que les États-Unis assurent un niveau adéquat de protection pour les données transférées vers " +
			"les organisations inscrites au cadre : les transferts vers ces entreprises sont licites sans autre garantie.",
		URL: urlDPF, Qualite: "OFFICIEL",
		Attendus: []string{"les États-Unis garantissent un niveau de protection adéquat"}},
	{ID: "tribunal-latombe-2025", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "données personnelles", Date: "2025-09-03", Auteur: "Tribunal de l'Union européenne (affaire T-553/23, Latombe/Commission)",
		Intitule: "Rejet du recours contre le cadre de transfert UE-États-Unis",
		Constat: "Le Tribunal rejette le recours du député Philippe Latombe et confirme qu'à la date de la décision de 2023 les États-Unis " +
			"assuraient un niveau adéquat de protection. Le cadre reste en vigueur ; un pourvoi devant la Cour de justice est possible.",
		URL: urlLatombe, Qualite: "OFFICIEL",
		Attendus: []string{"le Tribunal rejette le recours"}},

	// La règle française.
	{ID: "loi-sren-article-31", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "règles de l'État", Date: "2024-05-21", Auteur: "Parlement (loi n° 2024-449 du 21 mai 2024, article 31)",
		Intitule: "Données sensibles de l'État : protection obligatoire contre les accès d'autorités d'États tiers",
		Constat: "Quand l'État, ses opérateurs ou certains groupements d'intérêt public recourent au Cloud d'un prestataire privé (location d'ordinateurs dans des salles serveurs) pour des données d'une " +
			"sensibilité particulière, le service doit les protéger contre tout accès d'autorités publiques d'États tiers non autorisé " +
			"par le droit de l'Union ou d'un État membre. Le IV étend l'obligation au groupement de la plateforme des données de santé. " +
			"Un décret en Conseil d'État devait en fixer les critères dans les six mois.",
		URL: urlLoiSREN, Qualite: "OFFICIEL", JO: joLoiSREN, JOArticle: "31",
		Attendus: []string{"autorités publiques d'Etats tiers", "L. 1462-1 du code de la santé publique"}},
	{ID: "loi-republique-numerique-2016-art16", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "règles de l'État", Date: "2016-10-07", Auteur: "Parlement (loi n° 2016-1321 du 7 octobre 2016 pour une République numérique, article 16)",
		Intitule: "Les administrations veillent à la maîtrise et à l'indépendance de leurs systèmes d'information",
		Constat: "L'État, les collectivités, les autres personnes publiques et les personnes privées chargées d'un service public veillent " +
			"à préserver la maîtrise, la pérennité et l'indépendance de leurs systèmes d'information, et encouragent l'usage des " +
			"logiciels libres et des formats ouverts.",
		URL: urlSenat830, Page: "244", Qualite: "OFFICIEL",
		Attendus: []string{"préserver la maîtrise, la pérennité et l’indépendance de leurs systèmes d’information"}},
	{ID: "doctrine-cloud-au-centre", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "règles de l'État", Date: "2023-05-31", Auteur: "Première ministre (circulaire n° 6404/SG), doctrine publiée par la DINUM",
		Intitule: "Doctrine « cloud au centre » : SecNumCloud et immunité aux lois extra-européennes pour les données sensibles",
		Constat: "Pour les données d'une sensibilité particulière, l'offre Cloud commerciale retenue doit être qualifiée SecNumCloud (ou équivalent " +
			"européen) et immunisée contre tout accès non autorisé d'autorités d'États tiers ; les dérogations relèvent du ministre et du " +
			"Premier ministre. Le contrôle est intégré à l'avis de la DINUM sur les projets de plus de 9 M€.",
		URL: urlDoctrine, Qualite: "OFFICIEL",
		Attendus: []string{"respecter la qualification SecNumCloud", "immunisée contre tout accès non autorisé"}},
	{ID: "anssi-qualification-secnumcloud", Dossier: dossierSouv, Section: "CADRE", Type: "CONSTAT", Theme: "règles de l'État", Auteur: "ANSSI (réponses écrites à la commission d'enquête du Sénat)",
		Intitule: "La qualification SecNumCloud : exigences techniques, organisationnelles et juridiques, deux ans d'instruction",
		Constat: "La qualification vérifie la sécurité du service et sa résistance à une injonction étrangère (cloisonnement, exploitation " +
			"par le seul prestataire qualifié, protection juridique). En 2025, une quinzaine d'offres étaient qualifiées et une douzaine " +
			"en cours, pour 19 sociétés ; taux de réussite de 65 %, instruction de 18 à 24 mois, audits d'au moins 200 000 € sur trois ans.",
		URL: urlSenat830, Page: "245", Qualite: "OFFICIEL",
		Attendus: []string{"Le taux de réussite s’élève à 65 %", "compris entre 18 mois et 24 mois", "de l’ordre de 200 000 euros"}},
	{ID: "dinum-office365-2021", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "règles de l'État", Date: "2021-09-15", Auteur: "Directeur interministériel du numérique (note aux secrétaires généraux)", Groupe: groupeMS,
		Intitule: "Office 365 non conforme à la doctrine « cloud au centre » pour les services de l'État",
		Constat: "La note classe les outils collaboratifs, bureautiques et de messagerie des agents parmi les systèmes manipulant des données " +
			"sensibles, et conclut que leur migration vers Office 365 n'est pas conforme à la doctrine.",
		URL: urlSenat830, Page: "247", Qualite: "OFFICIEL",
		Attendus: []string{"n’est pas conforme à la doctrine cloud au centre"}},
	{ID: "education-suites-non-europeennes-2025", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "règles de l'État", Date: "2025-02-28", Auteur: "Secrétaire général du ministère de l'Éducation nationale (circulaire)",
		Intitule: "Proscription des suites collaboratives en ligne d'éditeurs états-uniens ou non européens dans les écoles",
		Constat: "Le ministère continue de proscrire tout déploiement de suites collaboratives en ligne d'éditeurs états-uniens ou non " +
			"européens dans les écoles et établissements publics.",
		URL: urlSenat830, Page: "252", Qualite: "OFFICIEL",
		Attendus: []string{"continue de proscrire tout déploiement de suites collaboratives en ligne d’éditeurs"}},
	{ID: "courrier-ministres-2025-04-22", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "règles de l'État", Date: "2025-04-22", Auteur: "Ministres de l'Action publique, des Comptes publics et du Numérique (courrier aux membres du Gouvernement)",
		Intitule: "Rappel de la doctrine et refus des achats non soumis à la DINUM à partir du 31 mai 2025",
		Constat: "Les ministres rappellent l'obligation de protéger les données sensibles (solutions collaboratives, bureautiques, messagerie, " +
			"IA) contre les accès d'États tiers et annoncent que chaque contrôleur budgétaire et comptable ministériel refusera tout achat " +
			"qui aurait dû recevoir l'avis préalable de la DINUM. La commission d'enquête y voit la preuve d'un pilotage défaillant.",
		URL: urlSenat830, Page: "256-257", Qualite: "OFFICIEL",
		Attendus: []string{"refusera tout achat"}},

	// Ce que l'enquête du Sénat établit (2025).
	{ID: "anssi-contre-mesures", Dossier: dossierSouv, Section: "ENJEUX", Type: "CONSTAT", Theme: "lois extraterritoriales", Date: "2025-05-28", Auteur: "ANSSI (Vincent Strubel, directeur général, audition)",
		Intitule: "Aucune parade technique ou contractuelle contre les lois extraterritoriales",
		Constat: "Selon le directeur général de l'ANSSI, ni le chiffrement, ni la localisation, ni l'anonymisation, ni un engagement " +
			"contractuel n'empêchent la captation des données en vertu du droit applicable au prestataire.",
		URL: urlSenat830, Page: "241", Qualite: "OFFICIEL",
		Attendus: []string{"ni contre-mesures techniques ni contre-mesures contractuelles efficaces"}},
	{ID: "microsoft-garantie-2025", Dossier: dossierSouv, Section: "ENJEUX", Type: "DECLARATION", Theme: "lois extraterritoriales", Date: "2025-06-10", Auteur: "Microsoft France (directeur des affaires publiques et juridiques, audition sous serment)", Groupe: groupeMS,
		Intitule: "Microsoft France ne peut pas garantir que les données ne seront pas transmises à des autorités étrangères",
		Constat: "Invité à garantir que les données des citoyens français qu'elle héberge ne seront jamais transmises à des autorités " +
			"étrangères sans l'accord des autorités françaises, Microsoft France répond qu'elle ne peut pas le garantir.",
		URL: urlSenat830, Page: "241", Qualite: "OFFICIEL",
		Attendus: []string{"Non, je ne peux pas le garantir"}},
	{ID: "microsoft-transparence-declaratif", Dossier: dossierSouv, Section: "ENJEUX", Type: "DECLARATION", Theme: "lois extraterritoriales", Auteur: "Microsoft France (contribution écrite)", Groupe: groupeMS,
		Intitule: "Aucune entreprise européenne visée par une demande américaine en 2023-2024, selon Microsoft",
		Constat: "Microsoft affirme qu'aucune entreprise européenne n'a fait l'objet d'une demande au titre du CLOUD Act en 2023-2024. " +
			"La commission souligne que ces chiffres, comme les rapports de transparence de l'entreprise, sont purement déclaratifs.",
		URL: urlSenat830, Page: "242", Qualite: "DECLARATIF",
		Attendus: []string{"aucune entreprise européenne n’a été concernée"}},
	{ID: "armees-fournisseurs-2025", Dossier: dossierSouv, Section: "ENJEUX", Type: "DECLARATION", Theme: "continuité et dépendance", Date: "2025-03-25", Auteur: "Ministère des Armées (directeur central du service du commissariat, audition)",
		Intitule: "Les Armées ne peuvent s'engager pour leurs fournisseurs en cas de rupture avec les fournisseurs américains",
		Constat: "Interrogé sur la capacité de la défense à fonctionner si les fournisseurs numériques américains coupaient tout lien, le " +
			"directeur répond que les données étatiques sont hébergées en interne mais qu'il ne peut pas s'engager pour les fournisseurs " +
			"du ministère.",
		URL: urlSenat830, Page: "243", Qualite: "OFFICIEL",
		Attendus: []string{"je ne peux pas m’engager pour les fournisseurs du ministère"}},
	{ID: "cpi-messagerie-2025", Dossier: dossierSouv, Section: "ENJEUX", Type: "DECLARATION", Theme: "continuité et dépendance", Auteur: "Presse, contestée par Microsoft France devant la commission", Groupe: groupeMS,
		Intitule: "Suspension de la messagerie du procureur de la Cour pénale internationale",
		Constat: "Les médias ont rapporté la suspension de la boîte de messagerie du procureur de la CPI ; Microsoft France l'a contesté " +
			"devant la commission, sans en apporter la preuve selon le rapport. Le fait lui-même n'est pas établi par l'institution.",
		URL: urlSenat830, Page: "243", Qualite: "DECLARATIF",
		Attendus: []string{"suspension de la boîte mail du procureur de la Cour pénale internationale"}},
	{ID: "anssi-offres-hybrides-2025", Dossier: dossierSouv, Section: "CONTROLE", Type: "CONSTAT", Theme: "continuité et dépendance", Auteur: "ANSSI (réponses écrites à la commission)",
		Intitule: "Bleu (Orange-Capgemini, Microsoft) et S3NS (Thales, Google Cloud) non qualifiés SecNumCloud en 2025",
		Constat: "Les offres hybrides Bleu et S3NS ont engagé la qualification mais ne sont pas qualifiées à la date de la réponse de " +
			"l'ANSSI (2025). Le catalogue de l'ANSSI chargé ici dit où elles en sont depuis.",
		URL: urlSenat830, Page: "248", Qualite: "OFFICIEL",
		Attendus: []string{"ont intégré le processus de qualification mais ne sont pas à la date de réponse"}},
	{ID: "poupard-hybrides-2025", Dossier: dossierSouv, Section: "ENJEUX", Type: "DECLARATION", Theme: "continuité et dépendance", Date: "2025-05-27", Auteur: "Guillaume Poupard (ancien directeur général de l'ANSSI, audition)",
		Intitule: "Les offres hybrides dépendent de la disponibilité des technologies américaines",
		Constat: "Selon l'ancien directeur de l'ANSSI, si les fournisseurs américains coupaient l'accès à leurs technologies et à leurs " +
			"mises à jour, les systèmes hybrides s'effondreraient en quelques jours ou semaines.",
		URL: urlSenat830, Page: "249", Qualite: "DECLARATIF",
		Attendus: []string{"les systèmes hybrides s’effondreront très rapidement"}},
	{ID: "sren-decret-non-publie-2025", Dossier: dossierSouv, Section: "CONTROLE", Type: "CONSTAT", Theme: "règles de l'État", Auteur: senatCommande,
		Intitule: "Décret de l'article 31 de la loi SREN non publié plus d'un an après la loi ; dérogations pour les suites bureautiques et la plateforme des données de santé",
		Constat: "Le décret attendu dans les six mois n'est toujours pas publié à la fin des travaux de la commission (2025). Selon la " +
			"DINUM, les dérogations portées à sa connaissance concernent des suites bureautiques de ministères ou d'organismes sous " +
			"tutelle et l'hébergement de la plateforme des données de santé.",
		URL: urlSenat830, Page: "251", Qualite: "OFFICIEL",
		Attendus: []string{"ce décret n’a toujours pas été publié", "l’hébergement de la plateforme des données de santé"}},
	{ID: "education-microsoft-2025", Dossier: dossierSouv, Section: "CONTROLE", Type: "CONTRAT", Theme: "achats publics", Date: "2025-03-14", Auteur: senatCommande, Groupe: groupeMS,
		Intitule: "Licences Microsoft de l'Éducation nationale : accord-cadre passé sans l'avis de la DINUM",
		Montant:  "74720000", Nature: "ESTIME",
		Constat: "Accord-cadre de solutions Microsoft pour environ 800 000 postes (74,72 M€ HT estimés sur quatre ans, maximum 152 M€), " +
			"attribué à des revendeurs. La DINUM n'a pas été saisie ; la commission juge le marché passé en méconnaissance de la " +
			"doctrine « cloud au centre », le lot 2 couvrant de l'hébergement Cloud.",
		URL: urlSenat830, Page: "252-256", Qualite: "OFFICIEL",
		Attendus: []string{"74,72 millions d’euros", "n’a pas été saisie en amont"}},
	{ID: "education-alternatives-couts", Dossier: dossierSouv, Section: "CONTROLE", Type: "DECLARATION", Theme: "achats publics", Auteur: "Responsable ministériel des achats de l'Éducation nationale (réponses écrites), contredit par un éditeur",
		Intitule: "Solutions souveraines « 200 % à 1 300 % plus chères » selon le ministère ; un éditeur cité dit n'avoir pas été consulté",
		Constat: "Le ministère justifie le choix de Microsoft par des solutions souveraines plus chères de 200 % à 1 300 % ; l'un des quatre " +
			"éditeurs présentés comme consultés affirme n'avoir « aucunement été consulté » ni proposé de prix.",
		URL: urlSenat830, Page: "258", Qualite: "DECLARATIF",
		Attendus: []string{"200 % à 1 300 %", "aucunement été consulté"}},
	{ID: "cloud-marche-francais-71", Dossier: dossierSouv, Section: "SITUATION", Type: "DECLARATION", Theme: "continuité et dépendance", Auteur: "France Digitale (réponses écrites à la commission)",
		Intitule: "71 % du marché français du cloud détenus par AWS, Google Cloud et Microsoft Azure",
		Constat:  "Chiffre avancé par une association professionnelle et repris par la commission, sans source statistique publique.",
		URL:      urlSenat830, Page: "258", Qualite: "DECLARATIF",
		Attendus: []string{"71 % du marché français du cloud"}},
	{ID: "ugap-multi-editeurs", Dossier: dossierSouv, Section: "CONTROLE", Type: "CONSTAT", Theme: "achats publics", Auteur: senatCommande,
		Intitule: "Bibliothèque multi-éditeurs de l'UGAP : 1,49 Md€ de commandes sans filtre d'exposition au droit étranger",
		Montant:  "1490000000", Nature: "COMMANDE",
		Constat: "Entre avril 2023 et le 10 mars 2025, 1,49 Md€ de commandes ont été passées via ce marché de l'UGAP (titulaire SCC France). " +
			"Le catalogue ne permet pas à l'acheteur d'identifier les éditeurs dont l'offre est immunisée contre le droit extraterritorial.",
		URL: urlSenat830, Page: "264-265", Qualite: "OFFICIEL",
		Attendus: []string{"1,49 milliard d’euros", "n’intègre pas de fonctionnalité"}},
	{ID: "ugap-microsoft-2024", Dossier: dossierSouv, Section: "SITUATION", Type: "CONSTAT", Theme: "continuité et dépendance", Date: "2024-12-31", Auteur: senatCommande, Groupe: groupeMS,
		Intitule: "Microsoft, 230 M€ de ventes de l'UGAP en 2024 ; sept des dix prestations les plus vendues début 2025",
		Montant:  "230000000", Nature: "VENTES",
		Constat: "Les marchés dédiés de l'UGAP ont représenté environ 230 M€ de ventes pour Microsoft et 100 M€ pour Oracle en 2024 ; au " +
			"premier trimestre 2025, sept des dix prestations de services les plus vendues par l'UGAP concernaient Microsoft.",
		URL: urlSenat830, Page: "265", Qualite: "OFFICIEL",
		Attendus: []string{"230 millions et 100 millions d’euros", "sept des dix prestations"}},
	{ID: "ugap-cloud-2020-2025", Dossier: dossierSouv, Section: "SITUATION", Type: "CONSTAT", Theme: "achats publics", Date: "2025-05-31", Auteur: "DINUM (réponse écrite à la commission d'enquête du Sénat)",
		Intitule: "Marché d'hébergement Cloud de l'UGAP : 146 M€ de commandes, 29 % chez des fournisseurs qualifiés SecNumCloud",
		Montant:  "146000000", Nature: "COMMANDE",
		Constat: "D'octobre 2020 au 31 mai 2025 : 146 M€ de commandes, dont 64 % à des fournisseurs français (29 % qualifiés SecNumCloud, " +
			"35 % non qualifiés). Parts : OVHcloud 37 %, Microsoft 19 %, Outscale 11 %, AWS 8 %, Scaleway 7 %. Le montant ne couvre ni le " +
			"logiciel à la demande ni les achats hors de ce marché.",
		URL: urlSenat830, Page: "265-266", Qualite: "OFFICIEL",
		Attendus: []string{"146 millions d’euros de commandes publiques cumulées", "OVHcloud en a été le premier bénéficiaire", "29 % à des fournisseurs qualifiés SecNumCloud"}},
	{ID: "depense-it-etat-2024", Dossier: dossierSouv, Section: "SITUATION", Type: "CONSTAT", Theme: "achats publics", Date: "2024-12-31", Auteur: "DINUM (réponse écrite à la commission d'enquête du Sénat)",
		Intitule: "Dépense informatique de l'État : 4,5 Md€ en 2024, dépense Cloud non suivie",
		Montant:  "4500000000", Nature: "DEPENSE",
		Constat: "L'extraction Chorus fait état de 4,5 Md€ de dépenses informatiques de l'État en 2024 (matériel, licences, prestations), " +
			"chiffre que la commission juge peu fiable ; la dépense publique de Cloud ne fait l'objet d'aucun suivi " +
			"centralisé. Aucune ventilation par fournisseur ni par nationalité n'est publiée.",
		URL: urlSenat830, Page: "265-266", Qualite: "OFFICIEL",
		Attendus: []string{"4,5 milliards d’euros en 2024", "ne fait pas l’objet d’un suivi centralisé"}},
	{ID: "senat-recommandations-22-31", Dossier: dossierSouv, Section: "CONTROLE", Type: "RECOMMANDATION", Theme: "règles de l'État", Auteur: senatCommande,
		Intitule: "Décret SREN, toutes les données publiques sensibles, clause de non-soumission aux lois extraterritoriales, SecNumCloud obligatoire",
		Constat: "La commission recommande de publier le décret de l'article 31, de considérer toutes les données publiques comme sensibles, " +
			"d'imposer une clause de non-soumission aux lois extraterritoriales dans les marchés d'hébergement et de conseil, de faire " +
			"respecter SecNumCloud pour les données sensibles en privilégiant les technologies intégralement souveraines, et de faire de " +
			"l'UGAP un outil de souveraineté (recommandations 22 à 31).",
		URL: urlSenat830, Page: "268-275", Qualite: "OFFICIEL",
		Attendus: []string{"clause de non-soumission aux lois extraterritoriales"}},

	// Sanctions nommées par l'autorité elle-même à la date du chargement.
	{ID: "dpc-meta-transferts-2023", Dossier: dossierSouv, Section: "SITUATION", Type: "SANCTION", Theme: "données personnelles", Date: "2023-05-22", Auteur: "Data Protection Commission (Irlande), sur décision contraignante du Comité européen de la protection des données",
		Groupe: "Meta Platforms Inc.", Intitule: "Amende de 1,2 Md€ pour transferts de données de Facebook vers les États-Unis",
		Montant: "1200000000", Nature: "AMENDE",
		Constat: "Meta Ireland a transféré des données personnelles d'utilisateurs européens vers les États-Unis sur la base de clauses " +
			"contractuelles types sans protéger ces données de la surveillance américaine décrite par l'arrêt Schrems II.",
		URL: urlDPCMeta, Qualite: "OFFICIEL",
		Attendus: []string{"€1.2 billion"}},
	{ID: "cnil-google-2025", Dossier: dossierSouv, Section: "SITUATION", Type: "SANCTION", Theme: "données personnelles", Date: "2025-09-01", Auteur: "CNIL (formation restreinte)",
		Groupe: "Alphabet Inc.", Intitule: "Amende de 325 M€ : publicités entre les courriels Gmail et traceurs sans consentement",
		Montant: "325000000", Nature: "AMENDE",
		Constat: "Sanction portant sur les services grand public de Google (Gmail, création de comptes), pas sur un contrat public.",
		URL:     urlCNILGoogle, Qualite: "OFFICIEL",
		Attendus: []string{"325 millions d’euros"}},
}
