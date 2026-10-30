package partis

import (
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Lecteur minimal d'un fichier .RData (sérialisation R, XDR, versions 2 et 3).
//
// Le jeu V-Party n'est publié, sans formulaire d'inscription, que dans ce format :
// le dépôt vdemdata de l'institut V-Dem (data/vparty.RData). Seul ce dont une
// table R (data.frame) a besoin est lu — listes, vecteurs d'entiers, de réels et
// de chaînes, attributs, facteurs. Un type que le lecteur ne connaît pas fait
// échouer la lecture avec son numéro, plutôt que d'être sauté : une table lue de
// travers serait pire qu'une table absente.

// rObj : un objet R. Seul le champ qui correspond à t est renseigné.
type rObj struct {
	t     int
	ints  []int32 // INTSXP, LGLSXP ; naInt marque NA
	reals []float64
	strs  []string // STRSXP ; naStr marque NA
	list  []*rObj  // VECSXP
	pairs []rPair  // LISTSXP
	attrs map[string]*rObj
}

type rPair struct {
	tag string
	val *rObj
}

const (
	naInt int32 = math.MinInt32
	naStr       = "\x00NA\x00"
)

type rReader struct {
	b    []byte
	i    int
	refs []string
}

func (r *rReader) int() (int32, error) {
	if r.i+4 > len(r.b) {
		return 0, io.ErrUnexpectedEOF
	}
	v := int32(binary.BigEndian.Uint32(r.b[r.i:]))
	r.i += 4
	return v, nil
}

func (r *rReader) length() (int, error) {
	n, err := r.int()
	if err != nil {
		return 0, err
	}
	if n != -1 {
		return int(n), nil
	}
	hi, err := r.int()
	if err != nil {
		return 0, err
	}
	lo, err := r.int()
	if err != nil {
		return 0, err
	}
	return int(hi)<<32 | int(uint32(lo)), nil
}

func (r *rReader) bytes(n int) ([]byte, error) {
	if n < 0 || r.i+n > len(r.b) {
		return nil, io.ErrUnexpectedEOF
	}
	s := r.b[r.i : r.i+n]
	r.i += n
	return s, nil
}

// item lit le prochain objet. Un CHARSXP et un symbole reviennent comme un rObj
// de type 9 (strs à un élément).
func (r *rReader) item() (*rObj, error) {
	flags, err := r.int()
	if err != nil {
		return nil, err
	}
	t := int(flags & 0xFF)
	hasAttr := flags&(1<<9) != 0
	hasTag := flags&(1<<10) != 0
	switch t {
	case 254, 253, 252, 251, 250, 242, 241, 0, 5: // NULL, environnements globaux, argument manquant…
		return nil, nil
	case 255: // REFSXP
		idx := int(flags >> 8)
		if idx == 0 {
			v, err := r.int()
			if err != nil {
				return nil, err
			}
			idx = int(v)
		}
		if idx < 1 || idx > len(r.refs) {
			return nil, fmt.Errorf("référence R %d hors limites", idx)
		}
		return &rObj{t: 9, strs: []string{r.refs[idx-1]}}, nil
	case 1: // SYMSXP
		name, err := r.item()
		if err != nil {
			return nil, err
		}
		r.refs = append(r.refs, name.strs[0])
		return name, nil
	case 9: // CHARSXP
		n, err := r.int()
		if err != nil {
			return nil, err
		}
		if n == -1 {
			return &rObj{t: 9, strs: []string{naStr}}, nil
		}
		s, err := r.bytes(int(n))
		if err != nil {
			return nil, err
		}
		return &rObj{t: 9, strs: []string{string(s)}}, nil
	case 2, 6: // LISTSXP, LANGSXP
		if hasAttr {
			if _, err := r.item(); err != nil {
				return nil, err
			}
		}
		tag := ""
		if hasTag {
			o, err := r.item()
			if err != nil {
				return nil, err
			}
			if o != nil {
				tag = o.strs[0]
			}
		}
		car, err := r.item()
		if err != nil {
			return nil, err
		}
		cdr, err := r.item()
		if err != nil {
			return nil, err
		}
		out := &rObj{t: 2, pairs: []rPair{{tag, car}}}
		if cdr != nil {
			out.pairs = append(out.pairs, cdr.pairs...)
		}
		return out, nil
	case 10, 13, 14, 16, 19: // LGLSXP, INTSXP, REALSXP, STRSXP, VECSXP
		n, err := r.length()
		if err != nil {
			return nil, err
		}
		o := &rObj{t: t}
		switch t {
		case 10, 13:
			raw, err := r.bytes(4 * n)
			if err != nil {
				return nil, err
			}
			o.ints = make([]int32, n)
			for k := range o.ints {
				o.ints[k] = int32(binary.BigEndian.Uint32(raw[4*k:]))
			}
		case 14:
			raw, err := r.bytes(8 * n)
			if err != nil {
				return nil, err
			}
			o.reals = make([]float64, n)
			for k := range o.reals {
				o.reals[k] = math.Float64frombits(binary.BigEndian.Uint64(raw[8*k:]))
			}
		case 16:
			o.strs = make([]string, n)
			for k := range o.strs {
				c, err := r.item()
				if err != nil {
					return nil, err
				}
				o.strs[k] = c.strs[0]
			}
		case 19:
			o.list = make([]*rObj, n)
			for k := range o.list {
				if o.list[k], err = r.item(); err != nil {
					return nil, err
				}
			}
		}
		if hasAttr {
			a, err := r.item()
			if err != nil {
				return nil, err
			}
			o.attrs = map[string]*rObj{}
			for _, p := range a.pairs {
				o.attrs[p.tag] = p.val
			}
		}
		return o, nil
	}
	return nil, fmt.Errorf("type SEXP %d à l'octet %d : non géré", t, r.i)
}

// RTable : une table R lue, colonne par colonne. Une cellule manquante est "" ;
// une colonne de facteurs est restituée par ses niveaux.
type RTable struct {
	Name    string
	Rows    int
	columns map[string][]string
}

// Column renvoie la colonne nommée, nil si elle n'existe pas.
func (t *RTable) Column(name string) []string { return t.columns[name] }

// ReadRData lit la première table (data.frame) d'un fichier .RData compressé en gzip.
func ReadRData(src io.Reader) (*RTable, error) {
	zr, err := gzip.NewReader(src)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	if len(b) < 14 || (string(b[:5]) != "RDX3\n" && string(b[:5]) != "RDX2\n") || string(b[5:7]) != "X\n" {
		return nil, fmt.Errorf("pas un fichier RData sérialisé en XDR")
	}
	r := &rReader{b: b, i: 7}
	version, err := r.int() // version de la sérialisation
	if err != nil {
		return nil, err
	}
	if _, err := r.int(); err != nil { // version de R qui l'a écrit
		return nil, err
	}
	if _, err := r.int(); err != nil { // version minimale de R pour le relire
		return nil, err
	}
	if version == 3 {
		n, err := r.int() // encodage natif
		if err != nil {
			return nil, err
		}
		if _, err := r.bytes(int(n)); err != nil {
			return nil, err
		}
	}
	top, err := r.item()
	if err != nil {
		return nil, err
	}
	if top == nil || len(top.pairs) == 0 {
		return nil, fmt.Errorf("RData vide")
	}
	df := top.pairs[0].val
	if df == nil || df.t != 19 || df.attrs["names"] == nil {
		return nil, fmt.Errorf("le premier objet n'est pas une table R")
	}
	names := df.attrs["names"].strs
	if len(names) != len(df.list) {
		return nil, fmt.Errorf("%d noms de colonnes pour %d colonnes", len(names), len(df.list))
	}
	t := &RTable{Name: top.pairs[0].tag, columns: map[string][]string{}}
	for k, col := range df.list {
		cells, err := cellsOf(col)
		if err != nil {
			return nil, fmt.Errorf("colonne %s : %w", names[k], err)
		}
		if k == 0 {
			t.Rows = len(cells)
		}
		t.columns[names[k]] = cells
	}
	return t, nil
}

func cellsOf(c *rObj) ([]string, error) {
	if c == nil {
		return nil, fmt.Errorf("colonne vide")
	}
	switch c.t {
	case 14:
		out := make([]string, len(c.reals))
		for i, v := range c.reals {
			if !math.IsNaN(v) {
				out[i] = fmt.Sprintf("%g", v)
			}
		}
		return out, nil
	case 10, 13:
		levels := []string(nil)
		if lv := c.attrs["levels"]; lv != nil {
			levels = lv.strs
		}
		out := make([]string, len(c.ints))
		for i, v := range c.ints {
			switch {
			case v == naInt:
			case levels != nil && int(v) >= 1 && int(v) <= len(levels):
				out[i] = levels[v-1]
			default:
				out[i] = fmt.Sprintf("%d", v)
			}
		}
		return out, nil
	case 16:
		out := make([]string, len(c.strs))
		for i, s := range c.strs {
			if s != naStr {
				out[i] = s
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("type de colonne %d non géré", c.t)
}
