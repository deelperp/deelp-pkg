package seguranca

import (
	"testing"
	"time"
)

func TestIPBlocker_InMemory_BloqueiaAposMaxTentativas(t *testing.T) {
	b := NewIPBlocker(IPBlockerConfig{
		MaxTentativas:    3,
		DuracaoBloqueio:  time.Minute,
		JanelaTentativas: time.Minute,
	})
	const ip = "203.0.113.10"

	if b.EstaBloqueado(ip) {
		t.Fatal("não deveria estar bloqueado antes de qualquer falha")
	}

	b.RegistrarFalha(ip)
	b.RegistrarFalha(ip)
	if b.EstaBloqueado(ip) {
		t.Fatal("não deveria bloquear antes de atingir o limite")
	}

	b.RegistrarFalha(ip)
	if !b.EstaBloqueado(ip) {
		t.Fatal("deveria estar bloqueado após atingir o limite")
	}
}

func TestIPBlocker_InMemory_RegistrarSucesso_ReseteContador(t *testing.T) {
	b := NewIPBlocker(IPBlockerConfig{
		MaxTentativas:    3,
		DuracaoBloqueio:  time.Minute,
		JanelaTentativas: time.Minute,
	})
	const ip = "203.0.113.11"

	b.RegistrarFalha(ip)
	b.RegistrarFalha(ip)
	b.RegistrarSucesso(ip)
	b.RegistrarFalha(ip)
	b.RegistrarFalha(ip)

	if b.EstaBloqueado(ip) {
		t.Fatal("sucesso deveria ter zerado o contador — 2 falhas pós-sucesso não devem bloquear")
	}
}

func TestIPBlocker_InMemory_RestanteRetornaTTLPositivo(t *testing.T) {
	b := NewIPBlocker(IPBlockerConfig{
		MaxTentativas:    1,
		DuracaoBloqueio:  10 * time.Second,
		JanelaTentativas: time.Minute,
	})
	const ip = "203.0.113.12"
	b.RegistrarFalha(ip)
	if !b.EstaBloqueado(ip) {
		t.Fatal("deveria bloquear após 1 falha")
	}
	r := b.Restante(ip)
	if r <= 0 || r > 10*time.Second {
		t.Fatalf("Restante fora do esperado: %v", r)
	}
}

func TestIPBlocker_InMemory_IPDesbloqueadoAposExpiracao(t *testing.T) {
	b := NewIPBlocker(IPBlockerConfig{
		MaxTentativas:    1,
		DuracaoBloqueio:  10 * time.Millisecond,
		JanelaTentativas: time.Minute,
	})
	const ip = "203.0.113.13"
	b.RegistrarFalha(ip)
	if !b.EstaBloqueado(ip) {
		t.Fatal("deveria bloquear inicialmente")
	}
	time.Sleep(25 * time.Millisecond)
	if b.EstaBloqueado(ip) {
		t.Fatal("deveria liberar após DuracaoBloqueio")
	}
}
