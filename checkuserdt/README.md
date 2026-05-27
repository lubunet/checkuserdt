# CheckUserDT Lubunet

CheckUser híbrido para usuário normal Linux e UUID Xray/V2ray.

## Instalação sem git

```bash
bash <(curl -sL https://raw.githubusercontent.com/lubunet/checkuserdt/main/checkuserdt/install.sh)
```

Por padrão o serviço usa:

- Serviço: `checkuser555.service`
- Pasta: `/root/checkuserdt`
- Comando do painel: `chk`
- Link do app: `http://vps.lubunet.shop:2052`

## Painel no terminal

```bash
chk
```

Funções do painel:

- Ver status
- Iniciar
- Parar
- Reiniciar
- Testar usuário ou UUID
- Ver usuários/UUIDs do Xray
- Mostrar link do app
- Ver logs
- Remover script/serviço

## Como funciona

O app deve receber apenas a URL base do CheckUser, por exemplo:

```text
http://vps.lubunet.shop:2052
```

O app envia o usuário normal ou UUID conforme o fluxo dele. O servidor também aceita diretamente:

- `/check/usuario`
- `/check/uuid`
- `/?username=usuario`
- `/?uuid=uuid`
- `/usuario`
- `/uuid`

Para UUID, ele lê `/usr/local/etc/xray/config.json`, encontra o client pelo campo `id`, pega o `email` e consulta a validade do usuário Linux correspondente.
