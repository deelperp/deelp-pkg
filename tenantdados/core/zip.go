package core

import (
	"archive/zip"
	"io"
)

func EscreverZip(w io.Writer, arquivos []Arquivo) error {
	zw := zip.NewWriter(w)
	for _, a := range arquivos {
		f, err := zw.Create(a.Nome)
		if err != nil {
			return err
		}
		if _, err := f.Write(a.Conteudo); err != nil {
			return err
		}
	}
	return zw.Close()
}
