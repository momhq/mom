---
name: peer-review-automatic
description: Todo trabalho passa por peer review automático e transparente antes de chegar ao founder.
---

## Regra

Nenhum trabalho chega ao founder sem ter passado por peer review. Review é feito por **outra instância do mesmo Manager**, em modo review, com contexto isolado, postura adversarial, e disparada automaticamente via sub-invocação — o founder nunca abre outra sessão manualmente.

## Por que

**Self-QA e peer review pegam coisas diferentes.** Self-QA é o autor checando o próprio trabalho — pega o que o autor consegue ver. Peer review é um par com o mesmo expertise olhando com fresh eyes e sem contexto do raciocínio original — pega viés confirmatório, atalhos, código morto, callsite errado, regressões, edge cases ignorados. Não são redundantes; são camadas complementares.

**Custo humano é o que torna peer review caro em times humanos.** Em IA, custo é segundos e tokens. Não há razão racional pra não ter review universal.

## Fluxo padrão

```
Founder → Leo → Manager recebe
                    ↓
                 Manager decompõe + delega pro specialist do time
                    ↓
                 Specialist executa → self-QA → reporta pro Manager
                    ↓
                 Manager (instância A) revisa — peer review natural
                 (Manager é o tech lead, revisa o time dele)
                    ↓ aprova → sintetiza pro Leo
                    ↓ rejeita → volta pro specialist com comentários
                 Leo → Founder
```

Quando o **Manager executa diretamente** (exceção — micro-tasks, emergência), o review acontece via:

```
Manager executa → self-QA
    ↓
Manager dispara sub-invocação de si mesmo em modo review
(via Task tool nativo do Claude Code, análogo a sub-agents)
    ↓
Nova instância do Manager recebe:
  - Arquivos mudados / diff
  - Output do self-QA
  - Contexto adversarial (modo review)
  - SEM o raciocínio/contexto da sessão de execução
    ↓
Revisa adversarialmente → aprova ou lista problemas
    ↓
Resultado volta pra sessão principal
Founder vê tudo como uma coisa só
```

## Contexto adversarial — template base

Quando o Manager dispara sua própria instância em modo review, o briefing inclui (no mínimo):

```
Você é [Manager name] em modo REVIEW. Outra instância sua acabou de
fazer o trabalho descrito abaixo. Sua função é revisar adversarialmente.

REGRAS DO MODO REVIEW:
1. Você NÃO tem acesso ao raciocínio da sessão que executou. Só aos
   artefatos (diff, arquivos, self-QA).
2. Procure ativamente bugs que o self-QA não pega:
   - Código morto introduzido
   - Regressões em outras partes
   - Callsite errado (o code path real é esse mesmo?)
   - Side effects não intencionais
   - Suposições não verificadas
   - Edge cases ignorados
   - Marca [INFERIDO] escondida sem sinalização
3. NÃO elogie. Se estiver ok, diga "aprovado" e pare.
4. Se houver problema, liste específico e concreto:
   - arquivo:linha
   - o que está errado
   - por que isso é problema
5. Você tem o mesmo expertise do executor — use adversarialmente.

MATERIAL A REVISAR:
[diff / arquivos mudados / output de self-QA]
```

Cada Manager pode **estender** esse template com critérios específicos de review da disciplina dele (Dev Manager adiciona checks de código, Designer checks visuais, etc.).

## Isolamento de contexto é obrigatório

A instância reviewer **não pode** ver o contexto da sessão original. Se ver, o viés confirmatório volta (o reviewer lê "eu escolhi fazer assim porque X" e concorda). Isolamento garante que o reviewer avalie o **resultado** pelo mérito, não pelo raciocínio.

Em termos de Task tool: invoque a sub-sessão com prompt fresco, contendo só os artefatos necessários.

## Iteração

Se o reviewer rejeita:
1. Volta pro agente que executou (ou pro specialist, se foi delegado)
2. Corrige baseado nos comentários específicos
3. Re-submete
4. Nova instância de review (ou a mesma, com novo contexto) revisa novamente
5. Loop até aprovar

Founder não vê cada iteração — recebe o resultado final quando a task está aprovada em review. Se o loop estiver demorando muito (3+ iterações), reporta pro founder pra decidir escalar ou abortar.

## Exceções

**Não há exceções.** Task de 10 segundos também passa por review — review de task de 10 segundos também dura 10 segundos. O custo marginal é zero. Se você está tentado a pular review "porque é simples", é justamente onde bugs passam despercebidos.

A única exceção legítima é quando a task **é** ela mesma uma ação de meta-review (ex: founder pede "revisa esse PR" — aí a task inteira é review, não precisa de review do review).
