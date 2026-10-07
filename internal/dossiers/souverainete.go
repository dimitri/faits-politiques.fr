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
	groupMS       = "Microsoft Corporation"
)

func init() {
	facts = append(facts, factsSouverainete...)
}

var factsSouverainete = []Fact{
	// Le contexte : l'intitulé du ministre chargé de l'économie.
	{ID: "ministere-souverainete-numerique-2022", Dossier: dossierSouv, Section: "CONTEXTE", Type: "TEXTE", Theme: "intitulés ministériels", Date: "2022-06-01", Author: "Gouvernement (décret n° 2022-826 du 1er juin 2022)",
		Title: "Le ministre de l'Économie devient ministre « de la souveraineté industrielle et numérique »",
		Finding: "Pour la première fois, la souveraineté numérique figure dans l'intitulé du ministre de l'Économie et des Finances. " +
			"En 2014, l'intitulé comprenait « le numérique », sans souveraineté.",
		URL: urlJORF + "JORFTEXT000045847934", Quality: "OFFICIEL", JO: "JORFTEXT000045847934",
		Expected: []string{"souveraineté industrielle et numérique"}},
	{ID: "ministere-intitule-2024", Dossier: dossierSouv, Section: "CONTEXTE", Type: "TEXTE", Theme: "intitulés ministériels", Date: "2024-10-10", Author: "Gouvernement (décret n° 2024-916 du 10 octobre 2024)",
		Title: "L'intitulé disparaît : ministre « de l'économie, des finances et de l'industrie »",
		Finding: "Dans le gouvernement formé en septembre 2024, le ministre de l'Économie n'a plus la souveraineté numérique dans son intitulé ; " +
			"le numérique relève d'une secrétaire d'État chargée de l'intelligence artificielle et du numérique.",
		URL: urlJORF + "JORFTEXT000050330364", Quality: "OFFICIEL", JO: "JORFTEXT000050330364",
		Expected: []string{"ministre de l'économie, des finances et de l'industrie"}},
	{ID: "ministere-intitule-2025", Dossier: dossierSouv, Section: "CONTEXTE", Type: "TEXTE", Theme: "intitulés ministériels", Date: "2025-01-08", Author: "Gouvernement (décret n° 2025-20 du 8 janvier 2025)",
		Title:   "L'intitulé revient : « souveraineté industrielle et numérique »",
		Finding: "Le gouvernement formé en décembre 2024 rétablit la souveraineté industrielle et numérique dans l'intitulé du ministre de l'Économie.",
		URL:     urlJORF + "JORFTEXT000050959974", Quality: "OFFICIEL", JO: "JORFTEXT000050959974",
		Expected: []string{"souveraineté industrielle et numérique"}},

	// Les lois étrangères et la jurisprudence européenne.
	{ID: "cloud-act-2018", Dossier: dossierSouv, Section: "ENJEUX", Type: "TEXTE", Theme: "lois extraterritoriales", Date: "2018-03-23", Author: "Congrès des États-Unis",
		Title: "CLOUD Act (Clarifying Lawful Overseas Use of Data Act), section 103, 18 U.S.C. 2713",
		Finding: "Un fournisseur de services de communication électronique ou d'informatique à distance soumis au droit américain " +
			"doit préserver ou divulguer les contenus et données de ses clients qu'il a en sa possession, sa garde ou son contrôle, " +
			"que ces données se trouvent ou non aux États-Unis.",
		URL: urlCloudAct, Quality: "OFFICIEL",
		Expected: []string{"regardless of whether such communication, record, or other information is located within or outside of the United States"}},
	{ID: "fisa-702", Dossier: dossierSouv, Section: "ENJEUX", Type: "TEXTE", Theme: "lois extraterritoriales", Author: "Congrès des États-Unis",
		Title: "FISA, section 702 (50 U.S.C. 1881a) : collecte de renseignement visant des personnes situées hors des États-Unis",
		Finding: "Sur autorisation conjointe du ministre de la Justice et du directeur du renseignement national, approuvée par la cour FISA, " +
			"un fournisseur de services de communication électronique peut être contraint de fournir immédiatement toute information " +
			"et toute assistance nécessaires, en gardant le secret. Texte lu dans l'édition 2023 du code ; la section a été prorogée " +
			"en avril 2024 pour deux ans, et son état après avril 2026 n'est pas vérifié ici.",
		URL: urlFISA702, Quality: "OFFICIEL",
		Expected: []string{"immediately provide the Government with all information, facilities, or assistance necessary"}},
	{ID: "cjue-schrems-ii-2020", Dossier: dossierSouv, Section: "ENJEUX", Type: "TEXTE", Theme: "données personnelles", Date: "2020-07-16", Author: "Cour de justice de l'Union européenne (affaire C-311/18, communiqué de presse n° 91/20)",
		Title: "Arrêt « Schrems II » : invalidation du bouclier de protection des données UE-États-Unis",
		Finding: "La Cour invalide la décision d'adéquation de 2016 (Privacy Shield) : les programmes de surveillance fondés sur le droit " +
			"américain ne limitent pas l'accès aux données transférées au strict nécessaire et n'offrent pas de recours effectif aux " +
			"personnes concernées.",
		URL: urlSchremsII, Quality: "OFFICIEL",
		Expected: []string{"la Cour déclare la décision 2016/1250 invalide"}},
	{ID: "ue-adequation-dpf-2023", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "données personnelles", Date: "2023-07-10", Author: "Commission européenne (décision d'exécution (UE) 2023/1795, communiqué de presse IP/23/3721)",
		Title: "Nouveau cadre de transfert UE-États-Unis (Data Privacy Framework)",
		Finding: "La Commission constate que les États-Unis assurent un niveau adéquat de protection pour les données transférées vers " +
			"les organisations inscrites au cadre : les transferts vers ces entreprises sont licites sans autre garantie.",
		URL: urlDPF, Quality: "OFFICIEL",
		Expected: []string{"les États-Unis garantissent un niveau de protection adéquat"}},
	{ID: "tribunal-latombe-2025", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "données personnelles", Date: "2025-09-03", Author: "Tribunal de l'Union européenne (affaire T-553/23, Latombe/Commission)",
		Title: "Rejet du recours contre le cadre de transfert UE-États-Unis",
		Finding: "Le Tribunal rejette le recours du député Philippe Latombe et confirme qu'à la date de la décision de 2023 les États-Unis " +
			"assuraient un niveau adéquat de protection. Le cadre reste en vigueur ; un pourvoi devant la Cour de justice est possible.",
		URL: urlLatombe, Quality: "OFFICIEL",
		Expected: []string{"le Tribunal rejette le recours"}},

	// La règle française.
	{ID: "loi-sren-article-31", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "règles de l'État", Date: "2024-05-21", Author: "Parlement (loi n° 2024-449 du 21 mai 2024, article 31)",
		Title: "Données sensibles de l'État : protection obligatoire contre les accès d'autorités d'États tiers",
		Finding: "Quand l'État, ses opérateurs ou certains groupements d'intérêt public recourent au Cloud d'un prestataire privé (location d'ordinateurs dans des salles serveurs) pour des données d'une " +
			"sensibilité particulière, le service doit les protéger contre tout accès d'autorités publiques d'États tiers non autorisé " +
			"par le droit de l'Union ou d'un État membre. Le IV étend l'obligation au groupement de la plateforme des données de santé. " +
			"Un décret en Conseil d'État devait en fixer les critères dans les six mois.",
		URL: urlLoiSREN, Quality: "OFFICIEL", JO: joLoiSREN, JOArticle: "31",
		Expected: []string{"autorités publiques d'Etats tiers", "L. 1462-1 du code de la santé publique"}},
	{ID: "loi-republique-numerique-2016-art16", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "règles de l'État", Date: "2016-10-07", Author: "Parlement (loi n° 2016-1321 du 7 octobre 2016 pour une République numérique, article 16)",
		Title: "Les administrations veillent à la maîtrise et à l'indépendance de leurs systèmes d'information",
		Finding: "L'État, les collectivités, les autres personnes publiques et les personnes privées chargées d'un service public veillent " +
			"à préserver la maîtrise, la pérennité et l'indépendance de leurs systèmes d'information, et encouragent l'usage des " +
			"logiciels libres et des formats ouverts.",
		URL: urlSenat830, Page: "244", Quality: "OFFICIEL",
		Expected: []string{"préserver la maîtrise, la pérennité et l’indépendance de leurs systèmes d’information"}},
	{ID: "doctrine-cloud-au-centre", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "règles de l'État", Date: "2023-05-31", Author: "Première ministre (circulaire n° 6404/SG), doctrine publiée par la DINUM",
		Title: "Doctrine « cloud au centre » : SecNumCloud et immunité aux lois extra-européennes pour les données sensibles",
		Finding: "Pour les données d'une sensibilité particulière, l'offre Cloud commerciale retenue doit être qualifiée SecNumCloud (ou équivalent " +
			"européen) et immunisée contre tout accès non autorisé d'autorités d'États tiers ; les dérogations relèvent du ministre et du " +
			"Premier ministre. Le contrôle est intégré à l'avis de la DINUM sur les projets de plus de 9 M€.",
		URL: urlDoctrine, Quality: "OFFICIEL",
		Expected: []string{"respecter la qualification SecNumCloud", "immunisée contre tout accès non autorisé"}},
	{ID: "anssi-qualification-secnumcloud", Dossier: dossierSouv, Section: "CADRE", Type: "CONSTAT", Theme: "règles de l'État", Author: "ANSSI (réponses écrites à la commission d'enquête du Sénat)",
		Title: "La qualification SecNumCloud : exigences techniques, organisationnelles et juridiques, deux ans d'instruction",
		Finding: "La qualification vérifie la sécurité du service et sa résistance à une injonction étrangère (cloisonnement, exploitation " +
			"par le seul prestataire qualifié, protection juridique). En 2025, une quinzaine d'offres étaient qualifiées et une douzaine " +
			"en cours, pour 19 sociétés ; taux de réussite de 65 %, instruction de 18 à 24 mois, audits d'au moins 200 000 € sur trois ans.",
		URL: urlSenat830, Page: "245", Quality: "OFFICIEL",
		Expected: []string{"Le taux de réussite s’élève à 65 %", "compris entre 18 mois et 24 mois", "de l’ordre de 200 000 euros"}},
	{ID: "dinum-office365-2021", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "règles de l'État", Date: "2021-09-15", Author: "Directeur interministériel du numérique (note aux secrétaires généraux)", Group: groupMS,
		Title: "Office 365 non conforme à la doctrine « cloud au centre » pour les services de l'État",
		Finding: "La note classe les outils collaboratifs, bureautiques et de messagerie des agents parmi les systèmes manipulant des données " +
			"sensibles, et conclut que leur migration vers Office 365 n'est pas conforme à la doctrine.",
		URL: urlSenat830, Page: "247", Quality: "OFFICIEL",
		Expected: []string{"n’est pas conforme à la doctrine cloud au centre"}},
	{ID: "education-suites-non-europeennes-2025", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "règles de l'État", Date: "2025-02-28", Author: "Secrétaire général du ministère de l'Éducation nationale (circulaire)",
		Title: "Proscription des suites collaboratives en ligne d'éditeurs états-uniens ou non européens dans les écoles",
		Finding: "Le ministère continue de proscrire tout déploiement de suites collaboratives en ligne d'éditeurs états-uniens ou non " +
			"européens dans les écoles et établissements publics.",
		URL: urlSenat830, Page: "252", Quality: "OFFICIEL",
		Expected: []string{"continue de proscrire tout déploiement de suites collaboratives en ligne d’éditeurs"}},
	{ID: "courrier-ministres-2025-04-22", Dossier: dossierSouv, Section: "CADRE", Type: "TEXTE", Theme: "règles de l'État", Date: "2025-04-22", Author: "Ministres de l'Action publique, des Comptes publics et du Numérique (courrier aux membres du Gouvernement)",
		Title: "Rappel de la doctrine et refus des achats non soumis à la DINUM à partir du 31 mai 2025",
		Finding: "Les ministres rappellent l'obligation de protéger les données sensibles (solutions collaboratives, bureautiques, messagerie, " +
			"IA) contre les accès d'États tiers et annoncent que chaque contrôleur budgétaire et comptable ministériel refusera tout achat " +
			"qui aurait dû recevoir l'avis préalable de la DINUM. La commission d'enquête y voit la preuve d'un pilotage défaillant.",
		URL: urlSenat830, Page: "256-257", Quality: "OFFICIEL",
		Expected: []string{"refusera tout achat"}},

	// Ce que l'enquête du Sénat établit (2025).
	{ID: "anssi-contre-mesures", Dossier: dossierSouv, Section: "ENJEUX", Type: "CONSTAT", Theme: "lois extraterritoriales", Date: "2025-05-28", Author: "ANSSI (Vincent Strubel, directeur général, audition)",
		Title: "Aucune parade technique ou contractuelle contre les lois extraterritoriales",
		Finding: "Selon le directeur général de l'ANSSI, ni le chiffrement, ni la localisation, ni l'anonymisation, ni un engagement " +
			"contractuel n'empêchent la captation des données en vertu du droit applicable au prestataire.",
		URL: urlSenat830, Page: "241", Quality: "OFFICIEL",
		Expected: []string{"ni contre-mesures techniques ni contre-mesures contractuelles efficaces"}},
	{ID: "microsoft-garantie-2025", Dossier: dossierSouv, Section: "ENJEUX", Type: "DECLARATION", Theme: "lois extraterritoriales", Date: "2025-06-10", Author: "Microsoft France (directeur des affaires publiques et juridiques, audition sous serment)", Group: groupMS,
		Title: "Microsoft France ne peut pas garantir que les données ne seront pas transmises à des autorités étrangères",
		Finding: "Invité à garantir que les données des citoyens français qu'elle héberge ne seront jamais transmises à des autorités " +
			"étrangères sans l'accord des autorités françaises, Microsoft France répond qu'elle ne peut pas le garantir.",
		URL: urlSenat830, Page: "241", Quality: "OFFICIEL",
		Expected: []string{"Non, je ne peux pas le garantir"}},
	{ID: "microsoft-transparence-declaratif", Dossier: dossierSouv, Section: "ENJEUX", Type: "DECLARATION", Theme: "lois extraterritoriales", Author: "Microsoft France (contribution écrite)", Group: groupMS,
		Title: "Aucune entreprise européenne visée par une demande américaine en 2023-2024, selon Microsoft",
		Finding: "Microsoft affirme qu'aucune entreprise européenne n'a fait l'objet d'une demande au titre du CLOUD Act en 2023-2024. " +
			"La commission souligne que ces chiffres, comme les rapports de transparence de l'entreprise, sont purement déclaratifs.",
		URL: urlSenat830, Page: "242", Quality: "DECLARATIF",
		Expected: []string{"aucune entreprise européenne n’a été concernée"}},
	{ID: "armees-fournisseurs-2025", Dossier: dossierSouv, Section: "ENJEUX", Type: "DECLARATION", Theme: "continuité et dépendance", Date: "2025-03-25", Author: "Ministère des Armées (directeur central du service du commissariat, audition)",
		Title: "Les Armées ne peuvent s'engager pour leurs fournisseurs en cas de rupture avec les fournisseurs américains",
		Finding: "Interrogé sur la capacité de la défense à fonctionner si les fournisseurs numériques américains coupaient tout lien, le " +
			"directeur répond que les données étatiques sont hébergées en interne mais qu'il ne peut pas s'engager pour les fournisseurs " +
			"du ministère.",
		URL: urlSenat830, Page: "243", Quality: "OFFICIEL",
		Expected: []string{"je ne peux pas m’engager pour les fournisseurs du ministère"}},
	{ID: "cpi-messagerie-2025", Dossier: dossierSouv, Section: "ENJEUX", Type: "DECLARATION", Theme: "continuité et dépendance", Author: "Presse, contestée par Microsoft France devant la commission", Group: groupMS,
		Title: "Suspension de la messagerie du procureur de la Cour pénale internationale",
		Finding: "Les médias ont rapporté la suspension de la boîte de messagerie du procureur de la CPI ; Microsoft France l'a contesté " +
			"devant la commission, sans en apporter la preuve selon le rapport. Le fait lui-même n'est pas établi par l'institution.",
		URL: urlSenat830, Page: "243", Quality: "DECLARATIF",
		Expected: []string{"suspension de la boîte mail du procureur de la Cour pénale internationale"}},
	{ID: "anssi-offres-hybrides-2025", Dossier: dossierSouv, Section: "CONTROLE", Type: "CONSTAT", Theme: "continuité et dépendance", Author: "ANSSI (réponses écrites à la commission)",
		Title: "Bleu (Orange-Capgemini, Microsoft) et S3NS (Thales, Google Cloud) non qualifiés SecNumCloud en 2025",
		Finding: "Les offres hybrides Bleu et S3NS ont engagé la qualification mais ne sont pas qualifiées à la date de la réponse de " +
			"l'ANSSI (2025). Le catalogue de l'ANSSI chargé ici dit où elles en sont depuis.",
		URL: urlSenat830, Page: "248", Quality: "OFFICIEL",
		Expected: []string{"ont intégré le processus de qualification mais ne sont pas à la date de réponse"}},
	{ID: "poupard-hybrides-2025", Dossier: dossierSouv, Section: "ENJEUX", Type: "DECLARATION", Theme: "continuité et dépendance", Date: "2025-05-27", Author: "Guillaume Poupard (ancien directeur général de l'ANSSI, audition)",
		Title: "Les offres hybrides dépendent de la disponibilité des technologies américaines",
		Finding: "Selon l'ancien directeur de l'ANSSI, si les fournisseurs américains coupaient l'accès à leurs technologies et à leurs " +
			"mises à jour, les systèmes hybrides s'effondreraient en quelques jours ou semaines.",
		URL: urlSenat830, Page: "249", Quality: "DECLARATIF",
		Expected: []string{"les systèmes hybrides s’effondreront très rapidement"}},
	{ID: "sren-decret-non-publie-2025", Dossier: dossierSouv, Section: "CONTROLE", Type: "CONSTAT", Theme: "règles de l'État", Author: senatCommande,
		Title: "Décret de l'article 31 de la loi SREN non publié plus d'un an après la loi ; dérogations pour les suites bureautiques et la plateforme des données de santé",
		Finding: "Le décret attendu dans les six mois n'est toujours pas publié à la fin des travaux de la commission (2025). Selon la " +
			"DINUM, les dérogations portées à sa connaissance concernent des suites bureautiques de ministères ou d'organismes sous " +
			"tutelle et l'hébergement de la plateforme des données de santé.",
		URL: urlSenat830, Page: "251", Quality: "OFFICIEL",
		Expected: []string{"ce décret n’a toujours pas été publié", "l’hébergement de la plateforme des données de santé"}},
	{ID: "education-microsoft-2025", Dossier: dossierSouv, Section: "CONTROLE", Type: "CONTRAT", Theme: "achats publics", Date: "2025-03-14", Author: senatCommande, Group: groupMS,
		Title:  "Licences Microsoft de l'Éducation nationale : accord-cadre passé sans l'avis de la DINUM",
		Amount: "74720000", Nature: "ESTIME",
		Finding: "Accord-cadre de solutions Microsoft pour environ 800 000 postes (74,72 M€ HT estimés sur quatre ans, maximum 152 M€), " +
			"attribué à des revendeurs. La DINUM n'a pas été saisie ; la commission juge le marché passé en méconnaissance de la " +
			"doctrine « cloud au centre », le lot 2 couvrant de l'hébergement Cloud.",
		URL: urlSenat830, Page: "252-256", Quality: "OFFICIEL",
		Expected: []string{"74,72 millions d’euros", "n’a pas été saisie en amont"}},
	{ID: "education-alternatives-couts", Dossier: dossierSouv, Section: "CONTROLE", Type: "DECLARATION", Theme: "achats publics", Author: "Responsable ministériel des achats de l'Éducation nationale (réponses écrites), contredit par un éditeur",
		Title: "Solutions souveraines « 200 % à 1 300 % plus chères » selon le ministère ; un éditeur cité dit n'avoir pas été consulté",
		Finding: "Le ministère justifie le choix de Microsoft par des solutions souveraines plus chères de 200 % à 1 300 % ; l'un des quatre " +
			"éditeurs présentés comme consultés affirme n'avoir « aucunement été consulté » ni proposé de prix.",
		URL: urlSenat830, Page: "258", Quality: "DECLARATIF",
		Expected: []string{"200 % à 1 300 %", "aucunement été consulté"}},
	{ID: "cloud-marche-francais-71", Dossier: dossierSouv, Section: "SITUATION", Type: "DECLARATION", Theme: "continuité et dépendance", Author: "France Digitale (réponses écrites à la commission)",
		Title:   "71 % du marché français du cloud détenus par AWS, Google Cloud et Microsoft Azure",
		Finding: "Chiffre avancé par une association professionnelle et repris par la commission, sans source statistique publique.",
		URL:     urlSenat830, Page: "258", Quality: "DECLARATIF",
		Expected: []string{"71 % du marché français du cloud"}},
	{ID: "ugap-multi-editeurs", Dossier: dossierSouv, Section: "CONTROLE", Type: "CONSTAT", Theme: "achats publics", Author: senatCommande,
		Title:  "Bibliothèque multi-éditeurs de l'UGAP : 1,49 Md€ de commandes sans filtre d'exposition au droit étranger",
		Amount: "1490000000", Nature: "COMMANDE",
		Finding: "Entre avril 2023 et le 10 mars 2025, 1,49 Md€ de commandes ont été passées via ce marché de l'UGAP (titulaire SCC France). " +
			"Le catalogue ne permet pas à l'acheteur d'identifier les éditeurs dont l'offre est immunisée contre le droit extraterritorial.",
		URL: urlSenat830, Page: "264-265", Quality: "OFFICIEL",
		Expected: []string{"1,49 milliard d’euros", "n’intègre pas de fonctionnalité"}},
	{ID: "ugap-microsoft-2024", Dossier: dossierSouv, Section: "SITUATION", Type: "CONSTAT", Theme: "continuité et dépendance", Date: "2024-12-31", Author: senatCommande, Group: groupMS,
		Title:  "Microsoft, 230 M€ de ventes de l'UGAP en 2024 ; sept des dix prestations les plus vendues début 2025",
		Amount: "230000000", Nature: "VENTES",
		Finding: "Les marchés dédiés de l'UGAP ont représenté environ 230 M€ de ventes pour Microsoft et 100 M€ pour Oracle en 2024 ; au " +
			"premier trimestre 2025, sept des dix prestations de services les plus vendues par l'UGAP concernaient Microsoft.",
		URL: urlSenat830, Page: "265", Quality: "OFFICIEL",
		Expected: []string{"230 millions et 100 millions d’euros", "sept des dix prestations"}},
	{ID: "ugap-cloud-2020-2025", Dossier: dossierSouv, Section: "SITUATION", Type: "CONSTAT", Theme: "achats publics", Date: "2025-05-31", Author: "DINUM (réponse écrite à la commission d'enquête du Sénat)",
		Title:  "Marché d'hébergement Cloud de l'UGAP : 146 M€ de commandes, 29 % chez des fournisseurs qualifiés SecNumCloud",
		Amount: "146000000", Nature: "COMMANDE",
		Finding: "D'octobre 2020 au 31 mai 2025 : 146 M€ de commandes, dont 64 % à des fournisseurs français (29 % qualifiés SecNumCloud, " +
			"35 % non qualifiés). Parts : OVHcloud 37 %, Microsoft 19 %, Outscale 11 %, AWS 8 %, Scaleway 7 %. Le montant ne couvre ni le " +
			"logiciel à la demande ni les achats hors de ce marché.",
		URL: urlSenat830, Page: "265-266", Quality: "OFFICIEL",
		Expected: []string{"146 millions d’euros de commandes publiques cumulées", "OVHcloud en a été le premier bénéficiaire", "29 % à des fournisseurs qualifiés SecNumCloud"}},
	{ID: "depense-it-etat-2024", Dossier: dossierSouv, Section: "SITUATION", Type: "CONSTAT", Theme: "achats publics", Date: "2024-12-31", Author: "DINUM (réponse écrite à la commission d'enquête du Sénat)",
		Title:  "Dépense informatique de l'État : 4,5 Md€ en 2024, dépense Cloud non suivie",
		Amount: "4500000000", Nature: "DEPENSE",
		Finding: "L'extraction Chorus fait état de 4,5 Md€ de dépenses informatiques de l'État en 2024 (matériel, licences, prestations), " +
			"chiffre que la commission juge peu fiable ; la dépense publique de Cloud ne fait l'objet d'aucun suivi " +
			"centralisé. Aucune ventilation par fournisseur ni par nationalité n'est publiée.",
		URL: urlSenat830, Page: "265-266", Quality: "OFFICIEL",
		Expected: []string{"4,5 milliards d’euros en 2024", "ne fait pas l’objet d’un suivi centralisé"}},
	{ID: "senat-recommandations-22-31", Dossier: dossierSouv, Section: "CONTROLE", Type: "RECOMMANDATION", Theme: "règles de l'État", Author: senatCommande,
		Title: "Décret SREN, toutes les données publiques sensibles, clause de non-soumission aux lois extraterritoriales, SecNumCloud obligatoire",
		Finding: "La commission recommande de publier le décret de l'article 31, de considérer toutes les données publiques comme sensibles, " +
			"d'imposer une clause de non-soumission aux lois extraterritoriales dans les marchés d'hébergement et de conseil, de faire " +
			"respecter SecNumCloud pour les données sensibles en privilégiant les technologies intégralement souveraines, et de faire de " +
			"l'UGAP un outil de souveraineté (recommandations 22 à 31).",
		URL: urlSenat830, Page: "268-275", Quality: "OFFICIEL",
		Expected: []string{"clause de non-soumission aux lois extraterritoriales"}},

	// Sanctions nommées par l'autorité elle-même à la date du chargement.
	{ID: "dpc-meta-transferts-2023", Dossier: dossierSouv, Section: "SITUATION", Type: "SANCTION", Theme: "données personnelles", Date: "2023-05-22", Author: "Data Protection Commission (Irlande), sur décision contraignante du Comité européen de la protection des données",
		Group: "Meta Platforms Inc.", Title: "Amende de 1,2 Md€ pour transferts de données de Facebook vers les États-Unis",
		Amount: "1200000000", Nature: "AMENDE",
		Finding: "Meta Ireland a transféré des données personnelles d'utilisateurs européens vers les États-Unis sur la base de clauses " +
			"contractuelles types sans protéger ces données de la surveillance américaine décrite par l'arrêt Schrems II.",
		URL: urlDPCMeta, Quality: "OFFICIEL",
		Expected: []string{"€1.2 billion"}},
	{ID: "cnil-google-2025", Dossier: dossierSouv, Section: "SITUATION", Type: "SANCTION", Theme: "données personnelles", Date: "2025-09-01", Author: "CNIL (formation restreinte)",
		Group: "Alphabet Inc.", Title: "Amende de 325 M€ : publicités entre les courriels Gmail et traceurs sans consentement",
		Amount: "325000000", Nature: "AMENDE",
		Finding: "Sanction portant sur les services grand public de Google (Gmail, création de comptes), pas sur un contrat public.",
		URL:     urlCNILGoogle, Quality: "OFFICIEL",
		Expected: []string{"325 millions d’euros"}},
}
