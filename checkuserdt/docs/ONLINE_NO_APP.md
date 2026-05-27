# Usuários online no CheckUserDT

A opção 5 do `chk` mostra dois tipos de registro:

1. **CheckUser**: usuário/UUID que passou recentemente pelo `/check`. Isso já funciona sem mexer no WebView.
2. **Heartbeat**: dados completos enviados pelo WebView/app para `/telemetry/heartbeat`.

O servidor sozinho não consegue descobrir qual config do app foi selecionada, IP local, Wi‑Fi, TIM/Vivo/Claro etc. Esses dados estão disponíveis dentro da bridge/WebView do DTunnel e precisam ser enviados pelo app.

Se o app só chamar o CheckUser uma vez ao conectar, o usuário aparecerá como `CheckUser` por até 10 minutos. Para online real, deixe o heartbeat ativo a cada 30 segundos.
