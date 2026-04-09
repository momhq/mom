---
name: know-what-you-dont-know
description: Managers e specialists param ANTES de executar quando detectam domínio fora da sua capacidade.
---

## Regra

Antes de escrever qualquer linha de código, desenho, ou copy, o agente executor **para e avalia** se realmente tem expertise pra entregar a task com qualidade. Se a resposta é "não", dispara Hiring Loop ou escalation — **antes** de errar, não depois.

## Por que isso precisa de enforcement explícito

Claude (o modelo) sabe quase tudo superficialmente. Um Dev Manager instruído a implementar APNs vai tentar — porque tem conhecimento superficial sobre APNs do treino. É exatamente o modo de falha que esta rule previne.

Rule dizendo "peça ajuda quando não souber" não basta. O modelo vai achar que sabe. Precisa de mecanismo que **força** o raciocínio meta a ser materializado.

## Os 4 mecanismos (todos obrigatórios)

### Mecanismo 1 — Pre-execution check obrigatório

Antes de escrever qualquer código, desenho, ou copy, o agente **preenche e cola na resposta** este formulário (não pode só "pensar", tem que escrever):

```
## Pre-execution check

- Domínio técnico específico desta task (1 frase):
  [...]

- Eu tenho um specialist no meu time com playbook pra este domínio?
  [ ] Sim → specialist: [nome + arquivo]
  [ ] Não → STOP: disparar Hiring Loop antes de continuar

- Se eu errar esta task por falta de expertise, qual é o pior cenário?
  [ ] Bug trivial, fácil de reverter → posso executar com cuidado
  [ ] Bug difícil de reverter ou com efeito cascata → specialist obrigatório
  [ ] Risco de produção / segurança / dinheiro → specialist obrigatório

- Minha confiança neste domínio é alta ou baixa?
  [ ] Alta, e sei por quê (cite a fonte: specialist X, memory Y, doc Z)
  [ ] Baixa OU "alta" sem fonte citável → STOP
```

**O truque é escrever.** Pensar "sei sim" é fácil. Ter que citar fonte verificável força o modelo a confrontar "realmente sei ou tô chutando?".

Se o agente tentar driblar escrevendo "sim" sem fonte ou "alta confiança" sem justificativa — founder vê no output e cobra. Transparência força honestidade.

### Mecanismo 2 — Trust gradient por categoria

Cada Manager tem uma tabela de trust default por categoria de task. Algumas categorias **nunca** executam sem specialist, mesmo que a task pareça simples.

Template (cada Manager personaliza no agent file dele):

| Categoria | Trust default | Specialist obrigatório? |
|---|---|---|
| [categoria baixo risco] | Alto | Não |
| [categoria médio risco] | Médio | Depende do escopo |
| [categoria alto risco] | **Baixo** | **Sempre** |

Exemplo pro Dev Manager:

| Categoria | Trust default | Specialist obrigatório? |
|---|---|---|
| Edição de texto/copy/JSON estático | Alto | Não |
| UI puro (componente novo, estilo) | Alto | Só se design system específico |
| CRUD padrão com ORM conhecido | Alto | Não |
| Integração com API externa | Médio | Sim se for protocolo (APNs, OAuth, WebAuthn) |
| Migração de schema complexa | Médio | Sim |
| Crypto / auth / security | **Baixo** | **Sempre** |
| Native bridging (Capacitor, React Native) | **Baixo** | **Sempre** |
| Infra / deploy / CI | **Baixo** | **Sempre** |

A coluna "**Sempre**" é linha dura. Mesmo com pressão, Manager não executa — dispara Hiring Loop ou escala pro founder.

### Mecanismo 3 — Post-failure hardening

Quando peer review (rule `peer-review-automatic`) detecta que um agente errou por **falta de expertise em domínio X**, essa falha vira input **automático** pro trust gradient.

Processo:
1. Peer review rejeita: "falhou porque não tinha playbook pra domínio X"
2. Leo registra a falha
3. Leo adiciona X na lista "Sempre specialist" do Manager **no projeto atual** (via extensão `.claude/agents/managers/<manager>.md`)
4. Na próxima vez que o Manager encontrar task em X, trust gradient já bloqueia execução sem specialist

Isso transforma falhas em enforcement automático — cada erro corrige o sistema pra não repetir. É o inverso do padrão antigo onde lições ficavam em memories que ninguém relia.

### Mecanismo 4 — Lessons learned pass

Após peer review rejeitar trabalho, **antes de corrigir a task**, o agente roda um lessons learned pass:

```
## Lessons learned (obrigatório após review rejection)

- Qual regra ou checklist item teria prevenido essa falha?
  [...]

- Essa lição é específica deste projeto ou universal?
  [ ] Específica do projeto → propor adição ao agent file estendido do projeto
  [ ] Universal → propor adição ao agent file do core

- A lição deve virar:
  [ ] Item no Self-QA do Manager
  [ ] Item no trust gradient (Mecanismo 2)
  [ ] Rule de domínio nova
  [ ] Atualização de specialist existente

- Proposta escrita (1 parágrafo):
  [...]

- Proposta vai pro Leo → R2 com founder antes de ser aplicada
```

Sem isso, as lições ficam na cabeça do founder (até esquecer) ou em memories stale. Com isso, cada falha tem chance de virar enforcement permanente.

## Como os 4 se encaixam

- **Mecanismo 1** é preventivo, roda antes de cada task
- **Mecanismo 2** é estrutural, define linhas duras por disciplina
- **Mecanismo 3** é reativo automático, fortalece após falha
- **Mecanismo 4** é reflexivo deliberado, extrai lição pra refinar o sistema

Juntos, formam um sistema que **aprende e se fortalece com cada falha real**, respeitando o R2 (nada se aplica sem founder aprovar).

## Responsabilidade

Esta rule aplica a **todo agente executor** (Managers quando executam exceção, specialists quando executam trabalho). Reviewers (Managers em modo review) também consultam trust gradient ao avaliar se a execução foi legítima ou se foi tentativa de driblar o sistema.
