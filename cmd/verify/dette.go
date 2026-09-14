package main

// Contrôles du bloc dette (docs/dette-donnees.md), dans un fichier à part :
// ils se lisent ensemble, parce que la plupart croisent deux sources ou deux
// ventilations d'une même source. Une identité comptable qui ne tient plus
// signale presque toujours une série mal qualifiée au chargement — un
// multiplicateur oublié, une échéance rangée du mauvais côté.
func init() {
	checks = append(checks, checksDette...)
}

// Vue commode sur les observations, reprise dans chaque requête.
const detteObs = `(SELECT s.code, s.pays, s.concept, s.mesure, s.unite, s.zone_detenteur,
                          s.secteur_detenteur, s.echeance, s.instrument, s.source_id,
                          o.periode, o.debut, o.valeur
                   FROM core.dette_observation o JOIN ref.dette_serie s ON s.code = o.serie)`

var checksDette = []check{
	{
		name: "les séries de dette sont chargées pour chaque source",
		query: `SELECT count(*) FROM raw.source r
		         WHERE r.slug IN ('insee-dette','eurostat-dette','fmi-weo-dette','aff-statistique-financiere',
		                          'bns-rendements-obligataires','aft-p117-performance','bdf-webstat-det2')
		           AND (SELECT count(*) FROM ref.dette_serie s WHERE s.source_id = r.id) > 0`,
		min: 7,
	},
	{
		name:  "des observations de dette sont chargées",
		query: `SELECT count(*) FROM core.dette_observation`,
		min:   40000,
	},
	{
		// L'AFT publie un total et ses ventilations ; l'INSEE les republie en
		// millions d'euros. Écart toléré : un million, l'arrondi de publication.
		name: "dette négociable de l'État : court terme + long terme = total",
		query: `SELECT count(*) FROM ` + detteObs + ` t
		         JOIN ` + detteObs + ` ct ON ct.periode = t.periode AND ct.code = 'insee:001711532'
		         JOIN ` + detteObs + ` lt ON lt.periode = t.periode AND lt.code = 'insee:001711533'
		         JOIN ` + detteObs + ` dv ON dv.periode = t.periode AND dv.code = 'insee:001719708'
		        WHERE t.code = 'insee:001739081'
		          AND abs(ct.valeur + lt.valeur + dv.valeur - t.valeur) > 1e6`,
	},
	{
		// Une exception, documentée et laissée telle quelle en base (on ne
		// corrige pas une source) : en octobre 2017, la ventilation publiée
		// fixe + indexée vaut exactement le TOTAL DE SEPTEMBRE (1 703 850 M€),
		// 23,7 Md€ au-dessus du total d'octobre. Le mois précédent a été
		// reporté dans la ventilation ; le total, lui, est cohérent avec la
		// série CT + LT. Voir docs/dette-donnees.md.
		name: "dette négociable de l'État : taux fixe + indexée = total",
		query: `SELECT count(*) FROM ` + detteObs + ` t
		         JOIN ` + detteObs + ` f ON f.periode = t.periode AND f.code = 'insee:001738853'
		         JOIN ` + detteObs + ` i ON i.periode = t.periode AND i.code = 'insee:001738854'
		        WHERE t.code = 'insee:001739081' AND abs(f.valeur + i.valeur - t.valeur) > 1e6
		          AND t.periode <> '2017-10'`,
	},
	{
		// Publiée en milliards à une décimale : trois arrondis cumulés peuvent
		// atteindre 0,15 Md€.
		name: "dette Maastricht trimestrielle : dépôts + titres + crédits = total",
		query: `SELECT count(*) FROM ` + detteObs + ` t
		         JOIN ` + detteObs + ` a ON a.periode = t.periode AND a.code = 'insee:010777606'
		         JOIN ` + detteObs + ` b ON b.periode = t.periode AND b.code = 'insee:010777624'
		         JOIN ` + detteObs + ` c ON c.periode = t.periode AND c.code = 'insee:010777607'
		        WHERE t.code = 'insee:010777616' AND abs(a.valeur + b.valeur + c.valeur - t.valeur) > 0.2e9`,
	},
	{
		// Contributions CONSOLIDÉES : elles se somment au total. Si ce contrôle
		// échoue, c'est qu'une série non consolidée s'est glissée à la place.
		name: "dette Maastricht trimestrielle : les quatre sous-secteurs se somment au total",
		query: `SELECT count(*) FROM ` + detteObs + ` t
		         JOIN (SELECT periode, sum(valeur) v, count(*) n FROM ` + detteObs + ` x
		                WHERE code IN ('insee:010777610','insee:010777613','insee:010777626','insee:010777625')
		                GROUP BY periode) s ON s.periode = t.periode
		        WHERE t.code = 'insee:010777616' AND (s.n <> 4 OR abs(s.v - t.valeur) > 0.2e9)`,
	},
	{
		// Même dette, deux producteurs : l'INSEE au quatrième trimestre,
		// Eurostat en fin d'année. Un écart signalerait un millésime décalé
		// ou un multiplicateur mal appliqué. Avant 1998, les deux producteurs
		// divergent de plusieurs milliards (−13 Md€ en 1995, +6 Md€ en 1997) :
		// la rétropolation trimestrielle de l'INSEE en base 2020 ne reprend pas
		// les mêmes sources que la série annuelle transmise à Eurostat pour ces
		// années-là. Comparaison limitée à 1998 et après.
		name: "dette Maastricht de la France : INSEE (T4) et Eurostat (annuel) concordent",
		query: `SELECT count(*) FROM ` + detteObs + ` i
		         JOIN ` + detteObs + ` e ON e.code = 'eurostat:gov_10dd_edpt1:FR:MIO_EUR:S13:GD'
		                                AND e.periode = left(i.periode, 4)
		        WHERE i.code = 'insee:010777616' AND i.periode LIKE '%-Q4' AND i.periode >= '1998'
		          AND abs(i.valeur - e.valeur) > 1e9`,
	},
	{
		name: "Eurostat : détention résidente + non résidente = dette totale",
		query: `SELECT count(*) FROM ` + detteObs + ` t
		         JOIN ` + detteObs + ` r ON r.pays = t.pays AND r.periode = t.periode AND r.unite = t.unite
		              AND r.code LIKE 'eurostat:gov_10dd_ggd:%' AND r.zone_detenteur = 'W2'
		              AND r.secteur_detenteur = '_T' AND r.echeance = '_T'
		         JOIN ` + detteObs + ` n ON n.pays = t.pays AND n.periode = t.periode AND n.unite = t.unite
		              AND n.code LIKE 'eurostat:gov_10dd_ggd:%' AND n.zone_detenteur = 'W1' AND n.echeance = '_T'
		        WHERE t.code LIKE 'eurostat:gov_10dd_ggd:%' AND t.zone_detenteur = 'W0' AND t.echeance = '_T'
		          AND abs(r.valeur + n.valeur - t.valeur) > CASE WHEN t.unite = 'EUR' THEN 2e6 ELSE 0.15 END`,
	},
	{
		// Banque de France : les secteurs résidents FEUILLES reconstituent le
		// total résident à long terme. Un agrégat compté comme une feuille
		// ferait doubler une part.
		name: "détention des titres de l'État : les secteurs résidents se somment au total",
		query: `SELECT count(*) FROM ` + detteObs + ` t
		         JOIN (SELECT periode, sum(valeur) v FROM ` + detteObs + ` x
		                WHERE concept = 'DETENTION_TITRES_ETAT' AND mesure = 'ENCOURS' AND zone_detenteur = 'W2'
		                  AND echeance = 'LT' AND instrument = '_T'
		                  AND secteur_detenteur IN ('S11','S121','S122','S123','S124','S125','S126','S127',
		                                            'S128','S129','S13','S14','S15','_Z')
		                GROUP BY periode) s ON s.periode = t.periode
		        WHERE t.concept = 'DETENTION_TITRES_ETAT' AND t.mesure = 'ENCOURS' AND t.zone_detenteur = 'W2'
		          AND t.secteur_detenteur = '_T' AND t.echeance = 'LT' AND t.instrument = '_T'
		          AND abs(s.v - t.valeur) > 1e5`,
	},
	{
		name: "détention des titres de l'État : les parts par catégorie font 100 %",
		query: `SELECT count(*) FROM (SELECT periode, sum(part_pct) p FROM derived.dette_detention_etat
		                           GROUP BY periode) x WHERE abs(p - 100) > 0.1`,
	},
	{
		// La Banque de France publie aussi la part des non-résidents en % ;
		// la recalculer depuis les encours doit redonner le même chiffre.
		name: "détention des titres de l'État : la part non résidente recalculée égale la part publiée",
		query: `SELECT count(*) FROM derived.dette_detention_etat d
		         JOIN ` + detteObs + ` p ON p.periode = d.periode
		              AND p.code = 'bdf:Q.N.FR.W1.S13111.S1.N.L.LE.F3.T._Z.PT._T.M.V.N._T'
		        WHERE d.categorie = 'NON_RESIDENTS' AND abs(d.part_pct - p.valeur) > 0.1`,
	},
	{
		// Un taux apparent hors de 0-15 % trahit un encours ou des intérêts
		// dans la mauvaise unité (millions pris pour des euros).
		name:  "le taux apparent de la dette reste dans des bornes plausibles",
		query: `SELECT count(*) FROM derived.dette_taux_apparent WHERE taux_apparent_pct NOT BETWEEN 0 AND 15`,
	},
	{
		// Identité du compte de capital : besoin de financement = épargne
		// brute négative + investissement + transferts en capital nets. Les
		// cinq termes sont publiés au dixième de million : 1,5 M€ de tolérance.
		name:  "compte de capital : le besoin de financement se décompose exactement",
		query: `SELECT count(*) FROM derived.dette_compte_capital WHERE abs(ecart_identite) > 1.5e6`,
	},
	{
		// La dépense par nature doit redonner la dépense totale : une opération
		// oubliée ou comptée deux fois fausserait toute répartition affichée.
		name: "dépense publique par nature : la somme des opérations égale la dépense totale",
		query: `SELECT count(*) FROM (
		          SELECT s.pays, split_part(s.code, ':', 5) AS secteur, o.periode,
		                 sum(o.valeur) FILTER (WHERE s.instrument IN ('P2','D1PAY','D29PAY','D3PAY','D4PAY',
		                   'D5PAY','D62PAY','D632PAY','D7PAY','D8','D9PAY','P5','NP')) AS somme,
		                 max(o.valeur) FILTER (WHERE s.code LIKE '%:TE') AS te
		          FROM core.dette_observation o JOIN ref.dette_serie s ON s.code = o.serie
		          WHERE s.code LIKE 'eurostat:gov_10a_main:FR:MIO_EUR:%'
		          GROUP BY 1, 2, 3) x
		        WHERE te IS NOT NULL AND abs(somme - te) > 2e6`,
	},
	{
		name:  "dépenses fiscales : les quatre millésimes sont chargés",
		query: `SELECT count(DISTINCT millesime) FROM core.depense_fiscale`,
		min:   4,
	},
	{
		// Un total annuel exécuté hors de 60-130 Md€ trahirait une erreur
		// d'unité (millions pris pour des euros) ou des lignes perdues.
		name: "dépenses fiscales : les totaux exécutés restent dans des bornes plausibles",
		query: `SELECT count(*) FROM (SELECT millesime, annee, sum(montant_eur) t FROM core.depense_fiscale
		                           WHERE stade = 'EXECUTION' GROUP BY 1, 2) x
		        WHERE t NOT BETWEEN 60e9 AND 130e9`,
	},
	{
		// La nature du bénéficiaire ne vient que du PLF 2023. Si les mesures
		// non classées dépassent 5 % du total d'une année, la répartition
		// ménages / entreprises affichée cesse d'être représentative.
		name: "dépenses fiscales : moins de 5 % du montant sans nature de bénéficiaire",
		query: `SELECT count(*) FROM (SELECT annee,
		                                  sum(montant_eur) FILTER (WHERE nature = 'NON_CLASSEE') nc,
		                                  sum(montant_eur) t
		                           FROM derived.depense_fiscale_retenue GROUP BY annee) x
		        WHERE coalesce(nc, 0) > 0.05 * t`,
	},
	{
		// Les séries révisées les plus suivies doivent être fraîches : la
		// dette négociable est mensuelle, la détention trimestrielle (publiée
		// avec environ un trimestre de retard).
		name: "dette négociable et détention sont à jour",
		query: `SELECT count(*) FROM (VALUES
		          ('insee:001739081', interval '5 months'),
		          ('bdf:Q.N.FR.W1.S13111.S1.N.L.LE.F3.T._Z.PT._T.M.V.N._T', interval '10 months'),
		          ('insee:010777616', interval '10 months')) c(code, delai)
		        WHERE (SELECT max(debut) FROM core.dette_observation WHERE serie = c.code)
		              > current_date - delai`,
		min: 3,
	},
}
