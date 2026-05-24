package seguranca

import "testing"

func TestValidarConfigProducao_DevSempreOk(t *testing.T) {
	if err := ValidarConfigProducao(ConfigProducao{Production: false}); err != nil {
		t.Fatalf("dev não deve falhar: %v", err)
	}
}

func TestValidarConfigProducao_ProdSemSecret(t *testing.T) {
	err := ValidarConfigProducao(ConfigProducao{Production: true, SecretKey: "", CORSOrigin: "https://app.deelp.com.br"})
	if err == nil {
		t.Fatal("esperava erro de SECRET_KEY")
	}
}

func TestValidarConfigProducao_ProdSemCors(t *testing.T) {
	err := ValidarConfigProducao(ConfigProducao{Production: true, SecretKey: "abc", CORSOrigin: ""})
	if err == nil {
		t.Fatal("esperava erro de CORS_ORIGIN")
	}
}

func TestValidarConfigProducao_ProdCorsCuringa(t *testing.T) {
	err := ValidarConfigProducao(ConfigProducao{Production: true, SecretKey: "abc", CORSOrigin: "*"})
	if err == nil {
		t.Fatal("esperava erro de CORS_ORIGIN curinga")
	}
}

func TestValidarConfigProducao_ProdOK(t *testing.T) {
	err := ValidarConfigProducao(ConfigProducao{
		Production: true,
		SecretKey:  "abc",
		CORSOrigin: "https://app.deelp.com.br",
	})
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}
}
