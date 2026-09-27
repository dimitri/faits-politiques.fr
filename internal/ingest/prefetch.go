package ingest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/faits-politiques/faits-politiques/internal/pipeline"
)

// PrefetchAll récupère tous les targets d'un coup, en une seule vague sans
// dépendances entre elles (internal/pipeline, comme le reste du graphe
// d'ingestion) : le but est de payer la latence réseau UNE fois, au maximum
// de front possible, plutôt qu'un connecteur après l'autre au fil du graphe
// de dépendances qui les enchaîne normalement — télécharger les scrutins de
// l'Assemblée n'a jamais eu besoin d'attendre que les comptes de campagne
// soient récupérés en premier, ce n'est qu'un ordre hérité de l'écriture du
// code, pas une vraie dépendance de données.
//
// Chaque téléchargement reste attribué à SA vraie source
// (EnsureSource/StartRun/EndRun, exactement comme le Fetch qu'un connecteur
// ferait lui-même) : ce qui change n'est que LE MOMENT où il a lieu, jamais
// sa comptabilité dans raw.source/raw.fetch_run/raw.retrieval. Le graphe
// construit ici n'est PAS publié (Registre.Publier) : ce sont des cibles
// jetables pour la durée d'une commande, pas des étapes nommées que
// fpctl list deps doit connaître.
//
// Le résultat, glissé dans le contexte renvoyé via archive.WithPrefetched,
// fait que chaque connecteur qui récupère ensuite CES MÊMES URL (an.Download,
// senat.Ingest, europe.Ingest, partis.IngestComptes/IngestPopuList/IngestCHES)
// retrouve directement ce qui vient d'être pris, sans repasser par le réseau.
func PrefetchAll(ctx context.Context, arch *archive.Archive,
	targets []archive.DownloadTarget, concurrence int) (context.Context, error) {

	if len(targets) == 0 {
		return ctx, nil
	}
	logs.Notice("prefetching " + logs.Plural(len(targets), "file") + " (all connectors, in parallel)")

	reg := pipeline.NouveauRegistre(nil)
	for _, t := range targets {
		t := t
		reg.Ajouter(pipeline.Etape{
			Nom: t.Nom, Description: "prefetching " + filenameOf(t.URL),
			Executer: func(ctx context.Context, _ pipeline.Results) (any, error) {
				srcID, err := arch.EnsureSource(ctx, t.Source)
				if err != nil {
					return nil, err
				}
				runID, err := arch.StartRun(ctx, srcID, "prefetch")
				if err != nil {
					return nil, err
				}
				f, err := arch.Fetch(ctx, srcID, runID, t.URL, t.Ext)
				if err != nil {
					arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
					return nil, fmt.Errorf("%s : %w", t.Nom, err)
				}
				arch.EndRun(ctx, runID, "SUCCESS", map[string]any{"sha256": f.SHA256}, "")
				return f, nil
			},
		})
	}
	resultats, err := reg.Executer(ctx, reg.Noms(), pipeline.Options{Concurrence: concurrence})
	if err != nil {
		return ctx, err
	}

	byURL := make(map[string]*archive.Fetched, len(targets))
	for _, t := range targets {
		f, ok := resultats[t.Nom].(*archive.Fetched)
		if !ok {
			continue
		}
		byURL[t.URL] = f
	}

	// Le rapport vient APRÈS la vague entière, pas fichier par fichier : les
	// NOTICE de téléchargement de archive.go (une paire par fichier, URL
	// complète) narrent déjà chaque récupération individuelle pendant
	// qu'elle a lieu ; celui-ci est le résumé qu'on relit une fois que tout
	// est arrivé, dans l'ordre des targets plutôt que dans l'ordre
	// d'arrivée (non déterministe) des goroutines.
	for _, t := range targets {
		f, ok := byURL[t.URL]
		if !ok {
			continue
		}
		taille := int64(0)
		if info, err := os.Stat(f.Path); err == nil {
			taille = info.Size()
		}
		logs.Notice(fmt.Sprintf("%s: %s, sha256 %s", filenameOf(t.URL), tailleLisiblePrefetch(taille), f.SHA256[:12]))
	}

	return archive.WithPrefetched(ctx, byURL), nil
}

// filenameOf isole le nom de fichier d'une URL, pour un rapport lisible :
// "AMO30_tous_acteurs_tous_mandats_tous_organes_historique.json.zip", jamais
// l'URL entière avec son chemin de dépôt.
func filenameOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return path.Base(u.Path)
}

// tailleLisiblePrefetch : même échelle (KB/MB) que internal/archive et
// cmd/fpctl/list.go, dupliquée plutôt que partagée — voir le commentaire de
// internal/archive.tailleLisible sur ce choix.
func tailleLisiblePrefetch(octets int64) string {
	const unite = 1024.0
	v := float64(octets)
	for _, suffixe := range []string{"B", "KB", "MB", "GB", "TB"} {
		if v < unite {
			return fmt.Sprintf("%.1f %s", v, suffixe)
		}
		v /= unite
	}
	return fmt.Sprintf("%.1f PB", v)
}
