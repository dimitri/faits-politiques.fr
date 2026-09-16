package verify

// Contrôles du bulletin de paie d'exemple (docs/cotisations-et-droits.md § 3 bis).
// Le bulletin est recalculé par des vues : ces contrôles figent ses totaux, qui
// ont été calculés indépendamment à la main, et vérifient que chaque euro du
// coût employeur trouve un destinataire.
func init() {
	checks = append(checks, checksPaie...)
}

var checksPaie = []check{
	{
		name:  "barème de paie 2026 chargé : taux, paramètres, destinataires",
		query: `SELECT count(*) FROM ref.taux_cotisation WHERE millesime = '2026-01-01'`,
		min:   20,
	},
	{
		name: "bulletin d'exemple : net payé 1 925,68 €, coût employeur 3 139,40 €",
		query: `SELECT count(*) FROM derived.bulletin_synthese
		         WHERE cas = 'technicienne-2500'
		           AND (net_paye <> 1925.68 OR cout_employeur <> 3139.40 OR reduction_generale <> 429.75
		                OR net_imposable <> 2050.22 OR coefficient <> 0.1719)`,
	},
	{
		name:  "bulletin d'exemple : présent",
		query: `SELECT count(*) FROM derived.bulletin_synthese WHERE cas = 'technicienne-2500'`,
		min:   1,
	},
	{
		// Salaire net + impôt + cotisations versées = coût employeur, au centime.
		name: "bulletin : les flux par destinataire redonnent le coût employeur",
		query: `SELECT count(*) FROM derived.bulletin_synthese s
		         WHERE s.cout_employeur <> (SELECT sum(verse) FROM derived.bulletin_flux f WHERE f.cas = s.cas)`,
	},
	{
		name: "bulletin : la réduction générale est imputée au centime près",
		query: `SELECT count(*) FROM derived.bulletin_synthese s
		         WHERE s.reduction_generale <> (SELECT coalesce(sum(imputation), 0) FROM derived.bulletin_reduction r WHERE r.cas = s.cas)`,
	},
	{
		name:  "bulletin : aucune cotisation n'est réduite au-delà de ce qui est dû",
		query: `SELECT count(*) FROM derived.bulletin_flux WHERE reduction > employeur_du`,
	},
	{
		name:  "bulletin : tout destinataire d'une ligne a un budget renseigné",
		query: `SELECT count(*) FROM derived.bulletin_flux WHERE organisme NOT IN ('SALARIE') AND budget IS NULL`,
	},
	{
		// Un taux nul ou absent sur une ligne non variable trahit une ligne mal transcrite.
		name:  "barème : chaque taux fixe est strictement positif",
		query: `SELECT count(*) FROM ref.taux_cotisation WHERE NOT variable AND (taux IS NULL OR taux <= 0)`,
	},
}
