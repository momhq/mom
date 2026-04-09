---
name: escalation-triggers
description: Lista explícita de situações que param o agente e exigem pergunta ao founder, sem negociação.
---

## Regra

Tem situações em que **nenhum agente** pode proceder sem aprovação explícita do founder, por mais "óbvio" que pareça o próximo passo. Esta lista é o contrato — não é sugestão.

Quando você encontrar qualquer um dos triggers abaixo, **pare**, documente a situação, apresente as opções ao founder, e espere resposta. Não "vá avançando enquanto founder decide" — ações tomadas sem resposta podem gerar trabalho perdido ou problema real.

## Triggers universais (aplicam a todos os agentes)

### 💸 Gasto de dinheiro

- Rodar comando que chama API paga (image gen, LLM de outro provider, transcrição, etc.)
- Deploy que consome budget de serviço cloud
- Compra de domínio, licença, plugin, font
- Ad spend, promoção paga
- Qualquer coisa que apareça na fatura do founder

### 📢 Publicação externa

- Git push pra `main`/`master` em repo público
- Merge de PR
- Publicação de post em rede social
- Envio de email pra mailing list
- Release pra app store (TestFlight, Play Store)
- Publicação de página em site vivo

### ⚠️ Ação destrutiva

- `rm -rf`, `git reset --hard`, `git push --force` em branch compartilhada
- Drop table, delete em massa no banco
- Revogação de credencial, chave, secret
- Deleção de issue, PR, branch com histórico relevante
- Overwrite de arquivo que pode ter conteúdo não commitado

### 🏗️ Mudança estrutural

- Criação de specialist ou manager novo (via Hiring Loop, mas com R2 do founder)
- Adição ou modificação de rule **no core**
- Mudança em decisão registrada em `context/decisions/`
- Mudança em arquitetura que não estava no escopo da task original
- Alteração em frontmatter de agent file (especialmente `extends`, `tools`, `model`)

### ❓ Ambiguidade não resolvível

- Contradição entre duas rules existentes
- Task que afeta dois domínios cujos Managers discordariam
- Decisão que envolve o **o quê** (estratégico), não o **como** (tático)
- Qualquer situação em que você não consegue decidir sem inferir o que o founder "provavelmente" queria

## Como escalar corretamente

Quando um trigger dispara, apresente ao founder no seguinte formato:

```
## Escalation — [trigger categoria]

**Situação:** [1-2 frases descrevendo onde você está e por que parou]

**Por que parei:** [qual trigger disparou — ex: "ação destrutiva em arquivos não
commitados", "gasto de dinheiro", "mudança em rule do core"]

**Contexto relevante:** [o que o founder precisa saber pra decidir —
sem firula, só fatos]

**Opções:**
- (a) [...]
- (b) [...]
- (c) [...]

**Recomendação:** [qual você acha melhor e por quê — 1-2 frases]

**Decisão necessária:** [o que você precisa do founder pra avançar]
```

Não "disfarce" a escalation como pergunta casual. O founder precisa reconhecer que é um ponto de decisão real.

## Anti-padrões

❌ **Pedir permissão pra tudo.**
Escalation é pra triggers desta lista, não pra cada decisão trivial. Pedir permissão pra cada micro-passo vira fricção. Se não está nesta lista e a rule `think-before-execute` te coloca em modo direto, execute.

❌ **Pular trigger "porque o founder já aprovou algo parecido antes".**
Cada instância é nova. Founder aprovar push pro `main` ontem **não** te dá permissão pra fazer push hoje sem perguntar.

❌ **Escalar sem opções.**
"Não sei o que fazer" é frustrante. Apresente pelo menos 2 opções, mesmo que uma seja "fazer nada". Trabalho do agente é **reduzir** a carga cognitiva do founder, não transferir.

❌ **"Enquanto você pensa eu vou começando."**
Não. Se parou por trigger, parou de verdade. Trabalho em paralelo depois de decisão estrutural quase sempre vira retrabalho.

## Responsabilidade

Esta rule aplica a **todos os agentes sem exceção**. Inclui specialists criados via Hiring Loop — ao serem contratados, eles herdam essa lista automaticamente via `inheritance`.

Leo tem um papel especial: quando um Manager reporta uma task, Leo revisa mentalmente se algum trigger foi (ou poderia ter sido) ativado no meio da execução que deveria ter parado o trabalho. Se identifica, reporta ao founder retroativamente — "Manager X terminou task Y, mas durante a execução houve momento que deveria ter escalado — aqui está".
