package dossiers

// Les faits de contexte, de cadre et de contrôle des dossiers statistiques
// (budget, dette, emploi, retraite, pauvreté, éducation, santé, sécurité,
// défense, pouvoirs publics, immigration, écologie, eau, évasion fiscale). Les
// chiffres des dossiers restent dans leurs tables ; ces faits disent dans quel
// cadre ils s'inscrivent et ce qu'en ont conclu les institutions de contrôle.

const (
	urlHCFP2025          = "https://www.hcfp.fr/sites/default/files/2025-10/Avis%20HCFP%202025%20%E2%80%93%205%20PLF-PLFSS%202026_0.pdf"
	urlCOR2025           = "https://www.cor-retraites.fr/sites/default/files/2025-06/Synth%C3%A8se_Def_.pdf"
	urlHCC2025           = "https://www.hautconseilclimat.fr/wp-content/uploads/2025/07/HCC_RA_2025-VDEF0207_web.pdf"
	urlUnedic2026        = "https://www.unedic.org/storage/uploads/2026/03/04/situation-financiere-assurance-chomage-a-horizon-2028_03-mars-2026_uid_69a7f392cd38b.pdf"
	urlDDD2024           = "https://www.defenseurdesdroits.fr/sites/default/files/2025-03/ddd_rapport-annuel-2024_20250305.pdf"
	urlSenatDefense      = "https://www.senat.fr/rap/l25-139-38/l25-139-38-syn.pdf"
	urlSenatPouvoirs     = "https://www.senat.fr/rap/l25-139-322/l25-139-322-syn.pdf"
	urlSenatEduc         = "https://www.senat.fr/rap/a25-144-31/a25-144-31-syn.pdf"
	urlSenatSecu         = "https://www.senat.fr/rap/a25-145-12/a25-145-12-syn.pdf"
	urlSenatImmig        = "https://www.senat.fr/rap/l25-139-315/l25-139-315_mono.html"
	urlSenatSolid        = "https://www.senat.fr/rap/a25-142-5/a25-142-5-syn.pdf"
	urlSenatPLFSS        = "https://www.senat.fr/lessentiel/plfss2026.pdf"
	urlSenatEau          = "https://www.senat.fr/fileadmin/Office_et_delegations/Annexe_-_Essentiel_-_Les_53_propositions.pdf"
	urlCPOPatrimoine     = "https://www.ccomptes.fr/sites/default/files/2025-12/20251201-Corriger-les-principales-distorsions-de-l-imposition-du-patrimoine.pdf"
	urlVoiesMoyens2025   = "https://www2.assemblee-nationale.fr/static/17/Annexes-DL/PLF-2025/Voies_et_moyens_Tome_2_2025.pdf"
	urlDGFiPEcartTVA     = "https://www.impots.gouv.fr/sites/default/files/media/9_statistiques/0_etudes_et_stats/0_publications/dgfip_analyses/2024/num07_09/dgfip_analyses_07_2024.pdf"
	urlDreesUrgences2023 = "https://drees.solidarites-sante.gouv.fr/publications-communique-de-presse/etudes-et-resultats/250319_ER_urgences-la-moitie-des-patients-y-restent-plus-de-trois-heures-en-2023"
	urlANMaintienOrdre   = "https://www.assemblee-nationale.fr/dyn/15/rapports/ceordre/l15b3786_rapport-enquete.pdf"
)

// law : un texte du Journal officiel chargé, relu par son intitulé.
func law(id, dossier, section, date, author, title, finding, jo string, expected ...string) Fact {
	return Fact{ID: id, Dossier: dossier, Section: section, Theme: "textes", Type: "TEXTE", Date: date, Author: author,
		Title: title, Finding: finding, Quality: "OFFICIEL", JO: jo, Expected: expected}
}

// lawArticle : un article du Journal officiel chargé, relu dans son contenu.
func lawArticle(id, dossier, section, date, author, title, finding, jo, article string, expected ...string) Fact {
	f := law(id, dossier, section, date, author, title, finding, jo, expected...)
	f.JOArticle = article
	return f
}

func init() {
	terms = append(terms,
		Term{"budget-donnees", "loi de finances", `lois? de finances`},
		Term{"budget-donnees", "dette sociale, Cades", `dette sociale|\mcades\M`},
		Term{"dette-donnees", "dette publique", `dette publique`},
		Term{"dette-donnees", "charge de la dette", `charge de la dette|charge d.intérêts`},
		Term{"chomage-donnees", "assurance chômage", `assurance chômage`},
		Term{"chomage-donnees", "RSA", `\mrsa\M|revenu de solidarité active`},
		Term{"retraite-donnees", "retraites", `retraites?`},
		Term{"retraite-donnees", "âge de départ", `âge (légal )?de départ`},
		Term{"pauvrete-donnees", "pauvreté", `pauvreté`},
		Term{"pauvrete-donnees", "aide alimentaire", `aide alimentaire`},
		Term{"education-donnees", "Éducation nationale", `éducation nationale`},
		Term{"education-donnees", "AESH", `\maesh\M`},
		Term{"sante-donnees", "hôpital", `hôpital|hôpitaux`},
		Term{"sante-donnees", "déserts médicaux", `déserts? médica`},
		Term{"securite-police-donnees", "police nationale", `police nationale`},
		Term{"securite-police-donnees", "LOPMI", `\mlopmi\M`},
		Term{"defense-donnees", "programmation militaire", `programmation militaire|\mlpm\M`},
		Term{"pouvoirs-publics-donnees", "Pouvoirs publics (dotations)", `dotations? des assemblées|mission « pouvoirs publics »`},
		Term{"immigration-donnees", "immigration", `immigration`},
		Term{"immigration-donnees", "titres de séjour", `titres? de séjour`},
		Term{"violences-policieres-donnees", "violences policières", `violences policières`},
		Term{"ecologie-donnees", "transition écologique", `transition écologique`},
		Term{"ecologie-donnees", "budget vert", `budget vert`},
		Term{"bassins-versants-donnees", "GEMAPI", `\mgemapi\M`},
		Term{"bassins-versants-donnees", "agences de l'eau", `agences? de l.eau`},
		Term{"tva-donnees", "TVA", `\mtva\M`},
		Term{"tva-donnees", "taux de TVA", `taux (normal |réduits? |intermédiaire )?de (la )?tva`},
		Term{"sci-holding-donnees", "SCI", `\bsci\b`},
		Term{"sci-holding-donnees", "holding, pacte Dutreil", `holdings?|pacte dutreil`},
		Term{"sci-holding-donnees", "IFI", `\mifi\M|fortune immobilière`},
		Term{"depenses-fiscales-donnees", "dépenses fiscales, niches fiscales", `dépenses? fiscales?|niches? fiscales?`},
		Term{"fraude-fiscale-donnees", "fraude fiscale", `fraude fiscale`},
		Term{"cotisations-et-droits", "cotisations sociales", `cotisations sociales`},
		Term{"cotisations-et-droits", "exonérations de cotisations", `exonérations? de cotisations`},
		Term{"securite-sociale-donnees", "sécurité sociale", `sécurité sociale`},
		Term{"evasion-fiscale-multinationales", "évasion fiscale", `évasion fiscale`},
		Term{"evasion-fiscale-multinationales", "taxe sur les services numériques, GAFAM", `taxe sur les services numériques|\mgafam\M`},
		Term{"appareil-productif-donnees", "désindustrialisation", `désindustrialisation`},
		Term{"appareil-productif-donnees", "délocalisation", `délocalisations?|relocalisations?`},
	)

	facts = append(facts,
		// Budget de l'État et de la Sécurité sociale.
		law("lolf-2001", "budget-donnees", "CADRE", "2001-08-01", "Parlement (loi organique n° 2001-692)",
			"La loi organique relative aux lois de finances (LOLF)",
			"Texte qui fixe le cadre des lois de finances de l'État.",
			"JORFTEXT000000394028", "relative aux lois de finances"),
		lawArticle("lo-gestion-finances-publiques-2021", "budget-donnees", "CADRE", "2021-12-28", "Parlement (loi organique n° 2021-1836)",
			"La modernisation de la gestion des finances publiques",
			"Révision de la loi organique relative aux lois de finances, qui touche notamment au Haut Conseil des finances publiques (article 30).",
			"JORFTEXT000044589827", "30", "Haut Conseil des finances publiques"),
		Fact{ID: "hcfp-avis-2025-5", Dossier: "budget-donnees", Section: "CONTROLE", Theme: "avis", Type: "EVALUATION",
			Date: "2025-10-09", Author: "Haut Conseil des finances publiques (avis n° HCFP-2025-5)",
			Title: "Budget 2026 : un scénario jugé optimiste et un solde « fragilisé »",
			Finding: "Le Haut Conseil juge optimistes les hypothèses économiques du projet de budget 2026 et estime la prévision de solde " +
				"public fragilisée par le risque que les mesures de recettes et d'économies ne soient pas réalisées.",
			URL: urlHCFP2025, Quality: "OFFICIEL",
			Expected: []string{"repose sur des hypothèses optimistes", "la prévision de solde public pour 2026 soumise au Haut Conseil est fragilisée"}},
		Fact{ID: "senat-plfss-2026-cades", Dossier: "budget-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-12-10", Author: "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			Title: "La dette sociale : un amortissement imposé d'ici 2033, des déficits qui s'accumulent à l'Acoss",
			Finding: "La commission rappelle que la dette sociale doit être amortie d'ici le 31 décembre 2033 et recommande un transfert " +
				"de la dette de l'Acoss, financée à court terme, vers la Cades.",
			URL: urlSenatPLFSS, Quality: "OFFICIEL",
			Expected: []string{"impose un amortissement de la dette sociale d'ici le 31 décembre 2033", "TRANSFERT DE LA DETTE DE L'ACOSS VERS LA CADES"}},

		// Dette publique.
		law("lpfp-2023-2027", "dette-donnees", "CADRE", "2023-12-18", "Parlement (loi n° 2023-1195)",
			"La loi de programmation des finances publiques 2023-2027",
			"Trajectoire pluriannuelle de solde et de dette publics à laquelle le Haut Conseil des finances publiques compare chaque budget.",
			"JORFTEXT000048581885", "de programmation des finances publiques pour les années 2023 à 2027"),
		Fact{ID: "senat-deficit-excessif-2024", Dossier: "dette-donnees", Section: "CADRE", Theme: "règles européennes", Type: "CONSTAT",
			Date: "2024-07-26", Author: "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			Title: "La France sous procédure de déficit excessif depuis juillet 2024",
			Finding: "La France est à nouveau sous procédure européenne de déficit excessif depuis juillet 2024 et s'est engagée à ramener " +
				"son déficit public sous 3 points de PIB en 2029.",
			URL: urlSenatPLFSS, Quality: "OFFICIEL",
			Expected: []string{"La France est à nouveau sous procédure de déficit excessif depuis juillet 2024", "sous 3 points de PIB en 2029"}},
		Fact{ID: "hcfp-dette-2026", Dossier: "dette-donnees", Section: "CONTROLE", Theme: "avis", Type: "EVALUATION",
			Date: "2025-10-09", Author: "Haut Conseil des finances publiques (avis n° HCFP-2025-5)",
			Title:  "Une dette à près de 118 points de PIB en 2026, une charge d'intérêts de 74 Md€",
			Amount: "74000000000", Nature: "DEPENSE",
			Finding: "Selon le Haut Conseil, la dette publique passerait de plus de 113 points de PIB en 2024 à près de 118 en 2026, et " +
				"la charge d'intérêts atteindrait 74 Md€, en hausse de plus de 13 Md€ en deux ans (prévision).",
			URL: urlHCFP2025, Quality: "OFFICIEL",
			Expected: []string{"passant de plus de 113 points de PIB en 2024 à près de 118 points en 2026", "pour atteindre 74 Md€"}},

		// Chômage.
		law("loi-rsa-2008", "chomage-donnees", "CADRE", "2008-12-01", "Parlement (loi n° 2008-1249)",
			"La généralisation du revenu de solidarité active (RSA)",
			"Loi qui généralise le revenu de solidarité active.",
			"JORFTEXT000019860428", "généralisant le revenu de solidarité active"),
		law("loi-marche-travail-2022", "chomage-donnees", "CADRE", "2022-12-21", "Parlement (loi n° 2022-1598)",
			"Les mesures d'urgence sur le marché du travail",
			"Loi de 2022 sur le fonctionnement du marché du travail, adoptée « en vue du plein emploi ».",
			"JORFTEXT000046771781", "fonctionnement du marché du travail en vue du plein emploi"),
		lawArticle("loi-plein-emploi-2023", "chomage-donnees", "CADRE", "2023-12-18", "Parlement (loi n° 2023-1196)",
			"La loi pour le plein emploi (France Travail)",
			"L'article 1er réécrit l'inscription sur la liste des demandeurs d'emploi, désormais tenue par l'opérateur France Travail.",
			"JORFTEXT000048581935", "1", "Est inscrite sur la liste des demandeurs d'emploi auprès de l'opérateur France Travail"),
		Fact{ID: "unedic-previsions-2026-03", Dossier: "chomage-donnees", Section: "CONTROLE", Theme: "prévisions du gestionnaire", Type: "EVALUATION",
			Date: "2026-03-03", Author: "Unédic (gestionnaire de l'Assurance chômage), prévisions financières",
			Title:  "Assurance chômage : 38 Md€ d'indemnisation et 61,5 Md€ de dette prévus en 2026",
			Amount: "38000000000", Nature: "DEPENSE",
			Finding: "Selon ses prévisions de mars 2026, les dépenses d'indemnisation atteindraient 38 Md€ en 2026 (37,2 Md€ en 2025) et " +
				"la dette du régime 61,5 Md€ (59,4 Md€ en 2025).",
			URL: urlUnedic2026, Quality: "OFFICIEL",
			Expected: []string{"38 Md€ en 2026 après 37,2 Md€ en 2025", "à 61,5 Md€, contre 59,4 Md€ en 2025"}},

		// Retraite.
		lawArticle("lfrss-2023-retraites", "retraite-donnees", "CADRE", "2023-04-14", "Parlement (loi n° 2023-270)",
			"La loi de financement rectificative de la sécurité sociale pour 2023",
			"Son article 10 porte l'âge d'ouverture des droits à la retraite à soixante-quatre ans, progressivement selon la génération.",
			"JORFTEXT000047445077", "10", "soixante-quatre ans"),
		Fact{ID: "cor-rapport-2025-depenses", Dossier: "retraite-donnees", Section: "CONTROLE", Theme: "rapports d'évaluation", Type: "EVALUATION",
			Date: "2025-06-12", Author: "Conseil d'orientation des retraites (rapport annuel 2025)",
			Title:  "407 Md€ de dépenses de retraite en 2024, 13,9 % du PIB ; un déficit de 1,7 Md€",
			Amount: "407000000000", Nature: "DEPENSE",
			Finding: "Le COR évalue les dépenses de retraite à 407 Md€ en 2024 (13,9 % du PIB, 24,4 % des dépenses publiques) et le solde " +
				"du système à −1,7 Md€, hors produits et charges financiers.",
			URL: urlCOR2025, Quality: "OFFICIEL",
			Expected: []string{"les dépenses de retraite représentent 407 milliards d'euros, soit 13,9 % du PIB", "déficitaire de 1,7 milliard d'euros"}},

		// Pauvreté.
		law("loi-rsa-2008-pauvrete", "pauvrete-donnees", "CADRE", "2008-12-01", "Parlement (loi n° 2008-1249)",
			"Le revenu de solidarité active et les politiques d'insertion",
			"Loi qui généralise le RSA et réforme les politiques d'insertion.",
			"JORFTEXT000019860428", "réformant les politiques d'insertion"),
		Fact{ID: "senat-prime-activite-2026", Dossier: "pauvrete-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-11-20", Author: "Sénat, commission des affaires sociales (avis sur la mission « Solidarité, insertion et égalité des chances », PLF 2026)",
			Title: "Prime d'activité : 4,57 millions de foyers en 2025, un nombre stable pour la première fois",
			Finding: "La commission relève que le nombre de foyers bénéficiaires de la prime d'activité est resté stable en 2025 (4,57 " +
				"millions), et que la précarité alimentaire ne diminue pas malgré la stabilisation du financement des associations.",
			URL: urlSenatSolid, Quality: "OFFICIEL",
			Expected: []string{"4,57 millions de foyers bénéficiaires", "sans pour autant que la précarité alimentaire ne diminue en France"}},

		// Éducation.
		lawArticle("loi-ecole-confiance-2019", "education-donnees", "CADRE", "2019-07-26", "Parlement (loi n° 2019-791)",
			"La loi pour une école de la confiance",
			"Son article 11 rend l'instruction obligatoire dès trois ans et jusqu'à seize ans.",
			"JORFTEXT000038829065", "11", "L'instruction est obligatoire pour chaque enfant dès l'âge de trois ans et jusqu'à l'âge de seize ans"),
		law("loi-aesh-temps-meridien-2024", "education-donnees", "CADRE", "2024-05-27", "Parlement (loi n° 2024-475)",
			"L'accompagnement des élèves handicapés pendant la pause méridienne pris en charge par l'État",
			"L'État prend en charge l'accompagnement humain des élèves en situation de handicap durant le temps de pause méridienne.",
			"JORFTEXT000049602933", "accompagnement humain des élèves en situation de handicap"),
		Fact{ID: "senat-enseignement-scolaire-2026", Dossier: "education-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-11-20", Author: "Sénat, commission de la culture (avis sur la mission « Enseignement scolaire », PLF 2026)",
			Title:  "63,02 Md€ pour l'enseignement scolaire en 2026, après 12,13 Md€ de hausse depuis 2019",
			Amount: "63020000000", Nature: "DEPENSE",
			Finding: "Hors pensions, les crédits de paiement des cinq programmes du ministère s'élèvent à 63,02 Md€ pour 2026, un budget " +
				"stable après une hausse de 12,13 Md€ depuis 2019 (crédits votés, pas exécutés).",
			URL: urlSenatEduc, Quality: "OFFICIEL",
			Expected: []string{"s'élèvent à 63,02 milliards d'euros", "HAUSSE DE 12,13 MILLIARDS D'EUROS DEPUIS 2019"}},

		// Santé et Sécurité sociale.
		lawArticle("loi-systeme-sante-2019", "sante-donnees", "CADRE", "2019-07-24", "Parlement (loi n° 2019-774)",
			"La loi relative à l'organisation et à la transformation du système de santé",
			"Loi d'organisation du système de santé, dont l'article 41 crée la plateforme des données de santé.",
			"JORFTEXT000038821260", "41", "Plateforme des données de santé"),
		Fact{ID: "drees-enquete-urgences-2023-duree", Dossier: "sante-donnees", Section: "CONTROLE", Theme: "enquête urgences", Type: "CONSTAT",
			Date: "2025-03-19", Author: "Drees, Enquête Urgences 2023 (comparée à l'édition 2013)",
			Title: "Aux urgences, la moitié des patients attendent plus de 3 heures en 2023, 45 minutes de plus qu'en 2013",
			Finding: "Sur une journée moyenne de semaine dans près de 720 services d'urgence, la moitié des patients pris en charge y " +
				"passent plus de 3 heures en 2023 contre 2h15 en 2013, et 15 % restent plus de 8 heures contre 9 % en 2013 — deux " +
				"instantanés à dix ans d'écart, pas une série continue (l'enquête n'a lieu qu'une fois par décennie).",
			URL: urlDreesUrgences2023, Quality: "OFFICIEL",
			Expected: []string{"la moitié des personnes prises en charge aux urgences y passe plus de 3 heures, soit 45 minutes de plus qu'en 2013",
				"15 % des patients passent plus de 8 heures aux urgences, contre 9 % en 2013"}},
		law("lfss-2024", "securite-sociale-donnees", "CADRE", "2023-12-26", "Parlement (loi n° 2023-1250)",
			"La loi de financement de la sécurité sociale pour 2024",
			"Exemple de loi annuelle qui fixe les objectifs de dépenses et les prévisions de recettes des branches de la Sécurité sociale.",
			"JORFTEXT000048668665", "de financement de la sécurité sociale pour 2024"),
		Fact{ID: "senat-plfss-2026-deficit", Dossier: "securite-sociale-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-12-10", Author: "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			Title: "Budget de la Sécurité sociale 2026 : des mesures de réduction du déficit ramenées à 9 Md€",
			Finding: "Selon la commission, les mesures de réduction du déficit, de 15 Md€ dans le texte initial, n'étaient plus que de " +
				"9 Md€ dans le texte adopté.",
			URL: urlSenatPLFSS, Quality: "OFFICIEL",
			Expected: []string{"n'étaient plus que de 9 milliards d'euros dans le texte adopté"}},
		Fact{ID: "senat-plfss-2026-cades-cotisations", Dossier: "cotisations-et-droits", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-12-10", Author: "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			Title: "Les déficits sociaux s'accumulent à l'Acoss, qui ne peut s'endetter qu'à court terme",
			Finding: "La commission rappelle que l'Acoss, qui finance la Sécurité sociale, ne peut s'endetter qu'à court terme, et qu'à droit " +
				"inchangé les déficits cumulés s'y accumuleraient.",
			URL: urlSenatPLFSS, Quality: "OFFICIEL",
			Expected: []string{"que la loi n'autorise à s'endetter qu'à court terme sur les marchés"}},
		lawArticle("lfrss-2023-cotisations", "cotisations-et-droits", "CADRE", "2023-04-14", "Parlement (loi n° 2023-270)",
			"La réforme des retraites de 2023",
			"Son article 10 porte l'âge d'ouverture des droits à la retraite à soixante-quatre ans : les cotisations versées ouvrent le droit plus tard.",
			"JORFTEXT000047445077", "10", "soixante-quatre ans"),

		// Sécurité, défense, pouvoirs publics, immigration.
		law("lopmi-2023", "securite-police-donnees", "CADRE", "2023-01-24", "Parlement (loi n° 2023-22)",
			"La loi d'orientation et de programmation du ministère de l'Intérieur (LOPMI)",
			"Loi de programmation du ministère de l'Intérieur, à laquelle le Sénat compare chaque budget de la mission « Sécurités ».",
			"JORFTEXT000047046768", "d'orientation et de programmation du ministère de l'intérieur"),
		Fact{ID: "senat-lopmi-postes-2026", Dossier: "securite-police-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-11-20", Author: "Sénat, commission des lois (avis sur la mission « Sécurités », PLF 2026)",
			Title: "L'objectif de 7 412 créations de postes de la LOPMI « de plus en plus compromis »",
			Finding: "Le rapporteur juge l'objectif de 7 412 créations de postes d'ici 2027 de plus en plus compromis, malgré 1 000 postes " +
				"prévus pour la police et 400 pour la gendarmerie en 2026.",
			URL: urlSenatSecu, Quality: "OFFICIEL",
			Expected: []string{"L'objectif de la Lopmi de 7 412 créations de postes à horizon 2027"}},
		law("lpm-2024-2030", "defense-donnees", "CADRE", "2023-08-01", "Parlement (loi n° 2023-703)",
			"La loi de programmation militaire 2024-2030",
			"Programmation des crédits et des effectifs des armées de 2024 à 2030.",
			"JORFTEXT000047914986", "relative à la programmation militaire pour les années 2024 à 2030"),
		Fact{ID: "senat-defense-2026", Dossier: "defense-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-11-20", Author: "Sénat, commission des finances (rapport spécial sur la mission « Défense », PLF 2026)",
			Person: "dominique-de-legge",
			Title:  "57,15 Md€ sur le périmètre de la LPM en 2026, et une actualisation de la loi jugée indispensable",
			Amount: "57150000000", Nature: "DEPENSE",
			Finding: "Sur le périmètre de la LPM (hors pensions), les crédits demandés pour 2026 atteignent 57,15 Md€, en hausse de 6,67 Md€ ; " +
				"le rapporteur spécial juge indispensable une actualisation de la loi de programmation militaire (crédits votés).",
			URL: urlSenatDefense, Quality: "OFFICIEL",
			Expected: []string{"les crédits demandés s'établissent à 57,15 milliards d'euros", "la présentation d'une actualisation de la LPM apparaît désormais indispensable"}},
		law("ordonnance-assemblees-1958", "pouvoirs-publics-donnees", "CADRE", "1958-11-17", "Gouvernement (ordonnance n° 58-1100)",
			"Le fonctionnement des assemblées parlementaires",
			"Ordonnance qui organise le fonctionnement des assemblées parlementaires, dont leurs règles budgétaires propres.",
			"JORFTEXT000000705067", "relative au fonctionnement des assemblées parlementaires"),
		Fact{ID: "senat-pouvoirs-publics-2026", Dossier: "pouvoirs-publics-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-11-20", Author: "Sénat, commission des finances (rapport spécial sur la mission « Pouvoirs publics », PLF 2026)",
			Person: "gregory-blanc",
			Title:  "Des dotations en hausse de 12 % en euros courants de 2011 à 2025",
			Finding: "Le rapporteur spécial relève que la dotation cumulée de la mission a progressé de 12 % entre 2011 et 2025 en euros " +
				"courants, et alerte sur les effets du gel prolongé des dotations sur les réserves des institutions.",
			URL: urlSenatPouvoirs, Quality: "OFFICIEL",
			Expected: []string{"La dotation cumulée de la mission a progressé de 12 % entre 2011 et 2025 en euros courants"}},
		law("loi-immigration-2024", "immigration-donnees", "CADRE", "2024-01-26", "Parlement (loi n° 2024-42)",
			"La loi pour contrôler l'immigration, améliorer l'intégration",
			"Dernière loi d'ensemble sur l'entrée, le séjour et l'éloignement des étrangers chargée dans le corpus.",
			"JORFTEXT000049040245", "pour contrôler l'immigration, améliorer l'intégration"),
		Fact{ID: "senat-immigration-cout-2026", Dossier: "immigration-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-11-20", Author: "Sénat, commission des finances (rapport spécial sur la mission « Immigration, asile et intégration », PLF 2026)",
			Title:  "7,82 Md€ : le coût estimé de la politique de l'immigration et de l'intégration en 2026",
			Amount: "7820000000", Nature: "ESTIME",
			Finding: "Le coût estimé de la politique française de l'immigration et de l'intégration, toutes missions confondues, est de " +
				"7,82 Md€ en 2026, contre 7,74 Md€ en 2025 (document de politique transversale, cité par le rapport).",
			URL: urlSenatImmig, Quality: "OFFICIEL",
			Expected: []string{"Le coût estimé de la politique française de l'immigration et de l'intégration est de 7,82 milliards d'euros en 2026"}},
		lawArticle("lo-defenseur-droits-2011", "violences-policieres-donnees", "CADRE", "2011-03-29", "Parlement (loi organique n° 2011-333)",
			"Le Défenseur des droits, compétent pour la déontologie des forces de sécurité",
			"Son article 4 charge le Défenseur des droits de veiller au respect de la déontologie par les personnes exerçant des activités de sécurité.",
			"JORFTEXT000023781167", "4", "déontologie"),
		Fact{ID: "ddd-deontologie-2024", Dossier: "violences-policieres-donnees", Section: "CONTROLE", Theme: "autorités indépendantes", Type: "EVALUATION",
			Date: "2025-03-05", Author: "Défenseur des droits (rapport annuel d'activité 2024)",
			Title: "2 434 réclamations sur la déontologie de la sécurité en 2024",
			Finding: "Le Défenseur des droits a reçu 2 434 réclamations en matière de déontologie de la sécurité en 2024. Une réclamation " +
				"n'est pas un manquement établi.",
			URL: urlDDD2024, Quality: "OFFICIEL",
			Expected: []string{"déontologie de la sécurité reçues par le Défenseur des droits en 2024 (N = 2 434)"}},
		Fact{ID: "flagrant-deni-elucidation-2025", Dossier: "violences-policieres-donnees", Section: "CONTROLE", Theme: "associations", Type: "EVALUATION",
			Date: "2025-11-17", Author: "Flagrant déni, Polices des polices : pourquoi il faut tout changer",
			Title: "Le taux d'élucidation des affaires de violences par PDAP a baissé de 25 points entre 2016 et 2024",
			Finding: "L'association Flagrant déni, à partir de données officielles inédites obtenues de la Chancellerie, constate que le taux d'affaires de " +
				"violences par personne dépositaire de l'autorité publique élucidées (au moins un auteur retrouvé) a baissé de 25 points entre 2016 et 2024, " +
				"et chiffre à 700 en 2016 puis 1 110 en 2024 le nombre de ces affaires — une hausse de l'ordre de 60 %.",
			URL:     "https://www.flagrant-deni.fr/wp-content/uploads/2025/11/FD-RAPPORT-numerique-vf.pdf",
			Quality: "DECLARATIF",
			Expected: []string{"le taux d'élucidation des affaires de violences policières a baissé de 25 % entre 2016 et 2024",
				"alors qu'il était de 700 en 2016, ce nombre d'affaires est de 1110 en 2024, soit une augmentation de l'ordre de 60%"}},
		Fact{ID: "an-maintien-ordre-2021-absence-stats", Dossier: "violences-policieres-donnees", Section: "CONTROLE",
			Theme: "Assemblée nationale", Type: "CONSTAT",
			Date: "2021-01-20", Author: "Commission d'enquête de l'Assemblée nationale sur le maintien de l'ordre",
			Title: "L'Assemblée nationale constate elle-même l'absence de statistiques sur les blessés parmi les manifestants",
			Finding: "La commission d'enquête, créée après les mobilisations des Gilets jaunes, écrit noir sur blanc qu'il n'existe pas de " +
				"statistiques précises sur les blessés parmi les manifestants — l'absence documentée dans cette note (§ 6) n'est donc pas " +
				"un simple constat d'un tiers, mais l'aveu de l'Assemblée nationale elle-même.",
			URL: urlANMaintienOrdre, Quality: "OFFICIEL",
			Expected: []string{"il n'existe pas de statistiques précises sur les blessés parmi les manifestants"}},
		Fact{ID: "an-maintien-ordre-2021-signalements-igpn", Dossier: "violences-policieres-donnees", Section: "CONTROLE",
			Theme: "Assemblée nationale", Type: "CONSTAT",
			Date: "2021-01-20", Author: "Commission d'enquête de l'Assemblée nationale sur le maintien de l'ordre",
			Title: "406 dossiers judiciaires ouverts par l'IGPN sur les Gilets jaunes, quatre condamnations",
			Finding: "La directrice de l'IGPN, entendue par la commission, indique que son service a traité 406 dossiers judiciaires liés aux " +
				"Gilets jaunes depuis le 17 novembre 2018, dont 311 retournés à l'autorité judiciaire — pour des suites connues de quatre " +
				"condamnations, six poursuites, quatre mises en examen et 205 classements sans suite par les parquets. Parallèlement, " +
				"67 enquêtes administratives ont été ouvertes, dont huit ont retenu un usage disproportionné de la force visant 17 policiers.",
			URL: urlANMaintienOrdre, Quality: "OFFICIEL",
			Expected: []string{"406 dossiers judiciaires, dont 311 ont été retournés à l'autorité judiciaire",
				"Soixante-sept enquêtes administratives ont été ouvertes par l'IGPN"}},

		// Écologie et eau.
		lawArticle("loi-energie-climat-2019", "ecologie-donnees", "CADRE", "2019-11-08", "Parlement (loi n° 2019-1147)",
			"La loi relative à l'énergie et au climat",
			"Son article 1er inscrit dans le code de l'énergie un objectif de neutralité carbone.",
			"JORFTEXT000039355955", "1", "neutralité carbone"),
		lawArticle("loi-climat-resilience-2021", "ecologie-donnees", "CADRE", "2021-08-22", "Parlement (loi n° 2021-1104)",
			"La loi climat et résilience",
			"Loi de lutte contre le dérèglement climatique, dont l'article 191 fixe l'objectif d'absence d'artificialisation nette des sols.",
			"JORFTEXT000043956924", "191", "artificialisation nette"),
		Fact{ID: "hcc-rapport-2025-emissions", Dossier: "ecologie-donnees", Section: "CONTROLE", Theme: "autorités indépendantes", Type: "EVALUATION",
			Date: "2025-07-03", Author: "Haut Conseil pour le climat (rapport annuel 2025)",
			Title: "Émissions brutes de 2024 inférieures de 32 % à 1990 ; deuxième budget carbone respecté",
			Finding: "Le Haut Conseil constate des émissions brutes 2024 inférieures de 32 % à leur niveau de 1990 et le respect du deuxième " +
				"budget carbone (406 Mt éqCO2 par an en moyenne de 2019 à 2023 pour un plafond de 425).",
			URL: urlHCC2025, Quality: "OFFICIEL",
			Expected: []string{"Le niveau atteint en 2024 pour les émissions brutes est inférieur de 32 % au niveau de 1990", "respectent le deuxième budget carbone"}},
		lawArticle("loi-maptam-gemapi-2014", "bassins-versants-donnees", "CADRE", "2014-01-27", "Parlement (loi n° 2014-58)",
			"La loi MAPTAM, qui crée la compétence GEMAPI",
			"Son article 56 organise la compétence de gestion des milieux aquatiques et de prévention des inondations (GEMAPI).",
			"JORFTEXT000028526298", "56", "gestion des milieux aquatiques et de prévention des inondations"),
		Fact{ID: "senat-gestion-eau-2023", Dossier: "bassins-versants-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "RECOMMANDATION",
			Date: "2023-07-11", Author: "Sénat, mission d'information sur la gestion durable de l'eau (rapporteur)", Person: "herve-gille",
			Title: "53 propositions, dont une taxe GEMAPI mutualisée à l'échelle du bassin versant",
			Finding: "La mission propose notamment de renforcer la gouvernance par bassin et de mutualiser une fraction de la taxe GEMAPI " +
				"sur l'ensemble du bassin versant pour les intercommunalités aux ressources faibles.",
			URL: urlSenatEau, Quality: "OFFICIEL",
			Expected: []string{"Mettre en place une fraction de taxe GEMAPI mutualisée sur l'ensemble du bassin versant", "Renforcer la gouvernance de l'eau"}},

		// TVA.
		lawArticle("loi-lfr-2012-taux-tva-2014", "tva-donnees", "CADRE", "2012-12-29", "Parlement (loi n° 2012-1510)",
			"La loi de finances rectificative pour 2012, qui fixe les taux actuels de 20 % et 10 %",
			"Son article 68 relève le taux normal de 19,60 % à 20 % et le taux intermédiaire de 7 % à 10 %, "+
				"pour les opérations dont le fait générateur intervient à compter du 1er janvier 2014.",
			"JORFTEXT000026857857", "68", "A la fin de l'article 278, le taux : « 19,60 % » est remplacé par le taux : « 20 % »",
			"le taux : « 7 % » est remplacé par le taux : « 10 % »"),
		Fact{ID: "cour-comptes-tva-part-recettes-2024", Dossier: "tva-donnees", Section: "ENJEUX", Type: "EVALUATION",
			Date: "2025-04-15", Author: "Cour des comptes, analyse de l'exécution budgétaire 2024 — Recettes fiscales de l'État",
			Title: "La TVA ne représente plus que 30 % des recettes fiscales nettes de l'État, contre 53 % en 2018",
			Finding: "La Cour constate que la TVA, \"principal impôt de rendement corrélé à la croissance économique\", fait l'objet depuis 2019 de " +
				"transferts croissants aux collectivités territoriales et à la Sécurité sociale, si bien qu'elle ne représente plus que 30 % des " +
				"recettes fiscales nettes de l'État en 2024, contre 53 % en 2018.",
			URL:     "https://www.ccomptes.fr/sites/default/files/2025-04/NEB-2024-Recettes-fiscales.pdf",
			Quality: "OFFICIEL",
			Expected: []string{"la TVA, principal impôt de rendement corrélé à la croissance",
				"la TVA ne représente plus que 30 % des recettes fiscales nettes en 2024, contre 53 % en 2018"}},

		// SCI et holdings.
		lawArticle("loi-pme-2005-dutreil-75", "sci-holding-donnees", "CADRE", "2005-08-02", "Parlement (loi n° 2005-882)",
			"La loi PME de 2005, qui porte l'exonération du pacte Dutreil à 75 %",
			"Son article 28 porte l'exonération de droits de mutation à titre gratuit du pacte Dutreil de la moitié à 75 % de la valeur des titres transmis.",
			"JORFTEXT000000452052", "28", "à concurrence de 75 % de leur valeur"),
		lawArticle("loi-2018-creation-ifi", "sci-holding-donnees", "CADRE", "2017-12-30", "Parlement (loi n° 2017-1837)",
			"La loi de finances pour 2018, qui crée l'impôt sur la fortune immobilière (IFI)",
			"Son article 31 institue l'IFI (seuil d'assujettissement de 1 300 000 €) et exonère les biens immobiliers affectés à l'activité professionnelle réelle du redevable, y compris via une société à l'IS sous conditions de fonction et de détention.",
			"JORFTEXT000036339197", "31", "Il est institué un impôt annuel sur les actifs immobiliers désigné sous le nom d'impôt sur la fortune immobilière"),
		Fact{ID: "cpo-holding-taux-effectif-2025", Dossier: "sci-holding-donnees", Section: "ENJEUX", Type: "EVALUATION",
			Date: "2025-12-01", Author: "Conseil des prélèvements obligatoires (Cour des comptes), Corriger les principales distorsions de l'imposition du patrimoine",
			Title: "Remonter des dividendes d'une filiale vers une holding est imposé à 1,25 % au maximum, pas 0 %",
			Finding: "Le CPO chiffre le régime mère-fille : la quote-part pour frais et charges (5 % du dividende, taxée à l'IS) fait que le transfert " +
				"de la filiale vers la holding est imposé à un taux effectif de 1,25 % au maximum — 0,25 % seulement en cas d'intégration fiscale.",
			URL:     urlCPOPatrimoine,
			Quality: "OFFICIEL",
			Expected: []string{"le transfert de la fille vers la holding est ainsi imposé à un taux effectif de 1,25 % au maximum",
				"ce taux effectif est plus faible, à 0,25 %"}},

		// Dépenses fiscales.
		lawArticle("lolf-2021-reforme-depenses-fiscales", "depenses-fiscales-donnees", "CADRE", "2021-12-28", "Parlement (loi organique n° 2021-1836)",
			"La réforme de la LOLF de 2021, qui renforce l'obligation de chiffrer les dépenses fiscales",
			"Son article 25 impose que l'annexe budgétaire sur les dépenses fiscales comporte l'évaluation de leur montant et le nombre de bénéficiaires, la liste de celles évaluées dans l'année, et l'écart entre exécution et prévision par mission.",
			"JORFTEXT000044589827", "25", "L'évaluation de leur montant et le nombre de bénéficiaires"),
		Fact{ID: "plf2025-depenses-fiscales-limites-chiffrage", Dossier: "depenses-fiscales-donnees", Section: "ENJEUX", Type: "TEXTE",
			Date: "2024-10-01", Author: "Direction du budget, Évaluation des voies et moyens tome II — Les dépenses fiscales, annexe au PLF 2025",
			Title: "Le chiffrage officiel n'intègre ni les effets comportementaux ni les interactions entre dispositifs",
			Finding: "L'administration précise elle-même que ce chiffrage n'intègre pas les effets secondaires d'une dépense fiscale qu'il est " +
				"impossible de prendre en compte, et que les interactions entre les mesures ne peuvent pas être quantifiées.",
			URL:     urlVoiesMoyens2025,
			Quality: "OFFICIEL",
			Expected: []string{"ce chiffrage n'intègre pas les effets secondaires d'une dépense fiscale qu'il est impossible de prendre en compte",
				"les interactions entre les mesures ne peuvent pas être quantifiées"}},

		// Fraude fiscale.
		lawArticle("loi-2018-fraude-publication-noms", "fraude-fiscale-donnees", "CADRE", "2018-10-23", "Parlement (loi n° 2018-898)",
			"La loi de 2018 contre la fraude, qui permet de publier le nom des fraudeurs les plus graves",
			"Son article 18 crée dans le CGI la possibilité de publier, pour les manquements les plus graves (au moins 50 000 € de droits fraudés avec manœuvre frauduleuse), la nature et le montant des droits fraudés ainsi que l'identité du contribuable.",
			"JORFTEXT000037518803", "18", "un minimum de 50 000 € et le recours à une manœuvre frauduleuse"),
		Fact{ID: "dgfip-ecart-tva-2024", Dossier: "fraude-fiscale-donnees", Section: "ENJEUX", Type: "EVALUATION",
			Date: "2024-09-01", Author: "DGFiP Analyses n°7, Le manque à gagner de TVA en France",
			Title: "L'écart de TVA est estimé entre 6 et 10 Md€, soit 4 à 5 % de la TVA collectée",
			Finding: "La DGFiP chiffre le manque à gagner de TVA dû à la sous-déclaration des entreprises dans une fourchette de 6 à 10 milliards " +
				"d'euros, soit 4 à 5 % du montant de TVA effectivement collecté — une méthode validée par une expérience de contrôles aléatoires.",
			URL:     urlDGFiPEcartTVA,
			Quality: "OFFICIEL",
			Expected: []string{"compris dans une fourchette de 6 à 10", "soit 4-5% du montant de TVA effectivement collecté",
				"expérience de contrôles aléatoires"}},

		// Évasion fiscale : le cadre (les contrôles sont dans ref.fait_multinationale).
		lawArticle("loi-taxe-services-numeriques-2019", "evasion-fiscale-multinationales", "CADRE", "2019-07-24", "Parlement (loi n° 2019-759)",
			"La taxe sur les services numériques",
			"Son article 1er institue une taxe sur certains services fournis par les grandes entreprises du numérique, au taux de 3 %.",
			"JORFTEXT000038811588", "1", "Taxe sur certains services fournis par les grandes entreprises du secteur numérique", "un taux de 3 %"),
		Fact{ID: "lf-2024-imposition-minimale", Dossier: "evasion-fiscale-multinationales", Section: "CADRE", Theme: "textes", Type: "TEXTE",
			Date: "2023-12-29", Author: "Parlement (loi de finances pour 2024, article 33)",
			Title:   "L'imposition minimale mondiale des grands groupes (pilier 2)",
			Finding: "L'article 33 crée dans le code général des impôts l'imposition minimale mondiale des groupes, avec un taux de 15 % et un seuil de 750 millions d'euros de chiffre d'affaires.",
			Quality: "OFFICIEL", JO: "JORFTEXT000048727345", JOArticle: "33",
			Expected: []string{"Imposition minimale mondiale des groupes d'entrep", "15 %", "750 millions"}},
		Fact{ID: "loi-sapin-2-cjip", Dossier: "evasion-fiscale-multinationales", Section: "CADRE", Theme: "textes", Type: "TEXTE",
			Date: "2016-12-09", Author: "Parlement (loi n° 2016-1691, dite Sapin 2, article 22)",
			Title: "La convention judiciaire d'intérêt public",
			Finding: "Avant toute poursuite, le procureur peut proposer à une personne morale mise en cause une convention judiciaire " +
				"d'intérêt public, notamment pour le blanchiment de fraude fiscale ; c'est la forme des règlements de Google (2019) et " +
				"de McDonald's (2022).",
			Quality: "OFFICIEL", JO: "JORFTEXT000033558528", JOArticle: "22",
			Expected: []string{"le procureur de la République peut proposer à une personne morale mise en cause", "blanchiment des infractions prévues aux articles 1741 et 1743 du code général des impôts"}},
	)
	// Enjeux : ce que les institutions de contrôle disent de l'importance du sujet.
	facts = append(facts,
		issue("enjeu-budget-dette-hcfp", "budget-donnees", "2025-10-09", "Haut Conseil des finances publiques (avis n° HCFP-2025-5)",
			"Une dette publique qui progresse « à un rythme préoccupant »",
			"Le Haut Conseil qualifie de préoccupant le rythme de progression de la dette publique prévu par le projet de budget 2026.",
			urlHCFP2025, "OFFICIEL", "La dette publique continuerait de ce fait de progresser à un rythme préoccupant"),
		issue("enjeu-dette-deficit-zone-euro", "dette-donnees", "2025-12-10", "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			"Le déficit rapporté au PIB le plus élevé de la zone euro, selon la Commission européenne",
			"Le rapport cite un déficit public estimé par le Gouvernement à 5,4 points de PIB en 2025 (5,8 en 2024), qui serait selon la "+
				"Commission européenne le plus élevé de la zone euro.",
			urlSenatPLFSS, "OFFICIEL", "il s'agirait du déficit rapporté au PIB le plus élevé de la zone euro", "5,4 points de PIB (après 5,8 points de PIB en 2024)"),
		issue("enjeu-secu-deficit-zone-euro", "securite-sociale-donnees", "2025-12-10", "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			"Des finances publiques « très dégradées »",
			"La commission ouvre son rapport sur le budget de la Sécurité sociale 2026 par la situation des finances publiques, qu'elle "+
				"qualifie de très dégradée.",
			urlSenatPLFSS, "OFFICIEL", "UNE SITUATION DES FINANCES PUBLIQUES TRÈS DÉGRADÉE"),
		issue("enjeu-cotisations-acoss-liquidite", "cotisations-et-droits", "2025-12-10", "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			"Le risque de liquidité d'une dette sociale financée à court terme",
			"La commission souligne le risque de liquidité inhérent à l'endettement de court terme de l'Acoss, qui finance la Sécurité sociale.",
			urlSenatPLFSS, "OFFICIEL", "risque de liquidité inhérent à ce type d'endettement"),
		issue("enjeu-retraite-cor-soutenabilite", "retraite-donnees", "2025-06-12", "Conseil d'orientation des retraites (rapport annuel 2025)",
			"Les dépenses de retraite rapportées au PIB, indicateur de soutenabilité",
			"Le COR retient la part des dépenses de retraite dans le PIB comme indicateur déterminant de la soutenabilité financière du système.",
			urlCOR2025, "OFFICIEL", "constituent un indicateur déterminant pour évaluer la soutenabilité financière du système"),
		issue("enjeu-chomage-desendettement", "chomage-donnees", "2026-03-03", "Unédic (gestionnaire de l'Assurance chômage), prévisions financières",
			"Le désendettement de l'Assurance chômage, un « défi » selon son gestionnaire",
			"Le gestionnaire du régime présente son désendettement comme un défi, dans des perspectives économiques qu'il juge moroses.",
			urlUnedic2026, "DECLARATIF", "l'Assurance chômage demeure confrontée au défi de son désendettement"),
		issue("enjeu-pauvrete-mission-solidarite", "pauvrete-donnees", "2025-11-20", "Sénat, commission des affaires sociales (avis sur la mission « Solidarité, insertion et égalité des chances », PLF 2026)",
			"Une mission budgétaire consacrée à la lutte contre la pauvreté",
			"La mission « Solidarité, insertion et égalité des chances » rassemble les crédits de l'État destinés à lutter contre la pauvreté et à protéger les personnes vulnérables.",
			urlSenatSolid, "OFFICIEL", "visant à lutter contre la pauvreté, à défendre et inclure les personnes vulnérables"),
		issue("enjeu-education-demographie", "education-donnees", "2025-11-20", "Sénat, commission de la culture (avis sur la mission « Enseignement scolaire », PLF 2026)",
			"La baisse du nombre d'élèves s'accélère",
			"Le rapport relève que la diminution du nombre de collégiens, limitée à 18 000 par an pendant deux rentrées, va s'accentuer très fortement.",
			urlSenatEduc, "OFFICIEL", "cette baisse va s'accentuer très fortement dès la prochaine rentrée"),
		issue("enjeu-sante-ondam-2026", "sante-donnees", "2025-12-10", "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			"267,5 Md€ de dépenses prévues pour la branche maladie en 2026",
			"L'objectif de dépenses de la branche maladie, maternité, invalidité et décès est fixé à 267,5 Md€ pour 2026, en hausse de 2 % sur l'exécution 2025 (objectif voté).",
			urlSenatPLFSS, "OFFICIEL", "est fixé à 267,5 milliards d'euros pour 2026, soit une hausse de 2 %"),
		issue("enjeu-police-infractions", "securite-police-donnees", "2025-11-20", "Sénat, commission des lois (avis sur la mission « Sécurités », PLF 2026)",
			"Des acteurs auditionnés qui décrivent plus d'infractions et plus de violence",
			"Selon le rapporteur, les acteurs auditionnés confirment une progression importante du nombre d'infractions et de leur niveau de violence (constat d'audition, pas une statistique).",
			urlSenatSecu, "OFFICIEL", "ont confirmé cette progression importante du nombre d'infractions et de leur niveau de violence"),
		issue("enjeu-defense-contexte", "defense-donnees", "2025-11-20", "Sénat, commission des finances (rapport spécial sur la mission « Défense », PLF 2026)",
			"Une programmation militaire dans un contexte « profondément déstabilisé »",
			"Le rapport situe la troisième annuité de la LPM dans un contexte géostratégique profondément déstabilisé par la guerre en Ukraine.",
			urlSenatDefense, "OFFICIEL", "dans un contexte géostratégique profondément déstabilisé par la guerre en Ukraine"),
		issue("enjeu-pouvoirs-publics-autonomie", "pouvoirs-publics-donnees", "2025-11-20", "Sénat, commission des finances (rapport spécial sur la mission « Pouvoirs publics », PLF 2026)",
			"L'autonomie financière des pouvoirs publics, corollaire de leur indépendance",
			"Le rapporteur spécial présente un niveau de réserves suffisant comme une condition de l'autonomie financière des pouvoirs publics, corollaire de leur indépendance institutionnelle.",
			urlSenatPouvoirs, "OFFICIEL", "une condition essentielle de l'autonomie financière des pouvoirs publics, corollaire de leur indépendance institutionnelle"),
		issue("enjeu-immigration-irreguliere", "immigration-donnees", "2025-11-20", "Sénat, commission des finances (rapport spécial sur la mission « Immigration, asile et intégration », PLF 2026)",
			"La hausse des crédits 2026 destinée surtout à la lutte contre l'immigration irrégulière",
			"Le programme « Immigration et asile » capte toute l'augmentation des crédits de la mission pour 2026, largement destinée à la lutte contre l'immigration irrégulière (crédits demandés).",
			urlSenatImmig, "OFFICIEL", "largement à destination de la lutte contre l'immigration irrégulière"),
		issue("enjeu-ddd-atteintes-droits", "violences-policieres-donnees", "2025-03-05", "Défenseur des droits (rapport annuel d'activité 2024)",
			"Des réclamations qui traduisent « de nombreuses atteintes aux droits et libertés »",
			"Le Défenseur des droits présente les réclamations reçues en 2024, toutes missions confondues, comme la traduction de nombreuses atteintes aux droits et libertés.",
			urlDDD2024, "OFFICIEL", "traduisent de nombreuses atteintes aux droits et libertés en France"),
		issue("enjeu-ecologie-retards", "ecologie-donnees", "2025-07-03", "Haut Conseil pour le climat (rapport annuel 2025)",
			"L'instabilité politique et les retards d'arbitrage pèsent sur les investissements",
			"Le Haut Conseil estime que l'instabilité et les retards d'arbitrage liés au contexte politique ont des répercussions tangibles sur les investissements dans la transition.",
			urlHCC2025, "OFFICIEL", "L'instabilité et les retards d'arbitrage du fait du contexte politique actuel ont des répercussions tangibles sur les investissements dans la transition"),
		issue("enjeu-eau-urgence", "bassins-versants-donnees", "2023-07-11", "Sénat, mission d'information sur la gestion durable de l'eau",
			"« L'urgence d'agir pour nos usages, nos territoires et notre environnement »",
			"Intitulé de la mission d'information du Sénat de 2023, qui place la gouvernance de l'eau en tête de ses propositions.",
			urlSenatEau, "OFFICIEL", "l'urgence d'agir pour nos usages, nos territoires et notre environnement"),
	)

}

// issue : ce qu'une institution dit de l'importance d'un sujet, relu dans le document.
func issue(id, dossier, date, author, title, finding, url, quality string, expected ...string) Fact {
	return Fact{ID: id, Dossier: dossier, Section: "ENJEUX", Type: "EVALUATION", Date: date, Author: author, Title: title,
		Finding: finding, URL: url, Quality: quality, Expected: expected}
}
