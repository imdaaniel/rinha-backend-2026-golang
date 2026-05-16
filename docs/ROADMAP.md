# Roadmap da solução Go

## O que já foi feito

1. Leitura e interpretação do desafio.
2. Criação do protótipo inicial em Go com:
   - `GET /ready`
   - `POST /fraud-score`
   - vetorização de payloads em 14 dimensões.
   - cálculo de `fraud_score` com KNN brute-force em um conjunto de exemplo.
3. Criação de um benchmark simples em `golang/bench` para medir p50/p90/p99 e taxa de erros.
4. Implementação de arquitetura de sidecar em `golang/sidecar`:
   - sidecar expõe `POST /search`
   - serviço principal Go consome o sidecar para buscar os K vizinhos.
   - fallback local para brute-force quando o sidecar não estiver disponível.
5. Adição de documentação e decisões técnicas em `golang/docs`.
6. Criação de `Dockerfile` e `docker-compose.yml` para rodar API + sidecar.

## Próximos passos

1. Preparar o dataset real:
   - descompactar `base/resources/references.json.gz`
   - validar formato e compatibilidade com a vetorização.
2. Construir o índice de busca vetorial:
   - usar Faiss / HNSW / IVFPQ para reduzir o custo da busca em 3 milhões de vetores.
3. Substituir a busca brute-force do sidecar por um índice real:
   - manter a interface HTTP do sidecar para o serviço Go.
4. Ajustar parâmetros do índice para maximizar a pontuação:
   - `nlist`, `pq_m`, `efSearch`, `M`, `ef` etc.
5. Medir novamente com `bench` e com o script de teste oficial (`k6`).
6. Otimizar uso de memória e CPU para caber no limite de 1 CPU e 350 MB.
7. Preparar `docker-compose.yml` final para submissão.

## Como testar a API localmente

### 1. Rodar o sidecar

```bash
cd golang
go build ./sidecar
./sidecar.exe
```

### 2. Rodar a API principal

```bash
cd golang
go build .
./golang.exe
```

### 3. Verificar prontidão

```bash
curl -v http://localhost:9999/ready
```

### 4. Testar `POST /fraud-score`

```bash
curl -sS -X POST http://localhost:9999/fraud-score \
  -H "Content-Type: application/json" \
  -d '{
    "id":"tx-3330991687",
    "transaction":{"amount":9505.97,"installments":10,"requested_at":"2026-03-14T05:15:12Z"},
    "customer":{"avg_amount":81.28,"tx_count_24h":0,"known_merchants":[]},
    "merchant":{"id":"MERC-068","mcc":"7802","avg_amount":54.86},
    "terminal":{"is_online":false,"card_present":true,"km_from_home":952.27},
    "last_transaction":null
  }'
```

### 5. Testar via `docker-compose`

```bash
docker compose -f golang/docker-compose.yml up --build
```

Depois de subir, use:

```bash
curl http://localhost:9999/ready
```

e o mesmo `POST /fraud-score` acima.

## Impacto de latência do sidecar

A chamada HTTP para o sidecar adiciona overhead extra, mas:

- se o sidecar estiver na mesma máquina/container, o custo é tipicamente de poucas centenas de microssegundos a poucos milissegundos;
- esse custo é muito menor do que a própria busca em 3 milhões de vetores se ela for brute-force;
- o lado positivo é que a implementação pode usar um índice otimizado (Faiss/IVFPQ/HNSW) e reduzir dramaticamente a maior parte do trabalho.

Em outras palavras:

- sim, há algum aumento de latência pela chamada HTTP;
- não é necessariamente um problema se o sidecar fizer a busca muito mais rápida do que o brute-force local;
- se for preciso, o overhead pode ser reduzido mais tarde com IPC mais leve (Unix socket, gRPC ou cgo), mas a arquitetura atual é boa para prototipagem e tuning.

## Observação

O foco do desafio é a pontuação combinada de latência e taxa de erro. O sidecar permite trocar o mecanismo de busca internamente sem alterar a API externa, o que é valioso para testar diferentes índices e alcançar o melhor equilíbrio entre velocidade e precisão.
