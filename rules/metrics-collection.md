---
name: metrics-collection
description: Coletar 5 métricas operacionais por task pra informar refinamento do sistema no futuro.
---

## Regra

Toda task executada deixa **uma entrada de métrica** num arquivo JSONL específico do projeto. Isso cria o dataset que vai permitir, no futuro, refinar os agents e rules baseado em dados reais — não em achismo.

**Onde:** `<projeto>/.claude/metrics/<YYYY-MM>.jsonl` (um arquivo por mês, uma linha por task)

**Responsabilidade:** Leo é quem escreve a entrada, ao fechar a task e sintetizar pro founder. Managers passam os dados brutos pro Leo como parte do report final.

## Por que

Inspirado no paradigma do autoresearch do Karpathy: qualquer sistema que se pretende refinável precisa de **fitness function mensurável**. Sem métricas, refinar o core vira achismo — "acho que o Dev Manager precisa dessa rule a mais". Com métricas, a gente consegue olhar os piores números e ir direto na dor.

Duas utilidades imediatas:

1. **Decisão sobre onde refinar.** Depois de ~20-30 tasks, founder e Leo revisam os números. Qual métrica está pior? Essa é a área que precisa de ajuste no core ou nas rules.
2. **Input pro loop offline de auto-refinamento** (horizonte 2 do RDD §8.10). Quando houver volume suficiente, dá pra construir benchmark a partir das tasks registradas e rodar refinamento deliberado de um Manager contra histórico real.

## As 5 métricas iniciais

### 1. Peer review pass rate
**O que mede:** % de tasks que passam no peer review **na primeira tentativa** (sem rework).
**Coleta:** a instância que faz o review registra "approved" ou "rejected" + contagem de iterações.
**Campo:** `review.first_pass` (bool), `review.iterations` (int)

### 2. Founder rejection rate
**O que mede:** % de entregas finais em que o founder rejeita ou pede mudança substancial após Leo reportar "pronto".
**Coleta:** Leo registra se, depois de reportar, o founder precisou pedir ajuste no mesmo turn ou turn seguinte.
**Campo:** `founder.accepted_on_delivery` (bool)

### 3. Self-QA honesty rate
**O que mede:** % de tasks em que o self-QA do agente executor foi honesto — ou seja, disse que passou E de fato passou no review.
**Coleta:** comparar o output do self-QA (todos os checks marcados ✅) com o resultado do peer review. Desonesto = self-QA disse "tudo ok" mas review achou problema.
**Campo:** `self_qa.honest` (bool)

### 4. Rework cycles
**O que mede:** número de idas e vindas entre executor e reviewer (ou entre Leo e founder) até a task fechar.
**Coleta:** Leo conta.
**Campo:** `rework_cycles` (int)

### 5. Hiring loop hit rate
**O que mede:** % de tasks onde o Manager reconheceu corretamente uma lacuna (e disparou Hiring Loop) **vs** % onde tentou sem specialist e errou por isso.
**Coleta:** quando peer review ou founder identifica que uma falha foi por falta de specialist, essa task conta como "hit rate miss". Tasks que dispararam Hiring Loop contam como "hit".
**Campo:** `hiring_loop.outcome` (string: "triggered" | "missed" | "na")

## Formato da entrada JSONL

Cada linha do arquivo é um JSON válido auto-contido:

```json
{
  "task_id": "2026-04-08-001",
  "timestamp": "2026-04-08T15:30:00Z",
  "founder_prompt_summary": "Adicionar tela de settings com toggle de dark mode",
  "manager": "dev",
  "specialist_used": "frontend-react-specialist",
  "domain_category": "ui_puro",
  "review": {
    "first_pass": true,
    "iterations": 1
  },
  "self_qa": {
    "honest": true
  },
  "founder": {
    "accepted_on_delivery": true
  },
  "rework_cycles": 0,
  "hiring_loop": {
    "outcome": "na"
  },
  "duration_minutes_approximate": 12,
  "notes": "Task clara, executou no primeiro passe."
}
```

**Campos obrigatórios:** `task_id`, `timestamp`, `manager`, `review`, `founder`, `rework_cycles`, `hiring_loop`
**Campos opcionais:** `specialist_used`, `domain_category`, `self_qa`, `duration_minutes_approximate`, `notes`

## Como aplicar

### Fim de task (Leo)

Antes de reportar pronto ao founder:

1. Colete os dados brutos de cada participante da task (Manager, specialists, reviewer)
2. Compile uma entrada JSONL seguindo o formato acima
3. Escreva (append) em `<projeto>/.claude/metrics/<YYYY-MM>.jsonl`
   - Criar arquivo se não existir
   - Criar diretório `.claude/metrics/` se não existir
4. Reporte ao founder normalmente (a coleta de métrica é transparente)

### Revisão periódica

A cada 20-30 tasks (ou uma vez por mês), founder pode pedir a Leo um **relatório de métricas**:

```
Leo, me mostra os números de métricas deste mês.
```

Leo lê o JSONL do mês, agrega, apresenta:
- Contagem total de tasks
- Taxa de cada métrica
- Piores casos (tasks com muito rework, review rejeitado, hiring loop missed)
- Padrões (alguma categoria de domínio concentra falhas?)

Esse é o input pra próxima conversa de refinamento do core.

## O que NÃO medir

- **Tempo real de execução** — Claude Code já não é determinístico no tempo, e o founder não liga se uma task demora 5 ou 15 minutos. `duration_minutes_approximate` é opcional e grosso.
- **Qualidade subjetiva** — "o código ficou bonito?" não vira métrica. Métricas são binárias ou contáveis.
- **Número de arquivos mudados** — correlação fraca com qualidade. Task grande e bem feita é melhor que task pequena mal feita.
- **Tokens consumidos** — vai variar demais, e a decisão de refinamento não deve ser otimizar custo em detrimento de qualidade.

## Privacidade

Logs de métrica vivem dentro do `.claude/metrics/` do projeto, **gitignored**. Não são commitados, não vão pro repo. São locais do founder e servem à análise do founder.

## Responsabilidade

Esta rule depende do Leo ser disciplinado em coletar e escrever. Se Leo esquecer de registrar tasks, o dataset fica furado e as decisões de refinamento serão baseadas em amostra viesada. Founder pode cobrar periodicamente: "Leo, mostra as últimas 10 entradas do metrics.jsonl" — se faltar, é sinal pra reforçar.
