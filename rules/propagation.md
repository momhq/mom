---
name: propagation
description: Toda decisão ou mudança de contexto deve propagar pros arquivos impactados antes de fechar a task
---

## Regra

Nenhuma task está completa até que o contexto do sistema reflita o que mudou. Se você tomou uma decisão, mudou uma stack, definiu um padrão novo, ou aprendeu algo que vale pra próxima vez — isso precisa ser materializado antes de reportar "pronto".

**Informação desatualizada é pior que informação ausente.** Ela induz erro silencioso.

## Quando aplicar

Propague sempre que a task incluir:

- **Decisão** de produto, design, tech, negócio ou marketing
- **Mudança de stack** (nova lib, migração, arquitetura alterada)
- **Novo padrão** (convenção, template, rule de domínio)
- **Mudança de contexto** (status do projeto, persona, concorrente novo, decisão revertida)
- **Asset ou template criado** que outros agentes podem reusar
- **Aprendizado** de falha que teria sido evitada com rule ou checklist

## Quando disparar o checklist completo

Sessões conversacionais não têm fim explícito. Cada turn é um potencial ponto de parada. Por isso o checklist de propagação **não roda após cada tool call nem após cada decisão individual** — isso seria ruidoso, caro, e arriscaria persistir estado intermediário que ainda vai mudar.

O checklist roda em **três situações**, com pesos diferentes:

### 1. Principal — sinal explícito de fim de sessão do founder

Quando o founder sinaliza que a sessão (ou o bloco de trabalho atual) acabou, Leo **invoca a skill `session-wrap-up`**, que roda o protocolo inteiro de forma determinística.

Exemplos de sinal em linguagem natural (a lista não é exaustiva — Leo reconhece a *intenção* em qualquer idioma, não a frase exata):

- "fecha a sessão" / "fecha aí" / "pode fechar"
- "vamos consolidar" / "consolida aí" / "salva isso"
- "acho que tá bom por hoje" / "tá bom assim" / "pronto por hoje"
- "próxima vez continua daqui" / "daqui a pouco volto"
- "manda bala, fecha" / "finaliza"
- "wrap up", "let's consolidate", "save this", "done for today", "we're good"

Se o sinal é ambíguo (ex: founder fala "ok" depois de uma task terminar — pode ser "ok, done for today" ou "ok, continua"), Leo **pergunta uma vez** antes de invocar a skill: *"Você quer que eu feche a sessão agora (rodar wrap-up) ou é só uma pausa?"*.

### 2. Secundário — propagação oportunística pontual

Quando uma decisão **claramente locked** é tomada mid-sessão — ex: founder fala "iOS 17 é o floor, ponto final" ou "decisão: TabBar está fora, drawer fica" — Leo pode propagar **apenas aquela decisão** imediatamente, criando ou editando o arquivo relevante (ex: `context/decisions/...`). Isso **não** dispara o checklist completo nem invoca a skill; é só capturar um fato isolado que não vai mudar.

Critério pra decidir se é "claramente locked":

- Founder usou palavra explícita de fechamento ("ponto final", "locked", "decidido", "não reabre")
- OU a decisão foi explicitamente contraposta a alternativas e uma foi escolhida com rationale
- **Em dúvida, esperar o wrap-up.** Propagar incremental demais re-cria o problema que essa rule resolve.

### 3. Safety net — pergunta única em sessão longa

Se Leo percebe que a sessão está ficando longa (muitas decisões acumuladas, contexto crescendo, múltiplos turns sem nenhum sinal de fechamento), pergunta **uma vez**: *"Tá juntando bastante coisa. Quer que eu feche a sessão agora ou seguimos?"*. Só isso — não insista. O safety net existe pra cobrir o caso "founder entrou no flow e esqueceu de fechar", não pra interromper ritmo.

**Anti-padrão:** Leo **nunca** roda o checklist completo sem um dos três gatilhos acima. Propagar mid-task por iniciativa própria é como commitar a cada linha de código — fere o ponto da rule.

## Como aplicar

Ao fechar uma task que se enquadra acima:

1. **Identifique** o que mudou (decisão, fato, padrão)
2. **Consulte** `context/propagation-map.md` do projeto (se existir) ou use o checklist mental abaixo pra mapear arquivos impactados
3. **Atualize** cada arquivo impactado
4. **Verifique** com grep que não restam referências stale
5. **Reporte** ao founder: "Propagação feita — atualizei X, Y, Z"

Quando o gatilho é o **sinal explícito de fim de sessão** (situação 1 acima), Leo invoca a skill `session-wrap-up`, que orquestra esses passos de forma determinística (inventário → classificação → plano → R2 → execução → report). Ver `~/.claude/skills/session-wrap-up/SKILL.md` (fonte em `copilot-core/skills/session-wrap-up/`).

## Checklist mental (Leo roda mentalmente ao fechar qualquer task)

- [ ] Alguma decisão foi tomada? → `context/decisions/{domínio}.md`
- [ ] O que o projeto **é** mudou? → `context/project.md`, `context/brand.md`
- [ ] A stack mudou? → `context/stack.md`
- [ ] Algum manager precisa saber disso? → agent file do manager (ou memória)
- [ ] Algum workflow referencia o que mudou?
- [ ] Alguma rule do projeto precisa update?
- [ ] Alguma memory está agora stale? → atualizar ou remover
- [ ] Algum doc canônico (PRD, RDD) referencia algo que mudou?

## O que NÃO propagar

- Detalhes de implementação que vivem no código (código é fonte de verdade)
- Estado temporário de task em andamento
- Informação que já está no git history
- Outputs de sessão (vão pra `outputs/`)

## Responsabilidade final

**Leo é o responsável final pela propagação.** Managers podem fazer propagação no escopo deles (rule de domínio, memory específica), mas Leo é quem garante que nada passou batido. Se founder cobrar propagação faltante, Leo responde — não o Manager.
