# CheckUserDT Lubunet

CheckUser hibrido para usuario normal e UUID do Xray/V2ray.

## Instalar na VPS

```bash
bash <(curl -sL https://raw.githubusercontent.com/lubunet/checkuserdt/main/checkuserdt/install.sh)
```

O instalador cria:

- `/root/checkuserdt`
- `/usr/local/bin/checkuserdt`
- `/usr/local/bin/chk`
- `/etc/systemd/system/checkuser555.service`

## Link do app

Use somente o link base exibido ao final da instalacao:

```txt
http://IP_DA_VPS:555
```

O app envia o usuario ou UUID e o CheckUserDT resolve automaticamente.

## Rotas principais

- `GET /check/USUARIO`
- `GET /check/UUID`
- `GET /details/USUARIO_OU_UUID`
- `GET /users`
- `POST /telemetry/heartbeat`

Quando a entrada for UUID, o retorno mostra `username` como o usuario real ligado ao UUID no Xray.

## Painel terminal

```bash
chk
```

Menu:

1. Ver status
2. Iniciar/parar servico automaticamente
3. Reiniciar servico
4. Verificar um usuario ou UUID
5. Usuarios online
6. Remover script

## Usuarios online

A opcao usuarios online depende do app/WebView enviar heartbeat para `/telemetry/heartbeat`.
Existe um exemplo em `docs/dtunnel-heartbeat-example.js`.

## Observacoes da v6

- Corrigido calculo de dias restantes usando a data do `/etc/shadow` como data pura, sem perder 1 dia por causa do horario/fuso.
- Ao validar UUID, o campo `username` volta como o usuario real ligado ao UUID.
- A opcao 5 do `chk` usa as categorias/configs enviadas pelo WebView via heartbeat.
