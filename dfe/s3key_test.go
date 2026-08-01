package dfe

import "testing"

func TestChaveDocumento(t *testing.T) {
	got := ChaveDocumento("01.234.567/0001-90", "homologation", TipoNFe, "2026-08-15", "422608880001904200100000002310000000198160", ArtefatoEnvio)
	want := "01234567000190/homologacao/nfe/2026-08/422608880001904200100000002310000000198160/envio.xml"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	got = ChaveDocumento("01234567000190", "1", TipoMDFe, "2026-08", "CHAVE", ArtefatoProc)
	want = "01234567000190/producao/mdfe/2026-08/CHAVE/proc.xml"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestChaveEvento(t *testing.T) {
	got := ChaveEvento("01234567000190", "homologacao", TipoNFe, "2026-08", "CHAVE", "cancelamento", 0)
	want := "01234567000190/homologacao/nfe/2026-08/CHAVE/eventos/cancelamento.xml"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	got = ChaveEvento("01234567000190", "producao", TipoNFe, "2026-08", "CHAVE", "cce", 2)
	want = "01234567000190/producao/nfe/2026-08/CHAVE/eventos/cce-seq2.xml"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizarAmbiente(t *testing.T) {
	cases := []struct{ in, want string }{
		{"production", AmbienteProducao},
		{"1", AmbienteProducao},
		{"2", AmbienteHomologacao},
		{"", AmbienteHomologacao},
	}
	for _, c := range cases {
		if got := NormalizarAmbiente(c.in); got != c.want {
			t.Fatalf("NormalizarAmbiente(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
