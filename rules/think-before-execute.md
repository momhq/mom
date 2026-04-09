---
name: think-before-execute
description: Em tasks ambíguas ou arquiteturais, pergunte antes de implementar. Em tasks diretas, execute.
---

## Regra

Antes de executar, decida em qual dos dois modos você está:

- **Modo direto** — task é clara e limitada: executa.
- **Modo alinhamento** — task tem ambiguidade, decisão arquitetural, ou mudança de comportamento não especificada: apresenta abordagem e espera aprovação antes de escrever código.

Não confunda os dois. Pedir permissão pra tudo vira fricção e o founder perde paciência. Executar decisão arquitetural sem alinhar gera retrabalho e frustração.

## Critério

**Modo direto** quando:
- Instrução clara e limitada ("troca essa cor pra gold", "renomeia esse arquivo", "adiciona esse texto aqui")
- Bug fix óbvio com causa raiz identificada
- Ajuste pontual em feature existente (conhecido o que é, onde é, como funciona)
- Você consegue descrever o diff final numa frase antes de começar

**Modo alinhamento** quando:
- Task envolve decisão de arquitetura, padrão, ou comportamento que o founder não especificou
- Mais de uma forma razoável de implementar, com trade-offs reais
- A task afeta múltiplos arquivos de forma não trivial
- Você precisa "inferir" o que o founder quis dizer em algum ponto
- Task é vaga ("melhore isso", "deixe mais rápido") sem métricas ou critério

## Como aplicar modo alinhamento

Não escreva código. Escreva:

1. **Resumo da task** — como você entendeu (1-2 frases)
2. **Decisão(ões) em jogo** — o que precisa ser decidido
3. **Opções** — no mínimo 2, com prós e contras concretos
4. **Recomendação** — qual você acha melhor e por quê
5. **Pergunta específica pro founder** — o que você precisa dele pra avançar

Espere resposta. Não vá "começando enquanto decide" — se o founder mudar a direção, você vai ter jogado trabalho fora.

## Por que a auto-checagem importa

Claude (o modelo) tem tendência a **modo direto por default**. Quer resolver, quer entregar. Isso funciona pra 70% das tasks, mas falha catastroficamente nos 30% que precisam de alinhamento — porque o modelo "decide sozinho" em pontos onde o founder era o decisor.

A auto-checagem não é opcional. É a única ferramenta que você tem pra resistir ao viés de execução.

## Exemplos

✅ **Modo direto:** "Muda a cor do botão primary pra gold na tela de login." → vai direto.

⚠️ **Modo alinhamento:** "Muda a cor primária do app." → para. "Isso afeta 40 componentes. Primary é variável CSS em index.css, posso mudar lá e propaga. Mas também tem uses hardcoded em alguns lugares que precisam refactor. Quer que eu (a) só atualize o token e deixe hardcoded como follow-up, (b) faça tudo de uma vez, ou (c) outra coisa?"

✅ **Modo direto:** "Adiciona log do erro na edge function `send-push`." → vai direto.

⚠️ **Modo alinhamento:** "Melhore o error handling do app." → para. Muito vago — precisa definir escopo, padrão, onde.
