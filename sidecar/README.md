# Sidecar de busca vetorial

Este diretório contém um servidor sidecar de busca vetorial que expõe `POST /search` na porta `:9998`.

O sidecar carrega `golang/data/references_example.json` por padrão e responde com os `K` vizinhos mais próximos.

## Como rodar

```bash
cd golang
go build ./sidecar
./sidecar.exe
```

## Endpoints

- `GET /ready` — healthcheck.
- `POST /search` — recebe JSON `{ "vector": [...], "k": 5 }` e responde com `{ "labels": [...], "distances": [...] }`.

## Próximo passo

Esta versão já substitui a busca por força bruta por um índice VP-tree. O servidor carrega o dataset e constrói o índice durante o startup, diminuindo o custo de busca para consultas KNN.
