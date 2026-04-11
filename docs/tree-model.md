# Tree Model — Knowledge Architecture

## 1. A Árvore (Visão Geral)

```mermaid
graph TD
    TRUNK["🌳 TRONCO<br/>Identidade do Projeto<br/><i>project.md, brand.md, stack.md</i>"]

    TRUNK --> ENG["galho<br/>Engineering"]
    TRUNK --> DESIGN["galho<br/>Design"]
    TRUNK --> MARKETING["galho<br/>Marketing"]
    TRUNK --> PRODUCT["galho<br/>Product"]

    ENG --> E_DEC["🍃 decisão<br/>migrar p/ Supabase"]
    ENG --> E_DEC2["🍃 decisão<br/>auth via RLS"]
    ENG --> E_BUG["🍃 bug resolvido<br/>JWT sem typ header"]
    ENG --> E_PAT["🍃 padrão<br/>edge functions pattern"]

    DESIGN --> D_DEC["🍃 decisão<br/>design system tokens"]
    DESIGN --> D_DEC2["🍃 decisão<br/>mobile-first"]
    DESIGN --> D_PAT["🍃 padrão<br/>spacing rhythm 4-8-16"]

    MARKETING --> M_DEC["🍃 decisão<br/>tom de voz casual"]
    MARKETING --> M_DEC2["🍃 decisão<br/>canal principal: Instagram"]
    MARKETING --> M_PAT["🍃 padrão<br/>post template"]

    PRODUCT --> P_DEC["🍃 decisão<br/>MVP = 3 features"]
    PRODUCT --> P_DEC2["🍃 decisão<br/>target: iOS 17+"]
    PRODUCT --> P_PAT["🍃 padrão<br/>PRD template"]

    style TRUNK fill:#5D4037,color:#fff,stroke:#3E2723
    style ENG fill:#2E7D32,color:#fff
    style DESIGN fill:#1565C0,color:#fff
    style MARKETING fill:#E65100,color:#fff
    style PRODUCT fill:#6A1B9A,color:#fff

    style E_DEC fill:#A5D6A7
    style E_DEC2 fill:#A5D6A7
    style E_BUG fill:#A5D6A7
    style E_PAT fill:#A5D6A7
    style D_DEC fill:#90CAF9
    style D_DEC2 fill:#90CAF9
    style D_PAT fill:#90CAF9
    style M_DEC fill:#FFCC80
    style M_DEC2 fill:#FFCC80
    style M_PAT fill:#FFCC80
    style P_DEC fill:#CE93D8
    style P_DEC2 fill:#CE93D8
    style P_PAT fill:#CE93D8
```

## 2. As Raízes (Conexões entre galhos)

As raízes são **tags compartilhadas** entre folhas de galhos diferentes.
Dois documentos nunca precisam saber da existência um do outro —
eles se conectam porque compartilham uma tag.

```mermaid
graph LR
    subgraph "Engineering"
        E1["decisão:<br/>auth via RLS<br/><b>tags: auth, supabase</b>"]
    end

    subgraph "Product"
        P1["decisão:<br/>MVP = 3 features<br/><b>tags: auth, mvp</b>"]
    end

    subgraph "Design"
        D1["decisão:<br/>login flow mobile-first<br/><b>tags: auth, mobile</b>"]
    end

    subgraph "Marketing"
        M1["decisão:<br/>launch messaging<br/><b>tags: mvp, launch</b>"]
    end

    E1 ---|"🌿 raiz: auth"| D1
    E1 ---|"🌿 raiz: auth"| P1
    P1 ---|"🌿 raiz: mvp"| M1

    style E1 fill:#A5D6A7
    style P1 fill:#CE93D8
    style D1 fill:#90CAF9
    style M1 fill:#FFCC80
```

## 3. O Ciclo de Vida (Renovação)

```mermaid
graph TD
    subgraph "🍃 Folhas — State (renovam rápido)"
        F1["bug resolvido"]
        F2["decisão tática"]
        F3["status atual de feature"]
    end

    subgraph "🌿 Galhos — Learning (renovam devagar)"
        G1["padrão de código"]
        G2["convenção de design"]
        G3["estratégia de canal"]
    end

    subgraph "🪵 Tronco — Identidade (raramente muda)"
        T1["o que o projeto é"]
        T2["stack core"]
        T3["marca / brand"]
    end

    F1 -.->|"cai e outra nasce<br/>(renovação natural)"| F1
    G1 -.->|"poda consciente<br/>(muda a disciplina)"| G1
    T1 -.->|"corte = recomeço<br/>(pivot do projeto)"| T1

    style F1 fill:#C8E6C9
    style F2 fill:#C8E6C9
    style F3 fill:#C8E6C9
    style G1 fill:#66BB6A,color:#fff
    style G2 fill:#66BB6A,color:#fff
    style G3 fill:#66BB6A,color:#fff
    style T1 fill:#5D4037,color:#fff
    style T2 fill:#5D4037,color:#fff
    style T3 fill:#5D4037,color:#fff
```

## 4. Quem Mantém o Quê

```mermaid
graph LR
    subgraph "IA (gasta tokens — só o que precisa pensar)"
        A1["Escrever conteúdo das folhas"]
        A2["Definir tags no frontmatter"]
        A3["Decisões, análises, julgamento"]
    end

    subgraph "Scripts (zero tokens — trabalho determinístico)"
        S1["Varrer frontmatters"]
        S2["Extrair tags → montar index"]
        S3["Detectar folhas stale"]
        S4["Gerar TREE.md / INDEX.md"]
        S5["Validar que toda folha tem tags"]
    end

    subgraph "Owner (direcionamento humano)"
        O1["Valida podas de galho"]
        O2["Decide mudanças no tronco"]
        O3["Aprova novas tags/categorias"]
    end

    A1 --> S1
    A2 --> S2
    S4 --> A3

    style A1 fill:#FFF9C4
    style A2 fill:#FFF9C4
    style A3 fill:#FFF9C4
    style S1 fill:#E0E0E0
    style S2 fill:#E0E0E0
    style S3 fill:#E0E0E0
    style S4 fill:#E0E0E0
    style S5 fill:#E0E0E0
    style O1 fill:#FFCDD2
    style O2 fill:#FFCDD2
    style O3 fill:#FFCDD2
```

## 5. Navegação do Agent

Como um agent encontra o que precisa sem ler tudo:

```mermaid
sequenceDiagram
    participant Agent
    participant INDEX as INDEX.md<br/>(gerado por script)
    participant Folha as Folha específica
    participant Raiz as Folhas conectadas<br/>(via tag compartilhada)

    Agent->>INDEX: Lê o index (leve, só mapa)
    INDEX-->>Agent: "auth" aparece em 3 folhas:<br/>eng/auth-rls.md<br/>design/login-flow.md<br/>product/mvp-scope.md

    Agent->>Folha: Lê só a folha do seu galho<br/>(eng/auth-rls.md)

    Note over Agent: Precisa de contexto<br/>cross-domain?

    Agent->>Raiz: Segue a raiz "auth"<br/>→ lê design/login-flow.md

    Note over Agent: Agora tem contexto<br/>sem ter lido tudo
```
