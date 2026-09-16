package main

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// commandeProvision : fpctl provision db, fpctl provision store. Les deux
// services locaux dont le développement et la CI ont besoin, démarrés par
// docker compose (docker-compose.yml) — jamais une réimplémentation de ce
// que compose fait déjà bien (attendre le healthcheck, réutiliser le
// conteneur existant, etc.).
func commandeProvision() *cobra.Command {
	cmd := &cobra.Command{Use: "provision", Short: "Démarre un service local nécessaire au développement ou à la CI"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "db",
			Short: "Démarre Postgres (docker compose, service db)",
			Long: "Démarre le conteneur Postgres du projet et attend son healthcheck\n" +
				"(jusqu'à 180s au premier démarrage : initdb puis les extensions).\n" +
				"Idempotent — un conteneur déjà en place n'est pas recréé.",
			RunE: func(_ *cobra.Command, args []string) error {
				return dockerComposeUpWait("db", args)
			},
		},
		&cobra.Command{
			Use:   "store",
			Short: "Démarre l'object store local (docker compose, service minio)",
			Long: "Démarre un MinIO local, compatible S3 — API sur :9090, console sur\n" +
				":9091. Sert à fpctl sync archive et fpctl sync site ; identifiants et\n" +
				"point d'accès par défaut lisibles dans internal/objectstore.",
			RunE: func(_ *cobra.Command, args []string) error {
				return dockerComposeUpWait("minio", args)
			},
		},
	)
	return cmd
}

func dockerComposeUpWait(service string, extra []string) error {
	args := append([]string{"compose", "up", "-d", "--wait", service}, extra...)
	cmd := exec.Command("docker", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
