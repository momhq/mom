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

## Como aplicar

Ao fechar uma task que se enquadra acima:

1. **Identifique** o que mudou (decisão, fato, padrão)
2. **Consulte** `context/propagation-map.md` do projeto (se existir) ou use o checklist mental abaixo pra mapear arquivos impactados
3. **Atualize** cada arquivo impactado
4. **Verifique** com grep que não restam referências stale
5. **Reporte** ao founder: "Propagação feita — atualizei X, Y, Z"

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
