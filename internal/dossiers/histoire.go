package dossiers

// Guerres de décolonisation : le cadre légal était vide (aucun texte chargé,
// contrairement aux autres sections) — deux textes réels, déjà dans le corpus
// JORF (jo.texte), jamais rattachés à ce dossier. Ni l'un ni l'autre n'a de
// article_num exploitable dans jo.bloc pour ce texte précis (vérifié
// directement) : la citation porte sur le titre complet du texte, pas sur un
// article particulier — d'où l'absence de JOArticle ci-dessous.

func init() {
	faits = append(faits,
		Fait{ID: "loi-62-421-accords-evian", Dossier: "guerres-decolonisation-donnees", Section: "CADRE",
			Theme: "textes", Type: "TEXTE",
			Date: "1962-04-13", Auteur: "Parlement (loi n° 62-421)",
			Intitule: "La loi qui donne effet aux accords d'Évian",
			Constat: "Votée au lendemain du référendum du 8 avril 1962 (90,8 % de oui), cette loi habilite le " +
				"président de la République à prendre par ordonnances les mesures nécessaires à l'application des " +
				"déclarations gouvernementales du 19 mars 1962 — le nom officiel des accords d'Évian dans les textes " +
				"français de l'époque, qui ne parlent pas d'un traité au sens classique.",
			Qualite: "OFFICIEL", JO: "JORFTEXT000000509045",
			Attendus: []string{"les accords à établir et les mesures à prendre au sujet de l'Algérie",
				"déclarations gouvernementales du 19 mars 1962"}},
		Fait{ID: "loi-99-882-guerre-algerie", Dossier: "guerres-decolonisation-donnees", Section: "CADRE",
			Theme: "textes", Type: "TEXTE",
			Date: "1999-10-18", Auteur: "Parlement (loi n° 99-882)",
			Intitule: "La loi qui reconnaît juridiquement l'expression « guerre d'Algérie »",
			Constat: "Jusqu'à cette loi, les textes officiels ne parlaient que d'« opérations effectuées en Afrique " +
				"du Nord » ; votée à l'unanimité des deux chambres, elle substitue partout l'expression « guerre " +
				"d'Algérie ou combats en Tunisie et au Maroc » — trente-sept ans après le cessez-le-feu.",
			Qualite: "OFFICIEL", JO: "JORFTEXT000000578132",
			Attendus: []string{"aux opérations effectuées en Afrique du Nord",
				"à la guerre d'Algérie ou aux combats en Tunisie et au Maroc"}},
	)
}
