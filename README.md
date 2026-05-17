# Golang prototype

Prototype do serviço em Go para a Rinha de Backend 2026.

A implementação atual usa um load balancer NGINX na porta `9999` e duas instâncias de API (`api1`, `api2`). Cada API carrega um índice local binário em vez de fazer chamadas HTTP a um sidecar de busca.

## Como rodar localmente

```bash
cd golang
docker compose up --build --force-recreate -d
```

### O que roda

- `lb` — NGINX distribuindo requisições round-robin para as APIs
- `api1` e `api2` — instâncias de serviço Go atendendo `POST /fraud-score`

O servidor expõe `GET /ready` e `POST /fraud-score` na porta `9999`.

## Dados vetoriais

O serviço carrega o dataset real em um formato binário compacto. Se necessário, ele gera `data/references.bin` a partir de `references.json.gz` no primeiro startup, evitando parse JSON em cada inicialização.

## Notas

- A porta `9999` é entregue pelo balanceador
- O `docker-compose.yml` está configurado para manter a soma de recursos em `1 CPU` e `350 MB`
