package dossiers

// Ce que core.collectivite_budget (niveau 'SDIS') ne couvre pas : Paris et
// Marseille n'ont pas de SDIS — la Brigade de sapeurs-pompiers de Paris
// (BSPP) et le Bataillon de marins-pompiers de Marseille (BMPM) sont des
// unités militaires, financées et commandées autrement. Les deux rapports
// de la Cour des comptes du 12 février 2025 sont la seule source structurée
// trouvée sur leur financement — aucun jeu de données, seulement ces PDF,
// chaque citation relue au chargement (textFromPDF, voir dossiers.go).

const urlCCBSPP = "https://www.ccomptes.fr/sites/default/files/2025-02/20250212-S2024-1573_Brigade-Sapeurs-pompiers-de-Paris.pdf"
const urlCCBMPM = "https://www.ccomptes.fr/sites/default/files/2025-02/20250212-S2024-1588-Le-bataillon-de%20marins-pompiers-de-Marseille.pdf"
const urlDGSCGCEffectifs = "https://www.interieur.gouv.fr/sites/minint/files/medias/documents/2025-12/2025-12-Statistiques-SDIS25.pdf"
const urlSenatLFIExecution2024 = "https://www.senat.fr/rap/l24-743-229-2/l24-743-229-2_mono.html"
const urlSenatFlotteCanadair = "https://www.senat.fr/rap/r22-838/r22-838-syn.pdf"
const urlFranceInfoSavoie = "https://www.franceinfo.fr/faits-divers/incendie/un-sapeur-pompier-volontaire-de-22-ans-meurt-en-savoie-lors-d-une-intervention-sur-un-feu-de-vegetation_8099114.html"
const urlCnewsGironde = "https://www.cnews.fr/france/2026-07-27/canadair-en-maintenance-feu-fixe-en-gironde-116085-hectares-brules-laurent-nunez"

func init() {
	facts = append(facts,
		Fact{ID: "cc-bspp-2025-budget", Dossier: "securite-civile-donnees", Section: "CADRE",
			Theme: "BSPP et BMPM : hors SDIS", Type: "CONSTAT",
			Date: "2025-02-12", Author: "Cour des comptes",
			Title: "Le budget de la Brigade de sapeurs-pompiers de Paris (BSPP) et son financement à quatre",
			Finding: "La BSPP n'est pas un SDIS : ses dépenses (493,4 M€ en 2022, dont 432,7 M€ de fonctionnement) " +
				"figurent au budget spécial de la préfecture de police de Paris, voté par le Conseil de Paris. " +
				"C'est un « budget de contributions » auquel participent l'État (sur le seul fonctionnement), la " +
				"Ville de Paris, les trois départements de la petite couronne et leurs 123 communes, « à hauteur " +
				"d'environ un quart pour chaque partie prenante ».",
			Amount: "493400000", Nature: "DEPENSE",
			Quality: "OFFICIEL", URL: urlCCBSPP,
			Expected: []string{
				"dépenses de la BSPP en 2022 s'élèvent à 493,4 M€, soit 432,7 M€ en fonctionnement",
				"à hauteur d'environ un quart pour chaque partie prenante",
			}},
		Fact{ID: "cc-bspp-bmpm-statut-distinct", Dossier: "securite-civile-donnees", Section: "CADRE",
			Theme: "BSPP et BMPM : hors SDIS", Type: "CONSTAT",
			Date: "2025-02-12", Author: "Cour des comptes",
			Title: "Deux statuts militaires, deux montages financiers différents",
			Finding: "Les sapeurs-pompiers de Paris ont un statut militaire mais sont rémunérés directement sur " +
				"le budget spécial de la préfecture de police — « à la différence du bataillon de marins-pompiers " +
				"de Marseille (BMPM) où les militaires sont rémunérés par le ministère des armées », qui se fait " +
				"ensuite rembourser une partie des rémunérations et des charges sociales par la ville de Marseille.",
			Quality: "OFFICIEL", URL: urlCCBSPP,
			Expected: []string{
				"à la différence du bataillon de marins-pompiers de Marseille (BMPM) où les militaires sont rémunérés par le ministère des armées",
			}},
		Fact{ID: "cc-bmpm-2025-budget", Dossier: "securite-civile-donnees", Section: "CADRE",
			Theme: "BSPP et BMPM : hors SDIS", Type: "CONSTAT",
			Date: "2025-02-12", Author: "Cour des comptes",
			Title: "Le budget du Bataillon de marins-pompiers de Marseille (BMPM)",
			Finding: "Le BMPM, lui non plus, n'est pas un SDIS : en 2022, ses dépenses ont atteint 123,2 M€ en " +
				"fonctionnement (dont 82 % de personnel) et 24 M€ en investissement — la ville de Marseille " +
				"assumant « la plus grande partie » de ces deux budgets.",
			Amount: "123200000", Nature: "DEPENSE",
			Quality: "OFFICIEL", URL: urlCCBMPM,
			Expected: []string{
				"les dépenses du bataillon ont atteint 123,2 M€ en fonctionnement",
				"et 24 M€ en investissement",
			}},
		Fact{ID: "cc-bmpm-2025-financeurs", Dossier: "securite-civile-donnees", Section: "CADRE",
			Theme: "BSPP et BMPM : hors SDIS", Type: "CONSTAT",
			Date: "2025-02-12", Author: "Cour des comptes",
			Title: "Qui finance le BMPM : État, département, métropole — hors CAS Pensions",
			Finding: "Hors pensions militaires (CAS Pensions, à part), « près de 40 % du coût » du bataillon est " +
				"financé par l'État et les collectivités : en 2024, la participation de l'État est passée de 10 à " +
				"15 M€, le département verse 10 M€ et la métropole 12,9 M€ — des montants fixés par le code " +
				"général des collectivités territoriales que la Cour relève comme non indexés, donc en baisse en " +
				"euros constants.",
			Quality: "OFFICIEL", URL: urlCCBMPM,
			Expected: []string{
				"En 2024, la participation de l'État est passée de 10 à 15 M€",
				"les participations du département (10 M€) et de la métropole (12,9 M€)",
			}},
		Fact{ID: "cc-bmpm-2025-tsca", Dossier: "securite-civile-donnees", Section: "ENJEUX",
			Theme: "la TSCA, un financement indirect des SDIS", Type: "CONSTAT",
			Date: "2025-02-12", Author: "Cour des comptes",
			Title: "Un mécanisme réel mais à part : la taxe sur les conventions d'assurance",
			Finding: "La dotation de l'État au BMPM est, à partir de 2025, indexée sur « la dynamique de la taxe " +
				"spéciale sur les conventions d'assurance prélevée au bénéfice des départements pour le " +
				"financement des SDIS » — confirmation officielle que cette taxe (TSCA) finance bien les " +
				"départements au titre de leur contribution aux SDIS, mais aucun jeu de données structuré n'a été " +
				"trouvé pour en suivre le produit SDIS par SDIS (voir « Ce que les données ne disent pas »).",
			Quality: "OFFICIEL", URL: urlCCBMPM,
			Expected: []string{
				"taxe spéciale sur les conventions d'assurance prélevée au bénéfice des départements pour le financement des SDIS",
			}},

		// Effectifs : le seul chiffre nationalement établi, et seulement en PDF
		// (DGSCGC) — voir « Ce que les données ne disent pas » pour l'absence de
		// série structurée SPP/SPV/PATS par SDIS et par année.
		Fact{ID: "dgscgc-2024-effectifs", Dossier: "securite-civile-donnees", Section: "ENJEUX",
			Theme: "les effectifs, un seul chiffre national", Type: "CONSTAT",
			Date: "2024-12-31", Author: "DGSCGC, ministère de l'Intérieur",
			Title: "256 379 sapeurs-pompiers, aux trois quarts volontaires",
			Finding: "Au 31 décembre 2024 (dernière édition disponible, parue en décembre 2025), la France compte 256 379 sapeurs-pompiers : " +
				"43 985 professionnels (SPP, 17 %), 198 918 volontaires (SPV, 78 %) et 13 476 militaires (BSPP et " +
				"BMPM, 5 %) — auxquels s'ajoutent 11 508 personnels administratifs, techniques et spécialisés " +
				"(PATS) employés par les SDIS, qui ne sont pas des sapeurs-pompiers.",
			Quality: "OFFICIEL", URL: urlDGSCGCEffectifs,
			Expected: []string{
				"L'effectif des sapeurs-pompiers est de 256 379",
				"dont 43 985 sapeurs-pompiers professionnels (SPP) soit 17 %",
				"dont 198 918 sapeurs-pompiers volontaires (SPV) soit 78 %",
			}},

		// Contrôles et évaluations : les deux rapports du Sénat sur le
		// programme 161 et la flotte aérienne — le seul endroit où l'écart
		// voté/exécuté et la promesse présidentielle de 2022 sont établis.
		Fact{ID: "senat-743-lfi-execution-2024", Dossier: "securite-civile-donnees", Section: "CONTROLE",
			Theme: "crédits votés contre crédits exécutés", Type: "EVALUATION",
			Date: "2025-06-18", Author: "Sénat, commission des finances (rapporteur spécial sur la loi de règlement " +
				"2024, mission « Sécurités »)",
			Title: "2024 : l'exécution dépasse le vote de 8 à 9 %, portée par l'urgence",
			Finding: "Pour le programme « Sécurité civile », les crédits exécutés en 2024 dépassent ceux votés en " +
				"loi de finances initiale de 8,7 % en autorisations d'engagement (901,7 M€ votés, 980,5 M€ " +
				"exécutés) et de 9,3 % en crédits de paiement (880,5 M€ votés, 962,6 M€ exécutés) — un écart que " +
				"le rapporteur spécial attribue à « d'importants reports de crédits d'investissement de 2023 " +
				"ainsi qu'à la gestion d'évènements exceptionnels ». Les gels de crédits de l'année ont " +
				"principalement porté sur l'investissement, « en raison des incertitudes associées aux délais de " +
				"réalisation du programme d'acquisition d'avions bombardiers d'eau » — un dépassement qui n'est " +
				"donc pas un surinvestissement choisi, mais la trace comptable d'une année où il a fallu dépenser " +
				"plus que prévu, notamment sur les moyens aériens.",
			Quality: "OFFICIEL", URL: urlSenatLFIExecution2024,
			Expected: []string{
				"s'explique par d'importants reports de crédits d'investissement de 2023 ainsi que par la gestion d'évènements exceptionnels",
				"en raison des incertitudes associées aux délais de réalisation du programme d'acquisition d'avions bombardiers d'eau",
			}},
		Fact{ID: "senat-838-flotte-canadair", Dossier: "securite-civile-donnees", Section: "CONTROLE",
			Theme: "la flotte aérienne, une promesse de 2022 toujours pas tenue", Type: "EVALUATION",
			Date: "2023-06-22", Author: "Sénat, commission des finances",
			Title: "12 Canadair, 8 Dash, 3 Beechcraft — et un renouvellement sans cesse repoussé",
			Finding: "La flotte française de bombardiers d'eau est composée de 12 avions Canadair CL415, 8 avions " +
				"Dash et 3 avions Beechcraft King 200. Le président de la République a annoncé, le 28 octobre 2022, " +
				"que « la flotte vieillissante de 12 Canadair serait intégralement renouvelée, et portée à 16 " +
				"appareils » ; leur livraison, « aujourd'hui estimée à 2027, mais qui a déjà plusieurs fois été " +
				"repoussée », dépend d'un constructeur unique (De Havilland Canada) qui a tardé à relancer sa " +
				"chaîne de production, arrêtée depuis 2015.",
			Quality: "OFFICIEL", URL: urlSenatFlotteCanadair,
			Expected: []string{
				"composée de 12 avions Canadair CL415, 8 avions Dash, et 3 avions Beechcraft King 200",
				"la flotte vieillissante de 12 Canadair serait intégralement renouvelée, et portée à 16 appareils",
				"aujourd'hui estimée à 2027, mais qui a déjà plusieurs fois été repoussée",
			}},

		// L'été 2026 : deux événements réels et distincts, jamais mêlés — le
		// feu de Gironde/Landes (aucun mort, un manque de Canadair documenté
		// par le ministre lui-même) et la mort d'un sapeur-pompier volontaire
		// en Savoie, un incendie différent, à près de 800 km de là.
		Fact{ID: "ete-2026-gironde-canadair", Dossier: "securite-civile-donnees", Section: "ENJEUX",
			Theme: "l'été 2026 : quand les moyens ont manqué", Type: "DECLARATION",
			Date: "2026-07-27", Author: "Laurent Nuñez, ministre de l'Intérieur (point de presse sur les incendies)",
			Title: "7 Canadair disponibles sur 12, en pleine saison des feux",
			Finding: "Au plus fort des incendies de Gironde (plus de 42 000 ha brûlés, environ 220 000 personnes " +
				"évacuées, fin juillet 2026), le ministre de l'Intérieur a annoncé que seuls 7 des 12 Canadair de " +
				"la flotte française étaient disponibles, les autres étant en maintenance — à l'échelle de la " +
				"saison, 13 566 départs de feux et 116 085 hectares brûlés ont été recensés en France.",
			Quality: "PRESSE", URL: urlCnewsGironde},
		Fact{ID: "ete-2026-savoie-deces", Dossier: "securite-civile-donnees", Section: "ENJEUX",
			Theme: "l'été 2026 : quand les moyens ont manqué", Type: "CONSTAT",
			Date: "2026-07-08", Author: "Laurent Nuñez, ministre de l'Intérieur",
			Title: "Un sapeur-pompier volontaire de 22 ans tué en Savoie — un feu différent, pas celui de Gironde",
			Finding: "Baptiste Gerfaud Valentin, jeune caporal du centre d'incendie et de secours d'Albertville, est " +
				"mort le 8 juillet 2026, emporté par une chute de bloc rocheux alors qu'il combattait depuis la " +
				"nuit un feu de forêt à Planay (Savoie), déclaré le 24 juin après un orage sur un terrain " +
				"difficile d'accès. Un incendie distinct de celui de Gironde, à ne jamais confondre avec lui : " +
				"même saison, deux événements différents.",
			Quality: "PRESSE", URL: urlFranceInfoSavoie},
	)
}
