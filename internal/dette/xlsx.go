// Lecteur XLSX minimal, sur la bibliothèque standard — copie volontaire de
// internal/presidentielle/xlsx.go et internal/macro/xlsx.go : plutôt
// qu'une dépendance partagée entre deux paquets qui n'ont rien d'autre en
// commun, une centaine de lignes dupliquées, faciles à auditer chacune dans
// leur contexte.
package dette

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Ici, pour les classeurs de l'Administration fédérale des finances suisse,
// qui ne publie ses statistiques financières qu'en Excel. On ne gère que ce
// dont on a besoin : chaînes partagées, valeurs numériques, et lecture d'une
// feuille par son nom.

type xlsxFile struct {
	zr     *zip.ReadCloser
	shared []string
	sheets map[string]string // nom de feuille -> chemin dans l'archive
}

func openXLSX(path string) (*xlsxFile, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	f := &xlsxFile{zr: zr, sheets: map[string]string{}}

	rels := map[string]string{}
	if b, err := f.read("xl/_rels/workbook.xml.rels"); err == nil {
		var doc struct {
			Rel []struct {
				ID     string `xml:"Id,attr"`
				Target string `xml:"Target,attr"`
			} `xml:"Relationship"`
		}
		if err := xml.Unmarshal(b, &doc); err != nil {
			return nil, err
		}
		for _, r := range doc.Rel {
			rels[r.ID] = strings.TrimPrefix(strings.TrimPrefix(r.Target, "/xl/"), "xl/")
		}
	}

	b, err := f.read("xl/workbook.xml")
	if err != nil {
		return nil, err
	}
	var wb struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := xml.Unmarshal(b, &wb); err != nil {
		return nil, err
	}
	for _, s := range wb.Sheets {
		if t, ok := rels[s.RID]; ok {
			f.sheets[s.Name] = "xl/" + t
		}
	}

	// Les chaînes partagées sont optionnelles : un classeur entièrement
	// numérique n'en a pas.
	if b, err := f.read("xl/sharedStrings.xml"); err == nil {
		var ss struct {
			SI []struct {
				T string `xml:"t"`
				R []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"si"`
		}
		if err := xml.Unmarshal(b, &ss); err != nil {
			return nil, err
		}
		for _, si := range ss.SI {
			if len(si.R) > 0 {
				var sb strings.Builder
				for _, r := range si.R {
					sb.WriteString(r.T)
				}
				f.shared = append(f.shared, sb.String())
			} else {
				f.shared = append(f.shared, si.T)
			}
		}
	}
	return f, nil
}

func (f *xlsxFile) Close() error { return f.zr.Close() }

func (f *xlsxFile) read(name string) ([]byte, error) {
	for _, zf := range f.zr.File {
		if zf.Name == name {
			rc, err := zf.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s absent de l'archive", name)
}

func (f *xlsxFile) sheetNames() []string {
	out := make([]string, 0, len(f.sheets))
	for n := range f.sheets {
		out = append(out, n)
	}
	return out
}

// rows renvoie les lignes de la feuille sous forme de maps colonne -> valeur,
// la colonne étant sa lettre ("A", "B"...). Les cellules vides sont absentes de
// la map : une cellule absente et une cellule vide sont la même chose ici.
func (f *xlsxFile) rows(sheet string) ([]map[string]string, error) {
	path, ok := f.sheets[sheet]
	if !ok {
		return nil, fmt.Errorf("feuille %q absente (présentes : %v)", sheet, f.sheetNames())
	}
	b, err := f.read(path)
	if err != nil {
		return nil, err
	}
	var ws struct {
		Rows []struct {
			Cells []struct {
				Ref  string `xml:"r,attr"`
				Type string `xml:"t,attr"`
				V    string `xml:"v"`
				IS   struct {
					T string `xml:"t"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(b, &ws); err != nil {
		return nil, err
	}
	out := make([]map[string]string, 0, len(ws.Rows))
	for _, r := range ws.Rows {
		m := map[string]string{}
		for _, c := range r.Cells {
			v := c.V
			switch c.Type {
			case "s":
				i := atoiSafe(v)
				if i < 0 || i >= len(f.shared) {
					continue
				}
				v = f.shared[i]
			case "inlineStr":
				v = c.IS.T
			}
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			m[colLetters(c.Ref)] = v
		}
		out = append(out, m)
	}
	return out, nil
}

func colLetters(ref string) string {
	for i := 0; i < len(ref); i++ {
		if ref[i] >= '0' && ref[i] <= '9' {
			return ref[:i]
		}
	}
	return ref
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	if s == "" {
		return -1
	}
	return n
}
