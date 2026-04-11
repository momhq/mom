# Knowledge Base Architecture — Vision Document

> Resultado da sessão de design entre owner e Leo em 2026-04-10.
> Este documento captura as decisões conceituais e arquiteturais
> para o sistema de Knowledge Base (KB) do projeto.

## 1. Filosofia

**A IA pensa, o código executa o determinístico.**

Tokens são recurso escasso e caro. Tudo que pode ser um script, deve ser
um script. A IA gasta tokens apenas em: julgamento, decisão, criatividade,
análise. Nunca em varrer arquivos, montar índices, validar schemas.

**Token Economy:** a arquitetura inteira é desenhada para minimizar o
consumo de tokens sem perder qualidade de raciocínio. Cada decisão
estrutural responde à pergunta "isso precisa de IA ou um script resolve?".

## 2. Modelo Mental: Rede Neural, não Árvore

### Por que não árvore
Uma árvore impõe hierarquia rígida via filesystem (pastas = galhos,
arquivos = folhas). Mas conhecimento real não tem um "pai" natural —
uma decisão de auth é igualmente sobre segurança, produto e enterprise.
A pasta onde ela mora seria sempre uma escolha arbitrária.

### O modelo escolhido
Inspirado em redes neurais e bancos não-relacionais (document stores):

- **Documentos** são unidades de conhecimento (JSON)
- **Tags** criam conexões entre documentos (sinapses)
- **Index** é o mapa da rede (gerado por script, não por IA)
- **Schema** define a estrutura mínima dos documentos (validação)
- **Sem hierarquia forçada** — conexões emergem dos dados

Analogia: MongoDB, não PostgreSQL. Documentos com estrutura mínima,
indexados por campos, consultáveis por qualquer combinação de tags.

### Lifecycle (renovação natural)
- `permanent` — identidade do projeto, muda raramente (como o tronco)
- `learning` — padrões, lições, decisões que envelhecem devagar
- `state` — fatos temporários que caem e são substituídos

## 3. Princípio Central: KB é para Agents, não para Humanos

O humano **nunca lê** o KB diretamente. O humano conversa com Leo.

Fluxo:
```
Humano ↔ Leo (conversa natural)
    ↓
Wrap-up → extrai conhecimento → grava docs JSON no KB
    ↓
Scripts (hooks) → rebuild index, valida schema, conecta tags
    ↓
Próxima sessão → Leo lê index → sabe tudo → humano continua de onde parou
    ↓
Humano pede output → Leo consulta KB → gera relatório/dashboard/resposta legível
```

O **output** (relatório, dashboard, levantamento) é efêmero e legível.
O **KB** é permanente e otimizado para máquina.

## 4. Formato: JSON

Cada documento de conhecimento é um arquivo JSON com schema validável.

### Exemplo de documento
```json
{
  "id": "abac-enterprise",
  "type": "decision",
  "lifecycle": "learning",
  "domain": "eng-auth",
  "tags": ["auth", "abac", "enterprise", "permissions"],
  "content": {
    "decision": "Implementar Attribute-Based Access Control para granularidade de permissões enterprise",
    "context": "O modelo RBAC atual não suporta permissões por atributo dinâmico",
    "impact": ["Requer refactor de middleware de auth", "Afeta todas as rotas admin"]
  },
  "created": "2026-03-15",
  "updated": "2026-04-02"
}
```

### Schema: tipos de documento
| Type | Lifecycle | Campos obrigatórios | Uso |
|---|---|---|---|
| `identity` | permanent | name, content, tags | O que o projeto É (stack, marca, arquitetura) |
| `decision` | learning | name, content, context, impact, tags | Decisões tomadas com contexto e impacto |
| `pattern` | learning | name, content, when_to_use, tags | Padrões e convenções reutilizáveis |
| `fact` | state | name, content, tags, expires | Fatos temporários (status, números, bloqueios) |
| `feedback` | permanent | name, content, tags | Correções do owner sobre comportamento dos agents |
| `reference` | state | name, url, purpose, tags | Ponteiros para recursos externos |

### Index (gerado por script)
```json
{
  "by_tag": {
    "auth": ["abac-enterprise", "multi-auth-strategy", "e2e-encryption-keys"],
    "enterprise": ["abac-enterprise", "open-core-split", "ddp-streamer-separate"]
  },
  "by_domain": {
    "eng-auth": ["abac-enterprise", "multi-auth-strategy"]
  },
  "by_type": {
    "decision": ["abac-enterprise", "meteor3-migration"]
  },
  "by_lifecycle": {
    "learning": ["abac-enterprise", "meteor3-migration"],
    "state": ["current-sprint-status"]
  },
  "stats": {
    "total_docs": 35,
    "total_tags": 45,
    "most_connected_tag": "enterprise (8 docs)",
    "stale_docs": 2,
    "last_rebuilt": "2026-04-10T14:30:00Z"
  }
}
```

## 5. Estrutura de Diretórios

```
.claude/kb/                    ← o "banco" (por projeto)
├── schema.json                ← define tipos e campos obrigatórios
├── index.json                 ← mapa da rede (gerado por script)
├── docs/                      ← documentos de conhecimento (flat)
│   ├── abac-enterprise.json
│   ├── meteor3-migration.json
│   ├── fuselage-over-mui.json
│   └── ...
└── scripts/                   ← manutenção automática (zero tokens)
    ├── build-index.sh         ← varre docs/ → gera index.json
    ├── validate.sh            ← valida docs contra schema.json
    ├── check-stale.sh         ← detecta docs com lifecycle:state sem update > N dias
    └── extract-tags.sh        ← (opcional) extrai keywords do conteúdo
```

## 6. Quem Faz o Quê

| Ator | Responsabilidade | Gasta tokens? |
|---|---|---|
| **Agent (Leo/Managers)** | Cria/atualiza docs com conteúdo e tags | Sim (pouco) |
| **Scripts (hooks)** | Rebuild index, valida schema, detecta stale | Não |
| **Opus (esporádico)** | Revisa rede, sugere reorganização, identifica gaps | Sim (raro) |
| **Owner** | Aprova mudanças estruturais, conversa com Leo | — |

### Manutenção da rede (3 camadas)
1. **Script (toda sessão):** detecta docs "soltos" (sem tags), docs stale, rebuilda index
2. **Opus (esporádico):** revisa a rede, propõe novas tags, identifica clusters, sugere merges
3. **Agent do dia:** cria docs com boas tags no momento da decisão/aprendizado

## 7. O que Isso Substitui

| Sistema atual | No novo modelo |
|---|---|
| `memory/` (MEMORY.md + .md files) | Docs no KB com lifecycle adequado |
| `context/decisions/` | Docs type: `decision` |
| `context/project.md` | Doc type: `identity` |
| `context/stack.md` | Doc type: `identity` |
| Propagation checklist (mental) | Script build-index + validate |
| State vs Learning (regra manual) | Campo `lifecycle` no schema |
| Metrics JSONL | Poderia ser absorvido como docs type: `metric` |

## 8. Expansibilidade

### Fase 1 (agora): Flat files JSON por projeto
```
projeto/.claude/kb/docs/*.json
```
Simples, funcional, validável.

### Fase 2 (futuro): Cross-project para orgs
```
~/.claude/kb/
├── project-index.json          ← mapeia projetos
├── projects/
│   ├── rocketchat-web/
│   ├── rocketchat-mobile/
│   └── rocketchat-docs/
└── cross-project-index.json   ← conexões entre projetos
```
Leo consulta cross-project-index quando precisa de contexto de outro repo.

### Fase 3 (horizonte): MongoDB ou outro document store
O schema JSON é compatível com MongoDB. Migrar = inserir os docs
no banco e trocar "lê arquivo" por "query no banco". Zero mudança
no schema, zero mudança no comportamento dos agents.

## 9. Decisões em Aberto

- [ ] Schema definitivo com todos os campos e validações
- [ ] Implementação dos scripts (build-index, validate, check-stale)
- [ ] Como o wrap-up gera docs JSON (adaptar a skill)
- [ ] Hook que roda os scripts automaticamente pós-sessão
- [ ] Migração do sistema atual (memory/ + context/) para o KB
- [ ] Nome do projeto (owner quer renomear copilot-core)

## 10. Inspirações

- **Andrej Karpathy LLM Wiki** — o tweet que iniciou a exploração.
  Conceito de raw → wiki → index → Q&A. Adaptamos o modelo
  substituindo hierarquia por rede neural e MD por JSON.
- **MongoDB document model** — documentos com schema flexível,
  indexados por campos, consultáveis por combinação.
- **Redes neurais biológicas** — conexões por sinapses (tags),
  sem hierarquia forçada, fortalecimento por uso.
- **Token Economy** — princípio de otimização econômica que guia
  toda decisão arquitetural: IA pensa, código executa.
