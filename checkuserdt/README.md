# CheckUserDT Lubunet

CheckUser híbrido para **usuário normal Linux** e **UUID Xray/V2ray**.

## Instalação sem git

```bash
bash <(curl -sL https://raw.githubusercontent.com/lubunet/checkuserdt/main/checkuserdt/install.sh)
```

Padrões:

- Serviço: `checkuser555.service`
- Pasta: `/root/checkuserdt`
- Porta: `555`
- Comando do painel: `chk`
- Link do app: detectado automaticamente como `http://IP_DA_VPS:555`

Se quiser forçar um domínio próprio no link final:

```bash
APP_HOST=vps.lubunet.shop bash <(curl -sL https://raw.githubusercontent.com/lubunet/checkuserdt/main/checkuserdt/install.sh)
```

## Painel no terminal

```bash
chk
```

Funções:

- Ver status
- Iniciar serviço
- Parar serviço
- Reiniciar serviço
- Testar usuário ou UUID
- Ver usuários/UUIDs do Xray
- Mostrar link do app
- Ver logs em tempo real
- Remover script/serviço

## Como funciona

O app deve receber apenas a URL base, por exemplo:

```text
http://IP_DA_VPS:555
```

O app envia o usuário normal ou UUID conforme o fluxo dele. O servidor aceita:

- `/check/usuario`
- `/check/uuid`
- `/?username=usuario`
- `/?uuid=uuid`
- `/usuario`
- `/uuid`

Para UUID, ele lê `/usr/local/etc/xray/config.json`, encontra o client pelo campo `id`, pega o `email` e consulta a validade do usuário Linux correspondente.
