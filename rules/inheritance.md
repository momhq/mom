---
name: inheritance
description: Agentes com `extends` no frontmatter carregam o arquivo base antes de executar e concatenam o comportamento.
---

## Regra

Quando um agent file (geralmente no projeto) tem campo `extends` no frontmatter apontando pra outro arquivo (tipicamente no core), você **carrega o arquivo base primeiro** e o trata como fundamento. O conteúdo do projeto **adiciona** ao base, nunca substitui.

Ordem de concatenação: **core primeiro, projeto depois**.

## Por que

Este é o mecanismo que permite ao core atualizar managers e rules sem quebrar projetos. Sem ele, o projeto teria que copiar o arquivo inteiro do core e você perderia o benefício de atualização global. Com ele, o projeto adiciona **só o que é específico** (stack, tokens, decisões locais) e herda o resto.

Também é como a filosofia "estender, nunca sobrescrever" é executada na prática. Se o projeto pudesse remover comportamento do core, a garantia de qualidade do core sumiria.

## Formato

Um agent file que herda tem frontmatter assim:

```yaml
---
name: Dev Manager (Saintfy)
description: Tech lead de dev do Saintfy, estendendo Dev Manager do core
extends: ../../../../.claude/agents/managers/dev.md
tools: Read, Edit, Write, Glob, Grep, Bash, Task
model: sonnet
skills: [saintfy-briefing]
---
```

O campo `extends` pode usar:
- Path relativo ao arquivo atual (preferido pra legibilidade)
- Path absoluto começando com `/` (pra referências ao core instalado globalmente)

Na prática, projetos estendem arquivos que vivem em `~/.claude/agents/managers/*.md` (symlinkados pelo `sync.sh` do core).

## Como aplicar

### Quando Leo delega pra um Manager do projeto

1. Leo lê o agent file do projeto (ex: `~/Github/saintfy/.claude/agents/managers/dev.md`)
2. Vê `extends: ../../../../.claude/agents/managers/dev.md` no frontmatter
3. Lê o arquivo base (o Dev Manager do core em `~/.claude/agents/managers/dev.md`)
4. Concatena mentalmente: **core primeiro, projeto depois**
5. Passa o contexto combinado como briefing pra sessão que executa

Leo **não inlineia** o conteúdo do core no arquivo do projeto — apenas garante que ambos estão carregados ao executar.

### Quando Manager herda rule de domínio

Rules de domínio vivem dentro dos agent files dos Managers. Quando o Manager do projeto estende o Manager do core, as rules de domínio do core vêm automaticamente e o projeto pode **adicionar** rules próprias ou **especializar** as existentes — mas não removê-las.

Exemplo: Dev Manager do core tem regra genérica "PR workflow". Dev Manager do Saintfy estende e adiciona "além disso, PRs tocando `.tsx` rodam `scripts/lint-shadcn.sh` no self-QA". Core continua válido, projeto é mais estrito.

## O que é proibido

- ❌ **Remover comportamento do core.** Se o projeto quer desabilitar uma rule universal, é sinal de que ou a rule está errada (propor mudança no core via R2) ou o projeto está errado. Nunca desabilite silenciosamente.
- ❌ **Renomear campos do frontmatter.** Se o core define `tools: Read, Edit, ...`, o projeto não pode usar nome diferente — só pode adicionar tools ou usar as mesmas.
- ❌ **Contradizer princípios.** Se o core diz "PR-first obrigatório", o projeto não pode ter rule "commit direto em main é ok". Se há genuína necessidade de contradição, o core que está errado — propor mudança via R2.
- ❌ **Inlinear o conteúdo do core no arquivo do projeto.** Isso mata o benefício de atualização global. Use `extends`, não copy-paste.

## O que é permitido

- ✅ **Adicionar princípios, self-QA items, escalation triggers** específicos do projeto
- ✅ **Especializar** rules genéricas com detalhes concretos da stack
- ✅ **Override** de campos do frontmatter quando há motivo (ex: `model: opus` quando evidência mostra que sonnet está errando)
- ✅ **Adicionar tools** à lista além das do core (mas nunca remover)
- ✅ **Referenciar skills específicas do projeto** no campo `skills`

## Responsabilidade

Leo é quem faz a concatenação mental na hora de delegar — ele é o único agente que **lê** agent files de outros para preparar briefing. Outros agentes recebem o briefing já preparado.

Quando founder questiona "por que esse Manager fez X?", Leo consegue explicar qual parte veio do core e qual do projeto — transparência da hierarquia.
