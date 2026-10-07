package communes

import (
	"fmt"

	"github.com/faits-politiques/faits-politiques/internal/archive"
)

// DownloadTargets liste toutes les URL que dimensionLocale récupère (COG,
// RNE, municipales 2026 et 2020, OFGL communes, BANATIC, comptes des
// régions/départements/groupements, SSMSI) — pour la récupération
// concurrente inter-connecteurs (voir internal/ingest.PrefetchAll, utilisée
// par fpctl build), exactement comme internal/an, internal/senat,
// internal/europe et internal/partis le font déjà.
//
// Ce paquet charge ses 8 sources dans un ordre strictement séquentiel,
// jamais en parallèle (ref.commune est référencé par tout le reste, et
// bulkload.SansContraintesFK prend un verrou ACCESS EXCLUSIVE sur les tables
// RÉFÉRENCÉES — voir dimensionLocale, internal/ingest/ingest.go) : paralléliser
// le CHARGEMENT interbloquerait. Mais rien n'empêche de paralléliser la
// RÉCUPÉRATION, qui ne touche pas la base — seule la mesure le disait : sur un
// run réel, les 9m46 de dimensionLocale se décomposaient en téléchargement ET
// traitement mêlés, jamais isolés l'un de l'autre jusqu'ici. Prérécupérer ici
// fait que chaque arch.Fetch de la boucle séquentielle trouve déjà son
// fichier sur disque (voir archive.WithPrefetched) : ce qui reste séquentiel
// n'est plus que le traitement, jamais plus l'attente réseau.
func DownloadTargets() []archive.DownloadTarget {
	var out []archive.DownloadTarget

	for _, m := range MillesimesCOG {
		out = append(out,
			archive.DownloadTarget{Name: fmt.Sprintf("communes-cog-communes-%d", m.Annee), Source: SourceCOG, URL: m.CommunesURL, Ext: ".csv"},
			archive.DownloadTarget{Name: fmt.Sprintf("communes-cog-mouvements-%d", m.Annee), Source: SourceCOG, URL: m.MvtURL, Ext: ".csv"},
		)
	}

	for i, f := range rneFichiers {
		out = append(out, archive.DownloadTarget{
			Name: fmt.Sprintf("communes-rne-%d", i), Source: SourceRNE, URL: f.url, Ext: ".csv",
		})
	}

	out = append(out,
		archive.DownloadTarget{Name: "communes-municipales-t1", Source: SourceMunicipales, URL: municipalesT1URL, Ext: ".csv"},
		archive.DownloadTarget{Name: "communes-municipales-t2", Source: SourceMunicipales, URL: municipalesT2URL, Ext: ".csv"},
		archive.DownloadTarget{Name: "communes-municipales2020-t1", Source: SourceMunicipales2020, URL: m2020T1URL, Ext: ".txt"},
		archive.DownloadTarget{Name: "communes-municipales2020-t2", Source: SourceMunicipales2020, URL: m2020T2URL, Ext: ".txt"},
	)

	for ex := ofglPremierExercice; ex <= ofglDernierExercice; ex++ {
		out = append(out, archive.DownloadTarget{
			Name: fmt.Sprintf("communes-ofgl-%d", ex), Source: SourceOFGL, URL: ofglURL(ex), Ext: ".csv",
		})
	}

	out = append(out,
		archive.DownloadTarget{Name: "communes-banatic-competences", Source: SourceBANATIC, URL: banaticCompetenceURL, Ext: ".json"},
		archive.DownloadTarget{Name: "communes-banatic-export", Source: SourceBANATIC, URL: banaticExportURL, Ext: ".xlsx"},
		archive.DownloadTarget{Name: "communes-banatic-correspondance-siren", Source: SourceBANATIC, URL: banaticCorrespondanceSirenURL(), Ext: ".csv"},
	)

	for _, n := range niveaux {
		for ex := ofglPremierExercice; ex <= ofglDernierExercice; ex++ {
			out = append(out, archive.DownloadTarget{
				Name: fmt.Sprintf("communes-collectivites-%s-%d", n.niveau, ex), Source: SourceOFGL, URL: collectivitesURL(n, ex), Ext: ".csv",
			})
		}
	}

	// "-fetch" : seule URL de cette source, donc pas de suffixe naturel
	// (contrairement à communes-ofgl-2018 ou communes-collectivites-REGION-
	// 2018) — un nom qui collisionnait avec celui du maillon réel de la
	// chaîne (catalogue.go), prêtant à confusion dans les logs (deux lignes
	// "communes-ssmsi : terminé en" sans rapport l'une avec l'autre, l'une
	// pour la récupération, l'autre pour le traitement).
	out = append(out, archive.DownloadTarget{Name: "communes-ssmsi-fetch", Source: SourceSSMSI, URL: ssmsiURL, Ext: ".csv.gz"})

	return out
}
