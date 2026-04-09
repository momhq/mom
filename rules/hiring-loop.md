---
name: hiring-loop
description: Manager reporta lacuna → Leo contrata specialist → devolve pro Manager executar.
---

## Regra

**Reconhecer lacuna** e **preencher lacuna** são responsabilidades separadas, atribuídas a papéis diferentes.

- Manager identifica que precisa de um specialist que não existe → **reporta** ao Leo (não tenta criar sozinho)
- Leo, com big picture e contexto cross-projeto, **contrata** o specialist formatando o playbook correto → devolve pro Manager
- Manager usa o specialist e **executa** a task

Isso espelha headhunting real: gerente de engenharia diz "preciso de iOS sênior com experiência em push", RH/CTO escreve a JD, busca, entrevista, contrata, entrega. Gerente executa o trabalho com o novo contratado. Separation of concerns.

## Por que Leo contrata e não o Manager

1. **Leo enxerga duplicação.** Se outro Manager já pediu specialist parecido nesta ou em outra sessão, Leo lembra. Manager sozinho não tem esse olhar cruzado.
2. **Leo enxerga reuso cross-projeto.** Se outro projeto em `~/Github/*/` já tem specialist similar, Leo propõe adaptar em vez de criar do zero.
3. **Leo impõe padrão estrutural.** Frontmatter, formato, nível de detalhe. Evita specialists bagunçados feitos por Managers diferentes com estilos diferentes.
4. **Manager fica focado.** Pediu, voltou pra executar a task original. Não perde contexto na meta-tarefa de "escrever um bom specialist".

## Quando Manager dispara hiring loop

**Dois casos legítimos:**

### Caso 1 — Constituir time inicial

Nas primeiras interações de um Manager com um projeto novo, ele identifica quais specialists generalistas precisa baseado na stack e nas tasks esperadas, e dispara Hiring Loop pra constituir o time básico.

Exemplos:
- Dev Manager num projeto React: "preciso de `frontend-react-specialist`, `backend-supabase-specialist`, `deploy-vercel-specialist`"
- Designer Manager num projeto mobile: "preciso de `mobile-ui-specialist`, `app-store-assets-specialist`"

### Caso 2 — Preencher lacuna de domínio específico

Durante execução, Manager encontra task que exige expertise profunda que o time atual não cobre.

Exemplos:
- Task pede integração APNs → Dev Manager pede `apns-push-protocol-specialist`
- Task pede animação Lottie complexa → Designer Manager pede `lottie-animation-specialist`

Em ambos os casos, **specialists vivem 100% no projeto**, nunca no core. Core mantém só managers universais.

## Fluxo passo-a-passo

```
1. Manager, executando task:
   "Essa task envolve [X domínio técnico]. Eu não tenho specialist
    no meu time com playbook pra isso. Preciso disparar Hiring Loop."

2. Manager → Leo:
   "Preciso de um specialist `[nome-proposto]`. Escopo: [o que ele sabe].
    Pra quê: [por que esta task precisa]. Pior cenário se executar sem:
    [consequência de não ter]."

3. Leo verifica:
   - Outro Manager deste projeto já pediu algo parecido recentemente?
   - Outro projeto tem specialist reusável? (olha ~/Github/*/.claude/specialists/)
   - O escopo proposto faz sentido? Muito largo ou muito estreito?

4. Leo formata proposta e apresenta ao founder (R2):
   "Manager [X] pediu specialist `[nome]` com escopo [Y]. Proposta:
    [playbook de 1 página]. Autoriza?"

5. Founder aprova, rejeita, ou pede ajuste.

6. Se aprovado: Leo cria o arquivo em .claude/specialists/{domain}/{name}.md
   no projeto. Devolve pro Manager com referência.

7. Manager lê o specialist como contexto e executa a task.
```

## Formato mínimo de specialist

Quando Leo cria specialist, ele segue formato similar ao dos Managers mas focado em conteúdo técnico acionável:

```markdown
---
name: <specialist name>
description: <o que ele sabe em 1 linha>
domain: <dev|design|marketing|pm|...>
---

## Domínio
[O que este specialist sabe e NÃO sabe]

## Playbook
[Passos, checklist, gotchas, anti-padrões — conteúdo técnico real]

## Referências
[Links, docs oficiais, memórias anteriores, PRs relevantes]

## Self-check
[O que specialist deve verificar antes de reportar pronto]
```

## Anti-padrões

❌ **Manager criando specialist sem passar pelo Leo.**
Sem o olhar cruzado do Leo, você gera duplicatas e inconsistência.

❌ **Specialists genéricos demais.**
"Frontend generalist" que tenta cobrir React + Vue + Svelte é inútil. Seja específico.

❌ **Specialists que replicam conhecimento do Manager.**
Se o Manager já sabe, não precisa specialist. Specialist existe pra conhecimento **profundo** ou **específico** que o Manager não tem.

❌ **Hiring loop pra task de 5 minutos.**
Se a task é tão pequena que criar specialist custa mais que executar com cuidado, o Manager pode executar (com self-QA rigoroso + peer review). Hiring loop é pra tasks onde o custo de errar justifica o custo de contratar.
