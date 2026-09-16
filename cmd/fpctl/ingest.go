package main

import "github.com/spf13/cobra"

// commandeIngest enveloppe cmd/ingest tel quel — y compris son option
// -only, qui reste la référence unique pour la liste des sources (une
// soixantaine) : la dupliquer ici en sous-commandes cobra créerait deux
// endroits à tenir synchronisés, l'un d'eux finirait par mentir. Voir
// « fpctl help ingest » pour la liste, ou -only= (vide) pour tout charger.
func commandeIngest() *cobra.Command {
	return &cobra.Command{
		Use:   "ingest [options]",
		Short: "Télécharge, archive et charge les jeux de données sources",
		Long: "Sans -only, recharge tout, dans l'ordre attendu par les dépendances\n" +
			"entre sources. Avec -only=<source> (migrate, checksums, ou l'un des\n" +
			"connecteurs), ne recharge que celle-là — voir « fpctl help ingest ».",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if estDemandeAide(args) {
				return afficherManuel("fpctl-ingest")
			}
			return execBinaire("fpingest", "cmd/ingest", args)
		},
	}
}
