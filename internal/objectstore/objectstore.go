// Package objectstore parle à un object storage compatible S3 — MinIO en
// local (voir docker-compose.yml, service minio), un vrai bucket en
// production le jour où c'est tranché (docs/decisions.md D-012 ; voir aussi
// docs/ci-pipeline.md). Deux usages, un seul client : synchroniser l'archive
// scellée (fp-archive) et, à l'étude seulement, une copie du site généré
// (fp-site) — voir cmd/fpctl/sync.go.
//
// Jamais de dépendance à cet objet dans le chemin de service normal :
// aujourd'hui, le site est servi depuis le disque local (site/) et l'archive
// lue depuis raw/. Ce paquet existe pour évaluer l'alternative, pas pour la
// remplacer sans décision explicite.
package objectstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config : les quatre informations nécessaires pour joindre un object
// storage compatible S3. Lue depuis l'environnement (FromEnv) —
// jamais un flag de plus à mémoriser par commande, les mêmes variables
// servent à fpctl provision store, sync archive, sync site et list sources.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// FromEnv lit FP_S3_ENDPOINT / FP_S3_ACCESS_KEY / FP_S3_SECRET_KEY /
// FP_S3_USE_SSL, avec les valeurs par défaut du MinIO local de
// docker-compose.yml (identifiants « fpminio » / « fp12345678 », qui ne
// protègent qu'un service qui n'écoute que sur localhost).
func FromEnv() Config {
	cfg := Config{
		Endpoint:  "localhost:9090",
		AccessKey: "fpminio",
		SecretKey: "fp12345678",
	}
	if v := os.Getenv("FP_S3_ENDPOINT"); v != "" {
		cfg.Endpoint = v
	}
	if v := os.Getenv("FP_S3_ACCESS_KEY"); v != "" {
		cfg.AccessKey = v
	}
	if v := os.Getenv("FP_S3_SECRET_KEY"); v != "" {
		cfg.SecretKey = v
	}
	cfg.UseSSL = os.Getenv("FP_S3_USE_SSL") == "1"
	return cfg
}

// Client ouvre une connexion au object storage décrit par cfg.
func Client(cfg Config) (*minio.Client, error) {
	return minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
}

// EnsureBucket crée le bucket s'il n'existe pas déjà — idempotent, pour que
// « sync » puisse toujours être relancé sans provisionnement préalable
// distinct.
func EnsureBucket(ctx context.Context, c *minio.Client, bucket string) error {
	exists, err := c.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if !exists {
		return c.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
	}
	return nil
}

// SyncDir envoie tous les fichiers de localRoot vers bucket, sous une clé
// égale à leur chemin relatif (les séparateurs Windows n'existent pas ici,
// ce projet ne tourne que sous Linux). Ignore un objet déjà présent à la
// même taille : l'archive est immuable et adressée par empreinte
// (internal/archive), un fichier qui n'a pas changé de taille n'a pas
// changé — cette hypothèse serait fausse pour un contenu muable, mais rien
// de ce que ce paquet synchronise ne l'est.
func SyncDir(ctx context.Context, c *minio.Client, bucket, localRoot string) (sent int, bytes int64, err error) {
	if err := EnsureBucket(ctx, c, bucket); err != nil {
		return 0, 0, err
	}
	err = filepath.WalkDir(localRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(localRoot, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if existing, err := c.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); err == nil && existing.Size == info.Size() {
			return nil // déjà présent, même taille : rien à renvoyer
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = c.PutObject(ctx, bucket, key, f, info.Size(), minio.PutObjectOptions{
			ContentType: contentType(path),
		})
		if err != nil {
			return fmt.Errorf("%s : %w", key, err)
		}
		sent++
		bytes += info.Size()
		return nil
	})
	return sent, bytes, err
}

// Object : ce qu'il faut à internal/sources pour décrire un document
// archivé, que la source réelle soit un répertoire local ou un bucket — les
// deux mêmes trois champs, jamais plus.
type Object struct {
	Key  string
	Size int64
}

// ListPrefix énumère les objets d'un bucket sous un préfixe — utilisé par
// fpctl list sources quand l'archive vit dans l'object storage plutôt que
// sur disque (voir internal/sources, DocumentsDeSource).
func ListPrefix(ctx context.Context, c *minio.Client, bucket, prefix string) ([]Object, error) {
	var out []Object
	for o := range c.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if o.Err != nil {
			return nil, o.Err
		}
		out = append(out, Object{Key: o.Key, Size: o.Size})
	}
	return out, nil
}

func contentType(path string) string {
	switch {
	case strings.HasSuffix(path, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(path, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(path, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(path, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(path, ".xml"):
		return "application/xml; charset=utf-8"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".pdf"):
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
