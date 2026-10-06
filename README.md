# track-message-bot

Bot de Discord que registra a última mensagem de cada pessoa do servidor.

## Comandos

- `/ultima @usuario`: quando, onde e o que a pessoa falou por último
- `/ranking`: quem está há mais tempo sem falar
- `/status`: último save e se houve restauração do backup

## Setup

1. No [Developer Portal](https://discord.com/developers/applications), crie a aplicação e, na aba **Bot**:
   - copie o token;
   - ligue **Message Content Intent** e **Server Members Intent**.
2. Em **OAuth2 → URL Generator**, marque os escopos `bot` e `applications.commands` e as permissões
   *View Channels*, *Send Messages* e *Read Message History*. Abra a URL gerada e adicione o bot ao servidor.
3. No Discord, ative o Modo Desenvolvedor (Configurações → Avançado) e use "Copiar ID" no servidor e no canal de relatórios.
4. No homelab:

```sh
cp .env.example .env   # preencha
mkdir -p data          # precisa pertencer ao PUID/PGID do .env
docker compose up -d --build
docker compose logs -f
```

## Armazenamento

`data/state.json` é o estado principal e `data/state.json.bak` é a última versão validada.
Se o principal estiver ausente ou corrompido na inicialização, ele é movido para `state.json.corrupt-<ts>`,
recriado a partir do `.bak` e a flag `meta.restored_from_backup` é preenchida. A restauração aparece no `/status`
e é avisada no canal de relatórios.

## Jobs

Só rodam com `REPORT_CHANNEL_ID` preenchido. O horário segue a variável `TZ`.

- Domingo às 20h: relatório semanal com o ranking.
- Todo dia às 12h: menciona quem passou de `INACTIVE_DAYS` dias sem falar (uma vez por período de silêncio).
