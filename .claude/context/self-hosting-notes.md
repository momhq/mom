# Self-hosting notes — cuidados específicos ao rodar Leo dentro do core

Trabalhar no copilot-core usando o próprio copilot-core é dogfooding útil, mas tem particularidades que não aplicam em projeto downstream. Este arquivo lista o que precisa de atenção extra.

## 1. Propagação é instantânea

`~/.claude/agents/`, `~/.claude/rules/`, `~/.claude/skills/` são **symlinks** pra este repo (criados via `scripts/sync.sh`). Quando Leo edita `rules/peer-review-automatic.md` aqui, a próxima sessão de qualquer projeto — Logbook, Saintfy, o que for — pega a versão nova.

**Consequência:** não existe "staging" entre editar a rule e ela valer. Toda mudança em `rules/`, `agents/`, `skills/` é commit cross-projeto por construção.

**Mitigação:** a rule `escalation-triggers.md` já lista "Adição ou modificação de rule no core" como trigger obrigatório de R2. Isso vale aqui também — Leo nunca edita rule sem você aprovar. Mesmo aqui, onde "não tem mais ninguém pra bloquear".

## 2. Mudanças em Manager propagam aos projetos que estendem

Se Leo edita `agents/managers/dev.md` aqui, todo projeto que tem `.claude/agents/managers/dev.md` com `extends: ../../../../.claude/agents/managers/dev.md` (via inheritance) herda a mudança imediatamente na próxima sessão.

**Consequência:** mudar Self-QA do Dev Manager no core afeta Logbook agora e Saintfy quando migrar. Propagação é ativa, não opcional.

**Mitigação:** antes de mudar Manager no core, responda mentalmente "algum projeto que herda este Manager depende do comportamento atual?". Se sim, a mudança exige nota de migração no commit e verificação no próximo trabalho do projeto afetado. Leo escreve essa nota como parte do diff, não separada.

## 3. Editar Leo dentro de uma sessão do Leo

Se você modificar `agents/leo.md` no meio de uma sessão, a **próxima** invocação de Leo (inclusive um sub-agent dentro da mesma sessão) carrega a versão nova. A sessão corrente continua com a versão antiga em memória.

**Consequência:** tem um lag de uma invocação entre editar Leo e Leo se comportar diferente. Normalmente invisível, mas pode gerar confusão se você espera mudança imediata no próximo turn da mesma sessão.

**Mitigação:** após editar `agents/leo.md`, abrir nova sessão pra validar o comportamento muda. Não confie no próximo turn da sessão corrente pra testar.

## 4. Sem outros projetos pra escalar

Em projeto downstream (Logbook, Saintfy), o "alvo de escalation" é o founder. Aqui também é o founder, mas sem uma camada de projeto entre você e decisão arquitetural. Isso aumenta a tentação de Leo "decidir sozinho" porque está "no core, é meta-trabalho".

**Mitigação:** as rules `think-before-execute` e `escalation-triggers` continuam valendo sem exceção. Meta-trabalho não é licença pra modo direto. Se algo parece decisão estrutural, é — e precisa de R2.

## 5. Idiomas dos artefatos — inconsistência conhecida

Estado atual (2026-04-09):

- `rules/*.md` — PT
- `agents/leo.md` — PT
- `agents/managers/*.md` — PT
- `docs/rdds/2026-04-08-copilot-core-architecture/rdd.md` — PT
- `docs/rdds/2026-04-08-copilot-core-architecture/refinements.md` — **EN**
- `docs/conventions/github-project-management.md` — **EN**
- `docs/conventions/templates/*` — **EN**
- `skills/session-wrap-up/SKILL.md` — EN
- `README.md` — PT

A convention `docs/conventions/github-project-management.md` diz "Never mix languages within a project". O core viola isso. Duas hipóteses defensáveis:

- **(A) Core em PT inteiro.** Consistente com a maioria histórica. Requer traduzir refinements, conventions, skills, README.
- **(B) Core em EN inteiro.** Neutral ground cross-projetos. Requer traduzir rules, managers, Leo, RDD.

Nenhuma é claramente melhor. É decisão que cabe a uma sessão dedicada no core, com R2 do founder e propagação cuidadosa porque afeta **todos** os projetos downstream.

**Por enquanto:** `project_files: pt` está no `project-config.yml`, mas novos artefatos EN não são rejeitados — a inconsistência está flagged, não resolvida.

## 6. Loose ends conhecidos

Coisas que estão bagunçadas no core e que merecem sessão dedicada pra limpar:

- **`skills/session-wrap-up/session-wrap-up`** — symlink circular (aponta pra si mesmo). Bug de criação manual ou do sync.sh. Inofensivo mas sujo.
- **Não existe `context/` no próprio core além deste scaffold** — quando o core ganhar estado que precisa ser rastreado (ex: roadmap de evolução), precisa decidir se vive em `context/` (como projetos) ou em `docs/` (como RDDs).
- **Não existe templates pra Managers novos ou rules novas** — se alguém precisar criar um `agents/managers/legal.md` ou `rules/retention-policy.md`, não tem esqueleto. Seria útil, mas especulativo agora.

## 7. Fluxo recomendado de sessão no core

1. Abrir Claude Code em `~/Github/copilot-core/`
2. Leo identifica que é sessão do core via `self_hosting: true` no `project-config.yml`
3. Founder descreve a intenção
4. Leo aplica `think-before-execute`: decisão estrutural é quase sempre modo alinhamento
5. Se for rule/Manager/skill, Leo escreve proposta (prosa, não código) pra R2
6. R2 aprovado → Leo edita
7. Evidência no final (diff, output de sync.sh rodando, o que for verificável)
8. Wrap-up no sinal do founder → `session-wrap-up` skill roda protocolo, commit com mensagem clara referenciando o RDD ou refinement relevante
9. Push pro `origin/main` (repo privado, sem escalation trigger; se virar público, escalation aplica)

Sessões no core tendem a ser mais curtas e mais "documentais" que sessões de projeto. Não é falha — é a natureza do trabalho aqui.
