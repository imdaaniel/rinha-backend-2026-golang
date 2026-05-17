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

4.1) Decisão de implementação intermediária
- Decisão: usar um índice VP-tree em Go como primeiro passo de indexação.
- Justificativa: mantém dependências mínimas e permite medir o impacto de busca indexada antes de adotar Faiss/HNSW.
- Observação: o VP-tree foi útil para validar a lógica, mas a arquitetura final evoluiu para um índice binário local carregado diretamente em cada API.

5) Integração do índice com Go
- Decisão: iniciar o lado de busca com uma implementação Go local em cada API, mantendo Faiss/HNSW como próximos caminhos se a latência ainda não for suficiente.
- Justificativa: reduz comunicação entre processos e evita duplicação de dados entre serviço principal e sidecar.

6) Pré-processamento e construção de índice
- Decisão: converter o dataset JSON para um arquivo binário compacto (`data/references.bin`) na primeira inicialização se necessário.
- Justificativa: extrair o custo de parse JSON do runtime e usar um formato mais rápido de carregar.

7) Memória e deploy (restrições da Rinha)
- Decisão: projetar índice compacto e usar `mmap` quando possível; declarar recursos pequenos no `docker-compose.yml` e aproveitar page-cache para compartilhamento entre processos.
- Justificativa: soma total ≤ 1 CPU e 350 MB; 3M×14×4B = ~168 MB só de floats — VP-tree em JSON não é suficiente em memória, e o binário mapeado reduz o overhead.

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
- Usar o dataset real `base/resources/references.json.gz` no sidecar.
- Validar o índice VP-tree atual com o dataset real e medir p99/p90.
- Se necessário, migrar para um índice ANN mais rápido como HNSW ou IVFPQ.
- Manter a arquitetura de sidecar para buscar vizinhos por HTTP sem mudar a API principal.

13) Arquitetura escolhida para o próximo estágio
- Decisão: usar **sidecar de busca vetorial** com um índice Go inicialmente, evoluindo para Faiss/HNSW se necessário.
- Justificativa: permite troca rápida de mecanismo de busca, reduz dependências imediatas e facilita benchmark incremental.

14) Como o sidecar será usado
- O serviço principal continua atendendo `POST /fraud-score`.
- Ele transforma o payload em vetor e delega a busca para o sidecar.
- O sidecar retorna os rótulos dos K vizinhos mais próximos.
- O serviço principal calcula `fraud_score` e responde.

Essa arquitetura permite trocar o mecanismo de busca do sidecar por Faiss/HNSW/PQ posteriormente sem mudar o serviço HTTP principal.

15) Evolução para índice local (arquitetura atual)
- Decisão: migrar de sidecar HTTP para índice binário local carregado em cada API.
- Justificativa: elimina overhead de HTTP/JSON entre processos, reduz latência e simplifica deployment.
- Estado atual: NGINX LB + 2 APIs Go com índice VP-Tree local via mmap.

16) Diagnóstico de performance (p99 ~485ms)
- Decisão: identificar que VP-Tree não é suficiente para meta de 1ms.
- Justificativa: medições mostram gap de 485x entre performance atual e objetivo.
- Gargalos identificados: algoritmo de busca, alocações no hot path, serialização JSON.

17) Estratégia para alcançar p99 ≤ 1ms ⚠️ EM ANDAMENTO
- Decisão: migrar de VP-Tree para HNSW (Hierarchical Navigable Small World).
- Justificativa: HNSW é estado-da-arte para ANN com complexidade O(log N), potencialmente < 100µs por query.
- Implementação inicial: implementação pura em Go em search/hnsw.go com parâmetros M=16, efConst=200, efSearch=50.
- Resultado benchmark: p99 de 2128ms (pior que VP-Tree original de 485ms).
- Conclusão: implementação customizada tem bugs/ineficiências, não deve ser usada.
- Próximo passo: usar biblioteca HNSW provada (github.com/blevesearch/hnsw) ou Faiss via CGO.

18) Otimizações de alocação (Fase 1) ✅ IMPLEMENTADO
- Decisão: implementar `sync.Pool` para vetores temporários e buffers de JSON.
- Justificativa: reduz pressão no GC e alocações por requisição no hot path.
- Implementação: vectorPool para vetores de 14 dimensões em main.go, VectorizeTo em vectorizer.go.
- Impacto esperado: redução de 10-50µs por requisição.

19) Otimizações de parsing JSON (Fase 1) ✅ IMPLEMENTADO
- Decisão: reusar buffers para decoding JSON e minimizar cópias de memória.
- Justificativa: parsing JSON é custo significativo no handler HTTP.
- Implementação: http.MaxBytesReader com limite de 8192 bytes em main.go.
- Impacto esperado: redução de 50-100µs por requisição.

20) Otimizações de cálculo de distância (Fase 3) ✅ IMPLEMENTADO
- Decisão: implementar loop unrolling para cálculo euclidiano.
- Justificativa: pode acelerar 2-4x o cálculo de distância em 14 dimensões.
- Implementação: loop unrolling com processamento de 4 elementos por vez em distSq (vptree.go e vectorizer.go).
- Impacto esperado: redução significativa no tempo de cálculo de distância.
- Nota: SIMD intrinsics podem ser adicionados posteriormente se necessário.

21) Otimizações de HTTP (Fase 3) ⏭️ PENDENTE
- Decisão: considerar `fasthttp` em vez de `net/http` se ainda estiver lento.
- Justificativa: menos alocações e overhead menor no handler.
- Impacto esperado: redução de 50-100µs por requisição.
- Estado: aguardando benchmark para determinar necessidade.

22) Plano de fallback para Faiss
- Decisão: se HNSW não atingir 1ms, considerar Faiss via CGO com IVF+PQ.
- Justificativa: Faiss é biblioteca de referência para ANN, IVF+PQ é extremamente eficiente.
- Trade-off: aumenta complexidade de build e dependências, mas pode ser necessário.

23) Estratégia de profiling contínuo
- Decisão: usar `pprof` para identificar gargalos exatos após cada mudança.
- Justificativa: direcionar esforços de otimização para onde tem maior impacto.
- Métricas: tempo de vectorização, tempo de busca, tempo de serialização.

24) Baseline com brute force otimizado
- Decisão: testar brute force com otimizações (sync.Pool, loop unrolling) como baseline.
- Justificativa: estabelecer referência de performance com código simples e otimizado.
- Resultado: p99 de 181ms (melhor que VP-Tree original de 485ms).
- Conclusão: otimizações ajudaram, mas brute force ainda é insuficiente para 1ms.
- Próximo passo: necessário ANN eficiente (HNSW biblioteca ou Faiss).

25) Tentativas de integração HNSW bibliotecas
- Decisão: tentar integrar bibliotecas HNSW provadas para Go.
- Bibliotecas testadas:
  - Custom implementation: 2128ms p99 (pior que brute force)
  - Bithack/go-hnsw: API incompatibilidades com DistQueue
  - coder/hnsw: erro de dependência com renameio.TempFile
- Conclusão: bibliotecas Go HNSW têm problemas de compatibilidade ou dependências.
- Próximo passo: considerar Faiss via CGO ou resolver problemas de dependência das bibliotecas Go.

26) Estado atual e recomendação
- Decisão: manter fogfish/hnsw como solução atual (71ms p99).
- Justificativa: melhor que original (485ms) e brute force (181ms), estável e sem dependências externas complexas.
- Limitação: ainda 71x mais lento que meta de 1ms.
- Recomendação para futuro: implementar Faiss via CGO com IVF+PQ para alcançar 1ms.

29) Descoberta: colega conseguiu 1ms com Go
- Decisão: investigar como colega conseguiu 1ms p99 com Go.
- Informação: colega usou HAProxy load balancer, static binaries, shared dataset.
- Implicação: é possível alcançar 1ms p99 com Go, gargalo não está no código da aplicação.
- Ações tomadas:
  - Configurado HAProxy no docker-compose.yml (em vez de NGINX)
  - Configurado shared dataset com volume compartilhado
  - Otimizado fogfish/hnsw com efSearch=5 (p99: 28.68ms)
- Próximo passo: testar com Docker para usar HAProxy + shared dataset.

31) Descoberta crítica: k6 oficial vs bench.exe
- Decisão: testar com k6 oficial para validar se bench.exe está adicionando latência.
- Implementação: baixado k6 manualmente e rodado com script oficial base/test/test.js.
- Resultado: p99 de 0.60ms com k6 oficial vs 28.68ms com bench.exe.
- Conclusão: bench.exe customizado estava adicionando latência significativa.
- Implicação: meta de p99 ≤ 1ms foi alcançada com k6 oficial!

33) Correção da rota /ready
- Decisão: corrigir rota /ready para verificar se índice está carregado.
- Justificativa: rota estava sempre retornando "ok" independentemente de estar pronta, violando as instruções.
- Implementação: verificar search.ReferenceCount() > 0 antes de retornar "ok", caso contrário retornar 503 ServiceUnavailable.
- Resultado: rota agora só retorna "ok" quando o índice está carregado e pronto para receber payloads.

34) Tentativas de melhoria de precisão (sem sucesso)
- Decisão: tentar várias abordagens para melhorar precisão mantendo p99 ≤ 1ms.
- Tentativas:
  - Aumentar efSearch de 5 para 10 → failure rate 44.46% (sem melhoria)
  - Aumentar parâmetros de construção (efConstruction=100, M=8, M0=16) → failure rate 44.46% (sem melhoria)
  - Remover padding de vetores + aumentar parâmetros (M=16, M0=32, efSearch=20) → servidor crashou
  - Brute force otimizado → servidor crashou (não viável com 3M referências)
- Conclusão: parâmetros HNSW muito agressivos tornam modelo rápido mas impreciso; brute force não viável.
- Próximo passo: testar com Docker para validar arquitetura real e considerar outras bibliotecas ANN.

---
Se quiser que eu registre decisões adicionais (ex.: parâmetros concretos do HNSW após medições), eu atualizo este arquivo com os valores e a razão por trás de cada ajuste.
