# CheckUserDT / CheckUser 555

CheckUser hibrido para DTunnel/Xray:

- Funciona com usuario Linux normal: `/check/usuario`
- Funciona com UUID do Xray/V2ray: `/check/uuid`
- Le o Xray em `/usr/local/etc/xray/config.json`
- Usa o campo `email` do client Xray como usuario Linux
- Consulta a validade em `/etc/shadow`
- Roda na porta `555`
- Cria o servico `checkuser555.service`
- Cria painel de terminal pelo comando `chk`

## Estrutura esperada no GitHub

Como o repositorio e `lubunet/checkuserdt`, envie esta pasta `checkuserdt` para dentro da raiz do repositorio:

```txt
checkuserdt/
├── cmd/
│   └── checkuserdt/
│       └── main.go
├── scripts/
│   └── chk
├── install.sh
├── uninstall.sh
├── go.mod
├── .gitattributes
└── README.md
```

Assim o link de instalacao fica:

```bash
bash <(curl -sL https://raw.githubusercontent.com/lubunet/checkuserdt/main/checkuserdt/install.sh)
```

## Instalar na VPS sem git

```bash
bash <(curl -sL https://raw.githubusercontent.com/lubunet/checkuserdt/main/checkuserdt/install.sh)
```

O instalador usa `curl` + `unzip` para baixar o ZIP do GitHub. Ele nao instala nem usa `git`.

## Link do app

```txt
http://IP_DA_VPS:555/check/USUARIO_OU_UUID
```

Exemplo com usuario normal:

```txt
http://IP_DA_VPS:555/check/isja
```

Exemplo com UUID Xray:

```txt
http://IP_DA_VPS:555/check/cb4c6db6-0712-42ca-a9ae-976fbc7b6041
```

## Comando do painel

```bash
chk
```

Funcoes do `chk`:

1. Ver status do servico
2. Testar `/health`
3. Testar usuario normal ou UUID
4. Mostrar usuarios Xray encontrados
5. Mostrar links do app
6. Iniciar servico
7. Parar servico
8. Reiniciar servico
9. Ver logs em tempo real
10. Remover script/servico

## Rotas

```txt
GET /health
GET /check/USUARIO_OU_UUID
GET /details/USUARIO_OU_UUID
GET /stats/USUARIO_OU_UUID
GET /users
GET /count
GET /panel
```

## Systemd

Servico criado:

```txt
checkuser555.service
```

Comandos:

```bash
sudo systemctl status checkuser555.service -l --no-pager
sudo systemctl start checkuser555.service
sudo systemctl stop checkuser555.service
sudo systemctl restart checkuser555.service
```

## Remover

Pelo painel:

```bash
chk
```

Ou direto:

```bash
sudo bash /root/checkuserdt/uninstall.sh
```
