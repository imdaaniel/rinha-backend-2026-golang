# Decisões e justificativas — implementação em Go

Este arquivo registra, de forma concisa, cada decisão técnica tomada até o momento para a solução em `golang/`. O objetivo é manter histórico e razões para facilitar mudanças futuras.

1) Linguagem
- Decisão: usar **Go** (Golang) para a API e vetorizador.
- Justificativa: binário compilado, baixa latência, boa concorrência, runtime pequeno e facilidade para construir containers leves.

2) Vetorização (14 dimensões)
- Decisão: implementar o vetorizador em Go exatamente conforme `REGRAS_DE_DETECCAO.md`.
- Justificativa: garante resultado determinístico e compatível com o dataset de referência; vetorizador é barato e deve ser otimizado para evitar alocações por requisição.

3) Tipos e representações
- Decisão: usar `float32` para vetores e distância (usar distância ao quadrado para ordenação).
- Justificativa: reduz memória pela metade vs `float64`, precisão suficiente para 14 dims; evita sqrt para menor custo computacional.

4) Estratégia de índice
- Decisão inicial: construir índice ANN offline. Prioridade:
  - `IndexIVFPQ` (IVF + Product Quantization) via Faiss (C++) — alta prioridade; ou
  - `HNSW` com parâmetros ajustados, como fallback se memória permitir.
- Justificativa: força bruta em 3M vetores é custosa; IVF+PQ gera índices compactos (bytes por vetor) e baixa latência; HNSW entrega ótima latência mas tende a usar mais RAM.

5) Integração do índice com Go
- Decisão: integrar Faiss via cgo ou executar um processo sidecar C++ que exponha buscas por IPC; alternar conforme restrições de imagem/memória.
- Justificativa: Faiss tem implementações maduras de IVFPQ/HNSW; cgo precisa de cuidado com tamanho da imagem, mas oferece menor latência. Sidecar reduz integração direta com o Go e pode diminuir complexidade de build da imagem.

6) Pré-processamento e construção de índice
- Decisão: executar construção do índice no build do container (ou em job de init) e persistir o binário do índice no container/volume.
- Justificativa: custo de indexação tira trabalho do runtime; index pronto reduz latência de cold-start e permite compartilhar páginas do kernel entre instâncias.

7) Memória e deploy (restrições da Rinha)
- Decisão: projetar índice compacto e usar `mmap` quando possível; declarar recursos pequenos no `docker-compose.yml` e aproveitar page-cache para compartilhamento entre processos.
- Justificativa: soma total ≤ 1 CPU e 350 MB; 3M×14×4B = ~168 MB só de floats — PQ ou compressão necessária para estar confortável com duas instâncias.

8) API e micro-otimizações
- Decisão: handler HTTP em Go com `net/http`, JSON decoding eficiente, `sync.Pool` para buffers e vetores temporários, evitar GC excessivo.
- Justificativa: reduzir latência e variância; evitar 5xx por OOM/GC.

9) Estratégia de fallback
- Decisão: começar com protótipo brute-force (validação funcional) e medir; se não atender latência, integrar índice ANN.
- Justificativa: protótipo confirma vetorização e resposta correta; serve para testes funcionais antes de investir tempo no índice.

10) Medição e objetivos
- Decisão: automatizar bench com o script k6 fornecido; metas iniciais: taxa de erros << 15%, p99 < 50ms; otimizações posteriores miram p99 < 10ms e depois < 1–3ms.

11) Observações operacionais
- Evitar respostas HTTP 5xx; em caso de erro interno preferir responder `approved=true` com `fraud_score=0.0` a retornar 500 (isso reduz impacto em `Err` que pesa 5×).
- Registrar parâmetros do índice (nlist, pq_m, efSearch, M/ef) na documentação para facilitar tuning.

12) Próximos passos (curto prazo)
- Implementar vetorizador em Go e um servidor HTTP mínimo que carrega `normalization.json` e `mcc_risk.json`.
- Criar protótipo brute-force (em memória) para validação funcional com um subconjunto de `references`.
- Rodar o `k6` local para medir p99 e taxa de erros.
- Adotar arquitetura de sidecar para busca vetorial: serviço principal Go chama `/search` no sidecar, que encapsula a busca em índices.

13) Arquitetura escolhida para o próximo estágio
- Decisão: usar **sidecar de busca vetorial** em vez de cgo direto.
- Justificativa: prototipagem mais rápida, menos acoplamento, mudança de índice mais fácil e build Go mais simples.

14) Como o sidecar será usado
- O serviço principal continua atendendo `POST /fraud-score`.
- Ele transforma o payload em vetor e delega a busca para o sidecar.
- O sidecar retorna os rótulos dos K vizinhos mais próximos.
- O serviço principal calcula `fraud_score` e responde.

Essa arquitetura permite trocar o mecanismo de busca do sidecar por Faiss/HNSW/PQ posteriormente sem mudar o serviço HTTP principal.

---
Se quiser que eu registre decisões adicionais (ex.: parâmetros concretos do IVFPQ ou do HNSW após medições), eu atualizo este arquivo com os valores e a razão por trás de cada ajuste.
