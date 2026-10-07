package dossiers

// Guerres de décolonisation : le cadre légal était vide (aucun texte chargé,
// contrairement aux autres sections) — deux textes réels, déjà dans le corpus
// JORF (jo.texte), jamais rattachés à ce dossier. Ni l'un ni l'autre n'a de
// article_num exploitable dans jo.bloc pour ce texte précis (vérifié
// directement) : la citation porte sur le titre complet du texte, pas sur un
// article particulier — d'où l'absence de JOArticle ci-dessous.

func init() {
	facts = append(facts,
		Fact{ID: "loi-62-421-accords-evian", Dossier: "guerres-decolonisation-donnees", Section: "CADRE",
			Theme: "textes", Type: "TEXTE",
			Date: "1962-04-13", Author: "Parlement (loi n° 62-421)",
			Title: "La loi qui donne effet aux accords d'Évian",
			Finding: "Votée au lendemain du référendum du 8 avril 1962 (90,8 % de oui), cette loi habilite le " +
				"président de la République à prendre par ordonnances les mesures nécessaires à l'application des " +
				"déclarations gouvernementales du 19 mars 1962 — le nom officiel des accords d'Évian dans les textes " +
				"français de l'époque, qui ne parlent pas d'un traité au sens classique.",
			Quality: "OFFICIEL", JO: "JORFTEXT000000509045",
			Expected: []string{"les accords à établir et les mesures à prendre au sujet de l'Algérie",
				"déclarations gouvernementales du 19 mars 1962"}},
		Fact{ID: "loi-99-882-guerre-algerie", Dossier: "guerres-decolonisation-donnees", Section: "CADRE",
			Theme: "textes", Type: "TEXTE",
			Date: "1999-10-18", Author: "Parlement (loi n° 99-882)",
			Title: "La loi qui reconnaît juridiquement l'expression « guerre d'Algérie »",
			Finding: "Jusqu'à cette loi, les textes officiels ne parlaient que d'« opérations effectuées en Afrique " +
				"du Nord » ; votée à l'unanimité des deux chambres, elle substitue partout l'expression « guerre " +
				"d'Algérie ou combats en Tunisie et au Maroc » — trente-sept ans après le cessez-le-feu.",
			Quality: "OFFICIEL", JO: "JORFTEXT000000578132",
			Expected: []string{"aux opérations effectuées en Afrique du Nord",
				"à la guerre d'Algérie ou aux combats en Tunisie et au Maroc"}},
	)
}
