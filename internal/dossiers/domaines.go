package dossiers

// Les faits de contexte, de cadre et de contrôle des dossiers statistiques
// (budget, dette, emploi, retraite, pauvreté, éducation, santé, sécurité,
// défense, pouvoirs publics, immigration, écologie, eau, évasion fiscale). Les
// chiffres des dossiers restent dans leurs tables ; ces faits disent dans quel
// cadre ils s'inscrivent et ce qu'en ont conclu les institutions de contrôle.

const (
	urlHCFP2025      = "https://www.hcfp.fr/sites/default/files/2025-10/Avis%20HCFP%202025%20%E2%80%93%205%20PLF-PLFSS%202026_0.pdf"
	urlCOR2025       = "https://www.cor-retraites.fr/sites/default/files/2025-06/Synth%C3%A8se_Def_.pdf"
	urlHCC2025       = "https://www.hautconseilclimat.fr/wp-content/uploads/2025/07/HCC_RA_2025-VDEF0207_web.pdf"
	urlUnedic2026    = "https://www.unedic.org/storage/uploads/2026/03/04/situation-financiere-assurance-chomage-a-horizon-2028_03-mars-2026_uid_69a7f392cd38b.pdf"
	urlDDD2024       = "https://www.defenseurdesdroits.fr/sites/default/files/2025-03/ddd_rapport-annuel-2024_20250305.pdf"
	urlSenatDefense  = "https://www.senat.fr/rap/l25-139-38/l25-139-38-syn.pdf"
	urlSenatPouvoirs = "https://www.senat.fr/rap/l25-139-322/l25-139-322-syn.pdf"
	urlSenatEduc     = "https://www.senat.fr/rap/a25-144-31/a25-144-31-syn.pdf"
	urlSenatSecu     = "https://www.senat.fr/rap/a25-145-12/a25-145-12-syn.pdf"
	urlSenatImmig    = "https://www.senat.fr/rap/l25-139-315/l25-139-315_mono.html"
	urlSenatSolid    = "https://www.senat.fr/rap/a25-142-5/a25-142-5-syn.pdf"
	urlSenatPLFSS    = "https://www.senat.fr/lessentiel/plfss2026.pdf"
	urlSenatEau      = "https://www.senat.fr/fileadmin/Office_et_delegations/Annexe_-_Essentiel_-_Les_53_propositions.pdf"
)

// loi : un texte du Journal officiel chargé, relu par son intitulé.
func loi(id, dossier, section, date, auteur, intitule, constat, jo string, attendus ...string) Fait {
	return Fait{ID: id, Dossier: dossier, Section: section, Theme: "textes", Type: "TEXTE", Date: date, Auteur: auteur,
		Intitule: intitule, Constat: constat, Qualite: "OFFICIEL", JO: jo, Attendus: attendus}
}

// loiArt : un article du Journal officiel chargé, relu dans son contenu.
func loiArt(id, dossier, section, date, auteur, intitule, constat, jo, article string, attendus ...string) Fait {
	f := loi(id, dossier, section, date, auteur, intitule, constat, jo, attendus...)
	f.JOArticle = article
	return f
}

func init() {
	termes = append(termes,
		Terme{"budget-donnees", "loi de finances", `lois? de finances`},
		Terme{"budget-donnees", "dette sociale, Cades", `dette sociale|\mcades\M`},
		Terme{"dette-donnees", "dette publique", `dette publique`},
		Terme{"dette-donnees", "charge de la dette", `charge de la dette|charge d.intérêts`},
		Terme{"chomage-donnees", "assurance chômage", `assurance chômage`},
		Terme{"chomage-donnees", "RSA", `\mrsa\M|revenu de solidarité active`},
		Terme{"retraite-donnees", "retraites", `retraites?`},
		Terme{"retraite-donnees", "âge de départ", `âge (légal )?de départ`},
		Terme{"pauvrete-donnees", "pauvreté", `pauvreté`},
		Terme{"pauvrete-donnees", "aide alimentaire", `aide alimentaire`},
		Terme{"education-donnees", "Éducation nationale", `éducation nationale`},
		Terme{"education-donnees", "AESH", `\maesh\M`},
		Terme{"sante-donnees", "hôpital", `hôpital|hôpitaux`},
		Terme{"sante-donnees", "déserts médicaux", `déserts? médica`},
		Terme{"securite-police-donnees", "police nationale", `police nationale`},
		Terme{"securite-police-donnees", "LOPMI", `\mlopmi\M`},
		Terme{"defense-donnees", "programmation militaire", `programmation militaire|\mlpm\M`},
		Terme{"pouvoirs-publics-donnees", "Pouvoirs publics (dotations)", `dotations? des assemblées|mission « pouvoirs publics »`},
		Terme{"immigration-donnees", "immigration", `immigration`},
		Terme{"immigration-donnees", "titres de séjour", `titres? de séjour`},
		Terme{"violences-policieres-donnees", "violences policières", `violences policières`},
		Terme{"ecologie-donnees", "transition écologique", `transition écologique`},
		Terme{"ecologie-donnees", "budget vert", `budget vert`},
		Terme{"bassins-versants-donnees", "GEMAPI", `\mgemapi\M`},
		Terme{"bassins-versants-donnees", "agences de l'eau", `agences? de l.eau`},
		Terme{"tva-donnees", "TVA", `\mtva\M`},
		Terme{"tva-donnees", "taux de TVA", `taux (normal |réduits? |intermédiaire )?de (la )?tva`},
		Terme{"cotisations-et-droits", "cotisations sociales", `cotisations sociales`},
		Terme{"cotisations-et-droits", "exonérations de cotisations", `exonérations? de cotisations`},
		Terme{"securite-sociale-donnees", "sécurité sociale", `sécurité sociale`},
		Terme{"evasion-fiscale-multinationales", "évasion fiscale", `évasion fiscale`},
		Terme{"evasion-fiscale-multinationales", "taxe sur les services numériques, GAFAM", `taxe sur les services numériques|\mgafam\M`},
	)

	faits = append(faits,
		// Budget de l'État et de la Sécurité sociale.
		loi("lolf-2001", "budget-donnees", "CADRE", "2001-08-01", "Parlement (loi organique n° 2001-692)",
			"La loi organique relative aux lois de finances (LOLF)",
			"Texte qui fixe le cadre des lois de finances de l'État.",
			"JORFTEXT000000394028", "relative aux lois de finances"),
		loiArt("lo-gestion-finances-publiques-2021", "budget-donnees", "CADRE", "2021-12-28", "Parlement (loi organique n° 2021-1836)",
			"La modernisation de la gestion des finances publiques",
			"Révision de la loi organique relative aux lois de finances, qui touche notamment au Haut Conseil des finances publiques (article 30).",
			"JORFTEXT000044589827", "30", "Haut Conseil des finances publiques"),
		Fait{ID: "hcfp-avis-2025-5", Dossier: "budget-donnees", Section: "CONTROLE", Theme: "avis", Type: "EVALUATION",
			Date: "2025-10-09", Auteur: "Haut Conseil des finances publiques (avis n° HCFP-2025-5)",
			Intitule: "Budget 2026 : un scénario jugé optimiste et un solde « fragilisé »",
			Constat: "Le Haut Conseil juge optimistes les hypothèses économiques du projet de budget 2026 et estime la prévision de solde " +
				"public fragilisée par le risque que les mesures de recettes et d'économies ne soient pas réalisées.",
			URL: urlHCFP2025, Qualite: "OFFICIEL",
			Attendus: []string{"repose sur des hypothèses optimistes", "la prévision de solde public pour 2026 soumise au Haut Conseil est fragilisée"}},
		Fait{ID: "senat-plfss-2026-cades", Dossier: "budget-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-12-10", Auteur: "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			Intitule: "La dette sociale : un amortissement imposé d'ici 2033, des déficits qui s'accumulent à l'Acoss",
			Constat: "La commission rappelle que la dette sociale doit être amortie d'ici le 31 décembre 2033 et recommande un transfert " +
				"de la dette de l'Acoss, financée à court terme, vers la Cades.",
			URL: urlSenatPLFSS, Qualite: "OFFICIEL",
			Attendus: []string{"impose un amortissement de la dette sociale d'ici le 31 décembre 2033", "TRANSFERT DE LA DETTE DE L'ACOSS VERS LA CADES"}},

		// Dette publique.
		loi("lpfp-2023-2027", "dette-donnees", "CADRE", "2023-12-18", "Parlement (loi n° 2023-1195)",
			"La loi de programmation des finances publiques 2023-2027",
			"Trajectoire pluriannuelle de solde et de dette publics à laquelle le Haut Conseil des finances publiques compare chaque budget.",
			"JORFTEXT000048581885", "de programmation des finances publiques pour les années 2023 à 2027"),
		Fait{ID: "senat-deficit-excessif-2024", Dossier: "dette-donnees", Section: "CADRE", Theme: "règles européennes", Type: "CONSTAT",
			Date: "2024-07-26", Auteur: "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			Intitule: "La France sous procédure de déficit excessif depuis juillet 2024",
			Constat: "La France est à nouveau sous procédure européenne de déficit excessif depuis juillet 2024 et s'est engagée à ramener " +
				"son déficit public sous 3 points de PIB en 2029.",
			URL: urlSenatPLFSS, Qualite: "OFFICIEL",
			Attendus: []string{"La France est à nouveau sous procédure de déficit excessif depuis juillet 2024", "sous 3 points de PIB en 2029"}},
		Fait{ID: "hcfp-dette-2026", Dossier: "dette-donnees", Section: "CONTROLE", Theme: "avis", Type: "EVALUATION",
			Date: "2025-10-09", Auteur: "Haut Conseil des finances publiques (avis n° HCFP-2025-5)",
			Intitule: "Une dette à près de 118 points de PIB en 2026, une charge d'intérêts de 74 Md€",
			Montant:  "74000000000", Nature: "DEPENSE",
			Constat: "Selon le Haut Conseil, la dette publique passerait de plus de 113 points de PIB en 2024 à près de 118 en 2026, et " +
				"la charge d'intérêts atteindrait 74 Md€, en hausse de plus de 13 Md€ en deux ans (prévision).",
			URL: urlHCFP2025, Qualite: "OFFICIEL",
			Attendus: []string{"passant de plus de 113 points de PIB en 2024 à près de 118 points en 2026", "pour atteindre 74 Md€"}},

		// Chômage.
		loi("loi-rsa-2008", "chomage-donnees", "CADRE", "2008-12-01", "Parlement (loi n° 2008-1249)",
			"La généralisation du revenu de solidarité active (RSA)",
			"Loi qui généralise le revenu de solidarité active.",
			"JORFTEXT000019860428", "généralisant le revenu de solidarité active"),
		loi("loi-marche-travail-2022", "chomage-donnees", "CADRE", "2022-12-21", "Parlement (loi n° 2022-1598)",
			"Les mesures d'urgence sur le marché du travail",
			"Loi de 2022 sur le fonctionnement du marché du travail, adoptée « en vue du plein emploi ».",
			"JORFTEXT000046771781", "fonctionnement du marché du travail en vue du plein emploi"),
		loiArt("loi-plein-emploi-2023", "chomage-donnees", "CADRE", "2023-12-18", "Parlement (loi n° 2023-1196)",
			"La loi pour le plein emploi (France Travail)",
			"L'article 1er réécrit l'inscription sur la liste des demandeurs d'emploi, désormais tenue par l'opérateur France Travail.",
			"JORFTEXT000048581935", "1", "Est inscrite sur la liste des demandeurs d'emploi auprès de l'opérateur France Travail"),
		Fait{ID: "unedic-previsions-2026-03", Dossier: "chomage-donnees", Section: "CONTROLE", Theme: "prévisions du gestionnaire", Type: "EVALUATION",
			Date: "2026-03-03", Auteur: "Unédic (gestionnaire de l'Assurance chômage), prévisions financières",
			Intitule: "Assurance chômage : 38 Md€ d'indemnisation et 61,5 Md€ de dette prévus en 2026",
			Montant:  "38000000000", Nature: "DEPENSE",
			Constat: "Selon ses prévisions de mars 2026, les dépenses d'indemnisation atteindraient 38 Md€ en 2026 (37,2 Md€ en 2025) et " +
				"la dette du régime 61,5 Md€ (59,4 Md€ en 2025).",
			URL: urlUnedic2026, Qualite: "OFFICIEL",
			Attendus: []string{"38 Md€ en 2026 après 37,2 Md€ en 2025", "à 61,5 Md€, contre 59,4 Md€ en 2025"}},

		// Retraite.
		loiArt("lfrss-2023-retraites", "retraite-donnees", "CADRE", "2023-04-14", "Parlement (loi n° 2023-270)",
			"La loi de financement rectificative de la sécurité sociale pour 2023",
			"Son article 10 porte l'âge d'ouverture des droits à la retraite à soixante-quatre ans, progressivement selon la génération.",
			"JORFTEXT000047445077", "10", "soixante-quatre ans"),
		Fait{ID: "cor-rapport-2025-depenses", Dossier: "retraite-donnees", Section: "CONTROLE", Theme: "rapports d'évaluation", Type: "EVALUATION",
			Date: "2025-06-12", Auteur: "Conseil d'orientation des retraites (rapport annuel 2025)",
			Intitule: "407 Md€ de dépenses de retraite en 2024, 13,9 % du PIB ; un déficit de 1,7 Md€",
			Montant:  "407000000000", Nature: "DEPENSE",
			Constat: "Le COR évalue les dépenses de retraite à 407 Md€ en 2024 (13,9 % du PIB, 24,4 % des dépenses publiques) et le solde " +
				"du système à −1,7 Md€, hors produits et charges financiers.",
			URL: urlCOR2025, Qualite: "OFFICIEL",
			Attendus: []string{"les dépenses de retraite représentent 407 milliards d'euros, soit 13,9 % du PIB", "déficitaire de 1,7 milliard d'euros"}},

		// Pauvreté.
		loi("loi-rsa-2008-pauvrete", "pauvrete-donnees", "CADRE", "2008-12-01", "Parlement (loi n° 2008-1249)",
			"Le revenu de solidarité active et les politiques d'insertion",
			"Loi qui généralise le RSA et réforme les politiques d'insertion.",
			"JORFTEXT000019860428", "réformant les politiques d'insertion"),
		Fait{ID: "senat-prime-activite-2026", Dossier: "pauvrete-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-11-20", Auteur: "Sénat, commission des affaires sociales (avis sur la mission « Solidarité, insertion et égalité des chances », PLF 2026)",
			Intitule: "Prime d'activité : 4,57 millions de foyers en 2025, un nombre stable pour la première fois",
			Constat: "La commission relève que le nombre de foyers bénéficiaires de la prime d'activité est resté stable en 2025 (4,57 " +
				"millions), et que la précarité alimentaire ne diminue pas malgré la stabilisation du financement des associations.",
			URL: urlSenatSolid, Qualite: "OFFICIEL",
			Attendus: []string{"4,57 millions de foyers bénéficiaires", "sans pour autant que la précarité alimentaire ne diminue en France"}},

		// Éducation.
		loiArt("loi-ecole-confiance-2019", "education-donnees", "CADRE", "2019-07-26", "Parlement (loi n° 2019-791)",
			"La loi pour une école de la confiance",
			"Son article 11 rend l'instruction obligatoire dès trois ans et jusqu'à seize ans.",
			"JORFTEXT000038829065", "11", "L'instruction est obligatoire pour chaque enfant dès l'âge de trois ans et jusqu'à l'âge de seize ans"),
		loi("loi-aesh-temps-meridien-2024", "education-donnees", "CADRE", "2024-05-27", "Parlement (loi n° 2024-475)",
			"L'accompagnement des élèves handicapés pendant la pause méridienne pris en charge par l'État",
			"L'État prend en charge l'accompagnement humain des élèves en situation de handicap durant le temps de pause méridienne.",
			"JORFTEXT000049602933", "accompagnement humain des élèves en situation de handicap"),
		Fait{ID: "senat-enseignement-scolaire-2026", Dossier: "education-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-11-20", Auteur: "Sénat, commission de la culture (avis sur la mission « Enseignement scolaire », PLF 2026)",
			Intitule: "63,02 Md€ pour l'enseignement scolaire en 2026, après 12,13 Md€ de hausse depuis 2019",
			Montant:  "63020000000", Nature: "DEPENSE",
			Constat: "Hors pensions, les crédits de paiement des cinq programmes du ministère s'élèvent à 63,02 Md€ pour 2026, un budget " +
				"stable après une hausse de 12,13 Md€ depuis 2019 (crédits votés, pas exécutés).",
			URL: urlSenatEduc, Qualite: "OFFICIEL",
			Attendus: []string{"s'élèvent à 63,02 milliards d'euros", "HAUSSE DE 12,13 MILLIARDS D'EUROS DEPUIS 2019"}},

		// Santé et Sécurité sociale.
		loiArt("loi-systeme-sante-2019", "sante-donnees", "CADRE", "2019-07-24", "Parlement (loi n° 2019-774)",
			"La loi relative à l'organisation et à la transformation du système de santé",
			"Loi d'organisation du système de santé, dont l'article 41 crée la plateforme des données de santé.",
			"JORFTEXT000038821260", "41", "Plateforme des données de santé"),
		loi("lfss-2024", "securite-sociale-donnees", "CADRE", "2023-12-26", "Parlement (loi n° 2023-1250)",
			"La loi de financement de la sécurité sociale pour 2024",
			"Exemple de loi annuelle qui fixe les objectifs de dépenses et les prévisions de recettes des branches de la Sécurité sociale.",
			"JORFTEXT000048668665", "de financement de la sécurité sociale pour 2024"),
		Fait{ID: "senat-plfss-2026-deficit", Dossier: "securite-sociale-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-12-10", Auteur: "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			Intitule: "Budget de la Sécurité sociale 2026 : des mesures de réduction du déficit ramenées à 9 Md€",
			Constat: "Selon la commission, les mesures de réduction du déficit, de 15 Md€ dans le texte initial, n'étaient plus que de " +
				"9 Md€ dans le texte adopté.",
			URL: urlSenatPLFSS, Qualite: "OFFICIEL",
			Attendus: []string{"n'étaient plus que de 9 milliards d'euros dans le texte adopté"}},
		Fait{ID: "senat-plfss-2026-cades-cotisations", Dossier: "cotisations-et-droits", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-12-10", Auteur: "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			Intitule: "Les déficits sociaux s'accumulent à l'Acoss, qui ne peut s'endetter qu'à court terme",
			Constat: "La commission rappelle que l'Acoss, qui finance la Sécurité sociale, ne peut s'endetter qu'à court terme, et qu'à droit " +
				"inchangé les déficits cumulés s'y accumuleraient.",
			URL: urlSenatPLFSS, Qualite: "OFFICIEL",
			Attendus: []string{"que la loi n'autorise à s'endetter qu'à court terme sur les marchés"}},
		loiArt("lfrss-2023-cotisations", "cotisations-et-droits", "CADRE", "2023-04-14", "Parlement (loi n° 2023-270)",
			"La réforme des retraites de 2023",
			"Son article 10 porte l'âge d'ouverture des droits à la retraite à soixante-quatre ans : les cotisations versées ouvrent le droit plus tard.",
			"JORFTEXT000047445077", "10", "soixante-quatre ans"),

		// Sécurité, défense, pouvoirs publics, immigration.
		loi("lopmi-2023", "securite-police-donnees", "CADRE", "2023-01-24", "Parlement (loi n° 2023-22)",
			"La loi d'orientation et de programmation du ministère de l'Intérieur (LOPMI)",
			"Loi de programmation du ministère de l'Intérieur, à laquelle le Sénat compare chaque budget de la mission « Sécurités ».",
			"JORFTEXT000047046768", "d'orientation et de programmation du ministère de l'intérieur"),
		Fait{ID: "senat-lopmi-postes-2026", Dossier: "securite-police-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-11-20", Auteur: "Sénat, commission des lois (avis sur la mission « Sécurités », PLF 2026)",
			Intitule: "L'objectif de 7 412 créations de postes de la LOPMI « de plus en plus compromis »",
			Constat: "Le rapporteur juge l'objectif de 7 412 créations de postes d'ici 2027 de plus en plus compromis, malgré 1 000 postes " +
				"prévus pour la police et 400 pour la gendarmerie en 2026.",
			URL: urlSenatSecu, Qualite: "OFFICIEL",
			Attendus: []string{"L'objectif de la Lopmi de 7 412 créations de postes à horizon 2027"}},
		loi("lpm-2024-2030", "defense-donnees", "CADRE", "2023-08-01", "Parlement (loi n° 2023-703)",
			"La loi de programmation militaire 2024-2030",
			"Programmation des crédits et des effectifs des armées de 2024 à 2030.",
			"JORFTEXT000047914986", "relative à la programmation militaire pour les années 2024 à 2030"),
		Fait{ID: "senat-defense-2026", Dossier: "defense-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-11-20", Auteur: "Sénat, commission des finances (rapport spécial sur la mission « Défense », PLF 2026)",
			Personne: "dominique-de-legge",
			Intitule: "57,15 Md€ sur le périmètre de la LPM en 2026, et une actualisation de la loi jugée indispensable",
			Montant:  "57150000000", Nature: "DEPENSE",
			Constat: "Sur le périmètre de la LPM (hors pensions), les crédits demandés pour 2026 atteignent 57,15 Md€, en hausse de 6,67 Md€ ; " +
				"le rapporteur spécial juge indispensable une actualisation de la loi de programmation militaire (crédits votés).",
			URL: urlSenatDefense, Qualite: "OFFICIEL",
			Attendus: []string{"les crédits demandés s'établissent à 57,15 milliards d'euros", "la présentation d'une actualisation de la LPM apparaît désormais indispensable"}},
		loi("ordonnance-assemblees-1958", "pouvoirs-publics-donnees", "CADRE", "1958-11-17", "Gouvernement (ordonnance n° 58-1100)",
			"Le fonctionnement des assemblées parlementaires",
			"Ordonnance qui organise le fonctionnement des assemblées parlementaires, dont leurs règles budgétaires propres.",
			"JORFTEXT000000705067", "relative au fonctionnement des assemblées parlementaires"),
		Fait{ID: "senat-pouvoirs-publics-2026", Dossier: "pouvoirs-publics-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-11-20", Auteur: "Sénat, commission des finances (rapport spécial sur la mission « Pouvoirs publics », PLF 2026)",
			Personne: "gregory-blanc",
			Intitule: "Des dotations en hausse de 12 % en euros courants de 2011 à 2025",
			Constat: "Le rapporteur spécial relève que la dotation cumulée de la mission a progressé de 12 % entre 2011 et 2025 en euros " +
				"courants, et alerte sur les effets du gel prolongé des dotations sur les réserves des institutions.",
			URL: urlSenatPouvoirs, Qualite: "OFFICIEL",
			Attendus: []string{"La dotation cumulée de la mission a progressé de 12 % entre 2011 et 2025 en euros courants"}},
		loi("loi-immigration-2024", "immigration-donnees", "CADRE", "2024-01-26", "Parlement (loi n° 2024-42)",
			"La loi pour contrôler l'immigration, améliorer l'intégration",
			"Dernière loi d'ensemble sur l'entrée, le séjour et l'éloignement des étrangers chargée dans le corpus.",
			"JORFTEXT000049040245", "pour contrôler l'immigration, améliorer l'intégration"),
		Fait{ID: "senat-immigration-cout-2026", Dossier: "immigration-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "EVALUATION",
			Date: "2025-11-20", Auteur: "Sénat, commission des finances (rapport spécial sur la mission « Immigration, asile et intégration », PLF 2026)",
			Intitule: "7,82 Md€ : le coût estimé de la politique de l'immigration et de l'intégration en 2026",
			Montant:  "7820000000", Nature: "ESTIME",
			Constat: "Le coût estimé de la politique française de l'immigration et de l'intégration, toutes missions confondues, est de " +
				"7,82 Md€ en 2026, contre 7,74 Md€ en 2025 (document de politique transversale, cité par le rapport).",
			URL: urlSenatImmig, Qualite: "OFFICIEL",
			Attendus: []string{"Le coût estimé de la politique française de l'immigration et de l'intégration est de 7,82 milliards d'euros en 2026"}},
		loiArt("lo-defenseur-droits-2011", "violences-policieres-donnees", "CADRE", "2011-03-29", "Parlement (loi organique n° 2011-333)",
			"Le Défenseur des droits, compétent pour la déontologie des forces de sécurité",
			"Son article 4 charge le Défenseur des droits de veiller au respect de la déontologie par les personnes exerçant des activités de sécurité.",
			"JORFTEXT000023781167", "4", "déontologie"),
		Fait{ID: "ddd-deontologie-2024", Dossier: "violences-policieres-donnees", Section: "CONTROLE", Theme: "autorités indépendantes", Type: "EVALUATION",
			Date: "2025-03-05", Auteur: "Défenseur des droits (rapport annuel d'activité 2024)",
			Intitule: "2 434 réclamations sur la déontologie de la sécurité en 2024",
			Constat: "Le Défenseur des droits a reçu 2 434 réclamations en matière de déontologie de la sécurité en 2024. Une réclamation " +
				"n'est pas un manquement établi.",
			URL: urlDDD2024, Qualite: "OFFICIEL",
			Attendus: []string{"déontologie de la sécurité reçues par le Défenseur des droits en 2024 (N = 2 434)"}},

		// Écologie et eau.
		loiArt("loi-energie-climat-2019", "ecologie-donnees", "CADRE", "2019-11-08", "Parlement (loi n° 2019-1147)",
			"La loi relative à l'énergie et au climat",
			"Son article 1er inscrit dans le code de l'énergie un objectif de neutralité carbone.",
			"JORFTEXT000039355955", "1", "neutralité carbone"),
		loiArt("loi-climat-resilience-2021", "ecologie-donnees", "CADRE", "2021-08-22", "Parlement (loi n° 2021-1104)",
			"La loi climat et résilience",
			"Loi de lutte contre le dérèglement climatique, dont l'article 191 fixe l'objectif d'absence d'artificialisation nette des sols.",
			"JORFTEXT000043956924", "191", "artificialisation nette"),
		Fait{ID: "hcc-rapport-2025-emissions", Dossier: "ecologie-donnees", Section: "CONTROLE", Theme: "autorités indépendantes", Type: "EVALUATION",
			Date: "2025-07-03", Auteur: "Haut Conseil pour le climat (rapport annuel 2025)",
			Intitule: "Émissions brutes de 2024 inférieures de 32 % à 1990 ; deuxième budget carbone respecté",
			Constat: "Le Haut Conseil constate des émissions brutes 2024 inférieures de 32 % à leur niveau de 1990 et le respect du deuxième " +
				"budget carbone (406 Mt éqCO2 par an en moyenne de 2019 à 2023 pour un plafond de 425).",
			URL: urlHCC2025, Qualite: "OFFICIEL",
			Attendus: []string{"Le niveau atteint en 2024 pour les émissions brutes est inférieur de 32 % au niveau de 1990", "respectent le deuxième budget carbone"}},
		loiArt("loi-maptam-gemapi-2014", "bassins-versants-donnees", "CADRE", "2014-01-27", "Parlement (loi n° 2014-58)",
			"La loi MAPTAM, qui crée la compétence GEMAPI",
			"Son article 56 organise la compétence de gestion des milieux aquatiques et de prévention des inondations (GEMAPI).",
			"JORFTEXT000028526298", "56", "gestion des milieux aquatiques et de prévention des inondations"),
		Fait{ID: "senat-gestion-eau-2023", Dossier: "bassins-versants-donnees", Section: "CONTROLE", Theme: "rapports parlementaires", Type: "RECOMMANDATION",
			Date: "2023-07-11", Auteur: "Sénat, mission d'information sur la gestion durable de l'eau (rapporteur)", Personne: "herve-gille",
			Intitule: "53 propositions, dont une taxe GEMAPI mutualisée à l'échelle du bassin versant",
			Constat: "La mission propose notamment de renforcer la gouvernance par bassin et de mutualiser une fraction de la taxe GEMAPI " +
				"sur l'ensemble du bassin versant pour les intercommunalités aux ressources faibles.",
			URL: urlSenatEau, Qualite: "OFFICIEL",
			Attendus: []string{"Mettre en place une fraction de taxe GEMAPI mutualisée sur l'ensemble du bassin versant", "Renforcer la gouvernance de l'eau"}},

		// TVA.
		loiArt("loi-lfr-2012-taux-tva-2014", "tva-donnees", "CADRE", "2012-12-29", "Parlement (loi n° 2012-1510)",
			"La loi de finances rectificative pour 2012, qui fixe les taux actuels de 20 % et 10 %",
			"Son article 68 relève le taux normal de 19,60 % à 20 % et le taux intermédiaire de 7 % à 10 %, "+
				"pour les opérations dont le fait générateur intervient à compter du 1er janvier 2014.",
			"JORFTEXT000026857857", "68", "A la fin de l'article 278, le taux : « 19,60 % » est remplacé par le taux : « 20 % »",
			"le taux : « 7 % » est remplacé par le taux : « 10 % »"),
		Fait{ID: "cour-comptes-tva-part-recettes-2024", Dossier: "tva-donnees", Section: "ENJEUX", Type: "EVALUATION",
			Date: "2025-04-15", Auteur: "Cour des comptes, analyse de l'exécution budgétaire 2024 — Recettes fiscales de l'État",
			Intitule: "La TVA ne représente plus que 30 % des recettes fiscales nettes de l'État, contre 53 % en 2018",
			Constat: "La Cour constate que la TVA, \"principal impôt de rendement corrélé à la croissance économique\", fait l'objet depuis 2019 de "+
				"transferts croissants aux collectivités territoriales et à la Sécurité sociale, si bien qu'elle ne représente plus que 30 % des "+
				"recettes fiscales nettes de l'État en 2024, contre 53 % en 2018.",
			URL:     "https://www.ccomptes.fr/sites/default/files/2025-04/NEB-2024-Recettes-fiscales.pdf",
			Qualite: "OFFICIEL",
			Attendus: []string{"la TVA, principal impôt de rendement corrélé à la croissance",
				"la TVA ne représente plus que 30 % des recettes fiscales nettes en 2024, contre 53 % en 2018"}},

		// Évasion fiscale : le cadre (les contrôles sont dans ref.fait_multinationale).
		loiArt("loi-taxe-services-numeriques-2019", "evasion-fiscale-multinationales", "CADRE", "2019-07-24", "Parlement (loi n° 2019-759)",
			"La taxe sur les services numériques",
			"Son article 1er institue une taxe sur certains services fournis par les grandes entreprises du numérique, au taux de 3 %.",
			"JORFTEXT000038811588", "1", "Taxe sur certains services fournis par les grandes entreprises du secteur numérique", "un taux de 3 %"),
		Fait{ID: "lf-2024-imposition-minimale", Dossier: "evasion-fiscale-multinationales", Section: "CADRE", Theme: "textes", Type: "TEXTE",
			Date: "2023-12-29", Auteur: "Parlement (loi de finances pour 2024, article 33)",
			Intitule: "L'imposition minimale mondiale des grands groupes (pilier 2)",
			Constat:  "L'article 33 crée dans le code général des impôts l'imposition minimale mondiale des groupes, avec un taux de 15 % et un seuil de 750 millions d'euros de chiffre d'affaires.",
			Qualite:  "OFFICIEL", JO: "JORFTEXT000048727345", JOArticle: "33",
			Attendus: []string{"Imposition minimale mondiale des groupes d'entrep", "15 %", "750 millions"}},
		Fait{ID: "loi-sapin-2-cjip", Dossier: "evasion-fiscale-multinationales", Section: "CADRE", Theme: "textes", Type: "TEXTE",
			Date: "2016-12-09", Auteur: "Parlement (loi n° 2016-1691, dite Sapin 2, article 22)",
			Intitule: "La convention judiciaire d'intérêt public",
			Constat: "Avant toute poursuite, le procureur peut proposer à une personne morale mise en cause une convention judiciaire " +
				"d'intérêt public, notamment pour le blanchiment de fraude fiscale ; c'est la forme des règlements de Google (2019) et " +
				"de McDonald's (2022).",
			Qualite: "OFFICIEL", JO: "JORFTEXT000033558528", JOArticle: "22",
			Attendus: []string{"le procureur de la République peut proposer à une personne morale mise en cause", "blanchiment des infractions prévues aux articles 1741 et 1743 du code général des impôts"}},
	)
	// Enjeux : ce que les institutions de contrôle disent de l'importance du sujet.
	faits = append(faits,
		enjeu("enjeu-budget-dette-hcfp", "budget-donnees", "2025-10-09", "Haut Conseil des finances publiques (avis n° HCFP-2025-5)",
			"Une dette publique qui progresse « à un rythme préoccupant »",
			"Le Haut Conseil qualifie de préoccupant le rythme de progression de la dette publique prévu par le projet de budget 2026.",
			urlHCFP2025, "OFFICIEL", "La dette publique continuerait de ce fait de progresser à un rythme préoccupant"),
		enjeu("enjeu-dette-deficit-zone-euro", "dette-donnees", "2025-12-10", "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			"Le déficit rapporté au PIB le plus élevé de la zone euro, selon la Commission européenne",
			"Le rapport cite un déficit public estimé par le Gouvernement à 5,4 points de PIB en 2025 (5,8 en 2024), qui serait selon la "+
				"Commission européenne le plus élevé de la zone euro.",
			urlSenatPLFSS, "OFFICIEL", "il s'agirait du déficit rapporté au PIB le plus élevé de la zone euro", "5,4 points de PIB (après 5,8 points de PIB en 2024)"),
		enjeu("enjeu-secu-deficit-zone-euro", "securite-sociale-donnees", "2025-12-10", "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			"Des finances publiques « très dégradées »",
			"La commission ouvre son rapport sur le budget de la Sécurité sociale 2026 par la situation des finances publiques, qu'elle "+
				"qualifie de très dégradée.",
			urlSenatPLFSS, "OFFICIEL", "UNE SITUATION DES FINANCES PUBLIQUES TRÈS DÉGRADÉE"),
		enjeu("enjeu-cotisations-acoss-liquidite", "cotisations-et-droits", "2025-12-10", "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			"Le risque de liquidité d'une dette sociale financée à court terme",
			"La commission souligne le risque de liquidité inhérent à l'endettement de court terme de l'Acoss, qui finance la Sécurité sociale.",
			urlSenatPLFSS, "OFFICIEL", "risque de liquidité inhérent à ce type d'endettement"),
		enjeu("enjeu-retraite-cor-soutenabilite", "retraite-donnees", "2025-06-12", "Conseil d'orientation des retraites (rapport annuel 2025)",
			"Les dépenses de retraite rapportées au PIB, indicateur de soutenabilité",
			"Le COR retient la part des dépenses de retraite dans le PIB comme indicateur déterminant de la soutenabilité financière du système.",
			urlCOR2025, "OFFICIEL", "constituent un indicateur déterminant pour évaluer la soutenabilité financière du système"),
		enjeu("enjeu-chomage-desendettement", "chomage-donnees", "2026-03-03", "Unédic (gestionnaire de l'Assurance chômage), prévisions financières",
			"Le désendettement de l'Assurance chômage, un « défi » selon son gestionnaire",
			"Le gestionnaire du régime présente son désendettement comme un défi, dans des perspectives économiques qu'il juge moroses.",
			urlUnedic2026, "DECLARATIF", "l'Assurance chômage demeure confrontée au défi de son désendettement"),
		enjeu("enjeu-pauvrete-mission-solidarite", "pauvrete-donnees", "2025-11-20", "Sénat, commission des affaires sociales (avis sur la mission « Solidarité, insertion et égalité des chances », PLF 2026)",
			"Une mission budgétaire consacrée à la lutte contre la pauvreté",
			"La mission « Solidarité, insertion et égalité des chances » rassemble les crédits de l'État destinés à lutter contre la pauvreté et à protéger les personnes vulnérables.",
			urlSenatSolid, "OFFICIEL", "visant à lutter contre la pauvreté, à défendre et inclure les personnes vulnérables"),
		enjeu("enjeu-education-demographie", "education-donnees", "2025-11-20", "Sénat, commission de la culture (avis sur la mission « Enseignement scolaire », PLF 2026)",
			"La baisse du nombre d'élèves s'accélère",
			"Le rapport relève que la diminution du nombre de collégiens, limitée à 18 000 par an pendant deux rentrées, va s'accentuer très fortement.",
			urlSenatEduc, "OFFICIEL", "cette baisse va s'accentuer très fortement dès la prochaine rentrée"),
		enjeu("enjeu-sante-ondam-2026", "sante-donnees", "2025-12-10", "Sénat, commission des affaires sociales (rapport sur le PLFSS 2026)",
			"267,5 Md€ de dépenses prévues pour la branche maladie en 2026",
			"L'objectif de dépenses de la branche maladie, maternité, invalidité et décès est fixé à 267,5 Md€ pour 2026, en hausse de 2 % sur l'exécution 2025 (objectif voté).",
			urlSenatPLFSS, "OFFICIEL", "est fixé à 267,5 milliards d'euros pour 2026, soit une hausse de 2 %"),
		enjeu("enjeu-police-infractions", "securite-police-donnees", "2025-11-20", "Sénat, commission des lois (avis sur la mission « Sécurités », PLF 2026)",
			"Des acteurs auditionnés qui décrivent plus d'infractions et plus de violence",
			"Selon le rapporteur, les acteurs auditionnés confirment une progression importante du nombre d'infractions et de leur niveau de violence (constat d'audition, pas une statistique).",
			urlSenatSecu, "OFFICIEL", "ont confirmé cette progression importante du nombre d'infractions et de leur niveau de violence"),
		enjeu("enjeu-defense-contexte", "defense-donnees", "2025-11-20", "Sénat, commission des finances (rapport spécial sur la mission « Défense », PLF 2026)",
			"Une programmation militaire dans un contexte « profondément déstabilisé »",
			"Le rapport situe la troisième annuité de la LPM dans un contexte géostratégique profondément déstabilisé par la guerre en Ukraine.",
			urlSenatDefense, "OFFICIEL", "dans un contexte géostratégique profondément déstabilisé par la guerre en Ukraine"),
		enjeu("enjeu-pouvoirs-publics-autonomie", "pouvoirs-publics-donnees", "2025-11-20", "Sénat, commission des finances (rapport spécial sur la mission « Pouvoirs publics », PLF 2026)",
			"L'autonomie financière des pouvoirs publics, corollaire de leur indépendance",
			"Le rapporteur spécial présente un niveau de réserves suffisant comme une condition de l'autonomie financière des pouvoirs publics, corollaire de leur indépendance institutionnelle.",
			urlSenatPouvoirs, "OFFICIEL", "une condition essentielle de l'autonomie financière des pouvoirs publics, corollaire de leur indépendance institutionnelle"),
		enjeu("enjeu-immigration-irreguliere", "immigration-donnees", "2025-11-20", "Sénat, commission des finances (rapport spécial sur la mission « Immigration, asile et intégration », PLF 2026)",
			"La hausse des crédits 2026 destinée surtout à la lutte contre l'immigration irrégulière",
			"Le programme « Immigration et asile » capte toute l'augmentation des crédits de la mission pour 2026, largement destinée à la lutte contre l'immigration irrégulière (crédits demandés).",
			urlSenatImmig, "OFFICIEL", "largement à destination de la lutte contre l'immigration irrégulière"),
		enjeu("enjeu-ddd-atteintes-droits", "violences-policieres-donnees", "2025-03-05", "Défenseur des droits (rapport annuel d'activité 2024)",
			"Des réclamations qui traduisent « de nombreuses atteintes aux droits et libertés »",
			"Le Défenseur des droits présente les réclamations reçues en 2024, toutes missions confondues, comme la traduction de nombreuses atteintes aux droits et libertés.",
			urlDDD2024, "OFFICIEL", "traduisent de nombreuses atteintes aux droits et libertés en France"),
		enjeu("enjeu-ecologie-retards", "ecologie-donnees", "2025-07-03", "Haut Conseil pour le climat (rapport annuel 2025)",
			"L'instabilité politique et les retards d'arbitrage pèsent sur les investissements",
			"Le Haut Conseil estime que l'instabilité et les retards d'arbitrage liés au contexte politique ont des répercussions tangibles sur les investissements dans la transition.",
			urlHCC2025, "OFFICIEL", "L'instabilité et les retards d'arbitrage du fait du contexte politique actuel ont des répercussions tangibles sur les investissements dans la transition"),
		enjeu("enjeu-eau-urgence", "bassins-versants-donnees", "2023-07-11", "Sénat, mission d'information sur la gestion durable de l'eau",
			"« L'urgence d'agir pour nos usages, nos territoires et notre environnement »",
			"Intitulé de la mission d'information du Sénat de 2023, qui place la gouvernance de l'eau en tête de ses propositions.",
			urlSenatEau, "OFFICIEL", "l'urgence d'agir pour nos usages, nos territoires et notre environnement"),
	)

}

// enjeu : ce qu'une institution dit de l'importance d'un sujet, relu dans le document.
func enjeu(id, dossier, date, auteur, intitule, constat, url, qualite string, attendus ...string) Fait {
	return Fait{ID: id, Dossier: dossier, Section: "ENJEUX", Type: "EVALUATION", Date: date, Auteur: auteur, Intitule: intitule,
		Constat: constat, URL: url, Qualite: qualite, Attendus: attendus}
}
