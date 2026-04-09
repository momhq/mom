# copilot-core

Método de trabalho replicável pra agentes Claude Code. Um gerente conversacional (Leo) + time de Managers por disciplina + regras universais. Estende-se por projeto sem reescrever a base.

**Status:** early-stage, sem piloto ainda. Repo privado enquanto o design amadurece.

## Filosofia em 2 frases

**Copilot-style, não Paperclip-style.** Você conversa com o Leo, ele delega pros Managers, eles delegam pros specialists (contratados via Hiring Loop), review é automático e transparente, você valida nos pontos de inflexão. Founder decide o *o quê*, Leo decide o *como*, volta pro founder no que é irreversível ou estrutural.

## Estrutura

```
copilot-core/
├── agents/
│   ├── leo.md                      ← Manager of Managers (model: opus)
│   └── managers/
│       ├── dev.md                  ← tech lead de desenvolvimento
│       ├── designer.md             ← tech lead de design
│       ├── pm.md                   ← tech lead de produto
│       └── marketing.md            ← tech lead de marketing
├── rules/                          ← 11 rules universais
│   ├── propagation.md
│   ├── anti-hallucination.md
│   ├── think-before-execute.md
│   ├── evidence-over-claim.md
│   ├── peer-review-automatic.md
│   ├── state-vs-learning.md
│   ├── hiring-loop.md
│   ├── know-what-you-dont-know.md
│   ├── escalation-triggers.md
│   ├── inheritance.md
│   └── metrics-collection.md
├── scripts/
│   └── sync.sh                     ← symlinka core → ~/.claude/ (a escrever — D8)
└── docs/
    └── rdds/                       ← decisões arquiteturais versionadas
```

## Como usar (uma vez que o piloto validar)

**Primeira vez numa máquina:**
```bash
git clone git@github.com:vmarinogg/copilot-core.git ~/Github/copilot-core
bash ~/Github/copilot-core/scripts/sync.sh
```

**Updates:**
```bash
cd ~/Github/copilot-core && git pull
# Symlinks apontam pro repo — conteúdo já atualizado.
# Rodar sync.sh só se topologia mudou (arquivos adicionados/removidos).
```

**Por projeto** (ex: Saintfy, logbook):
- Projeto ativo ganha `.claude/agents/managers/<manager>.md` com `extends: ../../../.claude/agents/managers/<manager>.md` no frontmatter
- Rules e specialists específicos do projeto vivem em `.claude/` do projeto, nunca aqui no core

## Convenções

**Managers são tech leads, não executores.** Recebem, decompõem, delegam pro time de specialists, revisam, sintetizam. Executam código/design/copy diretamente só em exceção (micro-tasks, emergência).

**Specialists vivem no projeto, nunca no core.** Core mantém só Managers universais + Rules universais. Cada projeto constitui seu time de specialists via Hiring Loop conforme precisa.

**Rules em 2 escopos:**
- **Universais** (aqui no core, em `rules/`) — "como a empresa trabalha". Carregam sempre.
- **Domínio** — "como aquele time trabalha". Embutidas nos arquivos dos Managers, carregam só quando o Manager é invocado.

**Estilo dos Managers: minimalist.** Identidade + princípios + checklist. Zero prose longa. Estrutura interna fixa: Papel → Princípios → Hiring loop → Self-QA → Escalation.

**Tom: casual.** Segunda pessoa, zero corporativês. Idioma de interação é configurável por projeto.

## Referência arquitetural

Decisões fundacionais estão em:
- `docs/rdds/2026-04-08-copilot-core-architecture/` — RDD principal (será copiado quando o repo estabilizar)
- Saintfy-Copilot/docs/rdds/2026-04-08-copilot-core-architecture/rdd.md (origem, pré-cópia)

## Estado atual

- ✅ Leo + 4 Managers (Dev, Designer, PM, Marketing) escritos
- ✅ 11 rules universais escritas (10 do RDD + metrics-collection)
- ⏳ `sync.sh` — D8 definido no plan, a implementar
- ⏳ Piloto no logbook — Q7
- ⏳ Migração do Saintfy — Q8

## Não é

- Não é framework de agentes autônomos (Paperclip-style)
- Não é CLI genérico pra Claude Code
- Não é substituto pra CLAUDE.md de projeto
- Não é produto open-source ainda (pode virar — ver parking lot do RDD)
