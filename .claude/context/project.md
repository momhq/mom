# copilot-core — o projeto

Este repo **é** o core do sistema multi-agente que gerencia todos os outros projetos do founder. É um projeto meta: o código aqui não é produto, é a definição do método de trabalho que os produtos (Logbook, Saintfy, futuros) herdam.

## O que vive aqui

- **`agents/leo.md`** — o Manager of Managers. Ponto de entrada conversacional em qualquer sessão de qualquer projeto.
- **`agents/managers/`** — os 4 tech leads universais (Dev, Designer, Marketing, PM). Projetos estendem via `extends:` pra adicionar contexto local sem reescrever a base.
- **`rules/`** — 11 rules universais que governam comportamento de todos os agentes. Propagation, anti-hallucination, peer-review, escalation, etc.
- **`skills/`** — skills model-invoked que os agentes usam sob demanda. Hoje só `session-wrap-up`.
- **`docs/conventions/`** — convenções operacionais compartilhadas entre projetos (ex: GitHub project management). Templates vivem em `docs/conventions/templates/`.
- **`docs/rdds/`** — Research & Design Docs. Cada decisão arquitetural grande vira um RDD com `rdd.md` + `refinements.md` append-only.
- **`scripts/sync.sh`** — instalador symlink-based. Copia `agents/leo.md`, `agents/managers/`, `rules/`, `skills/` pra `~/.claude/` via symlink, fazendo o core ser a fonte de verdade global em qualquer projeto.

## Stack

Majoritariamente markdown + yaml frontmatter. Uma pitada de bash (sync.sh). Zero código de aplicação. O "build" é `sync.sh`, o "teste" é usar os agentes em projetos reais e observar falhas.

## Filosofia fundamental

**Copilot-style, não Paperclip-style.** Founder conversa com Leo, Leo delega pros Managers, Managers delegam pros specialists. Review é automático via sub-instâncias em modo adversarial. Founder decide o *o quê*, Leo decide o *como*, volta pro founder no que é irreversível ou estrutural.

**Estender, nunca sobrescrever.** Projetos herdam o core via `extends:` no frontmatter dos Managers. Podem adicionar rules, self-QA items, específicos do stack local. Nunca podem remover comportamento universal.

## Como evoluir o core

O próprio core segue o método que ele define:

1. **Decisão arquitetural grande** → vira RDD novo em `docs/rdds/YYYY-MM-DD-slug/`
2. **Refinamento pós-implementação** → vira entry append-only em `docs/rdds/.../refinements.md` do RDD original
3. **Rule nova ou mudança em rule existente** → escalation trigger obrigatório; só aplica com R2 explícito do founder
4. **Manager novo** → decisão estratégica, passa por R2 e vira RDD
5. **Skill nova** → escalation trigger; só adiciona quando tem motivo claro (não criar skill especulativa)

## Dogfooding

A expectativa é que cada sessão no copilot-core seja ela mesma um exercício do método: PRD se for feature de sistema grande, RDD se for design técnico, peer review adversarial, evidence-over-claim, propagation no fim. Se o método fizer sentido em self-hosting, faz sentido nos produtos downstream.

## Histórico relevante

O core nasceu de um RDD inicial (`docs/rdds/2026-04-08-copilot-core-architecture/`) escrito a partir do aprendizado acumulado do projeto Saintfy que precedeu. Logbook é o primeiro piloto real (Pilot Phase 1, começou 2026-04-08).

O primeiro refinamento pós-implementação (ebf685b) foi sobre propagação wrap-up-driven — veio de uma falha real observada no piloto, não de especulação. Esse é o padrão de evolução esperado: falhas do campo → refinamentos documentados → propagação cross-projeto.
