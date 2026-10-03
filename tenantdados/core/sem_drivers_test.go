package core

import (
	"os/exec"
	"strings"
	"testing"
)

func TestNucleoNaoDependeDeDriversNemHTTP(t *testing.T) {
	saida, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list indisponível: %v", err)
	}
	prefixos := []string{"go.mongodb.org/", "github.com/lib/pq", "github.com/aws/", "github.com/deelperp/deelp-pkg/tenantdados/"}
	exatos := map[string]bool{"database/sql": true, "net/http": true}
	for _, dep := range strings.Split(string(saida), "\n") {
		if exatos[dep] {
			t.Errorf("núcleo depende de %s", dep)
		}
		for _, p := range prefixos {
			if strings.HasPrefix(dep, p) && dep != "github.com/deelperp/deelp-pkg/tenantdados/core" {
				t.Errorf("núcleo depende de %s", dep)
			}
		}
	}
}
