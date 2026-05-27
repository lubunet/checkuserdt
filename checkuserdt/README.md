# CheckUserDT / CheckUser 555

CheckUser híbrido para DTunnel/Xray:

- Mantém consulta por usuário normal: `/check/usuario`
- Adiciona consulta por UUID do Xray/V2ray: `/check/uuid`
- Lê UUID e usuário em `/usr/local/etc/xray/config.json`
- Lê validade do usuário Linux em `/etc/shadow`
- Roda na porta `555`
- Cria serviço `checkuser555.service`
- Cria painel de terminal com o comando `chk`
- Cria painel web simples em `/panel`

## Estrutura

```txt
checkuserdt/
├── cmd/
│   └── checkuserdt/
│       └── main.go
├── install.sh
├── uninstall.sh
├── go.mod
└── README.md
```

## Instalação local na VPS

```bash
cd /root/checkuserdt
sudo bash install.sh
```

## Instalação pelo GitHub

Depois de subir este projeto no seu GitHub, edite o `REPO_URL` no `install.sh` ou rode passando a variável:

```bash
bash <(curl -sL https://raw.githubusercontent.com/SEU_USUARIO/checkuserdt/main/install.sh)
```

Ou:

```bash
REPO_URL="https://github.com/SEU_USUARIO/checkuserdt.git" bash <(curl -sL https://raw.githubusercontent.com/SEU_USUARIO/checkuserdt/main/install.sh)
```

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

## Exemplos

```bash
curl http://127.0.0.1:555/health
curl http://127.0.0.1:555/check/isja
curl http://127.0.0.1:555/check/cb4c6db6-0712-42ca-a9ae-976fbc7b6041
curl http://127.0.0.1:555/users
```

## Serviço

```bash
sudo systemctl status checkuser555.service -l --no-pager
sudo systemctl restart checkuser555.service
sudo systemctl stop checkuser555.service
sudo systemctl start checkuser555.service
```

## Painel terminal

```bash
chk
```

## Limite de conexões

Se quiser definir limite manual por usuário, crie um arquivo com o número:

```bash
mkdir -p /root/checkuserdt/limits
echo 1 > /root/checkuserdt/limits/isja
```

## Observação sobre stats Xray

Para tráfego por usuário funcionar, seu Xray precisa ter `StatsService`, `stats: {}` e `policy.levels.0.statsUserUplink/statsUserDownlink` ativados. O campo `email` do client precisa estar preenchido, pois ele é usado como nome do usuário nas estatísticas.
