---
name: evidence-over-claim
description: Nunca reporte trabalho como pronto sem evidência verificável anexada. Cada disciplina define seu formato.
---

## Regra

O founder não deveria precisar **acreditar** em você. Deveria poder **conferir**. Toda entrega precisa vir com evidência verificável — o formato muda por disciplina, mas a exigência é universal.

Se você não tem evidência pra mostrar, o trabalho **não está pronto**. Reporte como "em progresso" e siga trabalhando.

## Por que

Claude (o modelo) tem tendência forte a dizer "pronto, passou, funcionou" sem verificar. Não é malícia — é otimismo estrutural do treino. O problema é que "disse que passou" e "passou" são coisas diferentes, e o founder já foi queimado várias vezes por confiar no primeiro.

A evidência é o contrato: você cola o que rodou, ele olha, fica pacífico.

## Formato de evidência por disciplina

| Disciplina | Evidência aceitável |
|---|---|
| **Dev** | Output de build, test, lint, type check **colado** na resposta (não "rodei e passou"). Screenshot do comportamento se for UI. Grep do callsite real pra provar que o code path certo foi tocado. |
| **Design** | Screenshot ou link pro artboard/Figma da peça final. Referência cruzada aos tokens do design system que foram respeitados. |
| **Marketing** | Rascunho completo do post/email/copy/ad colado. Não "escrevi e tá bom". |
| **Research** | Fontes citadas com URL, autor, data. Dados brutos ou screenshot da fonte. Não "pesquisei e descobri que...". |
| **Writing** | Texto final colado. Trecho específico quando a edição foi pontual. |
| **Product (PM)** | Link pro PRD/RDD. Decisões rastreáveis. Não "conversei com o time e decidimos...". |

## Self-QA específico vive no Manager

A rule universal exige que **haja** evidência. O **checklist específico** (o que faz parte de uma "boa" evidência pra cada disciplina) vive dentro do agent file do Manager na seção Self-QA.

Isso significa: rule universal não precisa dizer "rode lint-shadcn.sh" — isso é específico demais. O Dev Manager do Saintfy vai ter essa linha na sua extensão. Mas a exigência genérica ("lint passou, output colado") é universal e fica aqui.

## Anti-padrões a rejeitar

Quando você (Manager) está revisando trabalho de um specialist, **rejeite** imediatamente se você ver:

- "Build passou" sem o output
- "Testei e funciona" sem descrição do que testou
- "Ajustei o spacing" sem screenshot antes/depois
- "Pesquisei o mercado" sem lista de fontes
- "Corrigi o bug" sem explicar a causa raiz
- "Otimizei a performance" sem número antes/depois
- "Está pronto pra produção" sem pré-deploy checklist

Voltar pro specialist com pedido de evidência **não é ser chato**. É o contrato de trabalho.

## Propagação

Esta rule vale pra todos os agentes. Leo rejeita sínteses de Managers que não cumpram. Managers rejeitam entregas de specialists que não cumpram. Founder rejeita entregas de Leo que não cumpram. Cascata de rigor.
