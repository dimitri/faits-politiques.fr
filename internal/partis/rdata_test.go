package partis

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"math"
	"testing"
)

// xdr construit à la main le flux qu'écrirait saveRDS pour une table de deux
// colonnes — un réel et un facteur — dont une cellule est manquante.
type xdr struct{ bytes.Buffer }

func (x *xdr) i(v int32) { _ = binary.Write(x, binary.BigEndian, v) }
func (x *xdr) d(v float64) {
	_ = binary.Write(x, binary.BigEndian, math.Float64bits(v))
}
func (x *xdr) chars(s string) { x.i(9); x.i(int32(len(s))); x.WriteString(s) }
func (x *xdr) strs(ss ...string) {
	x.i(16)
	x.i(int32(len(ss)))
	for _, s := range ss {
		x.chars(s)
	}
}
func (x *xdr) sym(s string) { x.i(1); x.chars(s) }

func rdataFixture() []byte {
	x := &xdr{}
	x.WriteString("RDX3\nX\n")
	x.i(3)
	x.i(0x00040302)
	x.i(0x00030500)
	x.i(5)
	x.WriteString("UTF-8")

	x.i(2 | 1<<10) // pairlist, avec tag
	x.sym("tab")
	x.i(19 | 1<<9) // la table : liste de colonnes, avec attributs
	x.i(2)
	x.i(14) // colonne 1 : réels
	x.i(2)
	x.d(1.5)
	_ = binary.Write(x, binary.BigEndian, uint64(0x7FF00000000007A2)) // NA_real_
	x.i(13 | 1<<9)                                                    // colonne 2 : facteur
	x.i(2)
	x.i(1)
	x.i(int32(math.MinInt32)) // NA
	x.i(2 | 1<<10)            // attributs de la colonne 2 : levels
	x.sym("levels")
	x.strs("FN")
	x.i(254)
	x.i(2 | 1<<10) // attributs de la table : names
	x.sym("names")
	x.strs("vote", "parti")
	x.i(254)
	x.i(254) // fin de la liste de premier niveau

	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	_, _ = zw.Write(x.Bytes())
	_ = zw.Close()
	return out.Bytes()
}

func TestReadRData(t *testing.T) {
	tab, err := ReadRData(bytes.NewReader(rdataFixture()))
	if err != nil {
		t.Fatal(err)
	}
	if tab.Name != "tab" || tab.Rows != 2 {
		t.Fatalf("table %q, %d lignes", tab.Name, tab.Rows)
	}
	if got := tab.Column("vote"); got[0] != "1.5" || got[1] != "" {
		t.Errorf("vote = %q, une cellule manquante doit rester vide, jamais zéro", got)
	}
	if got := tab.Column("parti"); got[0] != "FN" || got[1] != "" {
		t.Errorf("parti = %q", got)
	}
	if tab.Column("absente") != nil {
		t.Error("une colonne inconnue doit renvoyer nil")
	}
}

func TestReadRDataRefuseUnTypeInconnu(t *testing.T) {
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	x := &xdr{}
	x.WriteString("RDX3\nX\n")
	x.i(3)
	x.i(0)
	x.i(0)
	x.i(0)
	x.i(2 | 1<<10)
	x.sym("t")
	x.i(23) // EXTPTRSXP : non géré
	_, _ = zw.Write(x.Bytes())
	_ = zw.Close()
	if _, err := ReadRData(&out); err == nil {
		t.Fatal("un type SEXP inconnu doit faire échouer la lecture")
	}
}
