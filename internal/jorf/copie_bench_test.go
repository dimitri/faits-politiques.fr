package jorf

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"io"
	"os"
	"strings"
	"testing"
)

// Un échantillon réel, prélevé une fois dans l'archive scellée et gardé en
// mémoire : on mesure le COÛT DE L'ANALYSE, pas celui de la décompression.
var echantillon [][]byte

func charger(tb testing.TB) [][]byte {
	if echantillon != nil {
		return echantillon
	}
	chemin := os.Getenv("JORF_ARCHIVE")
	if chemin == "" {
		tb.Skip("JORF_ARCHIVE non défini")
	}
	fh, err := os.Open(chemin)
	if err != nil {
		tb.Skip(err)
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		tb.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for len(echantillon) < 3000 {
		h, err := tr.Next()
		if err != nil {
			break
		}
		base := h.Name
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[i+1:]
		}
		if h.Typeflag != tar.TypeReg || !strings.HasPrefix(base, "JORFTEXT") {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			tb.Fatal(err)
		}
		echantillon = append(echantillon, b)
	}
	return echantillon
}

func octets(e [][]byte) int64 {
	var n int64
	for _, b := range e {
		n += int64(len(b))
	}
	return n
}

// Le décodage des métadonnées seul.
func BenchmarkMeta(b *testing.B) {
	e := charger(b)
	b.SetBytes(octets(e))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, raw := range e {
			_, _ = decoder(raw)
		}
	}
}

// Le découpage en blocs, qui refait une passe complète sur le même document.
func BenchmarkBlocs(b *testing.B) {
	e := charger(b)
	b.SetBytes(octets(e))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, raw := range e {
			_ = blocs(raw)
		}
	}
}

// Les deux, comme le prototype les enchaîne aujourd'hui.
func BenchmarkMetaEtBlocs(b *testing.B) {
	e := charger(b)
	b.SetBytes(octets(e))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, raw := range e {
			_, _ = decoder(raw)
			_ = blocs(raw)
		}
	}
}

// Le surcoût de string(raw) : une copie complète du document à chaque appel.
// blocs() et decoderDans() la font ; decoder() ne la fait pas.
func BenchmarkCopieInutile(b *testing.B) {
	e := charger(b)
	b.SetBytes(octets(e))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, raw := range e {
			_ = strings.NewReader(string(raw))
		}
	}
}

// Le plancher : tokeniser sans rien construire. C'est ce que coûte
// encoding/xml lui-même.
func BenchmarkTokenisation(b *testing.B) {
	e := charger(b)
	b.SetBytes(octets(e))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, raw := range e {
			d := xml.NewDecoder(bytes.NewReader(raw))
			d.Strict = false
			for {
				if _, err := d.Token(); err != nil {
					break
				}
			}
		}
	}
}
