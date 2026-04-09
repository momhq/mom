# RDD: Copilot-Core Architecture

**Status:** `Planejamento — aguardando decisões em aberto e piloto`
**Autor:** `Saintfy Copilot (Leo)` conduzindo conversa com founder (Vinícius)
**Data:** `2026-04-08`
**Aplicação:** `Saintfy-Copilot` (método de trabalho) → futuro repo `copilot-core`
**Tracking:** vmarinogg/Saintfy-Copilot#1

---

## Sumário

Este documento descreve a arquitetura proposta para um sistema replicável de "copiloto" baseado em agentes especializados, rules universais e playbooks sob demanda. A meta é transformar o método de trabalho construído empiricamente no Saintfy ao longo de semanas em uma camada genérica reutilizável por qualquer projeto do founder, sem sacrificar a especialização que cada projeto exige.

Este documento **não é** um plano de implementação. É um registro de decisões arquiteturais tomadas durante uma sessão de planejamento em 2026-04-08, com pontos em aberto explicitamente marcados para sessões futuras.

---

## 1. Contexto e motivação

### 1.1. O problema

O founder opera múltiplos projetos simultaneamente — Saintfy (app católico nativo), logbook (app iOS de treino), e outros no horizonte. Cada projeto tem suas particularidades, mas compartilha um **método de trabalho**: conversação com um "gerente" (Leo) que delega para especialistas, rules de processo, decisões canônicas, fluxo PR-first, propagação de contexto.

Hoje esse método vive inteiramente dentro do `Saintfy-Copilot`. Quando o founder abre Claude em outro projeto (logbook, por exemplo), começa do zero: sem Leo, sem rules, sem estrutura. O resultado é frustrante — a versão "crua" do Claude inventa abstrações desnecessárias, repete erros corrigidos em outros projetos, e perde o contexto filosófico que faz o trabalho ter qualidade.

Além disso, dentro do próprio Saintfy, a arquitetura atual tem limites estruturais:

- **Agentes monolíticos.** O Tomé (dev) tem um único agent file que tenta cobrir frontend, backend, Capacitor nativo, Supabase, deploy. Na prática, erra em todos os domínios porque não tem playbook especializado carregado no momento certo. A maratona de Apple Sign-In + Push Notifications (2026-04-06/07) teve 10 bugs encadeados que poderiam ter sido evitados com playbooks específicos.
- **Memories como lixeira.** Várias memories do Leo são, na verdade, skills disfarçadas — playbooks técnicos especializados carregados em toda sessão, mesmo quando irrelevantes. Desperdício de tokens + diluição de contexto.
- **Workflows órfãos.** `workflows/` tem SOPs valiosos que nenhum agente referencia, porque foram escritos para invocação via `/commands` (skills user-invoked) que o founder não usa.
- **Nenhum mecanismo de replicação.** Melhorias no método de trabalho feitas no Saintfy ficam presas no Saintfy.

### 1.2. Objetivos

1. **Extrair método genérico para um núcleo reutilizável** (`copilot-core`) que qualquer projeto novo possa herdar.
2. **Preservar especialização por projeto** — o core fornece a espinha dorsal, cada projeto estende com sua stack, domínio, decisões.
3. **Propagar melhorias automaticamente** — quando o founder refina o método em um projeto, a melhoria fica disponível para todos os outros sem trabalho manual.
4. **Resolver o problema de "Tomé genérico"** com uma hierarquia de agentes que separa processo (Manager), matéria (Domain Expert) e problema técnico específico (Specialist).
5. **Manter a filosofia conversacional** — o founder dirige, os agentes executam, mas sem transformar o método em um sistema autônomo tipo Paperclip que decide sozinho.

---

## 2. Filosofia central

Antes de qualquer decisão técnica, foi necessário nomear a filosofia do sistema porque ela pauta todas as outras decisões.

### 2.1. Copilot conversacional vs. agente autônomo

O mercado atual tem duas filosofias dominantes para agentes de IA em trabalho real:

| Dimensão | **Paperclip-style** | **Copilot-style** |
|---|---|---|
| Unidade de trabalho | Issue/ticket executado autonomamente | Conversa entre founder e agente |
| Modelo de agentes | Flat — "o agente" faz o trabalho | Hierárquico — manager delega pra specialists |
| Especialização | Via tools e integrações | Via skills/playbooks contratáveis |
| Onde o humano entra | Cria issue, recebe PR | Conversa com o manager, aprova cada passo relevante |
| Filosofia | "Delegue e esqueça" | "Converse e guie" |

Este sistema adota explicitamente a filosofia **Copilot-style**. Não é concorrência ao Paperclip — é uma alternativa filosoficamente oposta para um perfil de usuário diferente.

### 2.2. Três eixos de autonomia

A discussão sobre "quanta autonomia dar aos agentes" só fica clara quando se separa o que estava implícito: autonomia não é um eixo, são três.

| Eixo | O que é | Quem decide |
|---|---|---|
| **Estratégica** | O que trabalhar. Prioridades. Direção do projeto. Filtro "serve à missão?" | **Sempre o founder.** Não-negociável. |
| **Tática** | Como executar o que já foi estrategicamente aprovado. Decomposição, delegação, ordenação, execução. | **Leo.** Essa é literalmente a função dele como Manager of Managers. |
| **Criativa/Estrutural** | Criar specialist novo, mudar rule do core, abrir PRD, gastar dinheiro, publicar externamente. | **R2:** agente propõe, founder aprova. Nenhuma mudança estrutural acontece sem aprovação explícita. |

Isso resolve o aparente paradoxo entre "eu quero conversar, não delegar cego" e "eu não quero ficar travando o sistema a cada passo". O founder decide o **o quê**, Leo decide o **como**, e volta pro founder nos **pontos de inflexão**. É gestão humana normal, só que formalizada para agentes.

Este modelo será referenciado como **"R2 com autonomia tática"** no restante do documento.

---

## 3. Arquitetura de distribuição

### 3.1. Onde mora o core

**Decisão: Opção A — user-level via `~/.claude/`.**

```
~/Github/copilot-core/          ← repo git privado (versionado, fonte de verdade)
├── rules/                         (rules universais)
├── agents/                        (Leo + Managers)
├── templates/                     (boilerplate pra projetos novos)
└── README.md

         ↓ sync.sh (rsync)

~/.claude/                      ← Claude Code lê automaticamente em qualquer projeto
├── rules/
├── agents/
└── ...
```

Quando o founder melhora algo no core:
1. Edita em `~/Github/copilot-core/`
2. Commit no repo
3. Roda `bash sync.sh` (ou equivalente)
4. Todos os projetos (Saintfy, logbook, futuros) usam a versão nova na próxima sessão

### 3.2. Por que Opção A

Considerada contra alternativas:

| Alternativa | Por que não |
|---|---|
| **Git submodule** em cada projeto | UX do git é ruim, exige lembrança de atualizar projeto por projeto, confunde. |
| **Sync explícito por projeto** (`copilot sync`) | Drift garantido entre projetos, fonte de verdade fragmentada. |
| **Symlinks** | Não portável entre máquinas, frágil em git. |
| **Template copy por projeto** | Elimina o benefício de atualização global. |

Opção A ganha porque:

1. **Mecanismo nativo do Claude Code** — não luta contra a ferramenta
2. **Zero fricção pra projetos novos** — clona, abre, já tem Leo
3. **Um ponto único de atualização, um ponto único de rollback** — se algo quebrar, `git checkout` no `copilot-core` + `sync.sh` e tá resolvido
4. **Multi-máquina é 1 comando por Mac novo** — `git clone copilot-core && bash sync.sh`

### 3.3. Preservação de futuro multi-modo

Uma preocupação levantada: e quando isso virar produto/open-source? A distribuição ideal muda.

| Horizonte | Modo | Razão |
|---|---|---|
| **Agora** (privado, só founder) | Opção A — user-level sync | Zero fricção, modo de trabalho pessoal |
| **Futuro** (open-source / produto) | Opção C — per-project opt-in com versionamento | Controle por usuário, projetos pinados a versões estáveis |

**Como preservar os dois horizontes em um só desenho:** o `copilot-core` será estruturado como **repo de conteúdo puro** (markdown, sem lógica). O `sync.sh` vive **fora** do repo (scripts pessoais do founder). Isso garante que:

- Hoje: founder usa via sync.sh → `~/.claude/` (Opção A)
- Amanhã: outra pessoa clona e usa via seu próprio mecanismo (Opção C, CLI, etc.) — **sem precisar reescrever o conteúdo**

O conteúdo não muda. O mecanismo de entrega é configurável.

### 3.4. Regras sobre o conteúdo do core

Decididas durante a sessão:

1. **Repo git privado novo**, criado quando chegar a hora de implementar
2. **Zero menção a projetos específicos** em qualquer arquivo do core — nem "Saintfy", nem "logbook", nem "vmarino". Regras universais, templates neutros
3. **Script `sync.sh` fica fora do repo do core** (scripts pessoais do founder em `~/bin/` ou `.zshrc`)
4. **Credenciais, IDs, nomes de serviço** nunca entram no core — ficam sempre no projeto

---

## 4. Hierarquia de agentes

### 4.1. Três tipos distintos de agentes contratáveis

Durante a sessão foi descoberto que "agente" é um conceito que agrega três coisas diferentes. Cada uma merece tratamento diferente.

| Tipo | Papel | Vive onde | Quem cria | Ciclo de vida | Exemplo |
|---|---|---|---|---|---|
| **Manager** | **Tech lead** da disciplina — recebe, decompõe, delega, revisa, sintetiza. Executa só em exceção | **Core** (universal) | Já existe no core; estendido pelo projeto | Permanente | Dev Manager, Design Manager |
| **Domain Expert** | Consultor de matéria/assunto (o que é verdade sobre X) | **Projeto** | Hiring Loop (proposto pelo founder) | Permanente no projeto | Teólogo (Saintfy), Strength Coach (logbook) |
| **Specialist** | Executor de tarefas técnicas — compõe o "time" do Manager | **Projeto** | Hiring Loop (proposto pelo Manager) | Criado sob demanda, permanente no projeto | Frontend dev, APNs protocol, SwiftData |

**Distinção crítica entre Manager e Specialist:**

O Manager **não é o executor padrão**. Ele é o tech lead do time dele. Quando recebe task do Leo, ele **delega pros specialists do time**, revisa o trabalho deles, e sintetiza o resultado pro Leo. Pode executar diretamente em exceções (micro-tasks, emergência, criação de briefing), mas o padrão é delegação.

**Distinção crítica entre Domain Expert e Specialist:**

- **Specialist** é contratado pelo **Manager** para resolver **tarefas técnicas**. Seu conteúdo é playbook acionável e ele **executa** trabalho concreto.
- **Domain Expert** é contratado pelo **founder** (via Leo) como **consultor permanente** do projeto. Seu conteúdo é conhecimento amplo sobre a matéria, consultado em várias decisões ao longo do tempo. Ele **não executa** — é referência consultiva.

Um Dev Manager + um Teólogo colaboram: Dev Manager lidera implementação da feature com seu time de specialists; Teólogo valida se a feature bate com doutrina. Nenhum substitui o outro.

### 4.2. Managers no core (lista inicial)

Seis managers compõem o "time padrão" que qualquer projeto pode usar:

1. **Leo** — Manager of Managers. Único com esse papel. Coordena, delega, tem big picture cross-projeto.
2. **Dev Manager** — Processo de desenvolvimento de software. Rules de domínio: PR workflow, debugging sistemático, callsite real primeiro, self-QA com prova.
3. **Design Manager** — Processo de design. Rules de domínio: source of truth, não inventar elementos não definidos, design system como autoridade.
4. **Marketing Manager** — Processo de marketing e growth. Rules de domínio: ASO fundamentals, content calendar, brand voice.
5. **Research Manager** — Processo de pesquisa. Rules de domínio: credibilidade de fontes, primária vs secundária, síntese.
6. **Writing Manager** — Processo de escrita e comunicação. Rules de domínio: voz por audiência, estrutura, CTA.
7. **Product Manager** — Processo de produto. Rules de domínio: filtro de feature, PRD→RDD handoff, detecção de scope creep.

Notas:

- A estrutura **suporta "contratação" de novos managers** via conversa com Leo. Exemplo dado pelo founder: um manager com "formação teológica" para validar conteúdo do Saintfy, ou um "strength coach" para o logbook. Domain Experts cobrem a maior parte desses casos, mas um manager novo faz sentido se for uma **disciplina profissional** recorrente, não um corpo de conhecimento.
- Não foi incluído "DevOps manager" ou "Data manager" inicialmente — não há dor real nesses domínios nos projetos atuais. Podem ser adicionados via hiring loop quando surgirem.
- **Time mínimo de cada Manager cresce orgânico, não por decisão antecipada.** O core entrega os Managers "vazios de time". Nas primeiras interações com um projeto, o Manager identifica quais specialists precisa baseado na stack e nas tasks reais, e dispara Hiring Loop com o Leo para contratá-los. Nenhum specialist é pré-definido no core. Isso mantém o core enxuto e honesto com o princípio "nada no core sem evidência de uso real".

### 4.3. Leo — Manager of Managers

Leo tem papel único e não é "só mais um manager". Responsabilidades exclusivas:

1. **Roteamento** — recebe pedidos do founder, identifica o domínio, delega ao manager certo
2. **Hiring Loop contractor** — quando um manager reporta lacuna, Leo contrata o especialista (ver §4.4)
3. **Big picture cross-projeto** — Leo enxerga outros projetos em `~/Github/` quando a tarefa exige referência (ex: "já implementamos push em outro projeto?")
4. **Propagação de contexto** — Leo é o responsável final por garantir que decisões propagam para memories, docs canônicos, rules relevantes
5. **Síntese para o founder** — consolida trabalho dos managers em reports acionáveis

### 4.4. Hiring Loop

Decisão central: **reconhecer uma lacuna** e **preencher uma lacuna** são responsabilidades separadas, atribuídas a papéis diferentes.

```
Manager:  "preciso de alguém que manje APNs protocol profundamente"
       ↓ solicitação estruturada (o quê, pra quê, escopo)
Leo:      vê big picture (projetos, specialists existentes em outros
          projetos, memories, decisões, restrições)
       ↓ formata o specialist correto
Leo:      "contratado. Specialist `apns-push-protocol` criado.
          Devolvendo pro Dev Manager."
Manager:  delega a tarefa ao specialist, que executa
```

**Hiring Loop é usado em dois casos distintos:**

1. **Constituir time inicial do Manager** — nas primeiras interações com um projeto, o Manager identifica quais specialists generalistas precisa (ex: frontend dev, backend dev) e dispara Hiring Loop pra constituir seu time básico.
2. **Preencher lacuna de domínio específico** — quando uma task exige expertise profunda que o time atual não cobre (ex: APNs protocol, WebCrypto), Manager dispara Hiring Loop pra contratar specialist específico.

Em ambos os casos, specialists **vivem sempre no projeto**, nunca no core.

**Por que Leo contrata e não o próprio Manager:**

1. **Leo enxerga duplicação** — se outro manager já pediu specialist parecido, Leo lembra. Manager sozinho não tem esse olhar cruzado.
2. **Leo enxerga reuso cross-projeto** — se outro projeto já tem specialist similar, Leo propõe adaptar.
3. **Leo impõe padrão estrutural** — frontmatter, formato, nível de detalhe. Evita specialists bagunçados.
4. **Manager fica focado** — pediu, voltou pra executar. Não perde contexto na meta-tarefa de "escrever specialist".

Isso espelha como headhunting funciona em empresas reais. Gerente de engenharia diz "preciso de iOS sênior com push". RH/CTO escreve JD, busca, entrevista, contrata, entrega. Gerente executa. Separation of concerns.

**Regra adicional crítica:** Managers **param e reportam ao Leo antes de executar** quando reconhecem que o domínio está fora de sua capacidade. Isso é explicitado como rule universal `know-what-you-dont-know`, aplicável a todos os agentes. É antídoto direto ao padrão "Tomé achou que sabia e não sabia" — a principal causa da maratona de bugs 2026-04-06/07.

### 4.5. Manager como tech lead: delegação e peer review

O Manager não é o executor padrão — ele é o **tech lead** do time dele. O fluxo padrão de uma task segue esta sequência:

```
Founder → Leo → Manager recebe
                    ↓
                 Manager decompõe + decide quais specialists do time usar
                    ↓
                 Manager delega com briefing
                    ↓
                 Specialist executa → self-QA → reporta pro Manager
                    ↓
                 Manager revisa (peer review natural, do mesmo domínio, mais sênior)
                    ↓ aprova → sintetiza pro Leo
                    ↓ rejeita → volta pro specialist com comentários
                 Leo → Founder
```

**Review está embutido no papel do Manager.** Não é uma etapa extra nem um agente separado — é parte do que significa ser tech lead da disciplina. Manager revisa specialist do mesmo domínio porque tem o expertise pra fazê-lo com rigor.

#### Exceção: quando o Manager executa diretamente

Em alguns casos faz sentido o Manager executar em vez de delegar:

- Task tão pequena que criar/contratar specialist vira overhead (mudar uma cor, renomear arquivo)
- Task meta que é inerentemente do Manager (planejar decomposição, escrever briefing pra specialist)
- Emergência em que specialist não está disponível e Manager precisa assumir

Nesses casos, o review acontece via **sub-invocação transparente do próprio Manager em nova instância**, com isolamento de contexto:

```
Manager executa task pequena → self-QA
    ↓
Manager dispara sub-invocação de si mesmo em modo review
    ↓ (mesma sessão do founder, sem intervenção manual,
       análogo a como Claude Code dispara sub-agents via Task tool hoje)
Nova instância do Manager recebe só o output (diff + self-QA),
SEM acesso ao contexto/raciocínio da execução
    ↓
Revisa adversarialmente, aprova ou pede ajuste
    ↓
Resultado volta pra sessão principal do founder, que recebe
tudo junto: execução + review + resultado final
```

**Propriedade crítica:** o founder **nunca abre outra sessão manualmente**. Todo o ciclo de execução + review acontece dentro da sessão em que o founder está trabalhando, de forma transparente. O founder vê o progresso (análogo a ver um sub-agent rodando), recebe o resultado final, e pronto.

O isolamento de contexto da instância reviewer é obrigatório — sem ele, o viés confirmatório volta ("eu decidi assim porque X, Y, Z" → reviewer lê e concorda). A nova instância recebe apenas os artefatos: arquivos mudados/diff, relatório de self-QA, e o contexto de review ("você é [Manager name] em modo review, seja adversarial, procure bugs que self-QA não pega").

#### Por que mesmo Manager em modo review (e não Reviewer separado no core)

Foi considerada a opção de ter Reviewers separados 1:1 com Managers no core. Rejeitada porque:

1. **Fragmenta conhecimento.** Atualizar o Dev Manager exigia atualizar o Dev Reviewer em paralelo. Drift garantido.
2. **Uma só fonte de verdade.** Expertise técnico vive num só arquivo. Modo de invocação muda a lente, não o conteúdo.
3. **Espelha empresas reais.** Não existe cargo "iOS Reviewer" — é um iOS sênior revisando código de outro iOS. Mesma pessoa, mesma expertise, papel diferente.
4. **Projeto estende uma vez.** `.claude/agents/managers/dev.md` com `extends: core/managers/dev.md` serve pra execução E pra review. Sem duplicação.

---

## 5. Rules em dois escopos

### 5.1. A descoberta do escopo

Durante a sessão, ficou claro que "rule" é uma categoria que agrega duas coisas muito diferentes:

- **Como a empresa trabalha** — princípios filosóficos que valem pra qualquer agente em qualquer domínio
- **Como aquele time trabalha** — práticas técnicas específicas de um domínio profissional

Misturar os dois no mesmo balaio gera:
- Overhead de tokens (marketing não precisa carregar PR workflow)
- Confusão sobre o que é universal vs específico
- Dificuldade de evoluir uma sem afetar a outra

### 5.2. Rules universais (core/rules/)

Carregam **sempre**, em qualquer sessão, pra qualquer agente ativo. Definem a "constituição" do sistema.

| Rule | O que impõe |
|---|---|
| `propagation.md` | Toda decisão/mudança deve propagar pra memories, context, rules relevantes antes de fechar task |
| `anti-hallucination.md` | Resposta errada é 3x pior que "não sei". Marcar `[INFERIDO]` quando não veio de fonte verificável |
| `think-before-execute.md` | Em tasks ambíguas/complexas, perguntar antes de implementar. Em diretas, ir direto. Critério claro |
| `evidence-over-claim.md` | Nunca reportar trabalho como concluído sem evidência verificável anexada. Cada domínio define sua forma de evidência (build/test/lint pra dev, screenshot pra design, rascunho completo pra marketing, fontes citadas pra research, etc). O founder não deve precisar acreditar — deve poder conferir |
| `peer-review-automatic.md` | Todo trabalho passa por peer review antes de chegar ao founder. Review é feito por outra instância do mesmo Manager (modo review, contexto isolado, adversarial), disparada automaticamente via sub-invocação transparente — founder nunca abre outra sessão manualmente |
| `state-vs-learning.md` | Memories de estado envelhecem rápido, memories de aprendizado permanecem. Estado precisa ser revalidado antes de citar |
| `hiring-loop.md` | Manager reporta lacuna → Leo contrata specialist → devolve pro Manager. Usado tanto pra constituir time inicial quanto pra preencher lacunas de domínio específico |
| `know-what-you-dont-know.md` | Manager detecta domínio fora de sua capacidade → para e reporta lacuna ANTES de executar |
| `escalation-triggers.md` | Lista explícita de situações que sempre param o agente e forçam pergunta ao founder (gasto de dinheiro, publicação externa, ação destrutiva, criação de specialist/manager, mudança em rule do core, contradição entre rules existentes) |
| `inheritance.md` | Quando um agente tem `extends` no frontmatter, carregar o arquivo base antes de executar e concatenar comportamento |

### 5.3. Rules de domínio

Vivem **dentro do agent file do manager correspondente**, seja embutidas no próprio markdown ou em arquivos referenciados pelo frontmatter. Carregam **apenas quando** o Leo delega pra esse manager.

Exemplos do Dev Manager:
- PR workflow (worktree + branch + PR + Closes #N)
- Debugging 3-strikes
- Real callsite first
- Self-QA checklist específico: build output, lint, type check, prova de execução do code path real
- Code review checklist

Exemplos do Design Manager:
- No inventing design elements
- Design system as source of truth
- Artboard conventions (ferramenta-agnóstico)
- Self-QA específico: screenshot de comparação com spec, link pro artboard, verificação de tokens

Exemplos do Product Manager:
- Feature filter (serve à missão?)
- PRD→RDD handoff
- Scope creep detection
- Self-QA específico: PRD com todas as seções preenchidas, links rastreáveis, decisões explícitas

Nota sobre self-QA: a rule universal `evidence-over-claim` exige **que haja evidência**. O tipo de evidência e o checklist específico vivem como rule de domínio dentro de cada Manager, porque a forma de provar que dev trabalhou é diferente da forma de provar que designer trabalhou.

### 5.4. Regras específicas de projeto

`shadcn-first-enforcement` (Saintfy), `swiftui-conventions` (logbook), e equivalentes ficam no **projeto**, não no core. O core não impõe nenhuma stack específica.

---

## 6. Inheritance — como projeto estende core

### 6.1. Forma 1: extends via frontmatter

**Decisão:** projeto declara extensão explícita via campo `extends` no frontmatter.

```markdown
---
name: Dev Manager (Saintfy)
extends: core/agents/managers/dev.md
---

Além das rules e princípios do core/managers/dev.md, você também:

- Trabalha com stack React + TypeScript + Vite + shadcn/ui + Supabase + Capacitor
- Segue as rules específicas de shadcn-first-enforcement
- Debug prioritário em iOS nativo (Capacitor)
...
```

**Mecanismo:** quando Leo delega para um manager, ele lê o agent file do projeto, vê o `extends`, lê o arquivo do core, concatena ambos em ordem (core primeiro, projeto depois) e passa como briefing final.

### 6.2. Por que extends ganhou

Considerada contra:

- **Compile step** (script gera arquivo único) — build step adiciona complexidade, fragmenta fonte de verdade
- **Template copy** (core vira template copiado no projeto) — mata o benefício de atualização global

Extends ganha porque:

1. **Não quebra atualização global.** Core atualiza, projeto automaticamente puxa o conteúdo novo na próxima sessão.
2. **Explícito sobre mágica.** Você lê o arquivo do projeto, vê o `extends`, sabe exatamente o que vai ser concatenado.
3. **Estende em vez de sobrescrever.** Filosofia da arquitetura é clara: projeto **adiciona** conhecimento, nunca substitui o core. Bugs, inconsistências e perda de qualidade ficam confinados ao projeto.
4. **Rastreável.** `extends: core/managers/dev.md` deixa claro o que está sendo herdado.

### 6.3. Rule universal que sustenta o mecanismo

`rules/inheritance.md` no core instrui o Leo (e qualquer agente que delegue a outro) a respeitar o `extends`:

> Quando um agente tem campo `extends` no frontmatter, carregue o arquivo base antes de executar e concatene o comportamento. Ordem: core primeiro, projeto depois. O projeto não pode remover comportamento do core — apenas adicionar ou refinar.

---

## 7. Multi-surface — Remote Control como surface secundário

### 7.1. O problema original

O founder usa hoje Claude via VS Code extension, com acesso completo ao filesystem e estrutura do Copilot. Funciona bem mas tem um limite óbvio: **só funciona quando o founder está no Mac**. Ideias capturadas em campo (conversa com pessoal da paróquia, treino no ginásio) ficam fora do sistema até o founder voltar ao computador.

A dor real: "quero mandar uma mensagem pro Leo do Saintfy do celular e registrar uma ideia".

### 7.2. A descoberta

Durante a sessão, foi investigada a documentação oficial de **Cowork/Dispatch** e **Remote Control** (ambos features do Claude ecosystem). Conclusão:

| Feature | Funciona como | Adequação ao modelo Copilot |
|---|---|---|
| **Cowork/Dispatch** | "Delegue e esqueça" — task executada em background, resultado via push notification | **Contradiz filosofia conversacional.** Research preview, instabilidade, "instructions from phone can trigger real actions". Parking lot. |
| **Remote Control** | Conecta o celular a uma sessão Claude Code rodando localmente no Mac — mesma sessão, múltiplos devices | **Preserva filosofia 100%.** Mesma `~/.claude/`, mesmo projeto, mesmas memories, mesmo contexto. Maduro, GA. |

### 7.3. Decisão: Remote Control é o surface secundário recomendado

**Setup conceitual:**

1. Founder roda `claude remote-control --name "Saintfy"` no Mac do projeto (ou deixa rodando em background)
2. Sai pra paróquia / treino / café
3. Abre Claude mobile, vê "Saintfy" na lista de sessões
4. Conversa normalmente — é literalmente a mesma sessão do Mac, vista de outro device

**Propriedades importantes:**

- Nada move pra cloud. O Claude continua rodando localmente no Mac.
- Filesystem, MCP servers, `.claude/`, agents — tudo fica disponível igual à sessão local.
- Conversa sincroniza entre devices em tempo real. Founder pode começar no celular e continuar no VS Code ou vice-versa.
- Reconecta sozinho se laptop dormir ou rede cair.
- Funciona em Pro/Max (o plano do founder cobre).

### 7.4. Implicação pro copilot-core: nenhuma

Porque Remote Control é só uma forma de **acessar** a mesma sessão local, **nada no core muda**. A arquitetura desenhada nas seções 3-6 funciona idêntica em VS Code, terminal e mobile. O único adicional é o comando `claude remote-control` que o founder roda quando quer ativar o surface secundário.

### 7.5. Cowork fica no parking lot

Razões explícitas pra não adotar agora:

1. **Research preview** — instabilidade esperada, não vale apostar arquitetura nisso
2. **Filosofia errada** — "delegate and forget" contradiz "converse and guide"
3. **Risco de segurança** — instruções remotas disparando ações locais sem checkpoint
4. **Duplicação** — Remote Control resolve o caso de uso real ("mandar mensagem pro Leo do celular") sem os downsides

**Quando reconsiderar:** se Cowork sair de preview e o founder identificar caso de uso genuíno "modo Paperclip" pra tarefas massivas (ex: "execute esse refactor de 50 arquivos enquanto eu janto"). Por ora, não.

---

## 8. Decisões do Bloco 1 (sessão 2026-04-08, parte 2)

Depois da redação inicial deste RDD, uma segunda sessão resolveu o "Bloco 1" das questões em aberto — decisões de forma que destravam todo o resto. Plan file correspondente em `~/.claude/plans/snoopy-prancing-corbato.md`.

### 8.1. D1 — Estilo dos Managers: Minimalist

Identidade + princípios + checklist, sem prose longa. Manager é tech lead operando, não tutorial pra iniciante. Racional: tokens importam em sessões longas, memories já carregam, rules universais já carregam — Manager não precisa repetir.

### 8.2. D2 — Tom: Casual, idioma configurável

Segunda pessoa, zero corporativês ("Você é o tech lead de dev" não "Você é o líder técnico responsável pela disciplina de..."). Idioma é decidido no setup do projeto via `.claude/project-config.yml`, não bakeado no core. Founder tem preferência pessoal de interagir com IA em PT mas manter código/docs em EN — config legítima por usuário/projeto.

### 8.3. D3 — Frontmatter: 6 campos fixos

```yaml
---
name: <Nome do Manager>
description: <frase curta do papel>
extends: <path relativo ao arquivo base>    # opcional
tools: Read, Edit, Write, Glob, Grep, Bash, Task
model: <haiku|sonnet|opus>
skills: [...]
---
```

- **`Task` tool** incluída por padrão — necessária pra sub-invocação de peer review automático (Q4)
- **`workflows` não adicionado** — workflows importantes viram skills
- **`memory` não adicionado** — memories devem ser universais por sessão

**Seleção de model:**

| Model | Quando | Default pra |
|---|---|---|
| `opus` | Big picture, coordenação, hiring loop, síntese | Leo (sempre) |
| `sonnet` | Execução padrão: código, review, delegação | Todos Managers |
| `haiku` | Trabalho mecânico de baixo raciocínio | Specialists mecânicos |

Projeto pode override o model via `extends` — ex: `model: opus` no Dev Manager do projeto se houver evidência empírica de sonnet errando.

### 8.4. D4 — Estrutura interna fixa dos Managers

Toda Manager file segue esta ordem:

1. **Papel** — 1-2 frases (o que é, quando delega, quando executa em exceção)
2. **Princípios** — bullets curtos (3-5 princípios centrais)
3. **Hiring loop** — 1-2 frases sobre quando disparar contratação de specialist
4. **Self-QA** — checklist de prova específica da disciplina
5. **Escalation** — lista concreta do que para o agente e força pergunta ao founder

Seções extras só se justificadas pela natureza do domínio.

### 8.5. D5 — Managers iniciais: Leo + 4

Primeira leva: **Leo, Dev Manager, Designer Manager, PM Manager, Marketing Manager**. Cobrem todas as dores observadas no Saintfy e logbook. Research e Writing entram quando houver necessidade real.

### 8.6. D6 — Fluxo de inicialização de projeto (`copilot init`)

Visão documentada do founder:

1. **Bootstrap de máquina (primeira vez):** clona `copilot-core`, roda `sync.sh` pra popular `~/.claude/` com symlinks
2. **Project setup:** pergunta path(s) do repo(s), idioma de interação, idioma dos arquivos. Grava em `.claude/project-config.yml`
3. **Scaffolding:** cria estrutura vazia em `.claude/agents/managers/`, `rules/`, `context/project.md`, `specialists/`
4. **Context collection interativa:** sessão com Leo entrevistando founder, coletando arquivos de contexto (PRDs, docs, README), sintetizando primeira versão de `context/project.md`
5. **Encerramento:** commit do scaffolding, pronto pra trabalhar
6. **Uso normal:** founder conversa, managers operam, contexto enriquece organicamente

**Updates do core:** `cd ~/Github/copilot-core && git pull`. Graças aos symlinks do D8, updates são imediatos — re-rodar `sync.sh` só quando topologia muda (arquivos adicionados/removidos).

### 8.7. D7 — Hiring loop enforcement: forçando o modelo a reconhecer lacunas

**Problema identificado pelo founder:** Claude "sabe" fazer quase tudo superficialmente. Rule dizendo "peça specialist quando não souber" não basta — modelo vai achar que sabe.

**4 mecanismos combinados na rule `know-what-you-dont-know`:**

1. **Pre-execution check obrigatório**: template que força o Manager a colar resposta escrita (não só pensar) sobre domínio da task, specialist disponível, pior cenário de erro, confiança justificada
2. **Trust gradient por categoria**: tabela específica por Manager listando categorias com "sempre specialist" dura (crypto, auth, native bridging, infra — pro Dev Manager)
3. **Post-failure hardening**: peer review rejection → Leo adiciona lacuna ao trust gradient do projeto automaticamente
4. **Lessons learned pass**: após review rejection, agente preenche formulário "qual rule teria prevenido isso?", propõe refinamento ao core ou ao projeto via R2

### 8.8. D8 — sync.sh: symlinks por arquivo

Script vive em `copilot-core/scripts/sync.sh`. Design escolhido após rejeitar `rsync --delete` (perigoso), git submodule (UX ruim), symlink de diretório inteiro (apaga locais).

**Mecanismo:** symlink individual de cada `.md` de `copilot-core/agents/` e `copilot-core/rules/` pras pastas correspondentes em `~/.claude/`.

**Propriedades:**
- Idempotente (safe rodar N vezes via `ln -sf`)
- Zero-copy após primeiro run — `git pull` atualiza conteúdo via symlinks
- Re-run só quando topologia muda
- Arquivos locais do founder (`memory/`, `settings.json`, `projects/`) preservados
- Rollback trivial: `git checkout <rev>` no core
- Dangling cleanup automático

Script concreto está no plan file.

### 8.9. D9 — Métricas de outcome (inspirado em autoresearch)

Autoresearch do Karpathy insiste em fitness function mensurável. Adicionando **5 métricas básicas** pra coletar desde o piloto:

- **Peer review pass rate** — % de tasks que passam na primeira
- **Founder rejection rate** — % de entregas rejeitadas pelo founder
- **Self-QA honesty rate** — % de self-QA honestos (não "passou" que falhou em review)
- **Rework cycles** — média de idas e vindas por task
- **Hiring loop hit rate** — % de vezes que Manager reconheceu lacuna vs tentou sem specialist

Logs vivem em `.claude/metrics/<YYYY-MM>.jsonl`. Nova rule universal `metrics-collection.md` define formato e responsabilidade.

### 8.10. D10 — Auto-refinamento de agents (dois horizontes)

**Horizonte 1 — Online (durante uso real):** bakeado em D7 Mecanismo 3 + 4. Falhas reais viram propostas de refinamento via R2.

**Horizonte 2 — Offline (benchmark deliberado):** replicar paradigma autoresearch do Karpathy. Founder prepara benchmark de tasks representativas do domínio → roda Manager contra benchmark → agente propõe mudanças no agent file → re-roda → mantém se melhorou. **Não é MVP** — depende de métricas (D9) e volume de dados reais do piloto. Trigger: ~1 mês de piloto logbook com dados suficientes.

### 8.11. D11 — Parking lot updates

Adicionado: estilo/tom configurável por projeto (não agora), workflow field no frontmatter (rejeitado por redundância com skills), auto-refinamento offline com trigger claro (piloto + métricas).

---

## 9. Perguntas em aberto (após Bloco 1)

5 questões restantes pra próximas sessões:

1. **Q2 — Conteúdo exato das 10 rules universais.** Leo rascunha, founder revisa. Próxima sessão.
2. **Q3 — Prompt adversarial do modo review.** Parte da rule `peer-review-automatic`. Possivelmente "core + especificação por domínio".
3. **Q4 — Mecanismo técnico da sub-invocação.** Testar Task tool nativo do Claude Code no piloto. Se funcionar, trava. Se não, repensar.
4. **Q7 — Estratégia de piloto logbook.** Decidir depois de ter Managers + rules escritos.
5. **Q8 — Migração do Saintfy.** Só depois do piloto validar modelo.

**Resolvidas no Bloco 1:**
- ~~Q1 (conteúdo exato dos managers)~~ — forma resolvida (D1-D5), conteúdo em implementação ativa
- ~~Q5 (sync.sh)~~ — resolvida em D8
- ~~Q6 (inicialização de projeto novo)~~ — resolvida em D6

---

## 9. Parking lot (ideias futuras)

Capturadas durante a sessão para não perder. **Não entram no MVP.**

- **Nomes aleatórios para managers por projeto.** Ideia de produto: quando um projeto novo é inicializado, managers recebem nomes únicos (tipo "gracie" pro design manager do logbook). Gera personalidade, ajuda branding. Não é MVP — cabe quando o core virar produto.
- **`.claude/project.yml` declarando managers ativos.** Opção B do debate sobre quais managers carregar. Decisão atual foi Opção A (todos sempre ativos, Leo decide por contexto) porque é mais simples. Quando o peso morto de managers irrelevantes incomodar, migrar pra B.
- **Hooks determinísticos via `update-config`.** Algumas memories/rules (`lint-before-accept`, `pr-workflow` no que diz respeito a commits) são "enforcement behavior" que markdown não garante. Hooks do Claude Code resolvem — mas são complexidade separada. Vale uma sessão dedicada.
- **`cross-repo-reference.md` como rule formal.** Decidido que, por ora, founder pede explicitamente quando lembrar. Quando o reuso cross-projeto ficar frequente, formalizar como rule.
- **`copilot-core` como produto ou open-source.** Todo o desenho já é compatível. Quando o founder decidir fazer esse salto, o conteúdo já está pronto — só muda mecanismo de distribuição (Opção A → Opção C com CLI/npm/etc).
- **Channels e Scheduled Tasks.** Descobertos durante pesquisa sobre multi-surface. Channels encaminha mensagens de Telegram/Discord/iMessage pra sessão Claude. Scheduled tasks roda rotinas recorrentes. Não recomendados agora — vale saber que existem.
- **`morning-brief.md` como rule.** Sugerido durante a sessão e **rejeitado explicitamente** pelo founder: "minha rotina ideal é entrar no board, ver issues, e sair pedindo". Documentado pra não voltar a propor.
- **`autonomy-audit-trail.md` como rule.** Sugerido durante a sessão e rejeitado depois da descoberta do Remote Control: não há mais trabalho rodando "sem founder ver", então audit trail perde propósito.

---

## 10. Próximos passos

**Esta sessão encerra em planejamento.** Nenhum código ou configuração foi implementado durante a conversa. Ordem proposta para sessões futuras:

1. **Próxima sessão dedicada (planejamento, continuação)**
   Resolver as 6 questões em aberto (§8). Produto: versão final deste RDD, pronta para implementação.

2. **Piloto no logbook**
   - Criar repo privado `copilot-core` com estrutura mínima (Leo + Dev Manager + rules universais)
   - Escrever `sync.sh`
   - Ativar em `~/.claude/`
   - Abrir logbook no Claude, rodar sessão real de trabalho
   - Validar: Leo funciona? Dev Manager carrega? Extends funciona? Founder sente diferença qualitativa vs "Claude cru"?
   - Ajustar baseado em observação real

3. **Migração do Saintfy**
   Depois do piloto bem-sucedido, decidir e executar estratégia de migração (questão em aberto §8.6).

4. **Expansão do core**
   Adicionar managers restantes conforme demanda real surgir. Não criar por antecipação.

5. **Iteração e refinamento**
   Método de trabalho **replicável** significa refinamento contínuo. Cada projeto novo é oportunidade de descobrir lacunas do core.

---

## 11. Princípios que emergiram durante a sessão

Registrados para não se perderem — podem virar rules universais no futuro:

1. **Reconhecer é diferente de preencher.** Manager reconhece lacuna, Leo preenche. Separação de responsabilidades reflete empresas reais.
2. **Autonomia não é um eixo, são três.** Estratégica sempre do founder, tática do Leo, criativa/estrutural em R2.
3. **Estender em vez de sobrescrever.** Projeto adiciona conhecimento ao core, nunca remove. Bugs ficam confinados ao projeto.
4. **State memories vs learning memories.** Estado envelhece rápido e precisa revalidação. Aprendizado permanece. Misturar os dois é dívida.
5. **Filosofia pauta mecanismo.** Cowork é ótimo tecnicamente mas filosoficamente errado pra este modelo. Escolha de ferramenta vem depois da escolha de filosofia.
6. **Informação desatualizada é pior que informação ausente.** Regra de propagação existe porque memory stale gera erro silencioso — pior que não ter memory nenhuma.
7. **Manager é tech lead, não implementador.** O papel do Manager é decompor, delegar, revisar, sintetizar — executar é exceção. Specialists do time dele executam. Isso espelha prática profissional real de engenharia sênior.
8. **Autor checa, par valida.** Self-QA e peer review são camadas complementares, nunca redundantes. Self-QA pega o que o autor consegue checar; peer review pega o que o autor não vê por estar imerso no próprio raciocínio. Ambas são obrigatórias.
9. **Review transparente ao founder.** O founder nunca abre outra sessão manualmente pra revisar trabalho. Todo o ciclo execução → self-QA → peer review → ajuste → aprovação acontece dentro da sessão em que o founder está trabalhando, análogo a como Claude Code dispara sub-agents hoje. O founder vê o progresso e recebe o resultado final, não coordena o meio do caminho.
10. **Nada no core sem evidência de uso real.** Specialists, managers novos, rules novas — tudo entra no core só depois de provar utilidade em projeto real. Decisão antecipada é fonte de dívida.

---

## Apêndice A — Referências cruzadas

**Memories relevantes** (Saintfy-Copilot/memory/):
- `feedback_doc_canonical_locations` — PRDs em `saintfy/docs/prds/`, RDDs em `saintfy/docs/rdds/`
- `feedback_pr_workflow` — fluxo PR+worktree+Closes #N estabelecido 2026-04-07
- `feedback_real_callsite_first` — origem do princípio "grep callsite real antes de delegar refactor"
- `feedback_strategy_before_processing` — razão de ter feito este RDD antes de implementar
- `project_session_2026_04_06_07_recap` — maratona que expôs os limites da arquitetura atual
- `project_copilot_replicable` — primeira menção à meta de replicabilidade

**Issue tracker:**
- vmarinogg/Saintfy-Copilot#1 — issue mãe da arquitetura de skills

**Documentação oficial investigada:**
- https://support.claude.com/en/articles/13947068 — Cowork/Dispatch
- https://code.claude.com/docs/en/remote-control — Remote Control

---

**Fim do RDD.**

Este documento é a fotografia da sessão de planejamento de 2026-04-08. Não é fonte viva — representa as decisões tomadas nesta data. Mudanças estruturais na arquitetura geram novos RDDs; ajustes menores são registrados em commit no repo `copilot-core` quando ele existir.
