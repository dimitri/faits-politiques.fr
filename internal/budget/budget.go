// Package budget charge ce que les finances publiques françaises publient
// réellement en données ouvertes : les comptes de la protection sociale, une
// partie de l'exécution budgétaire de l'État, et les allègements de cotisations.
//
// Ce qu'il ne charge pas, faute de source :
//
//   - les tableaux d'équilibre VOTÉS. Une recherche « comptes de la sécurité
//     sociale » sur data.gouv.fr renvoie zéro jeu de données ; le seul jeu
//     rattaché à la loi de financement, les REPSS, est gelé depuis janvier 2022.
//     Les chiffres que le Parlement vote n'existent que dans le texte de loi et
//     dans des PDF. ref.loi_financiere liste donc les textes, et core.solde_vote
//     reste vide jusqu'à ce qu'un connecteur les en extraie.
//   - les comptes d'une branche. La CNAM publie cinquante et un jeux, dont les
//     dépenses par pathologie, mais pas les comptes de la branche maladie.
//     L'URSSAF en publie cent vingt-quatre sur son ACTIVITÉ de recouvrement, pas
//     sur ses comptes. Le circuit est documenté de partout sauf à l'endroit où
//     il se solde.
//
// Les séries de comptabilité nationale — dépenses, recettes et solde par
// sous-secteur — ne sont pas ici mais dans internal/macro, qui interroge déjà
// Eurostat. C'est le seul endroit où l'État et la Sécurité sociale se mesurent
// sur la même règle.
package budget

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/faits-politiques/faits-politiques/internal/archive"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ConnectorVersion = "budget-v1"

// Les portails Opendatasoft refusent `offset + limit > 10 000`, avec un HTTP 400
// explicite. Paginer un jeu de 15 654 lignes échoue donc à mi-parcours — et,
// pire, une pagination mal contrôlée s'arrêterait en silence sur les 10 000
// premières. L'export en un seul appel n'a pas cette limite.
//
// Le piège vaut pour TOUS les portails Opendatasoft, y compris data.caf.fr déjà
// utilisé par internal/macro.
func exportJSON(portail, dataset string) string {
	return "https://" + portail + "/api/explore/v2.1/catalog/datasets/" + dataset + "/exports/json"
}

// lireJSON lit un export et le décode, que les octets scellés soient compressés
// ou non.
//
// Go décompresse de lui-même quand c'est lui qui a demandé gzip, si bien que
// l'archive contient d'ordinaire du JSON en clair. Mais l'archive est faite pour
// être relue dans dix ans, par un programme qui n'aura pas forcément le même
// transport : renifler les deux octets magiques coûte trois lignes et évite un
// « invalid character » incompréhensible sur un fichier pourtant intact.
func lireJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return fmt.Errorf("%s : en-tête gzip mais flux illisible : %w", path, err)
		}
		defer zr.Close()
		if b, err = io.ReadAll(zr); err != nil {
			return fmt.Errorf("%s : décompression : %w", path, err)
		}
	}
	return json.Unmarshal(b, v)
}

// Ingest charge les trois jeux dans l'ordre de leur valeur : la série longue du
// champ social d'abord, l'exécution de l'État ensuite, les allègements enfin.
//
// Chaque chargement ouvre sa propre transaction et remet à zéro SA table. Une
// source qui échoue n'emporte donc pas les deux autres, et chacun est
// idempotent : deux exécutions de suite laissent la base dans le même état.
func Ingest(ctx context.Context, pool *pgxpool.Pool, arch *archive.Archive) error {
	if err := IngestProtectionSociale(ctx, pool, arch); err != nil {
		return err
	}
	if err := IngestExecutionEtat(ctx, pool, arch); err != nil {
		return err
	}
	return IngestURSSAF(ctx, pool, arch)
}

// nulF rend NULL plutôt que zéro pour une valeur absente. Un poste que la source
// ne renseigne pas et un poste à zéro euro sont deux faits différents, et les
// confondre fabriquerait des séries qui plongent là où la donnée manque.
func nulF(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
