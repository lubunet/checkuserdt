# CheckUserDT Lubunet

CheckUser híbrido para usuário normal e UUID do Xray/V2ray, usando porta 555.

Esta versão não tem opção de usuários online no painel `chk`.

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

- `GET /health`
- `GET /check/USUARIO`
- `GET /check/UUID`
- `GET /check?username=USUARIO`
- `GET /check?uuid=UUID`
- `GET /details/USUARIO_OU_UUID`
- `GET /users`
- `GET /count`

## Painel terminal

```bash
chk
```

Menu:

1. Ver status
2. Iniciar/parar serviço automaticamente
3. Reiniciar serviço
4. Verificar um usuário ou UUID
5. Ver link do app
6. Remover script

## Observações

- Serviço systemd: `checkuser555.service`.
- Pasta principal: `/root/checkuserdt`.
- Porta padrão: `555`.
- Cálculo de dias restantes por data pura, evitando diferença de 1 dia por horário/fuso.
- Para UUID Xray/V2ray, o campo `username` retorna o usuário real salvo no campo `email` do client no `/usr/local/etc/xray/config.json`.
