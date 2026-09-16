package main

import (
	"embed"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// Pages de manuel : sources en Markdown dans man/*.md, rendues en troff par
// pandoc (make man, voir man/generate.go) et gravées dans le binaire à la
// compilation — jamais regénérées à l'exécution. Un poste qui fait tourner
// fpctl n'a besoin que d'un afficheur de pages de manuel (man, mandoc,
// groff — déjà présents sur toute machine Unix courante) ; pandoc ne sert
// qu'à qui modifie la documentation, comme git ne demande AsciiDoc que pour
// reconstruire ses propres pages de manuel, jamais pour les lire.
//
//go:embed man/*.1
var pagesManuel embed.FS

func commandeHelp() *cobra.Command {
	return &cobra.Command{
		Use:   "help [commande]",
		Short: "Affiche la page de manuel de fpctl ou d'une de ses commandes",
		Long: "Sans argument, affiche le manuel général de fpctl. Avec un verbe\n" +
			"(build, ingest, verify, list, generate, provision, sync), affiche sa\n" +
			"page de manuel dédiée — exactement ce que fait « git help <commande> ».",
		ValidArgs: []string{"build", "ingest", "verify", "list", "generate", "provision", "sync"},
		Args:      cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			page := "fpctl"
			if len(args) == 1 {
				page = "fpctl-" + args[0]
			}
			return afficherManuel(page)
		},
	}
}

// afficherManuel extrait la page embarquée vers un fichier temporaire puis
// appelle l'afficheur système (man -l lit un fichier local sans exiger qu'il
// soit installé dans une MANPATH) — le même rendu, le même pagineur, les
// mêmes recherches (/) qu'une vraie page de manuel installée.
func afficherManuel(page string) error {
	contenu, err := pagesManuel.ReadFile("man/" + page + ".1")
	if err != nil {
		return fmt.Errorf("pas de page de manuel pour %q (essayez : fpctl help)", page)
	}
	f, err := os.CreateTemp("", page+"-*.1")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(contenu); err != nil {
		f.Close()
		return err
	}
	f.Close()

	cmd := exec.Command("man", "-l", f.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
