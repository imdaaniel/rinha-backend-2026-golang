# Golang prototype

Prototype do serviço em Go para a Rinha de Backend 2026.

Como rodar localmente:

```bash
go build ./golang
./golang
```

O servidor expõe `GET /ready` e `POST /fraud-score` na porta `:9999`.

O protótipo atualmente usa um sidecar de busca vetorial disponível em `golang/sidecar`. O sidecar expõe `POST /search` na porta `:9998` e responde com os rótulos dos vizinhos mais próximos.

Para rodar localmente, inicie o sidecar em um terminal e o serviço principal em outro:

```bash
cd golang
go build ./sidecar
./sidecar.exe
```

```bash
cd golang
go build .
./golang.exe
```

O serviço principal usa o sidecar por padrão e faz fallback para brute-force local se o sidecar estiver indisponível.
