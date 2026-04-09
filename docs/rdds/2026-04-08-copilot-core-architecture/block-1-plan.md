# Plan: Copilot-Core Architecture — Block 1 Definitions

## Context

Em 2026-04-08 rodamos uma sessão de planejamento longa que produziu o RDD da arquitetura `copilot-core` (`Saintfy-Copilot/docs/rdds/2026-04-08-copilot-core-architecture/rdd.md`). O RDD deixou 8 questões em aberto para serem resolvidas antes da implementação.

Esta sessão atacou o **Bloco 1** dessas questões — as decisões de "forma" que destravam todo o resto: formato dos Managers, escopo inicial do time, e fluxo de inicialização de projeto. O objetivo do plan é travar essas decisões em um documento executável para que a próxima sessão (implementação) comece com padrão único e sem ambiguidade.

**Importante**: este plan **não implementa nada**. Não cria o repo `copilot-core`, não escreve agent files, não roda sync.sh. É só o contrato de decisões do Bloco 1. A implementação é uma sessão separada.

---

## Decisões travadas nesta sessão

### D1 — Estilo dos Managers: **Minimalist**

Escolhido via comparação lado a lado com o estilo Verbose.

**Racional:**
- Tokens importam em sessões longas (memories já carregam, rules universais já carregam — Manager não precisa repetir o que está em outros lugares do core)
- Mais fácil manter consistência entre 4-6 managers diferentes
- Tom casual combina melhor com formato enxuto
- Referência/checklist é mais fiel ao papel real do Manager (tech lead operando, não treinando iniciante)

**Referência de como um Manager minimalist deve parecer** (versão Dev Manager, como ficou no preview):

```yaml
---
name: Dev Manager
extends: core/agents/managers/dev.md
tools: Read, Edit, Write, Glob, Grep, Bash, Task
model: sonnet
skills: [project-briefing]
---
```

```
Tech lead de dev. Recebe, delega, revisa, sintetiza.
Executa só em exceções.

## Princípios
- PR-first (worktree + Closes #N)
- Debugging 3-strikes antes de pedir ajuda
- Grep callsite real antes de refactor
- Self-QA com prova colada (build/lint/test)

## Hiring loop
Expertise que o time não cobre → para e reporta ao Leo.

## Self-QA
- [ ] Build passou (colar output)
- [ ] Lint passou (colar output)
- [ ] Type check passou
- [ ] Code path real exercitado

## Escalation
Pare antes de: gastar dinheiro, publicar externo,
ação destrutiva, mudar rule do core.
```

### D2 — Tom: **Casual**

- Segunda pessoa (direto ao agente), linguagem direta, zero corporativês
- Exemplo PT: "Você é o tech lead de dev" e não "Você é o líder técnico responsável pela disciplina de desenvolvimento..."
- Exemplo EN: "You're the dev tech lead" e não "You are the development technical lead responsible for..."
- **Idioma** de interação do Leo com o founder, e idioma dos arquivos do core, é decidido no setup do projeto (ver D6), não nesta decisão arquitetural. Founder tem preferência pessoal de interagir com IA em PT mas manter código/docs de projeto em EN — isso é legitimamente config por usuário/projeto, não por core.

### D3 — Frontmatter: 6 campos fixos

```yaml
---
name: <Nome do Manager>                    # obrigatório
description: <frase curta do papel>         # obrigatório
extends: <path relativo ao arquivo base>    # opcional, pra agents de projeto que estendem core
tools: Read, Edit, Write, Glob, Grep, Bash, Task  # lista de Claude Code tools permitidas
model: <haiku|sonnet|opus>                  # ver critério de seleção abaixo
skills: [...]                               # lista de model-invoked skills
---
```

**Decisões específicas sobre campos:**

- **`extends` é novo** — suporta inheritance do §6 do RDD (projeto estende core)
- **`Task` tool adicionada por padrão** nos Managers — necessária pra sub-invocação de peer review automático (Q4 deferida, testaremos no piloto)
- **`workflows` NÃO adicionado** — workflows que importam viram skills; campo separado seria redundante
- **`memory` NÃO adicionado** (existe no Saintfy hoje) — memories devem ser universais por sessão, não escopadas por agente

**Critério de seleção de model (parte da decisão D3):**

| Model | Quando usar | Agentes default |
|---|---|---|
| **`opus`** | Raciocínio de big picture, decisões arquiteturais, coordenação cross-projeto, hiring loop (formar specialists bem), síntese de trabalho de múltiplos agentes | **Leo** (sempre) |
| **`sonnet`** | Execução padrão: escrever código, revisar, delegar, aplicar rules, self-QA | **Managers** (todos por default: Dev, Designer, PM, Marketing) |
| **`haiku`** | Tarefas mecânicas de baixo raciocínio: formatadores, lint fixers, renomeação em massa, template generators, conversores simples | **Specialists mecânicos** (quando houver) |

Regras práticas:
1. **Default é sonnet.** Se não há razão explícita pra ser haiku ou opus, é sonnet.
2. **Leo é sempre opus** porque coordenação + big picture são intensivos em raciocínio, e Leo é invocado poucas vezes por sessão (custo justificado).
3. **Haiku só entra em specialists** que fazem trabalho mecânico comprovadamente. Nunca default pra um Manager — o risco de under-thinking em decisão de delegação é alto demais pra economia marginal.
4. **Opus em Managers individuais** só se houver evidência empírica de que sonnet está errando em decisões daquele domínio especificamente. Piloto vai informar isso.
5. **Projetos podem override o model do core** via `extends`: o agent do projeto declara `model: opus` e sobrepõe o default. Útil se um projeto tem dor específica que justifica custo maior.

### D4 — Estrutura interna fixa dos Managers

Toda arquivo de Manager segue esta ordem de seções (nomes exatos em português):

1. **Papel** — 1-2 frases. O que o agente é, quando delega, quando executa em exceção
2. **Princípios** — bullets curtos. Os 3-5 princípios centrais do papel
3. **Hiring loop** — 1-2 frases sobre quando disparar contratação de specialist
4. **Self-QA** — checklist de prova específica da disciplina
5. **Escalation** — lista concreta do que para o agente e força pergunta ao founder

Seções extras só se forem justificadas por natureza do domínio. Padrão é manter as 5 acima.

### D5 — Managers iniciais: **4 (Dev + Designer + PM + Marketing)**

Escrever primeiro:

| Manager | Por que agora | Insumo principal pra escrita |
|---|---|---|
| **Dev Manager** | Saintfy e logbook ambos precisam de dev; dor mais aguda observada | `~/Github/Saintfy-Copilot/.claude/agents/developer.md` + CLAUDE.md do Saintfy + memories de dev |
| **Designer Manager** | Saintfy tem design system massivo; logbook precisa de assets de store | `~/Github/Saintfy-Copilot/.claude/agents/designer.md` + `.claude/rules/design-system.md` |
| **PM Manager** | Fluxo PRD→RDD está estabelecido (memory `feedback_doc_canonical_locations`) e vai continuar em todos os projetos | `~/Github/Saintfy-Copilot/.claude/agents/pm.md` + `workflows/prd.md` atualizado |
| **Marketing Manager** | Saintfy (Instagram/ASO) e logbook (App Store listing) ambos precisam | `~/Github/Saintfy-Copilot/.claude/agents/marketer.md` |

Leo (Manager of Managers) também entra nesta primeira leva — é pré-requisito pra qualquer delegação funcionar.

**Não escrever ainda:** Research Manager, Writing Manager. Entram quando houver dor real de não existirem.

### D6 — Fluxo de inicialização de projeto

Visão do founder, documentada literal:

> "Eu imagino o seguinte como instalação: eu tenho algum tipo de `copilot init` como você mencionou, que gera um setup inicial do copilot. Nesse setup, ele vai praticamente clonar o repo copilot-core e vai pedir o repo (ou repos caso seja projeto grande) do projeto que o Copilot vai gerenciar. Nisso ele já monta a estrutura inicial e já linka com os repos de código mesmo. A partir disso, o próximo passo seria coletar mais contexto do projeto, poderia ser algum tipo de interação já via Claude Code, onde o usuário poderia enviar alguns arquivos (doc, md, pdf, o que tiver) e o Leo desse novo Copilot já faz uma primeira versão de contexto e terminar de fazer o setup."

**Fluxo detalhado (para a sessão de implementação):**

1. **Invocação** — founder roda `copilot init` em algum terminal (implementação concreta do comando é detalhe deferido)
2. **Bootstrap de máquina (só primeira vez)** — se `copilot-core` não está clonado em `~/Github/copilot-core/` nem sincado em `~/.claude/`, o init:
   - Clona `git clone <url> ~/Github/copilot-core`
   - Roda `bash ~/Github/copilot-core/scripts/sync.sh` (ver D8) pra popular `~/.claude/` com symlinks
3. **Project setup** — pergunta ao founder:
   - Path do repo principal do projeto
   - Paths de repos adicionais se for projeto multi-repo (ex: Saintfy = saintfy/ + saintfy-web/)
   - **Idioma de interação** do Leo com o founder (default: PT)
   - **Idioma dos arquivos do projeto** — código, docs, PRDs, RDDs (default: EN)
   - Essas duas escolhas ficam gravadas em `.claude/project-config.yml` e são lidas por Leo no começo de cada sessão
4. **Scaffolding** — cria em cada repo do projeto:
   - `.claude/agents/managers/` vazio (pronto pra estender)
   - `.claude/rules/` vazio (pronto pra rules específicas do projeto)
   - `.claude/context/project.md` — template vazio com seções a preencher
   - `.claude/specialists/` vazio (hiring loop populará)
5. **Context collection interativa** — abre uma sessão Claude Code e coloca Leo no papel de entrevistador:
   - Leo pede ao founder que compartilhe arquivos de contexto existentes (PRDs, docs, README, pitch, qualquer coisa)
   - Founder joga os arquivos na sessão (paste, paths, ou upload se Desktop)
   - Leo lê tudo, sintetiza primeira versão de `context/project.md`
   - Leo faz 3-5 perguntas de calibração se algo ficou ambíguo (stack, domínio, público, deadlines)
6. **Encerramento do setup** — Leo confirma com founder que o contexto capturado está correto, commita o scaffolding inicial no repo do projeto, e reporta "pronto pra trabalhar"
7. **Uso normal** — daí em diante founder conversa normalmente, Leo e managers operam, contexto enriquece organicamente

**Atualização do core (founder na máquina dele):**

- `cd ~/Github/copilot-core && git pull` — atualiza arquivos fonte
- Graças aos symlinks do D8, os updates são **imediatos** — não precisa re-rodar sync.sh se só conteúdo mudou
- Re-roda sync.sh apenas quando topologia do core muda (arquivos novos ou removidos)
- Todos os projetos daquela máquina pegam a versão nova na próxima sessão

**Multi-máquina:** cada Mac novo precisa rodar `copilot init` uma vez (pra bootstrap). Daí em diante os updates são `git pull + sync`.

### D7 — Hiring loop enforcement: como "forçar" o modelo a reconhecer lacunas

**Problema:** Claude como modelo "sabe" fazer quase tudo superficialmente. Um Dev Manager instruído a implementar APNs vai tentar — porque tem conhecimento superficial sobre o domínio no treino. Isso é exatamente o modo de falha que o hiring loop deveria prevenir.

O founder levantou esse ponto explicitamente: "vamos precisar, de alguma forma, 'forçar' o modelo identificar isso". Não basta uma rule dizendo "peça specialist quando não souber" — o modelo vai achar que sabe.

**Três mecanismos que vão ser combinados na rule `know-what-you-dont-know`:**

#### Mecanismo 1 — Self-interrogation obrigatória antes de executar código

Antes de escrever qualquer linha de código, o Manager DEVE preencher este formulário mental e **colar a resposta no output** (não só pensar — escrever):

```
## Pre-execution check (obrigatório)
- Qual é o domínio técnico específico desta task? (1 frase)
- Eu tenho um specialist no meu time com playbook para este domínio?
  [ ] Sim → qual specialist, referência ao arquivo
  [ ] Não → STOP, disparar hiring-loop
- Se eu errar essa task por falta de expertise, qual é o pior cenário?
  [ ] Bug fácil de reverter → pode executar com cuidado
  [ ] Bug difícil de reverter ou catastrófico → STOP, specialist obrigatório
- Minha confiança neste domínio é alta ou baixa?
  [ ] Alta e sei por quê (cite o specialist que cobre) → executa
  [ ] Baixa OU alta sem fonte citável → STOP
```

A obrigação de **escrever** a resposta (não só pensar) é o truque — força o modelo a materializar raciocínio meta que ele normalmente pula. Se o modelo tenta driblar escrevendo "sim" sem fonte, o founder vê e cobra.

#### Mecanismo 2 — Trust gradient por categoria de task

Rule define categorias com trust default diferente. Algumas NUNCA executam sem specialist. Exemplo pra Dev Manager:

| Categoria | Trust default | Specialist obrigatório? |
|---|---|---|
| Edição de texto/copy/JSON estático | Alto | Não |
| CRUD padrão com ORM/biblioteca conhecida | Alto | Não |
| UI puro (componente novo, estilo, layout) | Alto | Só se design system específico do projeto |
| Migração de banco de dados | Médio | Sim pra schema complex, não pra add column simples |
| Integração com API externa | Médio | Sim se for protocolo (APNs, OAuth, WebAuthn), não se for REST comum |
| Crypto / auth / security | **Baixo** | **Sempre** |
| Native bridging (Capacitor, React Native) | **Baixo** | **Sempre** |
| Infra / deploy / CI | **Baixo** | **Sempre** |

Essa tabela é **específica por Manager** (Dev tem a dele, Designer tem outra, etc.). Vive dentro do agent file do Manager na seção "Trust gradient". A categoria de "Sempre specialist" é a lista dura — mesmo com pressão, Manager não executa.

#### Mecanismo 3 — Post-failure hardening

Quando o Manager executou sem specialist e errou (detectado no peer review, ou pior, em produção), essa falha vira input automático pro trust gradient. A rule `propagation` entra aqui:

- Peer review detectou que Manager errou em domínio X porque faltava specialist
- Leo propaga: adiciona X na lista "Sempre specialist" do Manager no projeto
- Na próxima sessão, Manager tem o trust gradient atualizado

Isso transforma falhas em enforcement automático — cada erro corrige o sistema pra não repetir. É o inverso do padrão atual onde memories ficavam stale.

#### Mecanismo 4 — Lessons learned pass após falha (novo, inspirado em autoresearch)

Quando peer review rejeita trabalho de um Manager ou specialist, antes de simplesmente corrigir a task, o agente roda um **lessons learned pass**:

```
## Lessons learned (obrigatório após peer review rejection)
- Qual regra ou checklist item teria prevenido essa falha?
- Essa lição é específica do projeto atual ou universal?
- Se universal: propor ao Leo adição ao agent file do core
- Se específica do projeto: propor ao Leo adição ao agent file estendido do projeto
- Propostas vão pro founder via R2 antes de serem aplicadas
```

Isso institucionaliza o aprendizado por falha. Sem esse passo, as lições ficam na cabeça do founder (até esquecer) ou em memories que podem ficar stale. Com esse passo, cada falha tem chance de virar enforcement permanente.

**Implicação pra escrita da rule `know-what-you-dont-know`:** esses 4 mecanismos são requirements obrigatórios. Quando Q2 for abordada, a rule precisa descrever:
- Template do pre-execution check (mecanismo 1)
- Formato do trust gradient no agent file (mecanismo 2)
- Processo de post-failure hardening (mecanismo 3)
- Template do lessons learned pass (mecanismo 4)

### D8 — sync.sh: design concreto

**Problema:** founder quer updates do core via `git pull` simples, sem quebrar arquivos locais em `~/.claude/` (memories, settings, projects, etc.), funcionar multi-máquina, e ser recuperável via rollback.

**Opções avaliadas:**

| Opção | Como funciona | Por que rejeitada |
|---|---|---|
| `rsync --delete` | Copia core pra `~/.claude/`, deleta orphans | `--delete` é perigoso; bug pode apagar memories do founder |
| Git submodule em `~/.claude/` | `~/.claude/` vira parcialmente um git checkout | Mistura state do user com content do core; submodule UX é ruim; confunde Claude Code |
| Symlink do diretório inteiro | `ln -s core/agents ~/.claude/agents` | Substitui o diretório inteiro — founder perde agentes locais se houver |
| **Symlinks por arquivo (recomendada)** | Script symlinka cada arquivo individual do core pros locais correspondentes em `~/.claude/` | Funciona com loading flat do Claude Code, preserva arquivos locais, git pull = update imediato, idempotente |

**Design escolhido: `sync.sh` com symlinks por arquivo**

Script vive em `~/Github/copilot-core/scripts/sync.sh` (dentro do repo do core, pra estar disponível automaticamente em qualquer máquina que clone o repo).

Comportamento:

```bash
#!/bin/bash
# sync.sh — idempotent sync from copilot-core to ~/.claude/
set -e

CORE_DIR="${CORE_DIR:-$HOME/Github/copilot-core}"
CLAUDE_DIR="$HOME/.claude"

# Sanity check
if [ ! -d "$CORE_DIR" ]; then
  echo "Error: copilot-core not found at $CORE_DIR"
  echo "Clone it first: git clone <url> $CORE_DIR"
  exit 1
fi

mkdir -p "$CLAUDE_DIR/agents" "$CLAUDE_DIR/rules"

# Symlink every markdown file from core agents → ~/.claude/agents/
# Uses find to recurse in case managers/, specialists/, etc.
find "$CORE_DIR/agents" -type f -name "*.md" | while read src; do
  basename=$(basename "$src")
  ln -sf "$src" "$CLAUDE_DIR/agents/$basename"
  echo "synced agent: $basename"
done

# Symlink every rule file from core rules → ~/.claude/rules/
find "$CORE_DIR/rules" -type f -name "*.md" | while read src; do
  basename=$(basename "$src")
  ln -sf "$src" "$CLAUDE_DIR/rules/$basename"
  echo "synced rule: $basename"
done

# Clean up dangling symlinks (files removed from core on pull)
find "$CLAUDE_DIR/agents" -type l ! -exec test -e {} \; -delete 2>/dev/null || true
find "$CLAUDE_DIR/rules" -type l ! -exec test -e {} \; -delete 2>/dev/null || true

echo ""
echo "✓ Sync complete."
echo "Future updates: cd $CORE_DIR && git pull"
echo "Re-run sync.sh only if new files were added to core."
```

**Propriedades importantes:**

1. **Idempotente.** Safe rodar N vezes. `ln -sf` sobrescreve symlink existente, não erra.
2. **Zero-copy depois do primeiro run.** Depois que os symlinks estão criados, `git pull` no core é suficiente pra update — os symlinks apontam pra arquivos vivos do repo.
3. **Re-run só quando topologia muda.** Se core adicionar `agents/managers/research.md` novo, founder roda `sync.sh` pra criar o novo symlink. Se core só editar conteúdo de `dev.md` existente, zero trabalho — symlink já aponta pro arquivo.
4. **Local files preservados.** Memories em `~/.claude/memory/`, settings em `~/.claude/settings.json`, projects em `~/.claude/projects/` — nada disso é tocado.
5. **Rollback trivial.** `cd ~/Github/copilot-core && git checkout <rev>` — symlinks seguem automaticamente. Se o checkout removeu arquivos, rodar sync.sh limpa os dangling symlinks.
6. **Dangling cleanup.** Find com `! -exec test -e` detecta symlinks cujo target foi removido e limpa — evita clutter.
7. **Multi-máquina.** Cada Mac novo: `git clone <core-url> ~/Github/copilot-core && bash ~/Github/copilot-core/scripts/sync.sh`. Duas linhas, feito.

**Conflito potencial com arquivos locais de mesmo nome:** se founder tem `~/.claude/agents/dev.md` local e core também tem `dev.md`, o symlink sobrescreve o local. Solução: core usa nomes distintivos (ex: `core-dev-manager.md`) OU founder usa subdirectory local que não conflita. **Decisão:** core usa nomes limpos (`dev.md`, `designer.md`), founder evita conflitos mantendo agentes locais custom em nomes únicos (`dev-experimental.md`). Edge case, improvável na prática.

**Quando escrever de verdade:** junto com a criação do repo `copilot-core` na sessão de piloto. Não antes — não tem pra quê sem ter conteúdo no repo pra sincar.

### D9 — Métricas de outcome (inspirado em autoresearch)

Autoresearch do Karpathy insiste em fitness function mensurável como pré-requisito pra auto-refinamento. Nossa arquitetura até agora não tinha métricas. Vamos coletá-las desde o piloto pra ter dados reais pra refinar o core.

**5 métricas básicas a coletar a partir do piloto do logbook:**

| Métrica | O que mede | Como coletar |
|---|---|---|
| **Peer review pass rate** | % de tasks que passam no peer review na primeira tentativa | Instância de review logga aprovação/rejeição |
| **Founder rejection rate** | % de entregas que founder rejeita dizendo "não é o que pedi" | Leo logga quando founder rejeita síntese final |
| **Self-QA honesty rate** | % de tasks em que self-QA do agente foi honesto (não "disse que passou mas falhou em review") | Comparar output de self-QA com resultado de review |
| **Rework cycles** | Número médio de idas e vindas por task antes de aprovação | Leo conta iterações por task |
| **Hiring loop hit rate** | % de tasks onde Manager reconheceu lacuna corretamente (reportou ao Leo) vs tentou sem specialist e quebrou | Comparar hiring requests com failures em domínios não-cobertos |

**Onde os logs vivem:** `~/Github/<projeto>/.claude/metrics/<YYYY-MM>.jsonl` — arquivo por mês, uma entrada por task. Formato simples, legível, greppable. Fora do `outputs/` porque é métrica operacional contínua, não artefato de trabalho.

**Como vira refinamento:** depois de ~20-30 tasks no piloto (2-4 semanas), founder e Leo revisam as métricas juntos. Onde estão os piores números? Essa é a área que precisa de refinamento no core. Evita "achismo" sobre o que está errado.

**Decisão ativa (não parking lot):** métricas entram nas rules universais a serem escritas em Q2. Precisa de uma rule `metrics-collection.md` que todos os agentes carregam e respeitam.

### D10 — Auto-refinamento de agents e skills (inspirado em autoresearch, dois horizontes)

Você trouxe o autoresearch do Karpathy como pergunta: "faz sentido aproveitar pra auto-treinar agents e skills?". A resposta honesta tem **dois horizontes diferentes**, porque autoresearch é um paradigma que se aplica de duas formas no nosso contexto:

#### Horizonte 1 — Aprendizado online (durante uso real)

Já coberto por D7 Mecanismo 3 (post-failure hardening) + D7 Mecanismo 4 (lessons learned pass). Cada falha real detectada em peer review vira proposta de refinamento do agent file, validada pelo founder via R2, aplicada.

**Status:** bakeado nesta sessão. Faz parte de D7.

#### Horizonte 2 — Auto-refinamento offline (loop de treinamento deliberado)

O que você leu originalmente em autoresearch. A ideia é:

1. Founder escolhe um Manager ou skill pra refinar (ex: Dev Manager)
2. Prepara um **benchmark** — conjunto de tasks representativas do domínio com "respostas esperadas" ou critérios de sucesso
3. Roda o Manager atual contra o benchmark, mede resultado
4. Outra instância do agente (ou o founder via Claude) analisa os resultados, propõe mudanças ao agent file
5. Aplica mudança, re-roda benchmark, compara
6. Mantém se melhorou, descarta se piorou
7. Itera até convergir ou até diminishing returns

**Por que isso tem valor:** permite refinar um Manager **antes** de colocar em produção, ou **entre projetos**, sem depender de aguardar falhas reais aparecerem. É o equivalente a "treinar o time antes da temporada começar" — prática profissional normal.

**Por que NÃO é MVP:** três razões concretas:

1. **Precisamos de métricas primeiro.** Sem D9 implementado, não há como medir "melhorou ou piorou". O horizonte 2 depende de D9 funcionar.
2. **Precisamos de benchmark.** Construir um benchmark representativo pra cada Manager é trabalho — envolve coletar tasks passadas, definir critérios, validar que são realistas. Prematura otimização antes do piloto estar rodando.
3. **Precisamos de volume de dados.** Refinar algo sem ter rodado em produção vira chutar no escuro. Ainda que o loop seja fechado, o que é "melhor" depende do que acontece no uso real.

**Quando virar ativo:** depois do piloto do logbook produzir ~1 mês de dados reais (Q7). Com métricas D9 e feedback de uso, dá pra construir um benchmark pra Dev Manager (a disciplina onde temos mais dor observada) e rodar o primeiro loop offline de refinamento. Se funcionar, replica pros outros Managers.

**Status:** adicionado como próximo passo pós-piloto. Não é parking lot indefinido — é parking lot com trigger claro (piloto + 1 mês + métricas suficientes).

### D11 — Parking lot updates

Adicionado ao parking lot do RDD (§9):

- **Estilo configurável por projeto**: founder sugeriu que verbosity (minimalist vs verbose) poderia ser configurável via `project-config.yml`. Decisão: **não fazer agora**. Se algum dia um projeto precisar de estilo diferente (ex: projeto corporativo formal que exige prose explicativa), adiciona. Por ora, minimalist bakeado no core.
- **Tom configurável por projeto**: tom é decidido (casual) mas poderia virar config no futuro se alguém open-source usar em contexto corporativo formal. Não agora.
- **Workflow field no frontmatter**: considerado e rejeitado por redundância com skills. Se isso voltar a ser útil no futuro (ex: workflows que não são skills), reavaliar.
- **Idioma configurado por projeto é decisão ativa (D6), não parking lot.** Confirmado que vai ser config real no setup.
- **Auto-refinamento offline de agents (horizonte 2 de D10)**: não é parking lot indefinido. Tem trigger: piloto do logbook + 1 mês de dados de métricas D9. Depois disso, primeiro loop experimental de refinamento de Dev Manager.

---

## Deferido pra próximas sessões

Estas decisões ficaram **propositadamente em aberto** nesta sessão:

- **Q2 — Conteúdo exato das 10 rules universais**: founder confia no Leo pra redigir rascunho, ele revisa. Próxima sessão de implementação.
- **Q3 — Prompt adversarial do modo review**: parte da rule `peer-review-automatic`. Leo rascunha, founder revisa. Talvez com estrutura "core + especificação por domínio".
- **Q4 — Mecanismo técnico da sub-invocação**: testar Task tool nativo do Claude Code no piloto. Se funcionar, trava. Se não, repensar.
- **Q7 — Estratégia de piloto logbook**: decidir depois de ter os 4 managers escritos
- **Q8 — Migração do Saintfy**: decidir depois do piloto validar modelo

**Q5 (sync.sh) saiu da lista de deferidas** — resolvida em D8.
**Q6 (inicialização de projeto) saiu da lista de deferidas** — resolvida em D6.

---

## Arquivos críticos pra sessão de implementação

**Fonte dos Managers (a ler ao começar a escrever):**
- `~/Github/Saintfy-Copilot/CLAUDE.md` — identidade + regras atuais do Leo + Tomé
- `~/Github/Saintfy-Copilot/.claude/agents/developer.md` — Dev Manager base
- `~/Github/Saintfy-Copilot/.claude/agents/designer.md` — Designer Manager base
- `~/Github/Saintfy-Copilot/.claude/agents/pm.md` — PM Manager base
- `~/Github/Saintfy-Copilot/.claude/agents/marketer.md` — Marketing Manager base
- `~/Github/Saintfy-Copilot/.claude/rules/propagation.md` — rule universal já existente
- `~/Github/Saintfy-Copilot/.claude/rules/design-system.md` — pra Designer Manager
- `~/Github/Saintfy-Copilot/.claude/rules/paper-artboards.md` — pra Designer Manager (será generalizado pra "artboard-conventions" sem menção a ferramenta)

**Memories a consultar como insumo:**
- `feedback_pr_workflow` — base do princípio PR-first do Dev Manager
- `feedback_real_callsite_first` — princípio do Dev Manager
- `feedback_doc_canonical_locations` — base do fluxo PRD→RDD do PM Manager
- `feedback_strategy_before_processing` — base do `think-before-execute` universal
- `feedback_no_inventing_design` — princípio do Designer Manager
- `feedback_reusable_components` — princípio do Dev Manager
- `feedback_shadcn_first_enforcement` — **NÃO** vai pro core (específico do Saintfy), fica na extensão

**Fonte arquitetural:**
- `~/Github/Saintfy-Copilot/docs/rdds/2026-04-08-copilot-core-architecture/rdd.md` — spec completa

---

## Verificação (após sessão de implementação dos Managers)

Quando os 4 managers + Leo estiverem escritos, verificar:

1. **Consistência estrutural** — cada arquivo segue as 5 seções fixas (Papel, Princípios, Hiring loop, Self-QA, Escalation) na ordem, com os nomes exatos
2. **Frontmatter válido** — todos os 6 campos corretos, YAML válido, `model: sonnet` default
3. **Tom e estilo** — leitura rápida (founder lê cada manager em <3 minutos e entende o papel)
4. **Zero menção a projeto específico** — nenhum arquivo do core menciona "Saintfy", "logbook", stack específica, nome de pessoa, credencial, ID
5. **Cross-reference** — `extends` paths fazem sentido; rules de domínio citadas existem ou estão na lista de "a criar na Q2"
6. **Teste de extensão conceitual** — consigo mentalmente imaginar como o Saintfy extenderia o Dev Manager (adicionando shadcn-first) sem conflito?

Verificação empírica só vira possível com o piloto (Q7), que depende de ter os Managers + algumas rules universais prontos.

---

## Próximos passos depois deste plan ser aprovado

1. **Sessão de implementação dos Managers** — escrever Leo + 4 Managers seguindo D1-D5. Saída: 5 arquivos em `Saintfy-Copilot/docs/rdds/2026-04-08-copilot-core-architecture/draft-managers/` (rascunho pra revisão, ainda não é o core final porque o repo `copilot-core` não existe)
2. **Sessão de rules universais (Q2)** — Leo rascunha as 10 rules universais + `metrics-collection.md` (novo, D9), founder revisa
3. **Decisão sobre piloto (Q7)** — com managers + rules prontos, decidir escopo do piloto no logbook
4. **Piloto** — criar repo `copilot-core`, popular com conteúdo aprovado, rodar sync.sh (D8) pra ativar em `~/.claude/`, testar no logbook
5. **Ajustes baseados em piloto** — Q4 (mecanismo sub-invocação via Task tool) se resolve aqui
6. **Coleta de métricas (D9)** — durante ~1 mês de uso real, acumular dados de peer review pass rate, hiring loop hit rate, etc.
7. **Primeiro loop de auto-refinamento (D10 horizonte 2)** — com métricas em mãos, construir benchmark do Dev Manager e rodar loop offline de refinamento estilo autoresearch
8. **Migração Saintfy (Q8)** — só depois do piloto validar modelo

Este plan encerra o Bloco 1. O próximo plan (sessão de implementação dos Managers) vai ser ativo (cria arquivos), não passivo (só decide).
