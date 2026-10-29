package dossiers

// Les décisions de condamnation visant des personnes qui ont déclaré leur
// candidature à l'élection présidentielle de 2027 (data/candidats.csv). Règles
// du dossier (docs/candidats-condamnations.md) :
//
//   - une décision de juridiction, qui nomme la personne : ni enquête, ni mise
//     en examen, ni plainte, ni procès en cours ;
//   - le STADE y est toujours dit (première instance, appel, pourvoi, définitive)
//     et, quand la source ne le dit pas, elle le dit aussi : « non précisé » ;
//   - la qualité de la source est affichée : une décision de justice est
//     pseudonymisée dans les bases officielles (Judilibre, Légifrance : « M. [M] »)
//     et les sites des juridictions étaient inaccessibles à l'automate au
//     28 octobre 2026 (Justice.fr en maintenance, Légifrance en 403, site de la
//     Cour de cassation sans contenu pour l'automate) ; les trois affaires ci-dessous reposent
//     donc, pour l'instant, sur la presse qui les rapporte (PRESSE : citée, non
//     archivée). Chacune est à remplacer par le communiqué ou l'arrêt de la
//     juridiction dès qu'il est lisible ;
//   - rien n'est écrit qui ne figure dans la source citée : l'amende en appel et
//     le caractère exécutoire de l'inéligibilité de Marine Le Pen ne sont pas
//     donnés par elle, et le fait est dit tel quel plutôt que complété de mémoire.

const dossierJustice = "candidats-condamnations"

const (
	urlPSLePenAppel    = "https://www.publicsenat.fr/actualites/politique/direct-suivez-le-delibere-du-proces-en-appel-de-marine-le-pen"
	urlPSMelenchon2019 = "https://www.publicsenat.fr/actualites/politique/melenchon-condamne-a-trois-mois-avec-sursis-pour-la-perquisition-houleuse-a-lfi"
	urlICIZemmour2019  = "https://www.ici.fr/infos/societe/eric-zemmour-a-ete-definitivement-condamne-pour-provocation-a-la-haine-raciale-et-injures-racistes-6052200"
	urlOrangeZemmour20 = "https://actu.orange.fr/france/eric-zemmour-definitivement-condamne-pour-provocation-a-la-haine-magic-CNT000002lZEQx.html"
	themeLePen         = "Marine Le Pen"
	themeMelenchon     = "Jean-Luc Mélenchon"
	themeZemmour       = "Éric Zemmour"
	authorCAParis      = "Cour d'appel de Paris"
	authorCassation    = "Cour de cassation"
	authorTCBobigny    = "Tribunal correctionnel de Bobigny"
)

func init() {
	facts = append(facts,
		Fact{ID: "justice-cadre-ineligibilite-2017", Dossier: dossierJustice, Section: "CADRE", Theme: "textes", Type: "TEXTE",
			Date: "2017-09-15", Author: "Parlement (loi n° 2017-1339 pour la confiance dans la vie politique, article 1er)",
			Title: "L'inéligibilité est obligatoire pour les crimes et une liste de délits, sauf décision spécialement motivée",
			Finding: "Le texte publié au Journal officiel en septembre 2017 crée l'article 131-26-2 du code pénal : la peine " +
				"complémentaire d'inéligibilité est obligatoire contre toute personne coupable d'un crime ou de l'un des délits " +
				"que l'article énumère (dont, selon sa liste, des délits d'atteinte à la probité), et la condamnation est " +
				"mentionnée au bulletin n° 2 du casier judiciaire pendant la durée de l'inéligibilité. La juridiction peut " +
				"cependant décider de ne pas la prononcer, par une décision spécialement motivée. Le même texte ouvre aux " +
				"autorités qui reçoivent les déclarations de candidature l'accès à cette information. C'est le texte tel que " +
				"publié en 2017 : les modifications ultérieures du code ne sont pas vérifiées ici, et le texte ne traite ni " +
				"de l'exécution provisoire ni du recours.",
			Quality: "OFFICIEL", JO: "JORFTEXT000035567974", JOArticle: "1",
			Expected: []string{
				"est obligatoire à l'encontre de toute personne coupable d'un délit mentionné au II du présent article ou d'un crime",
				"par une décision spécialement motivée, décider de ne pas prononcer la peine",
				"autorités compétentes pour recevoir les déclarations de candidatures à une élection",
			}},
		Fact{ID: "justice-le-pen-appel-2026", Dossier: dossierJustice, Section: "CONTROLE",
			Theme: themeLePen, Type: "SANCTION", Date: "2026-07-07", Author: authorCAParis, Person: "marine-le-pen",
			Title: "En appel : coupable de détournement de fonds publics, peine réduite — pourvoi non établi",
			Finding: "La cour d'appel déclare Marine Le Pen coupable de détournement de fonds publics dans l'affaire des " +
				"assistants parlementaires du Front national (argent du Parlement européen). Peine rapportée : 3 ans " +
				"d'emprisonnement dont 2 avec sursis, la partie ferme (1 an) à purger sous bracelet électronique ; " +
				"inéligibilité de 45 mois dont 30 avec sursis, soit 15 mois ferme, décomptée depuis le jugement de " +
				"première instance du 31 mars 2025 (cinq ans en première instance). Le montant de l'amende en appel et " +
				"le caractère exécutoire de l'inéligibilité ne sont pas donnés par la source citée.",
			Quality: "PRESSE", URL: urlPSLePenAppel},
		Fact{ID: "justice-le-pen-situation", Dossier: dossierJustice, Section: "SITUATION",
			Theme: themeLePen, Type: "CONSTAT", Date: "2026-07-07", Author: authorCAParis, Person: "marine-le-pen",
			Title: "Stade au 7 juillet 2026 : décision d'appel rendue, pourvoi en cassation ni annoncé ni exclu",
			Finding: "Le jour de la décision, l'avocat de Marine Le Pen renvoie à une « communication ultérieure » ; un " +
				"coprévenu dit qu'on va « réfléchir à se pourvoir en cassation », et l'avocat du Parlement européen " +
				"relève que, sans pourvoi, la déclaration de culpabilité devient définitive. La suite (pourvoi, audience, " +
				"arrêt) n'a PAS été vérifiée dans une source officielle : l'état de la procédure au jour où vous lisez " +
				"ceci peut avoir changé.",
			Quality: "PRESSE", URL: urlPSLePenAppel},

		Fact{ID: "justice-melenchon-bobigny-2019", Dossier: dossierJustice, Section: "CONTROLE",
			Theme: themeMelenchon, Type: "SANCTION", Date: "2019-12-09", Author: authorTCBobigny, Person: "jean-luc-melenchon",
			Title: "Première instance : condamné pour rébellion et provocation après la perquisition de LFI",
			Finding: "Condamné à trois mois d'emprisonnement avec sursis et 8 000 € d'amende, à la suite de la " +
				"perquisition du siège de La France insoumise du 16 octobre 2018 ; la source cite « rébellion et " +
				"provocation ». Jean-Luc Mélenchon a annoncé le jour même qu'il ne ferait pas appel. La source ne dit " +
				"pas si le parquet a lui-même fait appel : la décision n'est donc pas établie définitive ici.",
			Quality: "PRESSE", URL: urlPSMelenchon2019},

		Fact{ID: "justice-zemmour-convention-droite", Dossier: dossierJustice, Section: "CONTROLE",
			Theme: themeZemmour, Type: "SANCTION", Date: "2025-09-16", Author: authorCassation, Person: "eric-zemmour",
			Title: "Définitive : provocation à la haine raciale et injures racistes (propos du 28 septembre 2019)",
			Finding: "La Cour de cassation rejette le pourvoi d'Éric Zemmour : la condamnation, prononcée par la cour d'appel " +
				"de Paris le 22 février 2024, est définitive — amende de 15 000 € et 1 000 € de dommages et intérêts à SOS " +
				"Racisme et aux autres parties civiles, pour des propos tenus le 28 septembre 2019 à une « convention de la " +
				"droite ». Historique selon la source : condamné en première instance (tribunal correctionnel de Paris, " +
				"septembre 2020), relaxé en appel, relaxe cassée en février 2023 et affaire renvoyée devant la cour d'appel " +
				"de Paris. La date exacte de l'arrêt n'est pas donnée : « ce mardi » dans un article du 17 septembre 2025.",
			Quality: "PRESSE", URL: urlICIZemmour2019},
		Fact{ID: "justice-zemmour-mineurs-isoles", Dossier: dossierJustice, Section: "CONTROLE",
			Theme: themeZemmour, Type: "SANCTION", Date: "2025-12-02", Author: authorCassation, Person: "eric-zemmour",
			Title: "Définitive : complicité d'injure publique et de provocation à la haine (propos de 2020 sur CNews)",
			Finding: "La Cour de cassation rejette le pourvoi d'Éric Zemmour et confirme la condamnation prononcée par la " +
				"cour d'appel de Paris en septembre 2024 (100 jours-amende à 100 €, soit 10 000 €), pour des propos tenus en " +
				"2020 sur CNews à propos des mineurs isolés. Selon la source, la Cour de cassation juge que la cour d'appel " +
				"a pu estimer ces propos au-delà des limites admissibles de la liberté d'expression, en raison de leur « " +
				"violence » et de leur « généralité ».",
			Quality: "PRESSE", URL: urlOrangeZemmour20},
	)
}
