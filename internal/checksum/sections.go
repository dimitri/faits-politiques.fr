package checksum

// Sections : la liste, tenue à la main, des tables dont dépend chaque section
// du site que cmd/build sait recopier plutôt que reconstruire (voir
// cmd/build/cache.go). Pas dérivée automatiquement des requêtes Go — un
// static-analysis qui lirait cmd/build pour la déduire serait plus élégant,
// mais bien plus fragile face à un JOIN ajouté sans y penser. La liste
// manuelle a le défaut inverse, plus honnête : elle peut oublier une table,
// jamais mentir sur celles qu'elle connaît. Sous-couvrir cette liste ne fait
// jamais servir une page fausse : au pire, une section reconstruite alors que
// rien n'a changé pour elle.
//
// Tenue à jour par quiconque ajoute une table lue par la section correspondante
// dans cmd/build — un oubli ici ne casse rien dans l'immédiat, mais fait
// recopier une page qui aurait dû être reconstruite.
var Sections = map[string][]string{
	// cmd/build/lieux_pages.go (chargerPagesCommunes, chargerPagesEPCI) et la
	// part de cmd/build/carte_situation.go qui alimente PageCommune.Situation.
	// PAS core.mandate ni core.person dans leur ensemble : seules les lignes
	// filtrées par MAIRE/CONSEILLER_MUNICIPAL comptent, mais la table entière
	// est ce que la requête pgcopydb-style peut mesurer sans la reproduire.
	"communes": {
		"core.mandate", "core.person", "core.commune_indicator",
		"core.commune_delinquance", "core.municipal_list", "core.association",
		"core.epci", "core.epci_membre", "ref.commune", "geo.contour_cog",
		"core.collectivite_budget",
	},
	// cmd/build/scrutins.go (buildScrutins et ses chargements associés :
	// loadDossiers, chargerExposes, groupBreakdown, nominalVotes).
	"scrutin": {
		"core.scrutin", "core.ballot", "core.dossier", "core.texte",
		"core.texte_expose", "core.organization", "core.person",
		"core.dossier_author", "core.texte_author",
	},
}
