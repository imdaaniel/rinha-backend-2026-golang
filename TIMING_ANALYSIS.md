# Análise de Performance - Medições de Tempo

## Configuração do Teste
- Dataset: 1000 referências (em vez de 3 milhões)
- Variável de ambiente: `TIMING_ENABLED=true`
- Arquivo de saída: `/tmp/timings.log`
- Teste k6: 1 VU, 10 segundos, 3544 requisições

## Medições de Construção do Índice (1000 referências)
- **hnsw_creation**: 71 µs
- **hnsw_insert_all**: 2985 µs (2.985ms)
- **index_build**: 3185 µs (3.185ms)

## Medições de Requisições (1772 amostras)

### Tempos Médios
- **json_unmarshal**: 3.21 µs (3.0% do total)
- **vectorize**: 2.04 µs (1.9% do total)
- **hnsw_search**: 4.35 µs (4.1% do total)
- **search (incl. overhead)**: 21.56 µs (20.2% do total)
- **total_request**: 106.82 µs

### Percentis (P99)
- **json_unmarshal**: 19 µs
- **vectorize**: 19 µs
- **hnsw_search**: 34 µs
- **search**: 89 µs
- **total_request**: 212 µs

## Análise do Gargalo

### Gargalo Principal: Overhead do fasthttp
A busca HNSW é muito rápida (4.35 µs em média, apenas 4.1% do tempo total), mas o tempo total da requisição é 106.82 µs. Isso indica que o gargalo principal não é a busca em si, mas sim:

1. **Overhead do fasthttp**: Network I/O, parsing de headers, etc.
2. **Operações não medidas**: Criação da resposta JSON, escrita no socket, etc.

### Observações Importantes
- A construção do índice com 1000 referências leva apenas 3.185ms, o que é muito rápido.
- A busca HNSW é extremamente rápida (4.35 µs em média).
- O tempo total da requisição (106.82 µs) é muito maior que a soma das operações medidas (~31 µs), indicando que ~75% do tempo é gasto em overhead não medido.

### Recomendações
1. **Para o dataset de 3 milhões**: A construção do índice provavelmente será o gargalo principal (escala linearmente com o número de referências).
2. **Para requisições**: O gargalo é o overhead do fasthttp, não a busca HNSW. A busca já está otimizada.
3. **Para melhorar a latência**: Considerar reduzir o overhead do fasthttp ou usar uma biblioteca HTTP mais leve.

## Como Usar as Medições

### Habilitar Timing
```bash
export TIMING_ENABLED=true
export TIMING_FILE=/tmp/timings.log
```

### No Docker Compose
```yaml
environment:
  - TIMING_ENABLED=true
  - TIMING_FILE=/tmp/timings.log
```

### Desabilitar Timing (produção)
```bash
export TIMING_ENABLED=false
# ou simplesmente não definir a variável (padrão é false)
```
