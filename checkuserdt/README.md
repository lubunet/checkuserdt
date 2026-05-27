# CheckUserDT Lubunet

CheckUser híbrido para usuário normal e UUID do Xray/V2ray, usando porta 555.

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

Use somente o link base exibido ao final da instalação:

```txt
http://IP_DA_VPS:555
```

O app envia/captura o usuário ou UUID automaticamente. Quando a entrada for UUID, o retorno mostra `username` como o usuário real ligado ao UUID no Xray.

## Rotas principais

- `GET /check/USUARIO`
- `GET /check/UUID`
- `GET /check?username=USUARIO`
- `GET /check?uuid=UUID`
- `GET /details/USUARIO_OU_UUID`
- `GET /users`
- `POST /telemetry/heartbeat`
- `GET /online/categories`
- `GET /online/category?category=NOME`

## Painel terminal

```bash
chk
```

Menu:

1. Ver status
2. Iniciar/parar serviço automaticamente
3. Reiniciar serviço
4. Verificar um usuário ou UUID
5. Usuários online
6. Ver link do app
7. Remover script

## Usuários online

A opção 5 mostra:

- usuários validados recentemente pelo próprio CheckUser, na categoria `CheckUser`;
- usuários com dados completos enviados pelo WebView/app em `/telemetry/heartbeat`.

Sem heartbeat do WebView, o servidor consegue mostrar o usuário validado recentemente, mas não consegue adivinhar config, IP local, Wi-Fi ou operadora. Para dados completos, use o exemplo em `docs/dtunnel-heartbeat-example.js`.

## Observações da v7

- Adicionada opção 6 no `chk`: ver link do app.
- Remoção movida para opção 7.
- Corrigida a opção 5 para mostrar também a categoria `CheckUser` e qualquer categoria real que vier do app.
- A lista online agora usa janela de 10 minutos para validações recentes.
- Corrigido cálculo de dias restantes usando a data do `/etc/shadow` como data pura.


## WebView heartbeat

Use o arquivo `docs/webviewer-heartbeat-patch.js` ou o HTML de exemplo para enviar `/telemetry/heartbeat`. Quando `vpn_state` vem como `DISCONNECTED`, o usuário sai dos online imediatamente.
