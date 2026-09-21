// Package an ingère l'open data de l'Assemblée nationale.
//
// Chaîne : téléchargement -> archive scellée -> raw.record -> core.
// Chaque étape est idempotente : rejouer l'ingestion doit produire un état
// identique (db/README.md, invariant de reconstructibilité).
package an

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/faits-politiques/faits-politiques/internal/logs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "an/1"

const Base = "https://data.assemblee-nationale.fr/static/openData/repository/17"

// BaseLegislature donne l'adresse du dépôt d'une législature antérieure.
func BaseLegislature(n int) string {
	return "https://data.assemblee-nationale.fr/static/openData/repository/" +
		strconv.Itoa(n)
}

var Sources = map[string]archive.Source{
	"an-amo": {
		Slug: "an-amo", Label: "AN — Tous acteurs, mandats et organes (AMO30)",
		Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
		Licence: "Licence Ouverte", ReuseClass: "ATTRIBUTION",
		Attribution: "Source : Assemblée nationale, open data",
		Cadence:     "continue, non contractuelle",
		Notes:       "Un champ peut être un objet ou un tableau selon le nombre d'éléments.",
	},
	"an-amo-16": {
		Slug: "an-amo-16", Label: "AN — Tous acteurs, mandats et organes (16e législature)",
		Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
		Licence: "Licence Ouverte", ReuseClass: "ATTRIBUTION",
		Attribution: "Source : Assemblée nationale, open data",
		Cadence:     "close",
		Notes:       "Publication propre à la 16e législature (2022-2024).",
	},
	"an-amo-15": {
		Slug: "an-amo-15", Label: "AN — Tous acteurs, mandats et organes (15e législature)",
		Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
		Licence: "Licence Ouverte", ReuseClass: "ATTRIBUTION",
		Attribution: "Source : Assemblée nationale, open data",
		Cadence:     "close",
		Notes:       "Publication propre à la 15e législature (2017-2022).",
	},
	"an-dossiers": {
		Slug: "an-dossiers", Label: "AN — Dossiers législatifs (17e législature)",
		Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
		Licence: "Licence Ouverte", ReuseClass: "ATTRIBUTION",
		Attribution: "Source : Assemblée nationale, open data",
		Cadence:     "continue",
		Notes: "Les exposés des motifs ne figurent PAS dans le JSON : seuls titres, " +
			"auteurs, dates et étapes y sont. Un résumé rédigé ne peut donc pas en être " +
			"tiré mécaniquement.",
	},
	"an-scrutins": {
		Slug: "an-scrutins", Label: "AN — Scrutins publics (17e législature)",
		Publisher: "Assemblée nationale", Tier: "PRIMARY_OFFICIAL",
		Licence: "Licence Ouverte", ReuseClass: "ATTRIBUTION",
		Attribution: "Source : Assemblée nationale, open data",
		Cadence:     "par séance",
		Notes: "Ne couvre que les scrutins PUBLICS : la majorité des votes ont lieu " +
			"à main levée et ne laissent aucune trace nominative.",
	},
}

// DownloadTargets liste les URL que Download récupère, sans les récupérer :
// seule source de vérité pour Download elle-même et pour la récupération
// concurrente inter-connecteurs (voir internal/ingest.PrefetchAll, utilisée
// par fpctl build), pour que les deux ne puissent pas diverger.
func DownloadTargets() []archive.DownloadTarget {
	return []archive.DownloadTarget{
		// AMO30 « tous acteurs, tous mandats, tous organes » et non AMO40
		// (« députés actifs ») ni AMO50 : ces deux derniers omettent des
		// députés ayant siégé puis quitté leur siège en cours de
		// législature. Leurs votes figurent pourtant dans les scrutins, et
		// charger sans eux ferait disparaître silencieusement des milliers
		// de votes — c'est-à-dire lire une absence de données comme une
		// absence d'action. Le contrôle de concordance de cmd/verify est
		// précisément là pour empêcher qu'une telle erreur soit publiée.
		{Nom: "an-amo", Source: Sources["an-amo"], Ext: ".zip",
			URL: Base + "/amo/tous_acteurs_mandats_organes_xi_legislature/AMO30_tous_acteurs_tous_mandats_tous_organes_historique.json.zip"},
		// Les législatures antérieures ont leur propre publication. Le
		// fichier de la 17e contient bien 3 121 acteurs, mais l'historique
		// de MANDATS qu'il porte ne couvre que ceux de la législature en
		// cours : nous avions 641 mandats de député pour 3 121 personnes.
		// Charger aussi les 15e et 16e fait remonter la couverture à 2017.
		// Au-delà, le dépôt de l'Assemblée répond 404 : les législatures 14
		// et antérieures ne sont pas publiées en open data. C'est une
		// limite de la source, pas du chargement.
		{Nom: "an-amo-16", Source: Sources["an-amo-16"], Ext: ".zip",
			URL: BaseLegislature(16) + "/amo/tous_acteurs_mandats_organes_xi_legislature/AMO30_tous_acteurs_tous_mandats_tous_organes_historique.json.zip"},
		{Nom: "an-amo-15", Source: Sources["an-amo-15"], Ext: ".zip",
			URL: BaseLegislature(15) + "/amo/tous_acteurs_mandats_organes_xi_legislature/AMO30_tous_acteurs_tous_mandats_tous_organes_historique.json.zip"},
		{Nom: "an-scrutins", Source: Sources["an-scrutins"], Ext: ".zip",
			URL: Base + "/loi/scrutins/Scrutins.json.zip"},
		{Nom: "an-dossiers", Source: Sources["an-dossiers"], Ext: ".zip",
			URL: Base + "/loi/dossiers_legislatifs/Dossiers_Legislatifs.json.zip"},
	}
}

// Download récupère les archives et les scelle. Séquentiel : quand un appelant
// a déjà récupéré ces mêmes URL de front (fpctl build, voir
// internal/ingest.PrefetchAll et archive.WithPrefetched), chaque Fetch ici
// retrouve directement ce qui a été pris, sans repasser par le réseau.
func Download(ctx context.Context, arch *archive.Archive) (map[string]*archive.Fetched, error) {
	out := map[string]*archive.Fetched{}
	for _, t := range DownloadTargets() {
		srcID, err := arch.EnsureSource(ctx, t.Source)
		if err != nil {
			return nil, err
		}
		runID, err := arch.StartRun(ctx, srcID, ConnectorVersion)
		if err != nil {
			return nil, err
		}
		f, err := arch.Fetch(ctx, srcID, runID, t.URL, t.Ext)
		if err != nil {
			arch.EndRun(ctx, runID, "FAILED", nil, err.Error())
			return nil, fmt.Errorf("%s : %w", t.Nom, err)
		}
		arch.EndRun(ctx, runID, "SUCCESS",
			map[string]any{"sha256": f.SHA256, "deja_archive": f.Cached}, "")
		// archive.go a déjà noté l'URL/la taille/l'état au NOTICE juste au-dessus
		// (téléchargement/téléchargé) — ceci associe ce même résultat au nom de
		// source par lequel le reste du catalogue le connaît.
		etat := "archived"
		if f.Cached {
			etat = "unchanged"
		}
		logs.Notice(fmt.Sprintf("source %s %s, sha256 %s", t.Nom, etat, f.SHA256[:12]))
		out[t.Nom] = f
	}
	return out, nil
}

// Extract déplie une archive zip vers raw.record. Le type d'enregistrement est
// déduit du répertoire, la clé naturelle de l'identifiant officiel.
func Extract(ctx context.Context, pool *pgxpool.Pool, f *archive.Fetched) (int, error) {
	var existing int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM raw.record WHERE document_id = $1`, f.DocumentID).Scan(&existing); err != nil {
		return 0, err
	}
	if existing > 0 {
		return existing, nil // déjà déplié : l'extraction est une fonction pure du document
	}

	zr, err := zip.OpenReader(f.Path)
	if err != nil {
		return 0, err
	}
	defer zr.Close()

	type row struct {
		recType string
		key     string
		payload string
	}
	var rows []row

	for _, e := range zr.File {
		if e.FileInfo().IsDir() || !strings.HasSuffix(e.Name, ".json") {
			continue
		}
		rc, err := e.Open()
		if err != nil {
			return 0, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return 0, err
		}

		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(b, &wrapper); err != nil {
			continue // fichier non conforme : ignoré, jamais deviné
		}
		for k, v := range wrapper {
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(v, &obj); err != nil {
				continue
			}
			uid := str(obj["uid"])
			if uid == "" {
				continue
			}
			rows = append(rows, row{recType: "an." + k, key: uid, payload: string(v)})
		}
	}

	n, err := pool.CopyFrom(ctx,
		pgx.Identifier{"raw", "record"},
		[]string{"document_id", "record_type", "natural_key", "payload"},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			return []any{f.DocumentID, rows[i].recType, rows[i].key, rows[i].payload}, nil
		}))
	return int(n), err
}
