package main

// Contrôles du bloc « la France est-elle un paradis fiscal ? »
// (docs/paradis-fiscal-donnees.md).
func init() {
	checks = append(checks, checksFiscalite...)
}

var checksFiscalite = []check{
	{
		name:  "listes UE : chaque version de l'annexe I est chargée",
		query: `SELECT count(DISTINCT version) FROM ref.juridiction_non_cooperative WHERE liste = 'UE_ANNEXE_I'`,
		min:   23,
	},
	{
		// Par construction, la liste européenne n'examine que des pays tiers :
		// un État membre dans l'annexe I trahirait une erreur de transcription.
		name: "listes UE : aucun État membre dans l'annexe I",
		query: `SELECT count(*) FROM ref.juridiction_non_cooperative
		         WHERE liste = 'UE_ANNEXE_I'
		           AND juridiction IN ('Austria','Belgium','Bulgaria','Croatia','Cyprus','Czechia','Denmark','Estonia',
		               'Finland','France','Germany','Greece','Hungary','Ireland','Italy','Latvia','Lithuania','Luxembourg',
		               'Malta','Netherlands','Poland','Portugal','Romania','Slovakia','Slovenia','Spain','Sweden')`,
	},
	{
		name:  "liste ETNC : les arrêtés à tableau depuis 2010 sont lus",
		query: `SELECT count(DISTINCT version) FROM ref.juridiction_non_cooperative WHERE liste = 'ETNC_FR'`,
		min:   8,
	},
	{
		// Depuis 2020 chaque inscription porte son fondement légal : un motif
		// manquant signale un rowspan mal suivi.
		name: "liste ETNC : depuis 2020, chaque juridiction a un motif",
		query: `SELECT count(*) FROM ref.juridiction_non_cooperative
		         WHERE liste = 'ETNC_FR' AND version >= '2020-01-01' AND motif IS NULL`,
	},
	{
		name: "CbCR : les groupes américains déclarent la France et le reste du monde chaque année",
		query: `SELECT count(DISTINCT annee) FROM core.cbcr_agregat
		         WHERE siege = 'USA' AND juridiction IN ('FRA','WXD') AND mesure = 'PROFIT' AND groupe_profit = '_T'`,
		min: 8,
	},
	{
		// Salariés d'une juridiction ≤ salariés de tout l'étranger du siège.
		name: "CbCR : une juridiction ne dépasse jamais le reste du monde en salariés",
		query: `SELECT count(*) FROM core.cbcr_agregat j
		          JOIN core.cbcr_agregat w ON w.annee = j.annee AND w.siege = j.siege AND w.juridiction = 'WXD'
		                                  AND w.mesure = j.mesure AND w.groupe_profit = j.groupe_profit
		         WHERE j.mesure = 'EMPLOYEES' AND j.groupe_profit = '_T'
		           AND j.juridiction NOT IN ('WXD', j.siege) AND j.valeur > w.valeur * 1.001`,
	},
	{
		// Les sous-panels « bénéficiaires » et « déficitaires » ne se somment pas
		// exactement au total publié (écart jusqu'à 150 Md$ sur le reste du monde
		// en 2022 : panels et totaux compilés séparément par les pays). Ce qui
		// doit tenir : les seuls bénéficiaires ne déclarent pas moins que le
		// total, pertes comprises.
		name: "CbCR : le bénéfice des sous-groupes bénéficiaires couvre le total",
		query: `SELECT count(*) FROM core.cbcr_agregat t
		          JOIN core.cbcr_agregat a ON a.annee = t.annee AND a.siege = t.siege AND a.juridiction = t.juridiction
		                                  AND a.mesure = t.mesure AND a.groupe_profit = 'PANELAI'
		         WHERE t.mesure = 'PROFIT' AND t.groupe_profit = '_T' AND t.siege = 'USA'
		           AND a.valeur < t.valeur - greatest(1e6, 0.005 * abs(t.valeur))`,
	},
	{
		name: "taux légal français de l'IS chargé de 2000 à aujourd'hui",
		query: `SELECT count(*) FROM core.fiscalite_pays
		         WHERE pays = 'FRA' AND indicateur = 'CIT.CIT_C' AND variante = 'ST.S13'`,
		min: 26,
	},
	{
		name:  "revenus d'IDE de la France chargés",
		query: `SELECT count(DISTINCT annee) FROM core.ide_revenu WHERE pays_declarant = 'FRA' AND contrepartie = 'W'`,
		min:   10,
	},
	{
		// Dividendes + bénéfices réinvestis = revenus des actions, à 1 % près.
		name: "IDE : dividendes + bénéfices réinvestis = revenus des actions",
		query: `SELECT count(*) FROM core.ide_revenu e
		          JOIN core.ide_revenu d USING (pays_declarant, annee, contrepartie, direction, type_entite, unite)
		          JOIN core.ide_revenu r USING (pays_declarant, annee, contrepartie, direction, type_entite, unite)
		         WHERE e.composante = 'REVENUS_ACTIONS' AND d.composante = 'DIVIDENDES'
		           AND r.composante = 'BENEFICES_REINVESTIS' AND e.contrepartie = 'W' AND e.unite = 'EUR'
		           AND abs(d.valeur + r.valeur - e.valeur) > greatest(5e6, 0.01 * abs(e.valeur))`,
	},
	{
		// Contrôle national + contrôle étranger = toutes les entreprises.
		name: "FATS : entreprises sous contrôle français + étranger = total",
		query: `SELECT count(*) FROM core.fats_controle w
		          JOIN core.fats_controle d USING (pays_hote, annee, activite, indicateur, serie)
		          JOIN core.fats_controle r USING (pays_hote, annee, activite, indicateur, serie)
		         WHERE w.serie = 'fats_ctrl' AND w.indicateur IN ('ENT_NR','EMP_NR','AV_MEUR')
		           AND w.pays_controle = 'WORLD' AND d.pays_controle = 'DOM' AND r.pays_controle = 'WRL_REST'
		           AND abs(d.valeur + r.valeur - w.valeur) > 0.005 * w.valeur`,
	},
	{
		name:  "estimations Tørsløv-Wier-Zucman : la France de 2015 à 2019",
		query: `SELECT count(*) FROM core.transfert_benefices_estimation WHERE pays = 'France' AND indicateur = 'BENEFICES_TRANSFERES'`,
		min:   5,
	},
	{
		name:  "filiales de groupes étrangers : repérage GLEIF et sélection nommée chargés",
		query: `SELECT count(*) FROM core.filiale_groupe_etranger`,
		min:   1500,
	},
	{
		name: "filiales : la sélection nommée a des comptes publiés pour 90 % des sociétés",
		query: `SELECT CASE WHEN count(*) FILTER (WHERE date_cloture IS NOT NULL) >= 0.9 * count(*) THEN 0 ELSE 1 END
		          FROM derived.filiale_etrangere_comptes WHERE origine = 'SELECTION'`,
	},
	{
		name:  "comptes : aucune filiale française n'est rattachée à une mère française",
		query: `SELECT count(*) FROM core.filiale_groupe_etranger WHERE pays_groupe = 'FR'`,
	},
}
