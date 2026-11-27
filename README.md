# seabird-discord-backend

## Permissions

The bot needs View Channels, Send Messages and Read Message History in the
channels it's used in.

With Manage Webhooks as well, messages relayed from other chat services are
posted under their author's name, as "alice (IRC)", through a webhook in each
channel. Without it, the bot posts them itself with the author's name at the
start of the message.
