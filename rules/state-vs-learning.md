---
name: state-vs-learning
description: Memories de estado envelhecem rápido. Memories de aprendizado permanecem. Trate cada uma diferente.
---

## Regra

Memories são de duas naturezas diferentes. Tratá-las igual é como se livros de história e manuais técnicos fossem indexados pela mesma regra — você perde a nuance que importa.

- **State memories** descrevem o estado factual do projeto num ponto no tempo (o que foi feito, o que está pendente, quem tem qual responsabilidade, qual é a pendência atual). **Envelhecem rápido.** Precisam ser revalidadas antes de serem citadas como fato.
- **Learning memories** descrevem aprendizados, regras de conduta, decisões de trabalho, padrões que deram ou não deram certo. **Envelhecem devagar.** São quase atemporais.

## Como distinguir

**State memory** (envelhece rápido):
- Descreve o que está feito vs pendente
- Contém lista de issues abertas
- Menciona versão atual do app, release cadence
- Nomeia quem está trabalhando em quê
- Cita métrica num momento específico ("X usuários", "Y% de conversão")
- Registra decisão que pode ser revertida ou evoluir
- Tem seção "Pendente"

**Learning memory** (envelhece devagar):
- Descreve regra de conduta ("sempre grep callsite real antes de refactor")
- Explica padrão observado ("iCloud Drive + cap sync = duplicatas `* 2`")
- Captura lição de falha ("JWT sem `typ: 'JWT'` quebra APNs")
- Documenta preferência do founder ("não quer toasts pra erros de form, usa inline")
- Define vocabulário ou convenção do projeto

## Como aplicar

### Quando escrever uma memory

Antes de salvar, classifique mentalmente: **state ou learning**? Se for state, marque data explicitamente no corpo e assume que vai envelhecer. Se for learning, escreva de forma atemporal quando possível.

### Quando ler uma memory

- **State memory**: **sempre verifique contra o estado atual** antes de agir sobre ela. Se a memory diz "pendente: feature X" e você vai implementar, primeiro confirma que ainda está pendente. Se diz "issue #12 está em progresso", confirme via `gh issue view 12` antes de citar.
- **Learning memory**: pode citar com mais confiança, mas ainda assim cuidado com lições que foram superadas por aprendizado mais recente.

### Quando propagação afeta memories

Quando você atualiza uma state memory porque ela envelheceu, aplica a rule `propagation` — atualize, não crie nova. Quando uma learning memory é refinada por uma falha nova (via `know-what-you-dont-know` Mecanismo 4), pode criar nova que suplanta a anterior, mas marque a antiga como superseded.

## Anti-padrão: memory como "último snapshot"

Se você está tentado a escrever uma memory que diz "status atual do projeto: X, Y, Z, pendente A, B", pare. Isso vai virar lixo em 1 semana. Pense se tem alguma **lição** genuína no que você quer salvar. Se não tiver, provavelmente não deveria ser memory — deveria ser um report pontual que vive no `outputs/` daquela sessão.

## Responsabilidade

Leo é o responsável por auditar memories periodicamente, identificar state memories stale, atualizar ou remover. Managers podem propor esse trabalho quando notarem que estão carregando memory desatualizada que conflita com estado observado.

**Regra prática:** se uma memory tem seção "Pendente" ou "Next steps", ela é quase certamente state e merece revisão a cada 1-2 semanas.
