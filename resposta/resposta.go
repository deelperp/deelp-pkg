// Package resposta é a fronteira HTTP compartilhada dos serviços Deelp.
//
// O cliente web e o app esperam o envelope {sucesso, mensagem, conteudo}.
// Cada serviço que inventa o próprio writer diverge o contrato em silêncio —
// este pacote é a fonte única.
package resposta

const (
	MsgSemEmpresa = "Token sem vínculo de empresa. Selecione uma colaboração novamente."
	MsgSemUsuario = "Token sem usuário válido."
)

// Corpo é o envelope JSON canônico da API.
type Corpo struct {
	Sucesso  bool   `json:"sucesso"`
	Mensagem string `json:"mensagem,omitempty"`
	Conteudo any    `json:"conteudo,omitempty"`
}

func Ok(conteudo any) Corpo {
	return Corpo{Sucesso: true, Conteudo: conteudo}
}

func Falha(mensagem string) Corpo {
	return Corpo{Sucesso: false, Mensagem: mensagem}
}
